[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)]
    [ValidateSet('a', 'b')]
    [string]$Instance,
    [switch]$NoBrowser,
    [switch]$NoBuild
)

$ErrorActionPreference = "Stop"
. (Join-Path $PSScriptRoot "..\wms-common.ps1")
Set-Location $WmsRoot

function Wait-HttpReady {
    param([string]$Url, [int]$TimeoutSeconds = 180)
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

$instance = $Instance.ToLowerInvariant()
$defaults = if ($instance -eq 'a') {
    @{
        MYSQL_DATABASE = 'gowms_demo_a'
        WMS_SERVER_NODE = '11'
        WMS_API_BIND = '127.0.0.1'
        WMS_API_PORT = '18080'
        WMS_WEB_PORT = '18081'
        WMS_PROMETHEUS_PORT = '9091'
        WMS_GRAFANA_PORT = '3001'
        WMS_DEMO_USERNAME = 'demo-a'
    }
}
else {
    @{
        MYSQL_DATABASE = 'gowms_demo_b'
        WMS_SERVER_NODE = '12'
        WMS_API_BIND = '127.0.0.1'
        WMS_API_PORT = '18090'
        WMS_WEB_PORT = '18082'
        WMS_PROMETHEUS_PORT = '9092'
        WMS_GRAFANA_PORT = '3002'
        WMS_DEMO_USERNAME = 'demo-b'
    }
}

$examplePath = Join-Path $WmsRoot "deploy\env.demo-$instance.example"
$envPath = Join-Path $WmsRoot "deploy\env.demo-$instance"
if (-not (Test-Path -LiteralPath $envPath)) {
    Copy-Item -LiteralPath $examplePath -Destination $envPath
}

$values = Read-DotEnv -Path $envPath
$changed = $false
foreach ($key in $defaults.Keys) {
    if (-not $values.Contains($key) -or [string]::IsNullOrWhiteSpace($values[$key])) {
        $values[$key] = $defaults[$key]
        $changed = $true
    }
}
if (-not $values.Contains('WMS_DEMO_ENABLED') -or $values['WMS_DEMO_ENABLED'] -ne 'true') {
    $values['WMS_DEMO_ENABLED'] = 'true'
    $changed = $true
}
if (-not $values.Contains('WMS_DEMO_SESSION_TTL_SECONDS') -or [string]::IsNullOrWhiteSpace($values['WMS_DEMO_SESSION_TTL_SECONDS'])) {
    $values['WMS_DEMO_SESSION_TTL_SECONDS'] = '300'
    $changed = $true
}
if (-not $values.Contains('WMS_SERVER_TRUSTED_PROXIES') -or [string]::IsNullOrWhiteSpace($values['WMS_SERVER_TRUSTED_PROXIES'])) {
    $values['WMS_SERVER_TRUSTED_PROXIES'] = '172.16.0.0/12'
    $changed = $true
}
if (Test-PlaceholderSecret $values['MYSQL_ROOT_PASSWORD']) {
    $values['MYSQL_ROOT_PASSWORD'] = New-RandomHex -ByteCount 24
    $changed = $true
}
if (Test-PlaceholderSecret $values['JWT_SECRET'] -or $values['JWT_SECRET'].Length -lt 32) {
    $values['JWT_SECRET'] = New-RandomHex -ByteCount 48
    $changed = $true
}
if (Test-PlaceholderSecret $values['WMS_INTEGRATION_API_KEY'] -or $values['WMS_INTEGRATION_API_KEY'].Length -lt 24) {
    $values['WMS_INTEGRATION_API_KEY'] = New-RandomHex -ByteCount 24
    $changed = $true
}
if (Test-PlaceholderSecret $values['WMS_GRAFANA_ADMIN_PASSWORD'] -or $values['WMS_GRAFANA_ADMIN_PASSWORD'].Length -lt 12) {
    $values['WMS_GRAFANA_ADMIN_PASSWORD'] = New-RandomHex -ByteCount 16
    $changed = $true
}
if (-not $values.Contains('WMS_DEMO_PASSWORD') -or (Test-PlaceholderSecret $values['WMS_DEMO_PASSWORD']) -or $values['WMS_DEMO_PASSWORD'].Length -lt 8) {
    $values['WMS_DEMO_PASSWORD'] = New-RandomHex -ByteCount 8
    $changed = $true
}
if ($changed) {
    Save-DotEnv -Path $envPath -Values $values
}

Write-Step "Checking Docker"
Assert-Docker

$project = "gowms-demo-$instance"
$composePath = Get-ComposePath
$composeArgs = @('compose', '-p', $project, '--env-file', $envPath, '-f', $composePath, 'up', '-d')
if (-not $NoBuild) {
    $composeArgs += '--build'
}

Write-Step "Starting WMS demo instance $instance"
& docker @composeArgs
if ($LASTEXITCODE -ne 0) {
    throw "docker compose failed with exit code $LASTEXITCODE"
}

$webPort = [int]$values['WMS_WEB_PORT']
if (-not (Wait-HttpReady -Url "http://127.0.0.1:$webPort/")) {
    & docker compose -p $project --env-file $envPath -f $composePath ps
    throw "Demo instance $instance did not become healthy in time."
}

Write-Ok "Demo instance $instance is ready"
Write-Host "Web:  http://127.0.0.1:$webPort" -ForegroundColor Green
Write-Host "User: $($values['WMS_DEMO_USERNAME'])" -ForegroundColor Green
Write-Host "Pass: $($values['WMS_DEMO_PASSWORD'])" -ForegroundColor Green
Write-Host "Env:  $envPath" -ForegroundColor Green

if (-not $NoBrowser) {
    Start-Process "http://127.0.0.1:$webPort"
}