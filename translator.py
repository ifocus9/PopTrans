"""
translator.py — Hy-MT2-1.8B 翻译引擎模块

使用腾讯 Hy-MT2-1.8B 提供高质量离线翻译。
支持中英文互译，首次运行需下载模型（约 1.13GB）。
使用 llama-cpp-python 进行推理，支持 CPU 加速。
"""

import os
import sys
import re
import threading
import logging
from typing import Optional, Tuple

logger = logging.getLogger(__name__)

# ── 模型配置 ──────────────────────────────────────────────

MODEL_ID = "tencent/Hy-MT2-1.8B-GGUF"
MODEL_FILENAME = "Hy-MT2-1.8B-Q4_K_M.gguf"

# 支持 PyInstaller 打包：优先使用 exe 所在目录，否则使用脚本所在目录
if getattr(sys, 'frozen', False):
    # PyInstaller 打包后，模型放在 exe 同级目录
    _BASE_DIR = os.path.dirname(sys.executable)
else:
    _BASE_DIR = os.path.dirname(os.path.abspath(__file__))

MODEL_DIR = os.path.join(_BASE_DIR, "models", "Hy-MT2-1.8B-GGUF")
MODEL_PATH = os.path.join(MODEL_DIR, MODEL_FILENAME)

# HuggingFace 镜像（国内加速）+ 绕过系统代理
HF_MIRROR = "https://hf-mirror.com"
os.environ["HF_ENDPOINT"] = HF_MIRROR
os.environ["NO_PROXY"] = "hf-mirror.com,huggingface.co"
os.environ["no_proxy"] = "hf-mirror.com,huggingface.co"

# Hy-MT2 推荐参数
GENERATION_CONFIG = {
    "temperature": 0.7,
    "top_p": 0.6,
    "top_k": 20,
    "repeat_penalty": 1.05,
    "max_tokens": 4096,
}

# 语言名称映射
LANG_NAMES = {
    "zh": "中文",
    "en": "英语",
}


def _create_no_proxy_session():
    """创建不使用代理的 requests session"""
    import requests
    session = requests.Session()
    session.trust_env = False
    session.proxies = {"http": "", "https": ""}
    return session


# 翻译 prompt 模板
PROMPT_TEMPLATE = "将以下文本翻译为{target_lang}，注意只需要输出翻译后的结果，不要额外解释：\n\n{source_text}"


class Translator:
    """Hy-MT2-1.8B 翻译引擎封装（使用 llama-cpp-python）"""

    # 中文 Unicode 范围正则
    _CJK_PATTERN = re.compile(r'[\u4e00-\u9fff\u3400-\u4dbf\uf900-\ufaff]')

    def __init__(self):
        self.ready = False
        self._model = None
        self._setup_lock = threading.Lock()
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
        from huggingface_hub import hf_hub_download, configure_http_backend

        update_status("正在从 HuggingFace 镜像下载模型...")
        os.makedirs(MODEL_DIR, exist_ok=True)

        # 禁用代理，直连镜像
        configure_http_backend(backend_factory=lambda: _create_no_proxy_session())

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
            n_ctx=4096,  # 上下文窗口大小
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

    def translate(self, text: str) -> Tuple[Optional[str], Optional[str]]:
        """
        翻译文本，自动检测翻译方向。

        Args:
            text: 待翻译文本

        Returns:
            (翻译结果, 错误信息) — 成功时错误信息为 None
        """
        if not self.ready:
            return None, self._status_message

        text = text.strip()
        if not text:
            return None, "文本为空"

        try:
            # 自动检测翻译方向
            if self._is_chinese(text):
                tgt_lang = "英语"
                direction = "中→英"
            else:
                tgt_lang = "中文"
                direction = "英→中"

            # 构造翻译 prompt
            prompt = PROMPT_TEMPLATE.format(target_lang=tgt_lang, source_text=text)
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
                    logger.info(f"翻译成功 [{direction}]: {text[:30]}...")
                    return result, None
                else:
                    return None, "翻译返回空结果"
            else:
                return None, "翻译返回无效响应"

        except Exception as e:
            logger.exception(f"翻译失败: {text[:30]}...")
            return None, f"翻译出错: {e}"

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
