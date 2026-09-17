[CmdletBinding()]
param(
    [switch]$NoBrowser,
    [switch]$NoBuild
)

$ErrorActionPreference = "Stop"
. (Join-Path $PSScriptRoot "..\wms-common.ps1")
Set-Location $WmsRoot

function Wait-HttpReady {
    param(
        [string]$Url,
        [int]$TimeoutSeconds = 180
    )

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

try {
    Write-Step "Checking Docker"
    Assert-Docker
    Write-Ok "Docker and Docker Compose are available"

    $envPath = Join-Path $WmsRoot ".env"
    $values = Read-DotEnv -Path $envPath
    $changed = $false

    if (Test-PlaceholderSecret $values["MYSQL_ROOT_PASSWORD"]) {
        $values["MYSQL_ROOT_PASSWORD"] = New-RandomHex -ByteCount 24
        $changed = $true
    }
    if (-not $values.Contains("MYSQL_DATABASE") -or [string]::IsNullOrWhiteSpace($values["MYSQL_DATABASE"])) {
        $values["MYSQL_DATABASE"] = "gowms"
        $changed = $true
    }
    if (Test-PlaceholderSecret $values["JWT_SECRET"] -or $values["JWT_SECRET"].Length -lt 32) {
        $values["JWT_SECRET"] = New-RandomHex -ByteCount 48
        $changed = $true
    }
    if (Test-PlaceholderSecret $values["WMS_INTEGRATION_API_KEY"] -or $values["WMS_INTEGRATION_API_KEY"].Length -lt 24) {
        $values["WMS_INTEGRATION_API_KEY"] = New-RandomHex -ByteCount 24
        $changed = $true
    }
    if (-not $values.Contains("WMS_SERVER_NODE") -or [string]::IsNullOrWhiteSpace($values["WMS_SERVER_NODE"])) {
        $values["WMS_SERVER_NODE"] = "1"
        $changed = $true
    }
    if (-not $values.Contains("WMS_API_PORT") -or [string]::IsNullOrWhiteSpace($values["WMS_API_PORT"])) {
        $values["WMS_API_PORT"] = "8080"
        $changed = $true
    }
    if (-not $values.Contains("WMS_WEB_PORT") -or [string]::IsNullOrWhiteSpace($values["WMS_WEB_PORT"])) {
        $values["WMS_WEB_PORT"] = "80"
        $changed = $true
    }

    if ($changed -or -not (Test-Path -LiteralPath $envPath)) {
        Save-DotEnv -Path $envPath -Values $values
        Write-Ok "Generated or repaired .env with secure random values"
    }
    else {
        Write-Ok "Using existing .env"
    }

    $apiPort = [int]$values["WMS_API_PORT"]
    $webPort = [int]$values["WMS_WEB_PORT"]

    Write-Step "Building and starting GoWMS"
    $composeArgs = @("up", "-d")
    if (-not $NoBuild) {
        $composeArgs += "--build"
    }
    Invoke-WmsCompose @composeArgs

    Write-Step "Waiting for services to become healthy"
    $apiReady = Wait-HttpReady -Url "http://127.0.0.1:$apiPort/healthz"
    $webReady = Wait-HttpReady -Url "http://127.0.0.1:$webPort/"

    if (-not $apiReady -or -not $webReady) {
        Write-Host "Service health check timed out. Current container status:" -ForegroundColor Yellow
        Invoke-WmsCompose ps
        throw "GoWMS did not become healthy in time."
    }

    Write-Ok "GoWMS is ready"
    Write-Host ""
    Write-Host "Web:  http://127.0.0.1:$webPort" -ForegroundColor Green
    Write-Host "API:  http://127.0.0.1:$apiPort" -ForegroundColor Green
    Write-Host "User: admin" -ForegroundColor Green
    Write-Host "Pass: admin123" -ForegroundColor Green
    Write-Host "Integration API Key is stored in .env as WMS_INTEGRATION_API_KEY." -ForegroundColor Green
    Write-Host "Change the default password immediately after first login." -ForegroundColor Yellow

    if (-not $NoBrowser) {
        Start-Process "http://127.0.0.1:$webPort"
    }
}
catch {
    Write-Host ""
    Write-Host "Start failed: $($_.Exception.Message)" -ForegroundColor Red
    Write-Host "Run 'docker compose -f deploy/docker-compose.yaml logs' for details." -ForegroundColor Yellow
    exit 1
}
