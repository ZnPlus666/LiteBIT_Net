$ErrorActionPreference = 'Stop'
try {
    $Identity = [Security.Principal.WindowsIdentity]::GetCurrent()
    $PrincipalCheck = New-Object Security.Principal.WindowsPrincipal($Identity)
    if (-not $PrincipalCheck.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) {
        throw '需要管理员权限。 / Administrator privileges are required.'
    }
    $Task = Get-ScheduledTask -TaskName 'SRun Machine Auto Login' -ErrorAction SilentlyContinue
    if ($Task) {
        Stop-ScheduledTask -TaskName $Task.TaskName
        Unregister-ScheduledTask -TaskName $Task.TaskName -Confirm:$false
        Write-Host '卸载完成。 / Uninstalled.'
    } else {
        Write-Host '未找到计划任务。 / Scheduled task not found.'
    }
    Write-Host "保留的文件 / Retained files: $(Join-Path $env:ProgramFiles 'SrunMachineLogin'); $(Join-Path $env:ProgramData 'SrunMachineLogin')"
} catch {
    Write-Host '卸载失败。 / Uninstall failed' -ForegroundColor Red
    Write-Host ('错误 / Error: ' + $_.Exception.Message) -ForegroundColor Red
    exit 1
}
