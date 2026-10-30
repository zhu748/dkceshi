@echo off
setlocal
cd /d "%~dp0"
if errorlevel 1 exit /b 1
set "DS2API_NODE=node"
if defined DS2API_DEVID_NODE set "DS2API_NODE=%DS2API_DEVID_NODE%"
"%DS2API_NODE%" --version
if errorlevel 1 (
    echo Node.js was not found. Install Node.js 22+ or set DS2API_DEVID_NODE.
    pause
    exit /b 1
)
echo Checking SDK assets and runtime without submitting a device profile...
"%DS2API_NODE%" "%~dp0scripts\devid-generate.cjs" --selftest
set "DS2API_EXIT_CODE=%ERRORLEVEL%"
echo.
echo SDK self-test exited with code %DS2API_EXIT_CODE%.
pause
exit /b %DS2API_EXIT_CODE%
