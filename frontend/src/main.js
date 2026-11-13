import "./styles.css"
import { GLOSSARY_PACKS } from "./glossary_packs"
import {
  createIcons,
  ArrowLeft,
  ArrowRightLeft,
  Check,
  CheckCheck,
  Clipboard,
  Cpu,
  Download,
  FileCode2,
  FileText,
  ImagePlus,
  Keyboard,
  Languages,
  Monitor,
  MonitorUp,
  Moon,
  MoreHorizontal,
  Network,
  Pause,
  Pin,
  PinOff,
  Play,
  RotateCcw,
  Search,
  Sliders,
  Sparkles,
  Sun,
  Trash2,
  X,
  Zap,
  Power,
} from "lucide"
import {
  CancelModelDownload,
  DeleteModelFile,
  HideWindow,
  IsOverlayProcess,
  ListModelFiles,
  LiveControl,
  ModelStatuses,
  OCRTranslateImage,
  RefreshOverlay,
  SaveConfig,
  SearchModels,
  SetHotkeyRecording,
  SetOverlayMenu,
  SetOverlayPinned,
  SetOverlayRegion,
  SetOverlayWindowRect,
  StartModelDownload,
  State,
  Translate,
} from "../wailsjs/go/wailsui/App"
import {
  EventsOn,
  WindowCenter,
  WindowSetAlwaysOnTop,
  WindowSetDarkTheme,
  WindowSetLightTheme,
  WindowSetSize,
  WindowSetSystemDefaultTheme,
  WindowShow,
} from "../wailsjs/runtime/runtime"

const app = document.querySelector("#app")

const icons = {
  ArrowLeft,
  ArrowRightLeft,
  Check,
  CheckCheck,
  Clipboard,
  Cpu,
  Download,
  FileCode2,
  FileText,
  ImagePlus,
  Keyboard,
  Languages,
  Monitor,
  MonitorUp,
  Moon,
  MoreHorizontal,
  Network,
  Pause,
  Pin,
  PinOff,
  Play,
  RotateCcw,
  Search,
  Sliders,
  Sparkles,
  Sun,
  Trash2,
  X,
  Zap,
  Power,
}
const LANGUAGE_OPTIONS = [
  ["zh", "中文"],
  ["en", "英语"],
  ["zh-hant", "繁体中文"],
  ["yue", "粤语"],
  ["vi", "越南语"],
  ["ja", "日语"],
  ["ko", "韩语"],
  ["th", "泰语"],
  ["it", "意大利语"],
  ["id", "印度尼西亚语"],
  ["ms", "马来语"],
  ["tl", "菲律宾语"],
  ["hi", "印地语"],
  ["fr", "法语"],
  ["es", "西班牙语"],
  ["de", "德语"],
  ["pt", "葡萄牙语"],
  ["ru", "俄语"],
  ["ar", "阿拉伯语"],
  ["pl", "波兰语"],
  ["cs", "捷克语"],
  ["nl", "荷兰语"],
  ["km", "高棉语"],
  ["my", "缅甸语"],
  ["fa", "波斯语"],
  ["gu", "古吉拉特语"],
  ["ur", "乌尔都语"],
  ["te", "泰卢固语"],
  ["mr", "马拉地语"],
  ["he", "希伯来语"],
  ["bn", "孟加拉语"],
  ["ta", "泰米尔语"],
  ["uk", "乌克兰语"],
  ["bo", "藏语"],
  ["kk", "哈萨克语"],
  ["mn", "蒙古语"],
  ["ug", "维吾尔语"],
]

function languageOptions(selected, autoLabel) {
  const current = selected || "auto"
  const items = [["auto", autoLabel], ...LANGUAGE_OPTIONS]
  return items
    .map(
      ([value, label]) =>
        `<option value="${value}" ${value === current ? "selected" : ""}>${label}</option>`,
    )
    .join("")
}

const systemTheme = window.matchMedia("(prefers-color-scheme: dark)")
let healthTimer
let resultSignature = ""
let lastResultHeight = 0
let windowShown = false
let unbindStateEvent = null
let activeTheme = "system"
try {
  const cachedTheme = localStorage.getItem("poptrans_theme")
  if (cachedTheme && ["light", "dark", "system"].includes(cachedTheme)) {
    activeTheme = cachedTheme
  }
} catch {
  // ignore
}
applyTheme(activeTheme)

let startupWindowSized = false
let startupDismissed = false
let resultShowTimestamp = 0

let state = {
  mode: "translate", // "translate" | "settings" | "result" | "overlay" | "live"
  active_tab: "translate", // "translate" | "settings"
  settingsTab: "general", // "general" | "glossary"
  glossaryPrepared: false,
  glossaryDraft: [],
  glossaryCategory: "all",
  glossarySearch: "",
  glossaryExtraCategories: [],
  settings_open: false,
  startup_loading: false,
  startup_status: "正在启动本地服务...",
  startup_error: "",
  result: {},
  config: {
    hotkey: "<ctrl>+<alt>+q",
    hotkey_display: "Ctrl+Alt+Q",
    ocr_enabled: true,
    ocr_hotkey: "<ctrl>+<alt>+e",
    ocr_hotkey_display: "Ctrl+Alt+E",
    live_enabled: true,
    live_hotkey: "<ctrl>+<alt>+r",
    live_hotkey_display: "Ctrl+Alt+R",
    logging_enabled: false,
    server_port: 8989,
    theme: "system",
    acceleration_device: "auto",
    ai_idle_minutes: 15,
    ai_keep_alive: true,
    ai_threads: 0,
    launch_at_login: false,
    model_download_source: "modelscope",
    model_download_base_url: "",
    models: [],
    active_model_id: "",
    source_language: "auto",
    target_language: "auto",
    overlay_font_family: "system",
    overlay_font_weight: 500,
    glossary: [],
  },
  health: {},
  // Model catalogue / search / download UI state.
  modelStatuses: {},
  modelSearch: {
    open: false,
    source: "modelscope",
    query: "",
    loading: false,
    results: [],
    repo: "",
    repoName: "",
    filesLoading: false,
    files: [],
    error: "",
  },
  modelDownload: {
    status: "",
    repo: "",
    file: "",
    name: "",
    percent: 0,
    speed: 0,
    error: "",
  },
  manualTranslate: {
    sourceText: "",
    translatedText: "",
    isLoading: false,
    error: "",
    loadingText: "",
    latencyMs: 0,
  },
  // Screenshot overlay payload, pushed through the dedicated "ui:overlay" event.
  overlay: null,
  overlayOriginal: false,
  overlayPinned: true,
  // Live-translation overlay payload, pushed through "ui:live".
  live: null,
  livePinned: true,
  liveOriginal: false,
  liveBlockOriginal: new Set(),
  overlayOnly: false,
}

function bindStateEvents() {
  if (unbindStateEvent || !window.runtime?.EventsOn) return
  unbindStateEvent = EventsOn("ui:state", (next) => {
    if (state.overlayOnly) return
    if (!next || typeof next !== "object") return
    const prevMode = state.mode
    const prevSignature = resultSignature
    const prevStartupLoading = Boolean(state.startup_loading)
    const prevStartupStatus = state.startup_status
    const nextMode = next.mode || state.mode

    // When startup finishes or user manually opens settings/translate tab, clear dismissed status
    if (!next.startup_loading || next.settings_open) {
      startupDismissed = false
    }

    state = {
      ...state,
      ...next,
      config: next.config || state.config,
      health: next.health || state.health,
      result: next.result || {},
      mode: nextMode,
      settings_open: Boolean(next.settings_open),
      startup_loading: Boolean(next.startup_loading),
      startup_status: next.startup_status || state.startup_status,
      startup_error: next.startup_error || "",
    }

    // Only follow host-driven mode switches. The settings tab itself is local,
    // so status updates (e.g. model download) must not reset it to translate.
    if (
      nextMode !== prevMode &&
      (nextMode === "settings" || nextMode === "translate")
    ) {
      state.active_tab = nextMode
    }

    const nextTheme = state.config?.theme
    if (nextTheme !== activeTheme) {
      applyTheme(nextTheme)
    }

    if (startupDismissed && state.startup_loading) {
      return
    }

    if (!windowShown || prevMode !== state.mode) {
      if (!windowShown) {
        state.manualTranslate = {
          sourceText: "",
          translatedText: "",
          isLoading: false,
          error: "",
          loadingText: "",
          latencyMs: 0,
        }
      }
      windowShown = false
      lastResultHeight = 0
      startupWindowSized = false
      if (state.mode === "result") {
        app.innerHTML = ""
      }
      render({ forceShow: true })
      return
    }

    if (
      prevStartupLoading !== state.startup_loading ||
      prevStartupStatus !== state.startup_status
    ) {
      const showingStartupPage =
        state.startup_loading &&
        state.mode === "settings" &&
        !state.settings_open
      // Never rebuild an already visible workbench/settings page on model
      // download progress: it would steal focus from settings inputs.
      if (!windowShown || showingStartupPage) {
        render({ forceShow: false })
      }
      return
    }

    if (state.mode === "result") {
      const nextSig = JSON.stringify(state.result || {})
      if (nextSig === prevSignature) return
      windowShown = false
      render({ forceShow: false })
      return
    }

    // Main window (translate/settings) can refresh engine badge without full rerender
    const node = document.querySelector("#engineState")
    const textNode = document.querySelector("#engineText")
    if (!node || !textNode) {
      render({ forceShow: false })
      return
    }
    const ready = Boolean(state.health?.translator_ready)
    node.classList.toggle("is-ready", ready)
    node.classList.toggle("is-loading", !ready)
    textNode.textContent = ready ? "引擎就绪" : "正在启动"
  })

  EventsOn("ui:overlay", (payload) => {
    if (!payload || typeof payload !== "object") return
    const previous = state.overlay
    const rectChanged =
      !previous ||
      previous.rect?.x !== payload.rect?.x ||
      previous.rect?.y !== payload.rect?.y ||
      previous.rect?.width !== payload.rect?.width ||
      previous.rect?.height !== payload.rect?.height
    state.overlay = payload
    if (payload.loading) {
      // Every screenshot starts in translation-only mode, even when the new
      // selection has exactly the same rectangle as the previous one.
      state.overlayOriginal = false
      state.overlayBlockOriginal = new Set()
    }
    if (rectChanged) {
      state.overlayOriginal = false
      state.overlayBlockOriginal = new Set()
      state.overlayPinned = true
    }
    if (state.mode !== "overlay") {
      state.mode = "overlay"
      windowShown = false
      render({ forceShow: true })
      return
    }
    renderOverlay()
  })

  EventsOn("ui:live", (payload) => {
    if (!payload || typeof payload !== "object") return
    const previous = state.live
    const rectChanged =
      !previous ||
      previous.window_rect?.x !== payload.window_rect?.x ||
      previous.window_rect?.y !== payload.window_rect?.y ||
      previous.window_rect?.width !== payload.window_rect?.width ||
      previous.window_rect?.height !== payload.window_rect?.height
    state.live = payload
    const startingSession =
      payload.status === "translating" &&
      (!previous ||
        (previous.status !== "translating" &&
          !(previous.blocks && previous.blocks.length)))
    if (startingSession) {
      state.liveOriginal = false
      state.liveBlockOriginal = new Set()
    }
    if (rectChanged) {
      state.livePinned = true
      state.liveOriginal = false
      state.liveBlockOriginal = new Set()
    }
    if (state.mode !== "live") {
      state.mode = "live"
      windowShown = false
      render({ forceShow: true })
      return
    }
    renderLive()
  })

  EventsOn("ui:model-download", (payload) => {
    if (!payload || typeof payload !== "object") return
    state.modelDownload = { ...state.modelDownload, ...payload }
    if (payload.status === "done") {
      ensureDownloadedModel(payload).then(() => {
        if (state.settingsTab === "models" && state.active_tab === "settings") {
          render()
        }
      })
    } else if (state.settingsTab === "models" && state.active_tab === "settings") {
      updateModelDownloadUI()
    }
  })
}

async function loadState() {
  if (!window.go?.wailsui?.App) {
    state = previewState()
    applyTheme(state.config?.theme)
    render()
    return
  }

  if (window.go?.wailsui?.App?.IsOverlayProcess) {
    try {
      if (await IsOverlayProcess()) {
        // Dedicated overlay/live process: never build the workbench UI.
        state.overlayOnly = true
        state.mode = "none"
        render()
        return
      }
    } catch {
      // Fall through to the normal workbench.
    }
  }

  try {
    const loaded = await State()
    state = {
      ...state,
      ...loaded,
      config: loaded.config || state.config,
      health: loaded.health || state.health,
      result: loaded.result || {},
      mode: loaded.mode || state.mode,
      active_tab:
        loaded.mode === "settings" || loaded.mode === "translate"
          ? loaded.mode
          : state.active_tab,
    }
  } catch (error) {
    state = {
      ...state,
      result: { error: readableError(error) },
      mode: "result",
    }
  }

  applyTheme(state.config?.theme)
  render()
}

function previewState() {
  const params = new URLSearchParams(window.location.search)
  const modeParam = params.get("mode")
  // ?loading=ocr|translate previews the progress indicator in the browser.
  const previewLoading = params.get("loading") || ""
  const previewLong = params.get("long") === "1"
  const previewMath = params.get("math") === "1"
  const previewProgress =
    previewLoading === "ocr" ? 15 : previewLoading === "translate" ? 55 : 0
  if (modeParam === "overlay") {
    const demoShot = `data:image/svg+xml;charset=utf-8,${encodeURIComponent(
      `<svg xmlns="http://www.w3.org/2000/svg" width="960" height="540">
        <rect width="960" height="540" fill="#111827"/>
        <text x="48" y="96" fill="#e5e7eb" font-size="34" font-family="Segoe UI">Settings</text>
        <text x="48" y="170" fill="#9ca3af" font-size="22" font-family="Segoe UI">Model download source</text>
        <text x="48" y="212" fill="#6b7280" font-size="18" font-family="Segoe UI">Choose ModelScope for faster downloads in China.</text>
        <rect x="48" y="300" width="240" height="64" rx="12" fill="#2563eb"/>
        <text x="98" y="341" fill="#ffffff" font-size="22" font-family="Segoe UI">Apply changes</text>
      </svg>`,
    )}`
    return {
      ...state,
      mode: "overlay",
      overlayPinned: true,
      overlayOriginal: false,
      overlayBlockOriginal: new Set(),
      overlay: {
        image_data_url: demoShot,
        image_width: 960,
        image_height: 540,
        loading: Boolean(previewLoading),
        stage: previewLoading,
        progress: previewProgress,
        lines: previewLoading === "translate" ? 4 : 0,
        error: "",
        rect: { x: 120, y: 120, width: 960, height: 540 },
        window_rect: { x: 118, y: 74, width: 964, height: 588 },
        bar_side: "top",
        latency_ms: 1830,
        source:
          "Settings\nModel download source\nChoose ModelScope for faster downloads in China.\nApply changes",
        translation:
          "设置\n模型下载源\n在中国选择魔搭可获得更快的下载速度。\n应用更改",
        blocks: [
          {
            text: "Settings",
            translation: previewLong
              ? "设置面板会保留上次使用的源语言并记住用户的所有偏好"
              : "设置",
            x: 48,
            y: 66,
            w: 130,
            h: 38,
            glossary_hits: [
              { term: "Settings", translation: "设置", source: "界面词汇表" },
            ],
          },
          {
            text: "Model download source",
            translation: previewMath
              ? "模型下载源，公式 E = mc² 与 f(x) = x² + 2x + 1。"
              : "模型下载源",
            x: 48,
            y: 148,
            w: 230,
            h: 28,
          },
          {
            text: "Choose ModelScope for faster downloads in China.",
            translation: "在中国选择魔搭可获得更快的下载速度。",
            x: 48,
            y: 192,
            w: 520,
            h: 26,
          },
          {
            text: "Apply changes",
            translation: "应用更改",
            x: 78,
            y: 318,
            w: 160,
            h: 30,
          },
        ],
      },
    }
  }
  if (modeParam === "live") {
    return {
      ...state,
      mode: "live",
      livePinned: true,
      live: {
        status: previewLoading ? "translating" : "listening",
        stage: previewLoading,
        progress: previewProgress,
        lines: previewLoading === "translate" ? 3 : 0,
        error: "",
        source: "Good design is invisible.",
        translation: "好的设计是无形的。",
        anchor: { x: 620, y: 180, width: 400, height: 240 },
        window_rect: { x: 618, y: 134, width: 404, height: 288 },
        bar_side: "top",
        latency_ms: 2140,
        blocks: [
          {
            text: "Good design is invisible.",
            translation: "好的设计是无形的。",
            x: 20,
            y: 20,
            w: 300,
            h: 28,
            glossary_hits: [
              {
                term: "Good design",
                translation: "好的设计",
                source: "设计术语表",
              },
            ],
          },
          {
            text: "It gets out of the way.",
            translation: "它不会妨碍你。",
            x: 20,
            y: 90,
            w: 260,
            h: 26,
          },
          {
            text: "Focus on what matters.",
            translation: "专注于真正重要的事情。",
            x: 20,
            y: 160,
            w: 320,
            h: 26,
          },
        ],
      },
    }
  }
  if (modeParam === "result") {
    return {
      ...state,
      mode: "result",
      result: {
        source:
          "A good interface gets out of the way and helps people focus on their work.",
        result: "好的界面不会喧宾夺主，而是帮助人们专注于手头的工作。",
      },
    }
  }
  if (modeParam === "settings") {
    return {
      ...state,
      mode: "settings",
      active_tab: "settings",
      health: { translator_ready: true },
    }
  }
  return {
    ...state,
    mode: "translate",
    active_tab: "translate",
    health: { translator_ready: true },
    manualTranslate: {
      sourceText: "PopTrans provides fast and offline translation.",
      translatedText: "PopTrans 提供快速且完全离线的翻译服务。",
      isLoading: false,
      error: "",
      loadingText: "",
      latencyMs: 1200,
    },
  }
}

function render(options = {}) {
  if (startupDismissed && state.startup_loading) {
    return
  }
  if (
    state.overlayOnly &&
    state.mode !== "overlay" &&
    state.mode !== "live"
  ) {
    app.innerHTML = ""
    return
  }
  const shouldShow =
    options.forceShow === true ||
    (options.forceShow !== false && !windowShown)

  clearInterval(healthTimer)
  document.documentElement.classList.toggle(
    "result-mode",
    state.mode === "result",
  )
  document.documentElement.classList.toggle(
    "overlay-mode",
    state.mode === "overlay",
  )
  document.documentElement.classList.toggle("live-mode", state.mode === "live")
  if (state.mode === "overlay") {
    renderOverlay()
  } else if (state.mode === "live") {
    renderLive()
  } else if (state.mode === "result") {
    renderResult()
  } else if (
    state.startup_loading &&
    state.mode === "settings" &&
    !state.settings_open
  ) {
    // Cold-start loading page only before the host opens the workbench.
    // Once the workbench is shown it stays usable while the model loads.
    renderStartup()
  } else {
    renderMainWindow()
    createIcons({ icons, attrs: { "stroke-width": 1.8 } })
  }
  if (shouldShow) {
    showWindowAfterRender()
  }
}

