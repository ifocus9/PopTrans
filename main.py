"""
main.py — 选中翻译工具主入口

Windows 桌面翻译工具：选中任意文本，按下 Ctrl+Alt+Q 即可弹窗显示翻译结果。
使用 NLLB-200 + CTranslate2 离线翻译引擎，支持中英文互译。
"""

import sys
import os
import io
import logging
from logging.handlers import RotatingFileHandler
import threading
import ctypes
def _configure_tcl_tk():
    """Point Tkinter at bundled Tcl/Tk files when PyInstaller cannot auto-detect them."""
    if getattr(sys, 'frozen', False):
        base_dir = os.path.join(os.path.dirname(sys.executable), '_internal')
        candidates = [
            (os.path.join(base_dir, '_tcl_data'), os.path.join(base_dir, '_tk_data')),
            (os.path.join(base_dir, 'tcl8.6'), os.path.join(base_dir, 'tk8.6')),
        ]
    else:
        python_dir = os.path.dirname(sys.executable)
        candidates = [
            (os.path.join(python_dir, 'tcl', 'tcl8.6'), os.path.join(python_dir, 'tcl', 'tk8.6')),
        ]

    for tcl_dir, tk_dir in candidates:
        if os.path.exists(os.path.join(tcl_dir, 'init.tcl')) and os.path.exists(os.path.join(tk_dir, 'tk.tcl')):
            os.environ.setdefault('TCL_LIBRARY', tcl_dir)
            os.environ.setdefault('TK_LIBRARY', tk_dir)
            break


_configure_tcl_tk()

# 设置 DPI 感知，解决高 DPI 显示器上的模糊问题
try:
    ctypes.windll.shcore.SetProcessDpiAwareness(1)  # PROCESS_SYSTEM_DPI_AWARE
except Exception:
    try:
        ctypes.windll.user32.SetProcessDPIAware()
    except Exception:
        pass

import tkinter as tk

from translator import Translator
from hotkey_manager import HotkeyManager
from popup_window import TranslationPopup
from tray_icon import TrayIcon
from config_manager import load_config, get_hotkey, get_hotkey_display, set_hotkey
from settings_window import SettingsWindow

# ── 日志配置 ──────────────────────────────────────────────

LOG_FORMAT = "%(asctime)s [%(levelname)s] %(name)s: %(message)s"
LOG_MAX_BYTES = 10 * 1024 * 1024
LOG_BACKUP_COUNT = 5

# 支持 PyInstaller 打包：日志写到 exe 所在目录
if getattr(sys, 'frozen', False):
    _BASE_DIR = os.path.dirname(sys.executable)
else:
    _BASE_DIR = os.path.dirname(os.path.abspath(__file__))

LOG_FILE = os.path.join(_BASE_DIR, "translate.log")

# 使用 UTF-8 编码的控制台输出，避免 GBK 编码错误
# PyInstaller --windowed 模式下 stdout/stderr 为 None
if sys.stdout and hasattr(sys.stdout, 'buffer'):
    _utf8_stdout = io.TextIOWrapper(sys.stdout.buffer, encoding="utf-8", errors="replace")
    _stream_handler = logging.StreamHandler(_utf8_stdout)
else:
    _stream_handler = logging.StreamHandler(open(os.devnull, 'w', encoding='utf-8'))

logging.basicConfig(
    level=logging.INFO,
    format=LOG_FORMAT,
    handlers=[
        RotatingFileHandler(
            LOG_FILE,
            maxBytes=LOG_MAX_BYTES,
            backupCount=LOG_BACKUP_COUNT,
            encoding="utf-8",
        ),
        _stream_handler,
    ],
)
logger = logging.getLogger(__name__)


def _set_window_icon(window: tk.Tk) -> None:
    """Prefer the PNG asset at runtime and fall back to ICO on Windows."""
    png_icon_path = os.path.join(_BASE_DIR, "icon.png")
    if os.path.exists(png_icon_path):
        try:
            icon_image = tk.PhotoImage(file=png_icon_path)
            window.iconphoto(True, icon_image)
            window._icon_image_ref = icon_image
            return
        except Exception as e:
            logger.error(f"无法设置 PNG 窗口图标: {e}")

    ico_icon_path = os.path.join(_BASE_DIR, "icon.ico")
    if os.path.exists(ico_icon_path):
        try:
            window.iconbitmap(ico_icon_path)
        except Exception as e:
            logger.error(f"无法设置 ICO 窗口图标: {e}")


