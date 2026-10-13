$ErrorActionPreference = 'Stop'
$TaskName = 'SRun Machine Auto Login'
$CodeDir = Join-Path $env:ProgramFiles 'SrunMachineLogin'
$DataDir = Join-Path $env:ProgramData 'SrunMachineLogin'
$PowerShellExe = Join-Path $env:SystemRoot 'System32\WindowsPowerShell\v1.0\powershell.exe'

trap {
    Write-Host ''
    Write-Host '安装失败。 / Installation failed.' -ForegroundColor Red
    Write-Host ('错误 / Error: ' + $_.Exception.Message) -ForegroundColor Red
    exit 1
}

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

function Assert-PlainPath([string]$Path) {
    $Current = [IO.Path]::GetFullPath($Path)
    while ($Current) {
        if (Test-Path -LiteralPath $Current) {
            $Item = Get-Item -LiteralPath $Current -Force
            if (($Item.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0) {
                throw "拒绝使用重定向或链接安装路径 / Refusing a redirected or linked installation path: $Current"
            }
        }
        $Parent = Split-Path -Parent $Current
        if ($Parent -eq $Current) { break }
        $Current = $Parent
    }
}

function Protect-AppPath([string]$Path, [bool]$Directory) {
    Assert-PlainPath $Path
    if ($Directory) { $Acl = New-Object Security.AccessControl.DirectorySecurity }
    else { $Acl = New-Object Security.AccessControl.FileSecurity }
    $Acl.SetAccessRuleProtection($true, $false)
    $AdminSid = New-Object Security.Principal.SecurityIdentifier('S-1-5-32-544')
    $Acl.SetOwner($AdminSid)
    foreach ($SidText in @('S-1-5-18', 'S-1-5-32-544')) {
        $Sid = New-Object Security.Principal.SecurityIdentifier($SidText)
        if ($Directory) {
            $Rule = New-Object Security.AccessControl.FileSystemAccessRule($Sid, 'FullControl', 'ContainerInherit,ObjectInherit', 'None', 'Allow')
        } else {
            $Rule = New-Object Security.AccessControl.FileSystemAccessRule($Sid, 'FullControl', 'Allow')
        }
        $Acl.AddAccessRule($Rule)
    }
    Set-Acl -LiteralPath $Path -AclObject $Acl
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

$Identity = [Security.Principal.WindowsIdentity]::GetCurrent()
$PrincipalCheck = New-Object Security.Principal.WindowsPrincipal($Identity)
if (-not $PrincipalCheck.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) {
    throw '需要管理员权限。 / Administrator privileges are required.'
}
foreach ($File in @('monitor_link2PC.ps1', 'crypto_link2PC.cs')) {
    if (-not (Test-Path -LiteralPath (Join-Path $PSScriptRoot $File))) {
        throw "缺少程序文件 / Program file missing: $File"
    }
}
Add-Type -AssemblyName System.Security
if (-not ('SrunMachineCrypto' -as [type])) { Add-Type -Path (Join-Path $PSScriptRoot 'crypto_link2PC.cs') }

Write-Host '请输入校园网账号和密码。 / Enter your campus-network account and password.' -ForegroundColor Cyan
$Username = (Read-Host '校园网账号 / Campus-network account').Trim()
$Password1 = Read-Host '校园网密码 / Campus-network password' -AsSecureString
$Password2 = Read-Host '再次输入密码 / Repeat password' -AsSecureString
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
    try { $Encrypted = [Security.Cryptography.ProtectedData]::Protect($Raw, $null, [Security.Cryptography.DataProtectionScope]::LocalMachine) }
    finally { [Array]::Clear($Raw, 0, $Raw.Length) }
} finally {
    $Plain = $null
    $Confirm = $null
    [Runtime.InteropServices.Marshal]::ZeroFreeBSTR($Pointer1)
    [Runtime.InteropServices.Marshal]::ZeroFreeBSTR($Pointer2)
    $Password1.Dispose()
    $Password2.Dispose()
}

$IntervalSeconds = Read-CheckIntervalSeconds

$Configuration = [ordered]@{
    server = 'http://10.0.0.55'
    username = $Username
    password_machine_dpapi = [Convert]::ToBase64String($Encrypted)
    ac_id = '8'
    check_interval_seconds = $IntervalSeconds
}
foreach ($Dir in @($CodeDir, $DataDir)) { Assert-PlainPath $Dir }
New-Item -ItemType Directory -Path $DataDir -Force | Out-Null
Protect-AppPath $DataDir $true
$ConfigPath = Join-Path $DataDir 'config.json'
Assert-PlainPath $ConfigPath
$HadConfig = Test-Path -LiteralPath $ConfigPath
$PreviousConfig = if ($HadConfig) { [IO.File]::ReadAllBytes($ConfigPath) } else { $null }
$ExistingMachineTask = Get-ScheduledTask -TaskName $TaskName -ErrorAction SilentlyContinue
$WasRunning = $ExistingMachineTask -and $ExistingMachineTask.State -in @('Running', 'Queued')
$OldUserTask = Get-ScheduledTask -TaskName 'SRun Auto Login' -ErrorAction SilentlyContinue
$UserWasRunning = $OldUserTask -and $OldUserTask.State -in @('Running', 'Queued')
$Completed = $false
$ConfigCommitted = $false
$TaskChanged = $false
$UserDisabled = $false
$TempConfigPath = Join-Path $DataDir ('config-' + [Guid]::NewGuid().ToString('N') + '.tmp')
try {
    if ($ExistingMachineTask -and $ExistingMachineTask.State -ne 'Disabled') { Stop-TaskForVerification $TaskName }
    if ($OldUserTask -and $OldUserTask.State -ne 'Disabled') { Stop-TaskForVerification 'SRun Auto Login' }
    $VerificationResult = Invoke-SetupVerification $Configuration 'password_machine_dpapi' ([Security.Cryptography.DataProtectionScope]::LocalMachine) (Join-Path $PSScriptRoot 'monitor_link2PC.ps1') $DataDir
    if ($VerificationResult -eq 'Cancelled') { throw '安装已取消。 / Installation cancelled.' }
    if ($VerificationResult -notin @('Verified', 'Skipped', 'Unverified')) {
        throw '新凭据验证失败，未保存新配置。 / New credential verification failed; new configuration was not saved.'
    }
    New-Item -ItemType Directory -Path $CodeDir -Force | Out-Null
    Protect-AppPath $CodeDir $true
    foreach ($File in @('monitor_link2PC.ps1', 'crypto_link2PC.cs')) {
        $Destination = Join-Path $CodeDir $File
        Assert-PlainPath $Destination
        if (Test-Path -LiteralPath $Destination) { Protect-AppPath $Destination $false }
        Copy-Item -LiteralPath (Join-Path $PSScriptRoot $File) -Destination $Destination -Force
        Protect-AppPath $Destination $false
    }
    $Configuration | ConvertTo-Json | Set-Content -LiteralPath $TempConfigPath -Encoding UTF8
    Protect-AppPath $TempConfigPath $false
    $ConfigCommitted = $true
    Move-Item -LiteralPath $TempConfigPath -Destination $ConfigPath -Force
    Protect-AppPath $ConfigPath $false
    Write-Host '配置已保存。 / Configuration saved.'
    foreach ($LogName in @('monitor.log', 'monitor.log.1', 'monitor.log.2')) {
        $ExistingLog = Join-Path $DataDir $LogName
        if (Test-Path -LiteralPath $ExistingLog) { Protect-AppPath $ExistingLog $false }
    }
    $Action = New-ScheduledTaskAction -Execute $PowerShellExe `
        -Argument ('-NoLogo -NoProfile -NonInteractive -WindowStyle Hidden -ExecutionPolicy Bypass -File "' + (Join-Path $CodeDir 'monitor_link2PC.ps1') + '" -DataDir "' + $DataDir + '"') `
        -WorkingDirectory $CodeDir
    $Trigger = New-ScheduledTaskTrigger -AtStartup
    $Trigger.Delay = 'PT60S'
    $Settings = New-ScheduledTaskSettingsSet -AllowStartIfOnBatteries -DontStopIfGoingOnBatteries `
        -StartWhenAvailable -MultipleInstances IgnoreNew -RestartCount 999 `
        -RestartInterval (New-TimeSpan -Minutes 1) -ExecutionTimeLimit ([TimeSpan]::Zero)
    $TaskPrincipal = New-ScheduledTaskPrincipal -UserId 'S-1-5-18' -LogonType ServiceAccount -RunLevel Highest
    $Definition = New-ScheduledTask -Action $Action -Trigger $Trigger -Settings $Settings -Principal $TaskPrincipal `
        -Description '关联电脑版 SRun 认证：开机由 SYSTEM 运行，按配置间隔检查，不依赖个人用户登录。 / Machine-wide SRun authentication: SYSTEM at boot; checks at the configured interval; independent of Windows sign-in.'
    $TaskChanged = $true
    Register-ScheduledTask -TaskName $TaskName -InputObject $Definition -Force | Out-Null
    Enable-ScheduledTask -TaskName $TaskName | Out-Null
    Write-Host '正在启动任务…… / Starting task...'
    Start-ScheduledTask -TaskName $TaskName
    Start-Sleep -Seconds 8
    if ((Get-ScheduledTask -TaskName $TaskName).State -ne 'Running') { throw '任务未运行。 / The task is not running.' }
    if ($OldUserTask -and $OldUserTask.State -ne 'Disabled') {
        $UserDisabled = $true
        Disable-ScheduledTask -TaskName 'SRun Auto Login' | Out-Null
    }
    $Completed = $true
} finally {
    if (-not $Completed) {
        if ($TaskChanged) {
            try {
                Stop-ScheduledTask -TaskName $TaskName -ErrorAction SilentlyContinue
                if ($ExistingMachineTask) { Register-ScheduledTask -TaskName $TaskName -InputObject $ExistingMachineTask -Force | Out-Null }
                elseif (Get-ScheduledTask -TaskName $TaskName -ErrorAction SilentlyContinue) { Unregister-ScheduledTask -TaskName $TaskName -Confirm:$false }
            } catch { Write-Warning '旧任务恢复失败。 / Previous task restoration failed.' }
        }
        if ($ConfigCommitted) {
            try {
                if ($HadConfig) {
                    [IO.File]::WriteAllBytes($TempConfigPath, $PreviousConfig)
                    Protect-AppPath $TempConfigPath $false
                    Move-Item -LiteralPath $TempConfigPath -Destination $ConfigPath -Force
                    Protect-AppPath $ConfigPath $false
                } elseif (Test-Path -LiteralPath $ConfigPath) { Remove-Item -LiteralPath $ConfigPath -Force }
            } catch { Write-Warning '旧配置恢复失败。 / Previous configuration restoration failed.' }
        }
        if ($UserDisabled) {
            try { Enable-ScheduledTask -TaskName 'SRun Auto Login' | Out-Null }
            catch { Write-Warning '旧任务恢复失败。 / Previous task restoration failed.' }
        }
        if ($WasRunning) { Resume-PreviousTask $TaskName }
        if ($UserWasRunning) { Resume-PreviousTask 'SRun Auto Login' }
    }
    if (Test-Path -LiteralPath $TempConfigPath) {
        try { Remove-Item -LiteralPath $TempConfigPath -Force }
        catch { Write-Warning '临时文件清理失败。 / Temporary file cleanup failed.' }
    }
}
Write-Host '安装完成。 / Installed.' -ForegroundColor Green
Write-Host "计划任务 / Task: $TaskName"
Write-Host "程序目录 / Program directory: $CodeDir"
Write-Host "日志文件 / Log file: $(Join-Path $DataDir 'monitor.log')"