function showWindowAfterRender() {
  if (startupDismissed && state.startup_loading) return
  if (!window.runtime?.WindowShow) return

  requestAnimationFrame(() => {
    requestAnimationFrame(() => {
      if (startupDismissed && state.startup_loading) return
      WindowShow()
      windowShown = true
      if (state.mode === "result") {
        resultShowTimestamp = Date.now()
        try {
          window.focus()
        } catch {
          // ignore
        }
      }
    })
  })
}

const OVERLAY_FONT_STACKS = {
  system: "'Microsoft YaHei UI', 'Segoe UI', system-ui, sans-serif",
  yahei: "'Microsoft YaHei', 'Microsoft YaHei UI', sans-serif",
  segoe: "'Segoe UI', 'Microsoft YaHei UI', sans-serif",
  simsun: "SimSun, '宋体', serif",
  simhei: "SimHei, '黑体', sans-serif",
  dengxian: "'DengXian', '等线', sans-serif",
}

const OVERLAY_FONT_OPTIONS = [
  ["system", "跟随系统"],
  ["yahei", "微软雅黑"],
  ["segoe", "Segoe UI"],
  ["simsun", "宋体"],
  ["simhei", "黑体"],
  ["dengxian", "等线"],
]

function overlayFontStack(key) {
  return OVERLAY_FONT_STACKS[key] || OVERLAY_FONT_STACKS.system
}

function overlayWeightLabel(value) {
  const weight = clampNumber(value, 300, 800, 500)
  const labels = {
    300: "细",
    400: "常规",
    500: "中等",
    600: "半粗",
    700: "粗",
    800: "特粗",
  }
  return `${labels[weight] || "中等"} (${weight})`
}

// Text colour comes from the blurred background luminance (computed by the
// host). A matching scrim + shadow keeps it readable even when the background
// is busy or changes abruptly.
function overlayTextAppearance(block) {
  const textColor = String(block.text_color || "#FFFFFF")
  const darkText =
    textColor.toUpperCase() === "#101010" || textColor.toUpperCase() === "#000000"
  const scrim = darkText
    ? "rgba(255, 255, 255, 0.38)"
    : "rgba(0, 0, 0, 0.42)"
  const background = String(block.background_image || "")
  const backgroundStyle = background
    ? `background-image:linear-gradient(${scrim},${scrim}),url('${background}');`
    : `background-color:${darkText ? "rgba(255,255,255,0.72)" : "rgba(0,0,0,0.58)"};`
  const textShadow = darkText
    ? "0 1px 1px rgba(255,255,255,0.6)"
    : "0 1px 2px rgba(0,0,0,0.65)"
  return { textColor, backgroundStyle, textShadow }
}

// Translate the OCR line geometry into an unrotated CSS box plus a rotation.
// Rotated lines (angle != 0) use the quadrilateral centre/edge lengths so the
// translation keeps the original orientation instead of using its AABB.
let overlayMeasureCanvas = null
let overlayMeasureContext = null

function measureOverlayText(text, font) {
  if (!overlayMeasureContext) {
    overlayMeasureCanvas = document.createElement("canvas")
    overlayMeasureContext = overlayMeasureCanvas.getContext("2d")
  }
  overlayMeasureContext.font = font
  return overlayMeasureContext.measureText(text).width
}

// Wrap text the same way the chip lays it out: CJK characters may break
// anywhere, Latin text prefers the last space.
function wrapOverlayText(text, maxWidth, font) {
  const lines = []
  for (const paragraph of String(text).split("\n")) {
    let line = ""
    for (const char of paragraph) {
      const candidate = line + char
      if (line && measureOverlayText(candidate, font) > maxWidth) {
        const space = line.lastIndexOf(" ")
        if (space > 0 && char !== " ") {
          lines.push(line.slice(0, space))
          line = line.slice(space + 1) + char
        } else {
          lines.push(line)
          line = char
        }
      } else {
        line = candidate
      }
    }
    lines.push(line)
  }
  return lines.length ? lines : [""]
}

// Shrink the translated text until it fits the OCR line box, so a longer
// translation stays readable without covering the next line.
function fitOverlayFontSize(
  text,
  boxWidth,
  boxHeight,
  basePx,
  fontFamily,
  fontWeight,
) {
  const minPx = 6.5
  const lineHeight = 1.16
  const availableWidth = Math.max(8, boxWidth - 8)
  const availableHeight = Math.max(6, boxHeight - 2)
  const fits = (size) => {
    const font = `${fontWeight} ${size}px ${fontFamily}`
    const lines = wrapOverlayText(text, availableWidth, font)
    return lines.length * size * lineHeight <= availableHeight + 0.5
  }
  if (fits(basePx)) return basePx
  let low = minPx
  let high = basePx
  let best = minPx
  for (let i = 0; i < 8; i++) {
    const mid = (low + high) / 2
    if (fits(mid)) {
      best = mid
      low = mid
    } else {
      high = mid
    }
  }
  return Math.max(minPx, best)
}

function fitOverlayBlockText({
  text,
  index,
  geometries,
  imageAreaW,
  imageAreaH,
  fontFamily,
  fontWeight,
}) {
  const geometry = geometries[index]
  const boxWidth = (geometry.width / 100) * imageAreaW
  const boxHeight = (geometry.height / 100) * imageAreaH
  const fontPx = fitOverlayFontSize(
    text,
    boxWidth,
    boxHeight,
    geometry.fontPx,
    fontFamily,
    fontWeight,
  )
  // Keep the OCR line box exactly where the original text is. Long translations
  // only shrink the font; the chip never grows outside the selected image area.
  return { ...geometry, fontPx }
}

function overlayBlockGeometry(block, imageW, imageH, areaHeight) {
  const angle = Number(block.angle) || 0
  const lineW = Number(block.line_width) || 0
  const lineH = Number(block.line_height) || 0
  const sourceH = Number(block.source_h) || 0
  const cx = Number(block.cx) || 0
  const cy = Number(block.cy) || 0
  let left
  let top
  let width
  let height
  let sourceHeight
  if (lineW > 0 && lineH > 0) {
    left = ((cx - lineW / 2) / imageW) * 100
    top = ((cy - lineH / 2) / imageH) * 100
    width = Math.max(1, (lineW / imageW) * 100)
    height = Math.max(1, (lineH / imageH) * 100)
    sourceHeight = sourceH || lineH
  } else {
    left = (Number(block.x) / imageW) * 100
    top = (Number(block.y) / imageH) * 100
    width = Math.max(2, (Number(block.w) / imageW) * 100)
    height = Math.max(1.2, (Number(block.h) / imageH) * 100)
    sourceHeight = sourceH || Number(block.h)
  }
  const fontPx = Math.max(8, (sourceHeight / imageH) * areaHeight * 0.82)
  return { left, top, width, height, fontPx, angle }
}

const MARKDOWN_SUPERSCRIPTS = {
  "⁰": "0", "¹": "1", "²": "2", "³": "3", "⁴": "4",
  "⁵": "5", "⁶": "6", "⁷": "7", "⁸": "8", "⁹": "9",
  "⁺": "+", "⁻": "-", "⁼": "=", "⁽": "(", "⁾": ")", "ⁿ": "n",
}
const MARKDOWN_SUBSCRIPTS = {
  "₀": "0", "₁": "1", "₂": "2", "₃": "3", "₄": "4",
  "₅": "5", "₆": "6", "₇": "7", "₈": "8", "₉": "9",
  "₊": "+", "₋": "-", "₌": "=", "₍": "(", "₎": ")",
}
const MARKDOWN_MATH_SYMBOLS = {
  "·": "\\cdot ", "×": "\\times ", "÷": "\\div ", "±": "\\pm ",
  "≤": "\\le ", "≥": "\\ge ", "≠": "\\ne ", "≈": "\\approx ",
  "∞": "\\infty ", "∂": "\\partial ", "∇": "\\nabla ",
  "∫": "\\int ", "∑": "\\sum ", "√": "\\sqrt ", "→": "\\to ",
  "π": "\\pi ", "α": "\\alpha ", "β": "\\beta ", "γ": "\\gamma ",
  "δ": "\\delta ", "ε": "\\varepsilon ", "ζ": "\\zeta ", "η": "\\eta ",
  "θ": "\\theta ", "λ": "\\lambda ", "μ": "\\mu ", "ν": "\\nu ",
  "ξ": "\\xi ", "ρ": "\\rho ", "σ": "\\sigma ", "τ": "\\tau ",
  "φ": "\\phi ", "χ": "\\chi ", "ψ": "\\psi ", "ω": "\\omega ",
  "Δ": "\\Delta ", "Σ": "\\Sigma ", "Φ": "\\Phi ", "Ψ": "\\Psi ", "Ω": "\\Omega ",
}
const MARKDOWN_CJK_RE =
  /[\u3000-\u303f\u3400-\u4dbf\u4e00-\u9fff\uf900-\ufaff\uac00-\ud7af]/
const MARKDOWN_MATH_RE = /[=^_\\∫∑√±≈≠≤≥∞∂∇·×÷]/
const MARKDOWN_SEGMENT_RE =
  /[^\u3000-\u303f\u3400-\u4dbf\u4e00-\u9fff\uf900-\ufaff\uac00-\ud7af\n]+/g

function unicodeMathToLatex(text) {
  let result = text.replace(/[⁰¹²³⁴⁵⁶⁷⁸⁹⁺⁻⁼⁽⁾ⁿ]+/g, (run) => {
    const value = [...run].map((char) => MARKDOWN_SUPERSCRIPTS[char] || char).join("")
    return `^{${value}}`
  })
  result = result.replace(/[₀₁₂₃₄₅₆₇₈₉₊₋₌₍₎]+/g, (run) => {
    const value = [...run].map((char) => MARKDOWN_SUBSCRIPTS[char] || char).join("")
    return `_{${value}}`
  })
  for (const [symbol, latex] of Object.entries(MARKDOWN_MATH_SYMBOLS)) {
    result = result.split(symbol).join(latex)
  }
  return result
}

function isUnchangedMathLine(source, translated) {
  const src = String(source || "").trim()
  const dst = String(translated || "").trim()
  if (!src || dst !== src) return false
  return /[0-9=^_\\+\-*/<>∫∑√±≈≠≤≥∞∂∇·×÷]/.test(src)
}

// Convert one translated OCR line to Markdown, wrapping formula-like spans in
// inline math so a Markdown renderer displays the equations.
function markdownMathLine(line) {
  const text = String(line || "").trim()
  if (!text) return ""
  if (text.includes("$")) return text
  if (!MARKDOWN_CJK_RE.test(text) && MARKDOWN_MATH_RE.test(text)) {
    return `$$${unicodeMathToLatex(text)}$$`
  }
  return text.replace(MARKDOWN_SEGMENT_RE, (segment) => {
    if (!segment.trim() || segment.includes("$")) return segment
    if (
      !MARKDOWN_MATH_RE.test(segment) &&
      !/[A-Za-z][0-9]|[0-9][A-Za-z]/.test(segment)
    ) {
      return segment
    }
    const leading = segment.match(/^\s*/)[0]
    const trailing = segment.match(/\s*$/)[0]
    return `${leading}$${unicodeMathToLatex(segment.trim())}$${trailing}`
  })
}

function markdownFromBlocks(blocks, fallback) {
  const lines = (blocks || [])
    .filter((block) => {
      const source = String(block.text || "").trim()
      const translated = String(block.translation || "").trim()
      return translated && !isUnchangedMathLine(source, translated)
    })
    .map((block) => String(block.translation || "").trim())
    .filter(Boolean)
  if (!lines.length) return String(fallback || "").trim()
  return lines.map(markdownMathLine).join("\n")
}

function overlayProgressHtml(active, progress) {
  return `<div class="shot-progress ${active ? "is-active" : ""}" aria-hidden="true">
    <span class="shot-progress-fill" style="width:${progress}%"></span>
  </div>`
}

function renderOverlay() {
  const overlay = state.overlay || {}
  const blocks = Array.isArray(overlay.blocks) ? overlay.blocks : []
  const loading = Boolean(overlay.loading)
  const error = overlay.error || ""
  const showOriginal = Boolean(state.overlayOriginal)
  const blockOriginal = state.overlayBlockOriginal || new Set()
  const imgW = Number(overlay.image_width) || 1
  const imgH = Number(overlay.image_height) || 1
  const cfg = state.config || {}
  const style = overlay.style || {}
  const overlayFont = overlayFontStack(
    style.font_family || cfg.overlay_font_family,
  )
  const overlayWeight = clampNumber(
    style.font_weight ?? cfg.overlay_font_weight,
    300,
    800,
    500,
  )
  const barSide = overlay.bar_side === "bottom" ? "bottom" : "top"
  const imageAreaH = Math.max(80, (window.innerHeight || 600) - 48)
  const imageAreaW = Math.max(80, (window.innerWidth || 800) - 4)
  const geometries = blocks.map((block) =>
    overlayBlockGeometry(block, imgW, imgH, imageAreaH),
  )

  const renderedBlocks = blocks
    .map((block, index) => {
      const source = String(block.text || "")
      const translated = String(block.translation || "")
      const showSource = showOriginal || blockOriginal.has(index)
      // Empty translations stay blank instead of covering the source text.
      if (!showSource && !translated) return ""
      const text = showSource ? source : translated
      if (!text) return ""
      // Formula lines that the translator left untouched are better left as the
      // original screen content than covered by garbled OCR text.
      if (!showSource && isUnchangedMathLine(source, translated)) return ""
      const { left, top, width, height, fontPx, angle } =
        fitOverlayBlockText({
          text,
          index,
          geometries,
          imageAreaW,
          imageAreaH,
          fontFamily: overlayFont,
          fontWeight: overlayWeight,
        })
      const classes = [
        "shot-block",
        showSource ? "is-original" : "",
        translated ? "" : "is-untranslated",
      ]
        .filter(Boolean)
        .join(" ")
      // Keep the translation inside the original OCR line box so neighbouring
      // lines are never covered. Long text is clipped and available via the
      // title/hover tooltip and the copy buttons.
      const sizeStyle = `width:${width}%;height:${height}%;`
      const textHtml = renderGlossaryText(
        text,
        block.glossary_hits,
        showSource ? "source" : "translation",
      )
      const { textColor, backgroundStyle, textShadow } =
        overlayTextAppearance(block)
      const title = `${showSource ? source : translated} · 单击切换原文/译文，双击复制`
      return `<div class="${classes}" data-shot-index="${index}"
        data-rect="${left},${top},${width},${height}" data-angle="${angle}"
        style="left:${left}%;top:${top}%;${sizeStyle}transform:rotate(${angle}deg);font-size:${fontPx}px;color:${textColor};${backgroundStyle}text-shadow:${textShadow};"
        title="${escapeHtml(title)}">${textHtml}</div>`
    })
    .join("")

  const fullText =
    !loading && !blocks.length && overlay.translation
      ? `<div class="shot-fulltext" title="整段译文">${escapeHtml(overlay.translation)}</div>`
      : ""
  const latencyMs = Number(overlay.latency_ms) || 0
  const latencyText = latencyMs > 0 ? `${(latencyMs / 1000).toFixed(1)}s` : ""
  const progress = clampNumber(Number(overlay.progress) || 0, 0, 100, 0)
  const stage = String(overlay.stage || "")
  const lines = Number(overlay.lines) || 0
  const progressStage =
    stage === "ocr" ? "识别文字" : stage === "translate" ? "翻译中" : "处理中"
  const progressDetail = stage === "translate" && lines > 0 ? ` · ${lines} 行` : ""
  const progressLabel = `${progressStage}${progressDetail}${
    progress > 0 ? ` · ${progress}%` : ""
  }`
  const progressActive = loading && progress > 0 && progress < 100
  const translatedOk =
    !loading &&
    !error &&
    (blocks.some((block) => String(block.translation || "").trim()) ||
      Boolean(overlay.translation))
  const statusBubble = loading
    ? `<span class="trans-status is-neutral">${escapeHtml(progressLabel)}</span>`
    : `<span class="trans-status ${translatedOk ? "is-ok" : "is-warn"}">${
        translatedOk ? "成功" : "未翻译"
      }${latencyText ? ` · ${latencyText}` : ""}</span>`

  app.innerHTML = `
    <div class="shot-window bar-${barSide}" id="shotWindow"
      style="--shot-font:${overlayFont};--shot-weight:${overlayWeight}">
      <div class="shot-bar" style="--wails-draggable: drag">
        <span class="shot-badge"><i data-lucide="monitor-up"></i><span>截图翻译</span></span>
        ${statusBubble}
        <button class="shot-btn ${showOriginal ? "is-active" : ""}" id="shotToggleBtn" type="button" title="切换原文/译文 (H)">
          <i data-lucide="arrow-right-left"></i><span>${showOriginal ? "显示译文" : "显示原文"}</span>
        </button>
        <button class="shot-btn" id="shotCopySourceBtn" type="button" title="复制原文">
          <i data-lucide="file-text"></i><span>原文</span>
        </button>
        <button class="shot-btn" id="shotCopyTargetBtn" type="button" title="复制译文 (Ctrl+C)">
          <i data-lucide="languages"></i><span>译文</span>
        </button>
        <button class="shot-btn" id="shotCopyMarkdownBtn" type="button" title="复制 Markdown 译文（公式为 LaTeX）">
          <i data-lucide="file-code-2"></i><span>MD</span>
        </button>
        <button class="shot-btn" id="shotRefreshBtn" type="button" title="刷新译文">
          <i data-lucide="rotate-ccw"></i><span>刷新</span>
        </button>
        <button class="shot-btn" id="shotPinBtn" type="button" title="${state.overlayPinned ? "取消置顶" : "置顶"}">
          <i data-lucide="${state.overlayPinned ? "pin" : "pin-off"}"></i>
        </button>
        <button class="shot-btn shot-more" id="shotMoreBtn" type="button" title="更多功能">
          <i data-lucide="more-horizontal"></i>
        </button>
        <button class="shot-btn shot-close" id="shotCloseBtn" type="button" title="关闭 (Esc)">
          <i data-lucide="x"></i>
        </button>
        ${overlayProgressHtml(progressActive, progress)}
      </div>
      <div class="shot-menu-scrim" id="shotMenuScrim" hidden></div>
      <div class="shot-menu" id="shotMenu" role="menu">
        <button class="shot-menu-item" type="button" data-target="shotToggleBtn">
          <i data-lucide="arrow-right-left"></i><span>${showOriginal ? "显示译文" : "显示原文"}</span>
        </button>
        <button class="shot-menu-item" type="button" data-target="shotCopySourceBtn">
          <i data-lucide="file-text"></i><span>复制原文</span>
        </button>
        <button class="shot-menu-item" type="button" data-target="shotCopyTargetBtn">
          <i data-lucide="languages"></i><span>复制译文</span>
        </button>
        <button class="shot-menu-item" type="button" data-target="shotCopyMarkdownBtn">
          <i data-lucide="file-code-2"></i><span>复制 Markdown 译文</span>
        </button>
        <button class="shot-menu-item" type="button" data-target="shotRefreshBtn">
          <i data-lucide="rotate-ccw"></i><span>刷新译文</span>
        </button>
        <button class="shot-menu-item" type="button" data-target="shotPinBtn">
          <i data-lucide="${state.overlayPinned ? "pin-off" : "pin"}"></i><span>${state.overlayPinned ? "取消置顶" : "置顶"}</span>
        </button>
        <button class="shot-menu-item" type="button" data-target="shotCloseBtn">
          <i data-lucide="x"></i><span>关闭</span>
        </button>
      </div>
      <div class="shot-image-wrap">
        <div class="shot-blocks">${renderedBlocks}</div>
        ${error ? `<div class="shot-pill is-error" title="${escapeHtml(error)}">${escapeHtml(error)}</div>` : ""}
        ${fullText}
      </div>
      <div class="shot-frame-border" aria-hidden="true"></div>
      <div class="shot-toast" id="shotToast" aria-live="polite"></div>
    </div>
  `

  createIcons({ icons, attrs: { "stroke-width": 1.8 } })

  document
    .querySelector("#shotToggleBtn")
    ?.addEventListener("click", () => {
      state.overlayOriginal = !state.overlayOriginal
      renderOverlay()
    })
  document
    .querySelector("#shotCopySourceBtn")
    ?.addEventListener("click", () =>
      copyShotText(overlay.source || "", "已复制原文"),
    )
  document
    .querySelector("#shotCopyTargetBtn")
    ?.addEventListener("click", () =>
      copyShotText(
        overlay.translation || translatedJoin(blocks),
        "已复制译文",
      ),
    )
  document
    .querySelector("#shotRefreshBtn")
    ?.addEventListener("click", () => {
      if (window.go?.wailsui?.App?.RefreshOverlay) {
        RefreshOverlay().catch(() => {})
        showShotToast("正在刷新译文...")
      }
    })
  document
    .querySelector("#shotCopyMarkdownBtn")
    ?.addEventListener("click", () =>
      copyShotText(
        markdownFromBlocks(blocks, overlay.translation),
        "已复制 Markdown 译文",
      ),
    )
  document.querySelector("#shotPinBtn")?.addEventListener("click", () => {
    state.overlayPinned = !state.overlayPinned
    if (window.runtime?.WindowSetAlwaysOnTop) {
      WindowSetAlwaysOnTop(state.overlayPinned)
    }
    if (window.go?.wailsui?.App?.SetOverlayPinned) {
      SetOverlayPinned(state.overlayPinned).catch(() => {})
    }
    renderOverlay()
  })
  document
    .querySelector("#shotCloseBtn")
    ?.addEventListener("click", closeWindow)

  document.querySelectorAll(".shot-block").forEach((node) => {
    const index = Number(node.dataset.shotIndex)
    node.addEventListener("click", (event) => {
      if (event.detail > 1) return
      if (!state.overlayBlockOriginal) state.overlayBlockOriginal = new Set()
      if (state.overlayBlockOriginal.has(index)) {
        state.overlayBlockOriginal.delete(index)
      } else {
        state.overlayBlockOriginal.add(index)
      }
      renderOverlay()
    })
    node.addEventListener("dblclick", (event) => {
      event.preventDefault()
      const block = blocks[index] || {}
      copyShotText(
        block.translation || block.text || "",
        "已复制该行译文",
      )
    })
  })
  bindShotMenu("shotMoreBtn", "shotMenu", "shotMenuScrim")
  bindOverlayResize()
  syncOverlayRegion()
}

