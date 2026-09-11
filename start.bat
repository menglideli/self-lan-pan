@echo off
rem CloudPan launcher. The first run creates the database; the admin account is always "admin",
rem and its initial password is randomly generated and printed once in the startup log below.
cd /d "%~dp0server"
if not exist cloudpan.exe (
  echo cloudpan.exe not found - please run build.bat first
  pause
  exit /b 1
)
echo CloudPan starting: http://localhost:18322  (Ctrl+C to stop)
echo Admin account: admin   -   initial password is printed in the log below
cloudpan.exe
