[CmdletBinding()]
param(
    [switch]$NoBuild
)

$ErrorActionPreference = "Stop"
. (Join-Path $PSScriptRoot "wms-common.ps1")
$root = Split-Path -Parent $PSScriptRoot
$composeFile = Join-Path $root "deploy/docker-compose.yaml"
$projectName = "gowms-e2e"
$webPort = 28081

if (-not (Get-Command docker -ErrorAction SilentlyContinue)) {
    throw "Docker was not found. Install Docker Desktop first."
}

Assert-Node

$env:MYSQL_ROOT_PASSWORD = "gowms-e2e-root"
$env:MYSQL_DATABASE = "gowms_e2e"
$env:JWT_SECRET = "gowms-e2e-jwt-secret-0123456789abcdef0123456789abcdef"
$env:WMS_INTEGRATION_API_KEY = "gowms-e2e-integration-api-key-0123456789abcdef"
$env:WMS_API_BIND = "127.0.0.1"
$env:WMS_API_PORT = "28080"
$env:WMS_WEB_PORT = "$webPort"
$env:WMS_DEMO_ENABLED = "true"
$env:WMS_DEMO_INSTANCES = "5"
$env:WMS_DEMO_PASSWORD = "demo123456"
$env:WMS_PERSONAL_ENABLED = "true"
$env:WMS_PERSONAL_INSTANCES = "3"
$env:WMS_PERSONAL_PASSWORD = "user123456"
$env:E2E_BASE_URL = "http://127.0.0.1:$webPort"

function Wait-WebReady {
    param([int]$TimeoutSeconds = 180)

    $deadline = (Get-Date).AddSeconds($TimeoutSeconds)
    while ((Get-Date) -lt $deadline) {
        try {
            $response = Invoke-WebRequest -Uri "$env:E2E_BASE_URL/healthz" -UseBasicParsing -TimeoutSec 3
            if ($response.StatusCode -eq 200) {
                return
            }
        }
        catch {
            Start-Sleep -Seconds 3
        }
    }
    throw "E2E services did not become healthy in time."
}

$upArgs = @("-p", $projectName, "-f", $composeFile, "up", "-d")
if (-not $NoBuild) {
    $upArgs += "--build"
}

try {
    Write-Host "`n==> Starting isolated E2E environment" -ForegroundColor Cyan
    & docker compose @upArgs
    if ($LASTEXITCODE -ne 0) {
        throw "docker compose failed with exit code $LASTEXITCODE"
    }

    Write-Host "==> Waiting for $env:E2E_BASE_URL" -ForegroundColor Cyan
    Wait-WebReady

    Write-Host "==> Running Playwright tests" -ForegroundColor Cyan
    & npm --prefix (Join-Path $root "web") run test:e2e
    if ($LASTEXITCODE -ne 0) {
        throw "Playwright tests failed with exit code $LASTEXITCODE"
    }
}
finally {
    Write-Host "`n==> Removing isolated E2E environment" -ForegroundColor Cyan
    & docker compose -p $projectName -f $composeFile down -v --remove-orphans
}
