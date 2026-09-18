[CmdletBinding()]
param()

$ErrorActionPreference = "Stop"
. (Join-Path $PSScriptRoot "..\wms-common.ps1")
Set-Location $WmsRoot

try {
    Write-Step "Checking Docker"
    Assert-Docker
    Write-Step "Stopping WMS"
    Invoke-WmsCompose @("down", "--remove-orphans")
    Write-Ok "WMS stopped. Database and upload volumes were preserved."
}
catch {
    Write-Host "Stop failed: $($_.Exception.Message)" -ForegroundColor Red
    exit 1
}