// In very narrow overlay windows the toolbar collapses to a single "…" button.
// The dropdown lives outside the bar (over the image area), so the host must
// stop treating that area as click-through while the menu is open.
// The overlay window is shaped to the rendered toolbar + chips, so everything
// else shows the real desktop with no blur. Recompute after every render.
function syncOverlayRegion() {
  if (!window.go?.wailsui?.App?.SetOverlayRegion) return
  const dpr = window.devicePixelRatio || 1
  const rects = []
  const polys = []
  const push = (rect, pad = 1) => {
    if (!rect || rect.width < 1 || rect.height < 1) return
    rects.push({
      x: Math.round(rect.left * dpr) - pad,
      y: Math.round(rect.top * dpr) - pad,
      w: Math.round(rect.width * dpr) + pad * 2,
      h: Math.round(rect.height * dpr) + pad * 2,
    })
  }
  const wrapEl = document.querySelector(".shot-image-wrap")
  const wrapRect = wrapEl?.getBoundingClientRect()
  document
    .querySelectorAll(
      ".shot-bar, .shot-block, .shot-pill, .shot-fulltext, .shot-menu.is-open, .shot-edge, .shot-toast.is-visible",
    )
    .forEach((node) => {
      if (node.classList.contains("shot-block")) {
        const angle = Number(node.dataset.angle) || 0
        const parts = String(node.dataset.rect || "")
          .split(",")
          .map(Number)
        if (
          Math.abs(angle) > 0.5 &&
          wrapRect &&
          parts.length === 4 &&
          parts.every(Number.isFinite)
        ) {
          // Rotated chips: the DOM bounding rect is the axis-aligned box, which
          // would show black wedges around tilted text. Send the exact rotated
          // quad so the window region follows the text.
          const [leftPct, topPct, widthPct, heightPct] = parts
          const x = wrapRect.left + (wrapRect.width * leftPct) / 100
          const y = wrapRect.top + (wrapRect.height * topPct) / 100
          const boxW = (wrapRect.width * widthPct) / 100
          const boxH = (wrapRect.height * heightPct) / 100
          const centerX = x + boxW / 2
          const centerY = y + boxH / 2
          const radians = (angle * Math.PI) / 180
          const cos = Math.cos(radians)
          const sin = Math.sin(radians)
          const corners = [
            [x, y],
            [x + boxW, y],
            [x + boxW, y + boxH],
            [x, y + boxH],
          ].map(([px, py]) => {
            const dx = px - centerX
            const dy = py - centerY
            return [
              Math.round((centerX + dx * cos - dy * sin) * dpr),
              Math.round((centerY + dx * sin + dy * cos) * dpr),
            ]
          })
          polys.push(corners)
          return
        }
      }
      push(node.getBoundingClientRect())
    })
  // Keep the accent frame visible: four thin strips along the window edges.
  const winEl = document.querySelector(".shot-window")
  if (winEl) {
    const r = winEl.getBoundingClientRect()
    const thickness = Math.max(2, Math.round(2 * dpr))
    push({ left: r.left, top: r.top, width: r.width, height: thickness })
    push({
      left: r.left,
      top: r.bottom - thickness,
      width: r.width,
      height: thickness,
    })
    push({ left: r.left, top: r.top, width: thickness, height: r.height })
    push({
      left: r.right - thickness,
      top: r.top,
      width: thickness,
      height: r.height,
    })
  }
  SetOverlayRegion(JSON.stringify({ rects, polys })).catch(() => {})
}

function bindOverlayResize() {
  if (!window.go?.wailsui?.App?.SetOverlayWindowRect) return
  document.querySelectorAll(".shot-edge").forEach((edge) => {
    edge.addEventListener("pointerdown", (event) => {
      event.preventDefault()
      event.stopPropagation()
      const dir = edge.dataset.edge || ""
      const startX = event.screenX
      const startY = event.screenY
      const start = {
        x: window.screenX,
        y: window.screenY,
        w: window.outerWidth,
        h: window.outerHeight,
      }
      const onMove = (move) => {
        const dx = move.screenX - startX
        const dy = move.screenY - startY
        let { x, y, w, h } = start
        if (dir === "e") {
          w = Math.max(200, start.w + dx)
        } else if (dir === "w") {
          w = Math.max(200, start.w - dx)
          x = start.x + (start.w - w)
        } else if (dir === "s") {
          h = Math.max(120, start.h + dy)
        } else if (dir === "n") {
          h = Math.max(120, start.h - dy)
          y = start.y + (start.h - h)
        }
        SetOverlayWindowRect(
          Math.round(x),
          Math.round(y),
          Math.round(w),
          Math.round(h),
        ).catch(() => {})
      }
      const onUp = () => {
        window.removeEventListener("pointermove", onMove)
        window.removeEventListener("pointerup", onUp)
      }
      window.addEventListener("pointermove", onMove)
      window.addEventListener("pointerup", onUp, { once: true })
    })
  })
}

function bindShotMenu(moreId, menuId, scrimId) {
  const more = document.querySelector(`#${moreId}`)
  const menu = document.querySelector(`#${menuId}`)
  const scrim = document.querySelector(`#${scrimId}`)
  if (!more || !menu) return
  const win = document.querySelector(".shot-window")

  // Anchor the dropdown to the "…" button itself, flipping above/below and
  // clamping inside the overlay window so it can never leave the screen.
  const positionMenu = () => {
    if (!win || !menu.classList.contains("is-open")) return
    const winRect = win.getBoundingClientRect()
    const winStyle = getComputedStyle(win)
    const borderLeft = parseFloat(winStyle.borderLeftWidth) || 0
    const borderTop = parseFloat(winStyle.borderTopWidth) || 0
    const originX = winRect.left + borderLeft
    const originY = winRect.top + borderTop
    const winWidth = winRect.width - borderLeft * 2
    const winHeight = winRect.height - borderTop * 2
    const buttonRect = more.getBoundingClientRect()
    menu.style.right = "auto"
    menu.style.bottom = "auto"
    menu.style.maxHeight = ""
    const menuRect = menu.getBoundingClientRect()
    const gap = 4
    const edge = 4
    const belowTop = buttonRect.bottom - originY + gap
    const aboveTop = buttonRect.top - originY - menuRect.height - gap
    const spaceBelow = winHeight - belowTop - edge
    const spaceAbove = buttonRect.top - originY - gap - edge
    let top = belowTop
    if (spaceBelow < menuRect.height && spaceAbove > spaceBelow) {
      top = aboveTop
      menu.style.maxHeight = `${Math.max(72, spaceAbove)}px`
    } else {
      menu.style.maxHeight = `${Math.max(72, spaceBelow)}px`
    }
    const maxLeft = winWidth - menuRect.width - edge
    const left = Math.min(
      Math.max(edge, buttonRect.left - originX),
      Math.max(edge, maxLeft),
    )
    menu.style.left = `${left}px`
    menu.style.top = `${Math.max(edge, top)}px`
  }

  const setOpen = (open) => {
    menu.classList.toggle("is-open", open)
    if (scrim) scrim.hidden = !open
    if (open) positionMenu()
    if (window.go?.wailsui?.App?.SetOverlayMenu) {
      SetOverlayMenu(open).catch(() => {})
    }
    syncOverlayRegion()
  }
  more.addEventListener("click", () => {
    setOpen(!menu.classList.contains("is-open"))
  })
  scrim?.addEventListener("click", () => setOpen(false))
  menu.querySelectorAll("[data-target]").forEach((item) => {
    item.addEventListener("click", () => {
      setOpen(false)
      document.querySelector(`#${item.dataset.target}`)?.click()
    })
  })
  // Clicks that land on the shaped window (bar/chips) close the menu; clicks
  // on the transparent desktop pass through to the app underneath instead.
  if (window.__shotMenuDocHandler) {
    document.removeEventListener("pointerdown", window.__shotMenuDocHandler, true)
  }
  window.__shotMenuDocHandler = (event) => {
    if (!menu.classList.contains("is-open")) return
    if (menu.contains(event.target) || more.contains(event.target)) return
    setOpen(false)
  }
  document.addEventListener("pointerdown", window.__shotMenuDocHandler, true)
}

function clampNumber(value, min, max, fallback) {
  const parsed = Number(value)
  if (!Number.isFinite(parsed)) return fallback
  return Math.min(max, Math.max(min, parsed))
}

function renderGlossaryText(text, hits, field) {
  const value = String(text || "")
  if (!value || !Array.isArray(hits) || !hits.length) {
    return escapeHtml(value)
  }

  const matches = []
  const lowerValue = value.toLowerCase()
  for (const hit of hits) {
    const needle = String(
      field === "source" ? hit.term || "" : hit.translation || "",
    )
    if (!needle) continue
    const lowerNeedle = needle.toLowerCase()
    let from = 0
    while (from <= lowerValue.length) {
      const index = lowerValue.indexOf(lowerNeedle, from)
      if (index < 0) break
      matches.push({
        start: index,
        end: index + lowerNeedle.length,
        hit,
      })
      from = index + Math.max(1, lowerNeedle.length)
    }
  }
  if (!matches.length) return escapeHtml(value)

  matches.sort(
    (a, b) =>
      b.end - b.start - (a.end - a.start) || a.start - b.start,
  )
  const chosen = []
  for (const match of matches) {
    if (chosen.some((other) => match.start < other.end && match.end > other.start)) {
      continue
    }
    chosen.push(match)
  }
  chosen.sort((a, b) => a.start - b.start)

  let html = ""
  let cursor = 0
  for (const match of chosen) {
    if (match.start < cursor) continue
    const tip = `${match.hit.term} → ${match.hit.translation}${
      match.hit.source ? ` · ${match.hit.source}` : ""
    }${match.hit.note ? ` · ${match.hit.note}` : ""}`
    html += escapeHtml(value.slice(cursor, match.start))
    html += `<span class="glossary-hit" title="${escapeHtml(tip)}">${escapeHtml(
      value.slice(match.start, match.end),
    )}</span>`
    cursor = match.end
  }
  html += escapeHtml(value.slice(cursor))
  return html
}

function translatedJoin(blocks) {
  return (blocks || [])
    .map((block) => block.translation || "")
    .filter(Boolean)
    .join("\n")
}

function sourceJoin(blocks) {
  return (blocks || [])
    .map((block) => block.text || "")
    .filter(Boolean)
    .join("\n")
}

function copyShotText(text, message) {
  if (!text) return
  const done = () => showShotToast(message)
  if (navigator.clipboard?.writeText) {
    navigator.clipboard.writeText(text).then(done).catch(done)
    return
  }
  done()
}

function showShotToast(message) {
  const toast = document.querySelector("#shotToast")
  if (!toast) return
  toast.textContent = message
  toast.classList.add("is-visible")
  syncOverlayRegion()
  clearTimeout(showShotToast.timer)
  showShotToast.timer = setTimeout(() => {
    toast.classList.remove("is-visible")
    syncOverlayRegion()
  }, 1400)
}

