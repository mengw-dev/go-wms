[CmdletBinding()]
param(
    [switch]$Force,
    [switch]$NoBrowser
)

$ErrorActionPreference = "Stop"
. (Join-Path $PSScriptRoot "..\wms-common.ps1")
Set-Location $WmsRoot

if (-not $Force) {
    Write-Host "WARNING: This permanently deletes the MySQL, Redis and upload volumes." -ForegroundColor Red
    $answer = Read-Host "Type RESET to continue"
    if ($answer -ne "RESET") {
        Write-Host "Reset cancelled." -ForegroundColor Yellow
        exit 0
    }
}

try {
    Write-Step "Checking Docker"
    Assert-Docker

    Write-Step "Removing containers and all WMS volumes"
    Invoke-WmsCompose @("down", "-v", "--remove-orphans")
    Write-Ok "Old data was removed"

    Write-Step "Starting a clean WMS instance"
    $startArgs = @{}
    if ($NoBrowser) {
        $startArgs["NoBrowser"] = $true
    }
    & (Join-Path $PSScriptRoot "start.ps1") @startArgs
    if ($LASTEXITCODE -ne 0) {
        throw "start.ps1 failed with exit code $LASTEXITCODE"
    }
}
catch {
    Write-Host "Reset failed: $($_.Exception.Message)" -ForegroundColor Red
    exit 1
}
