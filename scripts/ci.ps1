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
$notCovered = [System.Collections.Generic.List[string]]::new()

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

# Bash-Has says whether Git for Windows' bash finds every one of $Tools.
function Bash-Has([string[]]$Tools) {
    & $bash -c "for t in $($Tools -join ' '); do command -v `$t >/dev/null || exit 1; done"
    return $LASTEXITCODE -eq 0
}

# Unchanged reports a failure when generating code changed a committed file.
function Unchanged($Name, $Path) {
    git -C $root diff --quiet -- $Path
    if ($LASTEXITCODE -ne 0) { $failures.Add("$Name differs from the spec: regenerate and commit it") }
}

& "$PSScriptRoot\test.ps1"
if ($LASTEXITCODE -ne 0) { $failures.Add('tests (scripts\test.ps1)') }

Run-Step . 'go: gofmt' { & $bash checks/gofmt.sh origin/main }
# With the tag every file is built, until a file says it is built only without it.
git -C $root grep --untracked -qE '^//go:build .*!integration' -- '*.go'
if ($LASTEXITCODE -eq 0) { $failures.Add('a file is built only without -tags=integration: vet and staticcheck would skip it') }
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
    if (-not (Select-String -Path (Join-Path $root "web/$site/package.json") -Pattern 'csp-inline\.mjs dist' -Quiet)) {
        $failures.Add("${site}: its build no longer runs checks/csp-inline.mjs")
    }
    $want = (Get-Content (Join-Path $root "web/$site/.node-version") -Raw).Trim()
    $have = (node --version) -replace '^v(\d+).*', '$1'
    if ($want -ne $have) { $failures.Add("${site}: .node-version says '$want' but this is Node $have") }
}

Run-Step . 'checks: comment density' { & $bash checks/comment-density.sh }
Run-Step . 'checks: raw HTML' { & $bash checks/no-raw-html.sh }
Run-Step . 'checks: secret scan''s test' { & $bash checks/secret-scan.test.sh }
Run-Step . 'checks: secrets' { & $bash checks/secret-scan.sh --all }
Run-Step . 'checks: sign-off check''s test' { & $bash checks/dco.test.sh }
Run-Step . 'checks: pin check''s test' { & $bash checks/deploy-current.test.sh }
Run-Step . 'checks: CI path filter''s test' { & $bash checks/ci-changes.test.sh }
if (Bash-Has kustomize, yq) {
    Run-Step . 'checks: env values check''s test' { & $bash checks/deploy-env-strings.test.sh }
} else {
    $notCovered.Add("deploy/'s env values check's test: needs kustomize and yq in Git's bash")
}
if (Bash-Has jq) {
    Run-Step . 'checks: golden hashes'' test' { & $bash checks/engine-golden.test.sh }
    Run-Step . 'checks: golden hashes' { & $bash checks/engine-golden.sh }
    Run-Step . 'checks: digest check''s test' { & $bash checks/verify-digests.test.sh }
} else {
    $notCovered.Add("the engine's golden hashes and the digest check's test: need jq in Git's bash")
}
Run-Step . 'checks: PowerShell parses' { & "$root/checks/ps-parse.ps1" }
Write-Host "`n== git hooks"
if ((git -C $root config core.hooksPath) -ne '.githooks') { $failures.Add('git hooks are not enabled: git config core.hooksPath .githooks') }

$notCovered.AddRange([string[]]@(
    'Go race detector and the engine''s slow tests: Linux CI runs them'
    "deploy/'s pins and env values, image digests, workflows and shellcheck: CI runs them"
    'gofmt on Go files this branch does not change: main is clean, CI checks every file'
    "sign-offs on an outsider's pull request (checks/dco.sh): CI checks each pull request"
    "Cloudflare's own Workers build (check its build log after a merge to main)"
    'container images: built by the CI image jobs, not by this script'
    'gitleaks and semgrep: steps of CI''s checks job, after scripts/ci.sh'
))
Write-Host "`nNot covered here:"
foreach ($item in $notCovered) { Write-Host "  - $item" }
if ($failures.Count -gt 0) {
    Write-Host "`n$($failures.Count) step(s) FAILED: $($failures -join ', ')" -ForegroundColor Red
    exit 1
}
Write-Host "`nall steps passed" -ForegroundColor Green
