import logging
import math
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

_KOREAN_REC_MODEL = "korean_PP-OCRv3_rec_infer.onnx"
_KOREAN_REC_DICT = "korean_dict.txt"
_JAPANESE_REC_MODEL = "japanese_PP-OCRv3_rec_infer.onnx"
_JAPANESE_REC_DICT = "japanese_dict.txt"


def _is_korean_language(value) -> bool:
    """True for Korean language codes/names used by the settings UI."""
    if not value:
        return False
    text = str(value).strip().lower()
    return text.split("-", 1)[0] in ("ko", "kor") or text in (
        "korean",
        "한국어",
        "韩语",
        "韓語",
    )


def _is_japanese_language(value) -> bool:
    """True for Japanese language codes/names used by the settings UI."""
    if not value:
        return False
    text = str(value).strip().lower()
    return text.split("-", 1)[0] in ("ja", "jpn") or text in (
        "japanese",
        "日本語",
        "日语",
        "日語",
    )


def _contains_kana(text: str) -> bool:
    return any(
        "\u3040" <= char <= "\u30ff" or "\u31f0" <= char <= "\u31ff"
        for char in str(text or "")
    )


def _contains_hangul(text: str) -> bool:
    return any(
        "\uac00" <= char <= "\ud7af" or "\u1100" <= char <= "\u11ff"
        for char in str(text or "")
    )


def _meaningful_text_ratio(text: str) -> float:
    """Share of letters/digits in the recognized text.

    The default Chinese model sometimes turns Korean glyphs into punctuation
    with high confidence; a low ratio means the result is not real text.
    """
    compact = [char for char in str(text or "") if not char.isspace()]
    if not compact:
        return 0.0
    meaningful = sum(1 for char in compact if char.isalnum())
    return meaningful / len(compact)


