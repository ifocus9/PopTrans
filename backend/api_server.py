import io
import json
import logging
from contextlib import asynccontextmanager
from typing import List, Optional

from fastapi import FastAPI, UploadFile, File, Form, HTTPException
from pydantic import BaseModel
from fastapi.responses import StreamingResponse
from PIL import Image

from backend.ocr_service import OCRService
from backend.runtime_paths import log_path, settings_path
from backend.server_config import server_port
from backend.translator import (
    MODEL_DISPLAY_NAME,
    MODEL_PATH,
    MODEL_SOURCE,
    Translator,
    _glossary_hits,
    _read_glossary,
    normalize_source_language,
    normalize_target_language,
)


def configure_logging():
    try:
        settings = json.loads(settings_path().read_text(encoding="utf-8"))
        enabled = bool(settings.get("logging_enabled", False))
    except (OSError, ValueError, TypeError):
        enabled = False

    if not enabled:
        logging.basicConfig(
            level=logging.CRITICAL + 1,
            handlers=[logging.NullHandler()],
            force=True,
        )
        return

    handler = logging.FileHandler(
        log_path("translate.log"),
        mode="a",
        encoding="utf-8",
    )
    logging.basicConfig(
        level=logging.INFO,
        format="%(asctime)s [ai-engine] %(levelname)s %(name)s: %(message)s",
        handlers=[handler],
        force=True,
    )


configure_logging()
logger = logging.getLogger(__name__)

# 全局单例服务
translator_engine = Translator()
ocr_engine = OCRService()

@asynccontextmanager
async def lifespan(app: FastAPI):
    # 启动时初始化
    logger.info("正在初始化 AI 模型...")
    # translator.setup 在后台线程中加载
    translator_engine.setup()
    # OCR 是懒加载的，但可以预热
    # ocr_engine._ensure_engine()
    yield
    # 关闭时清理
    logger.info("正在关闭 AI 模型...")
    translator_engine.close()
    ocr_engine.release()

app = FastAPI(title="Translate API", lifespan=lifespan)

@app.get("/health")
def health_check():
    """检查模型是否准备就绪"""
    return {
        "status": "ok",
        "translator_ready": translator_engine.ready,
        "translator_status": translator_engine.status,
        "device_info": translator_engine.device_info,
        "ocr_loaded": ocr_engine.is_loaded(),
        "model_name": MODEL_DISPLAY_NAME,
        "model_path": MODEL_PATH,
        "model_source": MODEL_SOURCE,
    }

@app.post("/api/v1/ocr")
def recognize_ocr(
    file: UploadFile = File(...),
    allow_retry: bool = Form(True),
    source_lang: Optional[str] = Form(None),
):
    """接收图片并进行 OCR 识别（source_lang=ko 时使用韩语识别模型）"""
    if not (file.content_type or "").startswith("image/"):
        raise HTTPException(status_code=400, detail="Must be an image file")
    try:
        normalize_source_language(source_lang)
    except ValueError as error:
        raise HTTPException(status_code=400, detail=str(error))

    try:
        content = file.file.read()
        image = Image.open(io.BytesIO(content))
        # 确保图片格式正确，且不受限于 RGBA 问题
        if image.mode not in ("L", "RGB", "RGBA"):
            image = image.convert("RGB")
            
        result = ocr_engine.recognize_image(
            image, allow_retry=allow_retry, source_lang=source_lang
        )
        return result
    except Exception as e:
        logger.exception("OCR 处理异常")
        raise HTTPException(status_code=500, detail=str(e))

