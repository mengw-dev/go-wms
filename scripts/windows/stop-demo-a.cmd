@echo off
powershell -NoProfile -ExecutionPolicy Bypass -File "%~dp0stop-demo-a.ps1" %*
pause