"""
ocr_engine.py — OCR 识别引擎模块

封装「屏幕截图框选 + RapidOCR 文字识别」完整流程。
通过回调式接口与主程序交互，OCR 识别出的文本复用现有翻译管线。

核心类：
- OCREngine: 懒加载 RapidOCR，提供 capture_and_recognize 回调接口
- _OverlayWindow: tkinter 全屏半透明遮罩，鼠标框选截图区域
"""

import logging
import threading
import ctypes
import tkinter as tk

from PIL import ImageGrab, Image

logger = logging.getLogger(__name__)

_TARGET_SHORT_SIDE = 320
_MAX_AUTO_SCALE = 2.0
_MAX_PREPROCESS_AREA = 2_000_000
_RETRY_CONFIDENCE_THRESHOLD = 0.72
_SCALE_STEPS = (1.0, 1.5, 2.0)
_DEDUP_OVERLAP_THRESHOLD = 0.9
_DEDUP_SIMILAR_SIZE_RATIO = 1.35
_DEDUP_CHAR_WIDTH_RATIO = 0.45
_DEDUP_CHAR_ASPECT_RATIO = 1.25

# Windows 系统指标常量（用于获取虚拟桌面范围，支持多显示器）
SM_XVIRTUALSCREEN = 76
SM_YVIRTUALSCREEN = 77
SM_CXVIRTUALSCREEN = 78
SM_CYVIRTUALSCREEN = 79


def _get_virtual_screen_rect():
    """获取虚拟桌面（所有显示器合并）的物理像素范围 (x, y, w, h)"""
    user32 = ctypes.windll.user32
    x = user32.GetSystemMetrics(SM_XVIRTUALSCREEN)
    y = user32.GetSystemMetrics(SM_YVIRTUALSCREEN)
    w = user32.GetSystemMetrics(SM_CXVIRTUALSCREEN)
    h = user32.GetSystemMetrics(SM_CYVIRTUALSCREEN)
    return x, y, w, h


