"""
tray_icon.py — 系统托盘图标模块

使用 pystray 在 Windows 系统托盘显示图标，提供右键菜单进行管理。
"""

import logging

from PIL import Image, ImageDraw, ImageFont
import pystray
from pystray import MenuItem as Item

logger = logging.getLogger(__name__)


def _create_icon_image(size: int = 64) -> Image.Image:
    """
    动态生成托盘图标：紫色圆角矩形背景 + 白色 "译" 字。
    """
    img = Image.new("RGBA", (size, size), (0, 0, 0, 0))
    draw = ImageDraw.Draw(img)

    # 绘制圆角矩形背景
    margin = 4
    draw.rounded_rectangle(
        [margin, margin, size - margin, size - margin],
        radius=size // 5,
        fill=(139, 92, 246),  # #8b5cf6
    )

    # 尝试使用系统中文字体绘制 "译" 字
    label = "译"
    font_size = size // 2
    font = None

    # 按优先级尝试中文字体
    font_candidates = [
        "msyh.ttc",         # 微软雅黑
        "msyhbd.ttc",       # 微软雅黑 Bold
        "simhei.ttf",       # 黑体
        "simsun.ttc",       # 宋体
    ]
    for font_name in font_candidates:
        try:
            font = ImageFont.truetype(font_name, font_size)
            break
        except (OSError, IOError):
            continue

    if font is None:
        # 回退：使用默认字体，显示 "T" 代替
        try:
            font = ImageFont.truetype("arial.ttf", font_size)
        except (OSError, IOError):
            font = ImageFont.load_default()
        label = "T"

    # 居中绘制文字
    bbox = draw.textbbox((0, 0), label, font=font)
    text_w = bbox[2] - bbox[0]
    text_h = bbox[3] - bbox[1]
    text_x = (size - text_w) // 2
    text_y = (size - text_h) // 2 - bbox[1]  # 修正 baseline 偏移
    draw.text((text_x, text_y), label, fill="white", font=font)

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


