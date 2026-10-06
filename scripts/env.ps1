# Load .env into this PowerShell session, creating it from .env.example first.
# On Windows both databases live in one Postgres service, so the app database's
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

# Postgres's tools (psql) come with the Windows service; put them on PATH.
if ($onWindows) {
    $pgBin = 'C:\Program Files\PostgreSQL\18\bin'
    if ((Test-Path $pgBin) -and ($env:PATH -notlike "*$pgBin*")) { $env:PATH = "$pgBin;$env:PATH" }
}
# mise's shims put the pinned Go, Node and uv on PATH. Without these, a shim's
# first use installs every tool in mise.toml, CI's linters included.
$env:MISE_AUTO_INSTALL = 'false'
$env:MISE_EXEC_AUTO_INSTALL = 'false'
if ($env:LOCALAPPDATA) {
    $shims = Join-Path $env:LOCALAPPDATA 'mise\shims'
    if ((Test-Path $shims) -and ($env:PATH -notlike "*$shims*")) { $env:PATH = "$shims;$env:PATH" }
}
