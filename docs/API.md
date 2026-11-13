# 本地 HTTP API 接口说明

AI 引擎（`ai_engine.exe` / `backend/api_server.py`）基于 FastAPI 提供本地 HTTP 服务，供 Go 启动器、Wails 界面以及第三方程序调用。

- **基础地址**：`http://127.0.0.1:<server_port>`
- **默认端口**：`8989`（可在设置中修改，范围 `1024-65535`）
- **监听地址**：仅 `127.0.0.1`（本机回环，不对外网开放）
- **数据格式**：JSON（图片接口使用 `multipart/form-data`）
- **鉴权**：无

> 端口取自 `settings.json` 的 `server_port` 字段，修改端口后应用会自动重启本地服务。

## 接口总览

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| `GET` | `/health` | 健康检查，查询模型加载状态 |
| `POST` | `/api/v1/ocr` | 图片 OCR 文字识别（可返回行框） |
| `POST` | `/api/v1/ocr_translate` | 图片 OCR 识别并翻译 |
| `POST` | `/api/v1/ocr_translate_blocks` | 图片 OCR + 逐行翻译（截图悬浮层） |
| `POST` | `/api/v1/translate_blocks` | 翻译已识别的行框（实时翻译） |
| `POST` | `/v1/chat/completions` | OpenAI 兼容的翻译接口（支持流式） |

---

## GET /health

检查翻译与 OCR 模型是否就绪。通常用于启动器轮询等待模型加载完成。

### 请求参数

无。

### 返回参数

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `status` | string | 固定为 `ok`，表示服务进程存活 |
| `translator_ready` | bool | 翻译模型是否加载完成、可以翻译 |
| `translator_status` | string | 翻译模型当前状态描述文本（如加载进度/下载提示） |
| `ocr_loaded` | bool | OCR 引擎是否已加载（懒加载，首次识别后为 `true`） |
| `model_name` | string | 当前激活模型的显示名称 |
| `model_path` | string | 当前加载的 GGUF 文件绝对路径 |
| `model_source` | string | 当前模型的下载来源（`modelscope` / `huggingface` / `custom`） |

### 返回示例

```json
{
  "status": "ok",
  "translator_ready": true,
  "translator_status": "模型已就绪",
  "ocr_loaded": false,
  "model_name": "Tencent-Hunyuan/Hy-MT2-1.8B-GGUF",
  "model_path": "C:\\PopTrans\\models\\Hy-MT2-1.8B-GGUF\\Hy-MT2-1.8B-Q4_K_M.gguf",
  "model_source": "modelscope"
}
```

### 请求示例

```bash
curl http://127.0.0.1:8989/health
```

---

## POST /api/v1/ocr

上传一张图片，返回 OCR 识别的文字，不做翻译。

### 请求

- **Content-Type**：`multipart/form-data`

| 参数 | 位置 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- | --- |
| `file` | form-data | file | 是 | 图片文件，`Content-Type` 必须以 `image/` 开头 |
| `allow_retry` | form-data | bool | 否 | 默认 `true`。识别质量偏低时会再放大识别一次；实时翻译传 `false` 跳过第二轮，下一帧会重新识别 |

### 返回参数

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `text` | string | 识别出的完整文本，多行以 `\n` 拼接 |
| `lines` | string[] | 按行分组后的文本，每个元素为一行 |
| `line_boxes` | object[] | 每个 OCR 行的文本、置信度与像素坐标（含倾斜行几何） |
| `line_boxes[].angle` | number | 文本行角度（度）；负值表示向右上倾斜 |
| `line_boxes[].cx/cy` | number | 旋转行框中心点 |
| `line_boxes[].line_width/line_height` | number | 旋转行框沿文字方向/法线方向的长度 |
| `scale` | number | 识别时实际采用的图像缩放系数 |
| `image_width/image_height` | number | 图片像素尺寸 |

### 返回示例

```json
{
  "text": "Hello World\n你好世界",
  "lines": ["Hello World", "你好世界"],
  "scale": 1.5
}
```

### 错误响应

| 状态码 | 触发条件 | `detail` |
| --- | --- | --- |
| `400` | 上传文件不是图片 | `Must be an image file` |
| `500` | 识别过程异常 | 具体异常信息 |

### 请求示例

```bash
curl -X POST http://127.0.0.1:8989/api/v1/ocr \
  -F "file=@screenshot.png"
```

---

## POST /api/v1/ocr_translate

上传一张图片，先做 OCR 识别，再将识别文本翻译后一并返回。

