[CmdletBinding()]
param(
    [string]$PackageDirectory = ""
)

$ErrorActionPreference = "Stop"
if ([string]::IsNullOrWhiteSpace($PackageDirectory)) {
    $PackageDirectory = Join-Path $PSScriptRoot "..\release"
}
$packageRoot = [System.IO.Path]::GetFullPath($PackageDirectory)
$server = Join-Path $packageRoot "novelstudio.exe"
if (-not (Test-Path $server -PathType Leaf)) {
    throw "Local package not found at $packageRoot. Run scripts/build-local.ps1 first."
}

$listener = [System.Net.Sockets.TcpListener]::new([System.Net.IPAddress]::Loopback, 0)
$listener.Start()
$port = $listener.LocalEndpoint.Port
$listener.Stop()

$testRoot = Join-Path $packageRoot (".smoke-" + [guid]::NewGuid().ToString("N"))
$testDatabase = Join-Path $testRoot "smoke.sqlite"
$stdoutPath = Join-Path $testRoot "stdout.log"
$stderrPath = Join-Path $testRoot "stderr.log"
$environment = @{
    APP_ADDR   = "127.0.0.1:$port"
    APP_ENV    = "development"
    DB_PATH    = $testDatabase
    STATIC_DIR = Join-Path $packageRoot "frontend\dist"
}
$previousEnvironment = @{}
$process = $null

try {
    New-Item -ItemType Directory -Force -Path $testRoot | Out-Null
    foreach ($key in $environment.Keys) {
        $current = Get-Item "Env:$key" -ErrorAction SilentlyContinue
        $previousEnvironment[$key] = if ($current) { $current.Value } else { $null }
        Set-Item "Env:$key" $environment[$key]
    }

    $process = Start-Process -FilePath $server -WorkingDirectory $packageRoot -PassThru -WindowStyle Hidden `
        -RedirectStandardOutput $stdoutPath -RedirectStandardError $stderrPath
    $healthy = $false
    for ($attempt = 0; $attempt -lt 60; $attempt++) {
        if ($process.HasExited) {
            $details = Get-Content $stderrPath -Raw -ErrorAction SilentlyContinue
            throw "Packaged server exited with code $($process.ExitCode). $details"
        }
        try {
            $health = Invoke-RestMethod -Uri "http://127.0.0.1:$port/healthz" -TimeoutSec 2
            if ($health.status -eq "ok") {
                $healthy = $true
                break
            }
        }
        catch {
            Start-Sleep -Milliseconds 250
        }
    }
    if (-not $healthy) {
        throw "Packaged server did not become healthy on port $port."
    }

    $page = Invoke-WebRequest -Uri "http://127.0.0.1:$port/" -UseBasicParsing -TimeoutSec 5
    if ($page.StatusCode -ne 200 -or $page.Content -notmatch '<div id="app"') {
        throw "Packaged server did not return the frontend entry page."
    }
    $route = Invoke-WebRequest -Uri "http://127.0.0.1:$port/smoke/route" -UseBasicParsing -TimeoutSec 5
    if ($route.StatusCode -ne 200 -or $route.Content -notmatch '<div id="app"') {
        throw "Packaged server did not return the SPA fallback for client routes."
    }

    $assets = [regex]::Matches($page.Content, '(?:src|href)="(/assets/[^"]+)"')
    if ($assets.Count -eq 0) {
        throw "Frontend entry page did not reference built assets."
    }
    foreach ($asset in $assets) {
        $response = Invoke-WebRequest -Uri ("http://127.0.0.1:$port" + $asset.Groups[1].Value) -UseBasicParsing -TimeoutSec 5
        if ($response.StatusCode -ne 200) {
            throw "Packaged frontend asset failed: $($asset.Groups[1].Value)"
        }
    }

    $session = New-Object Microsoft.PowerShell.Commands.WebRequestSession
    $account = @{
        email = "smoke-$([guid]::NewGuid().ToString('N'))@example.test"
        password = "smoke-test-password-strong"
    } | ConvertTo-Json
    $registration = Invoke-RestMethod -Uri "http://127.0.0.1:$port/api/v1/auth/register" `
        -Method Post -WebSession $session -ContentType "application/json" -Body $account -TimeoutSec 5
    if (-not $registration.user.id) {
        throw "Packaged registration did not return a signed-in user."
    }
    $settings = Invoke-RestMethod -Uri "http://127.0.0.1:$port/api/v1/settings/llm" -WebSession $session -TimeoutSec 5
    if (-not $settings.canManage -or $settings.settings.PSObject.Properties.Name -contains "apiKey") {
        throw "Model settings should be admin-visible without returning the API key."
    }
    Write-Host "Local package smoke test passed: health, SPA routes, $($assets.Count) assets, and authenticated model settings."
}
finally {
    if ($process -and -not $process.HasExited) {
        Stop-Process -Id $process.Id
        Wait-Process -Id $process.Id -Timeout 10 -ErrorAction SilentlyContinue
    }
    if ($process) {
        $process.Dispose()
    }
    foreach ($key in $previousEnvironment.Keys) {
        if ($null -eq $previousEnvironment[$key]) {
            Remove-Item "Env:$key" -ErrorAction SilentlyContinue
        }
        else {
            Set-Item "Env:$key" $previousEnvironment[$key]
        }
    }
    for ($attempt = 0; $attempt -lt 10 -and (Test-Path $testRoot); $attempt++) {
        try {
            Remove-Item -LiteralPath $testRoot -Recurse -Force -ErrorAction Stop
        }
        catch {
            if ($attempt -eq 9) {
                Write-Warning "Could not remove smoke-test files at $testRoot."
            }
            else {
                Start-Sleep -Milliseconds 100
            }
        }
    }
}
