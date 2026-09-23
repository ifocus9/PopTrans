@echo off
chcp 65001 >nul
setlocal
pushd "%~dp0.."
set "ROOT=%CD%"

set PYINSTALLER_CONFIG_DIR=%ROOT%\.pyinstaller

echo ========================================================
echo  PopTrans AI 引擎构建 (支持显卡硬件加速)
echo ========================================================

echo [1/4] 检查 Python 与 llama-cpp 环境...
py -3.12 -c "import PyInstaller, fastapi, uvicorn, llama_cpp, rapidocr_onnxruntime, onnxruntime, backend.api_server" >nul 2>nul
if errorlevel 1 (
  python -c "import PyInstaller, fastapi, uvicorn, llama_cpp, rapidocr_onnxruntime, onnxruntime, backend.api_server" >nul 2>nul
  if errorlevel 1 (
    echo 缺失构建依赖，请先运行:
    echo   pip install -r backend\requirements-build.txt
    echo 若需启用 Vulkan GPU 加速，请先重新安装 GPU 版本的 llama-cpp-python:
    echo   set CMAKE_ARGS="-DGGML_VULKAN=on"
    echo   pip install llama-cpp-python --force-reinstall --no-cache-dir
    echo.
    pause
    popd
    exit /b 1
  )
  set "PY_CMD=python"
) else (
  set "PY_CMD=py -3.12"
)

%PY_CMD% -c "import llama_cpp; print('[信息] llama_cpp 版本:', llama_cpp.__version__, '| GPU 卸载支持:', getattr(llama_cpp, 'llama_supports_gpu_offload', lambda: False)())"
if errorlevel 1 (
  echo 使用 %PY_CMD% 导入 llama_cpp 失败。
  echo.
  pause
  popd
  exit /b 1
)

echo [2/4] 打包 ai_engine.exe...
%PY_CMD% -m PyInstaller --noconfirm --clean --distpath "%ROOT%\dist-ai" --workpath "%ROOT%\build-ai" "%ROOT%\backend\ai_engine.spec"
if errorlevel 1 (
  echo AI 引擎打包失败。
  echo.
  pause
  popd
  exit /b 1
)

echo [3/4] 复制外部 RapidOCR 模型...
%PY_CMD% -c "from pathlib import Path; import shutil, rapidocr_onnxruntime; src=Path(rapidocr_onnxruntime.__file__).resolve().parent/'models'; dst=Path(r'%ROOT%\dist-go')/'models'/'rapidocr'; dst.mkdir(parents=True, exist_ok=True); [shutil.copy2(src/name, dst/name) for name in ('ch_PP-OCRv4_det_infer.onnx','ch_PP-OCRv4_rec_infer.onnx','ch_ppocr_mobile_v2.0_cls_infer.onnx')]"
if errorlevel 1 (
  echo 复制 RapidOCR 模型失败。
  echo.
  pause
  popd
  exit /b 1
)

echo [4/4] 组装产物至 dist-go...
if not exist "%ROOT%\dist-go" mkdir "%ROOT%\dist-go"
copy /Y "%ROOT%\dist-ai\ai_engine.exe" "%ROOT%\dist-go\ai_engine.exe" >nul
if errorlevel 1 (
  echo 复制 ai_engine.exe 失败。
  echo.
  pause
  popd
  exit /b 1
)

for %%F in (api_server.py translator.py ocr_service.py runtime_paths.py requirements.txt) do (
  if exist "%ROOT%\dist-go\%%F" del /Q "%ROOT%\dist-go\%%F"
)

echo.
echo ========================================================
echo  AI 引擎构建完成: %ROOT%\dist-go\ai_engine.exe
echo ========================================================
popd
exit /b 0