class _OverlayWindow:
    """
    全屏遮罩窗口，支持鼠标框选截图区域。

    专业截图体验：
    - 选区外：黑色半透明遮罩（变暗，不可读）
    - 选区内：原屏内容清晰可见（"挖洞"效果）
    - 紫色边框（主色 #8B5CF6）+ 尺寸标签

    交互：
    - 鼠标左键按下记录起点
    - 拖拽时实时绘制选区矩形（主色边框）
    - 松开左键销毁窗口，回调 on_selected(bbox)
    - 按 Esc 取消，回调 on_cancel()
    """

    # 主色（与 settings_window.py 的 COLORS["accent"] 保持一致）
    _ACCENT_COLOR = "#8B5CF6"
    _ACCENT_BRIGHT = "#A78BFA"
    _MASK_ALPHA = 0.45  # 遮罩不透明度（0~1，越大越黑）

    def __init__(self, root: tk.Tk, on_selected, on_cancel):
        self.root = root
        self.on_selected = on_selected
        self.on_cancel = on_cancel
        self._start_x = 0
        self._start_y = 0
        self._rect_id = None
        self._dim_id = None       # 暗化遮罩的 image id
        self._bright_id = None    # 选区内亮图的 image id
        self._label_id = None     # 尺寸标签
        self._completed = False   # 防止重复回调

        vx, vy, vw, vh = _get_virtual_screen_rect()
        self._vx = vx
        self._vy = vy

        # 先截取整个虚拟桌面（用于绘制"挖洞"效果）
        self._fullscreen_img = ImageGrab.grab(bbox=(vx, vy, vx + vw, vy + vh), all_screens=True)
        self._fullscreen_tk = None  # 延迟创建 PhotoImage

        self.window = tk.Toplevel(root)
        self.window.overrideredirect(True)
        self.window.geometry(f"{vw}x{vh}+{vx}+{vy}")
        self.window.attributes("-topmost", True)
        self.window.configure(bg="black", cursor="cross")

        # Canvas 用于绘制遮罩 + 选区
        self.canvas = tk.Canvas(
            self.window,
            bg="black",
            highlightthickness=0,
            highlightbackground="black",
        )
        self.canvas.pack(fill=tk.BOTH, expand=True)

        # 绘制初始暗化遮罩（全屏暗色）
        self._draw_dim_mask()

        self.window.bind("<Escape>", self._on_escape)
        self.window.focus_force()
        self.window.after(100, self._bind_mouse)

    def _draw_dim_mask(self):
        """绘制全屏暗化遮罩：原屏截图叠加半透明黑色"""
        from PIL import ImageEnhance, ImageTk

        # 创建暗化版本（降低亮度）
        dark = ImageEnhance.Brightness(self._fullscreen_img).enhance(1.0 - self._MASK_ALPHA)
        # 再叠加一层黑色半透明
        overlay = Image.new("RGBA", dark.size, (0, 0, 0, int(255 * self._MASK_ALPHA * 0.6)))
        dark_rgba = dark.convert("RGBA")
        dark_rgba = Image.alpha_composite(dark_rgba, overlay)
        self._dim_img = dark_rgba.convert("RGB")

        self._dim_tk = ImageTk.PhotoImage(self._dim_img)
        self._dim_id = self.canvas.create_image(0, 0, anchor="nw", image=self._dim_tk)

    def _bind_mouse(self):
        """100ms 延迟绑定鼠标事件，避免吞入旧鼠标状态"""
        if not self.window or not self.window.winfo_exists():
            return
        self.canvas.bind("<ButtonPress-1>", self._on_press)
        self.canvas.bind("<B1-Motion>", self._on_drag)
        self.canvas.bind("<ButtonRelease-1>", self._on_release)

    def _on_press(self, event):
        self._start_x = event.x_root
        self._start_y = event.y_root

    def _on_drag(self, event):
        cur_x = event.x_root
        cur_y = event.y_root

        # Canvas 坐标（相对窗口左上角 = 虚拟桌面原点）
        x1 = self._start_x - self._vx
        y1 = self._start_y - self._vy
        x2 = cur_x - self._vx
        y2 = cur_y - self._vy

        # 规范化：左上角到右下角
        rx1, ry1 = min(x1, x2), min(y1, y2)
        rx2, ry2 = max(x1, x2), max(y1, y2)

        # ── "挖洞"效果：在选区位置绘制原图（亮色），覆盖暗化遮罩 ──
        from PIL import ImageTk

        crop = self._fullscreen_img.crop((rx1, ry1, rx2, ry2))
        self._bright_tk = ImageTk.PhotoImage(crop)

        if self._bright_id:
            self.canvas.coords(self._bright_id, rx1, ry1)
            self.canvas.itemconfig(self._bright_id, image=self._bright_tk)
        else:
            self._bright_id = self.canvas.create_image(rx1, ry1, anchor="nw", image=self._bright_tk)

        # ── 紫色边框 ──
        if self._rect_id:
            self.canvas.coords(self._rect_id, rx1, ry1, rx2, ry2)
        else:
            self._rect_id = self.canvas.create_rectangle(
                rx1, ry1, rx2, ry2,
                outline=self._ACCENT_COLOR,
                width=2,
            )
        # 确保边框在亮图之上
        self.canvas.tag_raise(self._rect_id, self._bright_id)

        # ── 尺寸标签 ──
        w = abs(cur_x - self._start_x)
        h = abs(cur_y - self._start_y)
        label_text = f"{w} × {h}"
        if self._label_id:
            self.canvas.itemconfig(self._label_id, text=label_text)
            # 标签放在选区下方，避免遮挡内容
            label_y = ry2 + 6
            self.canvas.coords(self._label_id, rx1, label_y)
        else:
            self._label_id = self.canvas.create_text(
                rx1, ry2 + 6,
                text=label_text,
                anchor="nw",
                fill=self._ACCENT_BRIGHT,
                font=("Microsoft YaHei UI", 10, "bold"),
            )
        self.canvas.tag_raise(self._label_id, self._rect_id)

    def _on_release(self, event):
        if self._completed:
            return
        self._completed = True

        end_x = event.x_root
        end_y = event.y_root

        x1 = min(self._start_x, end_x)
        y1 = min(self._start_y, end_y)
        x2 = max(self._start_x, end_x)
        y2 = max(self._start_y, end_y)

        self.window.destroy()

        # 选区太小（小于 5x5）视为误触，走取消
        if x2 - x1 < 5 or y2 - y1 < 5:
            logger.debug("选区太小，视为取消")
            self.on_cancel()
            return

        bbox = (x1, y1, x2, y2)
        logger.info(f"截图区域: {bbox}")
        self.on_selected(bbox)

    def _on_escape(self, _event):
        if self._completed:
            return
        self._completed = True
        self.window.destroy()
        self.on_cancel()


