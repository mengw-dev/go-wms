[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)]
    [ValidateSet('a', 'b')]
    [string]$Instance
)

$ErrorActionPreference = "Stop"
. (Join-Path $PSScriptRoot "..\wms-common.ps1")
Set-Location $WmsRoot

$instance = $Instance.ToLowerInvariant()
$envPath = Join-Path $WmsRoot "deploy\env.demo-$instance"
if (-not (Test-Path -LiteralPath $envPath)) {
    throw "Demo env file not found: $envPath"
}

Write-Step "Checking Docker"
Assert-Docker

$project = "gowms-demo-$instance"
& docker compose -p $project --env-file $envPath -f (Get-ComposePath) down --remove-orphans
if ($LASTEXITCODE -ne 0) {
    throw "docker compose failed with exit code $LASTEXITCODE"
}
Write-Ok "Demo instance $instance stopped. Volumes were preserved."