### 请求

- **Content-Type**：`multipart/form-data`

| 参数 | 位置 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- | --- |
| `file` | form-data | file | 是 | 图片文件，`Content-Type` 必须以 `image/` 开头 |

### 返回参数

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `source_text` | string | OCR 识别出的原文（已 `strip`）；无文字时为空字符串 |
| `translation` | string | 原文的翻译结果；`source_text` 为空时为空字符串 |
| `lines` | string[] | OCR 按行分组的文本 |
| `scale` | number \| null | 识别时采用的图像缩放系数 |

> 当图片中未识别到任何文字时，`source_text` 与 `translation` 均为 `""`，接口仍返回 `200`。

### 返回示例

```json
{
  "source_text": "Hello World",
  "translation": "你好世界",
  "lines": ["Hello World"],
  "scale": 1.5
}
```

### 错误响应

| 状态码 | 触发条件 | `detail` |
| --- | --- | --- |
| `400` | 上传文件不是图片 | `Must be an image file` |
| `400` | `target_lang` 不受支持 | 返回支持的语言代码列表 |
| `503` | 翻译模型尚未就绪 | `Translator model is loading or not ready.` |
| `500` | 识别或翻译过程异常 | 具体异常信息 |

### 请求示例

```bash
curl -X POST http://127.0.0.1:8989/api/v1/ocr_translate \
  -F "file=@screenshot.png" \
  -F "target_lang=vi"
```

---

## POST /api/v1/ocr_translate_blocks

上传一张图片，按 OCR 行返回逐行翻译与行框坐标，用于截图翻译悬浮层。

### 请求

- **Content-Type**：`multipart/form-data`

| 参数 | 位置 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- | --- |
| `file` | form-data | file | 是 | 图片文件，`Content-Type` 必须以 `image/` 开头 |
| `target_lang` | form-data | string | 否 | 目标语言代码/名称；省略或 `auto` 时中英自动互译 |
| `source_lang` | form-data | string | 否 | 原文语言代码；省略或 `auto` 时自动判断 |

### 返回参数

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `source_text` | string | OCR 识别出的完整原文 |
| `translation` | string | 逐行译文拼接（`\n` 分隔）；逐行翻译失败时回退为整段译文 |
| `blocks` | object[] | 每个 OCR 行的文本、译文与坐标 |
| `blocks[].text` | string | 该行原文 |
| `blocks[].translation` | string | 该行译文；无法翻译时为空字符串 |
| `blocks[].x/y/w/h` | number | 该行在原图像素坐标系中的左上角与宽高 |
| `blocks[].score` | number | OCR 置信度 |
| `blocks[].angle/cx/cy` | number | 倾斜文本行的角度与旋转框中心 |
| `blocks[].line_width/line_height` | number | 旋转行框沿文字方向/法线方向的长度 |
| `blocks[].glossary_hits` | object[] | 该行命中的词汇本条目：`term` / `translation` / `source` |
| `image_width/image_height` | number | 上传图片的像素尺寸 |
| `scale` | number | 坐标已还原到原图像素，固定为 `1.0` |

> 词汇本从 `settings.json` 的 `glossary` 字段读取，词条会加入翻译提示词并要求模型使用指定译法。

### 返回示例

```json
{
  "source_text": "Hello World\nSecond line",
  "translation": "你好世界\n第二行",
  "blocks": [
    {"text": "Hello World", "translation": "你好世界", "x": 12, "y": 20, "w": 140, "h": 18, "score": 0.97},
    {"text": "Second line", "translation": "第二行", "x": 12, "y": 44, "w": 120, "h": 17, "score": 0.95}
  ],
  "image_width": 800,
  "image_height": 600,
  "scale": 1.5
}
```

### 请求示例

```bash
curl -X POST http://127.0.0.1:8989/api/v1/ocr_translate_blocks \
  -F "file=@screenshot.png"
```

---

## POST /api/v1/translate_blocks

翻译已经识别好的 OCR 行框，不再做 OCR。实时翻译先调用 `/api/v1/ocr` 拿到行框，再调用本接口；逐行结果命中翻译缓存时不会触发模型推理。

### 请求

- **Content-Type**：`application/json`

| 字段 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| `lines` | object[] | 否 | `/api/v1/ocr` 返回的 `line_boxes` 原样传入 |
| `source_text` | string | 否 | 完整原文；`lines` 为空时按整段翻译 |
| `image_width/image_height` | number | 否 | 图片尺寸，原样回传 |
| `source_lang` | string | 否 | 原文语言；省略或 `auto` 时自动判断 |
| `target_lang` | string | 否 | 目标语言；省略时中英自动互译 |
| `force` | bool | 否 | `true` 时跳过逐行翻译缓存，强制重新推理（供手动刷新按钮使用） |

