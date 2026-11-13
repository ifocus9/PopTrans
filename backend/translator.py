"""
translator.py — Hy-MT2-1.8B 翻译引擎模块

使用腾讯 Hy-MT2-1.8B 提供高质量离线翻译。
支持多语言翻译，未指定目标语言时默认进行中英文互译。
使用 llama-cpp-python 进行推理，支持 CPU 加速。
"""

import os
import sys
import re
import json
import time
import socket
import ipaddress
import threading
import logging
from collections import OrderedDict
from typing import Optional, Tuple
from urllib.parse import quote, urljoin, urlsplit

from backend.runtime_paths import application_dir, settings_path

logger = logging.getLogger(__name__)

# ── 模型配置 ──────────────────────────────────────────────

MODEL_SOURCE_MODELSCOPE = "modelscope"
MODEL_SOURCE_HUGGINGFACE = "huggingface"
MODEL_SOURCE_CUSTOM = "custom"
DEFAULT_MODEL_SOURCE = MODEL_SOURCE_MODELSCOPE
MODEL_SOURCE_LABELS = {
    MODEL_SOURCE_MODELSCOPE: "魔搭 ModelScope",
    MODEL_SOURCE_HUGGINGFACE: "HuggingFace 官网",
    MODEL_SOURCE_CUSTOM: "自定义镜像",
}

HUGGINGFACE_ENDPOINT = "https://huggingface.co"
MODELSCOPE_ENDPOINT = "https://modelscope.cn"
DEFAULT_MODEL_SIZE = 1133080448

# The builtin model, used when settings.json has no usable model catalogue.
_DEFAULT_MODEL_ID = "hy-mt2-1.8b-q4km"
_DEFAULT_REPO_MODELSCOPE = "Tencent-Hunyuan/Hy-MT2-1.8B-GGUF"
_DEFAULT_REPO_HUGGINGFACE = "tencent/Hy-MT2-1.8B-GGUF"
_DEFAULT_FILENAME = "Hy-MT2-1.8B-Q4_K_M.gguf"
_DEFAULT_DIR = "Hy-MT2-1.8B-GGUF"

_BASE_DIR = os.fspath(application_dir())


def _read_settings_json() -> dict:
    try:
        with open(settings_path(), "r", encoding="utf-8") as handle:
            data = json.load(handle)
    except (OSError, ValueError, TypeError):
        return {}
    return data if isinstance(data, dict) else {}


def _sanitize_segment(value: str) -> str:
    cleaned = []
    for char in str(value or "").strip():
        if char.isalnum() or char in "-_.":
            cleaned.append(char)
        else:
            cleaned.append("-")
    return "".join(cleaned).strip("-.")


def _normalize_repo(repo: str) -> str:
    repo = str(repo or "").replace("\\", "/").strip("/")
    parts = [part for part in repo.split("/") if part]
    if not parts or len(parts) > 2:
        return ""
    cleaned = [_sanitize_segment(part) for part in parts]
    if any(not part or part in (".", "..") for part in cleaned):
        return ""
    return "/".join(cleaned)


def _normalize_model_file(value: str) -> str:
    """Validate a repo-relative .gguf path (kept verbatim for download URLs)."""
    raw = str(value or "").replace("\\", "/").strip("/")
    parts = [part for part in raw.split("/") if part]
    if not parts or len(parts) > 4:
        return ""
    for part in parts:
        if part in (".", ".."):
            return ""
        if any(ord(char) < 0x20 or ord(char) == 0x7F or char == ":" for char in part):
            return ""
    result = "/".join(parts)
    return result if result.lower().endswith(".gguf") else ""


def _local_model_file(value: str) -> str:
    """Map a repo-relative path to a Windows-safe relative path."""
    parts = str(value or "").replace("\\", "/").split("/")
    cleaned = []
    for part in parts:
        chars = []
        for char in part:
            if ord(char) < 0x20 or ord(char) == 0x7F or char in '<>:"\\|?*':
                chars.append("_")
            else:
                chars.append(char)
        segment = "".join(chars).rstrip(". ")
        cleaned.append(segment or "_")
    return os.path.join(*cleaned) if cleaned else ""


