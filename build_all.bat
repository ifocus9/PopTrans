@echo off
chcp 65001 >nul
setlocal

call "%~dp0build_wails.bat"
if errorlevel 1 exit /b 1

call "%~dp0build_ai_engine.bat"
if errorlevel 1 exit /b 1

call "%~dp0build_go.bat"
if errorlevel 1 exit /b 1

for %%F in (
  capture_selection.py
  settings.json
) do (
  if exist "%~dp0dist-go\%%F" del /Q "%~dp0dist-go\%%F"
)
if exist "%~dp0dist-go\*.log" del /Q "%~dp0dist-go\*.log"
if exist "%~dp0dist-go\__pycache__" rmdir /S /Q "%~dp0dist-go\__pycache__"

copy /Y "%~dp0build\bin\translate-wails.exe" "%~dp0dist-go\translate-wails.exe" >nul
if errorlevel 1 (
  echo Failed to assemble Wails UI in dist-go.
  exit /b 1
)

for %%F in (
  icon.ico
) do (
  copy /Y "%~dp0%%F" "%~dp0dist-go\%%F" >nul
  if errorlevel 1 (
    echo Failed to copy %%F to dist-go.
    exit /b 1
  )
)

set MODEL_REL=models\Hy-MT2-1.8B-GGUF\Hy-MT2-1.8B-Q4_K_M.gguf
if exist "%~dp0%MODEL_REL%" if not exist "%~dp0dist-go\%MODEL_REL%" (
  if not exist "%~dp0dist-go\models\Hy-MT2-1.8B-GGUF" mkdir "%~dp0dist-go\models\Hy-MT2-1.8B-GGUF"
  mklink /H "%~dp0dist-go\%MODEL_REL%" "%~dp0%MODEL_REL%" >nul 2>nul
  if errorlevel 1 copy /Y "%~dp0%MODEL_REL%" "%~dp0dist-go\%MODEL_REL%" >nul
  if errorlevel 1 (
    echo Failed to include the existing translation model.
    exit /b 1
  )
)

echo.
echo Complete application assembled in:
echo %~dp0dist-go
echo.
echo Start with:
echo %~dp0dist-go\translate-go.exe
echo.
echo Python is embedded in ai_engine.exe; no user installation is required.
echo AI models remain in the models directory.
exit /b 0
