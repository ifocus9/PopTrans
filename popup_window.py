"""
popup_window.py — 悬浮翻译窗口模块

使用 tkinter 实现的现代风格悬浮窗口，用于显示翻译结果。
深色主题、半透明效果、支持一键复制和快捷关闭。
"""

import tkinter as tk
import tkinter.font as tkfont
import logging

logger = logging.getLogger(__name__)

# ── 主题配色 ──────────────────────────────────────────────
COLORS = {
    "bg":             "#111115",    # Deep dark gray
    "header_bg":      "#1A1A21",    # Card/Header
    "border":         "#2D2D3A",    # Subdued border
    "text_muted":     "#9CA3AF",    # Secondary text
    "text_source":    "#E5E7EB",    # Source text
    "text_result":    "#F9FAFB",    # Translation text (white)
    "accent":         "#8B5CF6",    # Vibrant violet
    "accent_hover":   "#A78BFA",    # Violet hover
    "accent_glow":    "#7C3AED",    # Violet glow
    "divider":        "#262631",    # Divider
    "btn_copy_bg":    "#4C1D95",    # Dark violet btn
    "btn_copy_hover": "#5B21B6",    
    "btn_copy_active":"#6D28D9",    
    "close_normal":   "#6B7280",    
    "close_hover":    "#EF4444",    
    "loading_dot":    "#8B5CF6",    
    "error_text":     "#F87171",    
    "error_bg":       "#450A0A",    
    "success_text":   "#34D399",    
    "tag_bg":         "#374151",    
    "tag_text":       "#D1D5DB",    
    "shadow":         "#000000",    
}

# 字体配置
FONT_FAMILY = "Microsoft YaHei UI"
FONT_FALLBACK = "Segoe UI"
FONT_SIZE_LABEL = 12
FONT_SIZE_TEXT = 15
FONT_SIZE_TITLE = 14
FONT_SIZE_BTN = 13

# 窗口配置
WINDOW_WIDTH = 420
WINDOW_MIN_HEIGHT = 180
WINDOW_MAX_HEIGHT_RATIO = 0.72  # 超过屏幕高度的 72% 后使用滚动条
WINDOW_ALPHA = 0.96
WINDOW_PADDING = 24
FADE_STEP = 0.05
FADE_INTERVAL = 10  # ms
CORNER_RADIUS = 8   # 圆角半径（视觉参考）