function renderLive() {
  const live = state.live || {}
  const blocks = Array.isArray(live.blocks) ? live.blocks : []
  const status = live.status || "listening"
  const paused = status === "paused"
  const latencyMs = Number(live.latency_ms) || 0
  const latencyText = latencyMs > 0 ? `${(latencyMs / 1000).toFixed(1)}s` : ""
  const progress = clampNumber(Number(live.progress) || 0, 0, 100, 0)
  const stage = String(live.stage || "")
  const lines = Number(live.lines) || 0
  const progressActive = status === "translating" && progress > 0 && progress < 100
  const statusTitle = lines > 0 ? `${lines} 行` : ""
  const translatedOk =
    blocks.some((block) => String(block.translation || "").trim()) ||
    Boolean(live.translation)
  let bubbleText = "监听中"
  let bubbleClass = "is-neutral"
  if (status === "translating") {
    const stageLabel = stage === "ocr" ? "识别中" : "翻译中"
    bubbleText = `${stageLabel}${progress > 0 ? ` · ${progress}%` : ""}`
  } else if (status === "paused") {
    bubbleText = "已暂停"
  } else if (status === "error" || (latencyMs > 0 && !translatedOk)) {
    bubbleText = `未翻译${latencyText ? ` · ${latencyText}` : ""}`
    bubbleClass = "is-warn"
  } else if (latencyMs > 0) {
    bubbleText = `成功 · ${latencyText}`
    bubbleClass = "is-ok"
  }
  const cfg = state.config || {}
  const style = live.style || {}
  const overlayFont = overlayFontStack(
    style.font_family || cfg.overlay_font_family,
  )
  const overlayWeight = clampNumber(
    style.font_weight ?? cfg.overlay_font_weight,
    300,
    800,
    500,
  )
  const barSide = live.bar_side === "bottom" ? "bottom" : "top"
  const anchorW = Number(live.anchor?.width) || 1
  const anchorH = Number(live.anchor?.height) || 1
  const imageAreaH = Math.max(80, (window.innerHeight || 600) - 48)
  const imageAreaW = Math.max(80, (window.innerWidth || 800) - 4)
  const showOriginal = Boolean(state.liveOriginal)
  const blockOriginal = state.liveBlockOriginal || new Set()
  const geometries = blocks.map((block) =>
    overlayBlockGeometry(block, anchorW, anchorH, imageAreaH),
  )

  const renderedBlocks = blocks
    .map((block, index) => {
      const source = String(block.text || "")
      const translated = String(block.translation || "")
      const showSource = showOriginal || blockOriginal.has(index)
      // Empty translations stay blank instead of covering the source text.
      if (!showSource && !translated) return ""
      const text = showSource ? source : translated
      if (!text) return ""
      // Formula lines that the translator left untouched are better left as the
      // original screen content than covered by garbled OCR text.
      if (!showSource && isUnchangedMathLine(source, translated)) return ""
      const { left, top, width, height, fontPx, angle } =
        fitOverlayBlockText({
          text,
          index,
          geometries,
          imageAreaW,
          imageAreaH,
          fontFamily: overlayFont,
          fontWeight: overlayWeight,
        })
      const classes = [
        "shot-block",
        "live-block",
        showSource ? "is-original" : "",
        translated ? "" : "is-untranslated",
      ]
        .filter(Boolean)
        .join(" ")
      const sizeStyle = `width:${width}%;height:${height}%;`
      const textHtml = renderGlossaryText(
        text,
        block.glossary_hits,
        showSource ? "source" : "translation",
      )
      const { textColor, backgroundStyle, textShadow } =
        overlayTextAppearance(block)
      const title = `${showSource ? source : translated} · 单击切换原文/译文，双击复制`
      return `<div class="${classes}" data-live-index="${index}"
        data-rect="${left},${top},${width},${height}" data-angle="${angle}"
        style="left:${left}%;top:${top}%;${sizeStyle}transform:rotate(${angle}deg);font-size:${fontPx}px;color:${textColor};${backgroundStyle}text-shadow:${textShadow};"
        title="${escapeHtml(title)}">${textHtml}</div>`
    })
    .join("")

  const fallback =
    !blocks.length && live.translation
      ? `<div class="shot-fulltext live-fulltext" title="整段译文">${escapeHtml(live.translation)}</div>`
      : ""

  app.innerHTML = `
    <div class="shot-window live-window bar-${barSide}" id="liveWindow"
      style="--shot-font:${overlayFont};--shot-weight:${overlayWeight}">
      <div class="shot-bar" style="--wails-draggable: drag">
        <span class="shot-badge"><span class="live-dot ${status}"></span><span>实时翻译</span></span>
        <span class="trans-status ${bubbleClass}" ${statusTitle ? `title="${escapeHtml(statusTitle)}"` : ""}>${bubbleText}</span>
        <button class="shot-btn ${showOriginal ? "is-active" : ""}" id="liveToggleBtn" type="button" title="切换原文/译文 (H)">
          <i data-lucide="arrow-right-left"></i><span>${showOriginal ? "显示译文" : "显示原文"}</span>
        </button>
        <button class="shot-btn ${paused ? "is-active" : ""}" id="livePauseBtn" type="button" title="${paused ? "继续实时翻译" : "暂停实时翻译"}">
          <i data-lucide="${paused ? "play" : "pause"}"></i><span>${paused ? "继续" : "暂停"}</span>
        </button>
        <button class="shot-btn" id="liveCopySourceBtn" type="button" title="复制原文">
          <i data-lucide="copy"></i><span>原文</span>
        </button>
        <button class="shot-btn" id="liveCopyTargetBtn" type="button" title="复制译文">
          <i data-lucide="languages"></i><span>译文</span>
        </button>
        <button class="shot-btn" id="liveCopyMarkdownBtn" type="button" title="复制 Markdown 译文（公式为 LaTeX）">
          <i data-lucide="file-code-2"></i><span>MD</span>
        </button>
        <button class="shot-btn" id="liveRefreshBtn" type="button" title="刷新实时译文">
          <i data-lucide="rotate-ccw"></i><span>刷新</span>
        </button>
        <button class="shot-btn" id="livePinBtn" type="button" title="${state.livePinned ? "取消置顶" : "置顶"}">
          <i data-lucide="${state.livePinned ? "pin" : "pin-off"}"></i>
        </button>
        <button class="shot-btn shot-more" id="liveMoreBtn" type="button" title="更多功能">
          <i data-lucide="more-horizontal"></i>
        </button>
        <button class="shot-btn shot-close" id="liveCloseBtn" type="button" title="关闭 (Esc)">
          <i data-lucide="x"></i>
        </button>
        ${overlayProgressHtml(progressActive, progress)}
      </div>
      <div class="shot-menu-scrim" id="liveMenuScrim" hidden></div>
      <div class="shot-menu" id="liveMenu" role="menu">
        <button class="shot-menu-item" type="button" data-target="liveToggleBtn">
          <i data-lucide="arrow-right-left"></i><span>${showOriginal ? "显示译文" : "显示原文"}</span>
        </button>
        <button class="shot-menu-item" type="button" data-target="livePauseBtn">
          <i data-lucide="${paused ? "play" : "pause"}"></i><span>${paused ? "继续实时翻译" : "暂停实时翻译"}</span>
        </button>
        <button class="shot-menu-item" type="button" data-target="liveCopySourceBtn">
          <i data-lucide="file-text"></i><span>复制原文</span>
        </button>
        <button class="shot-menu-item" type="button" data-target="liveCopyTargetBtn">
          <i data-lucide="languages"></i><span>复制译文</span>
        </button>
        <button class="shot-menu-item" type="button" data-target="liveCopyMarkdownBtn">
          <i data-lucide="file-code-2"></i><span>复制 Markdown 译文</span>
        </button>
        <button class="shot-menu-item" type="button" data-target="liveRefreshBtn">
          <i data-lucide="rotate-ccw"></i><span>刷新实时译文</span>
        </button>
        <button class="shot-menu-item" type="button" data-target="livePinBtn">
          <i data-lucide="${state.livePinned ? "pin-off" : "pin"}"></i><span>${state.livePinned ? "取消置顶" : "置顶"}</span>
        </button>
        <button class="shot-menu-item" type="button" data-target="liveCloseBtn">
          <i data-lucide="x"></i><span>关闭</span>
        </button>
      </div>
      <div class="shot-image-wrap live-area">
        <div class="shot-blocks">${renderedBlocks}</div>
        ${
          live.error
            ? `<div class="shot-pill is-error" title="${escapeHtml(live.error)}">${escapeHtml(live.error)}</div>`
            : ""
        }
        ${fallback}
      </div>
      <div class="shot-frame-border" aria-hidden="true"></div>
      <div class="shot-edge" data-edge="n"></div>
      <div class="shot-edge" data-edge="s"></div>
      <div class="shot-edge" data-edge="e"></div>
      <div class="shot-edge" data-edge="w"></div>
      <div class="shot-toast" id="shotToast" aria-live="polite"></div>
    </div>
  `

  createIcons({ icons, attrs: { "stroke-width": 1.8 } })

  const setPaused = (next) => {
    if (window.go?.wailsui?.App?.LiveControl) {
      LiveControl(next ? "pause" : "resume").catch(() => {})
    }
    state.live = {
      ...live,
      status: next ? "paused" : "listening",
      stage: "",
      progress: 0,
      lines: 0,
    }
    renderLive()
  }
  document
    .querySelector("#livePauseBtn")
    ?.addEventListener("click", () => setPaused(!paused))
  document.querySelector("#liveToggleBtn")?.addEventListener("click", () => {
    state.liveOriginal = !state.liveOriginal
    renderLive()
  })
  document
    .querySelector("#liveCopySourceBtn")
    ?.addEventListener("click", () => {
      copyShotText(live.source || sourceJoin(blocks), "已复制原文")
    })
  document
    .querySelector("#liveCopyTargetBtn")
    ?.addEventListener("click", () => {
      const text = live.translation || translatedJoin(blocks)
      copyShotText(text, "已复制译文")
    })
  document
    .querySelector("#liveRefreshBtn")
    ?.addEventListener("click", () => {
      if (window.go?.wailsui?.App?.LiveControl) {
        LiveControl("refresh").catch(() => {})
        showShotToast("正在刷新实时译文...")
      }
    })
  document
    .querySelector("#liveCopyMarkdownBtn")
    ?.addEventListener("click", () =>
      copyShotText(
        markdownFromBlocks(blocks, live.translation),
        "已复制 Markdown 译文",
      ),
    )
  document.querySelector("#livePinBtn")?.addEventListener("click", () => {
    state.livePinned = !state.livePinned
    if (window.runtime?.WindowSetAlwaysOnTop) {
      WindowSetAlwaysOnTop(state.livePinned)
    }
    if (window.go?.wailsui?.App?.SetOverlayPinned) {
      SetOverlayPinned(state.livePinned).catch(() => {})
    }
    renderLive()
  })
  document
    .querySelector("#liveCloseBtn")
    ?.addEventListener("click", closeWindow)

  document.querySelectorAll(".live-block").forEach((node) => {
    const index = Number(node.dataset.liveIndex)
    node.addEventListener("click", (event) => {
      if (event.detail > 1) return
      if (!state.liveBlockOriginal) state.liveBlockOriginal = new Set()
      if (state.liveBlockOriginal.has(index)) {
        state.liveBlockOriginal.delete(index)
      } else {
        state.liveBlockOriginal.add(index)
      }
      renderLive()
    })
    node.addEventListener("dblclick", (event) => {
      event.preventDefault()
      const block = blocks[index] || {}
      copyShotText(block.translation || block.text || "", "已复制该行译文")
    })
  })
  bindShotMenu("liveMoreBtn", "liveMenu", "liveMenuScrim")
  bindOverlayResize()
  syncOverlayRegion()
}

function renderResult() {
  const result = state.result || {}
  const isLoading = Boolean(result.loading)
  const hasSource = Boolean((result.source || "").trim())
  const isRecognizing = isLoading && !hasSource
  const hasError = !isLoading && Boolean(result.error)
  const copyText = hasError ? result.error : result.result
  const latencyMs = Number(result.latency_ms) || 0
  resultSignature = JSON.stringify(result)

  if (!document.querySelector(".result-shell")) {
    app.innerHTML = `
    <main class="result-shell">
      <header class="app-header result-header">
        <div class="brand-mark" aria-hidden="true"><i data-lucide="languages"></i></div>
        <div class="header-copy">
          <h1 id="resultTitle">选中翻译</h1>
        </div>
        <div class="result-header-actions">
          <span id="copyStatus" class="result-copy-status" aria-live="polite"></span>
          <button class="icon-button result-copy-button" id="copyBtn" type="button" title="复制译文" aria-label="复制译文">
            <i data-lucide="clipboard"></i>
          </button>
          <button class="icon-button close-button" id="closeBtn" type="button" title="关闭窗口" aria-label="关闭窗口">
            <i data-lucide="x"></i>
          </button>
        </div>
      </header>
      <div class="result-content" id="resultContent"></div>
    </main>
  `
    document.querySelector("#closeBtn").addEventListener("click", closeWindow)
    document.querySelector("#copyBtn").addEventListener("click", () => {
      const textToCopy =
        document.querySelector("#copyBtn")?.dataset?.copyText || ""
      copyResult(textToCopy, "#copyStatus", "#copyBtn")
    })
    createIcons({ icons, attrs: { "stroke-width": 1.8 } })
  }

  const shell = document.querySelector(".result-shell")
  const title = document.querySelector("#resultTitle")
  const content = document.querySelector("#resultContent")
  const copyBtn = document.querySelector("#copyBtn")
  if (!shell || !title || !content || !copyBtn) return

  shell.classList.toggle("is-error", hasError)
  title.textContent = isRecognizing
    ? "正在识别"
    : isLoading
      ? "翻译中"
      : hasError
        ? "翻译失败"
        : "选中翻译"

  copyBtn.dataset.copyText = copyText || ""
  copyBtn.disabled = !copyText
  copyBtn.title = hasError ? "复制错误详情" : "复制译文"
  copyBtn.setAttribute("aria-label", copyBtn.title)
  if (!copyBtn.classList.contains("is-success")) {
    // Keep existing node
  } else {
    copyBtn.classList.remove("is-success")
    copyBtn.innerHTML = '<i data-lucide="clipboard"></i>'
    createIcons({ icons, attrs: { "stroke-width": 1.8 } })
  }
  const copyStatus = document.querySelector("#copyStatus")
  if (copyStatus) {
    copyStatus.textContent = ""
    copyStatus.className = "result-copy-status"
  }

  if (hasError) {
    content.innerHTML = `
      <section class="error-panel" role="alert">
        <span class="section-label">错误详情</span>
        <div class="result-text error-text">${escapeHtml(result.error || "")}</div>
      </section>
    `
  } else {
    content.innerHTML = `
      <section class="text-section translation-section">
        <span class="section-label accent-label">译文</span>
        ${
          isLoading
            ? `
          <div class="translation-loading" role="status" aria-live="polite">
            <span class="loading-spinner" aria-hidden="true"></span>
            <span>${isRecognizing ? "正在识别图片中的文字..." : "正在翻译..."}</span>
          </div>
        `
            : `
          <div class="result-text translated-text">${escapeHtml(result.result || "")}</div>
          ${
            latencyMs > 0
              ? `<div class="translation-latency">耗时 ${(latencyMs / 1000).toFixed(1)}s</div>`
              : ""
          }
        `
        }
      </section>
      ${
        hasSource
          ? `
        <div class="section-rule"></div>
        <section class="text-section">
          <span class="section-label">原文</span>
          <div class="result-text source-text">${escapeHtml(result.source || "")}</div>
        </section>
      `
          : ""
      }
    `
  }

  requestAnimationFrame(fitResultWindow)
}

function engineStatusInfo() {
  const ready = Boolean(state.health?.translator_ready)
  const status = String(state.health?.translator_status || "").trim()
  if (ready) {
    return { label: "引擎就绪", detail: "", state: "ready" }
  }
  if (/失败|error/i.test(status)) {
    return { label: "启动失败", detail: status, state: "error" }
  }
  if (/下载/.test(status)) {
    return { label: "模型下载中", detail: status, state: "loading" }
  }
  if (/加载/.test(status)) {
    return { label: "模型加载中", detail: status, state: "loading" }
  }
  if (/启动|starting/i.test(status)) {
    return { label: "服务启动中", detail: status, state: "loading" }
  }
  return {
    label: "引擎未就绪",
    detail: status || "正在启动本地服务...",
    state: "loading",
  }
}

function renderEngineBanner(engine) {
  const hidden = engine.state === "ready" ? 'style="display:none"' : ""
  return `
    <div id="engineBanner" class="engine-banner ${engine.state === "error" ? "is-error" : ""}" ${hidden}>
      <span class="engine-banner-dot"></span>
      <span class="engine-banner-text" id="engineBannerText">${escapeHtml(
        engine.detail || engine.label,
      )}</span>
    </div>
  `
}

function renderMainWindow() {
  const isTranslateTab = state.active_tab === "translate"
  const isSettingsTab = state.active_tab === "settings"
  const engine = engineStatusInfo()
  const isReady = engine.state === "ready"
  const titleText = isTranslateTab ? "文本翻译" : "设置"

  app.innerHTML = `
    <main class="settings-shell main-window-shell">
      <header class="app-header main-header">
        ${
          isSettingsTab
            ? `
          <button
            class="icon-button header-back-btn"
            id="headerBackBtn"
            type="button"
            title="返回文本翻译"
            aria-label="返回文本翻译"
          >
            <i data-lucide="arrow-left"></i>
          </button>
        `
            : `
          <div class="brand-mark" aria-hidden="true"><i data-lucide="languages"></i></div>
        `
        }
        <div class="header-copy">
          <h1 id="mainTitle">${titleText}</h1>
        </div>
        <div class="engine-state ${isReady ? "is-ready" : engine.state === "error" ? "is-error" : "is-loading"}" id="engineState" title="${escapeHtml(engine.detail || "AI 引擎状态")}">
          <span class="state-dot"></span>
          <span id="engineText">${engine.label}</span>
        </div>
        <div class="main-header-actions">
          ${
            isTranslateTab
              ? `
            <button
              class="icon-button header-tool-button"
              id="openSettingsBtn"
              type="button"
              title="设置"
              aria-label="设置"
            >
              <i data-lucide="sliders"></i>
            </button>
          `
              : ""
          }
          <button class="icon-button close-button" id="mainCloseBtn" type="button" title="关闭窗口" aria-label="关闭窗口">
            <i data-lucide="x"></i>
          </button>
        </div>
      </header>

      <div class="main-body">
        ${renderEngineBanner(engine)}
        ${isTranslateTab ? renderTranslateViewHTML() : renderSettingsViewHTML()}
      </div>

      ${isSettingsTab ? renderSettingsFooterHTML() : ""}
    </main>
  `

  document.querySelector("#headerBackBtn")?.addEventListener("click", () => {
    state.active_tab = "translate"
    state.glossaryPrepared = false
    render()
  })
  document.querySelector("#openSettingsBtn")?.addEventListener("click", () => {
    prepareGlossaryDraft()
    state.glossaryPrepared = true
    state.active_tab = "settings"
    render()
  })
  document
    .querySelector("#mainCloseBtn")
    ?.addEventListener("click", closeWindow)

  // Bind specific view events
  if (isTranslateTab) {
    bindTranslateViewEvents()
  } else {
    bindSettingsViewEvents()
  }

  healthTimer = setInterval(refreshHealth, 2500)
}

function renderTranslateViewHTML() {
  const source = state.manualTranslate.sourceText || ""
  const hasText = Boolean(source.trim())
  const charCount = source.length
  const isLoading = state.manualTranslate.isLoading

  return `
    <div class="translate-workbench">
      <section class="workbench-card source-card" aria-labelledby="sourceHeading">
        <div class="card-toolbar">
          <div class="card-title" id="sourceHeading">
            <i data-lucide="sparkles"></i>
            <span>原文 <span class="card-subtitle">(中英自动识别)</span></span>
          </div>
          <div class="card-actions">
            <button class="card-action-btn" id="importImageBtn" type="button" title="导入图片并识别翻译">
              <i data-lucide="image-plus"></i>
              <span>导入图片</span>
            </button>
            <button class="card-action-btn" id="clearBtn" type="button" title="清空内容" ${hasText ? "" : "disabled"}>
              <i data-lucide="trash-2"></i>
              <span>清空</span>
            </button>
            <span class="char-count" id="charCount">${charCount} 字符</span>
          </div>
        </div>
        <div class="input-wrapper">
          <textarea
            id="sourceInput"
            class="source-textarea"
            placeholder="输入需要翻译的文本，按 Ctrl + Enter 快速翻译；也可按 Ctrl + V 粘贴图片或点击「导入图片」"
            spellcheck="false"
          >${escapeHtml(source)}</textarea>
        </div>
        <input
          id="imageFileInput"
          class="sr-only"
          type="file"
          accept="image/png,image/jpeg,image/webp,image/bmp"
          tabindex="-1"
        />
        <div class="source-bottom-bar">
          <div class="shortcut-hint">
            <span class="kbd-badge">Ctrl</span> + <span class="kbd-badge">Enter</span> 快速翻译
          </div>
          <button
            id="doTranslateBtn"
            class="primary-button translate-submit-button"
            type="button"
            ${isLoading || !hasText ? "disabled" : ""}
          >
            ${
              isLoading
                ? '<span class="loading-spinner"></span><span>翻译中...</span>'
                : '<i data-lucide="arrow-right-left"></i><span>立即翻译</span>'
            }
          </button>
        </div>
      </section>

      <div class="workbench-divider">
        <div class="divider-line"></div>
        <div class="divider-badge" title="双向互译">
          <i data-lucide="arrow-right-left"></i>
        </div>
        <div class="divider-line"></div>
      </div>

      <section class="workbench-card target-card" aria-labelledby="targetHeading">
        <div class="card-toolbar">
          <div class="card-title accent-title" id="targetHeading">
            <i data-lucide="languages"></i>
            <span>译文</span>
          </div>
          <div class="card-actions">
            <span class="translation-latency" id="manualLatency"></span>
            <span id="manualCopyStatus" class="result-copy-status" aria-live="polite"></span>
            <button
              class="icon-button result-copy-button"
              id="manualCopyBtn"
              type="button"
              title="复制译文"
              aria-label="复制译文"
              ${!state.manualTranslate.translatedText ? "disabled" : ""}
            >
              <i data-lucide="clipboard"></i>
            </button>
          </div>
        </div>
        <div class="result-box" id="manualResultBox">${renderResultBoxContent()}</div>
      </section>
    </div>
  `
}