def _active_model_spec() -> dict:
    """Resolve the active model from settings.json.

    Go owns the model catalogue; the engine only needs the active entry to pick
    the GGUF path and download coordinates. Falls back to the builtin model.
    """
    data = _read_settings_json()
    models = data.get("models")
    if not isinstance(models, list):
        models = []
    active_id = str(data.get("active_model_id") or "").strip()
    entry = None
    for item in models:
        if isinstance(item, dict) and str(item.get("id") or "") == active_id:
            entry = item
            break
    if entry is None and models and isinstance(models[0], dict):
        entry = models[0]
    if not isinstance(entry, dict):
        entry = {}

    entry_id = str(entry.get("id") or "").strip()
    entry_repo = _normalize_repo(entry.get("repo") or "")
    # The builtin model keeps honouring the global download-source preference
    # (ModelScope / HuggingFace / custom mirror) from the 常规 settings page.
    builtin_like = (
        not entry
        or entry_id == _DEFAULT_MODEL_ID
        or entry_repo in (_DEFAULT_REPO_MODELSCOPE, _DEFAULT_REPO_HUGGINGFACE)
    )
    configured_source = str(data.get("model_download_source") or DEFAULT_MODEL_SOURCE).lower()
    source = configured_source if builtin_like else str(entry.get("source") or "").lower()
    if source not in (MODEL_SOURCE_MODELSCOPE, MODEL_SOURCE_HUGGINGFACE, MODEL_SOURCE_CUSTOM):
        source = DEFAULT_MODEL_SOURCE
    repo = entry_repo
    filename = _normalize_model_file(entry.get("file") or "")
    if builtin_like:
        repo = (
            _DEFAULT_REPO_HUGGINGFACE
            if source in (MODEL_SOURCE_HUGGINGFACE, MODEL_SOURCE_CUSTOM)
            else _DEFAULT_REPO_MODELSCOPE
        )
        filename = _DEFAULT_FILENAME
    if not repo or not filename.lower().endswith(".gguf"):
        repo = _DEFAULT_REPO_HUGGINGFACE if source == MODEL_SOURCE_HUGGINGFACE else _DEFAULT_REPO_MODELSCOPE
        filename = _DEFAULT_FILENAME
        builtin_like = True
        legacy = True
    else:
        legacy = False
    if builtin_like or legacy:
        # Keep the historical folder so existing installs reuse their model.
        dir_name = _DEFAULT_DIR
    else:
        dir_name = _sanitize_segment(entry.get("dir") or repo.replace("/", "-")) or _DEFAULT_DIR
    try:
        size = int(entry.get("size") or 0)
    except (TypeError, ValueError):
        size = 0
    name = str(entry.get("name") or "").strip() or repo
    local_file = _local_model_file(filename)
    return {
        "id": str(entry.get("id") or ""),
        "name": name,
        "source": source,
        "repo": repo,
        "hf_repo": _DEFAULT_REPO_HUGGINGFACE if builtin_like else repo,
        "file": filename,
        "local_file": local_file,
        "dir": dir_name,
        "size": size if size > 0 else DEFAULT_MODEL_SIZE,
        "path": os.path.join(_BASE_DIR, "models", dir_name, *local_file.split("/")),
    }


_ACTIVE_MODEL = _active_model_spec()
MODEL_ID = _ACTIVE_MODEL["repo"]
MODEL_HF_ID = _ACTIVE_MODEL.get("hf_repo") or MODEL_ID
MODELSCOPE_MODEL_ID = _ACTIVE_MODEL["repo"]
MODEL_FILENAME = _ACTIVE_MODEL["file"]
MODEL_DIR = os.path.dirname(_ACTIVE_MODEL["path"])
MODEL_PATH = _ACTIVE_MODEL["path"]
MODEL_SOURCE = _ACTIVE_MODEL["source"]
MODEL_DISPLAY_NAME = _ACTIVE_MODEL["name"]
MODEL_EXPECTED_SIZE = _ACTIVE_MODEL["size"]

_MAX_MODEL_REDIRECTS = 5


class UnsafeModelURLError(ValueError):
    """Raised when a configured download URL points somewhere we refuse to fetch."""

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
    if normalized in ("auto", "none", "null"):
        return None
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


def normalize_source_language(source_lang) -> Optional[str]:
    """Return a supported source language code, or None for automatic detection."""
    if source_lang is None:
        return None
    value = str(source_lang).strip()
    if not value or value.lower() == "auto":
        return None
    return normalize_target_language(value)


# 翻译 prompt 模板
PROMPT_TEMPLATE = (
    "Translate the following text into {target_lang}. Note that you should "
    "only output the translated result without any additional explanation:\n\n"
    "{glossary}{source_text}"
)
PROMPT_TEMPLATE_WITH_SOURCE = (
    "Translate the following {source_lang} text into {target_lang}. Note that "
    "you should only output the translated result without any additional "
    "explanation:\n\n{glossary}{source_text}"
)


def _term_present(term: str, text: str) -> bool:
    if not term or not text:
        return False
    return term.casefold() in text.casefold()


def _glossary_for_text(text: str, glossary) -> list[dict]:
    return [entry for entry in (glossary or []) if _term_present(entry["term"], text)]