class TranslateApp:
    """选中翻译应用主控制器"""

    def __init__(self):
        logger.info("=" * 50)
        logger.info("选中翻译工具启动")
        logger.info("=" * 50)

        # ── 初始化 tkinter ──
        self.root = tk.Tk()
        self._main_thread_id = threading.get_ident()
        self._is_quitting = False
        self.root.withdraw()  # 隐藏主窗口
        self.root.title("选中翻译")
        _set_window_icon(self.root)

        # 设置字体渲染质量
        try:
            # 设置 DPI 缩放
            self.root.tk.call('tk', 'scaling', 1.0)
            # 启用字体平滑
            self.root.option_add('*Font', 'Microsoft-YaHei-UI 10')
        except Exception:
            pass

        # ── 加载配置 ──
        self.config = load_config()
        self.current_hotkey = get_hotkey()
        self.current_hotkey_display = get_hotkey_display()

        # ── 初始化各模块 ──
        self.translator = Translator()
        self.popup = TranslationPopup(self.root)
        self.hotkey_manager = HotkeyManager(on_translate=self._on_text_captured, hotkey=self.current_hotkey)
        self.tray = TrayIcon(on_quit=self._quit, on_settings=self._open_settings)

        # ── 启动翻译引擎（异步下载模型） ──
        self.translator.setup(
            on_ready=self._on_translator_ready,
            on_status=self._on_translator_status,
        )

        # ── 启动全局热键 ──
        self.hotkey_manager.start()

        # ── 启动系统托盘 ──
        self.tray.start()

        from tkinter import messagebox
        logger.info(f"所有模块初始化完成，快捷键: {self.current_hotkey_display}")
        logger.info("等待用户操作...")
        
        # 弹窗提示启动成功
        messagebox.showinfo("选中翻译工具", f"翻译工具已在后台启动！\n\n快捷键：{self.current_hotkey_display}\n请尝试选中文本后按下快捷键。")

        # ── 启动主循环 ──
        try:
            self.root.mainloop()
        except KeyboardInterrupt:
            logger.info("收到键盘中断，退出")
            self._quit()

    # ── 设置 ──────────────────────────────────────────────

    def _open_settings(self):
        """打开快捷键设置窗口"""
        def on_hotkey_saved(pynput_hotkey, display_hotkey):
            """快捷键保存回调"""
            logger.info(f"快捷键已更新: {pynput_hotkey} ({display_hotkey})")
            
            # 保存到配置文件
            if set_hotkey(pynput_hotkey, display_hotkey):
                # 更新当前快捷键
                self.current_hotkey = pynput_hotkey
                self.current_hotkey_display = display_hotkey
                
                # 重新注册热键
                self.hotkey_manager.stop()
                self.hotkey_manager = HotkeyManager(on_translate=self._on_text_captured, hotkey=pynput_hotkey)
                self.hotkey_manager.start()
                
                # 更新托盘菜单
                self.tray.update_hotkey_display(display_hotkey)
                
                logger.info(f"快捷键已重新注册: {display_hotkey}")
            else:
                logger.error("保存快捷键配置失败")
        
        # 打开设置窗口
        settings_window = SettingsWindow(self.root, on_saved=on_hotkey_saved)
        settings_window.show(self.current_hotkey_display)

    # ── 回调处理 ──────────────────────────────────────────

    def _on_translator_ready(self, success: bool):
        """翻译引擎初始化完成回调"""
        if success:
            logger.info("翻译引擎初始化成功，工具已就绪")
            self.tray.set_status("就绪")
        else:
            logger.error("翻译引擎初始化失败")
            self.tray.set_status("初始化失败")

    def _on_translator_status(self, message: str):
        """翻译引擎状态更新回调"""
        self.tray.set_status(message)

    def _on_text_captured(self, text: str):
        """
        热键捕获到选中文本后的回调。
        注意：此方法在 keyboard 线程中调用，需要通过 root.after 调度到主线程。
        """
        logger.info(f"捕获文本: {text[:80]}...")
        # 调度到 tkinter 主线程
        self.root.after(0, self._translate, text)

    def _translate(self, text: str):
        """在主线程中发起翻译"""
        # 先显示加载状态
        self.popup.show_loading(text)

        # 在后台线程执行翻译
        self.translator.translate_async(text, self._on_translation_done)

    def _on_translation_done(self, original: str, result: str, error: str):
        """
        翻译完成回调。
        注意：此方法在翻译线程中调用，需要调度到主线程更新 UI。
        """
        self.root.after(0, self._show_translation_result, original, result, error)

    def _show_translation_result(self, original: str, result: str, error: str):
        """在主线程中显示翻译结果"""
        if error:
            logger.warning(f"翻译失败: {error}")
            self.popup.update_to_error(original, error)
        else:
            logger.info(f"翻译完成: {result[:80]}...")
            self.popup.update_to_result(original, result)

    # ── 退出 ──────────────────────────────────────────────

    def _quit(self):
        """Safely exit the app from the Tk main thread."""
        if threading.get_ident() != self._main_thread_id:
            try:
                self.root.after(0, self._quit)
            except Exception:
                self._force_exit()
            return

        if self._is_quitting:
            return
        self._is_quitting = True
        logger.info("正在退出...")

        try:
            self.hotkey_manager.stop()
        except Exception as e:
            logger.warning(f"停止热键时出错: {e}")

        try:
            self.translator.close()
        except Exception as e:
            logger.warning(f"关闭翻译模型时出错: {e}")

        try:
            self.tray.stop()
        except Exception as e:
            logger.warning(f"停止托盘时出错: {e}")

        try:
            self.root.quit()
            self.root.destroy()
        except Exception as e:
            logger.warning(f"关闭窗口时出错: {e}")

        logger.info("已退出")
        self._force_exit()

    @staticmethod
    def _force_exit():
        """Flush logs, then make sure PyInstaller leaves no process behind."""
        # 释放互斥量
        global mutex_handle
        if 'mutex_handle' in globals() and mutex_handle:
            ctypes.windll.kernel32.CloseHandle(mutex_handle)
            mutex_handle = None
        
        logging.shutdown()
        os._exit(0)
