# Install everything needed to develop natively on Windows, then create the
# local databases and logins. Safe to run again: each step skips what exists.
# Usage: scripts\install.ps1 [-Port 55432]   (PG_SUPERPASSWORD, if set, answers the prompt)
param(
    [int]$Port = 55432
)
$ErrorActionPreference = 'Stop'
Set-Location (Split-Path -Parent $PSScriptRoot)

function Test-Command($name) { [bool](Get-Command $name -ErrorAction SilentlyContinue) }

if (-not (Test-Command winget)) {
    throw 'install.ps1: winget is missing. Install "App Installer" from the Microsoft Store, then run this again.'
}
if (-not (Test-Command git)) {
    winget install --id Git.Git --exact --silent --accept-package-agreements --accept-source-agreements
}

# mise installs the toolchain mise.toml pins: the same versions as Linux and CI.
if (-not (Test-Command mise)) {
    winget install --id jdx.mise --exact --silent --accept-package-agreements --accept-source-agreements
    $env:PATH = [Environment]::GetEnvironmentVariable('PATH', 'Machine') + ';' + [Environment]::GetEnvironmentVariable('PATH', 'User')
}
mise trust
mise install go node uv
if ($LASTEXITCODE -ne 0) { throw 'install.ps1: mise install failed' }

# PostgreSQL 18 as a Windows service. Its superuser is postgres; the
# password chosen here is needed only by this script.
$service = Get-Service -Name 'postgresql-x64-18' -ErrorAction SilentlyContinue
if ($env:PG_SUPERPASSWORD) {
    $superPlain = $env:PG_SUPERPASSWORD
} else {
    $super = Read-Host -AsSecureString 'Password for the local postgres superuser (new, or the existing one)'
    $superPlain = [Runtime.InteropServices.Marshal]::PtrToStringAuto([Runtime.InteropServices.Marshal]::SecureStringToBSTR($super))
}
if (-not $service) {
    winget install --id PostgreSQL.PostgreSQL.18 --exact --accept-package-agreements --accept-source-agreements `
        --override "--mode unattended --unattendedmodeui none --superpassword `"$superPlain`" --serverport $Port --disable-components pgAdmin,stackbuilder"
    if ($LASTEXITCODE -ne 0) { throw 'install.ps1: PostgreSQL install failed' }
} else {
    Write-Host "install.ps1: using the existing service $($service.Name) on port $Port"
}

. "$PSScriptRoot\env.ps1"
if ($Port -ne 55432) {
    (Get-Content .env -Raw) -replace '127\.0\.0\.1:55432/', "127.0.0.1:$Port/" | Set-Content .env -NoNewline
    . "$PSScriptRoot\env.ps1"
}

# The roles the containers create as their superusers: ratings and app own
# their databases; ttac runs the tests, which create and drop databases.
$env:PGPASSWORD = $superPlain
$sql = @"
SELECT format('CREATE ROLE %I LOGIN CREATEDB CREATEROLE', r) FROM unnest(ARRAY['ratings','app','ttac']) r
  WHERE NOT EXISTS (SELECT FROM pg_roles WHERE rolname = r) \gexec
SELECT format('ALTER ROLE ratings PASSWORD %L', :'ratings_pw') \gexec
SELECT format('ALTER ROLE app PASSWORD %L', :'app_pw') \gexec
ALTER ROLE ttac PASSWORD 'local-only';
SELECT format('CREATE DATABASE %I OWNER %I', d, o) FROM (VALUES ('ratings','ratings'),('app','app'),('ttac_test','ttac')) v(d, o)
  WHERE NOT EXISTS (SELECT FROM pg_database WHERE datname = d) \gexec
"@
$sql | psql -X -q -v ON_ERROR_STOP=1 -v "ratings_pw=$env:RATINGS_DB_PASSWORD" -v "app_pw=$env:APP_DB_PASSWORD" -h 127.0.0.1 -p $Port -U postgres -d postgres
$failed = $LASTEXITCODE
Remove-Item Env:PGPASSWORD
$superPlain = $null
if ($failed -ne 0) { throw 'install.ps1: creating the local roles and databases failed' }

git config core.hooksPath .githooks
Write-Host 'install.ps1: done. Next: scripts\ratings-db.ps1 <baseline folder>, then scripts\dev.ps1'
