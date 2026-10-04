@echo off
REM NovelStudio - Setup and Build Script with vcpkg installation

echo ============================================
echo NovelStudio - Complete Setup Script
echo ============================================

REM Check if vcpkg exists
set VCPKG_ROOT=C:\vcpkg

if exist "%VCPKG_ROOT%\vcpkg.exe" (
    echo [OK] vcpkg found at %VCPKG_ROOT%
    goto install_deps
)

REM Install vcpkg
echo.
echo [1/4] vcpkg not found, installing...
echo This will download and install vcpkg to C:\vcpkg

if not exist C:\vcpkg (
    echo Cloning vcpkg repository...
    git clone https://github.com/Microsoft/vcpkg.git C:\vcpkg
    if %ERRORLEVEL% NEQ 0 (
        echo ERROR: Failed to clone vcpkg
        echo Please ensure git is installed and try again
        pause
        exit /b 1
    )
)

cd C:\vcpkg
echo Running vcpkg bootstrap...
call bootstrap-vcpkg.bat
if %ERRORLEVEL% NEQ 0 (
    echo ERROR: vcpkg bootstrap failed
    pause
    exit /b 1
)

echo Integrating vcpkg with Visual Studio...
vcpkg integrate install

cd /d "%~dp0"

:install_deps
REM Install dependencies
echo.
echo [2/4] Installing dependencies...
echo This may take 5-10 minutes on first run

"%VCPKG_ROOT%\vcpkg" install sqlite3:x64-windows
if %ERRORLEVEL% NEQ 0 (
    echo WARNING: sqlite3 installation had issues
)

"%VCPKG_ROOT%\vcpkg" install curl:x64-windows
if %ERRORLEVEL% NEQ 0 (
    echo WARNING: curl installation had issues
)

"%VCPKG_ROOT%\vcpkg" install nlohmann-json:x64-windows
if %ERRORLEVEL% NEQ 0 (
    echo WARNING: nlohmann-json installation had issues
)

REM Create build directory
echo.
echo [3/4] Configuring CMake...
if not exist build mkdir build
cd build

REM Configure with CMake
cmake .. -DCMAKE_TOOLCHAIN_FILE=%VCPKG_ROOT%/scripts/buildsystems/vcpkg.cmake -G "Visual Studio 17 2022" -A x64

if %ERRORLEVEL% NEQ 0 (
    echo ERROR: CMake configuration failed
    cd ..
    pause
    exit /b 1
)

REM Build
echo.
echo [4/4] Building project...
cmake --build . --config Release

if %ERRORLEVEL% NEQ 0 (
    echo ERROR: Build failed
    cd ..
    pause
    exit /b 1
)

cd ..

echo.
echo ============================================
echo Build completed successfully!
echo ============================================
echo.
echo Executable: build\bin\Release\NovelStudio.exe
echo.
echo To run the application:
echo   build\bin\Release\NovelStudio.exe
echo.
echo Or simply run: quick-run.bat
echo.

REM Create quick run script
echo @echo off > quick-run.bat
echo cd /d "%%~dp0" >> quick-run.bat
echo if exist build\bin\Release\NovelStudio.exe ( >> quick-run.bat
echo     build\bin\Release\NovelStudio.exe >> quick-run.bat
echo ) else if exist build\bin\Debug\NovelStudio.exe ( >> quick-run.bat
echo     build\bin\Debug\NovelStudio.exe >> quick-run.bat
echo ) else ( >> quick-run.bat
echo     echo ERROR: NovelStudio.exe not found >> quick-run.bat
echo     echo Please run setup-and-build.bat first >> quick-run.bat
echo     pause >> quick-run.bat
echo ) >> quick-run.bat

pause
exit /b 0