class TranslationPopup:
    """翻译结果悬浮窗口"""

    def __init__(self, root: tk.Tk):
        self.root = root
        self.window = None
        self._fade_alpha = 0.0
        self._loading_dots = 0
        self._loading_after_id = None
        self._auto_close_after_id = None
        self._layout_width = WINDOW_WIDTH

    # ── 公开接口 ──────────────────────────────────────────

    def show_loading(self, source_text: str):
        """显示翻译中的加载状态"""
        self._close_existing()
        self._prepare_layout(source_text)
        self._create_window()
        self._build_loading_ui(source_text)
        self._position_at_mouse()
        self._fade_in()

    def show_result(self, source_text: str, translated_text: str):
        """显示翻译结果"""
        self._close_existing()
        self._prepare_layout(source_text, translated_text)
        self._create_window()
        self._build_result_ui(source_text, translated_text)
        self._position_at_mouse()
        self._fade_in()

    def show_error(self, source_text: str, error_message: str):
        """显示错误信息"""
        self._close_existing()
        self._prepare_layout(source_text, error_message=error_message)
        self._create_window()
        self._build_error_ui(source_text, error_message)
        self._position_at_mouse()
        self._fade_in()

    def update_to_result(self, source_text: str, translated_text: str):
        """从加载状态更新为翻译结果（原地更新）"""
        if self.window and self.window.winfo_exists():
            # 记住当前位置
            x = self.window.winfo_x()
            y = self.window.winfo_y()
            self._close_existing()
            self._prepare_layout(source_text, translated_text)
            self._create_window()
            self._build_result_ui(source_text, translated_text)
            self._position_window(x, y)
            self.window.attributes("-alpha", WINDOW_ALPHA)
            self.window.deiconify()
        else:
            self.show_result(source_text, translated_text)

    def update_to_error(self, source_text: str, error_message: str):
        """从加载状态更新为错误信息"""
        if self.window and self.window.winfo_exists():
            x = self.window.winfo_x()
            y = self.window.winfo_y()
            self._close_existing()
            self._prepare_layout(source_text, error_message=error_message)
            self._create_window()
            self._build_error_ui(source_text, error_message)
            self._position_window(x, y)
            self.window.attributes("-alpha", WINDOW_ALPHA)
            self.window.deiconify()
        else:
            self.show_error(source_text, error_message)

    def close(self):
        """关闭窗口"""
        self._close_existing()

    # ── 窗口创建 ──────────────────────────────────────────

    def _create_window(self):
        """创建基础窗口"""
        self.window = tk.Toplevel(self.root)
        self.window.overrideredirect(True)           # 无边框
        self.window.attributes("-topmost", True)      # 置顶
        self.window.attributes("-alpha", 0.0)         # 初始透明
        
        # 设置字体渲染质量
        try:
            # 启用 ClearType 字体渲染
            self.window.tk.call('tk', 'scaling', 1.0)
        except Exception:
            pass
        
        # 外层边框容器（模拟圆角边框）
        outer_frame = tk.Frame(
            self.window,
            bg=COLORS["border"],
            padx=1,
            pady=1,
        )
        outer_frame.pack(fill="both", expand=True)
        
        # 内层内容容器
        self.content_frame = tk.Frame(
            outer_frame,
            bg=COLORS["bg"],
            padx=0,
            pady=0,
        )
        self.content_frame.pack(fill="both", expand=True)
        
        # 绑定事件
        self.window.bind("<Escape>", lambda e: self.close())
        self.window.bind("<FocusOut>", self._on_focus_out)

    def _close_existing(self):
        """关闭已存在的窗口"""
        if self._loading_after_id:
            self.root.after_cancel(self._loading_after_id)
            self._loading_after_id = None

        if self._auto_close_after_id:
            self.root.after_cancel(self._auto_close_after_id)
            self._auto_close_after_id = None
        self._layout_width = WINDOW_WIDTH

        if self.window and self.window.winfo_exists():
            try:
                self.window.destroy()
            except Exception:
                pass
        self.window = None

    def _prepare_layout(self, source_text: str, translated_text: str = "", error_message: str = ""):
        """固定宽度，高度由内容决定。"""
        self._layout_width = WINDOW_WIDTH

    # ── UI 构建：加载状态 ─────────────────────────────────

    def _build_loading_ui(self, source_text: str):
        """构建加载中的界面"""
        f = self.content_frame

        # 头部
        self._build_header(f)

        # 原文区域
        self._build_source_section(f, source_text)

        # 分隔线
        self._build_divider(f)

        # 加载动画
        loading_frame = tk.Frame(f, bg=COLORS["bg"])
        loading_frame.pack(fill="x", padx=WINDOW_PADDING, pady=(12, 16))

        # 加载标签
        tag_frame = tk.Frame(loading_frame, bg=COLORS["accent_glow"])
        tag_frame.pack(anchor="w")
        
        tk.Label(
            tag_frame,
            text=" 翻译中 ",
            font=(FONT_FAMILY, FONT_SIZE_LABEL, "bold"),
            fg="#ffffff",
            bg=COLORS["accent_glow"],
            padx=4,
            pady=1,
        ).pack()

        self._loading_label = tk.Label(
            loading_frame,
            text="正在翻译，请稍候...",
            font=(FONT_FAMILY, FONT_SIZE_TEXT),
            fg=COLORS["accent"],
            bg=COLORS["bg"],
        )
        self._loading_label.pack(anchor="w", pady=(6, 0))

        self._loading_dots = 0
        self._animate_loading()

    def _animate_loading(self):
        """加载动画：翻译中..."""
        if not self.window or not self.window.winfo_exists():
            return
        self._loading_dots = (self._loading_dots + 1) % 4
        dots = "." * self._loading_dots
        try:
            self._loading_label.config(text=f"正在翻译，请稍候{dots}")
        except tk.TclError:
            return
        self._loading_after_id = self.root.after(350, self._animate_loading)

    # ── UI 构建：翻译结果 ─────────────────────────────────

    def _build_result_ui(self, source_text: str, translated_text: str):
        """构建翻译结果界面"""
        f = self.content_frame

        # 头部
        self._build_header(f)

        # 原文区域
        self._build_source_section(f, source_text)

        # 分隔线
        self._build_divider(f)

        # 译文区域
        self._build_translation_section(f, translated_text)

        # 复制按钮
        self._build_copy_button(f, translated_text)

    def _build_error_ui(self, source_text: str, error_message: str):
        """构建错误提示界面"""
        f = self.content_frame

        # 头部
        self._build_header(f)

        # 原文区域
        self._build_source_section(f, source_text)

        # 分隔线
        self._build_divider(f)

        # 错误信息
        error_frame = tk.Frame(f, bg=COLORS["error_bg"])
        error_frame.pack(fill="x", padx=WINDOW_PADDING, pady=(10, 16))

        # 错误标签
        tag_frame = tk.Frame(error_frame, bg=COLORS["error_text"])
        tag_frame.pack(anchor="w", padx=8, pady=(8, 0))
        
        tk.Label(
            tag_frame,
            text=" ⚠ 错误 ",
            font=(FONT_FAMILY, FONT_SIZE_LABEL, "bold"),
            fg="#ffffff",
            bg=COLORS["error_text"],
            padx=4,
            pady=1,
        ).pack()

        tk.Label(
            error_frame,
            text=error_message,
            font=(FONT_FAMILY, FONT_SIZE_TEXT),
            fg=COLORS["error_text"],
            bg=COLORS["error_bg"],
            wraplength=self._layout_width - WINDOW_PADDING * 2 - 20,
            justify="left",
            padx=8,
            pady=(4, 8),
        ).pack(anchor="w")

    # ── UI 组件 ───────────────────────────────────────────

    def _build_header(self, parent):
        """构建顶部标题栏"""
        header = tk.Frame(parent, bg=COLORS["header_bg"], height=38)
        header.pack(fill="x")
        header.pack_propagate(False)

        # 左侧装饰条
        accent_bar = tk.Frame(header, bg=COLORS["accent"], width=3)
        accent_bar.pack(side="left", fill="y")

        # 图标 + 标题
        title_frame = tk.Frame(header, bg=COLORS["header_bg"])
        title_frame.pack(side="left", padx=(10, 0))

        tk.Label(
            title_frame,
            text="🌐",
            font=(FONT_FAMILY, 15),
            fg=COLORS["text_result"],
            bg=COLORS["header_bg"],
        ).pack(side="left")

        tk.Label(
            title_frame,
            text="选中翻译",
            font=(FONT_FAMILY, FONT_SIZE_TITLE, "bold"),
            fg=COLORS["text_result"],
            bg=COLORS["header_bg"],
        ).pack(side="left", padx=(6, 0))

        # 关闭按钮
        close_btn = tk.Label(
            header,
            text="✕",
            font=(FONT_FAMILY, 14),
            fg=COLORS["close_normal"],
            bg=COLORS["header_bg"],
            cursor="hand2",
            padx=14,
        )
        close_btn.pack(side="right", fill="y")

        close_btn.bind("<Enter>", lambda e: close_btn.config(fg=COLORS["close_hover"]))
        close_btn.bind("<Leave>", lambda e: close_btn.config(fg=COLORS["close_normal"]))
        close_btn.bind("<Button-1>", lambda e: self.close())

        # 可拖拽标题栏
        header.bind("<Button-1>", self._start_drag)
        header.bind("<B1-Motion>", self._on_drag)

    def _build_source_section(self, parent, source_text: str):
        """构建原文显示区域"""
        frame = tk.Frame(parent, bg=COLORS["bg"])
        frame.pack(fill="x", padx=WINDOW_PADDING, pady=(14, 0))

        # 标签容器
        tag_frame = tk.Frame(frame, bg=COLORS["tag_bg"])
        tag_frame.pack(anchor="w")
        
        tk.Label(
            tag_frame,
            text=" 原文 ",
            font=(FONT_FAMILY, FONT_SIZE_LABEL),
            fg=COLORS["tag_text"],
            bg=COLORS["tag_bg"],
            padx=4,
            pady=1,
        ).pack()

        # 原文内容（使用 Text 组件支持选择和多行）
        text_widget = tk.Text(
            frame,
            font=(FONT_FAMILY, FONT_SIZE_TEXT),
            fg=COLORS["text_source"],
            bg=COLORS["bg"],
            wrap="word",
            borderwidth=0,
            highlightthickness=0,
            height=self._calc_text_height(source_text, self._layout_width, FONT_SIZE_TEXT),
            padx=0,
            pady=6,
            cursor="arrow",
        )
        text_widget.insert("1.0", source_text)
        text_widget.config(state="disabled")  # 只读
        text_widget.pack(anchor="w", fill="x")

    def _build_divider(self, parent):
        """构建分隔线"""
        divider = tk.Frame(parent, bg=COLORS["divider"], height=1)
        divider.pack(fill="x", padx=WINDOW_PADDING, pady=(10, 0))

    def _build_translation_section(self, parent, translated_text: str):
        """构建译文显示区域"""
        frame = tk.Frame(parent, bg=COLORS["bg"])
        frame.pack(fill="x", padx=WINDOW_PADDING, pady=(8, 0))

        # 标签容器
        tag_frame = tk.Frame(frame, bg=COLORS["accent_glow"])
        tag_frame.pack(anchor="w")
        
        tk.Label(
            tag_frame,
            text=" 译文 ",
            font=(FONT_FAMILY, FONT_SIZE_LABEL, "bold"),
            fg="#ffffff",
            bg=COLORS["accent_glow"],
            padx=4,
            pady=1,
        ).pack()

        # 计算文本高度，超过屏幕一定比例后再滚动
        text_height = self._calc_text_height(translated_text, self._layout_width, FONT_SIZE_TEXT + 1)
        max_text_height = max(3, (self._get_max_window_height() - 200) // 15)  # 估算最大行数
        
        # 译文内容容器（支持滚动）
        text_container = tk.Frame(frame, bg=COLORS["bg"])
        text_container.pack(anchor="w", fill="x")
        
        # 译文内容
        text_widget = tk.Text(
            text_container,
            font=(FONT_FAMILY, FONT_SIZE_TEXT + 1),
            fg=COLORS["text_result"],
            bg=COLORS["bg"],
            wrap="word",
            borderwidth=0,
            highlightthickness=0,
            height=min(text_height, max_text_height),
            padx=0,
            pady=6,
            cursor="arrow",
        )
        
        # 添加滚动条（当内容较多时）
        if text_height > max_text_height:
            scrollbar = tk.Scrollbar(text_container, command=text_widget.yview)
            scrollbar.pack(side="right", fill="y")
            text_widget.config(yscrollcommand=scrollbar.set)
        
        text_widget.insert("1.0", translated_text)
        text_widget.config(state="disabled")
        text_widget.pack(side="left", anchor="w", fill="x", expand=True)

    def _build_copy_button(self, parent, text_to_copy: str):
        """构建复制按钮"""
        btn_frame = tk.Frame(parent, bg=COLORS["bg"])
        btn_frame.pack(fill="x", padx=WINDOW_PADDING, pady=(12, 16))

        copy_btn = tk.Label(
            btn_frame,
            text="  📋 复制译文  ",
            font=(FONT_FAMILY, FONT_SIZE_BTN),
            fg=COLORS["text_result"],
            bg=COLORS["btn_copy_bg"],
            cursor="hand2",
            padx=16,
            pady=6,
        )
        copy_btn.pack(anchor="e")

        def on_copy(e):
            self.root.clipboard_clear()
            self.root.clipboard_append(text_to_copy)
            copy_btn.config(text="  ✓ 已复制  ", fg=COLORS["success_text"], bg=COLORS["btn_copy_bg"])
            self.root.after(1500, lambda: (
                copy_btn.config(text="  📋 复制译文  ", fg=COLORS["text_result"], bg=COLORS["btn_copy_bg"])
                if copy_btn.winfo_exists() else None
            ))

        copy_btn.bind("<Button-1>", on_copy)
        copy_btn.bind("<Enter>", lambda e: copy_btn.config(bg=COLORS["btn_copy_hover"]))
        copy_btn.bind("<Leave>", lambda e: copy_btn.config(bg=COLORS["btn_copy_bg"]))

    # ── 窗口行为 ──────────────────────────────────────────

    def _position_at_mouse(self):
        """将窗口定位到鼠标光标附近"""
        mouse_x = self.root.winfo_pointerx()
        mouse_y = self.root.winfo_pointery()
        self._position_window(mouse_x + 15, mouse_y + 15)

    def _position_window(self, x: int, y: int):
        """根据当前内容宽度将窗口放到指定位置并限制在屏幕内。"""
        self.window.update_idletasks()

        win_w = self._layout_width
        win_h = self.window.winfo_reqheight()
        
        # 限制窗口最大高度
        win_h = min(win_h, self._get_max_window_height())

        screen_w = self.root.winfo_screenwidth()
        screen_h = self.root.winfo_screenheight()

        if x + win_w > screen_w - 10:
            x = x - win_w - 15

        if y + win_h > screen_h - 50:
            y = y - win_h - 15

        x = max(10, x)
        y = max(10, y)

        self.window.geometry(f"{win_w}x{win_h}+{x}+{y}")

    def _fade_in(self):
        """淡入动画"""
        self._fade_alpha = 0.0
        self.window.deiconify()
        self._do_fade_in()

    def _do_fade_in(self):
        """执行淡入帧"""
        if not self.window or not self.window.winfo_exists():
            return

        self._fade_alpha += FADE_STEP
        if self._fade_alpha >= WINDOW_ALPHA:
            self.window.attributes("-alpha", WINDOW_ALPHA)
            # 窗口显示后聚焦，以便 FocusOut 和 Escape 生效
            self.window.focus_force()
            return

        self.window.attributes("-alpha", self._fade_alpha)
        self.root.after(FADE_INTERVAL, self._do_fade_in)

    def _on_focus_out(self, event):
        """窗口失去焦点时关闭"""
        # 延迟检查，避免子控件切换焦点误触发
        self._auto_close_after_id = self.root.after(200, self._check_and_close)

    def _check_and_close(self):
        """检查是否应该关闭窗口"""
        if not self.window or not self.window.winfo_exists():
            return
        # 如果焦点不在窗口及其子组件上，关闭
        try:
            focus_widget = self.window.focus_get()
            if focus_widget is None:
                self.close()
        except Exception:
            self.close()

    # ── 拖拽支持 ──────────────────────────────────────────

    def _start_drag(self, event):
        """开始拖拽窗口"""
        self._drag_start_x = event.x
        self._drag_start_y = event.y

    def _on_drag(self, event):
        """拖拽窗口"""
        if not self.window:
            return
        x = self.window.winfo_x() + event.x - self._drag_start_x
        y = self.window.winfo_y() + event.y - self._drag_start_y
        self.window.geometry(f"+{x}+{y}")

    # ── 辅助方法 ──────────────────────────────────────────

    @staticmethod
    def _calc_text_height(text: str, wrap_width: int = WINDOW_WIDTH, font_size: int = FONT_SIZE_TEXT) -> int:
        """根据实际字体宽度估算 Text 组件需要的显示行数。"""
        font = tkfont.Font(family=FONT_FAMILY, size=font_size)
        max_line_width = max(80, wrap_width - WINDOW_PADDING * 2 - 12)
        total_lines = 0

        for paragraph in text.split("\n"):
            if not paragraph:
                total_lines += 1
                continue

            line_width = 0
            paragraph_lines = 1
            for char in paragraph:
                char_width = font.measure(char)
                if line_width > 0 and line_width + char_width > max_line_width:
                    paragraph_lines += 1
                    line_width = char_width
                else:
                    line_width += char_width

            total_lines += paragraph_lines

        return max(total_lines, 1)

    def _get_max_window_height(self) -> int:
        """Calculate the popup height cap from the current screen height."""
        screen_h = self.root.winfo_screenheight()
        return max(WINDOW_MIN_HEIGHT, int(screen_h * WINDOW_MAX_HEIGHT_RATIO))
