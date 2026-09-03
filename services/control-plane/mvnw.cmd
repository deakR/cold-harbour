@REM ----------------------------------------------------------------------------
@REM Maven Start Up Batch script for ColdHarbor Control Plane
@REM ----------------------------------------------------------------------------
@echo off
setlocal

set MAVEN_CMD="C:\Users\ROHITH\.m2\wrapper\dists\apache-maven-3.9.15\0226a00282e400185496f3b60ec5a3f029cbdc6893912937d4876d57695224e1\bin\mvn.cmd"
if exist %MAVEN_CMD% (
    %MAVEN_CMD% %*
    exit /b %ERRORLEVEL%
)

where mvn >nul 2>&1
if %ERRORLEVEL% equ 0 (
    mvn %*
    exit /b %ERRORLEVEL%
)

echo [ERROR] Maven not found.
exit /b 1
