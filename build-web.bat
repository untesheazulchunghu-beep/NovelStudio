@echo off
REM NovelStudio Web - Production Build Script

echo ========================================
echo NovelStudio Web - Production Build
echo ========================================
echo.

REM Build Frontend
echo [1/3] Building Frontend...
cd frontend
call npm install
call npm run build
cd ..

if not exist "frontend\dist" (
    echo [ERROR] Frontend build failed!
    pause
    exit /b 1
)

echo [OK] Frontend built to frontend\dist
echo.

REM Build Backend
echo [2/3] Building Backend...
cd backend
if not exist build mkdir build
cd build
cmake .. -G "Visual Studio 17 2022" -A x64
cmake --build . --config Release
cd ..\..

if not exist "backend\build\bin\Release\NovelStudioServer.exe" (
    echo [ERROR] Backend build failed!
    pause
    exit /b 1
)

echo [OK] Backend built to backend\build\bin\Release
echo.

REM Package
echo [3/3] Packaging...
if not exist dist mkdir dist
copy backend\build\bin\Release\NovelStudioServer.exe dist\
xcopy frontend\dist dist\frontend\ /E /I /Y >nul

copy config.example.json dist\config.json >nul

echo.
echo ========================================
echo Build Complete!
echo ========================================
echo.
echo Output: dist\
echo   - NovelStudioServer.exe (Backend)
echo   - frontend\ (Web UI)
echo   - config.json (Configuration)
echo.
echo To run: cd dist && NovelStudioServer.exe
echo Then open: http://localhost:3000
echo.
pause
