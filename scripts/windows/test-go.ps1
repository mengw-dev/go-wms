[CmdletBinding()]
param([switch]$AllowSkippedIntegration)

$ErrorActionPreference = "Stop"
$previousRequired = [Environment]::GetEnvironmentVariable("WMS_TEST_REQUIRED", "Process")
$skipped = 0
$passedPackages = 0

if (-not $AllowSkippedIntegration) {
    foreach ($name in @("WMS_TEST_DSN", "WMS_TEST_REDIS_ADDR")) {
        if ([string]::IsNullOrWhiteSpace([Environment]::GetEnvironmentVariable($name, "Process"))) {
            throw "完整测试必须显式配置 $name，指向本地或专用测试实例；不能使用生产实例。仅做部分检查可显式使用 -AllowSkippedIntegration。"
        }
    }
} else {
    Write-Warning "已允许集成测试跳过，本次不能作为完整验证通过。"
}

Push-Location (Join-Path $PSScriptRoot "..\..")
try {
    $env:WMS_TEST_REQUIRED = if ($AllowSkippedIntegration) { "0" } else { "1" }
    & go test -json ./... -count=1 | ForEach-Object {
        $event = $_ | ConvertFrom-Json -ErrorAction Stop
        $isTest = $event.PSObject.Properties.Name -contains "Test"
        if ($event.Action -eq "skip" -and $isTest) { $skipped++ }
        if ($event.Action -eq "pass" -and -not $isTest) { $passedPackages++ }
        if ($event.PSObject.Properties.Name -contains "Output") {
            Write-Host -NoNewline $event.Output
        }
    }
    if ($LASTEXITCODE -ne 0) {
        throw "Go 测试失败（退出码 $LASTEXITCODE）；不能认定为验证通过。"
    }
    if ($passedPackages -eq 0) {
        throw "没有成功完成测试的包，不能认定为验证通过。"
    }
    if ($skipped -gt 0 -and -not $AllowSkippedIntegration) {
        throw "完整测试检测到 $skipped 项跳过；不能认定为验证通过。"
    }
    if ($AllowSkippedIntegration) {
        Write-Warning "Go 部分检查结束：跳过 $skipped 项，未认证完整集成覆盖。"
    } else {
        Write-Host "Go 测试通过：依赖强制模式，跳过 0 项。" -ForegroundColor Green
    }
} finally {
    [Environment]::SetEnvironmentVariable("WMS_TEST_REQUIRED", $previousRequired, "Process")
    Pop-Location
}
