@echo off
powershell -NoProfile -ExecutionPolicy Bypass -File "%~dp0reset.ps1" %*
pause
