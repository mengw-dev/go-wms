# 无需 Pester；CI 的 pwsh 与本地 PowerShell 均可运行。
$ErrorActionPreference = "Stop"
$runner = Join-Path $PSScriptRoot "..\windows\test-go.ps1"
$saved = @{}
foreach ($name in @("WMS_TEST_DSN", "WMS_TEST_REDIS_ADDR", "WMS_TEST_REQUIRED")) {
    $saved[$name] = [Environment]::GetEnvironmentVariable($name, "Process")
}
$global:WmsGateFixture = @{ Invocations = 0; Mode = "pass" }

# 替代测试进程，验证真实脚本的门禁行为，绝不连接数据库。
function global:go {
    $global:WmsGateFixture.Invocations++
    $expectedRequired = if ($global:WmsGateFixture.Mode -eq "optional") { "0" } else { "1" }
    if ($env:WMS_TEST_REQUIRED -ne $expectedRequired) { throw "required mode not propagated" }
    if (($args -join " ") -ne "test -json ./... -count=1") { throw "unexpected go arguments" }
    if ($global:WmsGateFixture.Mode -eq "empty") { $global:LASTEXITCODE = 0; return }
    if ($global:WmsGateFixture.Mode -eq "malformed") { 'not JSON'; $global:LASTEXITCODE = 0; return }
    # 没有测试文件的包在 Go JSON 中也报告 skip，不能误计为集成测试被跳过。
    '{"Action":"skip","Package":"fixture/no-tests"}'
    if ($global:WmsGateFixture.Mode -in @("skip", "optional")) {
        '{"Action":"skip","Package":"fixture","Test":"TestIntegration"}'
    }
    '{"Action":"pass","Package":"fixture"}'
    $global:LASTEXITCODE = if ($global:WmsGateFixture.Mode -eq "failure") { 1 } else { 0 }
}

function Assert-Throws([scriptblock]$Action, [string]$MessagePart) {
    try { & $Action } catch {
        if ($_.Exception.Message -notlike "*$MessagePart*") { throw }
        return
    }
    throw "expected failure containing $MessagePart"
}

try {
    $env:WMS_TEST_REQUIRED = "previous-value"
    $env:WMS_TEST_DSN = "fixture-dsn"
    $env:WMS_TEST_REDIS_ADDR = "fixture-redis"

    $env:WMS_TEST_DSN = ""
    Assert-Throws { & $runner } "WMS_TEST_DSN"
    if ($global:WmsGateFixture.Invocations -ne 0) { throw "missing configuration still ran tests" }
    $env:WMS_TEST_DSN = "fixture-dsn"
    $env:WMS_TEST_REDIS_ADDR = ""
    Assert-Throws { & $runner } "WMS_TEST_REDIS_ADDR"
    $env:WMS_TEST_REDIS_ADDR = "fixture-redis"

    & $runner
    if ($env:WMS_TEST_REQUIRED -ne "previous-value") { throw "success leaked environment" }
    $global:WmsGateFixture.Mode = "failure"
    Assert-Throws { & $runner } "Go 测试失败"
    if ($env:WMS_TEST_REQUIRED -ne "previous-value") { throw "failure leaked environment" }
    $global:WmsGateFixture.Mode = "skip"
    Assert-Throws { & $runner } "跳过"
    $global:WmsGateFixture.Mode = "empty"
    Assert-Throws { & $runner } "没有成功完成测试的包"
    $global:WmsGateFixture.Mode = "malformed"
    Assert-Throws { & $runner } "JSON"
    $global:WmsGateFixture.Mode = "optional"
    $warnings = @(& $runner -AllowSkippedIntegration 3>&1)
    if (($warnings -join " ") -notlike "*不能作为完整验证通过*") { throw "optional mode omitted warning" }
    if ($env:WMS_TEST_REQUIRED -ne "previous-value") { throw "optional mode leaked environment" }
    Write-Host "验证门禁回归通过：缺少配置、测试失败、意外跳过均拒绝；显式部分检查有警告，环境恢复正确。"
} finally {
    Remove-Item Function:\go
    Remove-Variable WmsGateFixture -Scope Global
    foreach ($name in $saved.Keys) {
        [Environment]::SetEnvironmentVariable($name, $saved[$name], "Process")
    }
}
