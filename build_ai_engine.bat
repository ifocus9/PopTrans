@echo off
chcp 65001 >nul
setlocal
pushd "%~dp0"

set PYINSTALLER_CONFIG_DIR=%~dp0.pyinstaller

echo [1/4] Checking Python build dependencies...
python -c "import PyInstaller, fastapi, uvicorn, llama_cpp, rapidocr_onnxruntime, onnxruntime, backend.api_server" >nul
if errorlevel 1 (
  echo Missing AI engine build dependencies.
  echo Install them with: python -m pip install -r requirements-build.txt
  popd
  exit /b 1
)

echo [2/4] Building ai_engine.exe...
python -m PyInstaller --noconfirm --clean --distpath "%~dp0dist-ai" --workpath "%~dp0build-ai" "%~dp0ai_engine.spec"
if errorlevel 1 (
  echo AI engine build failed.
  popd
  exit /b 1
)

echo [3/4] Copying external RapidOCR models...
python -c "from pathlib import Path; import shutil, rapidocr_onnxruntime; src=Path(rapidocr_onnxruntime.__file__).resolve().parent/'models'; dst=Path(r'%~dp0dist-go')/'models'/'rapidocr'; dst.mkdir(parents=True, exist_ok=True); [shutil.copy2(src/name, dst/name) for name in ('ch_PP-OCRv4_det_infer.onnx','ch_PP-OCRv4_rec_infer.onnx','ch_ppocr_mobile_v2.0_cls_infer.onnx')]"
if errorlevel 1 (
  echo Failed to copy RapidOCR models.
  popd
  exit /b 1
)

echo [4/4] Assembling AI engine...
if not exist "%~dp0dist-go" mkdir "%~dp0dist-go"
copy /Y "%~dp0dist-ai\ai_engine.exe" "%~dp0dist-go\ai_engine.exe" >nul
if errorlevel 1 (
  echo Failed to copy ai_engine.exe to dist-go.
  popd
  exit /b 1
)

for %%F in (api_server.py translator.py ocr_service.py runtime_paths.py requirements.txt) do (
  if exist "%~dp0dist-go\%%F" del /Q "%~dp0dist-go\%%F"
)

echo AI engine build complete:
echo %~dp0dist-go\ai_engine.exe
popd
exit /b 0
