@echo off
chcp 65001 >nul
setlocal
pushd "%~dp0.."
set "ROOT=%CD%"

set GOPATH=%ROOT%\.gopath
set GOCACHE=%ROOT%\.gocache
set GOBIN=%ROOT%\.gobin
set GOTELEMETRY=off
set GOTELEMETRYDIR=%ROOT%\.gotelemetry
set GOSUMDB=off
set HOME=%ROOT%\.npmhome
set USERPROFILE=%ROOT%\.npmhome
set NPM_CONFIG_CACHE=%ROOT%\.npmcache

set WAILS_EXE=%ROOT%\.gobin\wails.exe
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
if not exist "%ROOT%\build\windows" mkdir "%ROOT%\build\windows"
copy /Y "%ROOT%\assets\icon.ico" "%ROOT%\build\windows\icon.ico" >nul
if errorlevel 1 (
  echo Failed to prepare the Wails application icon.
  popd
  exit /b 1
)

echo [2/3] Building Wails UI...
"%WAILS_EXE%" build -clean
if errorlevel 1 (
  echo.
  echo Wails build failed.
  popd
  exit /b 1
)

if exist "%ROOT%\build\bin\translate-wails.exe" del /Q "%ROOT%\build\bin\translate-wails.exe"

echo [3/3] Build complete:
echo %ROOT%\build\bin\translate-ui.exe
popd
exit /b 0
