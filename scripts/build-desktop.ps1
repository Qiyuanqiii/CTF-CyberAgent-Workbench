[CmdletBinding()]
param(
    [string]$OutputDirectory = "build/desktop",
    [string]$Version = "v0.1.0",
    [switch]$SkipFrontend,
    [switch]$VerifyReproducible
)

$ErrorActionPreference = "Stop"
$repositoryRoot = Split-Path -Parent $PSScriptRoot
$outputRoot = [System.IO.Path]::GetFullPath((Join-Path $repositoryRoot $OutputDirectory))
$repositoryFull = [System.IO.Path]::GetFullPath($repositoryRoot)
$binaryPath = Join-Path $outputRoot "cyberagent-desktop.exe"
$reproBinaryPath = Join-Path $outputRoot "cyberagent-desktop.repro.exe"
$metadataPath = Join-Path $outputRoot "release-metadata.json"

if ([System.Environment]::OSVersion.Platform -ne [System.PlatformID]::Win32NT) {
    throw "Desktop portable build currently supports only Windows"
}
if (-not $outputRoot.StartsWith($repositoryFull + [System.IO.Path]::DirectorySeparatorChar,
        [System.StringComparison]::OrdinalIgnoreCase)) {
    throw "Desktop output directory must remain inside the repository"
}
if ($Version -notmatch '^v[0-9]+\.[0-9]+\.[0-9]+(?:[-+][0-9A-Za-z.-]+)?$') {
    throw "Desktop release version is invalid"
}

function Invoke-Checked {
    param([scriptblock]$Command, [string]$Failure)
    & $Command
    if ($LASTEXITCODE -ne 0) { throw $Failure }
}

function Assert-NoOutputReparsePoint {
    param([string]$Root, [string]$Candidate)
    $separators = [char[]]@([System.IO.Path]::DirectorySeparatorChar,
        [System.IO.Path]::AltDirectorySeparatorChar)
    $rootPrefix = $Root.TrimEnd($separators) + [System.IO.Path]::DirectorySeparatorChar
    if (-not $Candidate.StartsWith($rootPrefix,
            [System.StringComparison]::OrdinalIgnoreCase)) {
        throw "Desktop output directory must remain inside the repository"
    }
    $relative = $Candidate.Substring($rootPrefix.Length)
    $current = $Root
    foreach ($component in [regex]::Split($relative, '[\\/]')) {
        if ([string]::IsNullOrWhiteSpace($component)) { continue }
        $current = Join-Path $current $component
        if (-not (Test-Path -LiteralPath $current)) { break }
        $item = Get-Item -Force -LiteralPath $current
        if (($item.Attributes -band [System.IO.FileAttributes]::ReparsePoint) -ne 0) {
            throw "Desktop output directory cannot traverse a reparse point"
        }
    }
}

Assert-NoOutputReparsePoint -Root $repositoryFull -Candidate $outputRoot

if (-not $SkipFrontend) {
    Push-Location (Join-Path $repositoryRoot "web")
    try {
        Invoke-Checked { npm ci } "npm ci failed"
        Invoke-Checked { npm run check:api } "frontend API contract check failed"
        Invoke-Checked { npm test } "frontend tests failed"
        Invoke-Checked { npm run build } "frontend production build failed"
    }
    finally {
        Pop-Location
    }
}

