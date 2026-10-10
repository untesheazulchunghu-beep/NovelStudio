@echo off
setlocal
title NovelStudio development
set "NOVELSTUDIO_DEV_ROOT=%~dp0"

start "NovelStudio API" powershell -NoExit -ExecutionPolicy Bypass -Command "Set-Location -LiteralPath $env:NOVELSTUDIO_DEV_ROOT; go run ./cmd/novelstudio"
start "NovelStudio UI" powershell -NoExit -ExecutionPolicy Bypass -Command "Set-Location -LiteralPath (Join-Path $env:NOVELSTUDIO_DEV_ROOT 'frontend'); npm ci; if ($LASTEXITCODE -ne 0) { Write-Host 'npm ci failed; dev server was not started.'; return }; npm run dev"

echo NovelStudio is starting.
echo API: http://localhost:8080
echo UI : http://localhost:5173