function renderResultBoxContent() {
  const { isLoading, error, translatedText, loadingText, latencyMs } =
    state.manualTranslate

  if (isLoading) {
    return `
      <div class="translation-loading manual-loading" role="status" aria-live="polite">
        <span class="loading-spinner" aria-hidden="true"></span>
        <span>${escapeHtml(loadingText || "正在进行离线大模型推理翻译...")}</span>
      </div>
    `
  }

  if (error) {
    return `
      <div class="error-panel" role="alert">
        <span class="section-label">翻译失败</span>
        <div class="result-text error-text">${escapeHtml(error.trim())}</div>
        <button class="retry-button" id="manualRetryBtn" type="button">
          <i data-lucide="rotate-ccw"></i>
          <span>重试</span>
        </button>
      </div>
    `
  }

  if (translatedText && translatedText.trim()) {
    return `<div class="result-text translated-text manual-translated-text" tabindex="0">${escapeHtml(translatedText.trim())}</div>`
  }

  return `
    <div class="manual-placeholder">
      <div class="placeholder-icon"><i data-lucide="languages"></i></div>
      <p>在上方输入文本并点击“立即翻译”或按 <span class="kbd-badge">Ctrl + Enter</span></p>
    </div>
  `
}

function bindTranslateViewEvents() {
  const textarea = document.querySelector("#sourceInput")
  const charCount = document.querySelector("#charCount")
  const clearBtn = document.querySelector("#clearBtn")
  const translateBtn = document.querySelector("#doTranslateBtn")
  const copyBtn = document.querySelector("#manualCopyBtn")
  const importBtn = document.querySelector("#importImageBtn")
  const imageInput = document.querySelector("#imageFileInput")

  importBtn?.addEventListener("click", () => imageInput?.click())
  imageInput?.addEventListener("change", (event) => {
    const file = event.target.files?.[0]
    event.target.value = ""
    if (file) translateImageFile(file)
  })

  if (textarea) {
    textarea.addEventListener("input", () => {
      state.manualTranslate.sourceText = textarea.value
      const len = textarea.value.length
      const hasText = Boolean(textarea.value.trim())
      if (charCount) charCount.textContent = `${len} 字符`
      if (clearBtn) clearBtn.disabled = !hasText
      if (translateBtn)
        translateBtn.disabled = !hasText || state.manualTranslate.isLoading
    })

    textarea.addEventListener("keydown", (event) => {
      if ((event.ctrlKey || event.metaKey) && event.key === "Enter") {
        event.preventDefault()
        handleManualTranslate()
      }
    })
  }

  translateBtn?.addEventListener("click", handleManualTranslate)

  clearBtn?.addEventListener("click", () => {
    if (textarea) {
      textarea.value = ""
      textarea.focus()
    }
    state.manualTranslate.sourceText = ""
    state.manualTranslate.translatedText = ""
    state.manualTranslate.error = ""
    if (charCount) charCount.textContent = "0 字符"
    if (clearBtn) clearBtn.disabled = true
    if (translateBtn) translateBtn.disabled = true
    updateManualResultBox()
  })

  copyBtn?.addEventListener("click", () => {
    copyResult(
      state.manualTranslate.translatedText,
      "#manualCopyStatus",
      "#manualCopyBtn",
    )
  })

  document
    .querySelector("#manualRetryBtn")
    ?.addEventListener("click", handleManualTranslate)
}

async function handleManualTranslate() {
  const text = (state.manualTranslate.sourceText || "").trim()
  if (!text || state.manualTranslate.isLoading) return

  state.manualTranslate.isLoading = true
  state.manualTranslate.error = ""
  state.manualTranslate.loadingText = "正在进行离线大模型推理翻译..."
  state.manualTranslate.latencyMs = 0
  updateManualResultBox()

  const translateBtn = document.querySelector("#doTranslateBtn")
  if (translateBtn) {
    translateBtn.disabled = true
    translateBtn.innerHTML =
      '<span class="loading-spinner"></span><span>翻译中...</span>'
  }

  try {
    let resultText = ""
    let latencyMs = 0
    if (window.go?.wailsui?.App?.Translate) {
      const res = await Translate(text)
      resultText = (res?.result || "").trim()
      latencyMs = Number(res?.latency_ms) || 0
    } else {
      // Mock for Vite local preview
      await new Promise((resolve) => setTimeout(resolve, 600))
      resultText = `好的设计让工具退居幕后，专注于创造价值。（模拟译文：${text}）`
      latencyMs = 600
    }
    state.manualTranslate.translatedText = resultText.trim()
    state.manualTranslate.latencyMs = latencyMs
  } catch (error) {
    state.manualTranslate.error = readableError(error)
    state.manualTranslate.latencyMs = 0
  } finally {
    state.manualTranslate.isLoading = false
    state.manualTranslate.loadingText = ""
    updateManualResultBox()
    const btn = document.querySelector("#doTranslateBtn")
    if (btn) {
      btn.disabled = !state.manualTranslate.sourceText.trim()
      btn.innerHTML =
        '<i data-lucide="arrow-right-left"></i><span>立即翻译</span>'
      createIcons({ icons, attrs: { "stroke-width": 1.8 } })
    }
  }
}

async function translateImageFile(file) {
  if (!file || !String(file.type || "").startsWith("image/")) {
    state.manualTranslate.error = "请选择图片文件"
    updateManualResultBox()
    return
  }
  if (state.manualTranslate.isLoading) return

  state.manualTranslate.isLoading = true
  state.manualTranslate.error = ""
  state.manualTranslate.loadingText = "正在识别图片文字并翻译..."
  state.manualTranslate.latencyMs = 0
  updateManualResultBox()

  const translateBtn = document.querySelector("#doTranslateBtn")
  if (translateBtn) {
    translateBtn.disabled = true
    translateBtn.innerHTML =
      '<span class="loading-spinner"></span><span>识别中...</span>'
  }

  try {
    const dataUrl = await imageFileToPngDataUrl(file)
    let result
    if (window.go?.wailsui?.App?.OCRTranslateImage) {
      result = await OCRTranslateImage(dataUrl)
    } else {
      // Mock for Vite local preview
      await new Promise((resolve) => setTimeout(resolve, 700))
      result = {
        source: "PopTrans supports pasting screenshots directly.",
        result: "PopTrans 支持直接粘贴截图进行翻译。",
        latency_ms: 700,
      }
    }

    const source = String(result?.source || "").trim()
    const translated = String(result?.result || "").trim()
    if (!source && !translated) {
      throw new Error("未识别到文字")
    }

    state.manualTranslate.sourceText = source
    state.manualTranslate.translatedText = translated
    state.manualTranslate.latencyMs = Number(result?.latency_ms) || 0

    const textarea = document.querySelector("#sourceInput")
    if (textarea) textarea.value = source
    const charCount = document.querySelector("#charCount")
    if (charCount) charCount.textContent = `${source.length} 字符`
    const clearBtn = document.querySelector("#clearBtn")
    if (clearBtn) clearBtn.disabled = !source
    if (translateBtn) translateBtn.disabled = !source
  } catch (error) {
    state.manualTranslate.error = readableError(error)
  } finally {
    state.manualTranslate.isLoading = false
    state.manualTranslate.loadingText = ""
    updateManualResultBox()
    const btn = document.querySelector("#doTranslateBtn")
    if (btn) {
      btn.disabled = !state.manualTranslate.sourceText.trim()
      btn.innerHTML =
        '<i data-lucide="arrow-right-left"></i><span>立即翻译</span>'
      createIcons({ icons, attrs: { "stroke-width": 1.8 } })
    }
  }
}

async function imageFileToPngDataUrl(file) {
  let image
  try {
    image = await createImageBitmap(file)
  } catch {
    image = await loadImageElement(file)
  }

  const maxSide = 2000
  const width = Number(image.width) || 1
  const height = Number(image.height) || 1
  const scale = Math.min(1, maxSide / Math.max(width, height))
  const targetWidth = Math.max(1, Math.round(width * scale))
  const targetHeight = Math.max(1, Math.round(height * scale))

  const canvas = document.createElement("canvas")
  canvas.width = targetWidth
  canvas.height = targetHeight
  const context = canvas.getContext("2d")
  context.drawImage(image, 0, 0, targetWidth, targetHeight)
  image.close?.()
  return canvas.toDataURL("image/png")
}

function loadImageElement(file) {
  return new Promise((resolve, reject) => {
    const url = URL.createObjectURL(file)
    const image = new Image()
    image.onload = () => {
      URL.revokeObjectURL(url)
      resolve(image)
    }
    image.onerror = () => {
      URL.revokeObjectURL(url)
      reject(new Error("无法读取图片"))
    }
    image.src = url
  })
}

function handleImagePaste(event) {
  if (
    state.mode !== "translate" ||
    state.active_tab !== "translate" ||
    !document.querySelector(".translate-workbench")
  ) {
    return
  }
  const items = event.clipboardData?.items || []
  for (const item of items) {
    if (item.kind === "file" && String(item.type).startsWith("image/")) {
      const file = item.getAsFile()
      if (file) {
        event.preventDefault()
        translateImageFile(file)
        return
      }
    }
  }
}

function handleImageDrop(event) {
  if (
    state.mode !== "translate" ||
    state.active_tab !== "translate" ||
    !document.querySelector(".translate-workbench")
  ) {
    return
  }
  const files = event.dataTransfer?.files || []
  for (const file of files) {
    if (String(file.type || "").startsWith("image/")) {
      event.preventDefault()
      translateImageFile(file)
      return
    }
  }
}

function updateManualResultBox() {
  const box = document.querySelector("#manualResultBox")
  if (box) {
    box.innerHTML = renderResultBoxContent()
    createIcons({ icons, attrs: { "stroke-width": 1.8 } })
    document
      .querySelector("#manualRetryBtn")
      ?.addEventListener("click", handleManualTranslate)
  }

  const latency = document.querySelector("#manualLatency")
  if (latency) {
    const value = Number(state.manualTranslate.latencyMs) || 0
    latency.textContent = value > 0 ? `耗时 ${(value / 1000).toFixed(1)}s` : ""
  }

  const copyBtn = document.querySelector("#manualCopyBtn")
  if (copyBtn) {
    copyBtn.disabled = !state.manualTranslate.translatedText
    if (copyBtn.classList.contains("is-success")) {
      copyBtn.classList.remove("is-success")
      copyBtn.innerHTML = '<i data-lucide="clipboard"></i>'
      createIcons({ icons, attrs: { "stroke-width": 1.8 } })
    }
  }
}

function renderSettingsViewHTML() {
  if (!state.glossaryPrepared) {
    prepareGlossaryDraft()
    state.glossaryPrepared = true
  }
  const tab =
    state.settingsTab === "glossary"
      ? "glossary"
      : state.settingsTab === "models"
        ? "models"
        : "general"
  return `
    <div class="settings-content">
      ${renderSettingsSubTabs(tab)}
      ${
        tab === "glossary"
          ? renderGlossaryPage()
          : tab === "models"
            ? renderModelsPage()
            : renderGeneralSettings()
      }
    </div>
  `
}

function renderSettingsSubTabs(tab) {
  const count = (state.glossaryDraft || []).filter(
    (entry) => entry.term.trim() && entry.translation.trim(),
  ).length
  return `
    <div class="settings-subtabs" role="tablist" aria-label="设置分类">
      <button class="settings-subtab ${tab === "general" ? "is-active" : ""}" data-settings-tab="general" type="button">常规</button>
      <button class="settings-subtab ${tab === "models" ? "is-active" : ""}" data-settings-tab="models" type="button">
        模型 <span class="subtab-count">${(state.config.models || []).length}</span>
      </button>
      <button class="settings-subtab ${tab === "glossary" ? "is-active" : ""}" data-settings-tab="glossary" type="button">
        词汇本 <span class="subtab-count">${count}</span>
      </button>
    </div>
  `
}

function renderGeneralSettings() {
  return `
      <section class="settings-section" aria-labelledby="themeHeading">
        <div class="section-heading">
          <div class="section-icon"><i data-lucide="sun"></i></div>
          <div>
            <h2 id="themeHeading">外观</h2>
            <p>选择应用界面的显示主题</p>
          </div>
        </div>
        <div class="theme-options" role="radiogroup" aria-labelledby="themeHeading">
          ${themeOption("light", "sun", "浅色")}
          ${themeOption("dark", "moon", "深色")}
          ${themeOption("system", "monitor", "跟随系统")}
        </div>
      </section>

      <div class="section-rule"></div>

      <section class="settings-section" aria-labelledby="deviceHeading">
        <div class="section-heading">
          <div class="section-icon"><i data-lucide="zap"></i></div>
          <div>
            <h2 id="deviceHeading">推理硬件加速</h2>
            <p>选择翻译模型运行设备（保存后自动重启 AI 服务生效）</p>
          </div>
        </div>
        <div class="theme-options" role="radiogroup" aria-labelledby="deviceHeading">
          ${deviceOption("auto", "sparkles", "自动检测")}
          ${deviceOption("gpu", "zap", "GPU 显卡加速")}
          ${deviceOption("cpu", "cpu", "仅使用 CPU")}
        </div>
        ${renderCurrentDeviceBadge()}
        <div class="port-field">
          <label class="field-label" for="aiThreads">CPU 线程数</label>
          <input
            class="number-input"
            id="aiThreads"
            type="number"
            inputmode="numeric"
            min="0"
            max="256"
            step="1"
            value="${Number.isInteger(Number(state.config.ai_threads)) ? Number(state.config.ai_threads) : 0}"
            aria-describedby="aiThreadsHelp"
          />
          <p class="field-help" id="aiThreadsHelp">0 = 自动；修改后重启 AI 服务生效</p>
        </div>
      </section>

      <div class="section-rule"></div>

      <section class="settings-section" aria-labelledby="languageHeading">
        <div class="section-heading">
          <div class="section-icon"><i data-lucide="languages"></i></div>
          <div>
            <h2 id="languageHeading">翻译语言</h2>
            <p>指定原文与译文语言；自动模式按中英互译</p>
          </div>
        </div>
        <div class="language-row">
          <div class="language-field">
            <label class="field-label" for="sourceLanguage">原文语言</label>
            <select class="select-input" id="sourceLanguage">
              ${languageOptions(state.config.source_language, "自动检测")}
            </select>
          </div>
          <div class="language-field">
            <label class="field-label" for="targetLanguage">译文语言</label>
            <select class="select-input" id="targetLanguage">
              ${languageOptions(state.config.target_language, "自动（中英互译）")}
            </select>
          </div>
        </div>
      </section>

      <div class="section-rule"></div>

      <section class="settings-section" aria-labelledby="overlayStyleHeading">
        <div class="section-heading">
          <div class="section-icon"><i data-lucide="monitor-up"></i></div>
          <div>
            <h2 id="overlayStyleHeading">截图译文样式</h2>
            <p>文字颜色按背景亮度自动选择；背景自动模糊，字号匹配原文字大小</p>
          </div>
        </div>
        <div class="style-grid">
          <label class="style-field" for="overlayFontFamily">
            <span class="field-label">字体</span>
            <select class="select-input" id="overlayFontFamily">
              ${OVERLAY_FONT_OPTIONS.map(
                ([value, label]) =>
                  `<option value="${value}" ${(state.config.overlay_font_family || "system") === value ? "selected" : ""}>${label}</option>`,
              ).join("")}
            </select>
          </label>
        </div>
        <div class="range-field">
          <label class="field-label" for="overlayFontWeight">
            字体粗细 <span id="overlayFontWeightValue">${overlayWeightLabel(state.config.overlay_font_weight)}</span>
          </label>
          <input type="range" id="overlayFontWeight" min="300" max="800" step="100"
            value="${clampNumber(state.config.overlay_font_weight, 300, 800, 500)}" />
        </div>
        <div class="overlay-style-preview" title="背景由原屏幕模糊生成；字号自动匹配原文字大小">
          <span id="overlayStylePreview">译文预览 Aa</span>
        </div>
      </section>

      <div class="section-rule"></div>

      <section class="settings-section" aria-labelledby="keyboardHeading">
        <div class="section-heading">
          <div class="section-icon"><i data-lucide="keyboard"></i></div>
          <div>
            <h2 id="keyboardHeading">文本翻译</h2>
            <p>选中文字后按下快捷键</p>
          </div>
        </div>
        <label class="field-label" for="hotkeyDisplay">触发快捷键</label>
        <input class="hotkey-input" id="hotkeyDisplay" value="${escapeHtml(state.config.hotkey_display || "")}" readonly aria-describedby="hotkeyHelp" />
        <p class="field-help" id="hotkeyHelp">点击输入框，然后按下新的组合键</p>
      </section>

      <div class="section-rule"></div>

      <section class="settings-section" aria-labelledby="ocrHeading">
        <div class="section-heading with-toggle">
          <div class="section-icon"><i data-lucide="monitor-up"></i></div>
          <div>
            <h2 id="ocrHeading">截图翻译</h2>
            <p>框选屏幕区域并识别文字</p>
          </div>
          <label class="switch" title="启用截图翻译">
            <input id="ocrEnabled" type="checkbox" ${state.config.ocr_enabled ? "checked" : ""} />
            <span class="switch-track"><span class="switch-thumb"></span></span>
            <span class="sr-only">启用截图翻译</span>
          </label>
        </div>
        <div id="ocrHotkeyField" class="ocr-field ${state.config.ocr_enabled ? "" : "is-disabled"}">
          <label class="field-label" for="ocrHotkeyDisplay">触发快捷键</label>
          <input class="hotkey-input" id="ocrHotkeyDisplay" value="${escapeHtml(state.config.ocr_hotkey_display || "")}" readonly ${state.config.ocr_enabled ? "" : "disabled"} />
        </div>
      </section>

      <div class="section-rule"></div>

      <section class="settings-section" aria-labelledby="liveHeading">
        <div class="section-heading with-toggle">
          <div class="section-icon"><i data-lucide="monitor-up"></i></div>
          <div>
            <h2 id="liveHeading">实时翻译</h2>
            <p>框选屏幕区域，内容变化时自动识别并翻译</p>
          </div>
          <label class="switch" title="启用实时翻译">
            <input id="liveEnabled" type="checkbox" ${state.config.live_enabled !== false ? "checked" : ""} />
            <span class="switch-track"><span class="switch-thumb"></span></span>
            <span class="sr-only">启用实时翻译</span>
          </label>
        </div>
        <div id="liveHotkeyField" class="ocr-field ${state.config.live_enabled !== false ? "" : "is-disabled"}">
          <label class="field-label" for="liveHotkeyDisplay">触发快捷键</label>
          <input class="hotkey-input" id="liveHotkeyDisplay" value="${escapeHtml(state.config.live_hotkey_display || "")}" readonly ${state.config.live_enabled !== false ? "" : "disabled"} />
        </div>
      </section>

      <div class="section-rule"></div>

      <section class="settings-section" aria-labelledby="loggingHeading">
        <div class="section-heading with-toggle">
          <div class="section-icon"><i data-lucide="file-text"></i></div>
          <div>
            <h2 id="loggingHeading">运行日志</h2>
            <p>仅在排查问题时记录应用运行信息</p>
          </div>
          <label class="switch" title="启用运行日志">
            <input id="loggingEnabled" type="checkbox" ${state.config.logging_enabled ? "checked" : ""} />
            <span class="switch-track"><span class="switch-thumb"></span></span>
            <span class="sr-only">启用运行日志</span>
          </label>
        </div>
      </section>

      <div class="section-rule"></div>

      <section class="settings-section" aria-labelledby="launchHeading">
        <div class="section-heading with-toggle">
          <div class="section-icon"><i data-lucide="power"></i></div>
          <div>
            <h2 id="launchHeading">开机启动</h2>
            <p>登录 Windows 后自动在后台启动</p>
          </div>
          <label class="switch" title="开机启动">
            <input id="launchAtLogin" type="checkbox" ${state.config.launch_at_login ? "checked" : ""} />
            <span class="switch-track"><span class="switch-thumb"></span></span>
            <span class="sr-only">开机启动</span>
          </label>
        </div>
      </section>

      <div class="section-rule"></div>

      <section class="settings-section" aria-labelledby="aiIdleHeading">
        <div class="section-heading with-toggle">
          <div class="section-icon"><i data-lucide="languages"></i></div>
          <div>
            <h2 id="aiIdleHeading">模型常驻后台</h2>
            <p>保持 AI 引擎运行，截图翻译不再等待模型冷启动</p>
          </div>
          <label class="switch" title="模型常驻后台">
            <input id="aiKeepAlive" type="checkbox" ${state.config.ai_keep_alive !== false ? "checked" : ""} />
            <span class="switch-track"><span class="switch-thumb"></span></span>
            <span class="sr-only">模型常驻后台</span>
          </label>
        </div>
        <div id="aiIdleField" class="port-field ${state.config.ai_keep_alive !== false ? "is-disabled" : ""}">
          <label class="field-label" for="aiIdleMinutes">空闲分钟</label>
          <input
            class="number-input"
            id="aiIdleMinutes"
            type="number"
            inputmode="numeric"
            min="0"
            step="1"
            value="${Number.isInteger(Number(state.config.ai_idle_minutes)) ? Number(state.config.ai_idle_minutes) : 15}"
            aria-describedby="aiIdleHelp"
            ${state.config.ai_keep_alive !== false ? "disabled" : ""}
          />
          <p class="field-help" id="aiIdleHelp">关闭常驻后生效，0 也表示不自动退出</p>
        </div>
      </section>

      <div class="section-rule"></div>

      <section class="settings-section" aria-labelledby="downloadHeading">
        <div class="section-heading">
          <div class="section-icon"><i data-lucide="network"></i></div>
          <div>
            <h2 id="downloadHeading">模型下载源</h2>
            <p>首次下载翻译模型时使用，默认魔搭（国内推荐）</p>
          </div>
        </div>
        <div class="theme-options" role="radiogroup" aria-labelledby="downloadHeading">
          ${sourceOption("modelscope", "sparkles", "魔搭")}
          ${sourceOption("huggingface", "network", "HuggingFace")}
          ${sourceOption("custom", "sliders", "自定义")}
        </div>
        <div id="customSourceField" class="source-field ${(state.config.model_download_source || "modelscope") === "custom" ? "" : "is-disabled"}">
          <label class="field-label" for="modelDownloadBaseUrl">镜像地址</label>
          <input
            class="text-input"
            id="modelDownloadBaseUrl"
            type="text"
            value="${escapeHtml(state.config.model_download_base_url || "")}"
            placeholder="https://hf-mirror.com"
            ${(state.config.model_download_source || "modelscope") === "custom" ? "" : "disabled"}
          />
          <p class="field-help" id="modelDownloadHelp">可填镜像站点根地址，或完整的 .gguf 文件链接</p>
        </div>
      </section>

      <div class="section-rule"></div>

      <section class="settings-section" aria-labelledby="serverHeading">
        <div class="section-heading">
          <div class="section-icon"><i data-lucide="network"></i></div>
          <div>
            <h2 id="serverHeading">本地服务端口</h2>
            <p>修改后将自动重启本地 AI 服务</p>
          </div>
        </div>
        <div class="port-field">
          <label class="field-label" for="serverPort">端口</label>
          <input
            class="number-input"
            id="serverPort"
            type="number"
            inputmode="numeric"
            min="1024"
            max="65535"
            step="1"
            value="${Number(state.config.server_port) || 8989}"
            aria-describedby="serverPortHelp"
          />
          <p class="field-help" id="serverPortHelp">可用范围 1024-65535</p>
        </div>
      </section>
  `
}

