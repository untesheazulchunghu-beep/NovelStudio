@echo off
REM NovelStudio Web - Development Server Startup

echo ========================================
echo NovelStudio Web Development
echo ========================================
echo.

REM Check if backend is built
if not exist "backend\build\bin\Release\NovelStudioServer.exe" (
    echo [INFO] Backend not built yet, building now...
    cd backend
    if not exist build mkdir build
    cd build
    cmake .. -G "Visual Studio 17 2022" -A x64
    cmake --build . --config Release
    cd ..\..
)

REM Check if frontend deps are installed
if not exist "frontend\node_modules" (
    echo [INFO] Installing frontend dependencies...
    cd frontend
    call npm install
    cd ..
)

echo.
echo [1/2] Starting Backend Server (Port 3000)...
start "NovelStudio Backend" cmd /k "cd backend && build\bin\Release\NovelStudioServer.exe"

timeout /t 3 /nobreak >nul

echo [2/2] Starting Frontend Dev Server (Port 5173)...
start "NovelStudio Frontend" cmd /k "cd frontend && npm run dev"

echo.
echo ========================================
echo NovelStudio Web is starting!
echo ========================================
echo.
echo Backend:  http://localhost:3000
echo Frontend: http://localhost:5173
echo API Docs: http://localhost:3000/api/health
echo.
echo Press any key to stop all servers...
pause >nul

taskkill /FI "WINDOWTITLE eq NovelStudio Backend" /F >nul 2>&1
taskkill /FI "WINDOWTITLE eq NovelStudio Frontend" /F >nul 2>&1

echo.
echo All servers stopped.
