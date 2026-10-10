@echo off
setlocal
set "ROOT=%~dp0"

pushd "%ROOT%frontend"
if errorlevel 1 exit /b 1
call npm ci
if errorlevel 1 (
  popd
  exit /b 1
)
call npm run build
if errorlevel 1 (
  popd
  exit /b 1
)
popd

if not exist "%ROOT%bin" mkdir "%ROOT%bin"
if errorlevel 1 exit /b 1
pushd "%ROOT%"
if errorlevel 1 exit /b 1
go build -o "%ROOT%bin\novelstudio.exe" .\cmd\novelstudio
if errorlevel 1 (
  popd
  exit /b 1
)
popd

echo Built bin\novelstudio.exe with frontend\dist.
