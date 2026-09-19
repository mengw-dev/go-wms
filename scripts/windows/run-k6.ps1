[CmdletBinding()]
param(
    [ValidateSet('check', 'flow', 'smoke', 'stress', 'wave')]
    [string]$Mode = 'check',
    [string]$BaseUrl = 'http://127.0.0.1:8080',
    [string]$Username = 'admin',
    [string]$Password = 'admin123',
    [string]$WarehouseId = '',
    [string]$SkuId = '',
    [int]$OrderQty = 1,
    [switch]$RemoteWrite
)

$ErrorActionPreference = "Stop"
$root = Split-Path -Parent (Split-Path -Parent $PSScriptRoot)
$k6Root = Join-Path $root "scripts\k6"
$k6 = Get-Command k6 -ErrorAction SilentlyContinue
if (-not $k6) {
    throw "k6 was not found. Install it from https://grafana.com/docs/k6/latest/set-up/install-k6/ or use Docker."
}

$env:BASE_URL = $BaseUrl
$env:K6_USERNAME = $Username
$env:K6_PASSWORD = $Password
if ($WarehouseId) {
    $env:WAREHOUSE_ID = $WarehouseId
}
if ($SkuId) {
    $env:SKU_ID = $SkuId
}
$env:ORDER_QTY = "$OrderQty"

$scriptPath = switch ($Mode) {
    'check'  { Join-Path $k6Root 'check-env.js' }
    'flow'   { Join-Path $k6Root 'demo-flow.js' }
    'smoke'  { Join-Path $k6Root 'outbound-e2e.js' }
    'stress' { Join-Path $k6Root 'outbound-stress.js' }
    'wave'   { Join-Path $k6Root 'pick-stress.js' }
}

$k6Args = @('run')
if ($RemoteWrite) {
    $env:K6_PROMETHEUS_RW_SERVER_URL = "http://127.0.0.1:9090/api/v1/write"
    $k6Args += @('--out', 'experimental-prometheus-rw')
}
$k6Args += $scriptPath

Write-Host "`n==> k6 mode=$Mode base=$BaseUrl" -ForegroundColor Cyan
if ($Mode -in @('smoke', 'stress', 'wave') -and (-not $WarehouseId -or -not $SkuId)) {
    throw "Mode $Mode requires -WarehouseId and -SkuId. Run -Mode check first."
}

# k6 的 console 日志输出到 stderr，而 PowerShell 5.1 在 $ErrorActionPreference='Stop' 下
# 会把原生命令的 stderr 输出当成终止性错误，导致脚本在第一条日志就中断。
# 因此调用期间临时放开，改用退出码判断成败。
$previousErrorAction = $ErrorActionPreference
$ErrorActionPreference = 'Continue'
& $k6.Source @k6Args
$k6ExitCode = $LASTEXITCODE
$ErrorActionPreference = $previousErrorAction

if ($k6ExitCode -ne 0) {
    throw "k6 failed with exit code $k6ExitCode"
}