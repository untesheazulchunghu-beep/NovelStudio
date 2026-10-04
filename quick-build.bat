@echo off
REM Quick development build script

echo Building NovelStudio (Debug)...

if not exist build (
    echo Build directory not found. Run build.bat first.
    exit /b 1
)

cd build
cmake --build . --config Debug --parallel
cd ..

echo Done!
