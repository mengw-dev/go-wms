@echo off
powershell -NoProfile -ExecutionPolicy Bypass -File "%~dp0start-monitoring.ps1" %*
if errorlevel 1 pause
