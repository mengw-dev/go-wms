[CmdletBinding()]
param(
    [switch]$WithRace,
    [switch]$WithE2E
)

$ErrorActionPreference = "Stop"
$root = Resolve-Path (Join-Path $PSScriptRoot "..\..")
Set-Location $root

function Invoke-Step {
    param(
        [string]$Name,
        [scriptblock]$Action
    )
    Write-Host ""
    Write-Host "==> $Name" -ForegroundColor Cyan
    & $Action
    if ($LASTEXITCODE -ne 0) {
        throw "$Name failed with exit code $LASTEXITCODE"
    }
}

Invoke-Step "Go formatting" {
    $output = gofmt -l .
    if ($output) {
        Write-Host $output
        exit 1
    }
}
Invoke-Step "Go build" { go build ./... }
Invoke-Step "Go tests" { go test ./... -count=1 }
Invoke-Step "Go vet" { go vet ./... }
Invoke-Step "golangci-lint" {
    go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.13.2 run
}

if ($WithRace) {
    Invoke-Step "Go race tests (Linux + MySQL + Redis)" {
        if (-not (Test-Path .env)) {
            throw "Race verification requires .env and the Compose MySQL/Redis services."
        }
        $values = @{}
        foreach ($line in Get-Content .env) {
            if ($line -match '^([^#=]+)=(.*)$') {
                $values[$matches[1].Trim()] = $matches[2].Trim()
            }
        }
        $password = $values["MYSQL_ROOT_PASSWORD"]
        $database = if ($values["MYSQL_DATABASE"]) { $values["MYSQL_DATABASE"] } else { "gowms" }
        if (-not $password) {
            throw "MYSQL_ROOT_PASSWORD is missing from .env"
        }
        foreach ($container in @("deploy-mysql-1", "deploy-redis-1")) {
            $running = docker inspect --format '{{.State.Running}}' $container 2>$null
            if ($running -ne "true") {
                throw "$container is not running; start the Compose stack first."
            }
        }
        $dsn = "root:$password@tcp(mysql:3306)/$database?charset=utf8mb4&parseTime=True&loc=Local"
        docker run --rm --network deploy_default -v "$($root.Path):/src" -w /src `
            -e "WMS_TEST_DSN=$dsn" -e "WMS_MYSQL_DSN=$dsn" `
            -e WMS_TEST_REDIS_ADDR=redis:6379 -e WMS_TEST_REQUIRED=1 `
            golang:1.26-alpine sh -c "apk add --no-cache gcc musl-dev >/dev/null && CGO_ENABLED=1 go test -race ./... -count=1"
    }
}

Push-Location web
try {
    if (-not (Test-Path node_modules)) {
        Invoke-Step "npm ci" { npm ci }
    }
    Invoke-Step "Frontend lint" { npm run lint }
    Invoke-Step "Frontend unit tests" { npm test }
    Invoke-Step "Frontend build" { npm run build }
    if ($WithE2E) {
        # E2E must run against a fresh database and Redis instance. Running it
        # against the persistent local stack makes the suite order-dependent.
        Invoke-Step "Playwright E2E (isolated Compose)" {
            & (Join-Path $root "scripts\e2e.ps1")
        }
    }
}
finally {
    Pop-Location
}

if (Test-Path .env) {
    Invoke-Step "Docker Compose config" {
        docker compose --env-file .env -f deploy/docker-compose.yaml config --quiet
    }
}

Invoke-Step "Git diff check" { git diff --check }
Write-Host ""
Write-Host "All verification steps passed." -ForegroundColor Green
