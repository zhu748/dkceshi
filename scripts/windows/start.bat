@echo off
setlocal
cd /d "%~dp0"
if errorlevel 1 exit /b 1
echo DS2API - Windows
echo Admin page: http://127.0.0.1:5001/admin
echo Keep this window open. Press Ctrl+C to stop.
echo.
"%~dp0ds2api.exe"
set "DS2API_EXIT_CODE=%ERRORLEVEL%"
echo.
echo DS2API exited with code %DS2API_EXIT_CODE%.
pause
exit /b %DS2API_EXIT_CODE%
