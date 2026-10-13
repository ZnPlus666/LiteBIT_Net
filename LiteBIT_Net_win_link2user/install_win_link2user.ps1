param([switch]$LibraryOnly)
$ErrorActionPreference = 'Stop'

function Read-CheckIntervalSeconds {
    while ($true) {
        $Value = (Read-Host '检查间隔（分钟，默认10） / Check interval (minutes, default 10)').Trim()
        if (-not $Value) { return 600 }
        $Minutes = 0
        if ($Value -match '^[0-9]+$' -and [int]::TryParse($Value, [ref]$Minutes) -and $Minutes -ge 1 -and $Minutes -le 35791394) {
            return ($Minutes * 60)
        }
        Write-Host '请输入有效的正整数分钟数。 / Enter a valid positive whole number of minutes.' -ForegroundColor Yellow
    }
}

function New-UserConfiguration(
    [string]$Username,
    [Security.SecureString]$Password1,
    [Security.SecureString]$Password2,
    [ValidateRange(15, 2147483647)][int]$IntervalSeconds
) {
    $Pointer1 = [Runtime.InteropServices.Marshal]::SecureStringToBSTR($Password1)
    $Pointer2 = [Runtime.InteropServices.Marshal]::SecureStringToBSTR($Password2)
    try {
        $Plain = [Runtime.InteropServices.Marshal]::PtrToStringBSTR($Pointer1)
        $Confirm = [Runtime.InteropServices.Marshal]::PtrToStringBSTR($Pointer2)
        if (-not $Username) { throw '账号不能为空。 / The account must not be empty.' }
        if (-not $Plain -or $Plain -cne $Confirm) { throw '密码为空或两次输入不一致。 / The password is empty or the passwords do not match.' }
        if ($Username -match '[^\x20-\x7E]' -or $Plain -match '[^\x20-\x7E]') {
            throw '账号和密码仅支持可打印 ASCII 字符。 / Only printable ASCII characters are supported for the account and password.'
        }
        $Raw = [Text.Encoding]::UTF8.GetBytes($Plain)
        try { $Encrypted = [Security.Cryptography.ProtectedData]::Protect($Raw, $null, [Security.Cryptography.DataProtectionScope]::CurrentUser) }
        finally { [Array]::Clear($Raw, 0, $Raw.Length) }
        return [ordered]@{
            server = 'http://10.0.0.55'
            username = $Username
            password_dpapi = [Convert]::ToBase64String($Encrypted)
            ac_id = '8'
            check_interval_seconds = $IntervalSeconds
        }
    } finally {
        $Plain = $null
        $Confirm = $null
        [Runtime.InteropServices.Marshal]::ZeroFreeBSTR($Pointer1)
        [Runtime.InteropServices.Marshal]::ZeroFreeBSTR($Pointer2)
    }
}

function Invoke-SetupVerification(
    [System.Collections.IDictionary]$Candidate,
    [string]$PasswordField,
    [Security.Cryptography.DataProtectionScope]$Scope,
    [string]$MonitorSource,
    [string]$VerificationDataDir
) {
    . $MonitorSource -LibraryOnly -DataDir $VerificationDataDir
    $Once = $true
    $script:Config = [pscustomobject]$Candidate
    $Raw = [Security.Cryptography.ProtectedData]::Unprotect([Convert]::FromBase64String($Candidate[$PasswordField]), $null, $Scope)
    try { $script:Password = [Text.Encoding]::UTF8.GetString($Raw) }
    finally { [Array]::Clear($Raw, 0, $Raw.Length) }
    try {
        Write-Host '正在验证新凭据…… / Verifying new credentials...' -ForegroundColor Cyan
        $Result = Test-NewCredentials
        if ($Result -eq 'DisconnectRequired') {
            $Answer = Read-Host '进行账号密码正确性验证将注销当前登录！输入 Y 确认验证，输入 N 或其他跳过验证。 / Verifying the account and password will sign out of the current session! Enter Y to verify, or N or anything else to skip verification.'
            if ($Answer -notin @('Y', 'y')) {
                Write-MonitorLog 'VERIFY_SKIPPED 已跳过账号密码正确性验证。 / Account/password verification skipped.'
                return 'Skipped'
            }
            $Result = Test-NewCredentials -AllowDisconnect
        }
        if ($Result -eq 'Unknown') {
            $Answer = Read-Host '是否继续创建任务？输入 Y 确认，输入 N 或其他中止。 / Continue creating the task? Enter Y to confirm, or N or anything else to abort.'
            if ($Answer -notin @('Y', 'y')) { return 'Cancelled' }
            Write-MonitorLog 'VERIFY_CONTINUED 验证结果未知，继续安装。 / Verification result unknown; continuing installation.'
            return 'Unverified'
        }
        return $Result
    } finally { $script:Password = $null }
}

