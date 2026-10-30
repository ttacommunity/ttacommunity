# Start the api, site and admin natively on Windows, against the local
# Postgres service, and check that they answer. Ctrl-C stops all three.
# Usage: scripts\dev.ps1   (after scripts\install.ps1)
$ErrorActionPreference = 'Stop'
. "$PSScriptRoot\env.ps1"
$root = Split-Path -Parent $PSScriptRoot
# .env is made once, so a setting added to .env.example since is missing from an older one.
$names = { param($file) Get-Content $file | ForEach-Object { if ($_ -match '^([A-Za-z_][A-Za-z0-9_]*)=') { $Matches[1] } } }
$have = & $names (Join-Path $root '.env')
$missing = @(& $names (Join-Path $root '.env.example') | Where-Object { $_ -notin $have })
if ($missing) { Write-Warning "dev.ps1: .env has no $($missing -join ', '), which .env.example sets; copy those lines across" }

# Login-Sql prints SQL that creates role $Role (no login) and login $Login in it when missing, and sets the
# login's password to psql's variable $PwVar: scripts/lib.sh's login_sql.
function Login-Sql($Role, $Login, $PwVar) {
    @"
DO `$`$ BEGIN CREATE ROLE $Role NOLOGIN; EXCEPTION WHEN duplicate_object THEN NULL; END `$`$;
DO `$`$ BEGIN CREATE ROLE $Login LOGIN; EXCEPTION WHEN duplicate_object THEN NULL; END `$`$;
SELECT format('ALTER ROLE %I PASSWORD %L', '$Login', :'$PwVar') \gexec
GRANT $Role TO $Login;
"@
}

# The api's read-only, admin and player logins, joining the roles the
# migrations grant to. Created or updated as the database's owner.
$sql = @(
    Login-Sql ratings_read ratings_api read_pw
    Login-Sql ratings_admin ratings_admin_api admin_pw
    Login-Sql ratings_player ratings_player_api player_pw
) -join "`n"
$sql | psql -X -q -v ON_ERROR_STOP=1 -v "read_pw=$env:RATINGS_READ_DB_PASSWORD" -v "admin_pw=$env:RATINGS_ADMIN_DB_PASSWORD" -v "player_pw=$env:RATINGS_PLAYER_DB_PASSWORD" $env:RATINGS_DATABASE_URL
if ($LASTEXITCODE -ne 0) { throw 'dev.ps1: could not create the api logins (is the Postgres service running?)' }

$env:COREPACK_ENABLE_DOWNLOAD_PROMPT = '0'
Push-Location (Join-Path $root 'web')
corepack pnpm install --frozen-lockfile --silent
$installed = $LASTEXITCODE
Pop-Location
if ($installed -ne 0) { throw 'dev.ps1: pnpm install failed' }

# Whether Google Fonts has newer fonts than the committed ones, checked beside the start as scripts/dev.sh does;
# shown once everything answers. Offline, it says nothing.
$fonts = if (Get-Command node -ErrorAction SilentlyContinue) {
    Start-Job -ArgumentList (Join-Path $root 'web\site') -ScriptBlock {
        param($Dir)
        Set-Location $Dir
        $out = @(node scripts/fonts.mjs check 2>$null)
        if ($LASTEXITCODE -ne 0 -and $out) { "dev.ps1: $($out[0])" }
    }
}

$procs = @(
    Start-Process go -ArgumentList 'run', './cmd/api' -WorkingDirectory (Join-Path $root 'backend') -NoNewWindow -PassThru
    # --ignore-lock stops astro detaching into a daemon when an AI agent runs this;
    # --host: on Windows astro otherwise listens on ::1 only.
    Start-Process corepack -ArgumentList 'pnpm', 'exec', 'astro', 'dev', '--ignore-lock', '--host', '127.0.0.1' -WorkingDirectory (Join-Path $root 'web\site') -NoNewWindow -PassThru
    Start-Process corepack -ArgumentList 'pnpm', 'exec', 'astro', 'dev', '--ignore-lock', '--host', '127.0.0.1' -WorkingDirectory (Join-Path $root 'web\admin') -NoNewWindow -PassThru
)

# Wait-Up polls $Url for up to 90 seconds and reports it under $Name.
function Wait-Up($Name, $Url) {
    for ($i = 0; $i -lt 90; $i++) {
        try {
            Invoke-WebRequest -Uri $Url -UseBasicParsing -TimeoutSec 2 | Out-Null
            Write-Host "  up      $Name  $Url"
            return $true
        } catch { Start-Sleep -Seconds 1 }
    }
    Write-Host "  DOWN    $Name  $Url did not answer within 90s"
    return $false
}

try {
    $apiUp = Wait-Up 'api' 'http://127.0.0.1:8080/healthz'
    $siteUp = Wait-Up 'site' 'http://127.0.0.1:4321/'
    $adminUp = Wait-Up 'admin' 'http://127.0.0.1:4322/'
    if (-not ($apiUp -and $siteUp -and $adminUp)) { throw 'dev.ps1: something did not start' }
    if ($fonts) { Wait-Job $fonts -Timeout 20 | Out-Null; Receive-Job $fonts | Write-Host; Remove-Job $fonts -Force }
    Write-Host 'dev.ps1: running. Ctrl-C stops api, site and admin.'
    Wait-Process -Id ($procs | ForEach-Object Id)
} finally {
    # go run and corepack start child processes, so stop each whole tree.
    foreach ($p in $procs) { taskkill /T /F /PID $p.Id 2>&1 | Out-Null }
}
