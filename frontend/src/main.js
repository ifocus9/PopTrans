import "./styles.css"
import { GLOSSARY_PACKS } from "./glossary_packs"
import {
  createIcons,
  ArrowLeft,
  ArrowRightLeft,
  Check,
  CheckCheck,
  CircleAlert,
  Clipboard,
  Cpu,
  Download,
  FileCode2,
  FileText,
  ImagePlus,
  Keyboard,
  Languages,
  Loader,
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
  ScanText,
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
  CircleAlert,
  Clipboard,
  Cpu,
  Download,
  FileCode2,
  FileText,
  ImagePlus,
  Keyboard,
  Languages,
  Loader,
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
  ScanText,
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
  ["ja", "日语"],
  ["ko", "韩语"],
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
    const prevStartupError = state.startup_error
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
      prevStartupStatus !== state.startup_status ||
      prevStartupError !== state.startup_error
    ) {
      const showingStartupPage =
        state.startup_loading &&
        state.mode === "settings" &&
        !state.settings_open
      // Never rebuild an already visible workbench/settings page on model
      // download progress: it would steal focus from settings inputs.
      if (!windowShown || showingStartupPage) {
        render({ forceShow: false })
        return
      }
      updateEngineIndicators()
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
    if (!document.querySelector("#engineState")) {
      render({ forceShow: false })
      return
    }
    updateEngineIndicators()
  })

  // The banner is re-rendered on engine status changes, so bind it at the
  // document level: a click anywhere in the banner opens the model settings.
  document.addEventListener("click", (event) => {
    if (!event.target.closest?.("#engineBanner")) return
    if (state.overlayOnly || state.mode === "overlay" || state.mode === "live") return
    prepareGlossaryDraft()
    state.glossaryPrepared = true
    state.settingsTab = "models"
    state.modelStatusesLoaded = false
    state.active_tab = "settings"
    render()
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
    if (payload.theme) applyTheme(payload.theme)
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
    if (payload.theme) applyTheme(payload.theme)
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
      renderLive()
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
        // Dedicated overlay/live process: never build the workbench UI, but keep
        // the live/overlay event subscriptions from bindStateEvents().
        state.overlayOnly = true
        state.mode = "none"
        bindStateEvents()
        try {
          const theme = await window.go?.wailsui?.App?.OverlayTheme?.()
          if (theme) applyTheme(theme)
        } catch {
          // Keep the cached theme.
        }
        startOverlayThemePoll()
        app.innerHTML = ""
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

let overlayLayoutTimer = 0
function scheduleOverlayLayout() {
  clearTimeout(overlayLayoutTimer)
  overlayLayoutTimer = setTimeout(() => {
    if (state.mode === "overlay") renderOverlay()
    else if (state.mode === "live") renderLive()
  }, 40)
}
if (typeof window !== "undefined") {
  window.addEventListener("resize", scheduleOverlayLayout)
  if (typeof ResizeObserver !== "undefined") {
    new ResizeObserver(scheduleOverlayLayout).observe(document.documentElement)
  }
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
  } else {
    // The workbench is always available immediately. Engine/model startup is
    // shown as a non-blocking banner so settings (including the model download
    // source) remain reachable even when the default mirror is offline.
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

// The overlay has a single copy button. It copies whatever is currently shown
// (translation or original) and falls back to the other one when that side is
// empty. Shift-click copies the Markdown version of the translation.
function copyCurrentOverlayText({ markdown, showOriginal, source, translation, markdownText }) {
  if (markdown) {
    if (markdownText) {
      copyShotText(markdownText, "已复制 Markdown 译文")
      return
    }
    showShotToast("没有可复制的译文")
    return
  }
  if (showOriginal) {
    if (source) copyShotText(source, "已复制原文")
    else if (translation) copyShotText(translation, "已复制译文")
    else showShotToast("没有可复制的文字")
    return
  }
  if (translation) copyShotText(translation, "已复制译文")
  else if (source) copyShotText(source, "已复制原文")
  else showShotToast("没有可复制的文字")
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

function renderOverlayBlocks(blocks, scope, { imgW, imgH, imageAreaW, fontFamily, fontWeight }) {
  const geometries = blocks.map((block) =>
    overlayBlockGeometry(block, imgW, imgH, imageAreaW),
  )
  return blocks
    .map((block, index) => {
      const source = String(block.text || "")
      const translated = String(block.translation || "")
      const blockOriginal =
        scope === "live" ? state.liveBlockOriginal : state.overlayBlockOriginal
      const showOriginal =
        scope === "live" ? state.liveOriginal : state.overlayOriginal
      const showSource = Boolean(showOriginal) || (blockOriginal || new Set()).has(index)
      if (showSource || !translated) return ""
      if (isUnchangedMathLine(source, translated)) return ""
      const { left, top, width, height, fontPx, angle } = fitOverlayBlockText({
        text: translated,
        index,
        geometries,
        imageAreaW,
        imageAreaH: imgH,
        fontFamily,
        fontWeight,
      })
      const textHtml = renderGlossaryText(translated, block.glossary_hits, "translation")
      const { textColor, backgroundStyle, textShadow } = overlayTextAppearance(block)
      const title = `${translated} · 单击显示原文，双击复制`
      return `<div class="shot-block" data-block-index="${index}" data-block-scope="${scope}"
        data-rect="${left},${top},${width},${height}" data-angle="${angle}"
        style="left:${left}%;top:${top}%;width:${width}%;height:${height}%;transform:rotate(${angle}deg);font-size:${fontPx}px;color:${textColor};${backgroundStyle}text-shadow:${textShadow};"
        title="${escapeHtml(title)}">${textHtml}</div>`
    })
    .join("")
}

function overlayRootHtml({ mode, barSide, imageLayout, toolbar, imageInner, toast = true }) {
  return `<div class="shot-window ${mode === "live" ? "live-window" : ""} bar-${barSide}"
      id="${mode === "live" ? "liveWindow" : "shotWindow"}">
    ${toolbar}
    <div class="shot-image-wrap" style="left:${imageLayout.left}px;top:${imageLayout.top}px;width:${imageLayout.width}px;height:${imageLayout.height}px">
      ${imageInner}
      <div class="shot-frame-border" aria-hidden="true"></div>
    </div>
    ${toast ? `<div class="shot-toast" id="shotToast" aria-live="polite"></div>` : ""}
  </div>`
}

function bindOverlayBlocks(scope) {
  document
    .querySelectorAll(`.shot-block[data-block-scope="${scope}"]`)
    .forEach((node) => {
      const index = Number(node.dataset.blockIndex)
      node.addEventListener("click", (event) => {
        if (event.detail > 1) return
        const key = scope === "live" ? "liveBlockOriginal" : "overlayBlockOriginal"
        if (!state[key]) state[key] = new Set()
        if (state[key].has(index)) state[key].delete(index)
        else state[key].add(index)
        if (scope === "live") renderLive()
        else renderOverlay()
      })
      node.addEventListener("dblclick", (event) => {
        event.preventDefault()
        const list = (scope === "live" ? state.live : state.overlay)?.blocks || []
        const block = list[index] || {}
        copyShotText(block.translation || block.text || "", "已复制该句译文")
      })
    })
}

// Shape the window to exactly what is drawn: the toolbar strip, the captured
// image and (when no frame exists) the chips/edges. The outline is pure CSS so
// it can never be clipped by the region.
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
  const barRect = document.querySelector(".ov-bar")?.getBoundingClientRect()
  const imageRect = document.querySelector(".shot-image-wrap")?.getBoundingClientRect()
  const hasFrame = Boolean(state.overlay?.image_data_url || state.live?.image_data_url)
  if (barRect) push(barRect)
  if (hasFrame) {
    if (imageRect) push(imageRect)
    SetOverlayRegion(JSON.stringify({ rects, polys: [] })).catch(() => {})
    return
  }
  const wrapRect = imageRect
  document
    .querySelectorAll(".shot-block, .shot-pill, .shot-fulltext, .shot-toast.is-visible")
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
          polys.push(
            [
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
            }),
          )
          return
        }
      }
      push(node.getBoundingClientRect())
    })
  if (wrapRect) push(wrapRect)
  SetOverlayRegion(JSON.stringify({ rects, polys })).catch(() => {})
}

