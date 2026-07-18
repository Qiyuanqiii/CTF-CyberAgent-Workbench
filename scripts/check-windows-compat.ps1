[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)][string]$BinaryPath,
    [Parameter(Mandatory = $true)][string]$MetadataPath,
    [string]$OutputPath = ""
)

$ErrorActionPreference = "Stop"
if ([System.Environment]::OSVersion.Platform -ne [System.PlatformID]::Win32NT) {
    throw "Windows compatibility checks require Windows"
}
$binary = [System.IO.Path]::GetFullPath($BinaryPath)
$metadataFile = [System.IO.Path]::GetFullPath($MetadataPath)
if (-not (Test-Path -LiteralPath $binary -PathType Leaf) -or
    -not (Test-Path -LiteralPath $metadataFile -PathType Leaf)) {
    throw "Portable binary and release metadata are required"
}
if ([string]::IsNullOrWhiteSpace($OutputPath)) {
    $OutputPath = Join-Path (Split-Path -Parent $metadataFile) "windows-compatibility.json"
}
$output = [System.IO.Path]::GetFullPath($OutputPath)

$checks = [System.Collections.Generic.List[object]]::new()
function Add-Check {
    param([string]$ID, [string]$Status, [string]$Detail)
    $checks.Add([pscustomobject][ordered]@{ id = $ID; status = $Status; detail = $Detail })
}

$metadata = Get-Content -LiteralPath $metadataFile -Raw | ConvertFrom-Json
$sha256 = (Get-FileHash -Algorithm SHA256 -LiteralPath $binary).Hash.ToLowerInvariant()

$stream = [System.IO.File]::Open($binary, [System.IO.FileMode]::Open,
    [System.IO.FileAccess]::Read, [System.IO.FileShare]::Read)
$reader = [System.IO.BinaryReader]::new($stream)
try {
    $mz = $reader.ReadUInt16()
    $stream.Position = 0x3c
    $peOffset = $reader.ReadInt32()
    if ($peOffset -lt 64 -or $peOffset -gt ($stream.Length - 24)) {
        throw "PE header offset is invalid"
    }
    $stream.Position = $peOffset
    $peSignature = $reader.ReadUInt32()
    $machineCode = $reader.ReadUInt16()
    $sectionCount = $reader.ReadUInt16()
    $coffTimestamp = $reader.ReadUInt32()
    $stream.Position = $peOffset + 22
    $characteristics = $reader.ReadUInt16()
}
finally {
    $reader.Dispose()
    $stream.Dispose()
}

$machine = switch ($machineCode) {
    0x8664 { "amd64" }
    0xaa64 { "arm64" }
    default { "unknown" }
}
Add-Check "pe_signature" $(if ($mz -eq 0x5a4d -and $peSignature -eq 0x00004550) { "pass" } else { "fail" }) `
    "binary has DOS and PE signatures"
Add-Check "pe_machine" $(if ($machine -ne "unknown" -and $machine -eq $metadata.target_arch) { "pass" } else { "fail" }) `
    "PE machine matches release target"
Add-Check "pe_executable" $(if (($characteristics -band 0x0002) -ne 0 -and
    ($characteristics -band 0x2000) -eq 0 -and $sectionCount -gt 0) { "pass" } else { "fail" }) `
    "PE is an executable and not a DLL"
Add-Check "coff_timestamp_zero" $(if ($coffTimestamp -eq 0) { "pass" } else { "fail" }) `
    "PE COFF timestamp is zero for deterministic Go linking"
Add-Check "sha256_binding" $(if ($metadata.sha256 -eq $sha256) { "pass" } else { "fail" }) `
    "binary SHA-256 matches release metadata"
Add-Check "release_identity" $(if ($metadata.protocol_version -eq "portable_release_metadata.v1" -and
    $metadata.app_version -match '^v[0-9]+\.[0-9]+\.[0-9]+' -and
    $metadata.revision -match '^[0-9a-f]{40}$' -and
    [int64]$metadata.source_date_epoch -gt 0) { "pass" } else { "fail" }) `
    "version, revision, and source date are pinned"
Add-Check "go_target" $(if ($metadata.target_os -eq "windows" -and
    $metadata.target_arch -in @("amd64", "arm64") -and
    $metadata.cgo_enabled -match '^[01]$') { "pass" } else { "fail" }) `
    "Windows target and CGO mode are recorded"
Add-Check "non_installing_boundary" $(if (-not $metadata.installer_included -and
    -not $metadata.registry_writes -and -not $metadata.startup_task -and
    -not $metadata.auto_update_enabled) { "pass" } else { "fail" }) `
    "build has no installer, registry, startup-task, or auto-update authority"

$moduleOutput = @(& go version -m $binary 2>&1)
$moduleExit = $LASTEXITCODE
$moduleText = $moduleOutput -join "`n"
Add-Check "go_build_metadata" $(if ($moduleExit -eq 0 -and
    $moduleText.Contains("cyberagent-workbench") -and
    $moduleText.Contains("-trimpath=true") -and $metadata.trimpath) { "pass" } else { "fail" }) `
    "Go module identity and trimpath are embedded"

if ($metadata.reproducibility_checked) {
    Add-Check "consecutive_build_hash" $(if ($metadata.reproducible) { "pass" } else { "fail" }) `
        "two consecutive builds produced the same SHA-256"
}
else {
    Add-Check "consecutive_build_hash" "manual" "run build-desktop.ps1 -VerifyReproducible before release"
}
Add-Check "windows_10_webview2_matrix" "manual" `
    "verify Windows 10, WebView2, display scaling, launch, and recovery on a clean machine"

$failed = @($checks | Where-Object { $_.status -eq "fail" })
$manual = @($checks | Where-Object { $_.status -eq "manual" })
$result = [ordered]@{
    protocol_version = "windows_portable_compatibility.v1"
    binary_name = [System.IO.Path]::GetFileName($binary)
    sha256 = $sha256
    machine = $machine
    coff_timestamp = [uint32]$coffTimestamp
    checks = $checks
    automated_checks_passed = ($failed.Count -eq 0)
    release_ready = ($failed.Count -eq 0 -and $manual.Count -eq 0)
    manual_windows_10_matrix_required = $true
}
$json = ($result | ConvertTo-Json -Depth 6) + "`n"
[System.IO.File]::WriteAllText($output, $json, [System.Text.UTF8Encoding]::new($false))
Write-Output "windows_compatibility: $output"
Write-Output "windows_automated_checks_passed: $($result.automated_checks_passed.ToString().ToLowerInvariant())"
Write-Output "windows_release_ready: $($result.release_ready.ToString().ToLowerInvariant())"
if ($failed.Count -ne 0) {
    throw "Windows portable compatibility checks failed: $($failed.id -join ', ')"
}
