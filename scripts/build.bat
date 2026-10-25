@echo off
REM Tiklive build script — frontend + backend.
setlocal
cd /d "%~dp0.."

echo [1/3] Building frontend (web)...
pushd web
call npm run build
if errorlevel 1 (echo FRONTEND BUILD FAILED & popd & exit /b 1)
popd

echo [2/3] Building backend (Go)...
go build -o bin\tiklive.exe cmd\server\main.go
if errorlevel 1 (echo GO BUILD FAILED & exit /b 1)

echo [3/3] Done.
echo   binary: bin\tiklive.exe
echo   web:    web\dist
echo Run: bin\tiklive.exe  (then open http://127.0.0.1:8080/?mode=dashboard)
endlocal