class OCREngine:
    """
    OCR 引擎：懒加载 RapidOCR，截图框选 + 文字识别。

    使用方式：
        engine = OCREngine(root)
        engine.capture_and_recognize(
            on_done=lambda text: ...,
            on_error=lambda msg: ...,
        )
    """

    def __init__(self, root: tk.Tk):
        self.root = root
        self._engine = None  # RapidOCR 实例，懒加载
        self._loading_lock = threading.Lock()
        self._state_lock = threading.Lock()
        self._busy = False  # 截图或识别是否正在运行（防重复启动）
        self._release_requested = False

    def is_loaded(self) -> bool:
        """OCR 模型是否已加载"""
        with self._loading_lock:
            return self._engine is not None

    def release(self) -> None:
        """释放 OCR 模型，回收内存"""
        with self._state_lock:
            if self._busy:
                self._release_requested = True
                logger.info("OCR 任务正在进行中，模型将在任务结束后释放")
                return

        self._clear_engine()

    def _clear_engine(self) -> None:
        """Thread-safe RapidOCR instance cleanup."""
        with self._loading_lock:
            if self._engine is not None:
                self._engine = None
                logger.info("OCR 模型已释放")

    def capture_and_recognize(self, on_done, on_error) -> bool:
        """
        启动截图框选 → OCR 识别流程。

        Args:
            on_done: 识别完成回调 callback(text: str)，在主线程执行
            on_error: 出错/取消回调 callback(msg: str)，在主线程执行

        Returns:
            True 表示已启动 OCR 流程，False 表示已有任务正在运行。
        """
        # 防止重复启动：上一次框选或识别还在进行中时直接忽略
        with self._state_lock:
            if self._busy:
                logger.warning("OCR 任务已在进行中，忽略重复触发")
                return False
            self._busy = True

        def _finish_task():
            """OCR 流程结束（取消/完成/出错）后重置状态"""
            should_release = False
            with self._state_lock:
                self._busy = False
                if self._release_requested:
                    self._release_requested = False
                    should_release = True
            if should_release:
                self._clear_engine()

        def _schedule_callback(callback, *args):
            try:
                self.root.after(0, callback, *args)
            except (RuntimeError, tk.TclError):
                logger.debug("Tk 主循环已关闭，跳过 OCR 回调")

        def on_selected(bbox):
            with self._state_lock:
                should_cancel = self._release_requested
            if should_cancel:
                _finish_task()
                _schedule_callback(on_error, "已取消")
                return

            # 在后台线程执行 OCR，避免阻塞 UI
            def worker():
                try:
                    text = self._recognize(bbox)
                    _schedule_callback(on_done, text)
                except Exception as e:
                    logger.error(f"OCR 识别失败: {e}", exc_info=True)
                    _schedule_callback(on_error, f"OCR 识别失败: {e}")
                finally:
                    _finish_task()

            t = threading.Thread(target=worker, daemon=True)
            try:
                t.start()
            except Exception:
                _finish_task()
                raise

        def on_cancel():
            _finish_task()
            _schedule_callback(on_error, "已取消")

        try:
            _OverlayWindow(self.root, on_selected=on_selected, on_cancel=on_cancel)
        except Exception:
            _finish_task()
            raise

        return True

    def _ensure_engine(self):
        """懒加载 RapidOCR（首次调用约 2-3s），线程安全"""
        with self._loading_lock:
            if self._engine is None:
                logger.info("首次加载 RapidOCR 模型...")
                from rapidocr_onnxruntime import RapidOCR
                # 调优参数：对小字号屏幕文字更友好
                self._engine = RapidOCR(
                    det_box_thresh=0.4,       # 降低检测阈值（默认0.5），提高灵敏度
                    det_unclip_ratio=1.8,      # 适度放大文本区域包围框，兼顾断词和相邻词重叠风险
                    use_det=True,
                    use_cls=False,             # 不需要方向分类（屏幕文字基本水平）
                    use_rec=True,
                )
                logger.info("RapidOCR 模型加载完成")
            return self._engine

    @staticmethod
    def _max_area_limited_scale(size: tuple) -> float:
        """Hard scale cap derived from the preprocess area budget."""
        width, height = size
        area = max(1, width * height)
        return min(_MAX_AUTO_SCALE, max(1.0, (_MAX_PREPROCESS_AREA / area) ** 0.5))

    @staticmethod
    def _scale_step_floor(max_scale: float) -> float:
        """Pick the highest configured scale step that does not exceed max_scale."""
        for scale in reversed(_SCALE_STEPS):
            if scale <= max_scale + 1e-6:
                return scale
        return 1.0

    @classmethod
    def _choose_scale_factor(cls, size: tuple) -> float:
        """Pick the highest safe default scale step for the initial OCR pass."""
        width, height = size
        short_side = max(1, min(width, height))
        target_cap = min(_MAX_AUTO_SCALE, max(1.0, _TARGET_SHORT_SIDE / short_side))
        safe_cap = min(target_cap, cls._max_area_limited_scale(size))
        return cls._scale_step_floor(safe_cap)

    @classmethod
    def _next_scale_factor(cls, size: tuple, current_scale: float) -> float | None:
        """Retry by moving up only one safe scale step at a time."""
        safe_cap = cls._max_area_limited_scale(size)

        for scale in _SCALE_STEPS:
            if scale > current_scale + 1e-6 and scale <= safe_cap + 1e-6:
                return scale
        return None

    @classmethod
    def _preprocess_image(cls, img: Image.Image, scale_factor: float = None) -> tuple:
        """
        图像预处理：灰度 + 自适应放大 + 暗色模式自适应。

        OCR 模型（ONNX）在自然图像（含抗锯齿边缘）上训练，
        二值化/锐化会破坏字符边缘的灰度渐变信息，反而降低准确率。
        加粗字尤甚——二值化阈值会切断笔划末端的反锯齿过渡。

        默认管线：灰度 → 自适应 1x/1.5x/2x 放大 → 暗底反转 → 输出。
        保留所有灰度层次，让模型用自己熟悉的输入格式。

        Args:
            img: PIL Image，原始截图

        Returns:
            (预处理后的 PIL Image（RGB 模式）, 实际缩放倍数)
        """
        # Step 1: 灰度化
        gray = img.convert("L")

        # Step 2: 自适应放大（LANCZOS 保留平滑边缘）
        w, h = gray.size
        if scale_factor is None:
            scale_factor = cls._choose_scale_factor((w, h))
        else:
            safe_cap = min(float(scale_factor), cls._max_area_limited_scale((w, h)))
            scale_factor = cls._scale_step_floor(safe_cap)

        if scale_factor > 1.0:
            upscaled = gray.resize(
                (
                    max(1, int(round(w * scale_factor))),
                    max(1, int(round(h * scale_factor))),
                ),
                Image.LANCZOS,
            )
        else:
            upscaled = gray

        # Step 3: 暗色模式检测 & 反转
        # 暗像素 > 50% → 暗底白字 → 反转为白底黑字
        hist = upscaled.histogram()
        total = sum(hist)
        if total > 0:
            dark_px = sum(hist[:128])   # 灰度值 < 128 的像素（暗色部分）
            if dark_px > total * 0.5:   # 暗色占主导 → 暗底白字 → 反转
                upscaled = upscaled.point(lambda x: 255 - x, mode="L")

        return upscaled.convert("RGB"), scale_factor

    @staticmethod
    def _summarize_ocr_result(result) -> dict:
        """Summarize OCR raw result quality for retry decisions."""
        texts = []
        scores = []

        for item in result or []:
            if not item or len(item) < 3:
                continue
            text = item[1].strip()
            if not text:
                continue
            texts.append(text)
            scores.append(item[2])

        count = len(texts)
        avg_score = sum(scores) / count if count else 0.0

        return {
            "count": count,
            "avg_score": avg_score,
        }

    @staticmethod
    def _needs_retry(stats: dict, retry_scale: float | None) -> bool:
        """Retry only when the first pass looks weak and a safe next scale exists."""
        if retry_scale is None:
            return False
        if stats["count"] == 0:
            return True
        if stats["avg_score"] < _RETRY_CONFIDENCE_THRESHOLD:
            return True
        return False

    @staticmethod
    def _result_quality_score(stats: dict) -> float:
        """Score OCR candidates using OCR confidence only."""
        if stats["count"] == 0:
            return float("-inf")
        return stats["avg_score"]

    @staticmethod
    def _box_area(box: tuple) -> float:
        """Axis-aligned area for a normalized OCR box tuple."""
        return max(0.0, box[5]) * max(0.0, box[6])

    @staticmethod
    def _intersection_area(box_a: tuple, box_b: tuple) -> float:
        """Intersection area between two normalized OCR box tuples."""
        y_top = max(box_a[0], box_b[0])
        y_bottom = min(box_a[1], box_b[1])
        x_left = max(box_a[2], box_b[2])
        x_right = min(box_a[3], box_b[3])
        if y_bottom <= y_top or x_right <= x_left:
            return 0.0
        return (y_bottom - y_top) * (x_right - x_left)

    @classmethod
    def _is_duplicate_box(cls, box_a: tuple, box_b: tuple) -> bool:
        """Drop only obvious geometric duplicates, without looking at text."""
        area_a = cls._box_area(box_a)
        area_b = cls._box_area(box_b)
        if area_a <= 0 or area_b <= 0:
            return False

        overlap = cls._intersection_area(box_a, box_b)
        if overlap <= 0:
            return False

        small_area = min(area_a, area_b)
        if overlap / small_area < _DEDUP_OVERLAP_THRESHOLD:
            return False

        width_a, width_b = max(1.0, box_a[6]), max(1.0, box_b[6])
        height_a, height_b = max(1.0, box_a[5]), max(1.0, box_b[5])
        width_ratio = max(width_a, width_b) / min(width_a, width_b)
        height_ratio = max(height_a, height_b) / min(height_a, height_b)

        # Same token detected twice with slightly different bounds.
        if (
            width_ratio <= _DEDUP_SIMILAR_SIZE_RATIO
            and height_ratio <= _DEDUP_SIMILAR_SIZE_RATIO
        ):
            return True

        # Small character-like box fully inside a larger word box, e.g. "P" + "Products".
        small_box = box_a if area_a <= area_b else box_b
        large_box = box_b if small_box is box_a else box_a
        small_width = max(1.0, small_box[6])
        small_height = max(1.0, small_box[5])
        large_width = max(1.0, large_box[6])
        small_aspect = small_width / small_height

        if (
            small_width / large_width <= _DEDUP_CHAR_WIDTH_RATIO
            and small_aspect <= _DEDUP_CHAR_ASPECT_RATIO
            and height_ratio <= _DEDUP_SIMILAR_SIZE_RATIO
        ):
            return True

        return False

    @classmethod
    def _dedupe_boxes(cls, boxes: list[tuple]) -> list[tuple]:
        """Keep the highest-confidence box from each obvious overlap cluster."""
        ranked = sorted(
            boxes,
            key=lambda b: (-b[7], -(b[5] * b[6]), b[0], b[2]),
        )
        kept = []
        dropped = 0

        for box in ranked:
            if any(cls._is_duplicate_box(box, kept_box) for kept_box in kept):
                dropped += 1
                continue
            kept.append(box)

        if dropped:
            logger.debug("OCR 几何去重: %d -> %d", len(boxes), len(kept))
        return kept

    def _recognize(self, bbox) -> str:
        """
        后台线程执行：截图 → 预处理 → OCR 识别(必要时重试) → 拼接文本

        Args:
            bbox: 截图区域 (x1, y1, x2, y2)，基于虚拟桌面的物理像素坐标

        Returns:
            识别出的文本（多行用换行符连接），无结果返回空字符串
        """
        engine = self._ensure_engine()

        img = ImageGrab.grab(bbox=bbox, all_screens=True)

        # ── 默认轻量预处理 + 按需重试 ──
        primary_img, primary_scale = self._preprocess_image(img)
        logger.info(
            "截图完成(%dx%d 预处理后, %.1fx)，开始 OCR 识别...",
            primary_img.size[0],
            primary_img.size[1],
            primary_scale,
        )

        primary_result, _ = engine(primary_img)
        primary_stats = self._summarize_ocr_result(primary_result)
        logger.debug(
            "OCR 首轮: scale=%.1fx, boxes=%d, avg_score=%.3f",
            primary_scale,
            primary_stats["count"],
            primary_stats["avg_score"],
        )

        merged = primary_result or []
        chosen_scale = primary_scale

        retry_scale = self._next_scale_factor(img.size, primary_scale)
        if self._needs_retry(primary_stats, retry_scale):
            retry_img, retry_scale = self._preprocess_image(img, scale_factor=retry_scale)
            retry_result, _ = engine(retry_img)
            retry_stats = self._summarize_ocr_result(retry_result)
            logger.debug(
                "OCR 重试: scale=%.1fx, boxes=%d, avg_score=%.3f",
                retry_scale,
                retry_stats["count"],
                retry_stats["avg_score"],
            )

            if self._result_quality_score(retry_stats) >= self._result_quality_score(primary_stats):
                merged = retry_result or []
                chosen_scale = retry_scale
                logger.info("OCR 回退启用: 采用 %.1fx 结果", retry_scale)
            else:
                logger.info("OCR 回退放弃: 保留 %.1fx 首轮结果", primary_scale)

        if not merged:
            logger.info("OCR 未识别到文本")
            return ""

        # result 格式: [(box, text, score), ...]
        # box 是 [[x1,y1], [x2,y2], [x3,y3], [x4,y4]]，用坐标判断同行 + 排序

        # ── 文本框排序：聚类分行 → 行间从上到下 → 行内从左到右 ──
        #
        # RapidOCR 返回的检测框顺序不保证符合阅读顺序（尤其是英文按词切分时），
        # 必须根据坐标重排。核心思路：
        #   1. 用自适应阈值（中位数字高的 50%）将框聚类为"行"
        #   2. 每行内部按 x_left 升序排列
        #   3. 各行按平均 y_top 升序排列

        # 提取每个检测框的位置信息。这里只保留 OCR 自己给出的文本和坐标，
        # 不再对内容做“猜词式”的修正。
        raw_boxes = []
        for item in merged:
            if not item or len(item) < 3:
                continue
            box, text, score = item[0], item[1], item[2]
            if score < 0.5 or not text.strip():
                continue
            xs = [p[0] for p in box]
            ys = [p[1] for p in box]
            y_top = min(ys)
            y_bottom = max(ys)
            x_left = min(xs)
            x_right = max(xs)
            height = y_bottom - y_top
            width = x_right - x_left
            raw_boxes.append((y_top, y_bottom, x_left, x_right, text, height, width, score))

        if not raw_boxes:
            return ""

        boxes = self._dedupe_boxes(raw_boxes)

        # 自适应行距阈值：所有框高度的中位数 × 0.5
        # （固定像素值无法适配不同字号；中位数抗个别异常大/小框干扰）
        heights = sorted(b[5] for b in boxes)
        median_height = heights[len(heights) // 2]
        line_gap_threshold = median_height * 0.5

        logger.debug(
            f"OCR 排序参数: {len(boxes)} 个文本框, "
            f"中位高度={median_height:.1f}px, 行距阈值={line_gap_threshold:.1f}px"
        )

        # ── Step 1: 质心聚类分行 ──
        # 每个框与已有行的平均 Y 质心比较，距离 ≤ 阈值则归入该行；
        # 否则新建一行。这比"只跟上一个框比较"的顺序分组稳定得多。
        lines = []  # List[List[tuple]]
        boxes.sort(key=lambda b: (((b[0] + b[1]) / 2.0), b[2]))

        for b in boxes:
            b_y_center = (b[0] + b[1]) / 2.0
            placed = False

            for line in lines:
                line_centers = [(lb[0] + lb[1]) / 2.0 for lb in line]
                avg_center = sum(line_centers) / len(line_centers)
                if abs(b_y_center - avg_center) <= line_gap_threshold:
                    line.append(b)
                    placed = True
                    break

            if not placed:
                lines.append([b])

        # ── Step 2: 行间排序（从上到下）──
        lines.sort(key=lambda line: sum(lb[0] for lb in line) / len(line))

        # ── Step 3: 行内排序 + 拼接 ──
        output_lines = []
        for line in lines:
            line.sort(key=lambda b: b[2])  # 按 x_left 升序
            texts = [b[4].strip() for b in line if b[4].strip()]
            output_lines.append(" ".join(texts))

        text = "\n".join(output_lines)

        # 仅做基础收尾，不对词形本身做修正。
        text = text.strip()

        logger.info("OCR 识别完成，共 %d 行文本，采用 %.1fx 预处理", len(output_lines), chosen_scale)
        return text