// Edge strips resize the live region. Widths/heights here are CSS pixels; the
// host scales them with the monitor DPI before applying them natively.
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
      const minW = 160
      const minH = 140
      const onMove = (move) => {
        const dx = move.screenX - startX
        const dy = move.screenY - startY
        let { x, y, w, h } = start
        if (dir === "e") w = Math.max(minW, start.w + dx)
        else if (dir === "w") {
          w = Math.max(minW, start.w - dx)
          x = start.x + (start.w - w)
        } else if (dir === "s") h = Math.max(minH, start.h + dy)
        else if (dir === "n") {
          h = Math.max(minH, start.h - dy)
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
    <span class="shot-progress-fill" style="width:${clampNumber(progress, 0, 100, 0)}%"></span>
  </div>`
}

function overlayStatusView({
  loading,
  error,
  empty,
  ok,
  progress,
  stage,
  lines,
  latencyText,
  paused,
  listening,
}) {
  if (error) {
    return { kind: "error", label: "失败", title: error, percent: 0 }
  }
  if (loading) {
    const pct = clampNumber(progress, 0, 100, 0)
    const stageLabel =
      stage === "ocr" ? "识别文字" : stage === "translate" ? "翻译中" : "处理中"
    const detail = stage === "translate" && lines > 0 ? ` · ${lines} 句` : ""
    return {
      kind: "loading",
      label: `${stageLabel}${pct > 0 ? ` ${pct}%` : ""}`,
      title: `${stageLabel}${detail}${pct > 0 ? ` · ${pct}%` : ""}`,
      percent: pct,
    }
  }
  if (empty) {
    // B plan: the toolbar shows only a warning icon; the full sentence lives in
    // the tooltip and the "…" menu so a tiny selection never shows a text blob.
    return {
      kind: "empty",
      label: "",
      title: "没有识别到文字",
      percent: 0,
    }
  }
  if (paused) {
    return {
      kind: "paused",
      label: "已暂停",
      title: "实时翻译已暂停；窗口仍可被其它截图工具捕获",
      percent: 0,
    }
  }
  if (ok) {
    return {
      kind: "ok",
      label: `成功${latencyText ? ` · ${latencyText}` : ""}`,
      title: `翻译完成${latencyText ? `，耗时 ${latencyText}` : ""}`,
      percent: 100,
    }
  }
  if (listening) {
    return {
      kind: "listening",
      label: "监听中",
      title: "画面变化后自动识别并翻译",
      percent: 0,
    }
  }
  return { kind: "warn", label: "未翻译", title: "未翻译", percent: 0 }
}

const OVERLAY_STATUS_ICONS = {
  loading: "loader",
  ok: "check",
  empty: "circle-alert",
  error: "circle-alert",
  paused: "pause",
  listening: "monitor",
  warn: "circle-alert",
}

function overlayStatusHtml(view) {
  const icon = OVERLAY_STATUS_ICONS[view.kind] || "circle-alert"
  const label = String(view.label || "").trim()
  return `<span class="ov-status is-${view.kind}" title="${escapeHtml(view.title)}">
    <i data-lucide="${icon}" class="ov-status-icon"></i>
    ${label ? `<span class="ov-status-label">${escapeHtml(label)}</span>` : ""}
  </span>`
}

// One action list drives both the persistent toolbar buttons and the overflow
// menu, so a narrow window never loses a command: it only moves it into "…".
function overlayActions({ mode, showOriginal, pinned, paused }) {
  const live = mode === "live"
  const actions = [
    {
      id: "toggle",
      icon: "arrow-right-left",
      label: showOriginal ? "显示译文" : "显示原文",
      shortcut: "H",
    },
  ]
  if (live) {
    actions.push({
      id: "pause",
      icon: paused ? "play" : "pause",
      label: paused ? "继续实时翻译" : "暂停实时翻译",
      active: paused,
      primary: true,
    })
  }
  actions.push({
    id: "copy",
    icon: "clipboard",
    label: "复制当前显示内容",
    shortcut: "Ctrl+C",
    primary: true,
  })
  actions.push({
    id: "copy-markdown",
    icon: "file-code-2",
    label: "复制 Markdown 译文",
    menuOnly: true,
  })
  actions.push({
    id: "refresh",
    icon: "rotate-ccw",
    label: "刷新译文",
  })
  actions.push({
    id: "pin",
    icon: pinned ? "pin" : "pin-off",
    label: pinned ? "取消置顶" : "置顶",
    active: pinned,
  })
  return actions
}

// Shared toolbar: a status dot is always visible, the badge and status text
// collapse first, secondary actions move into "…", but "…" and close never
// leave the bar. The progress line lives at the bar's bottom edge.
function overlayToolbarHtml({ mode, view, showOriginal, pinned, paused, actions }) {
  const live = mode === "live"
  const buttons = actions
    .filter((action) => !action.menuOnly)
    .map(
      (action) => `<button class="ov-btn ${action.active ? "is-active" : ""}"
        data-action="${action.id}"
        data-priority="${action.primary ? "primary" : "secondary"}"
        type="button"
        title="${escapeHtml(action.label)}${action.shortcut ? ` (${action.shortcut})` : ""}">
        <i data-lucide="${action.icon}"></i>
      </button>`,
    )
    .join("")
  const progressActive =
    view.kind === "loading" && view.percent > 0 && view.percent < 100
  return `<div class="ov-bar" data-mode="${mode}" data-kind="${view.kind}" style="--wails-draggable: drag">
    <span class="ov-dot is-${view.kind}" title="${escapeHtml(view.title)}"></span>
    <span class="ov-badge" title="${live ? "实时翻译" : "截图翻译"}">
      <i data-lucide="${live ? "scan-text" : "monitor-up"}"></i>
      <span class="ov-badge-text">${live ? "实时翻译" : "截图翻译"}</span>
    </span>
    ${overlayStatusHtml(view)}
    <span class="ov-spacer"></span>
    <div class="ov-actions">${buttons}</div>
    <button class="ov-btn ov-more" data-action="menu" type="button" title="更多操作（菜单）">
      <i data-lucide="more-horizontal"></i>
    </button>
    <button class="ov-btn ov-close" data-action="close" type="button" title="关闭 (Esc)">
      <i data-lucide="x"></i>
    </button>
    ${overlayProgressHtml(progressActive, view.percent)}
  </div>`
}

function overlayMenuEntries(ctx) {
  const entries = []
  let id = 1
  if (
    ctx.view.kind === "empty" ||
    ctx.view.kind === "error" ||
    ctx.view.kind === "loading"
  ) {
    entries.push({
      id: 101,
      label:
        ctx.view.kind === "empty"
          ? "没有识别到文字"
          : ctx.view.kind === "error"
            ? "翻译失败"
            : ctx.view.label,
      icon: ctx.view.kind === "loading" ? "info" : "warning",
      disabled: true,
    })
  }
  const actions = overlayActions({
    mode: ctx.scope,
    showOriginal: ctx.showOriginal,
    pinned: ctx.scope === "live" ? state.livePinned : state.overlayPinned,
    paused: ctx.paused,
  })
  const iconName = (action) => {
    switch (action.id) {
      case "toggle":
        return "toggle"
      case "copy":
        return "copy"
      case "copy-markdown":
        return "markdown"
      case "refresh":
        return "refresh"
      case "pin":
        return action.icon === "pin-off" ? "unpin" : "pin"
      case "close":
        return "close"
      default:
        return action.icon
    }
  }
  const shortLabel = (action) => {
    switch (action.id) {
      case "toggle":
        return ctx.showOriginal ? "显示译文" : "显示原文"
      case "pause":
        return ctx.paused ? "继续翻译" : "暂停翻译"
      case "copy":
        return "复制当前内容"
      case "copy-markdown":
        return "复制 Markdown"
      case "refresh":
        return "刷新译文"
      case "pin":
        return ctx.scope === "live"
          ? state.livePinned
            ? "取消置顶"
            : "置顶"
          : state.overlayPinned
            ? "取消置顶"
            : "置顶"
      default:
        return action.label
    }
  }
  for (const action of actions) {
    entries.push({
      id: id++,
      action: action.id,
      label: shortLabel(action),
      icon: iconName(action),
      shortcut: action.shortcut || "",
      checked: Boolean(action.active),
      disabled: false,
      danger: false,
    })
  }
  entries.push({ id: 0, separator: true, disabled: true })
  entries.push({
    id: id++,
    action: "close",
    label: "关闭",
    icon: "close",
    shortcut: "Esc",
    danger: true,
  })
  return entries
}

async function openOverlayMenu(ctx) {
  const button = document.querySelector(".ov-more")
  const bridge = window.go?.wailsui?.App
  if (!button || !bridge?.ShowOverlayMenu) return
  const entries = overlayMenuEntries(ctx)
  const rect = button.getBoundingClientRect()
  const barRect = document.querySelector(".ov-bar")?.getBoundingClientRect()
  // Align the popup's right edge with the toolbar's right edge so it stays
  // visually inside a normal selection instead of hanging off the "…" button.
  const anchorRight = (barRect?.right || rect.right) - 5
  const dpr = window.devicePixelRatio || 1
  const below = !document
    .querySelector(".shot-window")
    ?.classList.contains("bar-bottom")
  let selected = 0
  try {
    selected = await bridge.ShowOverlayMenu(
      entries.map(({ action, ...entry }) => entry),
      Math.round(anchorRight * dpr),
      Math.round((below ? rect.bottom : rect.top) * dpr),
      below,
    )
  } catch {
    return
  }
  const entry = entries.find((item) => item.id === Number(selected))
  if (entry?.action) runOverlayAction(entry.action, ctx)
}

function overlayActionContext(scope, view, blocks) {
  const payload = scope === "live" ? state.live : state.overlay
  return {
    scope,
    view,
    blocks,
    payload: payload || {},
    showOriginal:
      scope === "live" ? Boolean(state.liveOriginal) : Boolean(state.overlayOriginal),
    paused: scope === "live" && payload?.status === "paused",
    source: payload?.source || sourceJoin(blocks),
    translation: payload?.translation || translatedJoin(blocks),
    markdownText: markdownFromBlocks(blocks, payload?.translation),
  }
}

function runOverlayAction(action, ctx, event) {
  if (action === "close") {
    closeWindow()
    return
  }
  switch (action) {
    case "toggle":
      if (ctx.scope === "live") {
        state.liveOriginal = !state.liveOriginal
        state.liveBlockOriginal = new Set()
      } else {
        state.overlayOriginal = !state.overlayOriginal
        state.overlayBlockOriginal = new Set()
      }
      break
    case "pause": {
      const next = !ctx.paused
      window.go?.wailsui?.App?.LiveControl?.(next ? "pause" : "resume").catch(() => {})
      state.live = {
        ...state.live,
        status: next ? "paused" : "listening",
        stage: "",
        progress: 0,
        lines: 0,
      }
      break
    }
    case "copy": {
      const payload = ctx.scope === "live" ? state.live : state.overlay
      const fresh = overlayActionContext(ctx.scope, ctx.view, payload?.blocks || [])
      copyCurrentOverlayText({
        markdown: Boolean(event?.shiftKey),
        showOriginal: fresh.showOriginal,
        source: fresh.source,
        translation: fresh.translation,
        markdownText: fresh.markdownText,
      })
      break
    }
    case "copy-markdown": {
      const payload = ctx.scope === "live" ? state.live : state.overlay
      const fresh = overlayActionContext(ctx.scope, ctx.view, payload?.blocks || [])
      copyCurrentOverlayText({
        markdown: true,
        showOriginal: fresh.showOriginal,
        source: fresh.source,
        translation: fresh.translation,
        markdownText: fresh.markdownText,
      })
      break
    }
    case "refresh":
      if (ctx.scope === "live") {
        window.go?.wailsui?.App?.LiveControl?.("refresh").catch(() => {})
        showShotToast("正在刷新实时译文...")
      } else {
        window.go?.wailsui?.App?.RefreshOverlay?.().catch(() => {})
        showShotToast("正在刷新译文...")
      }
      break
    case "pin": {
      const key = ctx.scope === "live" ? "livePinned" : "overlayPinned"
      state[key] = !state[key]
      if (window.runtime?.WindowSetAlwaysOnTop) WindowSetAlwaysOnTop(state[key])
      if (window.go?.wailsui?.App?.SetOverlayPinned) {
        SetOverlayPinned(state[key]).catch(() => {})
      }
      break
    }
    default:
      return
  }
  if (ctx.scope === "live") renderLive()
  else renderOverlay()
}

function bindOverlayActions(scope, ctx) {
  document.querySelectorAll(".ov-btn[data-action]").forEach((node) => {
    node.addEventListener("click", (event) => {
      event.preventDefault()
      event.stopPropagation()
      const action = node.dataset.action || ""
      if (action === "menu") {
        openOverlayMenu(ctx)
        return
      }
      runOverlayAction(action, ctx, event)
    })
  })
}

function overlayImageLayout(area, windowRect) {
  const winW = Number(windowRect?.width) || 1
  const winH = Number(windowRect?.height) || 1
  const viewW = Math.max(1, window.innerWidth || 1)
  const viewH = Math.max(1, window.innerHeight || 1)
  const areaX = Number(area?.x) || 0
  const areaY = Number(area?.y) || 0
  const areaW = Number(area?.width) || Number(area?.w) || winW
  const areaH = Number(area?.height) || Number(area?.h) || winH
  const winX = Number(windowRect?.x) || 0
  const winY = Number(windowRect?.y) || 0
  return {
    left: Math.max(0, ((areaX - winX) / winW) * viewW),
    top: Math.max(0, ((areaY - winY) / winH) * viewH),
    width: Math.max(1, (areaW / winW) * viewW),
    height: Math.max(1, (areaH / winH) * viewH),
  }
}

function renderOverlay() {
  const overlay = state.overlay || {}
  const blocks = Array.isArray(overlay.blocks) ? overlay.blocks : []
  const loading = Boolean(overlay.loading)
  const error = overlay.error || ""
  const showOriginal = Boolean(state.overlayOriginal)
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
  const area = overlay.rect || { x: 0, y: 0, width: imgW, height: imgH }
  const windowRect = overlay.window_rect || area
  const imageLayout = overlayImageLayout(area, windowRect)
  const latencyMs = Number(overlay.latency_ms) || 0
  const latencyText = latencyMs > 0 ? `${(latencyMs / 1000).toFixed(1)}s` : ""
  const isEmpty =
    !loading &&
    !error &&
    !String(overlay.source || "").trim() &&
    !blocks.length &&
    !String(overlay.translation || "").trim()
  const translatedOk =
    !loading &&
    !error &&
    (blocks.some((block) => String(block.translation || "").trim()) ||
      Boolean(overlay.translation))
  const view = overlayStatusView({
    loading,
    error,
    empty: isEmpty,
    ok: translatedOk,
    progress: overlay.progress,
    stage: overlay.stage,
    lines: overlay.lines,
    latencyText,
  })
  const actions = overlayActions({
    mode: "overlay",
    showOriginal,
    pinned: state.overlayPinned,
    paused: false,
  })
  const ctx = overlayActionContext("overlay", view, blocks)
  const renderedBlocks = renderOverlayBlocks(blocks, "overlay", {
    imgW,
    imgH,
    imageAreaW: imageLayout.width,
    fontFamily: overlayFont,
    fontWeight: overlayWeight,
  })
  const fullText =
    !loading && !blocks.length && overlay.translation
      ? `<div class="shot-fulltext" title="整段译文">${escapeHtml(overlay.translation)}</div>`
      : ""
  const imageInner = `
    ${overlay.image_data_url ? `<img class="shot-image" src="${overlay.image_data_url}" alt="" draggable="false">` : ""}
    <div class="shot-blocks" style="--shot-font:${overlayFont};--shot-weight:${overlayWeight}">${renderedBlocks}</div>
    ${error ? `<div class="shot-pill is-error" title="${escapeHtml(error)}">${escapeHtml(error)}</div>` : ""}
    ${fullText}`

  app.innerHTML = overlayRootHtml({
    mode: "overlay",
    barSide,
    imageLayout,
    toolbar: overlayToolbarHtml({
      mode: "overlay",
      view,
      showOriginal,
      pinned: state.overlayPinned,
      paused: false,
      actions,
    }),
    imageInner,
  })
  createIcons({ icons, attrs: { "stroke-width": 1.8 } })

  bindOverlayActions("overlay", ctx)
  bindOverlayBlocks("overlay")
  bindOverlayResize()
  syncOverlayRegion()
}

function renderLive() {
  const live = state.live
  // Never paint the live view from a stale or empty payload: wait for the
  // host's first ShowLive (it carries the anchor and the captured frame).
  if (!live || !live.anchor) return
  if (state.overlayOnly) windowShown = true
  const blocks = Array.isArray(live.blocks) ? live.blocks : []
  const status = live.status || "listening"
  const paused = status === "paused"
  const loading = status === "translating"
  const error = live.error || ""
  const latencyMs = Number(live.latency_ms) || 0
  const latencyText = latencyMs > 0 ? `${(latencyMs / 1000).toFixed(1)}s` : ""
  const isEmpty =
    !loading &&
    !error &&
    !String(live.source || "").trim() &&
    !blocks.length &&
    !String(live.translation || "").trim()
  const translatedOk =
    blocks.some((block) => String(block.translation || "").trim()) ||
    Boolean(live.translation)
  const view = overlayStatusView({
    loading,
    error,
    empty: isEmpty,
    ok: translatedOk,
    progress: live.progress,
    stage: live.stage,
    lines: live.lines,
    latencyText,
    paused,
    listening: true,
  })
  const cfg = state.config || {}
  const style = live.style || {}
  const overlayFont = overlayFontStack(style.font_family || cfg.overlay_font_family)
  const overlayWeight = clampNumber(style.font_weight ?? cfg.overlay_font_weight, 300, 800, 500)
  const barSide = live.bar_side === "bottom" ? "bottom" : "top"
  const anchorW = Number(live.anchor?.width) || 1
  const anchorH = Number(live.anchor?.height) || 1
  const area = live.anchor || { x: 0, y: 0, width: anchorW, height: anchorH }
  const windowRect = live.window_rect || area
  const imageLayout = overlayImageLayout(area, windowRect)
  const actions = overlayActions({
    mode: "live",
    showOriginal: Boolean(state.liveOriginal),
    pinned: state.livePinned,
    paused,
  })
  const ctx = overlayActionContext("live", view, blocks)
  const renderedBlocks = renderOverlayBlocks(blocks, "live", {
    imgW: anchorW,
    imgH: anchorH,
    imageAreaW: imageLayout.width,
    fontFamily: overlayFont,
    fontWeight: overlayWeight,
  })
  const fallback =
    !blocks.length && live.translation
      ? `<div class="shot-fulltext" title="整段译文">${escapeHtml(live.translation)}</div>`
      : ""
  const imageInner = `
    ${live.image_data_url ? `<img class="shot-image" src="${live.image_data_url}" alt="" draggable="false">` : ""}
    <div class="shot-blocks" style="--shot-font:${overlayFont};--shot-weight:${overlayWeight}">${renderedBlocks}</div>
    ${error ? `<div class="shot-pill is-error" title="${escapeHtml(error)}">${escapeHtml(error)}</div>` : ""}
    ${fallback}
    <div class="shot-edge" data-edge="n"></div>
    <div class="shot-edge" data-edge="s"></div>
    <div class="shot-edge" data-edge="e"></div>
    <div class="shot-edge" data-edge="w"></div>`

  app.innerHTML = overlayRootHtml({
    mode: "live",
    barSide,
    imageLayout,
    toolbar: overlayToolbarHtml({
      mode: "live",
      view,
      showOriginal: Boolean(state.liveOriginal),
      pinned: state.livePinned,
      paused,
      actions,
    }),
    imageInner,
  })
  createIcons({ icons, attrs: { "stroke-width": 1.8 } })

  bindOverlayActions("live", ctx)
  bindOverlayBlocks("live")
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
  if (ready) {
    return { label: "引擎就绪", detail: "", state: "ready" }
  }
  const startupError = String(state.startup_error || "").trim()
  if (startupError) {
    return { label: "模型未就绪", detail: startupError, state: "error" }
  }
  const status = String(state.health?.translator_status || "").trim()
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
  const startupStatus = String(state.startup_status || "").trim()
  if (startupStatus && startupStatus !== "模型加载完成") {
    return {
      label: "引擎未就绪",
      detail: startupStatus,
      state: /未就绪|超时|失败/.test(startupStatus) ? "error" : "loading",
    }
  }
  return {
    label: "引擎未就绪",
    detail: status || startupStatus || "正在启动本地服务...",
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
      <button class="engine-banner-action" id="engineBannerSettings" type="button">模型设置</button>
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
            <p>自动模式：中文译为英文、其它语言译为中文；OCR 内置中英、日、韩识别</p>
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
              ${languageOptions(state.config.target_language, "自动（其它语言→中文）")}
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
  if (source === "huggingface") return "HuggingFace"
  if (source === "custom") return "自定义镜像"
  return "魔搭 ModelScope"
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
    <section class="settings-section" aria-labelledby="downloadHeading">
      <div class="section-heading">
        <div class="section-icon"><i data-lucide="network"></i></div>
        <div>
          <h2 id="downloadHeading">模型下载源</h2>
          <p>首次下载翻译模型时使用，默认魔搭（国内推荐）；切换后立即保存</p>
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
      <p class="field-help" id="modelSourceStatus" role="status"></p>
    </section>

    <div class="section-rule"></div>

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
  document.querySelectorAll('input[name="model_download_source"]').forEach((input) => {
    input.addEventListener("change", () => {
      toggleCustomSourceField({ currentTarget: input })
      saveModelSource()
    })
  })
  document
    .querySelector("#modelDownloadBaseUrl")
    ?.addEventListener("change", saveModelSource)
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

async function saveModelSource() {
  const status = document.querySelector("#modelSourceStatus")
  const source =
    document.querySelector('input[name="model_download_source"]:checked')?.value ||
    state.config.model_download_source ||
    "modelscope"
  const baseUrl =
    document.querySelector("#modelDownloadBaseUrl")?.value.trim() ||
    state.config.model_download_base_url ||
    ""
  if (source === "custom" && !baseUrl) {
    if (status) status.textContent = "请先填写镜像地址"
    return
  }
  const next = {
    ...state.config,
    model_download_source: source,
    model_download_base_url: baseUrl,
  }
  try {
    await SaveConfig(next)
    state.config = next
    if (status) {
      status.textContent = "下载源已保存"
      setTimeout(() => {
        if (status.textContent === "下载源已保存") status.textContent = ""
      }, 1800)
    }
  } catch (error) {
    if (status) status.textContent = `保存失败：${readableError(error)}`
  }
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

let overlayThemeTimer = 0
function startOverlayThemePoll() {
  clearInterval(overlayThemeTimer)
  overlayThemeTimer = setInterval(async () => {
    try {
      const theme = await window.go?.wailsui?.App?.OverlayTheme?.()
      if (theme && theme !== activeTheme) applyTheme(theme)
    } catch {
      // The daemon may be shutting down; keep the current theme.
    }
  }, 1200)
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
      ?.value ||
    state.config.model_download_source ||
    "modelscope"
  const downloadBaseUrl =
    document.querySelector("#modelDownloadBaseUrl")?.value.trim() ||
    state.config.model_download_base_url ||
    ""
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
    state.startup_error = next.startup_error || ""
    state.startup_status = next.startup_status || state.startup_status
    const engine = engineStatusInfo()
    updateEngineIndicators()
  } catch {
    // The next interval retries
  }
}

function updateEngineIndicators() {
  const engine = engineStatusInfo()
  const node = document.querySelector("#engineState")
  const text = document.querySelector("#engineText")
  if (node && text) {
    node.classList.toggle("is-ready", engine.state === "ready")
    node.classList.toggle("is-loading", engine.state === "loading")
    node.classList.toggle("is-error", engine.state === "error")
    node.title = engine.detail || "AI 引擎状态"
    text.textContent = engine.label
  }
  const banner = document.querySelector("#engineBanner")
  if (banner) {
    banner.style.display = engine.state === "ready" ? "none" : "flex"
    banner.classList.toggle("is-error", engine.state === "error")
    const bannerText = document.querySelector("#engineBannerText")
    if (bannerText) bannerText.textContent = engine.detail || engine.label
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
      copyCurrentOverlayText({
        showOriginal: Boolean(state.overlayOriginal),
        source: overlay.source || sourceJoin(overlay.blocks),
        translation: overlay.translation || translatedJoin(overlay.blocks),
        markdownText: markdownFromBlocks(overlay.blocks, overlay.translation),
      })
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
      copyCurrentOverlayText({
        showOriginal: Boolean(state.liveOriginal),
        source: live.source || sourceJoin(live.blocks),
        translation: live.translation || translatedJoin(live.blocks),
        markdownText: markdownFromBlocks(live.blocks, live.translation),
      })
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
