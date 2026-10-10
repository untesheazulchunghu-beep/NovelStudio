$ErrorActionPreference = 'Stop'

$root = $PSScriptRoot
$releaseRoot = Join-Path $root 'release'
$packageDirectory = Join-Path $releaseRoot 'NovelStudio'
$archivePath = Join-Path $releaseRoot 'NovelStudio-portable.zip'

if ((Test-Path -LiteralPath $packageDirectory) -or (Test-Path -LiteralPath $archivePath)) {
  throw "Release output already exists. Move or remove it before packaging: $releaseRoot"
}

& (Join-Path $root 'build-web.bat')
if ($LASTEXITCODE -ne 0) {
  throw "build-web.bat failed with exit code $LASTEXITCODE"
}

$binaryPath = Join-Path $root 'bin\novelstudio.exe'
$frontendBuild = Join-Path $root 'frontend\dist'
if (-not (Test-Path -LiteralPath $binaryPath -PathType Leaf)) {
  throw "Build output not found: $binaryPath"
}
if (-not (Test-Path -LiteralPath (Join-Path $frontendBuild 'index.html') -PathType Leaf)) {
  throw "Frontend build output not found: $frontendBuild"
}

New-Item -ItemType Directory -Path (Join-Path $packageDirectory 'frontend') -Force | Out-Null
Copy-Item -LiteralPath $binaryPath -Destination (Join-Path $packageDirectory 'novelstudio.exe')
Copy-Item -LiteralPath $frontendBuild -Destination (Join-Path $packageDirectory 'frontend') -Recurse
Copy-Item -LiteralPath (Join-Path $root 'README.md') -Destination $packageDirectory
Copy-Item -LiteralPath (Join-Path $root 'LICENSE') -Destination $packageDirectory

Compress-Archive -Path (Join-Path $packageDirectory '*') -DestinationPath $archivePath -CompressionLevel Optimal
Write-Host "Portable package created: $archivePath"
