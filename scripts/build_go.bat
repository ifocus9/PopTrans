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

echo [1/4] Generating Windows application icon...
go run .\tools\winres -icon "%ROOT%\assets\icon.ico" -output "%RESOURCE_FILE%" -arch amd64
if errorlevel 1 (
  echo.
  echo Icon resource generation failed.
  echo Make sure Go is installed and "go" is available on PATH.
  echo.
  pause
  popd
  exit /b 1
)

set "BUNDLE_TAGS="
echo [2/4] Embedding UI and AI engine (single-exe release)...
if exist "%ROOT%\build\bin\translate-ui.exe" if exist "%ROOT%\dist-go\ai_engine.exe" (
  if not exist "%ROOT%\internal\runtimebundle\assets" mkdir "%ROOT%\internal\runtimebundle\assets"
  copy /Y "%ROOT%\build\bin\translate-ui.exe" "%ROOT%\internal\runtimebundle\assets\translate-ui.exe" >nul
  if errorlevel 1 (
    echo Failed to stage translate-ui.exe for embedding.
    pause
    popd
    exit /b 1
  )
  copy /Y "%ROOT%\dist-go\ai_engine.exe" "%ROOT%\internal\runtimebundle\assets\ai_engine.exe" >nul
  if errorlevel 1 (
    echo Failed to stage ai_engine.exe for embedding.
    pause
    popd
    exit /b 1
  )
  set "BUNDLE_TAGS=-tags bundle"
) else (
  echo Bundled assets not found; building a multi-file development layout.
)

echo [3/4] Building Go frontend...
if not exist "%ROOT%\dist-go" mkdir "%ROOT%\dist-go"
set "BUILD_ID="
for /f %%I in ('powershell -NoProfile -Command "Get-Date -Format yyyyMMddHHmmss"') do set "BUILD_ID=%%I"
if not defined BUILD_ID set "BUILD_ID=dev-%RANDOM%%RANDOM%"
go build %BUNDLE_TAGS% -ldflags "-H windowsgui -X translate-plugin/internal/runtimebundle.buildID=%BUILD_ID%" -o "%ROOT%\dist-go\PopTrans.exe" .\cmd\translate-go
if errorlevel 1 (
  del /Q "%RESOURCE_FILE%" >nul 2>nul
  echo.
  echo Build failed.
  echo.
  pause
  popd
  exit /b 1
)

del /Q "%RESOURCE_FILE%" >nul 2>nul

echo [4/4] Build complete:
echo %ROOT%\dist-go\PopTrans.exe
popd
exit /b 0
