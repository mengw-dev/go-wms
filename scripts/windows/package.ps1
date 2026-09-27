[CmdletBinding()]
param(
    [string]$OutputPath = ""
)

$ErrorActionPreference = "Stop"
$root = Resolve-Path (Join-Path $PSScriptRoot "..\..")
Set-Location $root

if (git status --porcelain) {
    throw "打包前工作区必须干净。请先提交或移除未跟踪文件，避免把 .env、测试产物或本地文件带入交付包。"
}

$commit = (git rev-parse --short HEAD).Trim()
if (-not $OutputPath) {
    $OutputPath = Join-Path $root "dist\wms-$commit.zip"
} elseif (-not [System.IO.Path]::IsPathRooted($OutputPath)) {
    $OutputPath = Join-Path $root $OutputPath
}

$parent = Split-Path -Parent $OutputPath
if ($parent -and -not (Test-Path $parent)) {
    New-Item -ItemType Directory -Path $parent | Out-Null
}

# git archive 只包含当前提交中的跟踪文件，不会包含 .git、.env、
# node_modules、测试产物或任何未跟踪的本地文件。
git archive --format=zip --output=$OutputPath HEAD
if ($LASTEXITCODE -ne 0) {
    throw "git archive failed with exit code $LASTEXITCODE"
}

Write-Host "Created $OutputPath from $commit" -ForegroundColor Green
