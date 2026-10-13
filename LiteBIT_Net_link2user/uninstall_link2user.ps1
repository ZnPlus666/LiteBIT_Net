$ErrorActionPreference = 'Stop'
trap {
    Write-Host '卸载失败。 / Uninstall failed.' -ForegroundColor Red
    Write-Host ('错误 / Error: ' + $_.Exception.Message) -ForegroundColor Red
    exit 1
}
$TaskName = 'SRun Auto Login'
$Task = Get-ScheduledTask -TaskName $TaskName -ErrorAction SilentlyContinue
if ($Task) {
    Stop-ScheduledTask -TaskName $TaskName -ErrorAction SilentlyContinue
    Unregister-ScheduledTask -TaskName $TaskName -Confirm:$false
    Write-Host '卸载完成。 / Uninstalled.'
} else {
    Write-Host '未找到计划任务。 / Scheduled task not found.'
}

$AppDir = Join-Path $env:LOCALAPPDATA 'SrunAutoLogin'
Write-Host "保留的文件 / Retained files: $AppDir"
