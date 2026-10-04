@echo off
REM NovelStudio Build Script for Windows

echo ============================================
echo NovelStudio Build Script
echo ============================================

REM Check if vcpkg is installed
where vcpkg >nul 2>nul
if %ERRORLEVEL% NEQ 0 (
    echo ERROR: vcpkg not found in PATH
    echo Please install vcpkg and run 'vcpkg integrate install'
    exit /b 1
)

REM Install dependencies
echo.
echo [1/5] Installing dependencies...
vcpkg install sqlite3:x64-windows curl:x64-windows nlohmann-json:x64-windows

REM Create build directory
echo.
echo [2/5] Creating build directory...
if not exist build mkdir build
cd build

REM Configure CMake
echo.
echo [3/5] Configuring CMake...
cmake .. -DCMAKE_TOOLCHAIN_FILE=%VCPKG_ROOT%/scripts/buildsystems/vcpkg.cmake -G "Visual Studio 17 2022" -A x64

if %ERRORLEVEL% NEQ 0 (
    echo ERROR: CMake configuration failed
    cd ..
    exit /b 1
)

REM Build Debug
echo.
echo [4/5] Building Debug configuration...
cmake --build . --config Debug

if %ERRORLEVEL% NEQ 0 (
    echo ERROR: Debug build failed
    cd ..
    exit /b 1
)

REM Build Release
echo.
echo [5/5] Building Release configuration...
cmake --build . --config Release

if %ERRORLEVEL% NEQ 0 (
    echo ERROR: Release build failed
    cd ..
    exit /b 1
)

cd ..

echo.
echo ============================================
echo Build completed successfully!
echo ============================================
echo Debug executable: build\bin\Debug\NovelStudio.exe
echo Release executable: build\bin\Release\NovelStudio.exe
echo.

exit /b 0
