@echo off
rem CloudPan launcher.
rem
rem Works in two layouts:
rem   1) release bundle - cloudpan.exe sits right next to this script (out\)
rem   2) source tree    - cloudpan.exe sits in .\server\ (after build.bat)
rem
rem The working directory decides where .\data lives. Launching from somewhere
rem else silently creates a brand new empty database plus a new secret.key,
rem which looks exactly like "all my files are gone". So always cd into the
rem folder that holds the exe before starting it.
rem
rem NOTE: keep every comment in this file ASCII-only (cmd.exe may decode this
rem file as GBK; a UTF-8 Chinese byte can eat the following line).
setlocal
cd /d "%~dp0"

set "APPDIR="
if exist cloudpan.exe set "APPDIR=%~dp0"
if not defined APPDIR (
  if exist server\cloudpan.exe set "APPDIR=%~dp0server"
)

if not defined APPDIR (
  echo.
  echo cloudpan.exe not found.
  echo Run build.bat first, or put this script next to cloudpan.exe.
  echo.
  pause
  exit /b 1
)

cd /d "%APPDIR%"
echo CloudPan starting: http://localhost:18322   -- press Ctrl+C to stop
echo Admin account: admin   -- initial password is printed in the log below
echo Data folder:   %APPDIR%data
echo.
cloudpan.exe