@app.post("/api/v1/ocr_translate")
async def recognize_and_translate_ocr(
    file: UploadFile = File(...),
    target_lang: Optional[str] = Form(None),
    source_lang: Optional[str] = Form(None),
):
    """接收图片，OCR 识别后直接翻译并返回。"""
    if not file.content_type.startswith("image/"):
        raise HTTPException(status_code=400, detail="Must be an image file")
    try:
        normalize_target_language(target_lang)
        normalize_source_language(source_lang)
    except ValueError as error:
        raise HTTPException(status_code=400, detail=str(error))
    if not translator_engine.ready:
        raise HTTPException(status_code=503, detail="Translator model is loading or not ready.")

    try:
        content = await file.read()
        image = Image.open(io.BytesIO(content))
        if image.mode not in ("L", "RGB", "RGBA"):
            image = image.convert("RGB")

        ocr_result = ocr_engine.recognize_image(image, source_lang=source_lang)
        source_text = (ocr_result.get("text") or "").strip()
        if not source_text:
            return {
                "source_text": "",
                "translation": "",
                "lines": ocr_result.get("lines", []),
                "scale": ocr_result.get("scale"),
            }

        translation, error = translator_engine.translate(
            source_text, target_lang, source_lang
        )
        if error:
            raise HTTPException(status_code=500, detail=error)

        return {
            "source_text": source_text,
            "translation": translation,
            "lines": ocr_result.get("lines", []),
            "scale": ocr_result.get("scale"),
        }
    except HTTPException:
        raise
    except Exception as e:
        logger.exception("OCR 翻译处理异常")
        raise HTTPException(status_code=500, detail=str(e))


def _translate_line_boxes(
    line_boxes: List[dict],
    source_text: str,
    image_width: int,
    image_height: int,
    target_lang: Optional[str],
    source_lang: Optional[str],
    force: bool = False,
) -> dict:
    """Translate OCR line boxes and return the overlay block payload."""
    translations: list[str] = []
    if line_boxes:
        translations, error = translator_engine.translate_lines(
            [block.get("text", "") for block in line_boxes],
            target_lang,
            source_lang,
            use_cache=not force,
        )
        if error:
            raise HTTPException(status_code=500, detail=error)
    elif source_text:
        translated, error = translator_engine.translate(
            source_text, target_lang, source_lang
        )
        if error:
            raise HTTPException(status_code=500, detail=error)
        translations = [translated or ""]

    glossary = _read_glossary()
    blocks = []
    for index, block in enumerate(line_boxes):
        translation = translations[index] if index < len(translations) else ""
        block_text = block.get("text", "")
        blocks.append({
            "text": block_text,
            "translation": translation or "",
            "x": block.get("x", 0),
            "y": block.get("y", 0),
            "w": block.get("w", 0),
            "h": block.get("h", 0),
            "score": block.get("score", 0),
            "angle": block.get("angle", 0),
            "cx": block.get("cx", 0),
            "cy": block.get("cy", 0),
            "line_width": block.get("line_width", 0),
            "line_height": block.get("line_height", 0),
            "glossary_hits": _glossary_hits(block_text, glossary),
        })

    full_translation = "\n".join(
        block["translation"] for block in blocks if block["translation"]
    ).strip()

    # If per-line translation produced nothing (model returned unusable
    # numbering), fall back to translating the whole text at once.
    if source_text and not full_translation:
        fallback, fallback_error = translator_engine.translate(
            source_text, target_lang, source_lang
        )
        if fallback_error:
            raise HTTPException(status_code=500, detail=fallback_error)
        full_translation = (fallback or "").strip()

    return {
        "source_text": source_text,
        "translation": full_translation,
        "blocks": blocks,
        "image_width": image_width,
        "image_height": image_height,
        "scale": 1.0,
    }


@app.post("/api/v1/ocr_translate_blocks")
def recognize_and_translate_blocks(
    file: UploadFile = File(...),
    target_lang: Optional[str] = Form(None),
    source_lang: Optional[str] = Form(None),
):
    """OCR + per-line translation with bounding boxes for the screenshot overlay."""
    if not (file.content_type or "").startswith("image/"):
        raise HTTPException(status_code=400, detail="Must be an image file")
    try:
        normalize_target_language(target_lang)
        normalize_source_language(source_lang)
    except ValueError as error:
        raise HTTPException(status_code=400, detail=str(error))
    if not translator_engine.ready:
        raise HTTPException(status_code=503, detail="Translator model is loading or not ready.")

    try:
        content = file.file.read()
        image = Image.open(io.BytesIO(content))
        if image.mode not in ("L", "RGB", "RGBA"):
            image = image.convert("RGB")

        ocr_result = ocr_engine.recognize_image(image, source_lang=source_lang)
        return _translate_line_boxes(
            ocr_result.get("line_boxes") or [],
            (ocr_result.get("text") or "").strip(),
            ocr_result.get("image_width") or image.size[0],
            ocr_result.get("image_height") or image.size[1],
            target_lang,
            source_lang,
        )
    except HTTPException:
        raise
    except Exception as e:
        logger.exception("OCR 分块翻译处理异常")
        raise HTTPException(status_code=500, detail=str(e))


