param(
    [string]$DataDir = (Join-Path $env:ProgramData 'SrunMachineLogin'),
    [switch]$Once,
    [switch]$LibraryOnly
)
$ErrorActionPreference = 'Stop'
$LogPath = Join-Path $DataDir 'monitor.log'

function Write-MonitorLog([string]$Message) {
    if (Test-Path -LiteralPath $LogPath) {
        if ((Get-Item -LiteralPath $LogPath).Length -gt 1MB) {
            $Oldest = $LogPath + '.2'
            if (Test-Path -LiteralPath $Oldest) { Remove-Item -LiteralPath $Oldest -Force }
            $Previous = $LogPath + '.1'
            if (Test-Path -LiteralPath $Previous) { Move-Item -LiteralPath $Previous -Destination $Oldest -Force }
            Move-Item -LiteralPath $LogPath -Destination $Previous -Force
        }
    }
    $Line = (Get-Date -Format 'yyyy-MM-dd HH:mm:ss') + ' ' + $Message
    Add-Content -LiteralPath $LogPath -Value $Line -Encoding UTF8
    if ($Once) { Write-Host $Line }
}

function ConvertFrom-PortalJson([string]$Body) {
    $Body = $Body.Trim()
    if (-not $Body.StartsWith('{')) {
        if ($Body -notmatch '(?s)^[^(]*\((.*)\)\s*;?$') { throw '服务器返回的不是有效 JSON/JSONP。 / The server response is not valid JSON/JSONP.' }
        $Body = $Matches[1]
    }
    return ($Body | ConvertFrom-Json)
}

function Invoke-PortalRequest(
    [string]$Path,
    [System.Collections.IDictionary]$Parameters,
    [ValidateRange(1, 8000)][int]$TimeoutMilliseconds = 8000
) {
    $Pairs = New-Object 'System.Collections.Generic.List[string]'
    foreach ($Key in $Parameters.Keys) {
        $Pairs.Add([Uri]::EscapeDataString([string]$Key) + '=' + [Uri]::EscapeDataString([string]$Parameters[$Key]))
    }
    $Stamp = [DateTimeOffset]::UtcNow.ToUnixTimeMilliseconds()
    $Pairs.Add('callback=jQuery_srun_' + $Stamp)
    $Pairs.Add('_=' + $Stamp)
    $Url = $script:Config.server.TrimEnd('/') + $Path + '?' + ($Pairs -join '&')
    $Request = [Net.HttpWebRequest]::Create($Url)
    $Request.Proxy = $null
    $Request.AllowAutoRedirect = $false
    $Request.Timeout = $TimeoutMilliseconds
    $Request.ReadWriteTimeout = $TimeoutMilliseconds
    $Request.UserAgent = 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) SRunMachineLogin/1.0'
    $Request.Referer = $script:Config.server + '/srun_portal_pc?ac_id=' + $script:Config.ac_id + '&theme=bit'
    $Request.Headers.Add('X-Requested-With', 'XMLHttpRequest')
    $RequestTimer = [Diagnostics.Stopwatch]::StartNew()
    $Response = $Request.GetResponse()
    try {
        $ResponseStream = $Response.GetResponseStream()
        $Reader = New-Object IO.StreamReader($ResponseStream, [Text.Encoding]::UTF8)
        try {
            $Buffer = New-Object char[] 4096
            $Builder = New-Object Text.StringBuilder
            while ($true) {
                $Remaining = $TimeoutMilliseconds - [int]$RequestTimer.ElapsedMilliseconds
                if ($Remaining -le 0) { throw [TimeoutException]::new('Portal request timed out.') }
                if ($ResponseStream.CanTimeout) { $ResponseStream.ReadTimeout = $Remaining }
                $Count = $Reader.Read($Buffer, 0, $Buffer.Length)
                if ($Count -eq 0) { break }
                $null = $Builder.Append($Buffer, 0, $Count)
                if ($Builder.Length -gt 65536) { throw 'Portal response too large.' }
            }
            $Body = $Builder.ToString()
        } finally { $Reader.Dispose() }
    } finally { $Response.Dispose() }
    return (ConvertFrom-PortalJson $Body)
}

