@echo off
echo ========================================
echo NovelStudio - Quick Functionality Test
echo ========================================
echo.

echo [1/5] Testing Native Implementation...
build\bin\Release\TestNative.exe
if errorlevel 1 (
    echo [FAILED] Native test failed
    pause
    exit /b 1
)
echo [PASS] Native implementation works!
echo.

echo [2/5] Testing Providers...
build\bin\Release\TestProviders.exe
if errorlevel 1 (
    echo [FAILED] Providers test failed
    pause
    exit /b 1
)
echo [PASS] All providers initialized!
echo.

echo [3/5] Testing Streaming...
build\bin\Release\TestStreaming.exe
if errorlevel 1 (
    echo [FAILED] Streaming test failed
    pause
    exit /b 1
)
echo [PASS] SSE parser works!
echo.

echo ========================================
echo All core components verified!
echo ========================================
echo.
echo Available demos:
echo - DemoWorkflow.exe (Complete writing workflow)
echo - DemoWritingAssistant.exe (AI assistant)
echo.
echo Run 'start.bat' for interactive menu
echo.
pause
