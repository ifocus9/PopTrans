"""
translator.py — Hy-MT2-1.8B 翻译引擎模块

使用腾讯 Hy-MT2-1.8B 提供高质量离线翻译。
支持多语言翻译，未指定目标语言时默认进行中英文互译。
使用 llama-cpp-python 进行推理，支持 CPU 加速。
"""

import os
import sys
import re
import time
import threading
import logging
from collections import OrderedDict
from typing import Optional, Tuple

from backend.runtime_paths import application_dir

logger = logging.getLogger(__name__)

# ── 模型配置 ──────────────────────────────────────────────

MODEL_ID = "tencent/Hy-MT2-1.8B-GGUF"
MODEL_FILENAME = "Hy-MT2-1.8B-Q4_K_M.gguf"

_BASE_DIR = os.fspath(application_dir())

MODEL_DIR = os.path.join(_BASE_DIR, "models", "Hy-MT2-1.8B-GGUF")
MODEL_PATH = os.path.join(MODEL_DIR, MODEL_FILENAME)

# 直接从 HuggingFace 官网下载模型。
# 之前使用的 hf-mirror.com 镜像连不上会导致下载失败，故改为官方源。
HF_ENDPOINT = "https://huggingface.co"
os.environ["HF_ENDPOINT"] = HF_ENDPOINT

# 离线模式开关：设置环境变量 POPTRANS_OFFLINE=1 时禁止联网下载（其余情况本地缺模型即自动下载）。
_OFFLINE_MODE = os.environ.get("POPTRANS_OFFLINE", "").strip() == "1"

# Hy-MT2 推荐参数
MODEL_N_CTX = 1536
TRANSLATION_MAX_TOKENS = 384
CACHE_MAX_ENTRIES = 256
# 长文翻译：上下文窗口有限（n_ctx=1536）。单次请求必须同时容纳
# prompt 模板、源文与译文，源文过长时要么触发 "requested tokens exceed
# context window" 报错，要么输出被 max_tokens 截断。因此：
#   - 源文按句子边界分块，每块输入估算不超过 SOURCE_MAX_TOKENS；
#   - 每块按剩余上下文动态分配输出上限（最高 OUTPUT_MAX_TOKENS）。
OUTPUT_MAX_TOKENS = 768
TEMPLATE_OVERHEAD_TOKENS = 64
SAFETY_MARGIN_TOKENS = 32
SOURCE_MAX_TOKENS = (
    MODEL_N_CTX - TEMPLATE_OVERHEAD_TOKENS - OUTPUT_MAX_TOKENS - SAFETY_MARGIN_TOKENS
)

GENERATION_CONFIG = {
    "temperature": 0.7,
    "top_p": 0.6,
    "top_k": 20,
    "repeat_penalty": 1.05,
    # max_tokens 由 _complete 按剩余上下文动态计算，这里不写死。
}

# 语言名称映射
LANG_NAMES = {
    "zh": "中文",
    "en": "英语",
    "vi": "越南语",
    "ja": "日语",
    "tr": "土耳其语",
    "ko": "韩语",
    "th": "泰语",
    "it": "意大利语",
    "id": "印度尼西亚语",
    "ms": "马来语",
    "tl": "菲律宾语",
    "hi": "印地语",
    "zh-hant": "繁体中文",
    "fr": "法语",
    "es": "西班牙语",
    "de": "德语",
    "pt": "葡萄牙语",
    "ru": "俄语",
    "ar": "阿拉伯语",
    "pl": "波兰语",
    "cs": "捷克语",
    "nl": "荷兰语",
    "km": "高棉语",
    "my": "缅甸语",
    "fa": "波斯语",
    "gu": "古吉拉特语",
    "ur": "乌尔都语",
    "te": "泰卢固语",
    "mr": "马拉地语",
    "he": "希伯来语",
    "bn": "孟加拉语",
    "ta": "泰米尔语",
    "uk": "乌克兰语",
    "bo": "藏语",
    "kk": "哈萨克语",
    "mn": "蒙古语",
    "ug": "维吾尔语",
    "yue": "粤语",
}

