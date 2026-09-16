[CmdletBinding()]
param(
    [string]$BaseUrl = "",
    [string]$ApiKey = "",
    [string]$DataFile = (Join-Path $PSScriptRoot "outbound-orders.json")
)

$ErrorActionPreference = "Stop"
$root = Split-Path -Parent (Split-Path -Parent $PSScriptRoot)
if (-not $BaseUrl) {
    $port = 80
    $envPath = Join-Path $root ".env"
    if (Test-Path -LiteralPath $envPath) {
        $portLine = Get-Content -LiteralPath $envPath | Where-Object { $_ -match '^\s*WMS_WEB_PORT\s*=' } | Select-Object -First 1
        if ($portLine -match '=\s*(\d+)') { $port = [int]$Matches[1] }
    }
    $BaseUrl = if ($port -eq 80) { "http://127.0.0.1" } else { "http://127.0.0.1:$port" }
}
$BaseUrl = $BaseUrl.TrimEnd("/")

if (-not $ApiKey) {
    $envPath = Join-Path $root ".env"
    if (Test-Path -LiteralPath $envPath) {
        foreach ($line in Get-Content -LiteralPath $envPath) {
            if ($line -match '^\s*WMS_INTEGRATION_API_KEY\s*=\s*(.+?)\s*$') {
                $ApiKey = $Matches[1].Trim('"').Trim("'")
                break
            }
        }
    }
}
if (-not $ApiKey) {
    throw "WMS_INTEGRATION_API_KEY was not found. Run start.ps1 or pass -ApiKey explicitly."
}

$orders = Get-Content -LiteralPath $DataFile -Raw -Encoding utf8 | ConvertFrom-Json
foreach ($order in @($orders)) {
    try {
        $response = Invoke-RestMethod -Method Post `
            -Uri "$BaseUrl/api/v1/integration/outbound-orders" `
            -Headers @{ "X-API-Key" = $ApiKey } `
            -ContentType "application/json" `
            -Body ($order | ConvertTo-Json -Depth 10) `
            -TimeoutSec 30
    }
    catch {
        Write-Host "[FAIL] $($order.biz_order_no): $($_.ErrorDetails.Message)" -ForegroundColor Red
        continue
    }
    if ($response.code -ne 0) {
        Write-Host "[FAIL] $($order.biz_order_no): $($response.msg)" -ForegroundColor Red
        continue
    }
    $data = $response.data
    $idempotent = if ($data.idempotent) { " idempotent=true" } else { "" }
    Write-Host "[OK] $($data.order_no) biz=$($data.biz_order_no) status=$($data.status)$idempotent" -ForegroundColor Green
}
