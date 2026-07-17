@echo off
chcp 65001 >nul
setlocal
pushd "%~dp0"

set GOPATH=%~dp0.gopath
set GOCACHE=%~dp0.gocache
set GOBIN=%~dp0.gobin
set GOTELEMETRY=off
set GOTELEMETRYDIR=%~dp0.gotelemetry
set GOSUMDB=off
set HOME=%~dp0.npmhome
set USERPROFILE=%~dp0.npmhome
set NPM_CONFIG_CACHE=%~dp0.npmcache

set WAILS_EXE=%~dp0.gobin\wails.exe
if exist "%WAILS_EXE%" goto build

echo [1/3] Installing the Wails build tool...
for /f "usebackq delims=" %%V in (`go list -m -f "{{.Version}}" github.com/wailsapp/wails/v2`) do set WAILS_VERSION=%%V
if not defined WAILS_VERSION (
  echo Unable to determine the Wails version from go.mod.
  popd
  exit /b 1
)
go install github.com/wailsapp/wails/v2/cmd/wails@%WAILS_VERSION%
if errorlevel 1 (
  echo Wails tool installation failed.
  popd
  exit /b 1
)

:build
echo [2/3] Building Wails UI...
"%WAILS_EXE%" build -clean
if errorlevel 1 (
  echo.
  echo Wails build failed.
  popd
  exit /b 1
)

echo [3/3] Build complete:
echo %~dp0build\bin\translate-wails.exe
popd
exit /b 0
