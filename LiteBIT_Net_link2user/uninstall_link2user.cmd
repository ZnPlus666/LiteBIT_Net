@echo off
chcp 65001 >nul
title LiteBIT_Net 关联用户版卸载 / LiteBIT_Net link2user Uninstaller
powershell.exe -NoLogo -NoProfile -ExecutionPolicy Bypass -File "%~dp0uninstall_link2user.ps1"
set "ACTION_EXIT=%ERRORLEVEL%"
echo 按任意键退出…… / Press any key to exit...
pause >nul
exit /b %ACTION_EXIT%
