# Show a local site build through one Cloudflare quick tunnel, where a visitor signs in as an invented Discord
# player (README, "Let someone sign in as a test player"). scripts\dev.ps1 may keep running. Ctrl-C stops it all.
# Usage: scripts\share-player.ps1
$ErrorActionPreference = 'Stop'
. "$PSScriptRoot\env.ps1"
$root = Split-Path -Parent $PSScriptRoot
$apiPort, $sitePort, $proxyPort = 8090, 4323, 4325
$work = if ($env:LOCALAPPDATA) { Join-Path $env:LOCALAPPDATA 'ttacommunity\share-player' } else { Join-Path $HOME '.cache/ttacommunity/share-player' }
$exe = if ($onWindows) { '.exe' } else { '' }

# Wait-Status polls $Url for up to $Seconds until it answers with HTTP status $Want, and says what it got.
function Wait-Status($Url, $Want, $Seconds = 60) {
    $got = 0
    $end = (Get-Date).AddSeconds($Seconds)
    while ((Get-Date) -lt $end) {
        try { $got = [int](Invoke-WebRequest -Uri $Url -UseBasicParsing -TimeoutSec 5).StatusCode }
        catch { $got = if ($_.Exception.Response) { [int]$_.Exception.Response.StatusCode } else { 0 } }
        if ($got -eq $Want) { Write-Host "  ok      $Want  $Url"; return $true }
        Start-Sleep -Seconds 1
    }
    Write-Host "  FAILED  $Url answered $got, want $Want"
    return $false
}

# Use-Env sets the environment variables in $Vars, runs $Block, and puts back what was there before.
function Use-Env($Vars, [scriptblock]$Block) {
    $saved = @{}
    foreach ($k in $Vars.Keys) {
        $saved[$k] = [Environment]::GetEnvironmentVariable($k)
        [Environment]::SetEnvironmentVariable($k, $Vars[$k])
    }
    try { & $Block } finally { foreach ($k in $saved.Keys) { [Environment]::SetEnvironmentVariable($k, $saved[$k]) } }
}

if (-not (Get-Command cloudflared -ErrorAction SilentlyContinue)) {
    throw 'share-player.ps1: cloudflared is not installed (winget install --id Cloudflare.cloudflared)'
}
foreach ($port in $apiPort, $sitePort, $proxyPort) {
    $client = [Net.Sockets.TcpClient]::new()
    try { $busy = $client.ConnectAsync('127.0.0.1', $port).Wait(1000) } catch { $busy = $false } finally { $client.Dispose() }
    if ($busy) { throw "share-player.ps1: something listens on 127.0.0.1:$port; stop it first" }
}

New-Item -ItemType Directory -Force $work | Out-Null
Push-Location (Join-Path $root 'backend')
go build -o (Join-Path $work "api$exe") ./cmd/api
$built = $LASTEXITCODE
if ($built -eq 0) { go build -o (Join-Path $work "shareproxy$exe") ./cmd/shareproxy; $built = $LASTEXITCODE }
Pop-Location
if ($built -ne 0) { throw 'share-player.ps1: go build failed' }

$procs = @()
try {
    $log = Join-Path $work 'tunnel.log'
    $procs += Start-Process cloudflared -ArgumentList 'tunnel', '--no-autoupdate', '--url', "http://127.0.0.1:$proxyPort" -RedirectStandardError $log -NoNewWindow -PassThru
    $public = $null
    for ($i = 0; $i -lt 60 -and -not $public -and -not $procs[0].HasExited; $i++) {
        Start-Sleep -Seconds 1
        if ((Test-Path $log) -and ((Get-Content $log -Raw) -match 'https://[a-z0-9-]+\.trycloudflare\.com')) { $public = $Matches[0] }
    }
    if (-not $public) { throw "share-player.ps1: cloudflared gave no tunnel address; see $log" }

    # Only the Discord stand-in signs in; email's, if .env has it, prints its links here, so a visitor cannot finish one.
    $apiEnv = @{
        API_ADDR = "127.0.0.1:$apiPort"; METRICS_ADDR = ''
        SITE_ORIGIN = "http://127.0.0.1:$sitePort"; API_ORIGIN = "http://127.0.0.1:$apiPort"
        DISCORD_DEV_USER = 'local_player'; DISCORD_CLIENT_ID = ''; DISCORD_CLIENT_SECRET = ''
        ADMIN_DEV_USER = ''; MICROSOFT_DEV_USER = ''; GOOGLE_DEV_USER = ''
    }
    $procs += Use-Env $apiEnv { Start-Process (Join-Path $work "api$exe") -WorkingDirectory (Join-Path $root 'backend') -NoNewWindow -PassThru }
    if (-not (Wait-Status "http://127.0.0.1:$apiPort/healthz" 200)) { throw 'share-player.ps1: the api did not start' }

    $env:COREPACK_ENABLE_DOWNLOAD_PROMPT = '0'
    Push-Location (Join-Path $root 'web')
    corepack pnpm install --frozen-lockfile --silent
    $code = $LASTEXITCODE
    if ($code -eq 0) { Use-Env @{ PUBLIC_API_URL = $public } { corepack pnpm --filter '@ttac/site' build }; $code = $LASTEXITCODE }
    Pop-Location
    if ($code -ne 0) { throw 'share-player.ps1: the site build failed' }
    # node runs astro itself, so stopping the process stops the server; --ignore-lock keeps it in the foreground
    # when an AI agent runs this.
    $site = Join-Path $root 'web/site'
    $procs += Start-Process node -ArgumentList 'node_modules/astro/bin/astro.mjs', 'preview', '--ignore-lock', '--port', $sitePort, '--host', '127.0.0.1', '--allowed-hosts', ([uri]$public).Host -WorkingDirectory $site -NoNewWindow -PassThru
    $procs += Start-Process (Join-Path $work "shareproxy$exe") -ArgumentList '-public', $public, '-listen', "127.0.0.1:$proxyPort", '-api', "http://127.0.0.1:$apiPort", '-site', "http://127.0.0.1:$sitePort" -NoNewWindow -PassThru

    $checks = @(
        Wait-Status "http://127.0.0.1:$apiPort/v1/admin/me" 401
        # A new tunnel's name can take a minute or more to resolve.
        Wait-Status "$public/blitz/" 200 180
        Wait-Status "$public/v1/blitz" 200
        Wait-Status "$public/v1/admin/me" 403
    )
    if ($checks -contains $false) { throw 'share-player.ps1: not sharing: a check above failed' }

    Write-Host "share-player.ps1: share $public"
    Write-Host '  Anyone with it signs in as the invented Discord player local_player, on your local database.'
    Write-Host '  web\site\dist holds this build until another build replaces it. Ctrl-C stops the tunnel, api, site and proxy.'
    Wait-Process -Id ($procs | ForEach-Object Id)
} finally {
    foreach ($p in $procs) { if (-not $p.HasExited) { Stop-Process -Id $p.Id -Force -ErrorAction SilentlyContinue } }
}
