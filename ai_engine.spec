from PyInstaller.utils.hooks import (
    collect_data_files,
    collect_dynamic_libs,
    collect_submodules,
    copy_metadata,
)


binaries = collect_dynamic_libs("llama_cpp")
datas = collect_data_files(
    "rapidocr_onnxruntime",
    includes=["config.yaml"],
)
datas += copy_metadata("python-multipart")

hiddenimports = collect_submodules("uvicorn")
hiddenimports += collect_submodules("rapidocr_onnxruntime")
hiddenimports += [
    "multipart",
    "python_multipart",
    "python_multipart.multipart",
]

a = Analysis(
    ["backend/api_server.py"],
    pathex=["."],
    binaries=binaries,
    datas=datas,
    hiddenimports=hiddenimports,
    hookspath=[],
    hooksconfig={},
    runtime_hooks=[],
    excludes=[
        "IPython",
        "jupyter",
        "matplotlib",
        "onnxruntime.quantization",
        "onnxruntime.tools",
        "tkinter",
    ],
    noarchive=False,
    optimize=1,
)
pyz = PYZ(a.pure)

exe = EXE(
    pyz,
    a.scripts,
    a.binaries,
    a.datas,
    [],
    name="ai_engine",
    debug=False,
    bootloader_ignore_signals=False,
    strip=False,
    upx=False,
    console=False,
    disable_windowed_traceback=False,
    argv_emulation=False,
    target_arch=None,
    codesign_identity=None,
    entitlements_file=None,
    icon=["icon.ico"],
)
