"""
translator.py — Hy-MT2-1.8B 翻译引擎模块

使用腾讯 Hy-MT2-1.8B 提供高质量离线翻译。
支持多语言翻译，未指定目标语言时默认进行中英文互译。
使用 llama-cpp-python 进行推理，支持 CPU 加速。
"""

import os
import sys
import re
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

# HuggingFace 镜像（国内加速）+ 绕过系统代理
HF_MIRROR = "https://hf-mirror.com"
os.environ["HF_ENDPOINT"] = HF_MIRROR
os.environ["NO_PROXY"] = "hf-mirror.com,huggingface.co"
os.environ["no_proxy"] = "hf-mirror.com,huggingface.co"

# Hy-MT2 推荐参数
MODEL_N_CTX = 1536
TRANSLATION_MAX_TOKENS = 384
CACHE_MAX_ENTRIES = 256

GENERATION_CONFIG = {
    "temperature": 0.7,
    "top_p": 0.6,
    "top_k": 20,
    "repeat_penalty": 1.05,
    "max_tokens": TRANSLATION_MAX_TOKENS,
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


def _create_no_proxy_session():
    """创建不使用代理的 requests session"""
    import requests
    session = requests.Session()
    session.trust_env = False
    session.proxies = {"http": "", "https": ""}
    return session


def _configure_huggingface_http():
    """Configure a direct HTTP client across huggingface_hub versions."""
    try:
        from huggingface_hub import configure_http_backend
    except ImportError:
        import httpx
        from huggingface_hub import set_client_factory

        set_client_factory(
            lambda: httpx.Client(
                trust_env=False,
                follow_redirects=True,
                timeout=None,
            )
        )
        return

    configure_http_backend(backend_factory=_create_no_proxy_session)


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
                # 检查模型是否已下载
                if not os.path.exists(MODEL_PATH):
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
        """下载 Hy-MT2 GGUF 模型"""
        from huggingface_hub import hf_hub_download

        update_status("正在从 HuggingFace 镜像下载模型...")
        os.makedirs(MODEL_DIR, exist_ok=True)

        # 禁用代理，直连镜像
        _configure_huggingface_http()

        # 下载模型到本地目录
        hf_hub_download(
            repo_id=MODEL_ID,
            filename=MODEL_FILENAME,
            local_dir=MODEL_DIR,
            cache_dir=os.path.join(os.path.dirname(MODEL_DIR), "cache"),
        )

        update_status("模型下载完成")

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

            # 构造翻译 prompt
            prompt = build_translation_prompt(target_code, text)
            messages = [{"role": "user", "content": prompt}]

            # 使用 llama.cpp 进行翻译
            response = self._model.create_chat_completion(
                messages=messages,
                **GENERATION_CONFIG,
            )

            # 提取翻译结果
            if response and "choices" in response and len(response["choices"]) > 0:
                result = response["choices"][0]["message"]["content"].strip()
                if result:
                    self._store_cached_translation(cache_key, result)
                    logger.info(f"翻译成功 [{direction}]: {text[:30]}...")
                    return result, None
                else:
                    return None, "翻译返回空结果"
            else:
                return None, "翻译返回无效响应"

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

            prompt = build_translation_prompt(target_code, text)
            messages = [{"role": "user", "content": prompt}]

            response_stream = self._model.create_chat_completion(
                messages=messages,
                stream=True,
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
