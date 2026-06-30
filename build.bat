@echo off
chcp 65001 >nul
setlocal
set PYTHONUTF8=1
set PYTHONIOENCODING=utf-8

python -X utf8 build_release.py
exit /b %errorlevel%