class OCRService:
    """
    纯后端的 OCR 服务引擎，封装 RapidOCR，无任何 UI 依赖。
    """

    def __init__(self):
        self._engine = None
        self._korean_engine = None
        self._japanese_engine = None
        self._loading_lock = threading.Lock()
        # RapidOCR/ONNX sessions are not re-entrant; the API runs endpoints in
        # the threadpool, so guard the actual recognition calls.
        self._run_lock = threading.Lock()

    def is_loaded(self) -> bool:
        """OCR 模型是否已加载"""
        with self._loading_lock:
            return (
                self._engine is not None
                or self._korean_engine is not None
                or self._japanese_engine is not None
            )

    def release(self) -> None:
        """释放 OCR 模型，回收内存"""
        with self._loading_lock:
            if self._engine is not None:
                self._engine = None
                logger.info("OCR 模型已释放")
            if self._korean_engine is not None:
                self._korean_engine = None
                logger.info("韩语 OCR 模型已释放")
            if self._japanese_engine is not None:
                self._japanese_engine = None
                logger.info("日语 OCR 模型已释放")

    @staticmethod
    def _model_files(kind: str):
        if kind == "korean":
            return _KOREAN_REC_MODEL, _KOREAN_REC_DICT
        if kind == "japanese":
            return _JAPANESE_REC_MODEL, _JAPANESE_REC_DICT
        return None

    def _language_engine_available(self, kind: str) -> bool:
        files = self._model_files(kind)
        if files is None:
            return False
        model_dir = application_dir() / "models" / "rapidocr"
        return all((model_dir / name).is_file() for name in files)

    def _korean_available(self) -> bool:
        return self._language_engine_available("korean")

    def _japanese_available(self) -> bool:
        return self._language_engine_available("japanese")

    def _ensure_engine(self, kind: str = "default"):
        """懒加载 RapidOCR（默认中英、韩语或日语识别模型）"""
        with self._loading_lock:
            if kind == "korean":
                attr = "_korean_engine"
            elif kind == "japanese":
                attr = "_japanese_engine"
            else:
                attr = "_engine"
            engine = getattr(self, attr)
            if engine is None:
                labels = {"korean": "韩语", "japanese": "日语"}
                label = labels.get(kind, "默认")
                logger.info("首次加载 RapidOCR %s 模型...", label)
                from rapidocr_onnxruntime import RapidOCR

                config_path = self._create_rapidocr_config(kind)
                try:
                    engine = RapidOCR(config_path=config_path)
                finally:
                    try:
                        os.remove(config_path)
                    except OSError:
                        pass
                setattr(self, attr, engine)
                logger.info("RapidOCR %s 模型加载完成", label)
            return engine

    @staticmethod
    def _create_rapidocr_config(kind: str = "default") -> str:
        import rapidocr_onnxruntime
        import yaml

        package_dir = os.path.dirname(rapidocr_onnxruntime.__file__)
        source_config = os.path.join(package_dir, "config.yaml")
        with open(source_config, "r", encoding="utf-8") as file:
            config = yaml.safe_load(file)

        model_dir = application_dir() / "models" / "rapidocr"
        det_path = model_dir / "ch_PP-OCRv4_det_infer.onnx"
        cls_path = model_dir / "ch_ppocr_mobile_v2.0_cls_infer.onnx"
        if kind == "korean":
            rec_path = model_dir / _KOREAN_REC_MODEL
            keys_path = model_dir / _KOREAN_REC_DICT
        elif kind == "japanese":
            rec_path = model_dir / _JAPANESE_REC_MODEL
            keys_path = model_dir / _JAPANESE_REC_DICT
        else:
            rec_path = model_dir / "ch_PP-OCRv4_rec_infer.onnx"
            keys_path = None
        required = [det_path, rec_path, cls_path]
        if keys_path is not None:
            required.append(keys_path)
        missing = [str(path) for path in required if not path.is_file()]
        if missing:
            raise FileNotFoundError(
                "RapidOCR model files are missing: " + ", ".join(missing)
            )

        config["Global"]["use_det"] = True
        config["Global"]["use_cls"] = False
        config["Global"]["use_rec"] = True
        config["Det"]["model_path"] = os.fspath(det_path)
        config["Det"]["box_thresh"] = 0.4
        config["Det"]["unclip_ratio"] = 1.8
        config["Rec"]["model_path"] = os.fspath(rec_path)
        config["Cls"]["model_path"] = os.fspath(cls_path)
        if keys_path is not None:
            config["Rec"]["rec_keys_path"] = os.fspath(keys_path)

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

    @staticmethod
    def _line_geometry(line: list[tuple], scale: float) -> dict:
        """Rotated line box from the RapidOCR quadrilateral points.

        The line direction comes from the first box's top edge (p0 -> p1), which
        follows the actual text orientation. All line corners are projected onto
        that direction and its normal to get a tight rotated rectangle.
        """
        points = []
        for box in line:
            pts = box[8] if len(box) > 8 else None
            if pts:
                points.extend((float(p[0]), float(p[1])) for p in pts)
            else:
                points.extend(
                    [
                        (float(box[2]), float(box[0])),
                        (float(box[3]), float(box[0])),
                        (float(box[3]), float(box[1])),
                        (float(box[2]), float(box[1])),
                    ]
                )
        if not points:
            return {
                "angle": 0.0,
                "cx": 0.0,
                "cy": 0.0,
                "line_width": 0.0,
                "line_height": 0.0,
            }

        first = line[0][8] if len(line[0]) > 8 else None
        if first and len(first) >= 2:
            angle = math.atan2(
                float(first[1][1]) - float(first[0][1]),
                float(first[1][0]) - float(first[0][0]),
            )
        else:
            angle = 0.0
        while angle <= -math.pi / 2:
            angle += math.pi
        while angle > math.pi / 2:
            angle -= math.pi

        dx, dy = math.cos(angle), math.sin(angle)
        nx, ny = -dy, dx
        projections = [(x * dx + y * dy, x * nx + y * ny) for x, y in points]
        along_min = min(p[0] for p in projections)
        along_max = max(p[0] for p in projections)
        normal_min = min(p[1] for p in projections)
        normal_max = max(p[1] for p in projections)
        along_mid = (along_min + along_max) / 2.0
        normal_mid = (normal_min + normal_max) / 2.0
        cx = along_mid * dx + normal_mid * nx
        cy = along_mid * dy + normal_mid * ny
        return {
            "angle": round(math.degrees(angle), 1),
            "cx": round(cx / scale, 1),
            "cy": round(cy / scale, 1),
            "line_width": round((along_max - along_min) / scale, 1),
            "line_height": round((normal_max - normal_min) / scale, 1),
        }

    def _language_fallback(self, img, allow_retry, use_korean, use_japanese, text="", average_score=0.0):
        """Try other bundled recognizers when the default result is unusable.

        Candidates are picked from the recognized script first, so Japanese and
        Korean images do not need a manual source-language setting. When the
        default model returns nothing at all, both bundled recognizers are tried
        and the better result wins.
        """
        if use_korean or use_japanese:
            # An explicit source language owns the result; never bounce between
            # recognizers (that would recurse when neither finds text).
            return None
        candidates = []
        if _contains_kana(text) and not use_japanese and self._japanese_available():
            candidates.append("japanese")
        if _contains_hangul(text) and not use_korean and self._korean_available():
            candidates.append("korean")
        if not candidates:
            if not use_korean and self._korean_available():
                candidates.append("korean")
            if not use_japanese and self._japanese_available():
                candidates.append("japanese")

        best = None
        best_quality = 0.0
        for kind in candidates:
            language = "ko" if kind == "korean" else "ja"
            logger.info("OCR 默认结果不足，尝试%s识别模型", "韩语" if kind == "korean" else "日语")
            result = self.recognize_image(img, allow_retry, source_lang=language)
            if not result.get("text"):
                continue
            boxes = result.get("line_boxes") or []
            model_score = (
                sum(box.get("score", 0.0) for box in boxes) / len(boxes)
                if boxes
                else 0.0
            )
            quality = model_score + 0.2 * _meaningful_text_ratio(result.get("text"))
            if quality > best_quality:
                best = result
                best_quality = quality

        if best is None:
            return None
        if not text:
            return best
        kana_or_hangul = _contains_kana(text) or _contains_hangul(text)
        if kana_or_hangul or best_quality > average_score + 0.05:
            return best
        return None

    def recognize_image(
        self,
        img: Image.Image,
        allow_retry: bool = True,
        source_lang: str = None,
    ) -> dict:
        """
        接收 PIL Image 进行 OCR 识别。
        返回格式:
        {
            "text": "完整的拼接文本",
            "lines": ["第一行", "第二行"],
            "raw_results": [...] # 原始 OCR 结果，可选
        }

        allow_retry=False skips the second upscaled pass. The live loop captures
        continuously, so re-recognizing every frame at a larger scale doubles the
        cost for content the next frame will refresh anyway.

        source_lang selects a bundled recognizer: Chinese/English (default),
        Korean or Japanese. In auto mode the default model runs first; if it
        finds nothing or only low-confidence text, the other recognizers are
        tried and the best result is returned.
        """
        use_korean = _is_korean_language(source_lang) and self._korean_available()
        use_japanese = _is_japanese_language(source_lang) and self._japanese_available()
        if use_korean:
            engine_kind = "korean"
        elif use_japanese:
            engine_kind = "japanese"
        else:
            engine_kind = "default"
        engine = self._ensure_engine(engine_kind)

        primary_img, primary_scale = self._preprocess_image(img)
        logger.info(
            "截图预处理完成(%dx%d, %.1fx)，开始 OCR 识别...",
            primary_img.size[0],
            primary_img.size[1],
            primary_scale,
        )

        with self._run_lock:
            primary_result, _ = engine(primary_img)
        primary_stats = self._summarize_ocr_result(primary_result)

        merged = primary_result or []
        chosen_scale = primary_scale

        retry_scale = self._next_scale_factor(img.size, primary_scale)
        if allow_retry and self._needs_retry(primary_stats, retry_scale):
            retry_img, retry_scale = self._preprocess_image(img, scale_factor=retry_scale)
            with self._run_lock:
                retry_result, _ = engine(retry_img)
            retry_stats = self._summarize_ocr_result(retry_result)

            if self._result_quality_score(retry_stats) >= self._result_quality_score(primary_stats):
                merged = retry_result or []
                chosen_scale = retry_scale
                logger.info("OCR 回退启用: 采用 %.1fx 结果", retry_scale)
            else:
                logger.info("OCR 回退放弃: 保留 %.1fx 首轮结果", primary_scale)

        if not merged:
            fallback = self._language_fallback(
                img, allow_retry, use_korean, use_japanese
            )
            if fallback is not None:
                return fallback
            return {
                "text": "",
                "lines": [],
                "line_boxes": [],
                "scale": chosen_scale,
                "image_width": img.size[0],
                "image_height": img.size[1],
            }

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
            raw_boxes.append(
                (y_top, y_bottom, x_left, x_right, text, height, width, score, box)
            )

        if not raw_boxes:
            fallback = self._language_fallback(
                img, allow_retry, use_korean, use_japanese
            )
            if fallback is not None:
                return fallback
            return {
                "text": "",
                "lines": [],
                "line_boxes": [],
                "scale": chosen_scale,
                "image_width": img.size[0],
                "image_height": img.size[1],
            }

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
        line_boxes = []
        # OCR boxes live in the (possibly upscaled) preprocessed image. Divide
        # by the scale that produced the winning result to get original pixels.
        scale = chosen_scale if chosen_scale > 0 else 1.0

        for line in lines:
            line.sort(key=lambda b: b[2])
            texts = [b[4].strip() for b in line if b[4].strip()]
            text = " ".join(texts)
            output_lines.append(text)

            x_left = min(b[2] for b in line)
            x_right = max(b[3] for b in line)
            y_top = min(b[0] for b in line)
            y_bottom = max(b[1] for b in line)
            score = sum(b[7] for b in line) / len(line)
            geometry = self._line_geometry(line, scale)
            line_boxes.append({
                "text": text,
                "x": round(x_left / scale, 1),
                "y": round(y_top / scale, 1),
                "w": round((x_right - x_left) / scale, 1),
                "h": round((y_bottom - y_top) / scale, 1),
                "score": round(score, 3),
                "angle": geometry["angle"],
                "cx": geometry["cx"],
                "cy": geometry["cy"],
                "line_width": geometry["line_width"],
                "line_height": geometry["line_height"],
            })

        text = "\n".join(output_lines).strip()
        result = {
            "text": text,
            "lines": output_lines,
            "line_boxes": line_boxes,
            "scale": chosen_scale,
            "image_width": img.size[0],
            "image_height": img.size[1],
        }
        if not use_korean and not use_japanese:
            boxes = result["line_boxes"]
            average_score = (
                sum(box["score"] for box in boxes) / len(boxes) if boxes else 0.0
            )
            if (
                not text
                or (boxes and average_score < 0.6)
                or _meaningful_text_ratio(text) < 0.5
                or _contains_kana(text)
                or _contains_hangul(text)
            ):
                logger.info(
                    "OCR 默认模型结果不足（len=%d score=%.2f），尝试其它语言模型",
                    len(text),
                    average_score,
                )
                fallback = self._language_fallback(
                    img,
                    allow_retry,
                    use_korean,
                    use_japanese,
                    text=text,
                    average_score=average_score,
                )
                if fallback is not None:
                    return fallback
        return result
