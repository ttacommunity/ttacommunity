# Back up what exists in one place only: the Secrets in the ttacommunity namespace and the Cloudflare settings
# that are not in git. Each is exported as JSON, encrypted with age to the backup key and uploaded to the R2
# bucket ttacommunity-config-backup, only when its SHA-256 differs from the newest copy's. Nothing unencrypted
# touches the disk. The Windows twin of scripts/backup-config.sh. Usage: scripts\backup-config.ps1 [-Check]
param([switch]$Check)
$ErrorActionPreference = 'Stop'
Set-Location (Split-Path -Parent $PSScriptRoot)

$ns = 'ttacommunity'
$zone = if ($env:BACKUP_CONFIG_ZONE) { $env:BACKUP_CONFIG_ZONE } else { 'ttacommunity.org' }
$bucket = if ($env:BACKUP_CONFIG_BUCKET) { $env:BACKUP_CONFIG_BUCKET } else { 'ttacommunity-config-backup' }
$recipient = if ($env:BACKUP_CONFIG_AGE_RECIPIENT) { $env:BACKUP_CONFIG_AGE_RECIPIENT } else { 'age1chparhxyn3kvlr245t8u9zvm9agar9v995dyptuygepuvmeujazqkre4r9' }
$jurisdiction = 'eu'
$rcloneFlags = @('--s3-no-check-bucket', '--s3-no-head', '--retries', '3')

function Fail($message) { [Console]::Error.WriteLine("backup-config.ps1: $message"); exit 1 }
foreach ($tool in 'age', 'kubectl', 'cf', 'rclone', 'jq') {
    if (-not (Get-Command $tool -ErrorAction SilentlyContinue)) { Fail "$tool is not installed" }
}
function Get-SecretKey($key) {
    $b64 = kubectl -n $ns get secret backup-config-r2 -o "jsonpath={.data.$key}"
    if ($LASTEXITCODE -ne 0 -or -not $b64) { Fail 'cannot read the Secret backup-config-r2' }
    [Text.Encoding]::UTF8.GetString([Convert]::FromBase64String($b64))
}
# The remote comes from the environment alone, never from a maintainer's rclone.conf.
$env:RCLONE_CONFIG = if ($env:OS -eq 'Windows_NT') { 'NUL' } else { '/dev/null' }
$env:RCLONE_CONFIG_R2_TYPE = 's3'
$env:RCLONE_CONFIG_R2_PROVIDER = 'Cloudflare'
$env:RCLONE_CONFIG_R2_ENDPOINT = Get-SecretKey 'endpoint'
$env:RCLONE_CONFIG_R2_ACCESS_KEY_ID = Get-SecretKey 'access-key-id'
$env:RCLONE_CONFIG_R2_SECRET_ACCESS_KEY = Get-SecretKey 'secret-access-key'
cf auth whoami 2>$null | Out-Null
if ($LASTEXITCODE -ne 0) { Fail 'cf is not signed in' }

# Invoke-Bytes runs a program with bytes on its stdin and returns its stdout as bytes, so JSON, its hash and
# the encrypted upload are the same bytes the bash twin handles (PowerShell pipelines re-encode text).
function Invoke-Bytes([string]$File, [string[]]$Arguments, [byte[]]$InputBytes) {
    $psi = [Diagnostics.ProcessStartInfo]::new()
    $psi.FileName = (Get-Command $File).Source
    foreach ($a in $Arguments) { $psi.ArgumentList.Add($a) }
    $psi.UseShellExecute = $false
    $psi.RedirectStandardInput = $true
    $psi.RedirectStandardOutput = $true
    $p = [Diagnostics.Process]::Start($psi)
    $out = [IO.MemoryStream]::new()
    $copy = $p.StandardOutput.BaseStream.CopyToAsync($out)
    $p.StandardInput.BaseStream.Write($InputBytes, 0, $InputBytes.Length)
    $p.StandardInput.Close()
    $copy.Wait()
    $p.WaitForExit()
    if ($p.ExitCode -ne 0) { Fail "$File exited $($p.ExitCode)" }
    , $out.ToArray()
}
function ConvertTo-Bytes([string]$text) { , [Text.Encoding]::UTF8.GetBytes($text) }

