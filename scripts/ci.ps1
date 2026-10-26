# Run what CI runs that Windows can: every test (scripts\test.ps1), then the format, lint
# and type checks, the generated code, the site builds and checks/. The Windows twin of
# scripts/ci.sh; it ends by listing what it does not cover. Usage: scripts\ci.ps1
$ErrorActionPreference = 'Stop'
. "$PSScriptRoot\env.ps1"
$root = Split-Path -Parent $PSScriptRoot
$failures = [System.Collections.Generic.List[string]]::new()
# The checks written in bash run in Git for Windows' bash, which comes with git.
$bash = if ($onWindows) { Join-Path $env:ProgramFiles 'Git\bin\bash.exe' } else { 'bash' }
$devNull = if ($onWindows) { 'NUL' } else { '/dev/null' }

# Run-Step runs a native command in $Dir and records a failure by its exit code.
function Run-Step($Dir, $Name, [scriptblock]$Command) {
    Write-Host "`n== $Name"
    Push-Location (Join-Path $root $Dir)
    try {
        $global:LASTEXITCODE = 0
        & $Command
        if ($LASTEXITCODE -ne 0) { $failures.Add($Name) }
    } finally {
        Pop-Location
    }
}

# Unchanged reports a failure when generating code changed a committed file.
function Unchanged($Name, $Path) {
    git -C $root diff --quiet -- $Path
    if ($LASTEXITCODE -ne 0) { $failures.Add("$Name differs from the spec: regenerate and commit it") }
}

& "$PSScriptRoot\test.ps1"
if ($LASTEXITCODE -ne 0) { $failures.Add('tests (scripts\test.ps1)') }

Run-Step . 'go: gofmt' { & $bash checks/gofmt.sh origin/main }
# With the tag every file is built (scripts/ci.sh fails a file built only without it).
Run-Step backend 'go: vet' { go vet -tags=integration ./... }
Run-Step backend 'go: staticcheck' { go tool staticcheck -tags=integration ./... }
Run-Step backend 'go: build' { go build -o $devNull ./... }
Run-Step backend 'spec: generate Go' { go generate ./internal/httpapi }
Unchanged 'backend/internal/httpapi/api.gen.go' backend/internal/httpapi/api.gen.go

Run-Step engine 'engine: ruff' { uv run --frozen ruff check . }
Run-Step engine 'engine: ruff format' { uv run --frozen ruff format --check . }
Run-Step engine 'engine: mypy' { uv run --frozen mypy }

# The workspace scripts call pnpm by name, so it must be on PATH, not only `corepack pnpm`.
$pnpmShims = Join-Path ([System.IO.Path]::GetTempPath()) 'ttac-ci-pnpm'
New-Item -ItemType Directory -Force $pnpmShims | Out-Null
corepack enable --install-directory $pnpmShims pnpm
$env:PATH = "$pnpmShims$([System.IO.Path]::PathSeparator)$env:PATH"
Run-Step web 'web: generate TS' { corepack pnpm --filter '@ttac/shared' gen:api }
Unchanged 'web/shared/api' web/shared/api
Run-Step web 'web: lint' { corepack pnpm lint }
Run-Step web 'web: check' { corepack pnpm check }
Run-Step web 'web: build' { corepack pnpm build }
foreach ($site in 'site', 'admin') {
    $dist = Join-Path $root "web/$site/dist"
    if (-not (Test-Path (Join-Path $dist 'index.html'))) { $failures.Add("${site}: the build produced no dist/index.html") }
    $headers = Join-Path $dist '_headers'
    if (-not ((Test-Path $headers) -and (Select-String -Path $headers -Pattern '^  Content-Security-Policy:' -Quiet))) {
        $failures.Add("${site}: dist/_headers is missing or sets no CSP")
    }
    $want = (Get-Content (Join-Path $root "web/$site/.node-version") -Raw).Trim()
    $have = (node --version) -replace '^v(\d+).*', '$1'
    if ($want -ne $have) { $failures.Add("${site}: .node-version says '$want' but this is Node $have") }
}

Run-Step . 'checks: comment density' { & $bash checks/comment-density.sh }
Run-Step . 'checks: raw HTML' { & $bash checks/no-raw-html.sh }
Run-Step . 'checks: secret scan''s test' { & $bash checks/secret-scan.test.sh }
Run-Step . 'checks: secrets' { & $bash checks/secret-scan.sh --all }
Run-Step . 'checks: PowerShell parses' { & "$root/checks/ps-parse.ps1" }

Write-Host "`nNot covered here:"
Write-Host '  - Go race detector and the engine''s slow tests: Linux CI runs them'
Write-Host "  - deploy/'s pins and env values, image digests, workflows and shellcheck: CI runs them"
Write-Host '  - gofmt on Go files this branch does not change: main is clean, CI checks every file'
Write-Host '  - gitleaks and semgrep: steps of CI''s checks job, after scripts/ci.sh'
if ($failures.Count -gt 0) {
    Write-Host "`n$($failures.Count) step(s) FAILED: $($failures -join ', ')" -ForegroundColor Red
    exit 1
}
Write-Host "`nall steps passed" -ForegroundColor Green
