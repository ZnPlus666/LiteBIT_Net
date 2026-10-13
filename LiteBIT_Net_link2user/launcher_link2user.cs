using System;
using System.ComponentModel;
using System.Diagnostics;
using System.IO;
using System.Runtime.InteropServices;
using System.Text;
using Microsoft.Win32.SafeHandles;

// Compiled as a WindowsApplication, not a console application. The supervisor
// remains alive so Task Scheduler tracks the actual monitor's lifetime.
public static class LiteBITUserLauncher
{
    private sealed class JobHandle : SafeHandleZeroOrMinusOneIsInvalid
    {
        public JobHandle() : base(true) { }
        protected override bool ReleaseHandle() { return CloseHandle(handle); }
    }

    [StructLayout(LayoutKind.Sequential)]
    private struct BasicLimits
    {
        public long ProcessTime, JobTime;
        public uint Flags;
        public UIntPtr MinWorkingSet, MaxWorkingSet;
        public uint ActiveProcesses;
        public UIntPtr Affinity;
        public uint PriorityClass, SchedulingClass;
    }

    [StructLayout(LayoutKind.Sequential)]
    private struct IoCounters
    {
        public ulong ReadOperations, WriteOperations, OtherOperations;
        public ulong ReadBytes, WriteBytes, OtherBytes;
    }

    [StructLayout(LayoutKind.Sequential)]
    private struct ExtendedLimits
    {
        public BasicLimits Basic;
        public IoCounters Io;
        public UIntPtr ProcessMemory, JobMemory, PeakProcessMemory, PeakJobMemory;
    }

    [DllImport("kernel32.dll", CharSet = CharSet.Unicode, SetLastError = true)]
    private static extern JobHandle CreateJobObject(IntPtr attributes, string name);
    [DllImport("kernel32.dll", SetLastError = true)]
    [return: MarshalAs(UnmanagedType.Bool)]
    private static extern bool SetInformationJobObject(JobHandle job, int kind, ref ExtendedLimits limits, uint size);
    [DllImport("kernel32.dll", SetLastError = true)]
    [return: MarshalAs(UnmanagedType.Bool)]
    private static extern bool AssignProcessToJobObject(JobHandle job, IntPtr process);
    [DllImport("kernel32.dll", SetLastError = true)]
    [return: MarshalAs(UnmanagedType.Bool)]
    private static extern bool CloseHandle(IntPtr handle);

    private static string Quote(string value)
    {
        StringBuilder result = new StringBuilder("\"");
        int slashes = 0;
        foreach (char c in value)
        {
            if (c == '\\') { slashes++; continue; }
            if (c == '"') result.Append('\\', slashes * 2 + 1);
            else result.Append('\\', slashes);
            slashes = 0;
            result.Append(c);
        }
        result.Append('\\', slashes * 2);
        return result.Append('"').ToString();
    }

    [STAThread]
    public static int Main(string[] args)
    {
        string dataDir = Path.Combine(Environment.GetFolderPath(Environment.SpecialFolder.LocalApplicationData), "SrunAutoLogin");
        try
        {
            if (args.Length != 2 || args[0] != "-DataDir") throw new ArgumentException("Invalid launcher arguments");
            dataDir = Path.GetFullPath(args[1]);
            string codeDir = AppDomain.CurrentDomain.BaseDirectory;
            string script = Path.Combine(codeDir, "monitor_link2user.ps1");
            if (!File.Exists(script)) throw new FileNotFoundException("Monitor missing");
            string powershell = Path.Combine(Environment.GetFolderPath(Environment.SpecialFolder.System), @"WindowsPowerShell\v1.0\powershell.exe");
            using (JobHandle job = CreateJobObject(IntPtr.Zero, null))
            {
                if (job.IsInvalid) throw new Win32Exception(Marshal.GetLastWin32Error());
                ExtendedLimits limits = new ExtendedLimits();
                limits.Basic.Flags = 0x2000; // JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
                if (!SetInformationJobObject(job, 9, ref limits, (uint)Marshal.SizeOf(typeof(ExtendedLimits))))
                    throw new Win32Exception(Marshal.GetLastWin32Error());
                ProcessStartInfo options = new ProcessStartInfo(powershell);
                options.Arguments = "-NoLogo -NoProfile -NonInteractive -ExecutionPolicy Bypass -File " + Quote(script) + " -DataDir " + Quote(dataDir);
                options.WorkingDirectory = codeDir;
                options.UseShellExecute = false;
                options.CreateNoWindow = true;
                using (Process monitor = Process.Start(options))
                {
                    try
                    {
                        if (monitor.HasExited) return monitor.ExitCode;
                        if (!AssignProcessToJobObject(job, monitor.Handle)) throw new Win32Exception(Marshal.GetLastWin32Error());
                        monitor.WaitForExit();
                        return monitor.ExitCode;
                    }
                    catch
                    {
                        try { if (!monitor.HasExited) { monitor.Kill(); monitor.WaitForExit(); } } catch { }
                        throw;
                    }
                }
            }
        }
        catch (Exception failure)
        {
            // Never log exception messages or command lines containing secrets.
            try
            {
                string line = DateTime.Now.ToString("yyyy-MM-dd HH:mm:ss") + " LAUNCH_FAILED 后台启动失败 / Background launch failed; type=" + failure.GetType().Name + Environment.NewLine;
                File.AppendAllText(Path.Combine(dataDir, "srun-auto-login.log"), line, Encoding.UTF8);
            }
            catch { }
            return 1;
        }
    }
}
