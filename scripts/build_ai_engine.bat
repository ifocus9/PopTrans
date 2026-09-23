@echo off
chcp 65001 >nul
setlocal
pushd "%~dp0.."
set "ROOT=%CD%"

set PYINSTALLER_CONFIG_DIR=%ROOT%\.pyinstaller

echo [1/4] Checking Python build dependencies...
py -3.12 -c "import PyInstaller, fastapi, uvicorn, llama_cpp, rapidocr_onnxruntime, onnxruntime, backend.api_server" >nul 2>nul
if errorlevel 1 (
  python -c "import PyInstaller, fastapi, uvicorn, llama_cpp, rapidocr_onnxruntime, onnxruntime, backend.api_server" >nul 2>nul
  if errorlevel 1 (
    echo Missing AI engine build dependencies.
    echo Install them with: python -m pip install -r backend\requirements-build.txt
    echo.
    pause
    popd
    exit /b 1
  )
  set "PY_CMD=python"
) else (
  set "PY_CMD=py -3.12"
)

%PY_CMD% -c "import llama_cpp; print('[Info] llama_cpp version:', llama_cpp.__version__, '| GPU offload supported:', getattr(llama_cpp, 'llama_supports_gpu_offload', lambda: False)())"
if errorlevel 1 (
  echo Failed to import llama_cpp with %PY_CMD%.
  echo.
  pause
  popd
  exit /b 1
)

echo [2/4] Building ai_engine.exe...
%PY_CMD% -m PyInstaller --noconfirm --clean --distpath "%ROOT%\dist-ai" --workpath "%ROOT%\build-ai" "%ROOT%\backend\ai_engine.spec"
if errorlevel 1 (
  echo AI engine build failed.
  echo.
  pause
  popd
  exit /b 1
)

echo [3/4] Copying external RapidOCR models...
%PY_CMD% -c "from pathlib import Path; import shutil, rapidocr_onnxruntime; src=Path(rapidocr_onnxruntime.__file__).resolve().parent/'models'; dst=Path(r'%ROOT%\dist-go')/'models'/'rapidocr'; dst.mkdir(parents=True, exist_ok=True); [shutil.copy2(src/name, dst/name) for name in ('ch_PP-OCRv4_det_infer.onnx','ch_PP-OCRv4_rec_infer.onnx','ch_ppocr_mobile_v2.0_cls_infer.onnx')]"
if errorlevel 1 (
  echo Failed to copy RapidOCR models.
  echo.
  pause
  popd
  exit /b 1
)

echo [4/4] Assembling AI engine...
if not exist "%ROOT%\dist-go" mkdir "%ROOT%\dist-go"
copy /Y "%ROOT%\dist-ai\ai_engine.exe" "%ROOT%\dist-go\ai_engine.exe" >nul
if errorlevel 1 (
  echo Failed to copy ai_engine.exe to dist-go.
  echo.
  pause
  popd
  exit /b 1
)

for %%F in (api_server.py translator.py ocr_service.py runtime_paths.py requirements.txt) do (
  if exist "%ROOT%\dist-go\%%F" del /Q "%ROOT%\dist-go\%%F"
)

echo AI engine build complete:
echo %ROOT%\dist-go\ai_engine.exe
popd
exit /b 0
