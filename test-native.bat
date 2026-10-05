@echo off
REM Quick build script for testing native implementations

echo ========================================
echo NovelStudio - Test Native Build
echo ========================================
echo.

REM Create build directory
if not exist "build-test" mkdir build-test
cd build-test

REM Configure with CMake
echo Configuring CMake...
cmake .. -G "Visual Studio 17 2022" -A x64

if errorlevel 1 (
    echo.
    echo ERROR: CMake configuration failed
    pause
    exit /b 1
)

REM Build the test executable
echo.
echo Building TestNative...
cmake --build . --config Release --target TestNative

if errorlevel 1 (
    echo.
    echo ERROR: Build failed
    pause
    exit /b 1
)

echo.
echo ========================================
echo Build successful!
echo ========================================
echo.
echo Running TestNative...
echo.

REM Run the test
bin\Release\TestNative.exe

cd ..
pause
