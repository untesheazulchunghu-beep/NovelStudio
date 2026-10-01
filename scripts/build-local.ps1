[CmdletBinding()]
param(
    [string]$OutputDirectory = ""
)

$ErrorActionPreference = "Stop"
$projectRoot = [System.IO.Path]::GetFullPath((Join-Path $PSScriptRoot ".."))
if ([string]::IsNullOrWhiteSpace($OutputDirectory)) {
    $OutputDirectory = Join-Path $projectRoot "release"
}
elseif (-not [System.IO.Path]::IsPathRooted($OutputDirectory)) {
    $OutputDirectory = Join-Path $projectRoot $OutputDirectory
}
$outputRoot = [System.IO.Path]::GetFullPath($OutputDirectory)

if (-not (Get-Command npm -ErrorAction SilentlyContinue)) {
    throw "Node.js and npm are required to build the local package."
}
if (-not (Get-Command go -ErrorAction SilentlyContinue)) {
    throw "Go is required to build the local package."
}

$frontendRoot = Join-Path $projectRoot "frontend"
Push-Location $frontendRoot
try {
    if (-not (Test-Path (Join-Path $frontendRoot "node_modules"))) {
        npm ci
        if ($LASTEXITCODE -ne 0) {
            throw "npm ci failed with exit code $LASTEXITCODE."
        }
    }
    npm run build
    if ($LASTEXITCODE -ne 0) {
        throw "Frontend build failed with exit code $LASTEXITCODE."
    }
}
finally {
    Pop-Location
}

New-Item -ItemType Directory -Force -Path $outputRoot | Out-Null
$webRoot = Join-Path $outputRoot "frontend\dist"
New-Item -ItemType Directory -Force -Path $webRoot | Out-Null
New-Item -ItemType Directory -Force -Path (Join-Path $outputRoot "data") | Out-Null
Copy-Item -Path (Join-Path $frontendRoot "dist\*") -Destination $webRoot -Recurse -Force

$backendRoot = Join-Path $projectRoot "backend"
Push-Location $backendRoot
try {
    go build -o (Join-Path $outputRoot "novelstudio.exe") ./cmd/server
    if ($LASTEXITCODE -ne 0) {
        throw "Backend build failed with exit code $LASTEXITCODE."
    }
    go build -o (Join-Path $outputRoot "novelstudio-backup.exe") ./cmd/backup
    if ($LASTEXITCODE -ne 0) {
        throw "Backup utility build failed with exit code $LASTEXITCODE."
    }
}
finally {
    Pop-Location
}

$exampleEnv = Join-Path $backendRoot ".env.example"
$localEnv = Join-Path $outputRoot ".env"
if (-not (Test-Path $localEnv)) {
    $randomBytes = New-Object byte[] 32
    $random = [System.Security.Cryptography.RandomNumberGenerator]::Create()
    try {
        $random.GetBytes($randomBytes)
    }
    finally {
        $random.Dispose()
    }
    $secret = [BitConverter]::ToString($randomBytes).Replace("-", "").ToLowerInvariant()
    $contents = [System.IO.File]::ReadAllText($exampleEnv)
    $contents = [regex]::Replace($contents, "(?m)^SESSION_SECRET=.*$", "SESSION_SECRET=$secret")
    Set-Content -Path $localEnv -Value $contents -Encoding UTF8
}

Copy-Item -Path (Join-Path $PSScriptRoot "run-local.ps1") -Destination (Join-Path $outputRoot "Start-NovelStudio.ps1") -Force
Write-Host "Local package built at: $outputRoot"
Write-Host "Edit $localEnv if you need to configure a model provider."
Write-Host "Run Start-NovelStudio.ps1 from the package to start the app."
