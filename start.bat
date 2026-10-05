@echo off
REM NovelStudio Quick Start Script

echo ========================================
echo NovelStudio - Quick Start
echo ========================================
echo.

REM Check if already built
if not exist "build\bin\Release\DemoWorkflow.exe" (
    echo Building project...
    echo.

    cmake -G "Visual Studio 18 2026" -A x64 -B build
    if errorlevel 1 (
        echo [ERROR] CMake configuration failed
        pause
        exit /b 1
    )

    cmake --build build --config Release
    if errorlevel 1 (
        echo [ERROR] Build failed
        pause
        exit /b 1
    )

    echo.
    echo [SUCCESS] Build complete!
    echo.
)

REM Check config
if not exist "config.json" (
    echo [INFO] config.json not found, creating from example...
    copy config.example.json config.json >nul
    echo.
    echo Please edit config.json with your API keys:
    echo - openai_api_key
    echo - anthropic_api_key
    echo - ollama_endpoint
    echo.
    pause
)

REM Show menu
:menu
cls
echo ========================================
echo NovelStudio - Select Demo
echo ========================================
echo.
echo 1. Writing Workflow (Complete)
echo 2. AI Assistant (Provider Testing)
echo 3. Test Native Implementation
echo 4. Test All Providers
echo 5. Test Streaming
echo 0. Exit
echo.
set /p choice="Choice: "

if "%choice%"=="1" goto workflow
if "%choice%"=="2" goto assistant
if "%choice%"=="3" goto native
if "%choice%"=="4" goto providers
if "%choice%"=="5" goto streaming
if "%choice%"=="0" goto end
goto menu

:workflow
echo.
echo Launching Writing Workflow...
echo.
build\bin\Release\DemoWorkflow.exe
goto menu

:assistant
echo.
echo Launching AI Assistant...
echo.
build\bin\Release\DemoWritingAssistant.exe
goto menu

:native
echo.
echo Testing Native Implementation...
echo.
build\bin\Release\TestNative.exe
pause
goto menu

:providers
echo.
echo Testing Providers...
echo.
build\bin\Release\TestProviders.exe
pause
goto menu

:streaming
echo.
echo Testing Streaming...
echo.
build\bin\Release\TestStreaming.exe
pause
goto menu

:end
echo.
echo Thank you for using NovelStudio!
echo.
pause