function Get-PortalRouteIP {
    $Socket = New-Object Net.Sockets.Socket([Net.Sockets.AddressFamily]::InterNetwork, [Net.Sockets.SocketType]::Dgram, [Net.Sockets.ProtocolType]::Udp)
    try {
        $Socket.Connect(([Uri]$script:Config.server).Host, 80)
        return $Socket.LocalEndPoint.Address.ToString()
    } finally { $Socket.Dispose() }
}

function Get-VerificationTimeMilliseconds {
    return ([Diagnostics.Stopwatch]::GetTimestamp() * 1000.0 / [Diagnostics.Stopwatch]::Frequency)
}

function Get-SafeVerificationStatusCode([string]$Code) {
    # Never echo arbitrary response text: even an error field may contain credentials.
    if ($Code -in @('ok', 'not_online', 'not_online_error', 'password_error', 'user_not_exist', 'ip_already_online_error', 'logout_error')) { return $Code }
    if ($Code -cmatch '^E[0-9]{3,6}$') { return $Code }
    if (-not $Code) { return 'missing_status' }
    return 'unknown_status'
}

function Wait-NewCredentialConfirmation {
    $Started = Get-VerificationTimeMilliseconds
    $Codes = New-Object 'System.Collections.Generic.List[string]'
    # Query immediately, then at one-second intervals, within a five-second budget.
    for ($Attempt = 0; $Attempt -lt 5; $Attempt++) {
        $Elapsed = (Get-VerificationTimeMilliseconds) - $Started
        $Delay = [int][Math]::Ceiling(($Attempt * 1000) - $Elapsed)
        if ($Delay -gt 0) { Start-Sleep -Milliseconds $Delay }
        $Remaining = [int][Math]::Floor(5000 - ((Get-VerificationTimeMilliseconds) - $Started))
        if ($Remaining -le 0) { break }
        try {
            $Verified = Invoke-PortalRequest '/cgi-bin/rad_user_info' @{} -TimeoutMilliseconds $Remaining
            if (((Get-VerificationTimeMilliseconds) - $Started) -ge 5000) {
                $Code = 'request_timeout'
            } elseif ($Verified.error -eq 'ok') {
                if ([string]$Verified.user_name -ceq [string]$script:Config.username) {
                    Write-MonitorLog ('LOGIN_OK 自动登录已核实成功 / Automatic login verified; IP=' + $Verified.online_ip)
                    return 'Verified'
                }
                $Code = if ($Verified.user_name) { 'account_mismatch' } else { 'account_missing' }
            } else {
                $Code = Get-SafeVerificationStatusCode ([string]$Verified.error)
            }
        } catch {
            if ($_.Exception -is [TimeoutException] -or
                ($_.Exception -is [Net.WebException] -and $_.Exception.Status -eq [Net.WebExceptionStatus]::Timeout)) {
                $Code = 'request_timeout'
            } elseif ($_.Exception -is [Net.WebException] -and $_.Exception.Response -is [Net.HttpWebResponse]) {
                $Code = 'http_' + [int]$_.Exception.Response.StatusCode
            } else { $Code = 'request_failed' }
        }
        if (-not $Codes.Contains($Code)) { $Codes.Add($Code) }
    }
    $Remaining = [int][Math]::Floor(5000 - ((Get-VerificationTimeMilliseconds) - $Started))
    if ($Remaining -gt 0) { Start-Sleep -Milliseconds $Remaining }
    if ($Codes.Count -eq 0) { $Codes.Add('request_timeout') }
    Write-MonitorLog 'VERIFY_UNKNOWN 验证结果未知。 / Verification result unknown.'
    Write-MonitorLog ('VERIFY_STATUS 状态码 / Status code: ' + ($Codes -join ', '))
    return 'Unknown'
}