function Stop-TaskForVerification([string]$Name) {
    Stop-ScheduledTask -TaskName $Name
    for ($Attempt = 0; $Attempt -lt 20; $Attempt++) {
        $Task = Get-ScheduledTask -TaskName $Name -ErrorAction SilentlyContinue
        if (-not $Task -or $Task.State -notin @('Running', 'Queued')) { return }
        Start-Sleep -Milliseconds 250
    }
    throw '无法停止原任务，未验证新凭据。 / Cannot stop the previous task; new credentials were not verified.'
}

function Resume-PreviousTask([string]$Name) {
    Write-Host ('正在恢复原任务…… / Restoring previous task... ' + $Name)
    try {
        Start-ScheduledTask -TaskName $Name -ErrorAction Stop
        for ($Attempt = 0; $Attempt -le 5; $Attempt++) {
            $Task = Get-ScheduledTask -TaskName $Name -ErrorAction Stop
            if (-not $Task -or $Task.State -eq 'Disabled') { break }
            if ($Task.State -eq 'Running') {
                Write-Host '原任务已恢复。 / Previous task restored.' -ForegroundColor Green
                return
            }
            if ($Attempt -lt 5) { Start-Sleep -Milliseconds 1000 }
        }
    } catch {
        Write-Warning '原任务恢复失败。 / Previous task restoration failed.'
        return
    }
    Write-Warning '原任务恢复失败。 / Previous task restoration failed.'
}

Add-Type -AssemblyName System.Security
if ($LibraryOnly) { return }

$TaskName = 'SRun Auto Login'
$AppDir = Join-Path $env:LOCALAPPDATA 'SrunAutoLogin'
$PowerShellExe = Join-Path $env:SystemRoot 'System32\WindowsPowerShell\v1.0\powershell.exe'
$TargetScript = Join-Path $AppDir 'monitor_win_link2user.ps1'
$LauncherExe = Join-Path $AppDir 'launcher_win_link2user.exe'

trap {
    Write-Host ''
    Write-Host '安装失败。 / Installation failed.' -ForegroundColor Red
    Write-Host ('错误 / Error: ' + $_.Exception.Message) -ForegroundColor Red
    exit 1
}

foreach ($File in @('monitor_win_link2user.ps1', 'crypto_win_link2user.cs', 'launcher_win_link2user.cs')) {
    if (-not (Test-Path -LiteralPath (Join-Path $PSScriptRoot $File))) {
        throw "缺少程序文件 / Program file missing: $File"
    }
}
Write-Host '请输入校园网账号和密码。 / Enter your campus-network account and password.' -ForegroundColor Cyan
$Username = (Read-Host '校园网账号 / Campus-network account').Trim()
$Password1 = Read-Host '校园网密码 / Campus-network password' -AsSecureString
$Password2 = Read-Host '再次输入密码 / Repeat password' -AsSecureString
try {
    $IntervalSeconds = Read-CheckIntervalSeconds
    $Configuration = New-UserConfiguration $Username $Password1 $Password2 $IntervalSeconds
} finally {
    $Password1.Dispose()
    $Password2.Dispose()
}

