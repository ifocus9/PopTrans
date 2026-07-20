# 选中翻译 PopTrans

<div align="center">

[![Go](https://img.shields.io/badge/Go-1.26-00ADD8?logo=go&logoColor=white)](https://go.dev/)
[![Wails](https://img.shields.io/badge/Wails-v2.13-DF0000?logo=wails&logoColor=white)](https://wails.io/)
[![Python](https://img.shields.io/badge/Python-3.12-3776AB?logo=python&logoColor=white)](https://python.org/)
[![License](https://img.shields.io/badge/License-MIT-green)](LICENSE)
[![Platform](https://img.shields.io/badge/Platform-Windows%2010%2F11%20x64-blue)]()

</div>

PopTrans — Windows 本地离线翻译工具。选中文本 / OCR 截图后一键翻译，基于 Go + Wails + llama.cpp 纯 CPU 推理，无需 GPU。

---

## 预览

![翻译效果预览](images/translate.png)

![设置界面预览](images/setting.png)

---

## 功能

- 选中文本后快捷翻译（模拟 Ctrl+C 捕获剪贴板，最多重试 4 次）
- Go 原生 OCR 屏幕框选、截图与识别（多倍率自动缩放，低置信度自动重试）
- 中英文自动互译（基于 CJK 字符比例自动检测语种）
- 翻译结果 LRU 缓存（256 条），重复文本瞬时返回
- Wails 毛玻璃翻译结果与设置界面
- 系统托盘常驻、自定义快捷键和 OCR 开关
- 可选运行日志，默认关闭并可在设置中启用
- 本地模型推理与结果缓存

## 架构

```text
PopTrans.exe
├── Go: 托盘、全局快捷键、剪贴板、OCR 框选与截图
├── translate-ui.exe: 翻译结果和设置 UI
├── ai_engine.exe: 内置 Python 运行时、翻译与 OCR HTTP 服务
└── models/
    ├── Hy-MT2-1.8B-GGUF/
    └── rapidocr/（构建时复制到发行目录）
```

Wails 是唯一的桌面内容 UI。发布版通过 PyInstaller 将 Python、FastAPI、llama-cpp-python、RapidOCR 和 ONNX Runtime 打包进 `ai_engine.exe`，最终用户不需要安装 Python。GGUF 模型保存在根目录的 `models/` 中，RapidOCR 模型在构建时复制到 `dist-go/models/rapidocr/`。

## 技术栈

应用由三个进程组成，分别使用 Go、Web 前端和 Python 三套技术栈，通过本地 HTTP 与 Wails 桥接协作。

### Go 启动器与系统集成（`PopTrans.exe`）

| 技术                     | 版本    | 用途                                                       |
| ------------------------ | ------- | ---------------------------------------------------------- |
| Go                       | 1.26    | 托盘应用、进程编排与 Windows 系统集成的主语言              |
| Wails v2                 | v2.13.0 | 桌面 UI 框架，嵌入前端并暴露 Go ↔ JavaScript 绑定          |
| golang.org/x/sys         | v0.44.0 | 通过 Win32 API 实现剪贴板、全局热键、窗口与截图            |
| github.com/tc-hib/winres | v0.3.1  | 构建时生成 Windows 图标资源（`.syso`）                     |
| WebView2 (go-webview2)   | v1.0.22 | Wails 在 Windows 上的渲染内核（依赖系统 WebView2 Runtime） |

- Windows 平台能力集中在 [internal/platform/windows/](internal/platform/windows/)：剪贴板、热键、窗口毛玻璃（DWM）、屏幕截图。
- AI 引擎进程的启动、健康检查与 HTTP 调用由 [internal/backend/](internal/backend/) 的 supervisor 与 client 负责。
- 框选、截图与应用生命周期在 [internal/app/](internal/app/)，配置读写在 [internal/config/](internal/config/)。

### 前端界面（`translate-ui.exe`）

| 技术                         | 版本     | 用途                             |
| ---------------------------- | -------- | -------------------------------- |
| Vite                         | ^7.0.0   | 前端构建工具与开发服务器         |
| lucide                       | ^0.468.0 | 界面图标库                       |
| 原生 JavaScript / HTML / CSS | —        | 翻译结果与设置界面，无重量级框架 |

- 前端源码在 [frontend/](frontend/)，Wails 生成的 JS/TS 桥接代码位于 [frontend/wailsjs/](frontend/wailsjs/)。

### Python AI 服务（`ai_engine.exe`）

| 技术                 | 版本      | 用途                                                         |
| -------------------- | --------- | ------------------------------------------------------------ |
| FastAPI              | >=0.104.0 | 翻译 / OCR 的本地 HTTP 服务                                  |
| uvicorn              | >=0.23.2  | ASGI 服务器                                                  |
| python-multipart     | >=0.0.6   | 处理 OCR 图片上传                                            |
| llama-cpp-python     | >=0.3.0   | 加载 GGUF 翻译模型并进行 CPU 推理                            |
| huggingface_hub      | >=0.20.0  | 首次运行时下载模型（默认走 hf-mirror 镜像）                  |
| rapidocr-onnxruntime | >=1.3.0   | 基于 ONNX Runtime 的离线 OCR 识别                            |
| Pillow               | >=10.0.0  | OCR 前的图像预处理                                           |
| requests             | >=2.31.0  | 内部 HTTP 请求                                               |
| PyInstaller          | >=6.0,<7  | 将 Python 运行时与依赖打包为单一 `ai_engine.exe`（仅构建期） |

- 翻译引擎封装在 [backend/translator.py](backend/translator.py)，使用腾讯 Hy-MT2-1.8B GGUF 模型，内置结果 LRU 缓存。
- OCR 封装在 [backend/ocr_service.py](backend/ocr_service.py)，HTTP 路由在 [backend/api_server.py](backend/api_server.py)。

### 模型

| 模型                               | 用途             |
| ---------------------------------- | ---------------- |
| tencent/Hy-MT2-1.8B-GGUF（Q4_K_M） | 中英文离线互译   |
| RapidOCR（ONNX）                   | 屏幕截图文字识别 |

---

## 目录结构

以下是当前源码树的主要结构。带有"生成"标记的目录或文件由构建流程创建，不需要手动维护。根目录只保留 Go 与 Wails 强制要求的地基文件；构建脚本、Python 打包配置和资源已分别收拢到 `scripts/`、`backend/` 和 `assets/`。

```text
translate-plugin/
├── backend/                         Python AI 服务源码与打包配置
│   ├── api_server.py                FastAPI 翻译/OCR HTTP 服务
│   ├── ocr_service.py               RapidOCR 服务封装
│   ├── runtime_paths.py             开发模式与发布模式的路径解析
│   ├── translator.py                llama.cpp / Hugging Face 翻译模型封装
│   ├── ai_engine.spec               PyInstaller 打包配置
│   ├── requirements.txt             Python 运行时依赖
│   └── requirements-build.txt       Python 构建依赖
├── cmd/translate-go/
│   └── main.go                      Go 托盘应用入口
├── internal/
│   ├── app/                         框选、截图和应用生命周期
│   ├── backend/                     AI 引擎进程管理与 HTTP 客户端
│   ├── config/                      快捷键和本地设置读写
│   ├── logging/                     可开关的 Go 日志输出
│   ├── platform/windows/            Windows 剪贴板、热键、窗口和截图实现
│   └── wailsui/                     Wails 窗口及前端桥接逻辑
├── frontend/                        Wails 前端源码
│   ├── src/main.js                  翻译结果与设置界面
│   ├── src/styles.css               界面样式
│   ├── index.html                   前端 HTML 入口
│   ├── package.json                 npm 脚本和依赖声明
│   ├── package-lock.json            npm 依赖锁定文件
│   └── wailsjs/                     Wails 生成的 JavaScript/TypeScript 桥接代码
├── scripts/                         构建脚本（切回项目根后调用工具链）
│   ├── build_all.bat                构建并组装完整发行版
│   ├── build_ai_engine.bat          构建独立 AI 引擎
│   ├── build_go.bat                 构建 Go 启动器
│   └── build_wails.bat              构建 Wails 界面
├── tools/winres/                    Go 图标资源生成工具（生成 .syso）
├── assets/
│   └── icon.ico                     Windows 应用图标
├── models/                          外部模型文件，不编译进源码
│   └── Hy-MT2-1.8B-GGUF/            GGUF 翻译模型
├── dist-go/                         完整发行目录（生成/本地保留）
│   ├── ai_engine.exe                打包后的 Python 翻译/OCR 服务
│   ├── PopTrans.exe                 Go 托盘启动器
│   ├── translate-ui.exe             Wails 翻译窗口
│   └── models/                      发行版使用的 GGUF 和 RapidOCR 模型
├── go.mod / go.sum                  Go 模块依赖清单
├── .gitignore                       Git 忽略规则
├── wails.json                       Wails 项目配置
├── wails_main.go                    Wails 嵌入和桌面应用入口
├── docs/
│   └── API.md                       本地 HTTP API 接口说明
├── README.md                        项目使用、开发和构建说明
└── LICENSE                          项目许可证
```

### 关键文件与目录职责

| 路径                          | 用途                                                     |
| ----------------------------- | -------------------------------------------------------- |
| `scripts/build_all.bat`       | 按顺序构建前端、AI 引擎和 Go 程序，并整理到 `dist-go/`。 |
| `scripts/build_ai_engine.bat` | 使用 PyInstaller 将 `backend/` 打包为 `ai_engine.exe`。  |
| `scripts/build_wails.bat`     | 安装/调用 Wails CLI，生成桌面 UI。                       |
| `scripts/build_go.bat`        | 编译 `PopTrans.exe`。                                    |
| `backend/ai_engine.spec`      | PyInstaller 打包配置。                                   |
| `assets/icon.ico`             | Windows 应用图标，构建时复制到 Wails 资源与发行目录。    |
| `wails_main.go`               | 配置 Wails 绑定、窗口和前端资源嵌入。                    |
| `settings.json`               | 本机运行时设置，已被 Git 忽略。                          |
| `dist-go/`                    | 发布产物目录，包含三个可执行文件、图标和模型。           |

`.claude/`、`.workbuddy/` 和 `.agents/` 是本地工具或工作区元数据，不属于应用运行时源码；构建缓存、日志和 Python 字节码均可安全删除并会在需要时重新生成。RapidOCR 模型由 `scripts/build_ai_engine.bat` 从 Python 依赖中复制到 `dist-go/models/rapidocr/`。

## 快捷键

| 操作                  | 默认快捷键   |
| --------------------- | ------------ |
| 翻译选中文本          | `Ctrl+Alt+Q` |
| OCR 截图翻译          | `Ctrl+Alt+E` |
| 关闭翻译窗口/取消框选 | `Esc`        |

快捷键、OCR 开关、日志开关、界面主题和本地 AI 服务端口可从托盘菜单的"设置"中修改。服务端口默认为 `8989`，可设置为 `1024-65535`；修改端口后应用会自动重启本地服务。

界面主题支持三种取值：`system`（跟随系统，默认）、`light`（浅色）、`dark`（深色）。

快捷键必须包含至少一个修饰键，支持以下组合：

| 修饰键                        | 普通键                                                                   |
| ----------------------------- | ------------------------------------------------------------------------ |
| `Ctrl`、`Alt`、`Shift`、`Win` | `A-Z`、`0-9`、`F1-F24`、`Space`、`Enter`、`Tab`、`Esc`、方向键、`Delete` |

---

## 设置项

设置保存在应用目录下的 `settings.json` 中，由应用读写并已被 Git 忽略，通常无需手动编辑。字段说明如下：

| 字段                 | 类型   | 默认值           | 说明                                    |
| -------------------- | ------ | ---------------- | --------------------------------------- |
| `hotkey`             | string | `<ctrl>+<alt>+q` | 翻译选中文本的快捷键（内部格式）        |
| `hotkey_display`     | string | `Ctrl+Alt+Q`     | 翻译快捷键的显示文本                    |
| `ocr_enabled`        | bool   | `false`          | 是否启用 OCR 截图翻译                   |
| `ocr_hotkey`         | string | `<ctrl>+<alt>+e` | OCR 截图翻译的快捷键（内部格式）        |
| `ocr_hotkey_display` | string | `Ctrl+Alt+E`     | OCR 快捷键的显示文本                    |
| `logging_enabled`    | bool   | `false`          | 是否写入运行日志                        |
| `server_port`        | int    | `8989`           | 本地 AI 服务端口，有效范围 `1024-65535` |
| `theme`              | string | `system`         | 界面主题：`system` / `light` / `dark`   |

字段默认值定义见 [internal/config/config.go](internal/config/config.go)。

---

## 开发运行

开发源码模式需要安装 Python 依赖：

```powershell
python -m pip install -r backend/requirements.txt
```

运行 Go 托盘进程：

```powershell
go run ./cmd/translate-go
```

开发模式需要先构建一次 Wails UI，或确保项目的 `build/bin/translate-ui.exe` 存在：

```powershell
./scripts/build_wails.bat
```

---

## 构建

构建机需要安装 Python、Go、Node.js，以及 AI 引擎打包依赖：

```powershell
python -m pip install -r backend/requirements-build.txt
```

完整构建：

```powershell
./scripts/build_all.bat
```

完整应用会组装到 `dist-go/`，启动入口为：

```powershell
./dist-go/PopTrans.exe
```

也可以只重新构建 AI 引擎：

```powershell
./scripts/build_ai_engine.bat
```

---

## 运行要求

- Windows 10/11 x64
- WebView2 Runtime（[微软官方下载](https://developer.microsoft.com/zh-cn/microsoft-edge/webview2/)）
- 翻译为纯 CPU 推理（llama.cpp），无需独立显卡；建议内存 8GB 及以上
- 最终用户不需要安装 Python、Go 或 Node.js
- Go、Python 和 Node.js 仅用于开发与构建
- 翻译模型 `models/Hy-MT2-1.8B-GGUF/Hy-MT2-1.8B-Q4_K_M.gguf`
- RapidOCR 模型位于发行目录 `dist-go/models/rapidocr/`

### 首次运行与模型下载

若本地缺少翻译模型，应用**首次运行时会自动联网下载** Hy-MT2 GGUF 模型（约 1.13GB），因此首次启动需要可用的网络连接。下载默认走 HuggingFace 镜像 `https://hf-mirror.com`（国内加速），并绕过系统代理。模型下载完成后保存到 `models/Hy-MT2-1.8B-GGUF/`，后续运行离线即可使用。

可通过环境变量自定义模型下载行为：

| 环境变量      | 默认值                         | 说明                 |
| ------------- | ------------------------------ | -------------------- |
| `HF_ENDPOINT` | `https://hf-mirror.com`        | HuggingFace 镜像地址 |
| `NO_PROXY`    | `hf-mirror.com,huggingface.co` | 模型下载绕过系统代理 |

---

## 环境变量

| 变量                    | 默认值                         | 说明                                  |
| ----------------------- | ------------------------------ | ------------------------------------- |
| `TRANSLATE_SERVER_PORT` | `8989`                         | 覆盖 `settings.json` 中的 AI 服务端口 |
| `HF_ENDPOINT`           | `https://hf-mirror.com`        | HuggingFace 镜像地址（模型下载）      |
| `NO_PROXY`              | `hf-mirror.com,huggingface.co` | 模型下载绕过系统代理                  |

---

## 本地 HTTP API

AI 引擎在 `http://127.0.0.1:<server_port>`（默认 `8989`）上提供翻译与 OCR 的 HTTP 接口，并包含一个 OpenAI 兼容的 `/v1/chat/completions` 端点，可供二次开发或集成调用。完整的端点、入参与返回说明见 [docs/API.md](docs/API.md)。

### 端点一览

| 方法 | 路径                    | 说明                                   |
| ---- | ----------------------- | -------------------------------------- |
| GET  | `/health`               | 健康检查与模型状态                     |
| POST | `/api/v1/ocr`           | OCR 图片文字识别                       |
| POST | `/api/v1/ocr_translate` | OCR 识别 + 翻译（一步完成）            |
| POST | `/v1/chat/completions`  | OpenAI 兼容的翻译接口（支持 SSE 流式） |

---

## 验证

```powershell
go test ./...
cd frontend
npm run build
```

---

## 日志

运行日志默认关闭。需要排查问题时，可从托盘菜单进入"设置"，开启"运行日志"并保存。开启后，Go 主程序、Wails 窗口和 AI 后端会统一写入应用目录中的 `translate.log`。

日志内容带有 `[translate-go]`、`[translate-wails]` 或 `[ai-engine]` 标识，便于区分来源。新版本启动时会清理旧的分进程日志文件。关闭日志后，应用会停止写入；已有的 `translate.log` 不会自动删除。日志属于运行时生成文件，不应提交到源码仓库。

---

## 许可证

本项目基于 [MIT 许可证](LICENSE) 开源。

### 友情链接

[![Linux.do](https://img.shields.io/badge/Linux.do-论坛-1c1c1e?style=flat-square)](https://linux.do/)
