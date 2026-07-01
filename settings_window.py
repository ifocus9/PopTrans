"""
settings_window.py - 快捷键设置对话框

深色主题模态窗口，用户按下任意组合键后自动捕获并保存。
"""

import tkinter as tk
import logging
from typing import Callable, Optional, Set

logger = logging.getLogger(__name__)

# 配色（与 popup_window 一致）
COLORS = {
    "bg":           "#111115",
    "header_bg":    "#1A1A21",
    "border":       "#2D2D3A",
    "text_muted":   "#9CA3AF",
    "text_result":  "#F9FAFB",
    "accent":       "#8B5CF6",
    "accent_hover": "#A78BFA",
    "divider":      "#262631",
    "btn_bg":       "#374151",
    "btn_hover":    "#4B5563",
    "btn_primary":  "#4C1D95",
    "btn_primary_hover": "#5B21B6",
    "success_text": "#34D399",
    "error_text":   "#F87171",
    "close_normal": "#6B7280",
    "close_hover":  "#EF4444",
}

FONT_FAMILY = "Microsoft YaHei UI"

# 修饰键 -> pynput 名称
_MODIFIER_TO_PYNPUT = {
    "control_l": "<ctrl>",  "control_r": "<ctrl>",  "ctrl": "<ctrl>",
    "alt_l": "<alt>",       "alt_r": "<alt>",       "alt": "<alt>",  "menu": "<alt>",
    "shift_l": "<shift>",   "shift_r": "<shift>",   "shift": "<shift>",
    "super_l": "<cmd>",     "super_r": "<cmd>",     "win": "<cmd>",  "super": "<cmd>",
}

# 修饰键 -> 显示名
_MODIFIER_TO_DISPLAY = {
    "control_l": "Ctrl",  "control_r": "Ctrl",  "ctrl": "Ctrl",
    "alt_l": "Alt",       "alt_r": "Alt",       "alt": "Alt",  "menu": "Alt",
    "shift_l": "Shift",   "shift_r": "Shift",   "shift": "Shift",
    "super_l": "Win",     "super_r": "Win",     "win": "Win",  "super": "Win",
}

# 特殊键 -> 显示名
_SPECIAL_KEY_DISPLAY = {
    "space": "Space", "return": "Enter", "escape": "Esc",
    "tab": "Tab", "backspace": "Backspace", "delete": "Delete",
    "insert": "Insert", "home": "Home", "end": "End",
    "page_up": "PageUp", "page_down": "PageDown",
    "up": "Up", "down": "Down", "left": "Left", "right": "Right",
    "f1": "F1", "f2": "F2", "f3": "F3", "f4": "F4",
    "f5": "F5", "f6": "F6", "f7": "F7", "f8": "F8",
    "f9": "F9", "f10": "F10", "f11": "F11", "f12": "F12",
}


