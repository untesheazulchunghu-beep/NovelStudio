[CmdletBinding()]
param(
    [string]$PackageDirectory = ""
)

$ErrorActionPreference = "Stop"
if ([string]::IsNullOrWhiteSpace($PackageDirectory)) {
    if (Test-Path (Join-Path $PSScriptRoot "novelstudio.exe")) {
        $PackageDirectory = $PSScriptRoot
    }
    else {
        $PackageDirectory = Join-Path $PSScriptRoot "..\release"
    }
}
$packageRoot = [System.IO.Path]::GetFullPath($PackageDirectory)
$server = Join-Path $packageRoot "novelstudio.exe"
if (-not (Test-Path $server -PathType Leaf)) {
    throw "Local package not found at $packageRoot. Run scripts/build-local.ps1 first."
}

Push-Location $packageRoot
try {
    & $server
    if ($LASTEXITCODE -ne 0) {
        throw "Novel Studio exited with code $LASTEXITCODE."
    }
}
finally {
    Pop-Location
}
