[CmdletBinding()]
param()

$ErrorActionPreference = "Stop"
$root = Resolve-Path (Join-Path $PSScriptRoot "..\..")
$internal = Join-Path $root "internal"
$violations = [System.Collections.Generic.List[string]]::new()

Get-ChildItem -Path $internal -Recurse -Filter "*.go" -File |
    Where-Object { $_.Name -notlike "*_test.go" } |
    ForEach-Object {
        $lines = Get-Content -LiteralPath $_.FullName
        for ($i = 0; $i -lt $lines.Count; $i++) {
            if ($lines[$i] -notmatch "\.(Table|Raw)\(") { continue }
            $end = [Math]::Min($i + 8, $lines.Count - 1)
            $context = ($lines[$i..$end] -join "`n")
            if ($context -notmatch "tenant\.Scope|tenantScope|tenant_id") {
                $relative = Resolve-Path -Relative $_.FullName
                $violations.Add("${relative}:$($i + 1): $($lines[$i].Trim())")
            }
        }
    }

if ($violations.Count -gt 0) {
    Write-Host "Raw table queries must explicitly scope tenant_id:" -ForegroundColor Red
    $violations | ForEach-Object { Write-Host $_ -ForegroundColor Red }
    exit 1
}

Write-Host "Tenant raw query audit passed." -ForegroundColor Green
