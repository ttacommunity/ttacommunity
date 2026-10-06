# Seed the local ratings database from data kept outside the repo: a baseline
# export (a folder with manifest.json, restored whole) or a folder of match CSVs.
# Usage: scripts\ratings-db.ps1 [-Reset] <folder>
param(
    [Parameter(Mandatory)][string]$Dir,
    [switch]$Reset
)
$ErrorActionPreference = 'Stop'
. "$PSScriptRoot\env.ps1"

if (-not (Test-Path $Dir -PathType Container)) { throw "ratings-db.ps1: no folder $Dir" }
$Dir = (Resolve-Path $Dir).Path
$restore = Test-Path (Join-Path $Dir 'manifest.json')
if ($restore -and $Reset) {
    throw 'ratings-db.ps1: a baseline is restored into an empty database; to start again, drop and recreate the ratings database'
}

Push-Location (Join-Path (Split-Path -Parent $PSScriptRoot) 'backend')
try {
    if ($restore) { go run ./cmd/load -restore $Dir }
    elseif ($Reset) { go run ./cmd/load -reset $Dir }
    else { go run ./cmd/load $Dir }
    if ($LASTEXITCODE -ne 0) { throw 'ratings-db.ps1: the load failed' }
} finally {
    Pop-Location
}
