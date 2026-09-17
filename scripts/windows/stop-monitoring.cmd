@echo off
powershell -NoProfile -ExecutionPolicy Bypass -File "%~dp0stop-monitoring.ps1" %*
pause
