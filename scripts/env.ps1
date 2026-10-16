# Load .env into this PowerShell session, creating it from .env.example first.
# On Windows both databases live in one Postgres server, so the app database's
# port becomes the ratings one. Usage: . scripts\env.ps1   (dot-sourced)

$root = Split-Path -Parent $PSScriptRoot
$onWindows = $env:OS -eq 'Windows_NT'   # also true in Windows PowerShell 5.1, which has no $IsWindows
$envFile = Join-Path $root '.env'
if (-not (Test-Path $envFile)) {
    $example = Get-Content (Join-Path $root '.env.example') -Raw
    if ($onWindows) {
        $example = $example -replace '127\.0\.0\.1:55433/', '127.0.0.1:55432/'
    }
    Set-Content -Path $envFile -Value $example -NoNewline
    Write-Host 'env.ps1: created .env from .env.example (local-only values)'
}

foreach ($line in Get-Content $envFile) {
    if ($line -match '^\s*([A-Za-z_][A-Za-z0-9_]*)=(.*)$') {
        Set-Item -Path "Env:$($Matches[1])" -Value $Matches[2]
    }
}

# A user install of Postgres (install.ps1's default): its tools on PATH,
# PGDATA set so pg_ctl works directly, and started if it is not running.
$userPg = if ($env:LOCALAPPDATA) { Join-Path $env:LOCALAPPDATA 'ttacommunity' } else { $null }
if ($userPg -and (Test-Path (Join-Path $userPg 'pgdata\PG_VERSION'))) {
    $env:PGDATA = Join-Path $userPg 'pgdata'
    $userBin = Join-Path $userPg 'pgsql\bin'
    if ($env:PATH -notlike "*$userBin*") { $env:PATH = "$userBin;$env:PATH" }
    pg_ctl status | Out-Null
    if ($LASTEXITCODE -ne 0) {
        $pidFile = Join-Path $env:PGDATA 'postmaster.pid'
        if (Test-Path $pidFile) {
            $rawPid = (Get-Content $pidFile -TotalCount 1).Trim()
            if ($rawPid -match '^\d+$' -and -not (Get-Process -Id ([int]$rawPid) -ErrorAction SilentlyContinue)) {
                Remove-Item $pidFile -Force -ErrorAction SilentlyContinue
            }
        }
        # Not `pg_ctl start | Out-Null`: the server inherits the pipe, and the
        # pipe stays open until the server stops. Start it detached, then poll.
        $log = Join-Path $userPg 'postgres.log'
        $pgCtlExe = Join-Path $userBin 'pg_ctl.exe'
        $cmd = "`"$pgCtlExe`" start -D `"$env:PGDATA`" -l `"$log`""
        $started = $false
        try {
            $res = Invoke-CimMethod -ClassName Win32_Process -MethodName Create -Arguments @{ CommandLine = $cmd } -ErrorAction Stop
            if ($res.ReturnValue -eq 0) { $started = $true }
        } catch {}
        if (-not $started) {
            Start-Process -FilePath $pgCtlExe -WindowStyle Hidden -ArgumentList 'start', '-D', "`"$env:PGDATA`"", '-l', "`"$log`""
        }
        for ($i = 0; $i -lt 60; $i++) {
            Start-Sleep -Seconds 1
            pg_ctl status | Out-Null
            if ($LASTEXITCODE -eq 0) { break }
        }
        if ($LASTEXITCODE -ne 0) { throw "env.ps1: Postgres did not start within 60s; see $log" }
    }
}
# With -Service, Postgres's tools come with the Windows service; put them on PATH.
if ($onWindows) {
    $pgBin = 'C:\Program Files\PostgreSQL\18\bin'
    if ((Test-Path $pgBin) -and ($env:PATH -notlike "*$pgBin*")) { $env:PATH = "$pgBin;$env:PATH" }
}
# mise's shims put the pinned Go, Node and uv on PATH. Without these, a shim's
# first use installs every tool in mise.toml, CI's linters included.
$env:MISE_AUTO_INSTALL = 'false'
$env:MISE_EXEC_AUTO_INSTALL = 'false'
# A shell opened before install.ps1 lacks the PATH winget set for mise; read it as a new shell would.
if ($onWindows -and -not (Get-Command mise -ErrorAction SilentlyContinue)) {
    $env:PATH = [Environment]::GetEnvironmentVariable('PATH', 'Machine') + ';' +
        [Environment]::GetEnvironmentVariable('PATH', 'User') + ';' + $env:PATH
}
if ($env:LOCALAPPDATA) {
    $shims = Join-Path $env:LOCALAPPDATA 'mise\shims'
    if ((Test-Path $shims) -and ($env:PATH -notlike "*$shims*")) { $env:PATH = "$shims;$env:PATH" }
}
