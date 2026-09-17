@echo off
powershell -NoProfile -ExecutionPolicy Bypass -File "%~dp0stop-demo-b.ps1" %*
pause