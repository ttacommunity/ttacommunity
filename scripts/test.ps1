# Run every test natively on Windows: Go (unit and integration), the engine,
# web and the sign-in worker. The Windows twin of scripts/test.sh; keep the
# steps in step with scripts/ci.sh's test half. Usage: scripts\test.ps1
$ErrorActionPreference = 'Stop'
. "$PSScriptRoot\env.ps1"
$root = Split-Path -Parent $PSScriptRoot
$failures = [System.Collections.Generic.List[string]]::new()

# Run-Step runs a native command in $Dir and records a failure by its exit code.
function Run-Step($Dir, $Name, [scriptblock]$Command) {
    Write-Host "`n== $Name"
    Push-Location (Join-Path $root $Dir)
    try {
        & $Command
        if ($LASTEXITCODE -ne 0) { $failures.Add($Name) }
    } finally {
        Pop-Location
    }
}

# The tests' own login, made by install.ps1, on the same server as .env's databases.
if ($env:RATINGS_DATABASE_URL -notmatch '@([^/]+)/') { throw 'test.ps1: RATINGS_DATABASE_URL in .env has no host' }
$env:TEST_DATABASE_URL = "postgres://ttac:local-only@$($Matches[1])/ttac_test?sslmode=disable"

$cover = New-TemporaryFile
Run-Step backend 'go: unit tests' { go test -count=1 "-coverprofile=$cover" ./... }
# Coverage is reported, never enforced, as in scripts/ci.sh.
Push-Location (Join-Path $root backend)
Write-Host "  unit tests' total coverage: $((go tool cover "-func=$cover" | Select-Object -Last 1) -split '\s+' | Select-Object -Last 1)"
Pop-Location
Remove-Item $cover
Run-Step backend 'go: integration tests' { go test -tags=integration -count=1 ./... }
Run-Step engine 'engine: sync' { uv sync --frozen --quiet }
Run-Step engine 'engine: tests' { uv run --frozen pytest -q -m 'not slow' }
$env:COREPACK_ENABLE_DOWNLOAD_PROMPT = '0'
Run-Step web 'web: install' { corepack pnpm install --frozen-lockfile --silent }
Run-Step web 'web: tests' { corepack pnpm test }
Run-Step infra/access-oidc 'sign-in worker: install' { npm ci --omit=dev --ignore-scripts --no-audit --no-fund --silent }
Run-Step infra/access-oidc 'sign-in worker: tests' { npm test }
Run-Step infra/build-notifier 'build notifier: tests' { npm test }

Write-Host "`nNot covered here:"
Write-Host '  - Go race detector (needs a C compiler on Windows; Linux CI runs it)'
Write-Host '  - engine: slow tests, and the SIGTERM stop (Windows has no catchable SIGTERM)'
Write-Host '  - lint, type checks, site builds and checks/: scripts/ci.sh, in CI'
if ($failures.Count -gt 0) {
    Write-Host "`n$($failures.Count) step(s) FAILED: $($failures -join ', ')" -ForegroundColor Red
    exit 1
}
Write-Host "`nall tests passed" -ForegroundColor Green