function formatBytes(bytes) {
  const value = Number(bytes) || 0
  if (value < 1024) return `${value} B`
  if (value < 1024 * 1024) return `${(value / 1024).toFixed(0)} KB`
  if (value < 1024 * 1024 * 1024) {
    return `${(value / (1024 * 1024)).toFixed(0)} MB`
  }
  return `${(value / (1024 * 1024 * 1024)).toFixed(2)} GB`
}

function modelSourceLabel(source) {
  return source === "huggingface" ? "HuggingFace" : "魔搭 ModelScope"
}

function modelDisplaySize(entry, status) {
  const size = Number(status?.size) || Number(entry?.size) || 0
  return size > 0 ? formatBytes(size) : "大小未知"
}

async function refreshModelStatuses() {
  if (!window.go?.wailsui?.App?.ModelStatuses) return
  try {
    const statuses = await ModelStatuses(state.config.models || [])
    const map = {}
    for (const item of statuses || []) {
      if (item?.id) map[item.id] = item
    }
    state.modelStatuses = map
  } catch {
    state.modelStatuses = state.modelStatuses || {}
  }
}

async function persistModelCatalogue() {
  try {
    await SaveConfig(state.config)
    const loaded = await State()
    if (loaded?.config) state.config = loaded.config
  } catch (error) {
    state.modelSearch.error = String(error?.message || error)
  }
  await refreshModelStatuses()
}

function renderModelsPage() {
  if (!state.modelStatusesLoaded && window.go?.wailsui?.App?.ModelStatuses) {
    state.modelStatusesLoaded = true
    refreshModelStatuses().then(() => {
      if (state.settingsTab === "models" && state.active_tab === "settings") {
        render()
      }
    })
  }
  const models = Array.isArray(state.config.models) ? state.config.models : []
  const activeId = state.config.active_model_id || ""
  const download = state.modelDownload || {}
  const downloadActive = download.status === "downloading"
  const cards = models
    .map((entry) => {
      const status = state.modelStatuses[entry.id] || {
        installed: false,
        size: 0,
      }
      const active = entry.id === activeId
      const busy =
        downloadActive &&
        download.repo === entry.repo &&
        download.file === entry.file
      const actions = [
        active
          ? '<span class="model-pill is-active">使用中</span>'
          : status.installed
            ? `<button class="model-action" data-model-use="${escapeHtml(entry.id)}" type="button">使用</button>`
            : "",
        !status.installed && !busy
          ? `<button class="model-action is-primary" data-model-download="${escapeHtml(entry.id)}" type="button">下载</button>`
          : "",
        status.installed
          ? `<button class="model-action is-danger" data-model-delete="${escapeHtml(entry.id)}" type="button">删除</button>`
          : "",
      ]
        .filter(Boolean)
        .join("")
      return `
        <div class="model-card ${active ? "is-active" : ""}">
          <div class="model-card-main">
            <div class="model-card-title">
              <strong>${escapeHtml(entry.name || entry.repo)}</strong>
              ${status.installed ? '<span class="model-pill is-ok">已下载</span>' : '<span class="model-pill">未下载</span>'}
            </div>
            <div class="model-card-meta">
              ${escapeHtml(entry.repo)} · ${modelSourceLabel(entry.source)} · ${modelDisplaySize(entry, status)}
            </div>
          </div>
          <div class="model-card-actions">${actions}</div>
        </div>
      `
    })
    .join("")

  return `
    <section class="settings-section" aria-labelledby="modelsHeading">
      <div class="section-heading">
        <div class="section-icon"><i data-lucide="cpu"></i></div>
        <div>
          <h2 id="modelsHeading">翻译模型</h2>
          <p>下载并选择本地运行的 GGUF 模型</p>
        </div>
      </div>
      <div class="model-list">
        ${cards || '<div class="model-empty">还没有配置模型</div>'}
      </div>
      <div class="model-download ${downloadActive || download.status === "error" ? "is-active" : ""}" id="modelDownloadBox">
        <div class="model-download-head">
          <span id="modelDownloadLabel">${
            downloadActive
              ? `正在下载 ${escapeHtml(download.name || download.file || "")}`
              : download.status === "error"
                ? "下载失败"
                : "模型下载"
          }</span>
          ${
            downloadActive
              ? '<button class="model-action is-danger" id="modelDownloadCancel" type="button">取消</button>'
              : ""
          }
        </div>
        ${
          downloadActive
            ? `<div class="model-download-track"><div id="modelDownloadFill" style="width:${Math.max(0, Math.min(100, Number(download.percent) || 0))}%"></div></div>
               <div class="model-download-meta" id="modelDownloadMeta">${formatBytes(download.downloaded || 0)} / ${download.total ? formatBytes(download.total) : "?"} · ${formatBytes(download.speed || 0)}/s</div>`
            : download.status === "error"
              ? `<div class="model-download-meta">${escapeHtml(download.error || "下载失败")}</div>`
              : ""
        }
      </div>
      <button class="model-search-toggle" id="modelSearchToggle" type="button">
        <i data-lucide="search"></i>
        <span>${state.modelSearch.open ? "收起搜索" : "搜索模型下载"}</span>
      </button>
      ${state.modelSearch.open ? renderModelSearchPanel() : ""}
    </section>
  `
}

function renderModelSearchPanel() {
  const search = state.modelSearch || {}
  const results = Array.isArray(search.results) ? search.results : []
  const files = Array.isArray(search.files) ? search.files : []
  const sourceOptions = [
    ["modelscope", "魔搭 ModelScope"],
    ["huggingface", "HuggingFace"],
  ]
    .map(
      ([value, label]) =>
        `<option value="${value}" ${search.source === value ? "selected" : ""}>${label}</option>`,
    )
    .join("")

  let body = ""
  if (search.loading || search.filesLoading) {
    body = '<div class="model-search-loading"><span class="loading-spinner"></span><span>正在查询...</span></div>'
  } else if (search.error) {
    body = `<div class="model-search-error">${escapeHtml(search.error)}</div>`
  } else if (search.repo) {
    const rows = files.length
      ? files
          .map(
            (file) => `
            <div class="model-file-row">
              <div class="model-file-name" title="${escapeHtml(file.name)}">${escapeHtml(file.name)}</div>
              <span class="model-file-size">${formatBytes(file.size || 0)}</span>
              <button class="model-action is-primary" data-model-file="${escapeHtml(file.name)}" data-model-size="${Number(file.size) || 0}" type="button" ${state.modelDownload?.status === "downloading" ? "disabled" : ""}>下载</button>
            </div>
          `,
          )
          .join("")
      : '<div class="model-search-error">该仓库没有找到 GGUF 文件</div>'
    body = `
      <div class="model-file-head">
        <button class="model-action" data-model-back="1" type="button">返回</button>
        <span title="${escapeHtml(search.repo)}">${escapeHtml(search.repo)}</span>
      </div>
      <div class="model-file-list">${rows}</div>
    `
  } else if (results.length) {
    body = `<div class="model-result-list">${results
      .map((result) => {
        const meta = [
          result.parameters ? `参数 ${result.parameters}` : "",
          Number(result.size_bytes) > 0
            ? `仓库 ${formatBytes(result.size_bytes)}`
            : "",
          result.downloads
            ? `下载 ${Number(result.downloads).toLocaleString()} 次`
            : modelSourceLabel(result.source),
        ]
          .filter(Boolean)
          .join(" · ")
        return `
        <button class="model-result" data-model-repo="${escapeHtml(result.id)}" type="button">
          <span class="model-result-name">${escapeHtml(result.name || result.id)}</span>
          <span class="model-result-meta">${meta}</span>
        </button>
      `
      })
      .join("")}</div>`
  } else if (search.query) {
    body = '<div class="model-search-error">没有找到匹配的模型</div>'
  } else {
    body = '<div class="model-search-hint">输入关键词搜索可下载的 GGUF 模型，例如 “Hy-MT2” 或 “Qwen2.5”。</div>'
  }

  return `
    <div class="model-search">
      <div class="model-search-row">
        <select class="select-input" id="modelSearchSource">${sourceOptions}</select>
        <input class="text-input" id="modelSearchQuery" type="text"
          value="${escapeHtml(search.query || "")}"
          placeholder="搜索模型，例如 Hy-MT2" />
        <button class="model-action is-primary" id="modelSearchBtn" type="button">搜索</button>
      </div>
      ${body}
    </div>
  `
}

function bindModelsPageControls() {
  if (state.settingsTab !== "models") return
  const search = state.modelSearch
  document.querySelector("#modelSearchToggle")?.addEventListener("click", () => {
    search.open = !search.open
    render()
  })
  document
    .querySelector("#modelSearchSource")
    ?.addEventListener("change", (event) => {
      search.source = event.target.value === "huggingface" ? "huggingface" : "modelscope"
    })
  const queryInput = document.querySelector("#modelSearchQuery")
  queryInput?.addEventListener("keydown", (event) => {
    if (event.key === "Enter") {
      event.preventDefault()
      runModelSearch()
    }
  })
  document.querySelector("#modelSearchBtn")?.addEventListener("click", runModelSearch)
  document.querySelectorAll("[data-model-repo]").forEach((button) => {
    button.addEventListener("click", () => loadModelFiles(button.dataset.modelRepo))
  })
  document.querySelector("[data-model-back]")?.addEventListener("click", () => {
    search.repo = ""
    search.repoName = ""
    search.files = []
    search.error = ""
    render()
  })
  document.querySelectorAll("[data-model-file]").forEach((button) => {
    button.addEventListener("click", () =>
      startModelFileDownload(
        button.dataset.modelFile,
        Number(button.dataset.modelSize) || 0,
      ),
    )
  })
  document.querySelector("#modelDownloadCancel")?.addEventListener("click", () => {
    CancelModelDownload().catch(() => {})
  })
  document.querySelectorAll("[data-model-use]").forEach((button) => {
    button.addEventListener("click", () => useModel(button.dataset.modelUse))
  })
  document.querySelectorAll("[data-model-download]").forEach((button) => {
    button.addEventListener("click", () =>
      downloadConfiguredModel(button.dataset.modelDownload),
    )
  })
  document.querySelectorAll("[data-model-delete]").forEach((button) => {
    button.addEventListener("click", () =>
      deleteConfiguredModel(button.dataset.modelDelete),
    )
  })
}

async function runModelSearch() {
  const search = state.modelSearch
  const query = String(
    document.querySelector("#modelSearchQuery")?.value || search.query || "",
  ).trim()
  if (!query) return
  search.query = query
  search.results = []
  search.repo = ""
  search.files = []
  search.error = ""
  search.loading = true
  render()
  try {
    search.results = (await SearchModels(search.source, query)) || []
  } catch (error) {
    search.error = String(error?.message || error)
  }
  search.loading = false
  render()
}

async function loadModelFiles(repo) {
  const search = state.modelSearch
  if (!repo) return
  search.repo = repo
  search.repoName = repo
  search.files = []
  search.error = ""
  search.filesLoading = true
  render()
  try {
    search.files = (await ListModelFiles(search.source, repo)) || []
  } catch (error) {
    search.error = String(error?.message || error)
  }
  search.filesLoading = false
  render()
}

async function startModelFileDownload(file, size) {
  const search = state.modelSearch
  if (!search.repo || !file) return
  if (state.modelDownload?.status === "downloading") {
    search.error = "已有模型正在下载，请等待完成或先取消"
    render()
    return
  }
  state.modelSearch.error = ""
  state.modelDownload = {
    status: "downloading",
    source: search.source,
    repo: search.repo,
    file,
    name: `${search.repo} · ${file}`,
    downloaded: 0,
    total: size,
    percent: 0,
    speed: 0,
    error: "",
  }
  render()
  try {
    await StartModelDownload(search.source, search.repo, file, search.repo)
  } catch (error) {
    state.modelDownload.status = "error"
    state.modelDownload.error = String(error?.message || error)
    render()
  }
}

async function downloadConfiguredModel(id) {
  const entry = (state.config.models || []).find((item) => item.id === id)
  if (!entry) return
  if (state.modelDownload?.status === "downloading") {
    state.modelSearch.error = "已有模型正在下载，请等待完成或先取消"
    render()
    return
  }
  state.modelDownload = {
    status: "downloading",
    source: entry.source,
    repo: entry.repo,
    file: entry.file,
    name: entry.name || entry.repo,
    downloaded: 0,
    total: Number(entry.size) || 0,
    percent: 0,
    speed: 0,
    error: "",
  }
  render()
  try {
    await StartModelDownload(
      entry.source,
      entry.repo,
      entry.file,
      entry.name || entry.repo,
    )
  } catch (error) {
    state.modelDownload.status = "error"
    state.modelDownload.error = String(error?.message || error)
    render()
  }
}

async function useModel(id) {
  if (!id) return
  state.config.active_model_id = id
  await persistModelCatalogue()
  render()
}

async function deleteConfiguredModel(id) {
  const entry = (state.config.models || []).find((item) => item.id === id)
  if (!entry) return
  try {
    await DeleteModelFile(entry.repo, entry.file)
  } catch (error) {
    state.modelSearch.error = String(error?.message || error)
  }
  await refreshModelStatuses()
  render()
}

