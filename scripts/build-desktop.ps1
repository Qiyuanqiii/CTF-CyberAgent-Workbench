[CmdletBinding()]
param(
    [string]$OutputDirectory = "build/desktop"
)

$ErrorActionPreference = "Stop"
$repositoryRoot = Split-Path -Parent $PSScriptRoot
$outputRoot = Join-Path $repositoryRoot $OutputDirectory
$binaryPath = Join-Path $outputRoot "cyberagent-desktop.exe"

if ([System.Environment]::OSVersion.Platform -ne [System.PlatformID]::Win32NT) {
    throw "Desktop D0-A currently builds only on Windows"
}

Push-Location (Join-Path $repositoryRoot "web")
try {
    & npm ci
    if ($LASTEXITCODE -ne 0) { throw "npm ci failed" }
    & npm run check:api
    if ($LASTEXITCODE -ne 0) { throw "frontend API contract check failed" }
    & npm test
    if ($LASTEXITCODE -ne 0) { throw "frontend tests failed" }
    & npm run build
    if ($LASTEXITCODE -ne 0) { throw "frontend production build failed" }
}
finally {
    Pop-Location
}

Push-Location $repositoryRoot
try {
    & go test ./internal/desktop ./internal/webui -count=1
    if ($LASTEXITCODE -ne 0) { throw "Desktop Go boundary tests failed" }
    & go test -tags "desktop,wv2runtime.error" ./cmd/cyberagent-desktop -count=1
    if ($LASTEXITCODE -ne 0) { throw "Wails adapter tests failed" }
    New-Item -ItemType Directory -Force -Path $outputRoot | Out-Null
    & go build -tags "desktop,production,wv2runtime.error" -trimpath -ldflags "-s -w -H=windowsgui" -o $binaryPath ./cmd/cyberagent-desktop
    if ($LASTEXITCODE -ne 0) { throw "Desktop production build failed" }
}
finally {
    Pop-Location
}

$hash = Get-FileHash -Algorithm SHA256 -LiteralPath $binaryPath
Write-Output "desktop_binary: $binaryPath"
Write-Output "desktop_sha256: $($hash.Hash.ToLowerInvariant())"
Write-Output "desktop_profile_control_default: false"