function Invoke-CredentialLogin([switch]$RequireUsername) {
    $script:CredentialConfirmationUnknown = $false
    $IP = Get-PortalRouteIP
    $Challenge = Invoke-PortalRequest '/cgi-bin/get_challenge' @{username=$script:Config.username; ip=$IP}
    if ($Challenge.error -ne 'ok' -or -not $Challenge.challenge) {
        Write-MonitorLog 'CHALLENGE_FAILED 获取认证挑战值失败，下次检查时重试。 / Failed to obtain the authentication challenge; retry on the next check.'
        return $false
    }
    if ($Challenge.client_ip) { $IP = [string]$Challenge.client_ip }
    $Parameters = [SrunMachineCrypto]::LoginParameters(
        [string]$script:Config.username, $script:Password, $IP,
        [string]$Challenge.challenge, [string]$script:Config.ac_id
    )
    $Result = Invoke-PortalRequest '/cgi-bin/srun_portal' $Parameters
    # The portal may return error=ok with an already-online success message.
    # That is never proof that the submitted password was checked.
    if ($Result.error -eq 'ok' -and [string]$Result.suc_msg -notmatch 'already_online') {
        Write-MonitorLog 'LOGIN_ACCEPTED 登录请求已接受，正在核实认证状态。 / Login request accepted; verifying authentication status.'
        if ($RequireUsername) {
            if ((Wait-NewCredentialConfirmation) -eq 'Verified') { return $true }
            $script:CredentialConfirmationUnknown = $true
            return $false
        }
        $Verified = Invoke-PortalRequest '/cgi-bin/rad_user_info' @{}
        if ($Verified.error -eq 'ok') {
            Write-MonitorLog ('LOGIN_OK 自动登录已核实成功 / Automatic login verified; IP=' + $Verified.online_ip)
            return $true
        }
        Write-MonitorLog 'LOGIN_NOT_CONFIRMED 尚未确认认证成功，下次检查时重试。 / Authentication is not confirmed; retry on the next check.'
        return $false
    }
    $Code = if ([string]$Result.suc_msg -match 'already_online') { [string]$Result.suc_msg } else { [string]$Result.error }
    $Code = Get-SafeVerificationStatusCode $Code
    Write-MonitorLog ('LOGIN_FAILED 自动登录失败 / Automatic login failed; code=' + $Code + '；下次检查时重试。 / Retry on the next check.')
    return $false
}

function Test-AndRestoreLogin {
    try {
        $Status = Invoke-PortalRequest '/cgi-bin/rad_user_info' @{}
        if ($Status.error -eq 'ok') {
            Write-MonitorLog ('ONLINE 已认证 / Authenticated; IP=' + $Status.online_ip)
            return $true
        }
        Write-MonitorLog 'OFFLINE 未认证，正在登录 / Unauthenticated; logging in.'
        return (Invoke-CredentialLogin)
    } catch {
        # Never log exceptions containing authentication URLs or response bodies.
        Write-MonitorLog ('CHECK_FAILED 检查或登录失败 / Check or login failed; type=' + $_.Exception.GetType().Name + '；下次检查时重试。 / Retry on the next check.')
        return $false
    }
}