class TranslateBlocksBody(BaseModel):
    lines: Optional[List[dict]] = None
    source_text: str = ""
    image_width: int = 0
    image_height: int = 0
    source_lang: Optional[str] = None
    target_lang: Optional[str] = None
    # force=True bypasses the line cache so the manual refresh button gets a
    # fresh model result even when the OCR text is unchanged.
    force: bool = False


@app.post("/api/v1/translate_blocks")
def translate_blocks(body: TranslateBlocksBody):
    """Translate already-recognized line boxes (used to pipeline OCR + LLM)."""
    try:
        normalize_target_language(body.target_lang)
        normalize_source_language(body.source_lang)
    except ValueError as error:
        raise HTTPException(status_code=400, detail=str(error))
    if not translator_engine.ready:
        raise HTTPException(status_code=503, detail="Translator model is loading or not ready.")
    lines = [block for block in (body.lines or []) if isinstance(block, dict)]
    if len(lines) > 300:
        raise HTTPException(status_code=400, detail="Too many lines")
    try:
        return _translate_line_boxes(
            lines,
            (body.source_text or "").strip(),
            body.image_width,
            body.image_height,
            body.target_lang,
            body.source_lang,
            force=body.force,
        )
    except HTTPException:
        raise
    except Exception as e:
        logger.exception("分块翻译处理异常")
        raise HTTPException(status_code=500, detail=str(e))

class ChatMessage(BaseModel):
    role: str
    content: str

class ChatCompletionRequest(BaseModel):
    messages: List[ChatMessage]
    stream: Optional[bool] = False
    target_lang: Optional[str] = None
    source_lang: Optional[str] = None

@app.post("/v1/chat/completions")
async def chat_completions(req: ChatCompletionRequest):
    """OpenAI 兼容的对话/翻译接口"""
    if not translator_engine.ready:
        raise HTTPException(status_code=503, detail="Translator model is loading or not ready.")

    if not req.messages:
        raise HTTPException(status_code=400, detail="Messages cannot be empty")

    # 提取最后一条用户消息
    user_msg = next((m.content for m in reversed(req.messages) if m.role == "user"), None)
    if not user_msg:
        raise HTTPException(status_code=400, detail="No user message found")

    try:
        normalize_target_language(req.target_lang)
        normalize_source_language(req.source_lang)
    except ValueError as error:
        raise HTTPException(status_code=400, detail=str(error))

    if req.stream:
        # 如果需要流式输出，调用 translator_engine 新增的流式方法
        if hasattr(translator_engine, "chat_completion_stream"):
            generator = translator_engine.chat_completion_stream(
                user_msg,
                req.target_lang,
                req.source_lang,
            )
            
            def stream_openai_format():
                for chunk in generator:
                    data = {
                        "choices": [{"delta": {"content": chunk}}]
                    }
                    yield f"data: {json.dumps(data)}\n\n"
                yield "data: [DONE]\n\n"
                
            return StreamingResponse(stream_openai_format(), media_type="text/event-stream")
        else:
            raise HTTPException(status_code=501, detail="Streaming not implemented in translator")
    else:
        # 普通翻译
        result, error = translator_engine.translate(
            user_msg, req.target_lang, req.source_lang
        )
        if error:
            raise HTTPException(status_code=500, detail=error)
            
        return {
            "choices": [
                {
                    "message": {
                        "role": "assistant",
                        "content": result
                    }
                }
            ]
        }

def main():
    import uvicorn

    uvicorn.run(
        app,
        host="127.0.0.1",
        port=server_port(),
        access_log=False,
        log_config=None,
    )


if __name__ == "__main__":
    main()