def _glossary_signature(text: str, glossary) -> tuple:
    entries = _glossary_for_text(text, glossary)
    return tuple(sorted((entry["term"].casefold(), entry["translation"]) for entry in entries))


def _glossary_section(entries: list[dict]) -> str:
    if not entries:
        return ""
    lines = "\n".join(f"- {entry['term']} => {entry['translation']}" for entry in entries)
    return "Use these glossary translations consistently:\n" + lines + "\n\n"


def _glossary_hits(text: str, glossary) -> list[dict]:
    return [
        {
            "term": entry["term"],
            "translation": entry["translation"],
            "source": entry.get("source", ""),
            "note": entry.get("note", ""),
        }
        for entry in _glossary_for_text(text, glossary)
    ]


def build_translation_prompt(
    target_lang: str,
    source_text: str,
    source_lang: Optional[str] = None,
    glossary=None,
) -> str:
    """Build the Hy-MT2 translation instruction, optionally fixing the source."""
    glossary_section = _glossary_section(_glossary_for_text(source_text, glossary))
    if source_lang and source_lang in LANG_NAMES_EN:
        return PROMPT_TEMPLATE_WITH_SOURCE.format(
            source_lang=LANG_NAMES_EN[source_lang],
            target_lang=LANG_NAMES_EN[target_lang],
            glossary=glossary_section,
            source_text=source_text,
        )
    return PROMPT_TEMPLATE.format(
        target_lang=LANG_NAMES_EN[target_lang],
        glossary=glossary_section,
        source_text=source_text,
    )


# 截图覆盖层需要逐行译文与 OCR 行框一一对应。编号行让模型在一次推理里
# 完成整屏翻译，避免每行一次调用带来的数十秒延迟。
LINES_PROMPT_TEMPLATE = (
    "Translate each numbered line below into {target_lang}. "
    "Keep exactly one output line per input line, in the same order, formatted "
    "as \"[n] translated text\". Do not merge, split, omit or explain anything.\n\n"
    "{glossary}{lines_block}"
)
LINES_PROMPT_TEMPLATE_WITH_SOURCE = (
    "Translate each numbered line below from {source_lang} into {target_lang}. "
    "Keep exactly one output line per input line, in the same order, formatted "
    "as \"[n] translated text\". Do not merge, split, omit or explain anything.\n\n"
    "{glossary}{lines_block}"
)

# The plain variant drops the "[n] " markers from both input and output. The
# markers cost 4-5 generated tokens per line (~20% of decode time on CPU), so
# the live overlay uses this prompt first and only falls back to the numbered
# prompt when the model changes the line count.
LINES_PROMPT_PLAIN = (
    "Translate each line below into {target_lang}. "
    "Output exactly {count} lines, one translation per input line, in the same "
    "order. Do not merge, split, omit, number or explain anything.\n\n"
    "{glossary}{lines_block}"
)
LINES_PROMPT_PLAIN_WITH_SOURCE = (
    "Translate each line below from {source_lang} into {target_lang}. "
    "Output exactly {count} lines, one translation per input line, in the same "
    "order. Do not merge, split, omit, number or explain anything.\n\n"
    "{glossary}{lines_block}"
)

_LINE_MARKER_RE = re.compile(r"^\s*(?:\[(\d+)\]|(\d+)[.、:：])\s*(.*)$")


def build_lines_prompt(
    target_lang: str,
    lines: list[str],
    source_lang: Optional[str] = None,
    glossary=None,
    numbered: bool = True,
) -> str:
    """Build the OCR line translation prompt.

    numbered=True prefixes each line with [n] and asks for the same markers;
    numbered=False asks for one plain line per input, which decodes faster.
    """
    if numbered:
        block = "\n".join(
            f"[{index + 1}] {line}" for index, line in enumerate(lines)
        )
    else:
        block = "\n".join(lines)
    glossary_section = _glossary_section(_glossary_for_text(block, glossary))
    if numbered:
        template = (
            LINES_PROMPT_TEMPLATE_WITH_SOURCE
            if source_lang and source_lang in LANG_NAMES_EN
            else LINES_PROMPT_TEMPLATE
        )
    else:
        template = (
            LINES_PROMPT_PLAIN_WITH_SOURCE
            if source_lang and source_lang in LANG_NAMES_EN
            else LINES_PROMPT_PLAIN
        )
    values = {
        "target_lang": LANG_NAMES_EN[target_lang],
        "glossary": glossary_section,
        "lines_block": block,
        "count": len(lines),
    }
    if source_lang and source_lang in LANG_NAMES_EN:
        values["source_lang"] = LANG_NAMES_EN[source_lang]
    return template.format(**values)