$MachineTask = Get-ScheduledTask -TaskName 'SRun Machine Auto Login' -ErrorAction SilentlyContinue
if ($MachineTask -and $MachineTask.State -in @('Running', 'Queued')) {
    throw '请先卸载关联电脑版，再验证关联用户版。 / Uninstall link2PC before verifying link2user.'
}
New-Item -ItemType Directory -Path $AppDir -Force | Out-Null
$ConfigPath = Join-Path $AppDir 'config.json'
$HadConfig = Test-Path -LiteralPath $ConfigPath
$PreviousConfig = if ($HadConfig) { [IO.File]::ReadAllBytes($ConfigPath) } else { $null }
$ExistingTask = Get-ScheduledTask -TaskName $TaskName -ErrorAction SilentlyContinue
$WasRunning = $ExistingTask -and $ExistingTask.State -in @('Running', 'Queued')
$Completed = $false
$ConfigCommitted = $false
$TaskChanged = $false
$TempConfigPath = Join-Path $AppDir ('config-' + [Guid]::NewGuid().ToString('N') + '.tmp')
$TempLauncher = Join-Path $AppDir ('launcher-' + [Guid]::NewGuid().ToString('N') + '.exe')
try {
    if ($ExistingTask -and $ExistingTask.State -ne 'Disabled') { Stop-TaskForVerification $TaskName }
    $VerificationResult = Invoke-SetupVerification $Configuration 'password_dpapi' ([Security.Cryptography.DataProtectionScope]::CurrentUser) (Join-Path $PSScriptRoot 'monitor_win_link2user.ps1') $AppDir
    if ($VerificationResult -eq 'Cancelled') { throw '安装已取消。 / Installation cancelled.' }
    if ($VerificationResult -notin @('Verified', 'Skipped', 'Unverified')) {
        throw '新凭据验证失败，未保存新配置。 / New credential verification failed; new configuration was not saved.'
    }
    foreach ($File in @('monitor_win_link2user.ps1', 'crypto_win_link2user.cs')) {
        Copy-Item -LiteralPath (Join-Path $PSScriptRoot $File) -Destination (Join-Path $AppDir $File) -Force
    }
    Add-Type -Path (Join-Path $PSScriptRoot 'launcher_win_link2user.cs') -OutputAssembly $TempLauncher -OutputType WindowsApplication
    Move-Item -LiteralPath $TempLauncher -Destination $LauncherExe -Force
    $Configuration | ConvertTo-Json | Set-Content -LiteralPath $TempConfigPath -Encoding UTF8
    $ConfigCommitted = $true
    Move-Item -LiteralPath $TempConfigPath -Destination $ConfigPath -Force
    Write-Host '配置已保存。 / Configuration saved.'
    $ActionOptions = @{
        Execute = $LauncherExe
        Argument = '-DataDir "' + $AppDir + '"'
        WorkingDirectory = $AppDir
    }
    $Action = New-ScheduledTaskAction @ActionOptions
    $Trigger = New-ScheduledTaskTrigger -AtLogOn -User "$env:USERDOMAIN\$env:USERNAME"
    $SettingsOptions = @{
        AllowStartIfOnBatteries = $true
        DontStopIfGoingOnBatteries = $true
        StartWhenAvailable = $true
        MultipleInstances = 'IgnoreNew'
        RestartCount = 3
        RestartInterval = New-TimeSpan -Minutes 1
        ExecutionTimeLimit = [TimeSpan]::Zero
    }
    $Settings = New-ScheduledTaskSettingsSet @SettingsOptions
    $Principal = New-ScheduledTaskPrincipal -UserId "$env:USERDOMAIN\$env:USERNAME" -LogonType Interactive -RunLevel Limited
    $TaskOptions = @{
        TaskName = $TaskName
        Action = $Action
        Trigger = $Trigger
        Settings = $Settings
        Principal = $Principal
        Description = '关联用户版 SRun 认证：用户登录后运行，按配置间隔检查。 / User-linked SRun authentication: runs after Windows sign-in and checks at the configured interval.'
        Force = $true
    }
    $TaskChanged = $true
    Register-ScheduledTask @TaskOptions | Out-Null
    Enable-ScheduledTask -TaskName $TaskName | Out-Null
    Write-Host '正在启动任务…… / Starting task...'
    Start-ScheduledTask -TaskName $TaskName
    Start-Sleep -Seconds 8
    if ((Get-ScheduledTask -TaskName $TaskName).State -ne 'Running') { throw '任务未运行。 / The task is not running.' }
    $Completed = $true
} finally {
    if (-not $Completed) {
        if ($TaskChanged) {
            try {
                Stop-ScheduledTask -TaskName $TaskName -ErrorAction SilentlyContinue
                if ($ExistingTask) { Register-ScheduledTask -TaskName $TaskName -InputObject $ExistingTask -Force | Out-Null }
                elseif (Get-ScheduledTask -TaskName $TaskName -ErrorAction SilentlyContinue) { Unregister-ScheduledTask -TaskName $TaskName -Confirm:$false }
            } catch { Write-Warning '旧任务恢复失败。 / Previous task restoration failed.' }
        }
        if ($ConfigCommitted) {
            try {
                if ($HadConfig) {
                    [IO.File]::WriteAllBytes($TempConfigPath, $PreviousConfig)
                    Move-Item -LiteralPath $TempConfigPath -Destination $ConfigPath -Force
                } elseif (Test-Path -LiteralPath $ConfigPath) { Remove-Item -LiteralPath $ConfigPath -Force }
            } catch { Write-Warning '旧配置恢复失败。 / Previous configuration restoration failed.' }
        }
        if ($WasRunning) { Resume-PreviousTask $TaskName }
    }
    foreach ($TemporaryPath in @($TempConfigPath, $TempLauncher)) {
        if (Test-Path -LiteralPath $TemporaryPath) {
            try { Remove-Item -LiteralPath $TemporaryPath -Force }
            catch { Write-Warning '临时文件清理失败。 / Temporary file cleanup failed.' }
        }
    }
}
Write-Host '安装完成。 / Installed.' -ForegroundColor Green
Write-Host "计划任务 / Task: $TaskName"
Write-Host "程序目录 / Program directory: $AppDir"
Write-Host "日志文件 / Log file: $(Join-Path $AppDir 'srun-auto-login.log')"
