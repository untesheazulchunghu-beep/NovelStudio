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
    $profile = Invoke-RestMethod -Uri "http://127.0.0.1:$port/api/v1/me" -WebSession $session -TimeoutSec 5
    $workspace = $profile.workspaces | Select-Object -First 1
    if (-not $workspace.id) {
        throw "Packaged registration did not return a workspace."
    }
    $novelBody = @{
        workspaceId = $workspace.id
        title = "Smoke Novel"
        summary = "Packaged smoke test"
        genre = "Test"
        plannedChapters = 1
        approvalMode = "manual"
    } | ConvertTo-Json
    $novel = Invoke-RestMethod -Uri "http://127.0.0.1:$port/api/v1/novels" `
        -Method Post -WebSession $session -ContentType "application/json" -Body $novelBody -TimeoutSec 5
    if (-not $novel.id) {
        throw "Packaged novel creation did not return a novel."
    }
    $chapter = Invoke-RestMethod -Uri "http://127.0.0.1:$port/api/v1/novels/$($novel.id)/chapters" `
        -Method Post -WebSession $session -ContentType "application/json" -Body (@{ title = "Chapter 1" } | ConvertTo-Json) -TimeoutSec 5
    if (-not $chapter.id) {
        throw "Packaged chapter creation did not return a chapter."
    }
    $chapterUpdate = @{
        expectedRevision = $chapter.revision
        expectedUpdatedAt = $chapter.updatedAt
        title = $chapter.title
        body = "Smoke body content"
        goal = ""
        keyEvents = ""
        suspenseHook = ""
    } | ConvertTo-Json
    $savedChapter = Invoke-RestMethod -Uri "http://127.0.0.1:$port/api/v1/novels/$($novel.id)/chapters/$($chapter.id)" `
        -Method Put -WebSession $session -ContentType "application/json" -Body $chapterUpdate -TimeoutSec 5
    $savedChapter = (Invoke-RestMethod -Uri "http://127.0.0.1:$port/api/v1/novels/$($novel.id)/chapters" `
        -WebSession $session -TimeoutSec 5).chapters | Where-Object { $_.id -eq $chapter.id }
    if (-not $savedChapter -or $savedChapter.body -ne "Smoke body content") {
        throw "Packaged chapter save did not persist the saved body."
    }
    $search = Invoke-RestMethod -Uri "http://127.0.0.1:$port/api/v1/novels/$($novel.id)/search?q=Smoke%20body" `
        -WebSession $session -TimeoutSec 5
    if ($search.chapters.Count -ne 1 -or $search.chapters[0].id -ne $chapter.id) {
        throw "Packaged chapter search did not return the saved chapter."
    }
    $invalidSearchLimitRejected = $false
    try {
        Invoke-RestMethod -Uri "http://127.0.0.1:$port/api/v1/novels/$($novel.id)/search?q=Smoke&limit=0" `
            -WebSession $session -TimeoutSec 5 | Out-Null
    }
    catch {
        $invalidSearchLimitRejected = $_.Exception.Response.StatusCode.value__ -eq 400
    }
    if (-not $invalidSearchLimitRejected) {
        throw "Packaged chapter search accepted an invalid result limit."
    }
    $staleBody = @{
        expectedRevision = $chapter.revision
        expectedUpdatedAt = $chapter.updatedAt
        title = $chapter.title
        body = "Stale overwrite"
        goal = ""
        keyEvents = ""
        suspenseHook = ""
    } | ConvertTo-Json
    $staleRejected = $false
    try {
        Invoke-RestMethod -Uri "http://127.0.0.1:$port/api/v1/novels/$($novel.id)/chapters/$($chapter.id)" `
            -Method Put -WebSession $session -ContentType "application/json" -Body $staleBody -TimeoutSec 5 | Out-Null
    }
    catch {
        $staleRejected = $_.Exception.Response.StatusCode.value__ -eq 409
    }
    if (-not $staleRejected) {
        throw "Packaged chapter save accepted a stale revision."
    }
    $export = Invoke-WebRequest -Uri "http://127.0.0.1:$port/api/v1/novels/$($novel.id)/export?format=md" `
        -WebSession $session -UseBasicParsing -TimeoutSec 5
    if ($export.StatusCode -ne 200 -or $export.Content -notmatch "Chapter 1" -or $export.Content -notmatch "Smoke body content") {
        throw "Packaged Markdown export did not include the saved chapter content."
    }
    $docxExport = Invoke-WebRequest -Uri "http://127.0.0.1:$port/api/v1/novels/$($novel.id)/export?format=docx" `
        -WebSession $session -UseBasicParsing -TimeoutSec 5
    if ($docxExport.StatusCode -ne 200 -or $docxExport.Headers['Content-Type'] -notmatch 'application/vnd.openxmlformats-officedocument.wordprocessingml.document') {
        throw "Packaged Word export failed."
    }
    $invalidExportRejected = $false
    try {
        Invoke-WebRequest -Uri "http://127.0.0.1:$port/api/v1/novels/$($novel.id)/export?format=pdf" `
            -WebSession $session -UseBasicParsing -TimeoutSec 5 | Out-Null
    }
    catch {
        $invalidExportRejected = $_.Exception.Response.StatusCode.value__ -eq 400
    }
    if (-not $invalidExportRejected) {
        throw "Packaged export accepted an unsupported format."
    }
    $settings = Invoke-RestMethod -Uri "http://127.0.0.1:$port/api/v1/settings/llm" -WebSession $session -TimeoutSec 5
    if (-not $settings.canManage -or $settings.settings.PSObject.Properties.Name -contains "apiKey") {
        throw "Model settings should be admin-visible without returning the API key."
    }
    Write-Host "Local package smoke test passed: health, SPA routes, $($assets.Count) assets, auth, novel/chapter creation, chapter save, stale revision rejection, chapter search, Markdown/Word export, invalid format rejection, and model settings."
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
