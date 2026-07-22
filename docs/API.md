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
| `POST` | `/api/v1/ocr` | 图片 OCR 文字识别 |
| `POST` | `/api/v1/ocr_translate` | 图片 OCR 识别并翻译 |
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

### 返回示例

```json
{
  "status": "ok",
  "translator_ready": true,
  "translator_status": "模型已就绪",
  "ocr_loaded": false
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
| `target_lang` | form-data | string | 否 | 目标语言代码或名称；不传时保持中英文自动互译 |

### 返回参数

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `text` | string | 识别出的完整文本，多行以 `\n` 拼接 |
| `lines` | string[] | 按行分组后的文本，每个元素为一行 |
| `scale` | number | 识别时实际采用的图像缩放系数 |

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
