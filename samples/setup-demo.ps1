[CmdletBinding()]
param(
    [string]$BaseUrl = "",
    [string]$Username = "admin",
    [string]$Password = "admin123"
)

$ErrorActionPreference = "Stop"
if (-not $BaseUrl) {
    $port = 80
    $envPath = Join-Path (Split-Path -Parent $PSScriptRoot) ".env"
    if (Test-Path -LiteralPath $envPath) {
        $portLine = Get-Content -LiteralPath $envPath | Where-Object { $_ -match '^\s*WMS_WEB_PORT\s*=' } | Select-Object -First 1
        if ($portLine -match '=\s*(\d+)') { $port = [int]$Matches[1] }
    }
    $BaseUrl = if ($port -eq 80) { "http://127.0.0.1" } else { "http://127.0.0.1:$port" }
}
$BaseUrl = $BaseUrl.TrimEnd("/")

function Invoke-WmsApi {
    param(
        [string]$Method,
        [string]$Path,
        [object]$Body = $null,
        [string]$Token = ""
    )

    $headers = @{}
    if ($Token) {
        $headers.Authorization = "Bearer $Token"
    }
    $params = @{
        Uri         = "$BaseUrl/api/v1$Path"
        Method      = $Method
        Headers     = $headers
        TimeoutSec  = 30
        ErrorAction = "Stop"
    }
    if ($null -ne $Body) {
        $params.ContentType = "application/json"
        $params.Body = ($Body | ConvertTo-Json -Depth 10)
    }

    try {
        $response = Invoke-RestMethod @params
    }
    catch {
        $detail = $_.ErrorDetails.Message
        throw "$Method $Path failed: $detail"
    }
    if ($response.code -ne 0) {
        throw "$Method $Path -> $($response.msg)"
    }
    return $response.data
}

function Get-WmsList {
    param([string]$Path, [string]$Token)
    return Invoke-WmsApi -Method Get -Path $Path -Token $Token
}

$login = Invoke-WmsApi -Method Post -Path "/login" -Body @{
    username = $Username
    password = $Password
}
$token = $login.token

$warehouses = @(
    @{ code = "WH01"; name = "华东一号仓"; remark = "演示数据：上海中心仓" },
    @{ code = "WH02"; name = "华南二号仓"; remark = "演示数据：广州区域仓" }
)
foreach ($warehouse in $warehouses) {
    $list = Get-WmsList -Path "/basic/warehouses?page=1&page_size=100&keyword=$($warehouse.code)" -Token $token
    if (-not ($list.list | Where-Object { $_.code -eq $warehouse.code })) {
        Invoke-WmsApi -Method Post -Path "/basic/warehouses" -Token $token -Body $warehouse | Out-Null
        Write-Host "Created warehouse $($warehouse.code)"
    }
}

$warehouseRows = Get-WmsList -Path "/basic/warehouses?page=1&page_size=100" -Token $token
foreach ($warehouse in $warehouses) {
    $row = $warehouseRows.list | Where-Object { $_.code -eq $warehouse.code } | Select-Object -First 1
    foreach ($zone in @("A01", "B01")) {
        Invoke-WmsApi -Method Post -Path "/basic/locations/batch" -Token $token -Body @{
            warehouse_id = $row.id
            zone         = $zone
            row_from     = 1
            row_to       = 2
            col_from     = 1
            col_to       = 3
        } | Out-Null
    }
}

$skus = @(
    @{ code = "SKU000001"; barcode = "6901234500011"; name = "农夫山泉饮用天然水"; spec = "550ml×24瓶"; unit = "箱" },
    @{ code = "SKU000002"; barcode = "6901234500028"; name = "可口可乐汽水"; spec = "330ml×24罐"; unit = "箱" },
    @{ code = "SKU000003"; barcode = "6901234500035"; name = "康师傅红烧牛肉面"; spec = "105g×12桶"; unit = "箱" },
    @{ code = "SKU000004"; barcode = "6901234500042"; name = "旺旺雪饼"; spec = "540g"; unit = "袋" },
    @{ code = "SKU000005"; barcode = "6901234500059"; name = "双汇王中王火腿肠"; spec = "60g×40支"; unit = "箱" },
    @{ code = "SKU000006"; barcode = "6901234500066"; name = "维达超韧抽纸"; spec = "3层120抽×24包"; unit = "箱" },
    @{ code = "SKU000007"; barcode = "6901234500073"; name = "蓝月亮深层洁净洗衣液"; spec = "3kg"; unit = "瓶" },
    @{ code = "SKU000008"; barcode = "6901234500080"; name = "南孚5号碱性电池"; spec = "1.5V×40粒"; unit = "盒" },
    @{ code = "SKU000009"; barcode = "6901234500097"; name = "苏泊尔电热水壶"; spec = "1.5L 1800W"; unit = "台" },
    @{ code = "SKU000010"; barcode = "6901234500103"; name = "3M防护口罩"; spec = "KN95×50只"; unit = "盒" }
)

foreach ($sku in $skus) {
    $list = Get-WmsList -Path "/basic/skus?page=1&page_size=100&keyword=$($sku.code)" -Token $token
    if (-not ($list.list | Where-Object { $_.code -eq $sku.code })) {
        Invoke-WmsApi -Method Post -Path "/basic/skus" -Token $token -Body $sku | Out-Null
        Write-Host "Created SKU $($sku.code)"
    }
}

Write-Host "Demo base data is ready at $BaseUrl" -ForegroundColor Green
Write-Host "Next: import samples/inbound/入库单批量导入示例.xlsx from the inbound page." -ForegroundColor Green
