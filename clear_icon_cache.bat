@echo off
chcp 65001 >nul
echo ============================================
echo   Windows 图标缓存清理工具
echo ============================================
echo.
echo 用途：重新打包 exe 后，桌面/资源管理器仍显示旧图标（如白底）时使用。
echo 原理：Windows 会把 exe 图标缓存到本地数据库，改了 ICO 也不会立即刷新。
echo.

echo [1/4] 关闭资源管理器...
taskkill /IM explorer.exe /F >nul 2>&1

echo [2/4] 删除图标缓存数据库...
if exist "%localappdata%\IconCache.db" (
    del /A /Q "%localappdata%\IconCache.db"
    echo       已删除 IconCache.db
) else (
    echo       IconCache.db 不存在，跳过
)

echo [3/4] 删除 Explorer 图标缓存...
del /A /F /Q "%localappdata%\Microsoft\Windows\Explorer\iconcache_*.db" >nul 2>&1
echo       已清理 iconcache_*.db

echo [4/4] 重启资源管理器...
start explorer.exe

echo.
echo ============================================
echo   完成！请重新查看桌面图标
echo   如果仍未刷新，请重启电脑
echo ============================================
pause
