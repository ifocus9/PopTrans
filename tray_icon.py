"""
tray_icon.py — 系统托盘图标模块

使用 pystray 在 Windows 系统托盘显示图标，提供右键菜单进行管理。
"""

import logging

from PIL import Image, ImageDraw, ImageFont
import pystray
from pystray import MenuItem as Item

logger = logging.getLogger(__name__)


import os

def _create_icon_image(size: int = 64) -> Image.Image:
    """
    加载静态托盘图标
    """
    icon_path = os.path.join(os.path.dirname(os.path.abspath(__file__)), "icon.png")
    if os.path.exists(icon_path):
        try:
            img = Image.open(icon_path)
            # Ensure it's RGBA
            if img.mode != 'RGBA':
                img = img.convert('RGBA')
            return img.resize((size, size), Image.Resampling.LANCZOS)
        except Exception as e:
            logger.error(f"无法加载图标文件 {icon_path}: {e}")
    
    # 回退到简单的绘制
    img = Image.new("RGBA", (size, size), (139, 92, 246, 255))
    return img


class TrayIcon:
    """系统托盘图标管理器"""

    def __init__(self, on_quit, on_settings=None, on_toggle_hotkey=None):
        """
        Args:
            on_quit: 退出应用的回调
            on_settings: 打开设置窗口的回调（可选）
            on_toggle_hotkey: 启用/禁用热键的回调（可选）
        """
        self.on_quit = on_quit
        self.on_settings = on_settings
        self.on_toggle_hotkey = on_toggle_hotkey
        self._icon = None
        self._status_text = "正在初始化..."
        self._hotkey_display = "Ctrl+Alt+Q"

    def _create_menu(self):
        """创建菜单（每次调用生成新菜单）"""
        return pystray.Menu(
            Item(
                "选中翻译 v1.0",
                None,
                enabled=False,
            ),
            pystray.Menu.SEPARATOR,
            Item(
                f"状态: {self._status_text}",
                None,
                enabled=False,
            ),
            Item(
                f"快捷键: {self._hotkey_display}",
                None,
                enabled=False,
            ),
            Item(
                "设置快捷键",
                self._on_settings_clicked,
            ),
            pystray.Menu.SEPARATOR,
            Item(
                "退出",
                self._on_quit_clicked,
            ),
        )

    def set_status(self, status: str):
        """更新托盘提示文字和菜单"""
        self._status_text = status
        if self._icon:
            self._icon.title = f"选中翻译 — {status}"
            self._icon.menu = self._create_menu()

    def update_hotkey_display(self, hotkey_display: str):
        """更新快捷键显示"""
        self._hotkey_display = hotkey_display
        if self._icon:
            self._icon.menu = self._create_menu()

    def start(self):
        """启动托盘图标"""
        icon_image = _create_icon_image()

        self._icon = pystray.Icon(
            name="translate-plugin",
            icon=icon_image,
            title=f"选中翻译 - {self._status_text}",
            menu=self._create_menu(),
        )

        # 使用 run_detached 在后台运行，比手动线程更可靠
        self._icon.run_detached()
        logger.info("系统托盘图标已启动")

    def stop(self):
        """停止托盘图标"""
        if self._icon:
            try:
                self._icon.stop()
            except Exception as e:
                logger.warning(f"停止托盘图标时出错: {e}")

    def _on_settings_clicked(self, icon, item):
        """处理设置按钮点击"""
        logger.info("用户通过托盘打开设置窗口")
        if self.on_settings:
            self.on_settings()

    def _on_quit_clicked(self, icon, item):
        """处理退出按钮点击"""
        logger.info("用户通过托盘退出应用")
        if self.on_quit:
            self.on_quit()