LANG_NAMES_EN = {
    "zh": "Chinese",
    "en": "English",
    "vi": "Vietnamese",
    "ja": "Japanese",
    "tr": "Turkish",
    "ko": "Korean",
    "th": "Thai",
    "it": "Italian",
    "id": "Indonesian",
    "ms": "Malay",
    "tl": "Filipino",
    "hi": "Hindi",
    "zh-hant": "Traditional Chinese",
    "fr": "French",
    "es": "Spanish",
    "de": "German",
    "pt": "Portuguese",
    "ru": "Russian",
    "ar": "Arabic",
    "pl": "Polish",
    "cs": "Czech",
    "nl": "Dutch",
    "km": "Khmer",
    "my": "Burmese",
    "fa": "Persian",
    "gu": "Gujarati",
    "ur": "Urdu",
    "te": "Telugu",
    "mr": "Marathi",
    "he": "Hebrew",
    "bn": "Bengali",
    "ta": "Tamil",
    "uk": "Ukrainian",
    "bo": "Tibetan",
    "kk": "Kazakh",
    "mn": "Mongolian",
    "ug": "Uyghur",
    "yue": "Cantonese",
}

LANG_ALIASES = {
    "中文": "zh",
    "汉语": "zh",
    "普通话": "zh",
    "chinese": "zh",
    "英语": "en",
    "英文": "en",
    "english": "en",
    "越南语": "vi",
    "越南文": "vi",
    "vietnamese": "vi",
    "日语": "ja",
    "日文": "ja",
    "japanese": "ja",
    "土耳其语": "tr",
    "turkish": "tr",
    "韩语": "ko",
    "韩文": "ko",
    "korean": "ko",
    "泰语": "th",
    "thai": "th",
    "意大利语": "it",
    "italian": "it",
    "印度尼西亚语": "id",
    "印尼语": "id",
    "indonesian": "id",
    "马来语": "ms",
    "malay": "ms",
    "菲律宾语": "tl",
    "filipino": "tl",
    "印地语": "hi",
    "hindi": "hi",
    "繁体中文": "zh-hant",
    "traditional chinese": "zh-hant",
    "法语": "fr",
    "french": "fr",
    "西班牙语": "es",
    "西班牙文": "es",
    "spanish": "es",
    "德语": "de",
    "german": "de",
    "葡萄牙语": "pt",
    "portuguese": "pt",
    "俄语": "ru",
    "russian": "ru",
    "阿拉伯语": "ar",
    "arabic": "ar",
    "波兰语": "pl",
    "polish": "pl",
    "捷克语": "cs",
    "czech": "cs",
    "荷兰语": "nl",
    "dutch": "nl",
    "高棉语": "km",
    "khmer": "km",
    "缅甸语": "my",
    "burmese": "my",
    "波斯语": "fa",
    "persian": "fa",
    "古吉拉特语": "gu",
    "gujarati": "gu",
    "乌尔都语": "ur",
    "urdu": "ur",
    "泰卢固语": "te",
    "telugu": "te",
    "马拉地语": "mr",
    "marathi": "mr",
    "希伯来语": "he",
    "hebrew": "he",
    "孟加拉语": "bn",
    "bengali": "bn",
    "泰米尔语": "ta",
    "tamil": "ta",
    "乌克兰语": "uk",
    "ukrainian": "uk",
    "藏语": "bo",
    "tibetan": "bo",
    "哈萨克语": "kk",
    "kazakh": "kk",
    "蒙古语": "mn",
    "mongolian": "mn",
    "维吾尔语": "ug",
    "uyghur": "ug",
    "粤语": "yue",
    "cantonese": "yue",
}


def normalize_target_language(target_lang: Optional[str]) -> Optional[str]:
    """Return a supported ISO language code, or None for automatic mode."""
    if target_lang is None:
        return None

    value = str(target_lang).strip()
    if not value:
        return None

    normalized = value.lower().replace("_", "-")
    if normalized in LANG_NAMES:
        return normalized

    language_code = normalized.split("-", 1)[0]
    if language_code in LANG_NAMES:
        return language_code

    alias = LANG_ALIASES.get(normalized) or LANG_ALIASES.get(value)
    if alias:
        return alias

    supported = ", ".join(sorted(LANG_NAMES))
    raise ValueError(
        f"不支持的目标语言: {value}，支持的语言代码: {supported}"
    )


# 翻译 prompt 模板
PROMPT_TEMPLATE = (
    "Translate the following text into {target_lang}. Note that you should "
    "only output the translated result without any additional explanation:\n\n"
    "{source_text}"
)