class SettingsWindow:
    """快捷键设置对话框"""

    def __init__(self, root: tk.Tk, on_saved: Optional[Callable] = None):
        self.root = root
        self.on_saved = on_saved
        self.window: Optional[tk.Toplevel] = None
        self._pressed_modifiers: Set[str] = set()
        self._current_display = ""
        self._display_label: Optional[tk.Label] = None
        self._hint_label: Optional[tk.Label] = None
        self._save_btn: Optional[tk.Label] = None
        self._reset_btn: Optional[tk.Label] = None
        self._captured_pynput = ""
        self._captured_display = ""
        self._is_listening = False
        self._blink_after_id = None
        self._blink_state = False
        # OCR 相关
        self._capture_target = "main"  # "main" 或 "ocr"，标记当前捕获目标
        self._ocr_enabled_var: Optional[tk.BooleanVar] = None
        self._ocr_display_label: Optional[tk.Label] = None
        self._ocr_hint_label: Optional[tk.Label] = None
        self._ocr_capture_frame: Optional[tk.Frame] = None
        self._main_capture_frame: Optional[tk.Frame] = None
        self._ocr_current_display = ""
        self._ocr_captured_pynput = ""
        self._ocr_captured_display = ""

    def show(self, current_display: str = "Ctrl+Alt+Q",
             ocr_enabled: bool = False, ocr_current_display: str = "Ctrl+Alt+E"):
        """显示设置窗口"""
        if self.window and self.window.winfo_exists():
            self.window.lift()
            return

        self._current_display = current_display
        self._ocr_current_display = ocr_current_display
        self._captured_pynput = ""
        self._captured_display = ""
        self._ocr_captured_pynput = ""
        self._ocr_captured_display = ""
        self._pressed_modifiers = set()
        self._capture_target = "main"

        win = tk.Toplevel(self.root)
        self.window = win
        win.title("快捷键设置")
        win.overrideredirect(True)
        win.resizable(False, False)
        win.attributes("-topmost", True)
        win.configure(bg=COLORS["border"])
        
        # 设置字体渲染质量
        try:
            # 启用 ClearType 字体渲染
            win.tk.call('tk', 'scaling', 1.0)
        except Exception:
            pass

        # 内容区域
        content = tk.Frame(win, bg=COLORS["bg"], padx=0, pady=0)
        content.pack(fill="both", expand=True, padx=1, pady=1)

        # 标题栏
        header = tk.Frame(content, bg=COLORS["header_bg"], height=42)
        header.pack(fill="x")
        header.pack_propagate(False)

        accent_bar = tk.Frame(header, bg=COLORS["accent"], width=3)
        accent_bar.pack(side="left", fill="y")

        title_label = tk.Label(
            header, text="快捷键设置",
            font=(FONT_FAMILY, 15, "bold"),
            fg=COLORS["text_result"], bg=COLORS["header_bg"],
        )
        title_label.pack(side="left", padx=(12, 0))

        # 绑定拖拽
        header.bind("<Button-1>", self._start_drag)
        header.bind("<B1-Motion>", self._on_drag)
        title_label.bind("<Button-1>", self._start_drag)
        title_label.bind("<B1-Motion>", self._on_drag)

        close_btn = tk.Label(
            header, text="\u2715",
            font=(FONT_FAMILY, 14),
            fg=COLORS["close_normal"], bg=COLORS["header_bg"],
            cursor="hand2", padx=14,
        )
        close_btn.pack(side="right", fill="y")
        close_btn.bind("<Enter>", lambda e: close_btn.config(fg=COLORS["close_hover"]))
        close_btn.bind("<Leave>", lambda e: close_btn.config(fg=COLORS["close_normal"]))
        close_btn.bind("<Button-1>", lambda e: self._close())

        # 当前快捷键
        tk.Label(
            content, text=f"当前：{current_display}",
            font=(FONT_FAMILY, 14),
            fg=COLORS["text_muted"], bg=COLORS["bg"],
        ).pack(pady=(18, 4), padx=20, anchor="w")

        # 分隔线
        tk.Frame(content, bg=COLORS["divider"], height=1).pack(fill="x", padx=20, pady=(10, 0))

        # 翻译快捷键捕获区
        self._main_capture_frame = tk.Frame(content, bg=COLORS["bg"], highlightbackground=COLORS["accent"], highlightthickness=2)
        self._main_capture_frame.pack(fill="x", padx=20, pady=(12, 0))

        capture_inner = tk.Frame(self._main_capture_frame, bg=COLORS["bg"], padx=12, pady=8)
        capture_inner.pack(fill="x")

        self._display_label = tk.Label(
            capture_inner,
            text="按下新的快捷键...",
            font=(FONT_FAMILY, 20, "bold"),
            fg=COLORS["accent"],
            bg=COLORS["bg"],
            height=2,
            cursor="hand2",
        )
        self._display_label.pack(fill="x")
        self._display_label.bind("<Button-1>", lambda e: self._switch_capture_target("main"))

        self._hint_label = tk.Label(
            capture_inner,
            text="请按下包含修饰键的组合键（如 Ctrl+Shift+T）",
            font=(FONT_FAMILY, 12),
            fg=COLORS["text_muted"],
            bg=COLORS["bg"],
            cursor="hand2",
        )
        self._hint_label.pack(fill="x", pady=(4, 0))
        self._hint_label.bind("<Button-1>", lambda e: self._switch_capture_target("main"))

        # ── OCR 翻译分区 ──
        tk.Frame(content, bg=COLORS["divider"], height=1).pack(fill="x", padx=20, pady=(14, 0))

        ocr_header = tk.Frame(content, bg=COLORS["bg"])
        ocr_header.pack(fill="x", padx=20, pady=(10, 0))

        self._ocr_enabled_var = tk.BooleanVar(value=ocr_enabled)
        ocr_check = tk.Checkbutton(
            ocr_header,
            text="启用 OCR 翻译",
            font=(FONT_FAMILY, 14),
            fg=COLORS["text_result"],
            bg=COLORS["bg"],
            selectcolor=COLORS["header_bg"],
            activebackground=COLORS["bg"],
            activeforeground=COLORS["text_result"],
            variable=self._ocr_enabled_var,
            command=self._on_ocr_toggle,
        )
        ocr_check.pack(anchor="w")

        tk.Label(
            content, text=f"当前：{ocr_current_display}",
            font=(FONT_FAMILY, 13),
            fg=COLORS["text_muted"], bg=COLORS["bg"],
        ).pack(pady=(4, 0), padx=20, anchor="w")

        # OCR 快捷键捕获区
        self._ocr_capture_frame = tk.Frame(content, bg=COLORS["bg"], highlightbackground=COLORS["border"], highlightthickness=1)
        self._ocr_capture_frame.pack(fill="x", padx=20, pady=(6, 0))

        ocr_inner = tk.Frame(self._ocr_capture_frame, bg=COLORS["bg"], padx=12, pady=8)
        ocr_inner.pack(fill="x")

        self._ocr_display_label = tk.Label(
            ocr_inner,
            text="按下 OCR 快捷键...",
            font=(FONT_FAMILY, 20, "bold"),
            fg=COLORS["text_muted"],
            bg=COLORS["bg"],
            height=2,
            cursor="hand2",
        )
        self._ocr_display_label.pack(fill="x")
        self._ocr_display_label.bind("<Button-1>", lambda e: self._switch_capture_target("ocr"))

        self._ocr_hint_label = tk.Label(
            ocr_inner,
            text="先勾选上方开关，再按下组合键",
            font=(FONT_FAMILY, 12),
            fg=COLORS["text_muted"],
            bg=COLORS["bg"],
            cursor="hand2",
        )
        self._ocr_hint_label.pack(fill="x", pady=(4, 0))
        self._ocr_hint_label.bind("<Button-1>", lambda e: self._switch_capture_target("ocr"))

        # 根据初始状态调整 OCR 捕获区外观
        self._on_ocr_toggle()

        # 分隔线
        tk.Frame(content, bg=COLORS["divider"], height=1).pack(fill="x", padx=20, pady=(14, 0))

        # 按钮区
        btn_frame = tk.Frame(content, bg=COLORS["bg"])
        btn_frame.pack(fill="x", padx=20, pady=(14, 18))

        # 保存按钮
        self._save_btn = tk.Label(
            btn_frame, text="  保存  ",
            font=(FONT_FAMILY, 14),
            fg=COLORS["text_result"], bg=COLORS["btn_primary"],
            cursor="hand2", padx=24, pady=8,
        )
        self._save_btn.pack(side="right")
        self._save_btn.bind("<Button-1>", lambda e: self._save())
        self._save_btn.bind("<Enter>", lambda e: self._save_btn.config(bg=COLORS["btn_primary_hover"]))
        self._save_btn.bind("<Leave>", lambda e: self._save_btn.config(bg=COLORS["btn_primary"]))
        self._save_btn.config(state="disabled")

        # 恢复默认按钮
        self._reset_btn = tk.Label(
            btn_frame, text="恢复默认",
            font=(FONT_FAMILY, 14),
            fg=COLORS["text_result"], bg=COLORS["btn_bg"],
            cursor="hand2", padx=18, pady=8,
        )
        self._reset_btn.pack(side="right", padx=(0, 12))
        self._reset_btn.bind("<Button-1>", lambda e: self._reset_to_default())
        self._reset_btn.bind("<Enter>", lambda e: self._reset_btn.config(bg=COLORS["btn_hover"]))
        self._reset_btn.bind("<Leave>", lambda e: self._reset_btn.config(bg=COLORS["btn_bg"]))

        # 绑定键盘事件
        self._is_listening = True
        win.bind("<KeyPress>", self._on_key_press)
        win.bind("<KeyRelease>", self._on_key_release)
        win.bind("<Escape>", lambda e: self._close())

        # 窗口关闭协议
        win.protocol("WM_DELETE_WINDOW", self._close)

        # 居中
        win.update_idletasks()
        w, h = 460, 500
        x = (win.winfo_screenwidth() - w) // 2
        y = (win.winfo_screenheight() - h) // 2
        win.geometry(f"{w}x{h}+{x}+{y}")

        win.focus_force()
        self._start_blink()

    # ── 捕获目标切换 ──────────────────────────────────

    def _switch_capture_target(self, target: str):
        """切换当前键盘捕获目标（main / ocr）"""
        if target == "ocr" and not self._ocr_enabled_var.get():
            # OCR 未启用时不允许切换到 OCR 捕获，回退到 main
            self._capture_target = "main"
        else:
            self._capture_target = target
        self._update_capture_highlight()

    def _update_capture_highlight(self):
        """高亮当前活跃的捕获区，灰化非活跃区"""
        if not self._main_capture_frame or not self._ocr_capture_frame:
            return
        if self._capture_target == "main":
            self._main_capture_frame.config(highlightbackground=COLORS["accent"], highlightthickness=2)
            self._ocr_capture_frame.config(highlightbackground=COLORS["border"], highlightthickness=1)
        else:
            self._main_capture_frame.config(highlightbackground=COLORS["border"], highlightthickness=1)
            self._ocr_capture_frame.config(highlightbackground=COLORS["accent"], highlightthickness=2)

    def _on_ocr_toggle(self):
        """OCR 开关切换时的 UI 联动"""
        enabled = self._ocr_enabled_var.get()
        if not enabled:
            # 关闭时切回主捕获区
            self._capture_target = "main"
            self._update_capture_highlight()
            # 灰化 OCR 捕获区
            if self._ocr_display_label:
                self._ocr_display_label.config(fg=COLORS["text_muted"])
        else:
            if self._ocr_display_label:
                self._ocr_display_label.config(fg=COLORS["accent"])

    # ── 拖拽事件 ──────────────────────────────────────

    def _start_drag(self, event):
        self._drag_start_x = event.x
        self._drag_start_y = event.y

    def _on_drag(self, event):
        if not self.window:
            return
        x = self.window.winfo_x() + event.x - self._drag_start_x
        y = self.window.winfo_y() + event.y - self._drag_start_y
        self.window.geometry(f"+{x}+{y}")

    # ── 键盘事件 ──────────────────────────────────────

    def _on_key_press(self, event):
        if not self._is_listening:
            return

        # OCR 未启用时强制捕获目标为 main
        if self._capture_target == "ocr" and not self._ocr_enabled_var.get():
            self._capture_target = "main"

        keysym = event.keysym.lower()
        logger.debug(f"KeyPress: keysym={keysym}, keycode={event.keycode}, target={self._capture_target}")

        # 根据捕获目标选择对应的 label
        target_label = self._ocr_display_label if self._capture_target == "ocr" else self._display_label

        # 判断是否为修饰键
        if keysym in _MODIFIER_TO_PYNPUT:
            self._pressed_modifiers.add(keysym)
            # 实时显示当前按下的修饰键
            parts = []
            seen = set()
            for mod in sorted(self._pressed_modifiers):
                disp = _MODIFIER_TO_DISPLAY.get(mod, "")
                if disp and disp not in seen:
                    parts.append(disp)
                    seen.add(disp)
            if parts and target_label:
                target_label.config(text=" + ".join(parts) + " + ...", fg=COLORS["accent"])
            return

        # 非修饰键：组合成快捷键
        if not self._pressed_modifiers:
            if target_label:
                target_label.config(text="需要包含修饰键！", fg=COLORS["error_text"])
            self.root.after(1500, lambda: self._reset_display(self._capture_target))
            return

        # 构建 pynput 格式
        pynput_parts = []
        display_parts = []
        seen_pynput = set()
        seen_display = set()

        for mod in sorted(self._pressed_modifiers):
            p = _MODIFIER_TO_PYNPUT.get(mod)
            d = _MODIFIER_TO_DISPLAY.get(mod)
            if p and p not in seen_pynput:
                pynput_parts.append(p)
                seen_pynput.add(p)
            if d and d not in seen_display:
                display_parts.append(d)
                seen_display.add(d)

        # 主键
        main_key = keysym.lower()
        if main_key in _SPECIAL_KEY_DISPLAY:
            display_parts.append(_SPECIAL_KEY_DISPLAY[main_key])
            pynput_parts.append(main_key)
        elif len(main_key) == 1:
            display_parts.append(main_key.upper())
            pynput_parts.append(main_key)
        else:
            display_parts.append(main_key)
            pynput_parts.append(main_key)

        captured_pynput = "+".join(pynput_parts)
        captured_display = "+".join(display_parts)

        # 根据捕获目标存储
        if self._capture_target == "ocr":
            self._ocr_captured_pynput = captured_pynput
            self._ocr_captured_display = captured_display
        else:
            self._captured_pynput = captured_pynput
            self._captured_display = captured_display

        self._stop_blink()
        if target_label:
            target_label.config(text=captured_display, fg=COLORS["success_text"])
        self._save_btn.config(state="normal")
        logger.info(f"捕获快捷键 [{self._capture_target}]: {captured_pynput} ({captured_display})")

    def _on_key_release(self, event):
        if not self._is_listening:
            return
        keysym = event.keysym.lower()
        self._pressed_modifiers.discard(keysym)

    # ── 显示更新 ──────────────────────────────────────

    def _update_display(self, text: str, waiting: bool = False, error: bool = False, success: bool = False):
        if not self._display_label:
            return
        if error:
            fg = COLORS["error_text"]
        elif success:
            fg = COLORS["success_text"]
        elif waiting:
            fg = COLORS["accent"]
        else:
            fg = COLORS["text_muted"]
        self._display_label.config(text=text, fg=fg)

    def _reset_display(self, target: str = "main"):
        """重置指定捕获区的显示"""
        self._pressed_modifiers.clear()
        if target == "ocr":
            self._ocr_captured_pynput = ""
            self._ocr_captured_display = ""
            label = self._ocr_display_label
            placeholder = "按下 OCR 快捷键..."
        else:
            self._captured_pynput = ""
            self._captured_display = ""
            label = self._display_label
            placeholder = "按下新的快捷键..."

        # 只要有任一捕获区有结果就保持保存按钮可用
        has_result = bool(self._captured_pynput or self._ocr_captured_pynput)
        self._save_btn.config(state="normal" if has_result else "disabled")

        if label:
            fg = COLORS["accent"] if (target == "ocr" and self._ocr_enabled_var.get()) else COLORS["accent"]
            label.config(text=placeholder, fg=fg)
        self._start_blink()

    # ── 闪烁动画 ──────────────────────────────────────

    def _start_blink(self):
        self._stop_blink()
        self._do_blink()

    def _do_blink(self):
        if not self.window or not self.window.winfo_exists():
            return
        if not self._is_listening:
            return
        self._blink_state = not self._blink_state
        fg = COLORS["accent"] if self._blink_state else COLORS["text_muted"]
        try:
            if self._hint_label:
                self._hint_label.config(fg=fg)
        except tk.TclError:
            return
        self._blink_after_id = self.root.after(500, self._do_blink)

    def _stop_blink(self):
        if self._blink_after_id:
            self.root.after_cancel(self._blink_after_id)
            self._blink_after_id = None
        try:
            if self._hint_label:
                self._hint_label.config(fg=COLORS["text_muted"])
        except tk.TclError:
            pass

    # ── 操作 ──────────────────────────────────────────

    def _save(self):
        # 至少有一个捕获区有结果才能保存
        has_main = bool(self._captured_pynput)
        has_ocr = bool(self._ocr_captured_pynput) and self._ocr_enabled_var.get()
        if not has_main and not has_ocr:
            return

        ocr_enabled = self._ocr_enabled_var.get()
        # OCR 快捷键：如果用户没改但勾选了开关，用当前值
        ocr_pynput = self._ocr_captured_pynput or ""
        ocr_display = self._ocr_captured_display or ""

        logger.info(
            f"保存设置: 翻译热键={self._captured_pynput or '(未改)'}, "
            f"OCR enabled={ocr_enabled}, OCR 热键={ocr_pynput or '(未改)'}"
        )
        if self.on_saved:
            self.on_saved(
                self._captured_pynput,
                self._captured_display,
                ocr_enabled,
                ocr_pynput,
                ocr_display,
            )
        self._close()

    def _reset_to_default(self):
        from config_manager import DEFAULT_CONFIG
        self._captured_pynput = DEFAULT_CONFIG["hotkey"]
        self._captured_display = DEFAULT_CONFIG["hotkey_display"]
        self._ocr_captured_pynput = DEFAULT_CONFIG["ocr_hotkey"]
        self._ocr_captured_display = DEFAULT_CONFIG["ocr_hotkey_display"]
        self._stop_blink()
        if self._display_label:
            self._display_label.config(text=f"{self._captured_display}（默认）", fg=COLORS["success_text"])
        if self._ocr_display_label:
            self._ocr_display_label.config(text=f"{self._ocr_captured_display}（默认）", fg=COLORS["success_text"])
        self._save_btn.config(state="normal")
        logger.info("恢复默认快捷键（含 OCR）")

    def _close(self):
        self._is_listening = False
        self._stop_blink()
        if self.window and self.window.winfo_exists():
            try:
                self.window.destroy()
            except Exception:
                pass
        self.window = None