function updateModelDownloadUI() {
  const download = state.modelDownload || {}
  const box = document.querySelector("#modelDownloadBox")
  if (!box) {
    render()
    return
  }
  const label = box.querySelector("#modelDownloadLabel")
  if (label) {
    label.textContent = `正在下载 ${download.name || download.file || ""}`
  }
  const fill = box.querySelector("#modelDownloadFill")
  if (fill) {
    fill.style.width = `${Math.max(0, Math.min(100, Number(download.percent) || 0))}%`
  }
  const meta = box.querySelector("#modelDownloadMeta")
  if (meta) {
    meta.textContent = `${formatBytes(download.downloaded || 0)} / ${
      download.total ? formatBytes(download.total) : "?"
    } · ${formatBytes(download.speed || 0)}/s`
  }
}

async function ensureDownloadedModel(payload) {
  const models = Array.isArray(state.config.models)
    ? [...state.config.models]
    : []
  const exists = models.some(
    (entry) => entry.repo === payload.repo && entry.file === payload.file,
  )
  if (!exists) {
    models.push({
      id: "",
      name:
        payload.name && payload.name !== payload.repo
          ? payload.name
          : payload.repo,
      source: payload.source || "modelscope",
      repo: payload.repo,
      file: payload.file,
      size: Number(payload.total) || 0,
      dir: "",
    })
    state.config.models = models
    await persistModelCatalogue()
  } else {
    await refreshModelStatuses()
  }
  state.modelDownload = { ...state.modelDownload, status: "done", percent: 100 }
}

function renderSettingsFooterHTML() {
  return `
    <footer class="settings-footer">
      <span id="saveStatus" class="action-status" aria-live="polite">保存后立即应用设置</span>
      <div class="settings-footer-actions">
        <button id="cancelSettingsBtn" class="secondary-button" type="button">
          <span>返回</span>
        </button>
        <button id="saveBtn" class="primary-button" type="button">
          <span>保存</span>
        </button>
      </div>
    </footer>
  `
}

function bindSettingsViewEvents() {
  bindHotkeyInput(document.querySelector("#hotkeyDisplay"))
  bindHotkeyInput(document.querySelector("#ocrHotkeyDisplay"))
  bindHotkeyInput(document.querySelector("#liveHotkeyDisplay"))
  document
    .querySelectorAll('input[name="theme"]')
    .forEach((input) => input.addEventListener("change", previewTheme))
  document
    .querySelector("#ocrEnabled")
    ?.addEventListener("change", toggleOcrField)
  document
    .querySelector("#liveEnabled")
    ?.addEventListener("change", toggleLiveField)
  document
    .querySelector("#aiKeepAlive")
    ?.addEventListener("change", toggleKeepAliveField)
  document
    .querySelectorAll('input[name="model_download_source"]')
    .forEach((input) =>
      input.addEventListener("change", toggleCustomSourceField),
    )
  bindOverlayStyleControls()
  document.querySelectorAll("[data-settings-tab]").forEach((button) => {
    button.addEventListener("click", () => {
      const next = button.dataset.settingsTab
      state.settingsTab =
        next === "glossary" ? "glossary" : next === "models" ? "models" : "general"
      if (state.settingsTab === "models" && window.go?.wailsui?.App?.ModelStatuses) {
        state.modelStatusesLoaded = false
      }
      render()
    })
  })
  bindGlossaryPageControls()
  bindModelsPageControls()
  document.querySelector("#cancelSettingsBtn")?.addEventListener("click", () => {
    setHotkeyRecording(false)
    state.active_tab = "translate"
    state.glossaryPrepared = false
    render()
  })
  document.querySelector("#saveBtn")?.addEventListener("click", saveSettings)
}

function setHotkeyRecording(recording) {
  try {
    if (window.go?.wailsui?.App?.SetHotkeyRecording) {
      SetHotkeyRecording(recording).catch(() => {})
    }
  } catch {
    // ignore
  }
}

function toggleLiveField(event) {
  const enabled = event.currentTarget.checked
  const field = document.querySelector("#liveHotkeyField")
  const input = document.querySelector("#liveHotkeyDisplay")
  field?.classList.toggle("is-disabled", !enabled)
  if (input) input.disabled = !enabled
}

function toggleKeepAliveField(event) {
  const keepAlive = event.currentTarget.checked
  const field = document.querySelector("#aiIdleField")
  const input = document.querySelector("#aiIdleMinutes")
  field?.classList.toggle("is-disabled", keepAlive)
  if (input) input.disabled = keepAlive
}

function prepareGlossaryDraft() {
  state.glossaryDraft = (state.config.glossary || []).map((entry) => ({
    term: entry.term || "",
    translation: entry.translation || "",
    source: entry.source || "",
    note: entry.note || "",
  }))
  state.glossaryCategory = "all"
  state.glossarySearch = ""
  state.glossaryExtraCategories = []
  state.settingsTab = "general"
}

function collectGlossaryDraft() {
  return (state.glossaryDraft || [])
    .map((entry) => ({
      term: String(entry.term || "").trim(),
      translation: String(entry.translation || "").trim(),
      source: String(entry.source || "").trim(),
      note: String(entry.note || "").trim(),
    }))
    .filter((entry) => entry.term && entry.translation)
}

function glossaryCategories() {
  const sources = []
  const seen = new Set()
  const add = (source) => {
    const value = String(source || "").trim()
    if (!value || seen.has(value)) return
    seen.add(value)
    sources.push(value)
  }
  for (const source of state.glossaryExtraCategories || []) add(source)
  for (const entry of state.glossaryDraft || []) add(entry.source)
  return sources
}

function glossaryFilteredEntries() {
  const category = state.glossaryCategory || "all"
  const query = String(state.glossarySearch || "").trim().toLowerCase()
  return (state.glossaryDraft || [])
    .map((entry, index) => ({ entry, index }))
    .filter(({ entry }) => {
      if (
        category !== "all" &&
        String(entry.source || "").trim() !== category
      ) {
        return false
      }
      if (!query) return true
      return [entry.term, entry.translation, entry.source, entry.note]
        .map((value) => String(value || "").toLowerCase())
        .some((value) => value.includes(query))
    })
}

function glossaryPackEntries(pack) {
  return pack.entries.map(([term, translation]) => ({
    term,
    translation,
    source: pack.name,
    note: "",
  }))
}

function glossaryPackEnabled(pack) {
  const terms = new Set(
    (state.glossaryDraft || [])
      .map((entry) => String(entry.term || "").trim().toLowerCase())
      .filter(Boolean),
  )
  return pack.entries.every(([term]) => terms.has(String(term).toLowerCase()))
}

function toggleGlossaryPack(id, enabled) {
  const pack = GLOSSARY_PACKS.find((item) => item.id === id)
  if (!pack) return
  if (!Array.isArray(state.glossaryDraft)) state.glossaryDraft = []
  if (enabled) {
    const existing = new Set(
      state.glossaryDraft
        .map((entry) => String(entry.term || "").trim().toLowerCase())
        .filter(Boolean),
    )
    for (const entry of glossaryPackEntries(pack)) {
      const key = entry.term.toLowerCase()
      if (existing.has(key)) continue
      state.glossaryDraft.push(entry)
      existing.add(key)
    }
  } else {
    state.glossaryDraft = state.glossaryDraft.filter(
      (entry) => String(entry.source || "").trim() !== pack.name,
    )
  }
  render()
}

function renderGlossaryPage() {
  if (!Array.isArray(state.glossaryDraft)) state.glossaryDraft = []
  const categories = glossaryCategories()
  const category = state.glossaryCategory || "all"
  const counts = new Map()
  for (const entry of state.glossaryDraft) {
    const source = String(entry.source || "").trim()
    if (source) counts.set(source, (counts.get(source) || 0) + 1)
  }
  const filtered = glossaryFilteredEntries()
  const categoryButton = (value, label, count) => `
    <button class="glossary-category ${category === value ? "is-active" : ""}"
      data-glossary-category="${escapeHtml(value)}" type="button">
      <span class="glossary-category-name">${escapeHtml(label)}</span>
      <span class="glossary-count">${count}</span>
    </button>
  `
  return `
    <div class="glossary-page">
      <aside class="glossary-sidebar">
        <div class="glossary-sidebar-title">来源分类</div>
        ${categoryButton("all", "全部词汇", state.glossaryDraft.length)}
        ${categories
          .map((source) => categoryButton(source, source, counts.get(source) || 0))
          .join("")}
        <div class="glossary-new-category">
          <input class="text-input" id="glossaryNewCategory" type="text" placeholder="新建来源" />
          <button class="icon-button" id="glossaryAddCategory" type="button" title="新建来源">+</button>
        </div>
      </aside>
      <section class="glossary-main">
        <div class="glossary-packs">
          <div class="glossary-packs-head">
            <span>内置词汇包</span>
            <span class="glossary-packs-hint">勾选启用，保存后生效</span>
          </div>
          <div class="glossary-pack-list">
            ${GLOSSARY_PACKS.map(
              (pack) => `
              <label class="glossary-pack" title="${escapeHtml(pack.description)}">
                <input type="checkbox" data-glossary-pack="${escapeHtml(pack.id)}"
                  ${glossaryPackEnabled(pack) ? "checked" : ""} />
                <span class="glossary-pack-name">${escapeHtml(pack.name)}</span>
                <span class="glossary-pack-count">${pack.entries.length} 条</span>
              </label>
            `,
            ).join("")}
          </div>
        </div>
        <div class="glossary-toolbar">
          <input class="text-input" id="glossarySearch" type="text"
            placeholder="搜索词汇或译文" value="${escapeHtml(state.glossarySearch || "")}" />
          <button class="primary-button glossary-add-entry" id="glossaryAddEntry" type="button">+ 添加词条</button>
        </div>
        <div class="glossary-entries" id="glossaryEntries">
          ${
            filtered.length
              ? filtered
                  .map(({ entry, index }) =>
                    renderGlossaryEntry(entry, index, categories),
                  )
                  .join("")
              : `<div class="glossary-empty">${
                  category === "all"
                    ? "还没有词条，点击「添加词条」开始"
                    : "该来源下还没有词条"
                }</div>`
          }
        </div>
        <p class="field-help glossary-hint">
          翻译时优先使用词条释义；命中处会显示紫色下划线，悬停可查看来源。
        </p>
      </section>
    </div>
  `
}

function renderGlossaryEntry(entry, index, categories) {
  const sourceOptions = [
    `<option value="">未分类</option>`,
    ...categories.map(
      (source) =>
        `<option value="${escapeHtml(source)}" ${
          source === entry.source ? "selected" : ""
        }>${escapeHtml(source)}</option>`,
    ),
  ].join("")
  return `
    <div class="glossary-entry" data-glossary-index="${index}">
      <div class="glossary-entry-row">
        <input class="text-input glossary-term" type="text" placeholder="原文词汇"
          value="${escapeHtml(entry.term || "")}" />
        <input class="text-input glossary-translation" type="text" placeholder="指定译文"
          value="${escapeHtml(entry.translation || "")}" />
      </div>
      <div class="glossary-entry-row">
        <select class="select-input glossary-source">${sourceOptions}</select>
        <input class="text-input glossary-note" type="text" placeholder="备注（可选）"
          value="${escapeHtml(entry.note || "")}" />
        <button class="icon-button glossary-remove" type="button" title="删除词条">
          <i data-lucide="x"></i>
        </button>
      </div>
    </div>
  `
}

function bindGlossaryEntryControls() {
  const draft = state.glossaryDraft || []
  document.querySelectorAll(".glossary-entry").forEach((row) => {
    const index = Number(row.dataset.glossaryIndex)
    if (!Number.isInteger(index) || !draft[index]) return
    row
      .querySelector(".glossary-term")
      ?.addEventListener("input", (event) => {
        draft[index].term = event.target.value
      })
    row
      .querySelector(".glossary-translation")
      ?.addEventListener("input", (event) => {
        draft[index].translation = event.target.value
      })
    row
      .querySelector(".glossary-note")
      ?.addEventListener("input", (event) => {
        draft[index].note = event.target.value
      })
    const sourceSelect = row.querySelector(".glossary-source")
    sourceSelect?.addEventListener("change", () => {
      draft[index].source = sourceSelect.value
    })
    row.querySelector(".glossary-remove")?.addEventListener("click", () => {
      draft.splice(index, 1)
      render()
    })
  })
}

function bindGlossaryPageControls() {
  if (!document.querySelector(".glossary-page")) return
  document.querySelectorAll("[data-glossary-category]").forEach((button) => {
    button.addEventListener("click", () => {
      state.glossaryCategory = button.dataset.glossaryCategory || "all"
      render()
    })
  })

  document.querySelectorAll("[data-glossary-pack]").forEach((input) => {
    input.addEventListener("change", () => {
      toggleGlossaryPack(input.dataset.glossaryPack, input.checked)
    })
  })

  const search = document.querySelector("#glossarySearch")
  search?.addEventListener("input", () => {
    state.glossarySearch = search.value
    const list = document.querySelector("#glossaryEntries")
    if (!list) return
    const filtered = glossaryFilteredEntries()
    list.innerHTML = filtered.length
      ? filtered
          .map(({ entry, index }) =>
            renderGlossaryEntry(entry, index, glossaryCategories()),
          )
          .join("")
      : `<div class="glossary-empty">没有匹配的词条</div>`
    createIcons({ icons, attrs: { "stroke-width": 1.8 } })
    bindGlossaryEntryControls()
  })

  document.querySelector("#glossaryAddEntry")?.addEventListener("click", () => {
    const source =
      state.glossaryCategory === "all" ? "" : state.glossaryCategory
    state.glossaryDraft.push({
      term: "",
      translation: "",
      source,
      note: "",
    })
    state.glossarySearch = ""
    render()
    const rows = document.querySelectorAll(".glossary-entry")
    rows[rows.length - 1]?.querySelector(".glossary-term")?.focus()
  })

  document
    .querySelector("#glossaryAddCategory")
    ?.addEventListener("click", () => {
      const input = document.querySelector("#glossaryNewCategory")
      const value = input?.value.trim()
      if (!value) return
      if (!state.glossaryExtraCategories.includes(value)) {
        state.glossaryExtraCategories.push(value)
      }
      state.glossaryCategory = value
      render()
    })

  bindGlossaryEntryControls()
}

function bindOverlayStyleControls() {
  const fontFamily = document.querySelector("#overlayFontFamily")
  const fontWeight = document.querySelector("#overlayFontWeight")
  const weightLabel = document.querySelector("#overlayFontWeightValue")
  const preview = document.querySelector("#overlayStylePreview")
  if (!fontFamily && !fontWeight) return

  const update = () => {
    if (weightLabel && fontWeight) {
      weightLabel.textContent = overlayWeightLabel(fontWeight.value)
    }
    if (preview) {
      preview.style.color = "#FFFFFF"
      preview.style.textShadow = "0 1px 2px rgba(0,0,0,0.65)"
      preview.style.fontFamily = overlayFontStack(fontFamily?.value)
      preview.style.fontWeight = String(
        clampNumber(fontWeight?.value, 300, 800, 500),
      )
    }
  }
  ;[fontFamily, fontWeight]
    .filter(Boolean)
    .forEach((input) => input.addEventListener("input", update))
  fontFamily?.addEventListener("change", update)
  update()
}

function toggleCustomSourceField() {
  const selected = document.querySelector(
    'input[name="model_download_source"]:checked',
  )?.value
  const field = document.querySelector("#customSourceField")
  const input = document.querySelector("#modelDownloadBaseUrl")
  const custom = selected === "custom"
  field?.classList.toggle("is-disabled", !custom)
  if (input) input.disabled = !custom
}

function themeOption(value, icon, label) {
  const selected = (state.config.theme || "system") === value
  return `
    <label class="theme-option">
      <input type="radio" name="theme" value="${value}" ${selected ? "checked" : ""} />
      <span class="theme-option-content">
        <i data-lucide="${icon}"></i>
        <span>${label}</span>
      </span>
    </label>
  `
}

function sourceOption(value, icon, label) {
  const selected =
    (state.config.model_download_source || "modelscope") === value
  return `
    <label class="theme-option">
      <input type="radio" name="model_download_source" value="${value}" ${selected ? "checked" : ""} />
      <span class="theme-option-content">
        <i data-lucide="${icon}"></i>
        <span>${label}</span>
      </span>
    </label>
  `
}

function deviceOption(value, icon, label) {
  const selected = (state.config.acceleration_device || "auto") === value
  return `
    <label class="theme-option">
      <input type="radio" name="acceleration_device" value="${value}" ${selected ? "checked" : ""} />
      <span class="theme-option-content">
        <i data-lucide="${icon}"></i>
        <span>${label}</span>
      </span>
    </label>
  `
}

function renderCurrentDeviceBadge() {
  const info = state.health?.device_info
  if (!info) return ""
  const actual = String(info.actual || "cpu")
  const isGpu = actual.toLowerCase().includes("gpu")
  const label = isGpu ? "当前已生效：显卡 (GPU) 加速" : "当前运行模式：纯 CPU 推理"
  return `
    <div class="device-status-badge ${isGpu ? "is-gpu" : "is-cpu"}">
      <i data-lucide="${isGpu ? "zap" : "cpu"}"></i>
      <span>${label}</span>
    </div>
  `
}

function previewTheme(event) {
  applyTheme(event.currentTarget.value)
}

function applyTheme(theme = "system") {
  const selected = ["light", "dark", "system"].includes(theme)
    ? theme
    : "system"
  activeTheme = selected
  const resolved =
    selected === "system" ? (systemTheme.matches ? "dark" : "light") : selected

  document.documentElement.dataset.theme = resolved
  document.documentElement.style.colorScheme = resolved

  try {
    localStorage.setItem("poptrans_theme", selected)
  } catch {
    // ignore
  }

  if (!window.runtime) return
  if (selected === "light") WindowSetLightTheme()
  else if (selected === "dark") WindowSetDarkTheme()
  else WindowSetSystemDefaultTheme()
}