def build_translation_prompt(target_lang: str, source_text: str) -> str:
    """Build the official Hy-MT2 default-translation instruction."""
    return PROMPT_TEMPLATE.format(
        target_lang=LANG_NAMES_EN[target_lang],
        source_text=source_text,
    )


# ── 长文分块 ──────────────────────────────────────────────

# 中文/日文等 CJK 二元码约 1 token/字，其余按 ~0.35 token/字符估算。
# 估算刻意偏保守，宁可多切一刀也绝不让单块溢出上下文。
_CJK_TOKEN_CHARS_RE = re.compile(r'[\u4e00-\u9fff\u3400-\u4dbf\uf900-\ufaff]')
_NON_CJK_TOKEN_CHARS_RE = re.compile(r'[^\s\u4e00-\u9fff\u3400-\u4dbf\uf900-\ufaff]')
# 博客/文档常见断点：中文句末标点、英文句号后空白、以及换行/空行。
_SENTENCE_SPLIT_RE = re.compile(r'[\u3002\uff01\uff1f!?.…;;\n]+|(?<=[a-zA-Z0-9\u4e00-\u9fff])\.\s+')


def estimate_tokens(text: str) -> int:
    """估算 llama.cpp BPE 的 token 数（保守加权 + 少量固定开销）。"""
    if not text:
        return 0
    cjk = len(_CJK_TOKEN_CHARS_RE.findall(text))
    other = len(_NON_CJK_TOKEN_CHARS_RE.findall(text))
    return int(cjk * 1.0 + other * 0.35) + 8


def _hard_split_by_tokens(text: str, max_tokens: int) -> list[str]:
    """按 token 估算将一段超长文本硬切为多块（逐字符累积）。"""
    pieces = []
    current = []
    current_tokens = 0
    for char in text:
        if _CJK_TOKEN_CHARS_RE.match(char):
            tok = 1.0
        elif not char.isspace():
            tok = 0.35
        else:
            tok = 0.0
        if current and current_tokens + tok > max_tokens:
            pieces.append("".join(current))
            current = []
            current_tokens = 0
        current.append(char)
        current_tokens += tok
    if current:
        pieces.append("".join(current))
    return pieces


def _split_long_text(text: str):
    """把超长源文按句子边界拆成可放入单次上下文的块。

    Returns:
        (chunks, split): split 为 True 表示发生了分块。
        未超限时直接返回 [text], False，保持原有单次推理路径。
    """
    if estimate_tokens(text) <= SOURCE_MAX_TOKENS:
        return [text], False

    parts = [p.strip() for p in _SENTENCE_SPLIT_RE.split(text) if p and p.strip()]
    if not parts:
        parts = [text]

    chunks = []
    current = []
    current_tokens = 0
    for part in parts:
        part_tokens = estimate_tokens(part)
        if part_tokens > SOURCE_MAX_TOKENS:
            if current:
                chunks.append("\n".join(current))
                current = []
                current_tokens = 0
            chunks.extend(_hard_split_by_tokens(part, SOURCE_MAX_TOKENS))
            continue
        if current and current_tokens + part_tokens > SOURCE_MAX_TOKENS:
            chunks.append("\n".join(current))
            current = []
            current_tokens = 0
        current.append(part)
        current_tokens += part_tokens
    if current:
        chunks.append("\n".join(current))
    return chunks, True


