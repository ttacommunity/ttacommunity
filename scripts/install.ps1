# Install everything needed to develop natively on Windows, then create the
# local databases and logins. Safe to run again: each step skips what exists.
# Usage: scripts\install.ps1 [-Service] [-Port 55432]   (PG_SUPERPASSWORD, if set, answers the prompt)
param(
    [switch]$Service,
    [int]$Port = 55432
)
$ErrorActionPreference = 'Stop'
$env:MISE_AUTO_INSTALL = 'false'
$env:MISE_EXEC_AUTO_INSTALL = 'false'
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

# PostgreSQL 18: by default a user install (EDB's binaries, no admin, started
# by env.ps1 when needed); with -Service, a Windows service from winget (UAC).
# Its superuser is postgres; the password chosen here is needed only by this script.
if ($env:PG_SUPERPASSWORD) {
    $superPlain = $env:PG_SUPERPASSWORD
} else {
    $super = Read-Host -AsSecureString 'Password for the local postgres superuser (new, or the existing one)'
    $superPlain = [Runtime.InteropServices.Marshal]::PtrToStringAuto([Runtime.InteropServices.Marshal]::SecureStringToBSTR($super))
}
. "$PSScriptRoot\env.ps1"
if ($Port -ne 55432) {
    (Get-Content .env -Raw) -replace '127\.0\.0\.1:55432/', "127.0.0.1:$Port/" | Set-Content .env -NoNewline
}

if ($Service) {
    $existing = Get-Service -Name 'postgresql-x64-18' -ErrorAction SilentlyContinue
    if ($existing) {
        Write-Host "install.ps1: using the existing service $($existing.Name) on port $Port"
    } else {
        # The installer takes its password on the command line, where other local
        # processes can read it, so it gets a throwaway one, replaced below.
        $installPw = [guid]::NewGuid().ToString('N')
        winget install --id PostgreSQL.PostgreSQL.18 --exact --accept-package-agreements --accept-source-agreements `
            --override "--mode unattended --unattendedmodeui none --superpassword $installPw --serverport $Port --disable-components pgAdmin,stackbuilder"
        if ($LASTEXITCODE -ne 0) { throw 'install.ps1: PostgreSQL install failed' }
        . "$PSScriptRoot\env.ps1"
        $env:PGPASSWORD = $installPw
        "SELECT format('ALTER ROLE postgres PASSWORD %L', :'pw') \gexec" |
            psql -X -q -v ON_ERROR_STOP=1 -v "pw=$superPlain" -h 127.0.0.1 -p $Port -U postgres -d postgres
        $failed = $LASTEXITCODE
        Remove-Item Env:PGPASSWORD
        if ($failed -ne 0) { throw 'install.ps1: setting the postgres password failed' }
    }
} else {
    $pgHome = Join-Path $env:LOCALAPPDATA 'ttacommunity'
    $pgData = Join-Path $pgHome 'pgdata'
    if (-not (Test-Path (Join-Path $pgHome 'pgsql\bin\pg_ctl.exe'))) {
        # The same release winget's installer carries; EDB publishes no checksum, so ours is pinned.
        $zip = Join-Path $env:TEMP 'postgresql-18.6-5-windows-x64-binaries.zip'
        $ProgressPreference = 'SilentlyContinue'   # 5.1's progress bar slows a download many times over
        Invoke-WebRequest -UseBasicParsing -OutFile $zip -Uri 'https://get.enterprisedb.com/postgresql/postgresql-18.6-5-windows-x64-binaries.zip'
        if ((Get-FileHash $zip -Algorithm SHA256).Hash -ne 'E2246BA91D22345BC3D017586C09EDE52D9DF180B1EEB480F050445F1CAD84E2') {
            Remove-Item $zip
            throw 'install.ps1: the PostgreSQL download does not match its pinned SHA-256'
        }
        New-Item -ItemType Directory -Force $pgHome | Out-Null
        tar -xf $zip -C $pgHome --exclude 'pgsql/pgAdmin 4' --exclude 'pgsql/StackBuilder' --exclude 'pgsql/doc'
        $unpacked = $LASTEXITCODE
        Remove-Item $zip
        if ($unpacked -ne 0) { throw 'install.ps1: unpacking PostgreSQL failed' }
    }
    if (-not (Test-Path (Join-Path $pgData 'PG_VERSION'))) {
        $pwFile = Join-Path $env:TEMP ([guid]::NewGuid().ToString('N'))
        try {
            Set-Content -Path $pwFile -Value $superPlain -NoNewline
            & (Join-Path $pgHome 'pgsql\bin\initdb.exe') -D $pgData -U postgres --pwfile=$pwFile -E UTF8 --locale=C --auth=scram-sha-256
            if ($LASTEXITCODE -ne 0) { throw 'install.ps1: initdb failed' }
        } finally {
            Remove-Item $pwFile -ErrorAction SilentlyContinue
        }
        Add-Content (Join-Path $pgData 'postgresql.conf') "`nlisten_addresses = '127.0.0.1'`nport = $Port"
    }
}
. "$PSScriptRoot\env.ps1"

# The roles the containers create as their superusers, and superusers here
# too: the tests and migrations SET ROLE to roles they create, which a
# CREATEROLE role may not do since PostgreSQL 16.
$env:PGPASSWORD = $superPlain
$sql = @"
SELECT format('CREATE ROLE %I LOGIN', r) FROM unnest(ARRAY['ratings','app','ttac']) r
  WHERE NOT EXISTS (SELECT FROM pg_roles WHERE rolname = r) \gexec
ALTER ROLE ratings SUPERUSER;
ALTER ROLE app SUPERUSER;
ALTER ROLE ttac SUPERUSER;
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