# ── 防多开检查 ────────────────────────────────────────────

def check_single_instance():
    """检查是否已有实例在运行，防止程序多开"""
    import ctypes
    from ctypes import wintypes
    
    # 创建命名互斥量
    mutex_name = "TranslatePlugin_SingleInstance_Mutex"
    mutex = ctypes.windll.kernel32.CreateMutexW(None, False, mutex_name)
    
    # 检查是否已存在
    last_error = ctypes.windll.kernel32.GetLastError()
    ERROR_ALREADY_EXISTS = 183
    
    if last_error == ERROR_ALREADY_EXISTS:
        # 已有实例在运行
        ctypes.windll.kernel32.CloseHandle(mutex)
        
        # 弹窗提示
        try:
            from tkinter import messagebox
            root = tk.Tk()
            root.withdraw()
            messagebox.showwarning("选中翻译工具", "程序已在运行中！\n请检查系统托盘区域。")
            root.destroy()
        except:
            pass
        
        return False
    
    return mutex

# ── 入口 ──────────────────────────────────────────────────

if __name__ == "__main__":
    # 防多开检查
    mutex_handle = check_single_instance()
    if mutex_handle is False:
        sys.exit(0)
    
    try:
        TranslateApp()
    except Exception as e:
        # 捕获所有未处理异常，写入日志并弹窗提示
        import traceback
        error_msg = f"程序异常退出: {e}\n\n{traceback.format_exc()}"
        logger.exception("程序异常退出")
        
        # 尝试弹窗显示错误
        try:
            from tkinter import messagebox
            root = tk.Tk()
            root.withdraw()
            messagebox.showerror("选中翻译 - 错误", f"程序启动失败：\n\n{e}\n\n详见 translate.log")
            root.destroy()
        except:
            pass
        
        # 释放互斥量
        if 'mutex_handle' in globals() and mutex_handle:
            ctypes.windll.kernel32.CloseHandle(mutex_handle)