Push-Location $repositoryRoot
try {
    Invoke-Checked { go test ./internal/desktop ./internal/webui -count=1 } "Desktop Go boundary tests failed"
    Invoke-Checked { go test -tags "desktop,wv2runtime.error" ./cmd/cyberagent-desktop -count=1 } "Wails adapter tests failed"

    $revision = (& git rev-parse HEAD).Trim()
    if ($LASTEXITCODE -ne 0 -or $revision -notmatch '^[0-9a-f]{40}$') {
        throw "Git release revision is unavailable"
    }
    $sourceDateEpoch = (& git show -s --format=%ct HEAD).Trim()
    if ($LASTEXITCODE -ne 0 -or $sourceDateEpoch -notmatch '^[1-9][0-9]*$') {
        throw "Git source date epoch is unavailable"
    }
    $dirtyOutput = @(& git status --porcelain)
    if ($LASTEXITCODE -ne 0) { throw "Git worktree state is unavailable" }
    $modified = if ($dirtyOutput.Count -gt 0) { "true" } else { "false" }
    $cgoEnabled = (& go env CGO_ENABLED).Trim()
    if ($LASTEXITCODE -ne 0 -or $cgoEnabled -notmatch '^[01]$') {
        throw "Go CGO build metadata is invalid"
    }
    $goVersion = (& go env GOVERSION).Trim()
    if ($LASTEXITCODE -ne 0 -or $goVersion -notmatch '^go[0-9]+\.[0-9]+') {
        throw "Go version build metadata is invalid"
    }
    $targetOS = (& go env GOOS).Trim()
    if ($LASTEXITCODE -ne 0 -or $targetOS -ne "windows") {
        throw "Desktop portable build requires GOOS=windows"
    }
    $targetArch = (& go env GOARCH).Trim()
    if ($LASTEXITCODE -ne 0 -or $targetArch -notin @("amd64", "arm64")) {
        throw "Go Windows build metadata is invalid"
    }

    New-Item -ItemType Directory -Force -Path $outputRoot | Out-Null
    $ldflags = @(
        "-s", "-w", "-H=windowsgui",
        "-X=cyberagent-workbench/internal/buildinfo.Version=$Version",
        "-X=cyberagent-workbench/internal/buildinfo.Revision=$revision",
        "-X=cyberagent-workbench/internal/buildinfo.SourceDateEpoch=$sourceDateEpoch",
        "-X=cyberagent-workbench/internal/buildinfo.Modified=$modified",
        "-X=cyberagent-workbench/internal/buildinfo.CGOEnabled=$cgoEnabled"
    ) -join " "
    $previousSourceDateEpoch = $env:SOURCE_DATE_EPOCH
    $env:SOURCE_DATE_EPOCH = $sourceDateEpoch
    try {
        Invoke-Checked {
            go build -tags "desktop,production,wv2runtime.error" -trimpath -ldflags $ldflags `
                -o $binaryPath ./cmd/cyberagent-desktop
        } "Desktop production build failed"
        $reproducible = $false
        if ($VerifyReproducible) {
            Invoke-Checked {
                go build -tags "desktop,production,wv2runtime.error" -trimpath -ldflags $ldflags `
                    -o $reproBinaryPath ./cmd/cyberagent-desktop
            } "Desktop reproducibility build failed"
            $firstHash = (Get-FileHash -Algorithm SHA256 -LiteralPath $binaryPath).Hash
            $secondHash = (Get-FileHash -Algorithm SHA256 -LiteralPath $reproBinaryPath).Hash
            if ($firstHash -ne $secondHash) {
                throw "Desktop reproducibility check failed: consecutive binary hashes differ"
            }
            $reproducible = $true
        }
    }
    finally {
        $env:SOURCE_DATE_EPOCH = $previousSourceDateEpoch
        if (Test-Path -LiteralPath $reproBinaryPath -PathType Leaf) {
            Remove-Item -LiteralPath $reproBinaryPath -Force
        }
    }

    $hash = (Get-FileHash -Algorithm SHA256 -LiteralPath $binaryPath).Hash.ToLowerInvariant()
    $metadata = [ordered]@{
        protocol_version = "portable_release_metadata.v1"
        app_version = $Version
        revision = $revision
        source_date_epoch = [int64]$sourceDateEpoch
        modified = [System.Convert]::ToBoolean($modified)
        go_version = $goVersion
        target_os = $targetOS
        target_arch = $targetArch
        cgo_enabled = $cgoEnabled
        trimpath = $true
        binary_name = "cyberagent-desktop.exe"
        sha256 = $hash
        reproducibility_checked = [bool]$VerifyReproducible
        reproducible = $reproducible
        installer_included = $false
        registry_writes = $false
        startup_task = $false
        auto_update_enabled = $false
        manual_windows_10_matrix_required = $true
    }
    $metadataJSON = ($metadata | ConvertTo-Json -Depth 4) + "`n"
    [System.IO.File]::WriteAllText($metadataPath, $metadataJSON,
        [System.Text.UTF8Encoding]::new($false))

    & (Join-Path $PSScriptRoot "check-windows-compat.ps1") `
        -BinaryPath $binaryPath -MetadataPath $metadataPath
    if ($LASTEXITCODE -ne 0) { throw "Windows portable compatibility checklist failed" }
}
finally {
    Pop-Location
}

Write-Output "desktop_binary: $binaryPath"
Write-Output "desktop_sha256: $hash"
Write-Output "release_metadata: $metadataPath"
Write-Output "desktop_reproducible: $reproducible"
Write-Output "desktop_installer_included: false"
Write-Output "desktop_registry_writes: false"
Write-Output "desktop_profile_control_default: false"