function bindHotkeyInput(input) {
  if (!input) return
  input.addEventListener("focus", () => {
    setHotkeyRecording(true)
    input.dataset.previous = input.value
    input.value = "请按下组合键"
    input.classList.add("is-recording")
  })

  input.addEventListener("blur", () => {
    setHotkeyRecording(false)
    if (input.classList.contains("is-recording"))
      input.value = input.dataset.previous || ""
    input.classList.remove("is-recording")
  })

  input.addEventListener("keydown", (event) => {
    event.preventDefault()
    event.stopPropagation()
    if (event.key === "Escape") {
      input.value = input.dataset.previous || ""
      input.blur()
      return
    }
    if (["Control", "Alt", "Shift", "Meta"].includes(event.key)) return

    const parts = []
    if (event.ctrlKey) parts.push("Ctrl")
    if (event.altKey) parts.push("Alt")
    if (event.shiftKey) parts.push("Shift")
    if (event.metaKey) parts.push("Win")
    const key = normalizeKey(event.key)
    if (!parts.length || !key) return
    parts.push(key)
    input.value = parts.join("+")
    input.dataset.previous = input.value
    input.classList.remove("is-recording")
    input.blur()
  })
}

function toggleOcrField(event) {
  const enabled = event.currentTarget.checked
  const field = document.querySelector("#ocrHotkeyField")
  const input = document.querySelector("#ocrHotkeyDisplay")
  field?.classList.toggle("is-disabled", !enabled)
  if (input) input.disabled = !enabled
}

async function saveSettings() {
  const status = document.querySelector("#saveStatus")
  const button = document.querySelector("#saveBtn")

  if (state.settingsTab === "models" && state.active_tab === "settings") {
    if (status) showStatus(status, "模型设置会自动保存", "")
    return
  }

  if (state.settingsTab === "glossary" && state.active_tab === "settings") {
    const next = { ...state.config, glossary: collectGlossaryDraft() }
    if (button) button.disabled = true
    if (status) showStatus(status, "正在保存...", "pending")
    try {
      await SaveConfig(next)
      state.config = next
      if (status) {
        showStatus(status, "词汇本已保存", "success")
        setTimeout(() => {
          if (status.textContent === "词汇本已保存") {
            showStatus(status, "保存后立即应用设置", "")
          }
        }, 2500)
      }
    } catch (error) {
      if (status) showStatus(status, readableError(error), "error")
    } finally {
      if (button) button.disabled = false
    }
    return
  }

  const hotkeyDisplay =
    document.querySelector("#hotkeyDisplay")?.value.trim() || ""
  const ocrHotkeyDisplay =
    document.querySelector("#ocrHotkeyDisplay")?.value.trim() || ""
  const liveHotkeyDisplay =
    document.querySelector("#liveHotkeyDisplay")?.value.trim() || ""
  const serverPort = Number(document.querySelector("#serverPort")?.value)
  const aiIdleMinutes = Number(document.querySelector("#aiIdleMinutes")?.value)
  const aiKeepAlive = Boolean(document.querySelector("#aiKeepAlive")?.checked)
  const aiThreads = Number(document.querySelector("#aiThreads")?.value)
  const downloadSource =
    document.querySelector('input[name="model_download_source"]:checked')
      ?.value || "modelscope"
  const downloadBaseUrl =
    document.querySelector("#modelDownloadBaseUrl")?.value.trim() || ""
  const sourceLanguage =
    document.querySelector("#sourceLanguage")?.value || "auto"
  const targetLanguage =
    document.querySelector("#targetLanguage")?.value || "auto"
  const overlayFontFamily =
    document.querySelector("#overlayFontFamily")?.value || "system"
  const overlayFontWeight = clampNumber(
    document.querySelector("#overlayFontWeight")?.value,
    300,
    800,
    500,
  )
  const glossary = collectGlossaryDraft()

  if (
    !isValidHotkey(hotkeyDisplay) ||
    !isValidHotkey(ocrHotkeyDisplay) ||
    !isValidHotkey(liveHotkeyDisplay)
  ) {
    if (status) showStatus(status, "快捷键需要包含修饰键和普通按键", "error")
    return
  }
  if (downloadSource === "custom") {
    let validUrl = false
    try {
      const parsed = new URL(downloadBaseUrl)
      validUrl = parsed.protocol === "http:" || parsed.protocol === "https:"
    } catch {
      validUrl = false
    }
    if (!validUrl) {
      if (status)
        showStatus(status, "自定义下载地址需以 http:// 或 https:// 开头", "error")
      document.querySelector("#modelDownloadBaseUrl")?.focus()
      return
    }
  }
  if (
    !Number.isInteger(serverPort) ||
    serverPort < 1024 ||
    serverPort > 65535
  ) {
    if (status)
      showStatus(status, "端口必须是 1024 到 65535 之间的整数", "error")
    document.querySelector("#serverPort")?.focus()
    return
  }
  if (!Number.isInteger(aiIdleMinutes) || aiIdleMinutes < 0) {
    if (status)
      showStatus(status, "AI 空闲时间必须是大于等于 0 的整数", "error")
    document.querySelector("#aiIdleMinutes")?.focus()
    return
  }
  if (!Number.isInteger(aiThreads) || aiThreads < 0 || aiThreads > 256) {
    if (status) showStatus(status, "CPU 线程数必须是 0 到 256 的整数", "error")
    document.querySelector("#aiThreads")?.focus()
    return
  }

  const next = {
    ...state.config,
    hotkey: displayHotkeyToExpr(hotkeyDisplay),
    hotkey_display: normalizeDisplay(hotkeyDisplay),
    ocr_enabled: Boolean(document.querySelector("#ocrEnabled")?.checked),
    ocr_hotkey: displayHotkeyToExpr(ocrHotkeyDisplay),
    ocr_hotkey_display: normalizeDisplay(ocrHotkeyDisplay),
    live_enabled: Boolean(document.querySelector("#liveEnabled")?.checked),
    live_hotkey: displayHotkeyToExpr(liveHotkeyDisplay),
    live_hotkey_display: normalizeDisplay(liveHotkeyDisplay),
    logging_enabled: Boolean(
      document.querySelector("#loggingEnabled")?.checked,
    ),
    launch_at_login: Boolean(
      document.querySelector("#launchAtLogin")?.checked,
    ),
    server_port: serverPort,
    ai_keep_alive: aiKeepAlive,
    ai_threads: aiThreads,
    ai_idle_minutes: aiIdleMinutes,
    model_download_source: downloadSource,
    model_download_base_url: downloadBaseUrl,
    source_language: sourceLanguage,
    target_language: targetLanguage,
    overlay_font_family: overlayFontFamily,
    overlay_font_weight: overlayFontWeight,
    glossary,
    acceleration_device:
      document.querySelector('input[name="acceleration_device"]:checked')?.value || "auto",
    theme:
      document.querySelector('input[name="theme"]:checked')?.value || "system",
  }

  if (button) button.disabled = true
  if (status) showStatus(status, "正在保存...", "pending")
  setHotkeyRecording(false)
  try {
    await SaveConfig(next)
    state.config = next
    if (status) {
      showStatus(status, "设置已保存", "success")
      setTimeout(() => {
        if (status.textContent === "设置已保存") {
          showStatus(status, "保存后立即应用设置", "")
        }
      }, 2500)
    }
  } catch (error) {
    if (status) showStatus(status, readableError(error), "error")
  } finally {
    if (button) button.disabled = false
  }
}

function parseProgress(status) {
  if (!status) return null
  const match = status.match(/(\d+(?:\.\d+)?)%/)
  if (match) {
    const val = parseFloat(match[1])
    if (!isNaN(val) && val >= 0 && val <= 100) return val
  }
  return null
}

function isDownloadingModel(status) {
  if (!status || typeof status !== "string") return false
  if (status.includes("下载完成")) return false
  return (
    status.includes("下载") ||
    status.includes("官网") ||
    parseProgress(status) !== null
  )
}

function renderStartup() {
  const isMain =
    (state.mode === "settings" || state.mode === "translate") &&
    state.settings_open
  if (window.runtime?.WindowSetSize && !isMain && !startupWindowSized) {
    WindowSetSize(360, 220)
    if (window.runtime?.WindowCenter) WindowCenter()
    startupWindowSized = true
  }
  const status = state.startup_status || "正在加载翻译模型..."
  const error = state.startup_error
  const progress = parseProgress(status)
  const isDownloading = isDownloadingModel(status)

  app.innerHTML = `
    <main class="startup-shell" role="status" aria-live="polite">
      <header class="startup-header" style="--wails-draggable: drag;">
        <div class="startup-brand">
          <div class="brand-mark" aria-hidden="true"><i data-lucide="languages"></i></div>
          <span class="startup-title">${isDownloading ? "PopTrans · 模型准备" : "PopTrans · 正在启动"}</span>
        </div>
        <button class="startup-close-btn" id="startupDismissBtn" type="button" title="${isDownloading ? "最小化到托盘后台下载" : "最小化到托盘"}" aria-label="${isDownloading ? "最小化到托盘后台下载" : "最小化到托盘"}">
          <i data-lucide="x"></i>
        </button>
      </header>
      <div class="startup-body">
        <p class="startup-status">${escapeHtml(status)}</p>
        ${error ? `<p class="startup-error">${escapeHtml(error)}</p>` : ""}
        <div class="startup-progress" aria-hidden="true">
          <span ${progress !== null ? `style="width: ${progress}%; animation: none;"` : ""}></span>
        </div>
        ${
          isDownloading
            ? `
        <div class="startup-actions">
          <button class="startup-bg-btn" id="startupBgBtn" type="button" title="转入后台静默下载">
            <i data-lucide="sparkles"></i>
            <span>转入后台静默下载</span>
          </button>
          <p class="startup-hint">首次启动需下载模型，可隐藏窗口后台运行</p>
        </div>
        `
            : ""
        }
      </div>
    </main>
  `

  document
    .querySelector("#startupDismissBtn")
    ?.addEventListener("click", dismissStartup)
  document
    .querySelector("#startupBgBtn")
    ?.addEventListener("click", dismissStartup)
  createIcons({ icons, attrs: { "stroke-width": 1.8 } })
}

function dismissStartup() {
  startupDismissed = true
  windowShown = false
  try {
    if (window.go?.wailsui?.App?.HideWindow) {
      HideWindow()
      return
    }
  } catch {
    // fall through
  }
  try {
    window.close()
  } catch {
    // ignore
  }
}

async function refreshHealth() {
  if (!window.go?.wailsui?.App) return
  try {
    const next = await State()
    state.health = next.health
    const engine = engineStatusInfo()
    if (engine.state === "ready") clearInterval(healthTimer)
    const node = document.querySelector("#engineState")
    const text = document.querySelector("#engineText")
    if (!node || !text) return
    node.classList.toggle("is-ready", engine.state === "ready")
    node.classList.toggle("is-loading", engine.state === "loading")
    node.classList.toggle("is-error", engine.state === "error")
    node.title = engine.detail || "AI 引擎状态"
    text.textContent = engine.label

    const banner = document.querySelector("#engineBanner")
    if (banner) {
      banner.style.display = engine.state === "ready" ? "none" : "flex"
      banner.classList.toggle("is-error", engine.state === "error")
      const bannerText = document.querySelector("#engineBannerText")
      if (bannerText) bannerText.textContent = engine.detail || engine.label
    }
  } catch {
    // The next interval retries
  }
}

async function copyResult(
  text,
  statusSelector = "#copyStatus",
  btnSelector = "#copyBtn",
) {
  const status = document.querySelector(statusSelector)
  const button = document.querySelector(btnSelector)
  try {
    await navigator.clipboard.writeText(text || "")
    if (status) {
      status.textContent = "已复制"
      status.className = "result-copy-status success"
    }
    if (button) {
      button.innerHTML = '<i data-lucide="check"></i>'
      button.classList.add("is-success")
      button.title = "已复制"
      createIcons({ icons, attrs: { "stroke-width": 1.8 } })
      setTimeout(() => {
        if (button.classList.contains("is-success")) {
          button.classList.remove("is-success")
          button.innerHTML = '<i data-lucide="clipboard"></i>'
          button.title = "复制译文"
          createIcons({ icons, attrs: { "stroke-width": 1.8 } })
        }
        if (status) {
          status.textContent = ""
          status.className = "result-copy-status"
        }
      }, 1800)
    }
  } catch {
    if (status) {
      status.textContent = "复制失败"
      status.className = "result-copy-status error"
    }
  }
}

function closeWindow() {
  if (state.startup_loading) {
    startupDismissed = true
  }
  setHotkeyRecording(false)
  resultShowTimestamp = 0
  windowShown = false
  lastResultHeight = 0
  resultSignature = ""
  state.manualTranslate = {
    sourceText: "",
    translatedText: "",
    isLoading: false,
    error: "",
    loadingText: "",
    latencyMs: 0,
  }
  state.overlay = null
  state.overlayOriginal = false
  state.overlayBlockOriginal = new Set()
  state.live = null
  state.liveOriginal = false
  state.liveBlockOriginal = new Set()
  state.glossaryPrepared = false
  state.settingsTab = "general"
  if (
    document.querySelector(".result-shell") ||
    document.querySelector(".shot-overlay") ||
    document.querySelector(".live-panel")
  ) {
    app.innerHTML = ""
  }
  try {
    if (window.go?.wailsui?.App?.HideWindow) {
      HideWindow()
      return
    }
  } catch {
    // fall through
  }
  try {
    window.close()
  } catch {
    // ignore
  }
}

function handleWindowKeydown(event) {
  if (event.key === "Escape" && !event.repeat) {
    if (state.startup_loading) {
      event.preventDefault()
      dismissStartup()
      return
    }
    if (
      state.mode === "result" ||
      state.mode === "overlay" ||
      state.mode === "live"
    ) {
      event.preventDefault()
      closeWindow()
      return
    }
  }

  if (state.mode === "overlay" && !event.repeat) {
    if (!event.ctrlKey && !event.metaKey && event.key.toLowerCase() === "h") {
      event.preventDefault()
      state.overlayOriginal = !state.overlayOriginal
      renderOverlay()
      return
    }
    const selection = window.getSelection?.()?.toString() || ""
    if (
      (event.ctrlKey || event.metaKey) &&
      event.key.toLowerCase() === "c" &&
      !selection
    ) {
      event.preventDefault()
      const overlay = state.overlay || {}
      const text = state.overlayOriginal
        ? overlay.source || ""
        : overlay.translation || translatedJoin(overlay.blocks)
      copyShotText(text, "已复制")
    }
  }

  if (state.mode === "live" && !event.repeat) {
    if (!event.ctrlKey && !event.metaKey && event.key.toLowerCase() === "h") {
      event.preventDefault()
      state.liveOriginal = !state.liveOriginal
      renderLive()
      return
    }
    const selection = window.getSelection?.()?.toString() || ""
    if (
      (event.ctrlKey || event.metaKey) &&
      event.key.toLowerCase() === "c" &&
      !selection
    ) {
      event.preventDefault()
      const live = state.live || {}
      const text = state.liveOriginal
        ? live.source || ""
        : live.translation || translatedJoin(live.blocks)
      copyShotText(text, "已复制")
    }
  }
}

function fitResultWindow() {
  if (!window.runtime?.WindowSetSize) return

  const header = document.querySelector(".result-header")
  const content = document.querySelector(".result-content")
  const shell = document.querySelector(".result-shell")
  if (!header || !content || !shell) return

  const contentStyle = getComputedStyle(content)
  const contentPadding =
    parseFloat(contentStyle.paddingTop) + parseFloat(contentStyle.paddingBottom)
  const shellStyle = getComputedStyle(shell)
  const shellSpacing =
    parseFloat(shellStyle.marginTop) +
    parseFloat(shellStyle.marginBottom) +
    parseFloat(shellStyle.borderTopWidth) +
    parseFloat(shellStyle.borderBottomWidth)
  const childrenHeight = Array.from(content.children).reduce((total, child) => {
    const style = getComputedStyle(child)
    return (
      total +
      child.scrollHeight +
      parseFloat(style.marginTop) +
      parseFloat(style.marginBottom)
    )
  }, 0)
  const contentHeight = Math.max(
    180,
    Math.min(
      440,
      Math.ceil(
        shellSpacing +
          header.offsetHeight +
          contentPadding +
          childrenHeight +
          2,
      ),
    ),
  )

  if (Math.abs(contentHeight - lastResultHeight) < 2) return
  lastResultHeight = contentHeight
  WindowSetSize(425, contentHeight)
}

function showStatus(node, message, type) {
  node.textContent = message
  node.className = `action-status ${type}`
}

function isValidHotkey(value) {
  const parts = value
    .split("+")
    .map((part) => part.trim())
    .filter(Boolean)
  return (
    parts.length >= 2 &&
    parts.slice(0, -1).some((part) => /^(ctrl|alt|shift|win)$/i.test(part))
  )
}

function displayHotkeyToExpr(value) {
  return normalizeDisplay(value)
    .split("+")
    .map((part) => {
      const p = part.toLowerCase()
      if (p === "ctrl") return "<ctrl>"
      if (p === "alt") return "<alt>"
      if (p === "shift") return "<shift>"
      if (p === "win") return "<cmd>"
      return p
    })
    .join("+")
}

function normalizeDisplay(value) {
  return value
    .split("+")
    .map((part) => {
      const p = part.trim().toLowerCase()
      if (p === "ctrl" || p === "control") return "Ctrl"
      if (p === "alt") return "Alt"
      if (p === "shift") return "Shift"
      if (p === "win" || p === "cmd" || p === "meta") return "Win"
      return p.length === 1 ? p.toUpperCase() : p[0].toUpperCase() + p.slice(1)
    })
    .join("+")
}

function normalizeKey(key) {
  if (key === " ") return "Space"
  if (key === "+") return "Plus"
  return key.length === 1
    ? key.toUpperCase()
    : key[0].toUpperCase() + key.slice(1)
}

function readableError(error) {
  return String(error?.message || error || "未知错误").replace(/^Error:\s*/, "")
}

function escapeHtml(value) {
  return String(value).replace(
    /[&<>"']/g,
    (char) =>
      ({
        "&": "&amp;",
        "<": "&lt;",
        ">": "&gt;",
        '"': "&quot;",
        "'": "&#039;",
      })[char],
  )
}

window.addEventListener("keydown", handleWindowKeydown)
window.addEventListener("resize", () => {
  if (state.mode === "overlay") renderOverlay()
  else if (state.mode === "live") renderLive()
})
window.addEventListener("paste", handleImagePaste)
window.addEventListener("dragover", (event) => {
  if (event.dataTransfer?.types?.includes("Files")) event.preventDefault()
})
window.addEventListener("drop", handleImageDrop)
systemTheme.addEventListener("change", () => {
  if (activeTheme === "system") applyTheme("system")
})
let overlayRegionResizeTimer = 0
window.addEventListener("resize", () => {
  if (state.mode !== "overlay" && state.mode !== "live") return
  clearTimeout(overlayRegionResizeTimer)
  overlayRegionResizeTimer = setTimeout(syncOverlayRegion, 80)
})
bindStateEvents()
loadState()