class Translator:
    """Hy-MT2-1.8B 翻译引擎封装（使用 llama-cpp-python）"""

    # 中文 Unicode 范围正则
    _CJK_PATTERN = re.compile(r'[\u4e00-\u9fff\u3400-\u4dbf\uf900-\ufaff]')

    def __init__(self):
        self.ready = False
        self._model = None
        self._setup_lock = threading.Lock()
        self._cache_lock = threading.Lock()
        self._translation_cache = OrderedDict()
        self._status_message = "翻译引擎未初始化"

    @property
    def status(self) -> str:
        return self._status_message

    # ── 初始化 ──────────────────────────────────────────────

    def setup(self, on_ready=None, on_status=None):
        """
        异步初始化翻译引擎（加载模型）。

        Args:
            on_ready: 初始化完成时的回调 callback(success: bool)
            on_status: 状态变化时的回调 callback(message: str)
        """
        thread = threading.Thread(
            target=self._setup_worker,
            args=(on_ready, on_status),
            daemon=True,
        )
        thread.start()

    def _setup_worker(self, on_ready, on_status):
        """后台线程：加载模型"""
        def update_status(msg):
            self._status_message = msg
            logger.info(msg)
            if on_status:
                on_status(msg)

        with self._setup_lock:
            try:
                # 本地缺少模型时，自动从 HuggingFace 官网下载
                if not os.path.exists(MODEL_PATH):
                    if _OFFLINE_MODE:
                        raise FileNotFoundError(
                            f"模型文件未找到: {MODEL_PATH}\n"
                            "当前为离线模式（POPTRANS_OFFLINE=1），已禁止联网下载。"
                            "请将模型文件放置到上述路径，或取消离线模式后重试。"
                        )
                    update_status("首次使用需下载 Hy-MT2 模型（约 1.13GB）...")
                    self._download_model(update_status)

                update_status("正在加载翻译模型...")
                self._load_model()

                self.ready = True
                update_status("翻译引擎就绪")

                if on_ready:
                    on_ready(True)

            except Exception as e:
                error_msg = f"翻译引擎初始化失败: {e}"
                update_status(error_msg)
                logger.exception("翻译引擎初始化异常")
                if on_ready:
                    on_ready(False)

    def _download_model(self, update_status):
        """从 HuggingFace 官网下载 Hy-MT2 GGUF 模型（支持断点续传、重试）"""
        import requests
        from huggingface_hub import hf_hub_url

        update_status("正在从 HuggingFace 官网下载模型...")
        os.makedirs(MODEL_DIR, exist_ok=True)

        # 走系统代理访问官网（默认 requests 行为），避免直连被网络阻断
        session = requests.Session()
        url = hf_hub_url(MODEL_ID, MODEL_FILENAME, repo_type="model")

        # 探测文件大小（最多重试 5 次）
        total_size = 1133080448
        for attempt in range(1, 6):
            try:
                resp = session.head(url, verify=False, timeout=(15, 30), allow_redirects=True)
                resp.raise_for_status()
                try:
                    size = int(resp.headers.get("Content-Length", total_size))
                except (ValueError, TypeError):
                    size = total_size
                if size >= 1024 * 1024:
                    total_size = size
                break
            except Exception as e:
                logger.warning("探测模型大小第 %d/5 次失败: %s", attempt, e)
                if attempt == 5:
                    update_status("无法获取模型大小，使用默认 1.13GB 继续下载")
                else:
                    update_status(f"连接官网失败，正在重试（{attempt}/5）...")
                    time.sleep(1)

        part_path = MODEL_PATH + ".part"
        max_retries = 10
        chunk_size = 1024 * 1024  # 1 MB

        for attempt in range(1, max_retries + 1):
            existing = 0
            if os.path.exists(part_path):
                existing = os.path.getsize(part_path)
                if existing >= total_size:
                    os.replace(part_path, MODEL_PATH)
                    update_status("模型下载完成")
                    return

            headers = {}
            mode = "wb"
            if existing > 0:
                headers["Range"] = f"bytes={existing}-"
                mode = "ab"

            try:
                response = session.get(
                    url,
                    stream=True,
                    headers=headers,
                    verify=False,
                    timeout=(15, 300),
                    allow_redirects=True,
                )

                # 200 表示服务器忽略 Range 要求从头传；206 才是断点续传
                if response.status_code == 206:
                    content_range = response.headers.get("Content-Range", "")
                    # bytes 402536757-1133080447/1133080448
                    if content_range and "/" in content_range:
                        try:
                            total_size = int(content_range.split("/")[-1])
                        except (ValueError, IndexError):
                            pass
                elif response.status_code == 200:
                    # 不支持 Range，只能从头再来
                    existing = 0
                    mode = "wb"
                    update_status("服务器不支持断点续传，从头开始下载...")
                else:
                    response.raise_for_status()

                downloaded = existing
                last_reported_mb = existing // (1024 * 1024)
                fsync_counter = 0
                with open(part_path, mode) as f:
                    for chunk in response.iter_content(chunk_size=chunk_size):
                        if not chunk:
                            continue
                        f.write(chunk)
                        downloaded += len(chunk)
                        fsync_counter += len(chunk)
                        # 每写入约 10 MB 做一次 fsync，确保断点可靠
                        if fsync_counter >= 10 * 1024 * 1024:
                            f.flush()
                            os.fsync(f.fileno())
                            fsync_counter = 0
                        current_mb = downloaded // (1024 * 1024)
                        if current_mb > last_reported_mb:
                            last_reported_mb = current_mb
                            percent = min(int(downloaded / total_size * 100), 100)
                            update_status(
                                f"正在下载模型... {downloaded / (1024*1024):.1f}/{total_size / (1024*1024):.1f} MB ({percent}%)"
                            )
                    f.flush()
                    os.fsync(f.fileno())

                if downloaded >= total_size:
                    os.replace(part_path, MODEL_PATH)
                    update_status("模型下载完成")
                    return

                # 走到这里说明连接被提前关闭，但已有数据已落盘，可续传
                raise IOError(f"连接提前关闭：已下载 {downloaded} / {total_size} 字节")

            except Exception as e:
                logger.warning("模型下载第 %d/%d 次失败: %s", attempt, max_retries, e)
                if attempt < max_retries:
                    wait = min(2 ** (attempt - 1), 30)  # 1, 2, 4, 8, 16, 30...
                    update_status(
                        f"下载中断，已保存 {existing / (1024*1024):.1f} MB，"
                        f"{wait} 秒后断点续传（{attempt}/{max_retries}）..."
                    )
                    time.sleep(wait)
                else:
                    update_status(f"模型下载失败（已重试 {max_retries} 次）: {e}")
                    raise RuntimeError(f"模型下载失败（已重试 {max_retries} 次）: {e}") from e

    def _load_model(self):
        """加载 llama-cpp-python 模型"""
        # PyInstaller 打包后，需要手动设置 DLL 路径
        if getattr(sys, 'frozen', False):
            import ctypes
            _internal_dir = os.path.join(os.path.dirname(sys.executable), '_internal')
            _lib_dir = os.path.join(_internal_dir, 'llama_cpp', 'lib')
            if os.path.exists(_lib_dir):
                os.add_dll_directory(_lib_dir)
                os.environ["PATH"] = _lib_dir + os.pathsep + os.environ.get("PATH", "")

        from llama_cpp import Llama

        self._model = Llama(
            model_path=MODEL_PATH,
            n_ctx=MODEL_N_CTX,  # 划词翻译以短文本为主，收紧上下文可减少推理开销
            n_threads=os.cpu_count(),  # 使用所有 CPU 核心
            verbose=False,  # 关闭详细日志
        )


    def close(self):
        """Release the loaded llama.cpp model before application exit."""
        self.ready = False
        model = self._model
        self._model = None
        if model and hasattr(model, "close"):
            model.close()
    # ── 翻译 ──────────────────────────────────────────────

    def _complete(self, source_text: str, target_code: str) -> Optional[str]:
        """单次调用模型完成一段文本的翻译，按剩余上下文动态分配输出上限。

        max_tokens 不再写死：短文本沿用接近 384 的默认，长文分块可获得更大
        输出空间，同时不会超过 n_ctx 导致上下文溢出。
        """
        prompt = build_translation_prompt(target_code, source_text)
        prompt_tokens = estimate_tokens(prompt)
        max_tokens = max(
            64,
            min(OUTPUT_MAX_TOKENS, MODEL_N_CTX - prompt_tokens - SAFETY_MARGIN_TOKENS),
        )

        response = self._model.create_chat_completion(
            messages=[{"role": "user", "content": prompt}],
            max_tokens=max_tokens,
            **GENERATION_CONFIG,
        )
        if (
            response
            and "choices" in response
            and len(response["choices"]) > 0
        ):
            content = response["choices"][0]["message"]["content"]
            if content and content.strip():
                return content.strip()
        return None

    def translate(
        self,
        text: str,
        target_lang: Optional[str] = None,
    ) -> Tuple[Optional[str], Optional[str]]:
        """
        翻译文本。未指定目标语言时，自动进行中英文互译。

        Args:
            text: 待翻译文本
            target_lang: 目标语言代码或名称，例如 vi、vi-VN、越南语

        Returns:
            (翻译结果, 错误信息) — 成功时错误信息为 None
        """
        if not self.ready:
            return None, self._status_message

        text = text.strip()
        if not text:
            return None, "文本为空"

        try:
            normalized_target = normalize_target_language(target_lang)
        except ValueError as error:
            return None, str(error)

        cache_key = (text, normalized_target or "auto")
        cached_result = self._get_cached_translation(cache_key)
        if cached_result is not None:
            logger.info(f"翻译缓存命中: {text[:30]}...")
            return cached_result, None

        try:
            if normalized_target:
                tgt_lang = LANG_NAMES[normalized_target]
                direction = f"目标语言={tgt_lang}"
                target_code = normalized_target
            else:
                # 保留现有前端依赖的中英自动互译行为。
                if self._is_chinese(text):
                    tgt_lang = LANG_NAMES["en"]
                    direction = "中→英"
                    target_code = "en"
                else:
                    tgt_lang = LANG_NAMES["zh"]
                    direction = "英→中"
                    target_code = "zh"

            # 构造翻译 prompt。长文先分块，避免上下文溢出报错与输出截断。
            chunks, split = _split_long_text(text)
            if split:
                logger.info(f"翻译长文分块: chunks={len(chunks)}")
                results = []
                for chunk in chunks:
                    part = self._complete(chunk, target_code)
                    if part is None:
                        return None, "翻译分块失败（模型返回空结果）"
                    results.append(part)
                result = "\n".join(results)
            else:
                result = self._complete(text, target_code)

            if result:
                self._store_cached_translation(cache_key, result)
                logger.info(f"翻译成功 [{direction}]{'（分块 %d）' % len(chunks) if split else ''}: {text[:30]}...")
                return result, None
            else:
                return None, "翻译返回空结果"

        except Exception as e:
            logger.exception(f"翻译失败: {text[:30]}...")
            return None, f"翻译出错: {e}"

    def chat_completion_stream(
        self,
        text: str,
        target_lang: Optional[str] = None,
    ):
        """流式输出翻译结果的生成器"""
        if not self.ready:
            yield "翻译引擎未就绪"
            return

        text = text.strip()
        if not text:
            return

        try:
            normalized_target = normalize_target_language(target_lang)
            if normalized_target:
                target_code = normalized_target
            else:
                target_code = "en" if self._is_chinese(text) else "zh"

            chunks, split = _split_long_text(text)
            for index, chunk in enumerate(chunks):
                if split and index > 0:
                    yield "\n"
                prompt = build_translation_prompt(target_code, chunk)
                prompt_tokens = estimate_tokens(prompt)
                max_tokens = max(
                    64,
                    min(OUTPUT_MAX_TOKENS, MODEL_N_CTX - prompt_tokens - SAFETY_MARGIN_TOKENS),
                )
                response_stream = self._model.create_chat_completion(
                    messages=[{"role": "user", "content": prompt}],
                    stream=True,
                    max_tokens=max_tokens,
                    **GENERATION_CONFIG,
                )

            for chunk in response_stream:
                if "choices" in chunk and len(chunk["choices"]) > 0:
                    delta = chunk["choices"][0].get("delta", {})
                    content = delta.get("content")
                    if content:
                        yield content

        except Exception as e:
            logger.exception(f"流式翻译失败: {text[:30]}...")
            yield f"\n[翻译出错: {e}]"

    def _is_chinese(self, text: str) -> bool:
        """
        判断文本是否主要为中文。
        如果中文字符占比超过 30%，视为中文文本。
        """
        if not text:
            return False
        cjk_chars = len(self._CJK_PATTERN.findall(text))
        # 去除空白字符后的总字符数
        total_chars = len(text.replace(" ", "").replace("\n", "").replace("\t", ""))
        if total_chars == 0:
            return False
        return (cjk_chars / total_chars) > 0.3

    def _get_cached_translation(self, cache_key) -> Optional[str]:
        """返回缓存中的翻译结果，并在命中时刷新 LRU 顺序。"""
        with self._cache_lock:
            cached_result = self._translation_cache.get(cache_key)
            if cached_result is None:
                return None

            self._translation_cache.move_to_end(cache_key)
            return cached_result

    def _store_cached_translation(self, cache_key, result: str):
        """缓存成功的翻译结果，避免重复文本反复推理。"""
        with self._cache_lock:
            self._translation_cache[cache_key] = result
            self._translation_cache.move_to_end(cache_key)

            while len(self._translation_cache) > CACHE_MAX_ENTRIES:
                self._translation_cache.popitem(last=False)

    def translate_async(self, text: str, callback):
        """
        异步翻译，在后台线程执行翻译并通过回调返回结果。

        Args:
            text: 待翻译文本
            callback: 回调函数 callback(original, result, error)
        """
        def worker():
            result, error = self.translate(text)
            callback(text, result, error)

        thread = threading.Thread(target=worker, daemon=True)
        thread.start()
