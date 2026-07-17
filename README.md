# 选中翻译

Windows 快捷翻译工具。选中文本或框选屏幕区域后按下快捷键，应用会调用本地翻译/OCR 服务，并通过 Wails 毛玻璃窗口显示结果。

## 功能

- 选中文本后快捷翻译
- Go 原生 OCR 屏幕框选、截图与识别
- 中英文自动互译
- Wails 翻译结果与设置界面
- 系统托盘常驻、自定义快捷键和 OCR 开关
- 可选运行日志，默认关闭并可在设置中启用
- 本地模型推理与结果缓存

## 架构

```text
translate-go.exe
├── Go: 托盘、全局快捷键、剪贴板、OCR 框选与截图
├── translate-wails.exe: 翻译结果和设置 UI
├── ai_engine.exe: 内置 Python 运行时、翻译与 OCR HTTP 服务
└── models/
    ├── Hy-MT2-1.8B-GGUF/
    └── rapidocr/（构建时复制到发行目录）
```

Wails 是唯一的桌面内容 UI。发布版通过 PyInstaller 将 Python、FastAPI、llama-cpp-python、RapidOCR 和 ONNX Runtime 打包进 `ai_engine.exe`，最终用户不需要安装 Python。GGUF 模型保存在根目录的 `models/` 中，RapidOCR 模型在构建时复制到 `dist-go/models/rapidocr/`。

## 目录结构与文件说明

以下是当前源码树的主要结构。带有“生成”标记的目录或文件由构建流程创建，不需要手动维护。

```text
translate-plugin/
├── backend/                         Python AI 服务源码
│   ├── api_server.py                FastAPI 翻译/OCR HTTP 服务
│   ├── ocr_service.py               RapidOCR 服务封装
│   ├── runtime_paths.py             开发模式与发布模式的路径解析
│   └── translator.py                llama.cpp / Hugging Face 翻译模型封装
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
│   ├── package-lock.json             npm 依赖锁定文件
│   └── wailsjs/                     Wails 生成的 JavaScript/TypeScript 桥接代码
├── models/                           外部模型文件，不编译进源码
│   └── Hy-MT2-1.8B-GGUF/            GGUF 翻译模型
├── dist-go/                          完整发行目录（生成/本地保留）
│   ├── ai_engine.exe                 打包后的 Python 翻译/OCR 服务
│   ├── translate-go.exe              Go 托盘启动器
│   ├── translate-wails.exe           Wails 翻译窗口
│   └── models/                       发行版使用的 GGUF 和 RapidOCR 模型
├── ai_engine.spec                    PyInstaller 打包配置
├── build_ai_engine.bat               构建独立 AI 引擎
├── build_all.bat                     构建并组装完整发行版
├── build_go.bat                      构建 Go 启动器
├── build_wails.bat                   构建 Wails 界面
├── go.mod / go.sum                   Go 模块依赖清单
├── requirements.txt                  Python 运行时依赖
├── requirements-build.txt            Python 构建依赖
├── .gitignore                         Git 忽略规则
├── wails.json                        Wails 项目配置
├── wails_main.go                     Wails 嵌入和桌面应用入口
├── icon.ico                          Windows 应用图标
├── README.md                         项目使用、开发和构建说明
└── LICENSE                           项目许可证
```

### 根目录文件职责

| 文件 | 用途 |
| --- | --- |
| `build_all.bat` | 按顺序构建前端、AI 引擎和 Go 程序，并整理到 `dist-go/`。 |
| `build_ai_engine.bat` | 使用 PyInstaller 将 `backend/` 打包为 `ai_engine.exe`。 |
| `build_wails.bat` | 安装/调用 Wails CLI，生成桌面 UI。 |
| `build_go.bat` | 编译 `translate-go.exe`。 |
| `wails_main.go` | 配置 Wails 绑定、窗口和前端资源嵌入。 |
| `settings.json` | 本机运行时设置，已被 Git 忽略。 |
| `dist-go/` | 发布产物目录，包含三个可执行文件、图标和模型。 |

`.claude/`、`.workbuddy/` 和 `.agents/` 是本地工具或工作区元数据，不属于应用运行时源码；构建缓存、日志和 Python 字节码均可安全删除并会在需要时重新生成。RapidOCR 模型由 `build_ai_engine.bat` 从 Python 依赖中复制到 `dist-go/models/rapidocr/`。

## 快捷键

| 操作 | 默认快捷键 |
| --- | --- |
| 翻译选中文本 | `Ctrl+Alt+Q` |
| OCR 截图翻译 | `Ctrl+Alt+E` |
| 关闭翻译窗口/取消框选 | `Esc` |

快捷键、OCR 开关、日志开关和本地 AI 服务端口可从托盘菜单的“设置”中修改。服务端口默认为 `8989`，可设置为 `1024-65535`；修改端口后应用会自动重启本地服务。

## 开发运行

开发源码模式需要安装 Python 依赖：

```powershell
python -m pip install -r requirements.txt
```

运行 Go 托盘进程：

```powershell
go run ./cmd/translate-go
```

开发模式需要先构建一次 Wails UI，或确保项目的 `build/bin/translate-wails.exe` 存在：

```powershell
./build_wails.bat
```

## 构建

构建机需要安装 Python、Go、Node.js，以及 AI 引擎打包依赖：

```powershell
python -m pip install -r requirements-build.txt
```

完整构建：

```powershell
./build_all.bat
```

完整应用会组装到 `dist-go/`，启动入口为：

```powershell
./dist-go/translate-go.exe
```

也可以只重新构建 AI 引擎：

```powershell
./build_ai_engine.bat
```

## 运行要求

- Windows 10/11 x64
- WebView2 Runtime
- 最终用户不需要安装 Python、Go 或 Node.js
- Go、Python 和 Node.js 仅用于开发与构建
- 翻译模型 `models/Hy-MT2-1.8B-GGUF/Hy-MT2-1.8B-Q4_K_M.gguf`
- RapidOCR 模型位于发行目录 `dist-go/models/rapidocr/`

## 验证

```powershell
go test ./...
cd frontend
npm run build
```

## 日志

运行日志默认关闭。需要排查问题时，可从托盘菜单进入“设置”，开启“运行日志”并保存。开启后，Go 主程序、Wails 窗口和 AI 后端会统一写入应用目录中的 `translate.log`。

日志内容带有 `[translate-go]`、`[translate-wails]` 或 `[ai-engine]` 标识，便于区分来源。新版本启动时会清理旧的分进程日志文件。关闭日志后，应用会停止写入；已有的 `translate.log` 不会自动删除。日志属于运行时生成文件，不应提交到源码仓库。
