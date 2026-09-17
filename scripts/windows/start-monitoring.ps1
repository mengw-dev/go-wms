[CmdletBinding()]
param()

$ErrorActionPreference = "Stop"
. (Join-Path $PSScriptRoot "..\wms-common.ps1")
Set-Location $WmsRoot

function Wait-HttpReady {
    param([string]$Url, [int]$TimeoutSeconds = 120)

    $deadline = (Get-Date).AddSeconds($TimeoutSeconds)
    while ((Get-Date) -lt $deadline) {
        try {
            $response = Invoke-WebRequest -Uri $Url -UseBasicParsing -TimeoutSec 3
            if ($response.StatusCode -ge 200 -and $response.StatusCode -lt 400) {
                return $true
            }
        }
        catch {
            Start-Sleep -Seconds 2
        }
    }
    return $false
}

Assert-Docker

$envPath = Join-Path $WmsRoot ".env"
$values = Read-DotEnv -Path $envPath
$changed = $false
if (-not $values.Contains("WMS_GRAFANA_PORT") -or [string]::IsNullOrWhiteSpace($values["WMS_GRAFANA_PORT"])) {
    $values["WMS_GRAFANA_PORT"] = "3000"
    $changed = $true
}
if (-not $values.Contains("WMS_GRAFANA_ADMIN_USER") -or [string]::IsNullOrWhiteSpace($values["WMS_GRAFANA_ADMIN_USER"])) {
    $values["WMS_GRAFANA_ADMIN_USER"] = "admin"
    $changed = $true
}
if (-not $values.Contains("WMS_GRAFANA_ADMIN_PASSWORD")) {
    $values["WMS_GRAFANA_ADMIN_PASSWORD"] = ""
    $changed = $true
}
$grafanaPassword = [string]$values["WMS_GRAFANA_ADMIN_PASSWORD"]
if (Test-PlaceholderSecret $grafanaPassword -or $grafanaPassword.Length -lt 12) {
    $values["WMS_GRAFANA_ADMIN_PASSWORD"] = New-RandomHex -ByteCount 16
    $changed = $true
}
if ($changed) {
    Save-DotEnv -Path $envPath -Values $values
}

Write-Step "Starting Prometheus and Grafana"
Invoke-WmsCompose @("--profile", "monitoring", "up", "-d", "prometheus", "grafana")

$grafanaPort = [int]$values["WMS_GRAFANA_PORT"]
if (-not (Wait-HttpReady -Url "http://127.0.0.1:$grafanaPort/api/health")) {
    Invoke-WmsCompose @("--profile", "monitoring", "ps")
    throw "Grafana did not become healthy in time."
}

Write-Ok "Monitoring is ready"
Write-Host "Prometheus: http://127.0.0.1:$($values['WMS_PROMETHEUS_PORT'])" -ForegroundColor Green
Write-Host "Grafana:    http://127.0.0.1:$grafanaPort" -ForegroundColor Green
Write-Host "User:       $($values['WMS_GRAFANA_ADMIN_USER'])" -ForegroundColor Green
Write-Host "Password:   stored in .env as WMS_GRAFANA_ADMIN_PASSWORD" -ForegroundColor Green
