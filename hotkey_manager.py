"""
hotkey_manager.py — 全局热键管理模块

使用 pynput 监听全局快捷键（默认 Ctrl+Alt+Q），获取选中文本并触发翻译。
pynput 不需要管理员权限即可在 Windows 上捕获全局按键。
通过模拟 Ctrl+C 获取选中文本，翻译完成后恢复原始剪贴板内容。
"""

import time
import threading
import logging
import ctypes
import ctypes.wintypes
import uuid

from pynput import keyboard
import pyperclip

logger = logging.getLogger(__name__)


class HotkeyManager:
    """全局热键管理器（基于 pynput）"""

    DEFAULT_HOTKEY = "<ctrl>+<alt>+q"
    DEFAULT_OCR_HOTKEY = "<ctrl>+<alt>+e"

    def __init__(self, on_translate, hotkey: str = None, on_ocr=None, ocr_hotkey: str = None):
        """
        Args:
            on_translate: 获取到选中文本后的回调 callback(text: str)
            hotkey: 自定义快捷键，默认为 Ctrl+Alt+Q
            on_ocr: OCR 触发回调 callback()，不传则不注册 OCR 热键
            ocr_hotkey: OCR 快捷键，默认为 Ctrl+Alt+E
        """
        self.on_translate = on_translate
        self.hotkey = hotkey or self.DEFAULT_HOTKEY
        self.on_ocr = on_ocr
        self.ocr_hotkey = ocr_hotkey or self.DEFAULT_OCR_HOTKEY
        self._active = False
        self._lock = threading.Lock()
        self._hotkey_listener = None

    def _build_hotkey_map(self):
        """根据当前配置构建热键字典（单 listener 管理所有热键，避免双 listener 冲突）"""
        mapping = {self.hotkey: self._on_hotkey_pressed}
        if self.on_ocr and self.ocr_hotkey:
            mapping[self.ocr_hotkey] = self._on_ocr_hotkey_pressed
        return mapping

    def start(self):
        """注册全局热键并开始监听"""
        if self._active:
            return

        self._start_listener()
        self._active = True
        logger.info(f"全局热键已注册: {', '.join(self._build_hotkey_map().keys())}")

    def _start_listener(self):
        """创建并启动全局热键 listener（单实例管理所有热键）"""
        self._hotkey_listener = keyboard.GlobalHotKeys(self._build_hotkey_map())
        self._hotkey_listener.daemon = True
        self._hotkey_listener.start()

    def _restart_listener(self):
        """重启 listener（热键映射变化时调用）"""
        if self._hotkey_listener:
            try:
                self._hotkey_listener.stop()
            except Exception as e:
                logger.warning(f"停止热键 listener 时出错: {e}")
            self._hotkey_listener = None
        self._start_listener()

    def register_ocr_hotkey(self, on_ocr, hotkey: str):
        """动态注册 OCR 热键（重启 listener 以包含新热键）"""
        self.on_ocr = on_ocr
        self.ocr_hotkey = hotkey
        self._restart_listener()
        logger.info(f"OCR 热键已注册: {hotkey}")

    def unregister_ocr_hotkey(self):
        """注销 OCR 热键（重启 listener 以移除 OCR 热键）"""
        self.on_ocr = None
        self._restart_listener()
        logger.info("OCR 热键已注销")

    def stop(self):
        """停止监听并注销所有热键"""
        if not self._active:
            return

        try:
            if self._hotkey_listener:
                self._hotkey_listener.stop()
                self._hotkey_listener = None
        except Exception as e:
            logger.warning(f"注销热键时出错: {e}")

        self.on_ocr = None
        self._active = False
        logger.info("全局热键已注销")

    def _on_ocr_hotkey_pressed(self):
        """OCR 热键按下：直接触发回调（无需模拟 Ctrl+C）"""
        # 使用与主翻译热键相同的锁，防止两个热键同时触发
        # （Ctrl+Alt+Q 和 Ctrl+Alt+E 共享 Ctrl+Alt 修饰键，pynput 可能同时回调）
        if not self._lock.acquire(blocking=False):
            return
        try:
            if self.on_ocr:
                logger.info("OCR 热键触发")
                self.on_ocr()
        except Exception as e:
            logger.error(f"OCR 热键处理出错: {e}")
        finally:
            self._lock.release()

    def _on_hotkey_pressed(self):
        """
        热键按下时的处理流程：
        1. 保存当前剪贴板
        2. 模拟 Ctrl+C 复制选中内容
        3. 读取剪贴板获取选中文本
        4. 恢复原始剪贴板
        5. 触发翻译回调
        """
        # 使用锁防止快速重复按键导致的竞争
        if not self._lock.acquire(blocking=False):
            return

        try:
            logger.info("热键触发，正在捕获选中文本...")
            self._capture_and_translate()
        except Exception as e:
            logger.error(f"热键处理出错: {e}")
        finally:
            self._lock.release()

    def _capture_and_translate(self):
        """捕获选中文本并触发翻译"""
        # Step 1: 保存当前剪贴板内容
        original_clipboard = ""
        try:
            original_clipboard = pyperclip.paste()
        except Exception:
            logger.debug("无法读取剪贴板内容")

        # Step 2: 写入哨兵值，后续用它判断 Ctrl+C 是否真的更新了剪贴板
        sentinel = f"__translate_plugin_clipboard_sentinel_{uuid.uuid4()}__"
        try:
            pyperclip.copy(sentinel)
        except Exception as e:
            logger.debug(f"无法准备剪贴板: {e}")
            return

        # Step 3: 模拟 Ctrl+C 复制选中内容
        time.sleep(0.15)
        self._simulate_ctrl_c()

        # Step 4: 等待剪贴板更新
        selected_text = ""
        deadline = time.time() + 0.8
        while time.time() < deadline:
            try:
                selected_text = pyperclip.paste()
            except Exception:
                logger.debug("无法读取选中文本")
                break

            if selected_text != sentinel:
                break

            time.sleep(0.05)

        # Step 5: 恢复原始剪贴板
        try:
            pyperclip.copy(original_clipboard)
        except Exception:
            logger.debug("无法恢复剪贴板")

        # Step 6: 触发翻译
        if selected_text and selected_text != sentinel and selected_text.strip():
            text = selected_text.strip()
            logger.info(f"捕获选中文本: {text[:50]}...")
            self.on_translate(text)
        else:
            logger.warning("未检测到选中文本，请确认当前应用支持 Ctrl+C 复制选中内容")

    @staticmethod
    def _simulate_ctrl_c():
        """使用 Windows API 模拟 Ctrl+C 按键"""
        VK_CONTROL = 0x11
        VK_MENU = 0x12
        VK_SHIFT = 0x10
        VK_C = 0x43
        KEYEVENTF_KEYUP = 0x0002

        user32 = ctypes.windll.user32

        # 热键触发时 Ctrl/Alt 可能仍处于按下状态；先释放修饰键，避免发送成 Ctrl+Alt+C。
        user32.keybd_event(VK_MENU, 0, KEYEVENTF_KEYUP, 0)
        user32.keybd_event(VK_CONTROL, 0, KEYEVENTF_KEYUP, 0)
        user32.keybd_event(VK_SHIFT, 0, KEYEVENTF_KEYUP, 0)
        time.sleep(0.03)

        # 按下 Ctrl
        user32.keybd_event(VK_CONTROL, 0, 0, 0)
        # 按下 C
        user32.keybd_event(VK_C, 0, 0, 0)
        # 释放 C
        user32.keybd_event(VK_C, 0, KEYEVENTF_KEYUP, 0)
        # 释放 Ctrl
        user32.keybd_event(VK_CONTROL, 0, KEYEVENTF_KEYUP, 0)
