[CmdletBinding()]
param()
& (Join-Path $PSScriptRoot "stop-demo-instance.ps1") -Instance b