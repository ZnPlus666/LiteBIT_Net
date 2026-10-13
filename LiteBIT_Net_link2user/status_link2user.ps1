$ErrorActionPreference = 'Stop'
function Get-CheckIntervalText([string]$ConfigPath) {
    try {
        if (-not (Test-Path -LiteralPath $ConfigPath)) { return '未找到配置。 / Configuration not found.' }
        $Config = Get-Content -LiteralPath $ConfigPath -Raw -Encoding UTF8 | ConvertFrom-Json
        $Value = [string]$Config.check_interval_seconds
        $Seconds = 0
        if ($Value -notmatch '^[0-9]+$' -or -not [int]::TryParse($Value, [ref]$Seconds) -or $Seconds -lt 15) {
            return '无效。 / Invalid.'
        }
        $Minutes = ($Seconds / 60.0).ToString('0.##', [Globalization.CultureInfo]::InvariantCulture)
        return ($Minutes + ' 分钟 / minutes（' + $Seconds + ' 秒 / seconds）')
    } catch {
        # Configuration may contain credentials; never echo parse errors or raw content.
        return '无法读取。 / Unavailable.'
    }
}

try {
    $Task = Get-ScheduledTask -TaskName 'SRun Auto Login'
    $Info = Get-ScheduledTaskInfo -TaskName $Task.TaskName
    Write-Host "计划任务 / Task: $($Task.TaskName)"
    Write-Host "运行状态 / State: $($Task.State)"
    Write-Host "执行账户 / Run account: $($Task.Principal.UserId)"
    $IntervalText = Get-CheckIntervalText (Join-Path $env:LOCALAPPDATA 'SrunAutoLogin\config.json')
    Write-Host ("检查间隔 / Check interval: " + $IntervalText)
    Write-Host "最近启动时间 / Last start: $($Info.LastRunTime)"
    Write-Host "最近返回码 / Last result: $($Info.LastTaskResult)（267009 表示正在运行 / 267009 means Running）"
    Write-Host ''
    Write-Host '最近的监控日志 / Recent monitoring log entries:'
    Get-Content -LiteralPath (Join-Path $env:LOCALAPPDATA 'SrunAutoLogin\srun-auto-login.log') -Tail 30 -Encoding UTF8
} catch {
    Write-Host ('错误 / Error: ' + $_.Exception.Message) -ForegroundColor Red
    exit 1
}
