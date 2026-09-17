[CmdletBinding()]
param()

$ErrorActionPreference = "Stop"
. (Join-Path $PSScriptRoot "..\wms-common.ps1")
Set-Location $WmsRoot

Assert-Docker
Write-Step "Stopping Prometheus and Grafana"
Invoke-WmsCompose -ComposeArgs @("--profile", "monitoring", "stop", "prometheus", "grafana")
Write-Ok "Monitoring stopped. WMS, MySQL and Redis were not stopped."
