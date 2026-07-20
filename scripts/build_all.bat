@echo off
chcp 65001 >nul
setlocal
pushd "%~dp0.."
set "ROOT=%CD%"

call "%~dp0build_wails.bat"
if errorlevel 1 (
  popd
  exit /b 1
)

call "%~dp0build_ai_engine.bat"
if errorlevel 1 (
  popd
  exit /b 1
)

call "%~dp0build_go.bat"
if errorlevel 1 (
  popd
  exit /b 1
)

for %%F in (
  capture_selection.py
  settings.json
) do (
  if exist "%ROOT%\dist-go\%%F" del /Q "%ROOT%\dist-go\%%F"
)
if exist "%ROOT%\dist-go\*.log" del /Q "%ROOT%\dist-go\*.log"
if exist "%ROOT%\dist-go\__pycache__" rmdir /S /Q "%ROOT%\dist-go\__pycache__"
if exist "%ROOT%\dist-go\translate-wails.exe" del /Q "%ROOT%\dist-go\translate-wails.exe"

copy /Y "%ROOT%\build\bin\translate-ui.exe" "%ROOT%\dist-go\translate-ui.exe" >nul
if errorlevel 1 (
  echo Failed to assemble Wails UI in dist-go.
  popd
  exit /b 1
)

for %%F in (
  icon.ico
) do (
  copy /Y "%ROOT%\assets\%%F" "%ROOT%\dist-go\%%F" >nul
  if errorlevel 1 (
    echo Failed to copy %%F to dist-go.
    popd
    exit /b 1
  )
)

set MODEL_REL=models\Hy-MT2-1.8B-GGUF\Hy-MT2-1.8B-Q4_K_M.gguf
if exist "%ROOT%\%MODEL_REL%" if not exist "%ROOT%\dist-go\%MODEL_REL%" (
  if not exist "%ROOT%\dist-go\models\Hy-MT2-1.8B-GGUF" mkdir "%ROOT%\dist-go\models\Hy-MT2-1.8B-GGUF"
  mklink /H "%ROOT%\dist-go\%MODEL_REL%" "%ROOT%\%MODEL_REL%" >nul 2>nul
  if errorlevel 1 copy /Y "%ROOT%\%MODEL_REL%" "%ROOT%\dist-go\%MODEL_REL%" >nul
  if errorlevel 1 (
    echo Failed to include the existing translation model.
    popd
    exit /b 1
  )
)

echo.
echo Complete application assembled in:
echo %ROOT%\dist-go
echo.
echo Start with:
echo %ROOT%\dist-go\PopTrans.exe
echo.
echo Python is embedded in ai_engine.exe; no user installation is required.
echo AI models remain in the models directory.
popd
exit /b 0
