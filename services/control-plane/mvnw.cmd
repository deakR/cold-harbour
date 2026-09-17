@echo off
setlocal

@REM Delegate to an installed Maven, matching the Unix launcher.
where mvn.cmd >nul 2>&1
if errorlevel 1 (
    echo [ERROR] Maven not found. Install Maven and add its bin directory to PATH. >&2
    exit /b 1
)

call mvn.cmd %*
exit /b %ERRORLEVEL%
