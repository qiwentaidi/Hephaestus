@echo off
setlocal
rem Usage: install.bat -Server https://your-server [-Action update] [-NoStart]
set "HEPHAESTUS_AGENT_BAT=%~f0"
set "HEPHAESTUS_AGENT_PS=%TEMP%\hephaestus-install-%RANDOM%-%RANDOM%.ps1"
powershell.exe -NoLogo -NoProfile -Command "$ErrorActionPreference='Stop'; $payload=Get-Content -LiteralPath $env:HEPHAESTUS_AGENT_BAT -Tail 1; [IO.File]::WriteAllBytes($env:HEPHAESTUS_AGENT_PS,[Convert]::FromBase64String($payload))"
if errorlevel 1 exit /b 1
powershell.exe -NoLogo -NoProfile -ExecutionPolicy Bypass -File "%HEPHAESTUS_AGENT_PS%" %*
set "RESULT=%ERRORLEVEL%"
del "%HEPHAESTUS_AGENT_PS%" >nul 2>&1
if "%~1"=="" pause
exit /b %RESULT%
__HEPHAESTUS_POWERSHELL_BASE64__
