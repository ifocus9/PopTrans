# PopTrans (选中翻译)

<div align="center">

[![Go](https://img.shields.io/badge/Go-1.26-00ADD8?logo=go&logoColor=white)](https://go.dev/)
[![Wails](https://img.shields.io/badge/Wails-v2.13-DF0000?logo=wails&logoColor=white)](https://wails.io/)
[![Python](https://img.shields.io/badge/Python-3.12-3776AB?logo=python&logoColor=white)](https://python.org/)
[![License](https://img.shields.io/badge/License-MIT-green)](LICENSE)
[![Platform](https://img.shields.io/badge/Platform-Windows%2010%2F11%20x64-blue)]()

</div>

**PopTrans** 是一款专为 Windows 打造的本地离线翻译与 OCR 工具。支持选中文本或屏幕截图后一键翻译。
基于 Go + Wails 构建流畅的原生交互与现代界面，后端采用 Python 与 llama.cpp 实现纯 CPU 离线大模型推理，**无需 GPU 加速，彻底告别隐私泄露与网络依赖**。

---

## 🌟 核心特性

- **🚀 快捷翻译**：一键捕获选中文本（模拟 Ctrl+C，内置多次智能重试），并即刻显示翻译结果。
- **📷 离线 OCR**：原生 Go 实现的屏幕框选与截图，内置多倍率自动缩放与低置信度重试，识别更准。
- **🌐 智能互译**：基于文本中的中英文字符比例，自动判断并进行中英文双向互译。
- **⚡ 极速响应**：内置 LRU 结果缓存（256 条），重复文本翻译实现秒级返回。
- **🎨 现代 UI**：基于 Wails 构建的亚克力毛玻璃界面，完美支持系统深色/浅色主题自适应。
- **🧩 UI 单实例**：设置页与结果弹窗复用同一个 `translate-ui` 进程，减少重复启动与内存抖动。
- **🔧 灵活配置**：系统托盘常驻，支持自定义全局快捷键、OCR 开关、本地端口、UI 空闲退出等，配置热加载即刻生效。

---

## 📸 效果预览

![翻译效果预览](images/translate.png)

![设置界面预览](images/setting.png)

---

## 🚀 快速上手 (面向用户)

### 运行环境

- **操作系统**：Windows 10 / 11 (x64)
- **依赖运行库**：需系统已安装 [WebView2 Runtime](https://developer.microsoft.com/zh-cn/microsoft-edge/webview2/) (Windows 11 通常自带)
- **硬件要求**：纯 CPU 推理，无需独立显卡，建议 8GB 及以上内存。

### 首次运行与自动下载

1. 双击运行发行版中的 `PopTrans.exe`。
2. **下载模型**：若本地无翻译模型，首次启动时会**自动联网下载** 腾讯 Hy-MT2-1.8B GGUF 模型 (约 1.13GB)。默认通过 HuggingFace 国内加速镜像 (`https://hf-mirror.com`) 下载并绕过系统代理。
3. 下载完成后，模型将保存在 `models/Hy-MT2-1.8B-GGUF/` 目录下，后续运行即为完全离线状态。

### 默认快捷键

| 操作                    | 默认快捷键   |
| ----------------------- | ------------ |
| **翻译选中文本**        | `Ctrl+Alt+Q` |
| **OCR 截图翻译**        | `Ctrl+Alt+E` |
| **关闭窗口 / 取消框选** | `Esc`        |

> _提示：快捷键可在托盘菜单的“设置”中随时自定义修改（支持 `Ctrl`/`Alt`/`Shift`/`Win` 等组合修饰键）。_

---

## 🏗️ 架构设计 (v2.x)

为解决旧版本单进程 Python 带来的系统交互卡顿与不稳定问题，v2.x 采用 **Go + Wails + Python 三进程架构**，并在近期进一步收敛了 UI 生命周期：

- **Go 宿主程序 (`PopTrans.exe`)**：专注系统集成，负责系统托盘、全局 Win32 快捷键注册、剪贴板捕获、原生 DWM 截图框选，以及 UI / AI 子进程的生命周期管理。
- **Wails UI (`translate-ui.exe`)**：单实例桌面 UI。设置页与翻译结果窗复用同一进程，通过本机 IPC 接收宿主指令；关闭窗口默认隐藏，空闲后可自动退出以释放内存。
- **Python AI 服务 (`ai_engine.exe`)**：专注后台 AI 计算，内置 FastAPI、llama.cpp、RapidOCR 提供本地 HTTP 服务，被 PyInstaller 独立打包，无需用户电脑安装 Python。

| 对比维度     | v1.x (Python)                            | v2.x (Go + Wails + Python)                                      |
| ------------ | ---------------------------------------- | --------------------------------------------------------------- |
| **稳定性**   | 任意模块崩溃导致应用直接退出             | 进程隔离，AI 引擎崩溃自动重启，托盘/UI 不受影响                 |
| **界面 UI**  | Tkinter 原生窗口，界面简陋               | Wails (WebView2) 亚克力毛玻璃界面；UI 单实例，结果更新无整窗重绘 |
| **系统交互** | pynput 监听，易冲突，需要管理员权限      | 原生 Win32 API 注册，精确可靠，无需提权                         |
| **OCR 框选** | Python 截屏，高 DPI 下存在坐标偏移与延迟 | Go 原生多显示器虚拟坐标覆盖，无延迟，完美适配高 DPI             |
| **进程模型** | 单进程耦合                               | 宿主 + 单实例 UI + AI；UI 支持空闲自动退出                      |

---

## 💻 开发与构建 (面向开发者)

项目划分为三大模块，需要分别具备 Go, Node.js, Python 开发环境。

### 目录结构

```text
translate-plugin/
├── backend/          # Python AI 服务代码与打包配置 (FastAPI, llama.cpp, RapidOCR)
├── cmd/translate-go/ # Go 托盘宿主程序入口
├── frontend/         # Wails/Vite 前端界面代码 (HTML/CSS/JS)
├── internal/         # Go 核心逻辑
│   ├── app/          # 宿主主流程（托盘、热键、翻译/OCR 调度）
│   ├── backend/      # AI 引擎客户端与进程监管
│   ├── config/       # 配置读写
│   ├── platform/     # Windows 原生能力（剪贴板、截图、热键等）
│   ├── uidaemon/     # UI 单实例进程管理与本机 IPC
│   └── wailsui/      # Wails UI 后端绑定
├── scripts/          # 构建打包脚本
├── models/           # 外部 AI 模型存放路径
└── dist-go/          # 构建产物输出目录
```

### 1. 开发环境启动

```powershell
# 1. 准备 Python 环境依赖 (用于本地运行 AI 服务)
python -m pip install -r backend/requirements.txt

# 2. 准备 UI 资源 (需构建一次前端，或者通过 wails dev 启动)
./scripts/build_wails.bat

# 3. 启动 Go 托盘主进程 (会自动拉起后台 Python 服务)
go run ./cmd/translate-go
```

### 2. 构建与打包发行版

打包完整发行版需要同时安装打包相关的依赖环境：

```powershell
python -m pip install -r backend/requirements-build.txt
```

**一键完整构建：**

```powershell
./scripts/build_all.bat
```

构建脚本会自动按顺序编译前端、Python 后端 (`ai_engine.exe`) 以及 Go 主程序 (`PopTrans.exe`)，并自动组装依赖到 `dist-go/` 目录。

> _注：您也可以使用 `scripts/` 下的独立 bat 脚本单独构建某个模块。_

---

## ⚙️ 进阶配置与 API 服务

### 应用配置 (`settings.json`)

应用配置会在同级目录生成 `settings.json`，支持通过设置界面或直接修改文件（自动热加载）：

- `server_port`: AI 引擎内部服务端口，默认为 `8989`。
- `theme`: 界面主题模式，可选 `system`, `light`, `dark`。
- `logging_enabled`: 是否开启文件日志。开启后，各进程日志将统一输出至 `translate.log`。
- `ui_idle_minutes`: UI 进程空闲自动退出时间（分钟）。窗口隐藏后开始计时；默认 `5`，设为 `0` 表示永不自动退出。

### 环境变量

- `TRANSLATE_SERVER_PORT`：强制覆盖配置文件中的 AI 服务端口。
- `HF_ENDPOINT`：自定义 HuggingFace 镜像下载地址（默认为 `https://hf-mirror.com`）。
- `NO_PROXY`：下载模型时绕过代理的域名列表。

### 本地 HTTP API (二次开发)

AI 后台引擎默认在 `http://127.0.0.1:<server_port>` 提供 API 服务，支持外部工具或脚本直接调用，详情请参阅 [本地 API 文档](docs/API.md)：

- **`GET /health`**：服务与模型健康状态检查。
- **`POST /api/v1/ocr`**：RapidOCR 纯离线图像文字识别。
- **`POST /api/v1/ocr_translate`**：OCR 识别 + 翻译一步集成。
- **`POST /v1/chat/completions`**：OpenAI 兼容的翻译/对话接口（支持流式响应 SSE）。

---

## 📝 更新记录

### 2026-07-27 — UI 单实例与生命周期优化

本次更新聚焦“进程过多、结果窗体验不稳”的问题，不改变整体 Go + Wails + Python 方向，优先收敛 UI 生命周期。

#### 主要变更

1. **UI 单实例**
   - 设置页与翻译结果窗复用同一个 `translate-ui.exe` 进程。
   - 宿主按需拉起 UI daemon（`--daemon`），后续通过本机 IPC 控制显示/隐藏，不再每次翻译都新建 UI 进程。
2. **本机 IPC 控制面**
   - 新增 `internal/uidaemon`：宿主与 UI 通过 loopback HTTP + token 通信。
   - 运行时会生成 `translate-ui-endpoint.json` 用于 UI 发现（本地运行时文件，不随发行包分发）。
3. **UI 空闲自动退出**
   - 窗口隐藏后默认 **5 分钟** 自动退出 UI 进程，降低低频使用场景下的内存占用。
   - 可通过 `settings.json` 的 `ui_idle_minutes` 调整；`0` 表示永不自动退出。
4. **结果窗体验修复**
   - 修复翻译完成时结果窗“跳一下”：loading → 结果更新不再重新定位到鼠标位置。
   - 修复结果窗“闪一下”：结果外壳只创建一次，内容区局部更新，避免重复触发入场动画。

#### 配置示例

```json
{
  "hotkey": "<ctrl>+<alt>+q",
  "hotkey_display": "Ctrl+Alt+Q",
  "ocr_enabled": true,
  "ocr_hotkey": "<ctrl>+<alt>+e",
  "ocr_hotkey_display": "Ctrl+Alt+E",
  "logging_enabled": false,
  "server_port": 8989,
  "theme": "system",
  "ui_idle_minutes": 5
}
```

#### 开发者说明

- 相关目录：`internal/uidaemon/`、`internal/wailsui/`、`internal/app/`、`frontend/src/`
- 修改 UI 后需重新构建：

```powershell
./scripts/build_wails.bat
# 如宿主侧也有改动
./scripts/build_go.bat
# 或一键完整构建
./scripts/build_all.bat
```

- 验证建议：
  - 连续翻译多次，任务管理器中 `translate-ui.exe` 应保持为 1 个进程。
  - 关闭结果窗后等待 `ui_idle_minutes`，UI 进程应自动退出；再次翻译会重新拉起。
  - loading 到译文完成时，窗口位置应保持稳定，且不应整窗闪烁。

---

## 📄 许可证

本项目基于 [MIT License](LICENSE) 协议开源。

### 友情链接

[![Linux.do](https://img.shields.io/badge/Linux.do-论坛-1c1c1e?style=flat-square)](https://linux.do/)