def parse_numbered_translations(content: str, expected: int) -> Optional[list[str]]:
    """Parse '[n] text' model output into a list in input order.

    Returns None when the output does not look line-numbered at all; callers
    then retry or fall back to per-line translation. Missing entries stay None.
    """
    if expected <= 0:
        return []
    results: list[Optional[str]] = [None] * expected
    matched = 0
    raw_lines = [line.strip() for line in (content or "").splitlines() if line.strip()]

    for line in raw_lines:
        marker = _LINE_MARKER_RE.match(line)
        if not marker:
            continue
        index = int(marker.group(1) or marker.group(2))
        text = marker.group(3).strip()
        if 1 <= index <= expected and text:
            results[index - 1] = text
            matched += 1

    if matched == 0:
        # Some models drop the markers but still emit one line per input.
        if len(raw_lines) == expected:
            return raw_lines
        return None
    return results


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


def _is_http_url(value: str) -> bool:
    try:
        parts = urlsplit(value.strip())
    except ValueError:
        return False
    return parts.scheme.lower() in ("http", "https") and bool(parts.hostname)


def _read_model_download_config() -> tuple[str, str]:
    """Read the configured download source, falling back to ModelScope.

    Mirrors config.NormalizeModelDownloadSource in Go so a hand-edited or
    legacy settings.json cannot break startup.
    """
    source = DEFAULT_MODEL_SOURCE
    base_url = ""
    try:
        cfg_file = settings_path()
        if cfg_file.is_file():
            data = json.loads(cfg_file.read_text(encoding="utf-8"))
            raw_source = str(data.get("model_download_source") or "").strip().lower()
            if raw_source in MODEL_SOURCE_LABELS:
                source = raw_source
            base_url = str(data.get("model_download_base_url") or "").strip()
    except Exception as e:
        logger.warning("读取模型下载源配置失败，使用默认 %s: %s", DEFAULT_MODEL_SOURCE, e)

    base_url = base_url.rstrip("/")
    if source == MODEL_SOURCE_CUSTOM and not _is_http_url(base_url):
        source = DEFAULT_MODEL_SOURCE
        base_url = ""
    return source, base_url


def _resolve_model_url(source: str, base_url: str = "") -> str:
    """Build the GGUF download URL for the configured source."""
    filename = "/".join(quote(part, safe="") for part in MODEL_FILENAME.split("/"))
    if source == MODEL_SOURCE_HUGGINGFACE:
        return f"{HUGGINGFACE_ENDPOINT}/{MODEL_HF_ID}/resolve/main/{filename}"
    if source == MODEL_SOURCE_CUSTOM:
        candidate = (base_url or "").rstrip("/")
        lowered = candidate.lower()
        # Accept either a complete file URL or a HuggingFace-style mirror root.
        if lowered.endswith(".gguf") or "/resolve/" in lowered:
            return candidate
        return f"{candidate}/{MODEL_HF_ID}/resolve/main/{filename}"
    return f"{MODELSCOPE_ENDPOINT}/models/{MODELSCOPE_MODEL_ID}/resolve/master/{filename}"


def _validate_model_url(url: str) -> str:
    """Reject non-HTTP and local/private/reserved download targets.

    This is a desktop app fetching a user-configurable URL, but the request is
    still server-side: keep it out of loopback, LAN and cloud-metadata ranges.
    """
    try:
        parts = urlsplit(url)
    except ValueError as error:
        raise UnsafeModelURLError(f"无效的下载地址: {error}") from error

    scheme = parts.scheme.lower()
    if scheme not in ("http", "https"):
        raise UnsafeModelURLError(f"仅支持 http/https 下载地址: {url}")

    host = parts.hostname
    if not host:
        raise UnsafeModelURLError(f"下载地址缺少主机名: {url}")

    lowered = host.lower()
    if lowered == "localhost" or lowered.endswith(".localhost"):
        raise UnsafeModelURLError(f"拒绝下载到本机地址: {host}")

    addresses = []
    try:
        addresses = [ipaddress.ip_address(host)]
    except ValueError:
        try:
            port = parts.port or (443 if scheme == "https" else 80)
            infos = socket.getaddrinfo(host, port, type=socket.SOCK_STREAM)
        except OSError as error:
            raise UnsafeModelURLError(f"无法解析下载地址主机 {host}: {error}") from error
        for info in infos:
            raw = str(info[4][0]).split("%", 1)[0]
            try:
                addresses.append(ipaddress.ip_address(raw))
            except ValueError:
                continue

    if not addresses:
        raise UnsafeModelURLError(f"下载地址无法解析: {host}")

    for address in addresses:
        if (
            address.is_loopback
            or address.is_private
            or address.is_link_local
            or address.is_reserved
            or address.is_multicast
            or address.is_unspecified
        ):
            raise UnsafeModelURLError(f"下载地址 {host} 指向回环/内网/保留地址，已拒绝")

    return url


