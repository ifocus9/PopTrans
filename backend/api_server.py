import io
import json
import logging
from contextlib import asynccontextmanager

from fastapi import FastAPI, UploadFile, File, HTTPException
from fastapi.responses import StreamingResponse
from PIL import Image

from backend.ocr_service import OCRService
from backend.runtime_paths import log_path, settings_path
from backend.server_config import server_port
from backend.translator import Translator


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
        "ocr_loaded": ocr_engine.is_loaded()
    }

@app.post("/api/v1/ocr")
async def recognize_ocr(file: UploadFile = File(...)):
    """接收图片并进行 OCR 识别"""
    if not file.content_type.startswith("image/"):
        raise HTTPException(status_code=400, detail="Must be an image file")

    try:
        content = await file.read()
        image = Image.open(io.BytesIO(content))
        # 确保图片格式正确，且不受限于 RGBA 问题
        if image.mode not in ("L", "RGB", "RGBA"):
            image = image.convert("RGB")
            
        result = ocr_engine.recognize_image(image)
        return result
    except Exception as e:
        logger.exception("OCR 处理异常")
        raise HTTPException(status_code=500, detail=str(e))

@app.post("/api/v1/ocr_translate")
async def recognize_and_translate_ocr(file: UploadFile = File(...)):
    """接收图片，OCR 识别后直接翻译并返回。"""
    if not file.content_type.startswith("image/"):
        raise HTTPException(status_code=400, detail="Must be an image file")
    if not translator_engine.ready:
        raise HTTPException(status_code=503, detail="Translator model is loading or not ready.")

    try:
        content = await file.read()
        image = Image.open(io.BytesIO(content))
        if image.mode not in ("L", "RGB", "RGBA"):
            image = image.convert("RGB")

        ocr_result = ocr_engine.recognize_image(image)
        source_text = (ocr_result.get("text") or "").strip()
        if not source_text:
            return {
                "source_text": "",
                "translation": "",
                "lines": ocr_result.get("lines", []),
                "scale": ocr_result.get("scale"),
            }

        translation, error = translator_engine.translate(source_text)
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

from pydantic import BaseModel
from typing import List, Optional

class ChatMessage(BaseModel):
    role: str
    content: str

class ChatCompletionRequest(BaseModel):
    messages: List[ChatMessage]
    stream: Optional[bool] = False

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

    if req.stream:
        # 如果需要流式输出，调用 translator_engine 新增的流式方法
        if hasattr(translator_engine, "chat_completion_stream"):
            generator = translator_engine.chat_completion_stream(user_msg)
            
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
        result, error = translator_engine.translate(user_msg)
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