`lines` 最多 300 行。

### 返回参数

与 `/api/v1/ocr_translate_blocks` 相同：`source_text`、`translation`、`blocks`（含逐行译文与行框）、`image_width/image_height`、`scale`。

### 请求示例

```bash
curl -X POST http://127.0.0.1:8989/api/v1/translate_blocks \
  -H "Content-Type: application/json" \
  -d '{"lines":[{"text":"Hello World","x":12,"y":20,"w":140,"h":18}],"source_text":"Hello World"}'
```

---

## POST /v1/chat/completions

OpenAI Chat Completions 兼容接口。服务会取消息列表中**最后一条 `role=user` 的消息内容**作为待翻译文本，返回翻译结果。支持一次性返回与 SSE 流式返回两种模式。

### 请求

- **Content-Type**：`application/json`

| 字段 | 类型 | 必填 | 默认 | 说明 |
| --- | --- | --- | --- | --- |
| `messages` | object[] | 是 | — | 消息列表，不能为空 |
| `messages[].role` | string | 是 | — | 消息角色，如 `user` / `assistant` / `system` |
| `messages[].content` | string | 是 | — | 消息内容 |
| `stream` | bool | 否 | `false` | 是否使用 SSE 流式返回 |
| `target_lang` | string | 否 | `null` | 目标语言代码或名称；不传时保持中英文自动互译 |

> 仅最后一条 `user` 消息会被用于翻译；其它消息（含 `system`）当前不参与处理。
>
> `target_lang` 支持 ISO 语言代码、区域代码或常用中英文名称，例如 `vi`、`vi-VN`、`越南语`、`Vietnamese`。

### 请求示例

```json
{
  "messages": [
    { "role": "user", "content": "你好，欢迎使用翻译服务。" }
  ],
  "stream": false,
  "target_lang": "vi"
}
```

### 返回参数（非流式，`stream=false`）

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `choices` | object[] | 结果列表，固定包含一个元素 |
| `choices[].message.role` | string | 固定为 `assistant` |
| `choices[].message.content` | string | 翻译结果文本 |

#### 返回示例

```json
{
  "choices": [
    {
      "message": {
        "role": "assistant",
        "content": "你好世界"
      }
    }
  ]
}
```

### 返回内容（流式，`stream=true`）

- **Content-Type**：`text/event-stream`

以 SSE 逐块推送，每个数据块为一段增量文本，结构与 OpenAI 流式格式一致；流结束时发送 `[DONE]`：

```text
data: {"choices":[{"delta":{"content":"你好"}}]}

data: {"choices":[{"delta":{"content":"世界"}}]}

data: [DONE]
```

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `choices[].delta.content` | string | 本次推送的增量文本片段 |

### 错误响应

| 状态码 | 触发条件 | `detail` |
| --- | --- | --- |
| `400` | `messages` 为空 | `Messages cannot be empty` |
| `400` | 未找到 `user` 消息 | `No user message found` |
| `400` | `target_lang` 不受支持 | 返回支持的语言代码列表 |
| `503` | 翻译模型尚未就绪 | `Translator model is loading or not ready.` |
| `500` | 翻译过程出错 | 具体错误信息 |
| `501` | 请求流式但引擎不支持 | `Streaming not implemented in translator` |

### 请求示例

```bash
curl -X POST http://127.0.0.1:8989/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{"messages":[{"role":"user","content":"你好，欢迎使用翻译服务。"}],"target_lang":"vi"}'
```

流式：

```bash
curl -N -X POST http://127.0.0.1:8989/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{"messages":[{"role":"user","content":"Hello World"}],"stream":true}'
```

---

## 通用错误格式

除流式响应外，错误统一采用 FastAPI 的标准结构：

```json
{
  "detail": "错误描述"
}
```

## 备注

- 不传 `target_lang` 时，翻译方向仍由输入文本自动判断，保持原有中英文互译行为。
- 显式传入 `target_lang` 时，文本会固定翻译为指定语言。
- 翻译结果带有 LRU 缓存，缓存按“原文 + 目标语言”隔离。
- 首次调用翻译前，模型可能仍在加载或下载（约 1.13GB），此时相关接口返回 `503`，可先轮询 `/health` 等待 `translator_ready` 为 `true`。