def _request_with_validated_redirects(session, method: str, url: str, **kwargs):
    """Issue a request, validating every redirect hop before following it."""
    current = url
    for _ in range(_MAX_MODEL_REDIRECTS + 1):
        _validate_model_url(current)
        response = session.request(method, current, allow_redirects=False, **kwargs)
        if response.status_code in (301, 302, 303, 307, 308):
            location = response.headers.get("Location")
            response.close()
            if not location:
                raise IOError("下载服务器返回了缺少 Location 的重定向")
            current = urljoin(current, location)
            continue
        return response
    raise IOError("模型下载重定向次数过多")


def _read_cpu_threads() -> Optional[int]:
    """Read the user's llama.cpp CPU thread count (0/auto -> None)."""
    try:
        cfg_file = settings_path()
        if cfg_file.is_file():
            data = json.loads(cfg_file.read_text(encoding="utf-8"))
            threads = int(data.get("ai_threads", 0) or 0)
            if threads > 0:
                return min(threads, 256)
    except Exception as e:
        logger.warning("读取 CPU 线程配置失败，使用自动值: %s", e)
    return None


def _read_glossary() -> list[dict]:
    """Read the user's glossary from settings.json (term/translation/source)."""
    entries: list[dict] = []
    try:
        cfg_file = settings_path()
        if cfg_file.is_file():
            data = json.loads(cfg_file.read_text(encoding="utf-8"))
            raw = data.get("glossary") or []
            if isinstance(raw, list):
                for item in raw:
                    if not isinstance(item, dict):
                        continue
                    term = str(item.get("term") or "").strip()
                    translation = str(item.get("translation") or "").strip()
                    source = str(item.get("source") or "").strip()
                    note = str(item.get("note") or "").strip()
                    if not term or not translation:
                        continue
                    entries.append({
                        "term": term[:100],
                        "translation": translation[:200],
                        "source": source[:300],
                        "note": note[:300],
                    })
                    if len(entries) >= 500:
                        break
    except Exception as e:
        logger.warning("读取词汇本失败: %s", e)
    return entries


def _read_acceleration_preference() -> str:
    """读取用户配置的推理硬件偏好（auto / gpu / cpu）"""
    try:
        cfg_file = settings_path()
        if cfg_file.is_file():
            data = json.loads(cfg_file.read_text(encoding="utf-8"))
            dev = str(data.get("acceleration_device", "auto")).strip().lower()
            if dev in ("auto", "gpu", "cpu"):
                return dev
    except Exception as e:
        logger.warning("读取加速配置失败，使用默认 auto: %s", e)
    return "auto"


