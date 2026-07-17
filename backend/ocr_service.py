import logging
import os
import tempfile
import threading
from PIL import Image

from backend.runtime_paths import application_dir

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


class OCRService:
    """
    纯后端的 OCR 服务引擎，封装 RapidOCR，无任何 UI 依赖。
    """

    def __init__(self):
        self._engine = None
        self._loading_lock = threading.Lock()

    def is_loaded(self) -> bool:
        """OCR 模型是否已加载"""
        with self._loading_lock:
            return self._engine is not None

    def release(self) -> None:
        """释放 OCR 模型，回收内存"""
        with self._loading_lock:
            if self._engine is not None:
                self._engine = None
                logger.info("OCR 模型已释放")

    def _ensure_engine(self):
        """懒加载 RapidOCR"""
        with self._loading_lock:
            if self._engine is None:
                logger.info("首次加载 RapidOCR 模型...")
                from rapidocr_onnxruntime import RapidOCR

                config_path = self._create_rapidocr_config()
                try:
                    self._engine = RapidOCR(config_path=config_path)
                finally:
                    try:
                        os.remove(config_path)
                    except OSError:
                        pass
                logger.info("RapidOCR 模型加载完成")
            return self._engine

    @staticmethod
    def _create_rapidocr_config() -> str:
        import rapidocr_onnxruntime
        import yaml

        package_dir = os.path.dirname(rapidocr_onnxruntime.__file__)
        source_config = os.path.join(package_dir, "config.yaml")
        with open(source_config, "r", encoding="utf-8") as file:
            config = yaml.safe_load(file)

        model_dir = application_dir() / "models" / "rapidocr"
        model_paths = {
            "Det": model_dir / "ch_PP-OCRv4_det_infer.onnx",
            "Rec": model_dir / "ch_PP-OCRv4_rec_infer.onnx",
            "Cls": model_dir / "ch_ppocr_mobile_v2.0_cls_infer.onnx",
        }
        missing = [str(path) for path in model_paths.values() if not path.is_file()]
        if missing:
            raise FileNotFoundError(
                "RapidOCR model files are missing: " + ", ".join(missing)
            )

        config["Global"]["use_det"] = True
        config["Global"]["use_cls"] = False
        config["Global"]["use_rec"] = True
        config["Det"]["model_path"] = os.fspath(model_paths["Det"])
        config["Det"]["box_thresh"] = 0.4
        config["Det"]["unclip_ratio"] = 1.8
        config["Rec"]["model_path"] = os.fspath(model_paths["Rec"])
        config["Cls"]["model_path"] = os.fspath(model_paths["Cls"])

        file = tempfile.NamedTemporaryFile(
            mode="w",
            suffix=".yaml",
            prefix="translate-rapidocr-",
            encoding="utf-8",
            delete=False,
        )
        try:
            yaml.safe_dump(config, file, allow_unicode=True, sort_keys=False)
            return file.name
        finally:
            file.close()

    @staticmethod
    def _max_area_limited_scale(size: tuple) -> float:
        width, height = size
        area = max(1, width * height)
        return min(_MAX_AUTO_SCALE, max(1.0, (_MAX_PREPROCESS_AREA / area) ** 0.5))

    @staticmethod
    def _scale_step_floor(max_scale: float) -> float:
        for scale in reversed(_SCALE_STEPS):
            if scale <= max_scale + 1e-6:
                return scale
        return 1.0

    @classmethod
    def _choose_scale_factor(cls, size: tuple) -> float:
        width, height = size
        short_side = max(1, min(width, height))
        target_cap = min(_MAX_AUTO_SCALE, max(1.0, _TARGET_SHORT_SIDE / short_side))
        safe_cap = min(target_cap, cls._max_area_limited_scale(size))
        return cls._scale_step_floor(safe_cap)

    @classmethod
    def _next_scale_factor(cls, size: tuple, current_scale: float) -> float | None:
        safe_cap = cls._max_area_limited_scale(size)
        for scale in _SCALE_STEPS:
            if scale > current_scale + 1e-6 and scale <= safe_cap + 1e-6:
                return scale
        return None

    @classmethod
    def _preprocess_image(cls, img: Image.Image, scale_factor: float = None) -> tuple:
        gray = img.convert("L")
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

        hist = upscaled.histogram()
        total = sum(hist)
        if total > 0:
            dark_px = sum(hist[:128])
            if dark_px > total * 0.5:
                upscaled = upscaled.point(lambda x: 255 - x, mode="L")

        return upscaled.convert("RGB"), scale_factor

    @staticmethod
    def _summarize_ocr_result(result) -> dict:
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
        return {"count": count, "avg_score": avg_score}

    @staticmethod
    def _needs_retry(stats: dict, retry_scale: float | None) -> bool:
        if retry_scale is None:
            return False
        if stats["count"] == 0:
            return True
        if stats["avg_score"] < _RETRY_CONFIDENCE_THRESHOLD:
            return True
        return False

    @staticmethod
    def _result_quality_score(stats: dict) -> float:
        if stats["count"] == 0:
            return float("-inf")
        return stats["avg_score"]

    @staticmethod
    def _box_area(box: tuple) -> float:
        return max(0.0, box[5]) * max(0.0, box[6])

    @staticmethod
    def _intersection_area(box_a: tuple, box_b: tuple) -> float:
        y_top = max(box_a[0], box_b[0])
        y_bottom = min(box_a[1], box_b[1])
        x_left = max(box_a[2], box_b[2])
        x_right = min(box_a[3], box_b[3])
        if y_bottom <= y_top or x_right <= x_left:
            return 0.0
        return (y_bottom - y_top) * (x_right - x_left)

    @classmethod
    def _is_duplicate_box(cls, box_a: tuple, box_b: tuple) -> bool:
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

        if (
            width_ratio <= _DEDUP_SIMILAR_SIZE_RATIO
            and height_ratio <= _DEDUP_SIMILAR_SIZE_RATIO
        ):
            return True

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

    def recognize_image(self, img: Image.Image) -> dict:
        """
        接收 PIL Image 进行 OCR 识别。
        返回格式:
        {
            "text": "完整的拼接文本",
            "lines": ["第一行", "第二行"],
            "raw_results": [...] # 原始 OCR 结果，可选
        }
        """
        engine = self._ensure_engine()

        primary_img, primary_scale = self._preprocess_image(img)
        logger.info(
            "截图预处理完成(%dx%d, %.1fx)，开始 OCR 识别...",
            primary_img.size[0],
            primary_img.size[1],
            primary_scale,
        )

        primary_result, _ = engine(primary_img)
        primary_stats = self._summarize_ocr_result(primary_result)

        merged = primary_result or []
        chosen_scale = primary_scale

        retry_scale = self._next_scale_factor(img.size, primary_scale)
        if self._needs_retry(primary_stats, retry_scale):
            retry_img, retry_scale = self._preprocess_image(img, scale_factor=retry_scale)
            retry_result, _ = engine(retry_img)
            retry_stats = self._summarize_ocr_result(retry_result)

            if self._result_quality_score(retry_stats) >= self._result_quality_score(primary_stats):
                merged = retry_result or []
                chosen_scale = retry_scale
                logger.info("OCR 回退启用: 采用 %.1fx 结果", retry_scale)
            else:
                logger.info("OCR 回退放弃: 保留 %.1fx 首轮结果", primary_scale)

        if not merged:
            return {"text": "", "lines": [], "scale": chosen_scale}

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
            return {"text": "", "lines": [], "scale": chosen_scale}

        boxes = self._dedupe_boxes(raw_boxes)

        heights = sorted(b[5] for b in boxes)
        median_height = heights[len(heights) // 2]
        line_gap_threshold = median_height * 0.5

        lines = []
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

        lines.sort(key=lambda line: sum(lb[0] for lb in line) / len(line))

        output_lines = []
        for line in lines:
            line.sort(key=lambda b: b[2])
            texts = [b[4].strip() for b in line if b[4].strip()]
            output_lines.append(" ".join(texts))

        text = "\n".join(output_lines).strip()
        return {
            "text": text,
            "lines": output_lines,
            "scale": chosen_scale
        }
