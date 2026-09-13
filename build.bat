@echo off
rem CloudPan one-click build: frontend -> embed -> single binary -> out\ bundle
rem NOTE: keep every comment in this file ASCII-only. cmd.exe may decode this
rem file as GBK; a UTF-8 Chinese comment byte can eat the following line.
setlocal
set "PATH=%PATH%;C:\Program Files\nodejs;C:\Program Files\Go\bin"
cd /d "%~dp0"

echo [1/4] building frontend...
cd web
call npm run build || goto :err
cd ..

echo [2/4] embedding dist...
if exist server\internal\web\dist rmdir /s /q server\internal\web\dist
xcopy /e /i /q web\dist server\internal\web\dist >nul || goto :err
rem placeholder: .gitignore keeps this file so a fresh clone can still run go build
type nul > server\internal\web\dist\.keep

echo [3/4] building cloudpan.exe...
cd server
go build -o cloudpan.exe . || goto :err
cd ..

echo [4/4] assembling out\ ...
rem NOTE: out\ is deliberately NOT wiped. If you already ran the exe from out\,
rem your data lives in out\data\ and a rebuild must not destroy it.
if not exist out mkdir out
if exist out\data echo       (out\data found - keeping it untouched)
copy /y server\cloudpan.exe out\ >nul || goto :err
copy /y start.bat           out\ >nul || goto :err
copy /y start.sh            out\ >nul || goto :err
copy /y packaging\README.txt out\ >nul || goto :err
copy /y LICENSE             out\ >nul || goto :err

rem Force the text files in out\ to CRLF. The delivery folder is for Windows
rem users (cmd + Notepad), but the working copy here may well be LF: an editor
rem on Linux/macOS, or any tool that rewrites the file, will drop the CRLF that
rem ".gitattributes eol=crlf" only restores at checkout time. LF-only .bat files
rem happen to work on this machine, but that is luck, not a guarantee.
powershell -NoProfile -Command "$cr=[char]13;$lf=[char]10;Get-ChildItem 'out\*' -Include *.bat,*.txt -File | ForEach-Object { $t=[IO.File]::ReadAllText($_.FullName); $t=$t.Replace($cr.ToString()+$lf.ToString(),$lf.ToString()).Replace($lf.ToString(),$cr.ToString()+$lf.ToString()); [IO.File]::WriteAllText($_.FullName,$t,(New-Object System.Text.UTF8Encoding($false))) }" || goto :err

echo.
echo BUILD OK
echo   binary : server\cloudpan.exe
echo   bundle : out\   (copy this whole folder anywhere, then run start.bat)
goto :eof

:err
echo BUILD FAILED
exit /b 1
