@echo off
chcp 65001 >nul
setlocal

echo ========================================
echo   translate-plugin build
echo ========================================
echo.

echo [1/4] clean old files...
if exist build rmdir /s /q build
if exist dist rmdir /s /q dist
if exist "选中翻译.spec" del /f /q "选中翻译.spec"

echo [2/4] run PyInstaller...
python -m PyInstaller --windowed -y --name "选中翻译" --add-binary "C:\Users\admin\AppData\Local\Programs\Python\Python312\DLLs\_tkinter.pyd;." --add-binary "C:\Users\admin\AppData\Local\Programs\Python\Python312\DLLs\tcl86t.dll;." --add-binary "C:\Users\admin\AppData\Local\Programs\Python\Python312\DLLs\tk86t.dll;." --add-data "C:\Users\admin\AppData\Local\Programs\Python\Python312\tcl\tcl8.6;_tcl_data" --add-data "C:\Users\admin\AppData\Local\Programs\Python\Python312\tcl\tk8.6;_tk_data" --add-data "translator.py;." --add-data "hotkey_manager.py;." --add-data "popup_window.py;." --add-data "tray_icon.py;." --add-data "C:\Users\admin\AppData\Local\Programs\Python\Python312\Lib\site-packages\llama_cpp;llama_cpp" main.py

if errorlevel 1 (
    echo.
    echo [ERROR] PyInstaller failed
    exit /b 1
)

echo [3/4] copy model files...
xcopy /E /I /Y models "dist\选中翻译\models"

if errorlevel 1 (
    echo.
    echo [ERROR] model copy failed
    exit /b 1
)

echo [4/4] clean temp files...
if exist build rmdir /s /q build
if exist "选中翻译.spec" del /f /q "选中翻译.spec"

echo.
echo ========================================
echo   build complete
echo   output: dist\选中翻译\
echo   exe: dist\选中翻译\选中翻译.exe
echo ========================================
endlocal
