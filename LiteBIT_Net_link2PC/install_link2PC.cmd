@echo off
chcp 65001 >nul
title LiteBIT_Net 关联电脑版安装 / LiteBIT_Net link2PC Installer
powershell.exe -NoLogo -NoProfile -ExecutionPolicy Bypass -File "%~dp0install_link2PC.ps1"
set "INSTALL_EXIT=%ERRORLEVEL%"
if not "%INSTALL_EXIT%"=="0" (
  echo 安装失败，退出码为 %INSTALL_EXIT%。 / Installation failed with exit code %INSTALL_EXIT%.
)
echo 按任意键退出…… / Press any key to exit...
pause >nul
exit /b %INSTALL_EXIT%
