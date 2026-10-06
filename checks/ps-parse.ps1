# Fail when a PowerShell script in scripts/ or checks/ does not parse.
# Linux CI has no Windows, so this is the cheap net under the .ps1 twins.
# Usage: pwsh -NoProfile -File checks/ps-parse.ps1
$root = Split-Path -Parent $PSScriptRoot
$files = Get-ChildItem (Join-Path $root 'scripts'), (Join-Path $root 'checks') -Filter *.ps1
$bad = 0
foreach ($f in $files) {
    $errors = $null
    [void][System.Management.Automation.Language.Parser]::ParseFile($f.FullName, [ref]$null, [ref]$errors)
    foreach ($e in $errors) {
        Write-Host "  $($f.Name):$($e.Extent.StartLineNumber): $($e.Message)"
        $bad++
    }
}
if ($files.Count -eq 0) { Write-Host 'ps-parse: found no .ps1 files, which is itself suspicious'; exit 1 }
if ($bad -gt 0) { Write-Host "ps-parse: $bad error(s)"; exit 1 }
Write-Host "  ps-parse: $($files.Count) file(s) parse"