class Translator:
    """Hy-MT2-1.8B 翻译引擎封装（使用 llama-cpp-python）"""

    # 中文 Unicode 范围正则
    _CJK_PATTERN = re.compile(r'[\u4e00-\u9fff\u3400-\u4dbf\uf900-\ufaff]')

    def __init__(self):
        self.ready = False
        self._model = None
        self._setup_lock = threading.Lock()
        # llama.cpp contexts are not safe for concurrent inference; endpoints
        # now run in the threadpool (to overlap OCR with the LLM), so serialize
        # model calls explicitly.
        self._infer_lock = threading.Lock()
        self._cache_lock = threading.Lock()
        self._translation_cache = OrderedDict()
        self._status_message = "翻译引擎未初始化"
        self._configured_device = "auto"
        self._actual_device = "cpu"
        self._gpu_layers = 0

    @property
    def status(self) -> str:
        return self._status_message

    @property
    def device_info(self) -> dict:
        return {
            "configured": self._configured_device,
            "actual": self._actual_device,
            "gpu_layers": self._gpu_layers,
        }

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
                update_status(self._status_message)

                if on_ready:
                    on_ready(True)

            except Exception as e:
                error_msg = f"翻译引擎初始化失败: {e}"
                update_status(error_msg)
                logger.exception("翻译引擎初始化异常")
                if on_ready:
                    on_ready(False)

    def _download_model(self, update_status):
        """按激活模型的下载源下载 GGUF 模型（支持断点续传、重试）"""
        import requests

        # The model entry owns the source; the global preference only supplies a
        # custom mirror base URL (and legacy fallbacks).
        configured_source, base_url = _read_model_download_config()
        source = MODEL_SOURCE
        if source == MODEL_SOURCE_CUSTOM and not base_url:
            source, base_url = configured_source, ""
        if source == MODEL_SOURCE_CUSTOM and not base_url:
            source = DEFAULT_MODEL_SOURCE
        label = MODEL_SOURCE_LABELS.get(source, source)
        url = _resolve_model_url(source, base_url)
        try:
            _validate_model_url(url)
        except UnsafeModelURLError as e:
            update_status(f"模型下载地址不安全，已拒绝: {e}")
            raise RuntimeError(f"模型下载地址不安全，已拒绝: {e}") from e

        logger.info("模型下载源: model=%s source=%s url=%s", MODEL_DISPLAY_NAME, source, url)
        update_status(f"正在从{label}下载模型 {MODEL_DISPLAY_NAME}...")
        os.makedirs(os.path.dirname(MODEL_PATH) or MODEL_DIR, exist_ok=True)

        # 走系统代理访问下载源（默认 requests 行为），避免直连被网络阻断。
        # 证书校验保持开启；如需企业 CA，请通过 REQUESTS_CA_BUNDLE 指定。
        session = requests.Session()

        # 探测文件大小（最多重试 5 次）
        total_size = MODEL_EXPECTED_SIZE
        for attempt in range(1, 6):
            try:
                resp = _request_with_validated_redirects(
                    session,
                    "HEAD",
                    url,
                    timeout=(15, 30),
                )
                resp.raise_for_status()
                try:
                    size = int(resp.headers.get("Content-Length", total_size))
                except (ValueError, TypeError):
                    size = total_size
                if size >= 1024 * 1024:
                    total_size = size
                resp.close()
                break
            except UnsafeModelURLError as e:
                update_status(f"模型下载地址不安全，已拒绝: {e}")
                raise RuntimeError(f"模型下载地址不安全，已拒绝: {e}") from e
            except Exception as e:
                logger.warning("探测模型大小第 %d/5 次失败: %s", attempt, e)
                if attempt == 5:
                    update_status("无法获取模型大小，使用默认 1.13GB 继续下载")
                else:
                    update_status(f"连接下载源失败，正在重试（{attempt}/5）...")
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
                response = _request_with_validated_redirects(
                    session,
                    "GET",
                    url,
                    stream=True,
                    headers=headers,
                    timeout=(15, 300),
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

            except UnsafeModelURLError as e:
                update_status(f"模型下载地址不安全，已拒绝: {e}")
                raise RuntimeError(f"模型下载地址不安全，已拒绝: {e}") from e
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
        """加载 llama-cpp-python 模型，支持根据配置启用显卡加速并在异常时自动回退 CPU"""
        # PyInstaller 打包后，需要手动设置 DLL 路径
        if getattr(sys, 'frozen', False):
            import ctypes
            _internal_dir = os.path.join(os.path.dirname(sys.executable), '_internal')
            _lib_dir = os.path.join(_internal_dir, 'llama_cpp', 'lib')
            if os.path.exists(_lib_dir):
                os.add_dll_directory(_lib_dir)
                os.environ["PATH"] = _lib_dir + os.pathsep + os.environ.get("PATH", "")

        from llama_cpp import Llama, llama_supports_gpu_offload

        preference = _read_acceleration_preference()
        self._configured_device = preference
        configured_threads = _read_cpu_threads()
        threads = configured_threads or os.cpu_count() or 4
        logger.info(
            "llama.cpp CPU 线程: %d（%s）",
            threads,
            "用户指定" if configured_threads else "自动",
        )

        can_gpu = False
        try:
            can_gpu = bool(llama_supports_gpu_offload())
        except Exception:
            can_gpu = False

        logger.info(
            "加载翻译模型: 配置偏好=%s, 环境支持GPU卸载=%s",
            preference,
            can_gpu,
        )

        if preference == "cpu":
            logger.info("用户配置使用 CPU 推理")
            self._model = Llama(
                model_path=MODEL_PATH,
                n_ctx=MODEL_N_CTX,
                n_gpu_layers=0,
                n_threads=threads,
                verbose=False,
            )
            self._actual_device = "cpu"
            self._gpu_layers = 0
            self._status_message = "翻译引擎就绪 (CPU)"
            return

        if not can_gpu:
            if preference == "gpu":
                logger.warning("用户配置为 GPU 模式，但当前 llama-cpp 运行库未启用 GPU 支持，降级使用 CPU")
            else:
                logger.info("当前环境未检测到 GPU 卸载支持，使用 CPU 推理")
            self._model = Llama(
                model_path=MODEL_PATH,
                n_ctx=MODEL_N_CTX,
                n_gpu_layers=0,
                n_threads=threads,
                verbose=False,
            )
            self._actual_device = "cpu"
            self._gpu_layers = 0
            self._status_message = "翻译引擎就绪 (CPU)"
            return

        # 尝试使用 GPU 加载
        try:
            logger.info("尝试启用显卡 (GPU) 加速加载模型 (n_gpu_layers=-1)...")
            self._model = Llama(
                model_path=MODEL_PATH,
                n_ctx=MODEL_N_CTX,
                n_gpu_layers=-1,
                n_threads=threads,
                verbose=False,
            )
            self._actual_device = "gpu"
            self._gpu_layers = -1
            self._status_message = "翻译引擎就绪 (GPU 加速)"
            logger.info("模型成功加载至 GPU")
        except Exception as gpu_err:
            logger.warning("显卡 (GPU) 加载失败 (%s)，安全降级至纯 CPU 模式...", gpu_err)
            self._model = Llama(
                model_path=MODEL_PATH,
                n_ctx=MODEL_N_CTX,
                n_gpu_layers=0,
                n_threads=threads,
                verbose=False,
            )
            self._actual_device = "cpu"
            self._gpu_layers = 0
            self._status_message = "翻译引擎就绪 (CPU 降级)"


    def close(self):
        """Release the loaded llama.cpp model before application exit."""
        self.ready = False
        model = self._model
        self._model = None
        if model and hasattr(model, "close"):
            model.close()
    # ── 翻译 ──────────────────────────────────────────────

    def _complete_prompt(self, prompt: str) -> Optional[str]:
        """Run one chat completion with a context-aware output budget."""
        prompt_tokens = estimate_tokens(prompt)
        max_tokens = max(
            64,
            min(OUTPUT_MAX_TOKENS, MODEL_N_CTX - prompt_tokens - SAFETY_MARGIN_TOKENS),
        )

        with self._infer_lock:
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

    def _complete(
        self,
        source_text: str,
        target_code: str,
        source_code: Optional[str] = None,
        glossary=None,
    ) -> Optional[str]:
        """单次调用模型完成一段文本的翻译，按剩余上下文动态分配输出上限。

        max_tokens 不再写死：短文本沿用接近 384 的默认，长文分块可获得更大
        输出空间，同时不会超过 n_ctx 导致上下文溢出。
        """
        return self._complete_prompt(
            build_translation_prompt(
                target_code, source_text, source_code, glossary
            )
        )

    def translate(
        self,
        text: str,
        target_lang: Optional[str] = None,
        source_lang: Optional[str] = None,
    ) -> Tuple[Optional[str], Optional[str]]:
        """
        翻译文本。未指定目标语言时，自动进行中英文互译。

        Args:
            text: 待翻译文本
            target_lang: 目标语言代码或名称，例如 vi、vi-VN、越南语
            source_lang: 原文语言代码；为空/auto 时自动判断

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
            normalized_source = normalize_source_language(source_lang)
        except ValueError as error:
            return None, str(error)

        glossary = _read_glossary()
        cache_key = (
            text,
            normalized_target or "auto",
            normalized_source or "auto",
            _glossary_signature(text, glossary),
        )
        cached_result = self._get_cached_translation(cache_key)
        if cached_result is not None:
            logger.info(f"翻译缓存命中: {text[:30]}...")
            return cached_result, None

        try:
            if normalized_target:
                tgt_lang = LANG_NAMES[normalized_target]
                direction = f"目标语言={tgt_lang}"
                target_code = normalized_target
            elif normalized_source:
                if normalized_source in ("zh", "zh-hant", "yue"):
                    target_code = "en"
                else:
                    target_code = "zh"
                tgt_lang = LANG_NAMES[target_code]
                direction = f"{LANG_NAMES[normalized_source]}→{tgt_lang}"
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
                    part = self._complete(
                        chunk, target_code, normalized_source, glossary
                    )
                    if part is None:
                        return None, "翻译分块失败（模型返回空结果）"
                    results.append(part)
                result = "\n".join(results)
            else:
                result = self._complete(
                    text, target_code, normalized_source, glossary
                )

            if result:
                self._store_cached_translation(cache_key, result)
                logger.info(f"翻译成功 [{direction}]{'（分块 %d）' % len(chunks) if split else ''}: {text[:30]}...")
                return result, None
            else:
                return None, "翻译返回空结果"

        except Exception as e:
            logger.exception(f"翻译失败: {text[:30]}...")
            return None, f"翻译出错: {e}"

    def translate_lines(
        self,
        lines: list[str],
        target_lang: Optional[str] = None,
        source_lang: Optional[str] = None,
        use_cache: bool = True,
    ) -> Tuple[list[str], Optional[str]]:
        """Translate OCR lines while keeping one result per input line.

        Returns (translations, error). Untranslatable lines are returned as
        empty strings so callers can fall back to the source text for that box.
        use_cache=False forces a fresh model call for every line (the manual
        refresh button), while still updating the cache with the new result.
        """
        if not self.ready:
            return [], self._status_message

        cleaned = [str(line).strip() for line in (lines or [])]
        if not cleaned:
            return [], None

        try:
            normalized_target = normalize_target_language(target_lang)
            normalized_source = normalize_source_language(source_lang)
        except ValueError as error:
            return [], str(error)

        joined = "\n".join(line for line in cleaned if line)
        if normalized_target:
            target_code = normalized_target
        elif normalized_source:
            target_code = (
                "en" if normalized_source in ("zh", "zh-hant", "yue") else "zh"
            )
        else:
            target_code = "en" if self._is_chinese(joined) else "zh"

        glossary = _read_glossary()
        results = ["" for _ in cleaned]
        pending: list[int] = []
        for index, line in enumerate(cleaned):
            if not line:
                continue
            cached = None
            if use_cache:
                cached = self._get_cached_translation(
                    (
                        line,
                        target_code,
                        normalized_source or "auto",
                        _glossary_signature(line, glossary),
                    )
                )
            if cached is not None:
                results[index] = cached
            else:
                pending.append(index)

        if not pending:
            return results, None

        # Group uncached lines into batches that fit the context window.
        batches: list[list[int]] = []
        current: list[int] = []
        current_tokens = 0
        for index in pending:
            cost = estimate_tokens(cleaned[index]) + 8
            if current and current_tokens + cost > SOURCE_MAX_TOKENS:
                batches.append(current)
                current = []
                current_tokens = 0
            current.append(index)
            current_tokens += cost
        if current:
            batches.append(current)

        fallback_calls = 0
        max_fallback_calls = 8

        for batch in batches:
            batch_texts = [cleaned[index] for index in batch]
            # Plain one-line-per-input output avoids the "[n] " markers, which
            # cost several generated tokens per line and dominate CPU decode
            # time. If the model merges or splits lines, fall back to the
            # numbered prompt so alignment stays exact.
            plain_prompt = build_lines_prompt(
                target_code,
                batch_texts,
                normalized_source,
                glossary,
                numbered=False,
            )
            content = self._complete_prompt(plain_prompt)
            parsed = parse_numbered_translations(content or "", len(batch))
            numbered_prompt = ""

            if parsed is None or any(value is None for value in parsed):
                numbered_prompt = build_lines_prompt(
                    target_code, batch_texts, normalized_source, glossary
                )
                numbered_content = self._complete_prompt(numbered_prompt)
                numbered_parsed = parse_numbered_translations(
                    numbered_content or "", len(batch)
                )
                if numbered_parsed is not None:
                    if parsed is None:
                        parsed = numbered_parsed
                    else:
                        parsed = [
                            old if old is not None else new
                            for old, new in zip(parsed, numbered_parsed)
                        ]

            # One retry with the same strict instruction before per-line calls.
            if parsed is None or any(value is None for value in parsed):
                retry_content = self._complete_prompt(numbered_prompt)
                retry_parsed = parse_numbered_translations(
                    retry_content or "", len(batch)
                )
                if retry_parsed is not None:
                    if parsed is None:
                        parsed = retry_parsed
                    else:
                        parsed = [
                            old if old is not None else new
                            for old, new in zip(parsed, retry_parsed)
                        ]

            for local_index, source_index in enumerate(batch):
                value = parsed[local_index] if parsed is not None else None
                if value:
                    results[source_index] = value
                    self._store_cached_translation(
                        (
                            cleaned[source_index],
                            target_code,
                            normalized_source or "auto",
                            _glossary_signature(
                                cleaned[source_index], glossary
                            ),
                        ),
                        value,
                    )
                elif fallback_calls < max_fallback_calls:
                    fallback_calls += 1
                    single = self._complete(
                        cleaned[source_index],
                        target_code,
                        normalized_source,
                        glossary,
                    )
                    if single:
                        results[source_index] = single
                        self._store_cached_translation(
                            (
                                cleaned[source_index],
                                target_code,
                                normalized_source or "auto",
                                _glossary_signature(
                                    cleaned[source_index], glossary
                                ),
                            ),
                            single,
                        )

        return results, None

    def chat_completion_stream(
        self,
        text: str,
        target_lang: Optional[str] = None,
        source_lang: Optional[str] = None,
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
            normalized_source = normalize_source_language(source_lang)
            if normalized_target:
                target_code = normalized_target
            elif normalized_source:
                target_code = (
                    "en"
                    if normalized_source in ("zh", "zh-hant", "yue")
                    else "zh"
                )
            else:
                target_code = "en" if self._is_chinese(text) else "zh"

            glossary = _read_glossary()
            chunks, split = _split_long_text(text)
            for index, chunk in enumerate(chunks):
                if split and index > 0:
                    yield "\n"
                prompt = build_translation_prompt(
                    target_code, chunk, normalized_source, glossary
                )
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

            with self._infer_lock:
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