# Read-Cf runs a cf read, retrying the API's intermittent "Envelope Error", and fails when it never answers JSON.
function Read-Cf([string[]]$Arguments) {
    for ($try = 1; $try -le 3; $try++) {
        $out = (& cf @Arguments 2>$null) -join "`n"
        if ($LASTEXITCODE -eq 0 -and $out) {
            try { $null = $out | ConvertFrom-Json -AsHashtable; return $out } catch { }
        }
        if ($try -lt 3) { Start-Sleep -Seconds 2 }
    }
    Fail "cf $Arguments gave no JSON after 3 tries"
}
function Invoke-Jq([string]$json, [string[]]$Arguments) { (($json | jq @Arguments) -join "`n") }

# Export-Secrets returns every Secret in the namespace as a kubectl-applicable List: name, type and data only.
function Export-Secrets {
    $raw = (kubectl -n $ns get secrets -o json) -join "`n"
    if ($LASTEXITCODE -ne 0) { Fail 'cannot list the Secrets' }
    Invoke-Bytes jq @('-S', '{apiVersion: "v1", kind: "List", items: [.items[] |
      {apiVersion, kind, type, metadata: {name: .metadata.name, namespace: .metadata.namespace}, data: (.data // {})}]
      | sort_by(.metadata.name)}') (ConvertTo-Bytes $raw)
}

# Export-Cloudflare returns one JSON object of every Cloudflare setting cf can read; docs/architecture.md names
# the rest. Lists ask for 50 a page: the account has a handful of each. A read that fails aborts the export.
function Export-Cloudflare {
    $script:out = Invoke-Jq '{}' @('-n', '--arg', 'z', $zone, '{zone: $z, rulesets: {}, build_variables: {}, r2_buckets: {}}')
    # Add stores a cf read at a dotted path of the export.
    function Add([string]$path, [string[]]$cfArgs) {
        $v = Read-Cf $cfArgs
        $script:out = Invoke-Jq $script:out @('--arg', 'p', $path, '--argjson', 'v', $v, 'setpath($p | split("."); $v)')
    }
    Add 'organization' @('zero-trust', 'organizations', 'list')
    Add 'access_applications' @('zero-trust', 'access', 'applications', 'list', '--per-page', '50')
    Add 'access_policies' @('zero-trust', 'access', 'policies', 'list', '--per-page', '50')
    Add 'access_groups' @('zero-trust', 'access', 'groups', 'list', '--per-page', '50')
    Add 'access_service_tokens' @('zero-trust', 'access', 'service-tokens', 'list', '--per-page', '50')
    Add 'identity_providers' @('zero-trust', 'identity-providers', 'list', '--per-page', '50')
    Add 'dns_records' @('dns', 'records', 'list', '-z', $zone, '--per-page', '50')
    Add 'turnstile_widgets' @('turnstile', 'widgets', 'list', '--per-page', '50')
    Add 'email_routing.settings' @('email-routing', 'settings', 'get', '-z', $zone)
    Add 'email_routing.rules' @('email-routing', 'rules', 'list-account', '-z', $zone, '--per-page', '50')
    Add 'email_routing.catch_all' @('email-routing', 'rules', 'catch-all', 'get', '-z', $zone)
    Add 'email_routing.addresses' @('email-routing', 'addresses', 'list', '--per-page', '50')
    $rulesets = Read-Cf @('rulesets', 'account-rulesets', 'list', '-z', $zone)
    foreach ($id in (Invoke-Jq $rulesets @('-r', '.[] | select(.kind == "zone") | .id')) -split "`n" | Where-Object { $_ }) {
        Add "rulesets.$id" @('rulesets', 'account-rulesets', 'get', $id, '-z', $zone)
    }
    Add 'workers' @('workers', 'scripts', 'search')
    $triggers = @()
    foreach ($line in (Invoke-Jq $script:out @('-r', '.workers[] | .id + " " + .script_name')) -split "`n" | Where-Object { $_ }) {
        $worker, $tag = $line -split ' ', 2
        Add "build_triggers.$tag" @('builds', 'triggers', 'list', '--external-script-id', $worker)
        # A Worker deployed by hand has no build configuration (404).
        if ((Invoke-Jq $script:out @('-r', '--arg', 't', $tag, '.build_triggers[$t] | length')) -ne '0') {
            Add "builds.$tag" @('builds', 'workers', 'get', $worker)
            $triggers += (Invoke-Jq $script:out @('-r', '--arg', 't', $tag, '.build_triggers[$t][].trigger_uuid')) -split "`n" | Where-Object { $_ }
        }
    }
    foreach ($trigger in $triggers) {
        Add "build_variables.$trigger" @('builds', 'triggers', 'environment-variables', 'list', '--trigger-uuid', $trigger)
    }
    $buckets = Read-Cf @('r2', 'buckets', 'list', '--cf-r2-jurisdiction', $jurisdiction)
    foreach ($name in (Invoke-Jq $buckets @('-r', '.buckets[].name')) -split "`n" | Where-Object { $_ }) {
        Add "r2_buckets.$name.lifecycle" @('r2', 'buckets', 'lifecycle', 'get', $name, '--cf-r2-jurisdiction', $jurisdiction)
        Add "r2_buckets.$name.locks" @('r2', 'buckets', 'locks', 'get', $name, '--cf-r2-jurisdiction', $jurisdiction)
    }
    Invoke-Bytes jq @('-S', '.') (ConvertTo-Bytes $script:out)
}

# Get-NewestSha returns the SHA-256 recorded on the newest copy under a prefix, or nothing when there is none.
function Get-NewestSha([string]$prefix) {
    $listing = (rclone lsjson "r2:$bucket/$prefix" -M @rcloneFlags 2>$null) -join "`n"
    if ($LASTEXITCODE -ne 0 -or -not $listing) { return $null }
    $files = ($listing | ConvertFrom-Json) | Where-Object { -not $_.IsDir } | Sort-Object Path
    if ($files) { ($files | Select-Object -Last 1).Metadata.sha256 }
}

# Publish-IfChanged exports one kind (secrets or cloudflare) with the given function, and uploads it encrypted
# when its fingerprint differs from the newest copy's. The export lives in memory only.
function Publish-IfChanged([string]$prefix, [scriptblock]$export) {
    [byte[]]$json = & $export
    # Without jq's final newline, as bash's $(...) hands it to the twin: the same bytes, the same fingerprint.
    if ($json[-1] -eq 10) { $json = $json[0..($json.Length - 2)] }
    $sha = ([BitConverter]::ToString([Security.Cryptography.SHA256]::HashData($json)) -replace '-', '').ToLower()
    $newest = Get-NewestSha $prefix
    if ($sha -eq $newest) { Write-Host "${prefix}: unchanged ($sha)"; return }
    $key = "$prefix/$([DateTime]::UtcNow.ToString('yyyy-MM-ddTHHmmssZ')).json.age"
    if ($Check) { Write-Host "${prefix}: would upload $key ($sha; newest copy $(if ($newest) { $newest } else { 'none' }))"; return }
    $encrypted = Invoke-Bytes age @('-r', $recipient) $json
    $null = Invoke-Bytes rclone (@('rcat', "r2:$bucket/$key", '-M', '--metadata-set', "sha256=$sha") + $rcloneFlags) $encrypted
    Write-Host "${prefix}: uploaded $key ($sha)"
}

Publish-IfChanged 'secrets' ${function:Export-Secrets}
Publish-IfChanged 'cloudflare' ${function:Export-Cloudflare}
