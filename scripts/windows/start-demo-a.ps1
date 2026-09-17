[CmdletBinding()]
param([switch]$NoBrowser, [switch]$NoBuild)
& (Join-Path $PSScriptRoot "start-demo-instance.ps1") -Instance a -NoBrowser:$NoBrowser -NoBuild:$NoBuild