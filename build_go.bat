@echo off
chcp 65001 >nul
setlocal

set GOTELEMETRY=off
set GOTELEMETRYDIR=%~dp0.gotelemetry
set GOSUMDB=off
set GOPATH=%~dp0.gopath
set GOCACHE=%~dp0.gocache

echo [1/2] Building Go frontend...
if not exist "%~dp0dist-go" mkdir "%~dp0dist-go"
go build -ldflags "-H windowsgui" -o "%~dp0dist-go\translate-go.exe" .\cmd\translate-go
if errorlevel 1 (
  echo.
  echo Build failed.
  pause
  exit /b 1
)

echo [2/2] Build complete:
echo %~dp0dist-go\translate-go.exe
exit /b %errorlevel%
