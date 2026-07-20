@echo off
chcp 65001 >nul
setlocal
pushd "%~dp0.."
set "ROOT=%CD%"

set GOTELEMETRY=off
set GOTELEMETRYDIR=%ROOT%\.gotelemetry
set GOSUMDB=off
set GOPATH=%ROOT%\.gopath
set GOCACHE=%ROOT%\.gocache

set RESOURCE_FILE=%ROOT%\cmd\translate-go\translate-go-res.syso

echo [1/3] Generating Windows application icon...
go run .\tools\winres -icon "%ROOT%\assets\icon.ico" -output "%RESOURCE_FILE%" -arch amd64
if errorlevel 1 (
  echo.
  echo Icon resource generation failed.
  popd
  exit /b 1
)

echo [2/3] Building Go frontend...
if not exist "%ROOT%\dist-go" mkdir "%ROOT%\dist-go"
go build -ldflags "-H windowsgui" -o "%ROOT%\dist-go\PopTrans.exe" .\cmd\translate-go
if errorlevel 1 (
  del /Q "%RESOURCE_FILE%" >nul 2>nul
  echo.
  echo Build failed.
  popd
  exit /b 1
)

del /Q "%RESOURCE_FILE%" >nul 2>nul

echo [3/3] Build complete:
echo %ROOT%\dist-go\PopTrans.exe
popd
exit /b 0