function Test-NewCredentials([switch]$AllowDisconnect) {
    $script:CredentialConfirmationUnknown = $false
    try {
        $Status = Invoke-PortalRequest '/cgi-bin/rad_user_info' @{}
        if ($Status.error -eq 'ok') {
            if (-not $AllowDisconnect) {
                Write-MonitorLog 'VERIFY_ONLINE 当前已在线，新凭据尚未验证。 / Already authenticated; new credentials have not been verified.'
                return 'DisconnectRequired'
            }
            $LogoutParameters = @{action='logout'; ac_id=$script:Config.ac_id; ip=$Status.online_ip}
            if (-not $LogoutParameters.ip) { $LogoutParameters.ip = Get-PortalRouteIP }
            # Sign out the CURRENT account, never the candidate account.
            if ($Status.user_name) { $LogoutParameters.username = [string]$Status.user_name }
            Write-MonitorLog 'VERIFY_LOGOUT 正在注销当前校园网…… / Signing out of the current campus-network session...'
            $LogoutResult = Invoke-PortalRequest '/cgi-bin/srun_portal' $LogoutParameters
            if ($LogoutResult.error -notin @('ok', 'logout_ok')) {
                Write-MonitorLog 'VERIFY_LOGOUT_FAILED 注销失败，未验证新凭据。 / Sign-out failed; new credentials were not verified.'
                return 'Failed'
            }
            # MAC authentication or another client may immediately log in again.
            # Do not accept an existing session as validation.
            for ($Attempt = 0; $Attempt -lt 5; $Attempt++) {
                $Status = Invoke-PortalRequest '/cgi-bin/rad_user_info' @{}
                if ($Status.error -in @('not_online', 'not_online_error')) { break }
                if ($Status.error -ne 'ok') {
                    Write-MonitorLog 'VERIFY_STATE_UNKNOWN 无法确认当前认证状态，未验证新凭据。 / Cannot confirm authentication status; new credentials were not verified.'
                    return 'Failed'
                }
                if ($Attempt -lt 4) { Start-Sleep -Milliseconds 500 }
            }
        }
        if ($Status.error -notin @('not_online', 'not_online_error')) {
            Write-MonitorLog 'VERIFY_NOT_OFFLINE 未确认下线，未验证新凭据。 / Sign-out was not confirmed; new credentials were not verified.'
            return 'Failed'
        }
        Write-MonitorLog 'VERIFY_LOGIN 正在使用新凭据登录…… / Logging in with the new credentials...'
        if (Invoke-CredentialLogin -RequireUsername) {
            Write-MonitorLog 'CREDENTIALS_VALID 新凭据验证成功。 / New credentials verified.'
            return 'Verified'
        }
        if ($script:CredentialConfirmationUnknown) { return 'Unknown' }
        return 'Failed'
    } catch {
        Write-MonitorLog ('VERIFY_FAILED 新凭据验证未成功 / New credential verification did not succeed; type=' + $_.Exception.GetType().Name)
        return 'Failed'
    }
}

try {
    Add-Type -AssemblyName System.Security
    if (-not ('SrunMachineCrypto' -as [type])) { Add-Type -Path (Join-Path $PSScriptRoot 'crypto_link2PC.cs') }
    if ($LibraryOnly) { return }
    $script:Config = Get-Content -LiteralPath (Join-Path $DataDir 'config.json') -Raw -Encoding UTF8 | ConvertFrom-Json
    $Encrypted = [Convert]::FromBase64String($script:Config.password_machine_dpapi)
    $RawPassword = [Security.Cryptography.ProtectedData]::Unprotect($Encrypted, $null, [Security.Cryptography.DataProtectionScope]::LocalMachine)
    try { $script:Password = [Text.Encoding]::UTF8.GetString($RawPassword) } finally { [Array]::Clear($RawPassword, 0, $RawPassword.Length) }
    $Interval = [int]$script:Config.check_interval_seconds
    if ($Interval -lt 15) { throw '检查间隔无效。 / Invalid check interval.' }
    Write-MonitorLog ('MONITOR_STARTED 监控已启动 / Monitor started; identity=' + [Security.Principal.WindowsIdentity]::GetCurrent().Name + '；检查间隔 / interval=' + $Interval + 's')
    if ($Once) {
        if (Test-AndRestoreLogin) { exit 0 } else { exit 1 }
    }
    while ($true) {
        $null = Test-AndRestoreLogin
        Start-Sleep -Seconds $Interval
    }
} catch {
    try { Write-MonitorLog ('START_FAILED 监控启动失败 / Monitor startup failed; type=' + $_.Exception.GetType().Name) } catch {}
    exit 1
}
