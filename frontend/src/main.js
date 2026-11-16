import "./styles.css"
import {
  createIcons,
  ArrowLeft,
  ArrowRightLeft,
  Check,
  CheckCheck,
  Clipboard,
  Clock,
  Cloud,
  Cpu,
  Download,
  FileText,
  Globe,
  ImageUp,
  Keyboard,
  Languages,
  Link,
  Maximize2,
  Minimize2,
  Monitor,
  MonitorUp,
  Moon,
  Network,
  Power,
  RotateCcw,
  Sliders,
  Sparkles,
  Sun,
  Trash2,
  X,
  Zap,
} from "lucide"
import {
  HideWindow,
  OCRImage,
  SaveConfig,
  SetHotkeyRecording,
  State,
  Translate,
} from "../wailsjs/go/wailsui/App"
import {
  EventsOn,
  WindowCenter,
  WindowIsMaximised,
  WindowSetDarkTheme,
  WindowSetLightTheme,
  WindowSetSize,
  WindowSetSystemDefaultTheme,
  WindowShow,
  WindowToggleMaximise,
} from "../wailsjs/runtime/runtime"

const app = document.querySelector("#app")

const icons = {
  ArrowLeft,
  ArrowRightLeft,
  Check,
  CheckCheck,
  Clipboard,
  Clock,
  Cloud,
  Cpu,
  Download,
  FileText,
  Globe,
  ImageUp,
  Keyboard,
  Languages,
  Link,
  Maximize2,
  Minimize2,
  Monitor,
  MonitorUp,
  Moon,
  Network,
  Power,
  RotateCcw,
  Sliders,
  Sparkles,
  Sun,
  Trash2,
  X,
  Zap,
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
let mainWindowSized = false
let startupDismissed = false
let resultShowTimestamp = 0

const MAX_OCR_IMAGE_BYTES = 10 * 1024 * 1024

function emptyManualTranslateState() {
  return {
    sourceText: "",
    translatedText: "",
    isLoading: false,
    ocrLoading: false,
    error: "",
    elapsedText: "",
    notice: "",
  }
}

let state = {
  mode: "translate", // "translate" | "settings" | "result"
  active_tab: "translate", // "translate" | "settings"
  settings_open: false,
  startup_loading: false,
  startup_status: "正在启动本地服务...",
  startup_error: "",
  result: {},
  config: {
    hotkey: "<ctrl>+<alt>+q",
    hotkey_display: "Ctrl+Alt+Q",
    ocr_enabled: false,
    ocr_hotkey: "<ctrl>+<alt>+e",
    ocr_hotkey_display: "Ctrl+Alt+E",
    logging_enabled: false,
    server_port: 8989,
    theme: "system",
    acceleration_device: "auto",
    cpu_threads: 0,
    ui_idle_minutes: 5,
    ai_idle_minutes: 15,
    launch_at_login: false,
    model_download_source: "modelscope",
    model_download_base_url: "",
  },
  health: {},
  manualTranslate: emptyManualTranslateState(),
}

function bindStateEvents() {
  if (unbindStateEvent || !window.runtime?.EventsOn) return
  unbindStateEvent = EventsOn("ui:state", (next) => {
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
      startup_status: next.startup_status ?? "",
      startup_error: next.startup_error || "",
    }

    if (nextMode !== prevMode && (nextMode === "settings" || nextMode === "translate")) {
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
        state.manualTranslate = emptyManualTranslateState()
      }
      windowShown = false
      lastResultHeight = 0
      startupWindowSized = false
      mainWindowSized = false
      if (state.mode === "result") {
        app.innerHTML = ""
      }
      render({ forceShow: true })
      return
    }

    if (prevStartupLoading !== state.startup_loading) {
      render({ forceShow: false })
      return
    }

    if (state.mode === "result") {
      const nextSig = JSON.stringify(state.result || {})
      if (nextSig === prevSignature) return
      windowShown = false
      render({ forceShow: false })
      return
    }

    // Main window (translate/settings) updates status badge & banner in place without wiping inputs
    if (!document.querySelector("#engineState")) {
      render({ forceShow: false })
      return
    }
    updateEngineIndicators()
  })
}

async function loadState() {
  if (!window.go?.wailsui?.App) {
    state = previewState()
    applyTheme(state.config?.theme)
    render()
    return
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
  const modeParam = new URLSearchParams(window.location.search).get("mode")
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
    },
  }
}

function render(options = {}) {
  if (startupDismissed && state.startup_loading) {
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
  if (state.startup_loading) {
    renderStartup()
  } else if (state.mode === "result") {
    renderResult()
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

function renderResult() {
  const result = state.result || {}
  const isLoading = Boolean(result.loading)
  const hasSource = Boolean((result.source || "").trim())
  const isRecognizing = isLoading && !hasSource
  const hasError = !isLoading && Boolean(result.error)
  const copyText = hasError ? result.error : result.result
  const elapsedLabel =
    !isLoading && !hasError && Number(result.elapsed_ms) > 0
      ? formatDuration(Number(result.elapsed_ms))
      : ""
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
          <span id="resultElapsed" class="elapsed-badge hidden" aria-live="polite"></span>
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
  const resultElapsed = document.querySelector("#resultElapsed")
  if (!shell || !title || !content || !copyBtn) return

  if (resultElapsed) {
    resultElapsed.textContent = elapsedLabel ? `耗时 ${elapsedLabel}` : ""
    resultElapsed.classList.toggle("hidden", !elapsedLabel)
  }

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

function isFailureStatusText(text) {
  if (!text || text.includes("正在重试") || text.includes("断点续传")) {
    return false
  }
  const lower = text.toLowerCase()
  return (
    text.includes("失败") ||
    text.includes("拒绝") ||
    text.includes("未找到") ||
    text.includes("超时") ||
    lower.includes("error") ||
    lower.includes("failed")
  )
}

function engineStatusInfo() {
  const ready = Boolean(state.health?.translator_ready)
  if (ready) {
    return {
      ready: true,
      isError: false,
      badgeClass: "is-ready",
      badgeText: "引擎就绪",
      bannerText: "",
    }
  }

  const healthStatus = (state.health?.translator_status || "").trim()
  const errText = (state.startup_error || "").trim()
  const statusText = (
    healthStatus ||
    state.startup_status ||
    "正在后台准备翻译模型..."
  ).trim()

  if (errText || isFailureStatusText(healthStatus)) {
    return {
      ready: false,
      isError: true,
      badgeClass: "is-error",
      badgeText: "启动异常",
      bannerText: errText || healthStatus,
    }
  }

  return {
    ready: false,
    isError: false,
    badgeClass: "is-loading",
    badgeText: "正在启动",
    bannerText: statusText || "正在后台下载或加载翻译模型...",
  }
}

function updateEngineIndicators() {
  const info = engineStatusInfo()
  const node = document.querySelector("#engineState")
  const textNode = document.querySelector("#engineText")
  if (node && textNode) {
    node.classList.toggle("is-ready", info.ready)
    node.classList.toggle("is-loading", !info.ready && !info.isError)
    node.classList.toggle("is-error", !info.ready && info.isError)
    textNode.textContent = info.badgeText
  }

  const banner = document.querySelector("#engineBanner")
  const bannerText = document.querySelector("#engineBannerText")
  const bannerAction = document.querySelector("#engineBannerActionBtn")
  if (banner && bannerText) {
    banner.classList.toggle("hidden", info.ready)
    banner.classList.toggle("is-loading", !info.ready && !info.isError)
    banner.classList.toggle("is-error", !info.ready && info.isError)
    bannerText.textContent = info.bannerText
    if (bannerAction) {
      bannerAction.textContent = info.isError ? "切换下载源" : "下载源"
      bannerAction.classList.toggle("hidden", state.active_tab !== "translate")
    }
  }
}

function renderMainWindow() {
  const isTranslateTab = state.active_tab === "translate"
  const isSettingsTab = state.active_tab === "settings"
  const info = engineStatusInfo()
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
        <div class="engine-state ${info.badgeClass}" id="engineState" title="AI 引擎状态">
          <span class="state-dot"></span>
          <span id="engineText">${info.badgeText}</span>
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
          <button
            class="icon-button header-tool-button"
            id="mainMaxBtn"
            type="button"
            title="最大化 / 还原"
            aria-label="最大化 / 还原"
          >
            <i data-lucide="maximize-2"></i>
          </button>
          <button class="icon-button close-button" id="mainCloseBtn" type="button" title="关闭窗口" aria-label="关闭窗口">
            <i data-lucide="x"></i>
          </button>
        </div>
      </header>

      <div class="engine-banner ${info.ready ? "hidden" : info.isError ? "is-error" : "is-loading"}" id="engineBanner" role="status">
        <span class="engine-banner-dot"></span>
        <span class="engine-banner-text" id="engineBannerText">${escapeHtml(info.bannerText)}</span>
        <button class="engine-banner-action ${isTranslateTab ? "" : "hidden"}" id="engineBannerActionBtn" type="button">${info.isError ? "切换下载源" : "下载源"}</button>
      </div>

      <div class="main-body">
        ${isTranslateTab ? renderTranslateViewHTML() : renderSettingsViewHTML()}
      </div>

      ${isSettingsTab ? renderSettingsFooterHTML() : ""}
    </main>
  `

  document.querySelector("#headerBackBtn")?.addEventListener("click", () => {
    state.active_tab = "translate"
    render()
  })
  document.querySelector("#openSettingsBtn")?.addEventListener("click", () => {
    state.active_tab = "settings"
    render()
  })
  document
    .querySelector("#engineBannerActionBtn")
    ?.addEventListener("click", () => {
      state.active_tab = "settings"
      render()
      requestAnimationFrame(() => {
        document
          .querySelector("#modelDownloadSection")
          ?.scrollIntoView({ behavior: "smooth", block: "start" })
      })
    })
  const maxBtn = document.querySelector("#mainMaxBtn")
  const syncMaxIcon = async () => {
    if (!maxBtn || !window.runtime?.WindowIsMaximised) return
    try {
      const maximised = await WindowIsMaximised()
      maxBtn.innerHTML = `<i data-lucide="${maximised ? "minimize-2" : "maximize-2"}"></i>`
      maxBtn.title = maximised ? "还原窗口" : "最大化"
      createIcons({ icons, attrs: { "stroke-width": 1.8 } })
    } catch {
      // ignore
    }
  }
  maxBtn?.addEventListener("click", () => {
    if (!window.runtime?.WindowToggleMaximise) return
    WindowToggleMaximise()
    setTimeout(syncMaxIcon, 80)
  })
  syncMaxIcon()
  document
    .querySelector("#mainCloseBtn")
    ?.addEventListener("click", closeWindow)

  // Bind specific view events
  if (isTranslateTab) {
    bindTranslateViewEvents()
  } else {
    bindSettingsViewEvents()
  }

  if (healthTimer) clearInterval(healthTimer)
  healthTimer = setInterval(refreshHealth, 1200)
}

function renderTranslateViewHTML() {
  const source = state.manualTranslate.sourceText || ""
  const hasText = Boolean(source.trim())
  const charCount = source.length
  const isBusy =
    state.manualTranslate.isLoading || state.manualTranslate.ocrLoading
  const elapsedText = state.manualTranslate.elapsedText || ""

  return `
    <div class="translate-workbench">
      <section class="workbench-card source-card" id="sourceCard" aria-labelledby="sourceHeading">
        <div class="card-toolbar">
          <div class="card-title" id="sourceHeading">
            <i data-lucide="sparkles"></i>
            <span>原文 <span class="card-subtitle">(支持中英文本及图片 OCR)</span></span>
          </div>
          <div class="card-actions">
            <button
              class="card-action-btn"
              id="importImageBtn"
              type="button"
              title="导入图片并识别文字（支持粘贴 / 拖拽图片）"
              ${isBusy ? "disabled" : ""}
            >
              <i data-lucide="image-up"></i>
              <span>图片 OCR</span>
            </button>
            <input
              id="imageFileInput"
              type="file"
              accept="image/png,image/jpeg,image/webp,image/bmp,image/gif,image/*"
              class="hidden"
            />
            <button class="card-action-btn" id="clearBtn" type="button" title="清空内容" ${hasText && !isBusy ? "" : "disabled"}>
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
            placeholder="输入需要翻译的文本，或直接粘贴 (Ctrl+V) / 拖拽图片到此处识别..."
            spellcheck="false"
          >${escapeHtml(source)}</textarea>
          <div class="source-drop-overlay" aria-hidden="true">
            <i data-lucide="image-up"></i>
            <span>松开即可识别图片文字并翻译</span>
          </div>
        </div>
        <div class="source-bottom-bar">
          <div class="shortcut-hint">
            <span class="kbd-badge">Ctrl</span> + <span class="kbd-badge">Enter</span> 翻译 · 可粘贴/拖拽图片
          </div>
          <button
            id="doTranslateBtn"
            class="primary-button translate-submit-button"
            type="button"
            ${isBusy || !hasText ? "disabled" : ""}
          >
            ${
              state.manualTranslate.isLoading
                ? '<span class="loading-spinner"></span><span>翻译中...</span>'
                : state.manualTranslate.ocrLoading
                  ? '<span class="loading-spinner"></span><span>识别中...</span>'
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
            <span id="manualElapsed" class="elapsed-badge ${elapsedText ? "" : "hidden"}" aria-live="polite">
              <i data-lucide="clock"></i>
              <span>${escapeHtml(elapsedText)}</span>
            </span>
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
  const { isLoading, ocrLoading, error, translatedText, notice } =
    state.manualTranslate

  if (ocrLoading) {
    return `
      <div class="translation-loading manual-loading" role="status" aria-live="polite">
        <span class="loading-spinner" aria-hidden="true"></span>
        <span>正在识别图片中的文字...</span>
      </div>
    `
  }

  if (isLoading) {
    return `
      <div class="translation-loading manual-loading" role="status" aria-live="polite">
        <span class="loading-spinner" aria-hidden="true"></span>
        <span>正在进行离线大模型推理翻译...</span>
      </div>
    `
  }

  if (error) {
    return `
      <div class="error-panel" role="alert">
        <span class="section-label">处理失败</span>
        <div class="result-text error-text">${escapeHtml(error.trim())}</div>
        ${
          state.manualTranslate.sourceText.trim()
            ? `<button class="retry-button" id="manualRetryBtn" type="button">
                <i data-lucide="rotate-ccw"></i>
                <span>重试</span>
              </button>`
            : ""
        }
      </div>
    `
  }

  if (translatedText && translatedText.trim()) {
    return `<div class="result-text translated-text manual-translated-text" tabindex="0">${escapeHtml(translatedText.trim())}</div>`
  }

  if (notice) {
    return `
      <div class="manual-placeholder manual-notice">
        <div class="placeholder-icon"><i data-lucide="image-up"></i></div>
        <p>${escapeHtml(notice)}</p>
      </div>
    `
  }

  return `
    <div class="manual-placeholder">
      <div class="placeholder-icon"><i data-lucide="languages"></i></div>
      <p>输入文本后按 <span class="kbd-badge">Ctrl + Enter</span>，或粘贴/拖拽/导入图片进行 OCR 翻译</p>
    </div>
  `
}

function bindTranslateViewEvents() {
  const sourceCard = document.querySelector("#sourceCard")
  const textarea = document.querySelector("#sourceInput")
  const charCount = document.querySelector("#charCount")
  const clearBtn = document.querySelector("#clearBtn")
  const importImageBtn = document.querySelector("#importImageBtn")
  const imageFileInput = document.querySelector("#imageFileInput")
  const translateBtn = document.querySelector("#doTranslateBtn")
  const copyBtn = document.querySelector("#manualCopyBtn")

  const syncInputUI = () => {
    const value = textarea?.value || ""
    state.manualTranslate.sourceText = value
    const hasText = Boolean(value.trim())
    const isBusy =
      state.manualTranslate.isLoading || state.manualTranslate.ocrLoading
    if (charCount) charCount.textContent = `${value.length} 字符`
    if (clearBtn) clearBtn.disabled = !hasText || isBusy
    if (translateBtn) translateBtn.disabled = !hasText || isBusy
    if (importImageBtn) importImageBtn.disabled = isBusy
  }

  if (textarea) {
    textarea.addEventListener("input", syncInputUI)

    textarea.addEventListener("keydown", (event) => {
      if ((event.ctrlKey || event.metaKey) && event.key === "Enter") {
        event.preventDefault()
        handleManualTranslate()
      }
    })
  }

  const handlePaste = (event) => {
    const items = Array.from(event.clipboardData?.items || [])
    const imageItem = items.find(
      (item) => item.kind === "file" && item.type?.startsWith("image/"),
    )
    const file =
      imageItem?.getAsFile() ||
      Array.from(event.clipboardData?.files || []).find(isSupportedImageFile)
    if (!file) return
    event.preventDefault()
    handleImageOCRFile(file)
  }

  sourceCard?.addEventListener("paste", handlePaste)

  if (sourceCard) {
    let dragDepth = 0
    sourceCard.addEventListener("dragenter", (event) => {
      if (!hasDraggedFiles(event.dataTransfer)) return
      event.preventDefault()
      dragDepth += 1
      sourceCard.classList.add("is-drag-over")
    })
    sourceCard.addEventListener("dragover", (event) => {
      if (!hasDraggedFiles(event.dataTransfer)) return
      event.preventDefault()
      if (event.dataTransfer) event.dataTransfer.dropEffect = "copy"
    })
    sourceCard.addEventListener("dragleave", (event) => {
      if (!hasDraggedFiles(event.dataTransfer)) return
      dragDepth = Math.max(0, dragDepth - 1)
      if (dragDepth === 0 || !sourceCard.contains(event.relatedTarget)) {
        dragDepth = 0
        sourceCard.classList.remove("is-drag-over")
      }
    })
    sourceCard.addEventListener("drop", (event) => {
      event.preventDefault()
      dragDepth = 0
      sourceCard.classList.remove("is-drag-over")
      const files = Array.from(event.dataTransfer?.files || [])
      if (!files.length) return
      const imageFile = files.find(isSupportedImageFile)
      if (!imageFile) {
        state.manualTranslate.error =
          "请拖入 PNG、JPG、WebP、BMP 或 GIF 格式的图片文件"
        updateManualResultBox()
        return
      }
      handleImageOCRFile(imageFile)
    })
  }

  importImageBtn?.addEventListener("click", () => {
    if (!imageFileInput || state.manualTranslate.ocrLoading || state.manualTranslate.isLoading)
      return
    imageFileInput.value = ""
    imageFileInput.click()
  })

  imageFileInput?.addEventListener("change", () => {
    const file = imageFileInput.files?.[0]
    if (file) {
      handleImageOCRFile(file)
    }
    imageFileInput.value = ""
  })

  translateBtn?.addEventListener("click", () => handleManualTranslate())

  clearBtn?.addEventListener("click", () => {
    if (textarea) {
      textarea.value = ""
      textarea.focus()
    }
    state.manualTranslate = emptyManualTranslateState()
    syncInputUI()
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
    ?.addEventListener("click", () => handleManualTranslate())
}

function hasDraggedFiles(dataTransfer) {
  if (!dataTransfer) return false
  if (dataTransfer.files?.length) return true
  return Array.from(dataTransfer.types || []).includes("Files")
}

function isSupportedImageFile(file) {
  if (!file) return false
  if (file.type && file.type.startsWith("image/")) return true
  return /\.(png|jpe?g|webp|bmp|gif)$/i.test(file.name || "")
}

async function fileToBase64(file) {
  const buffer = await file.arrayBuffer()
  const bytes = new Uint8Array(buffer)
  const chunkSize = 0x8000
  let binary = ""
  for (let i = 0; i < bytes.length; i += chunkSize) {
    binary += String.fromCharCode(...bytes.subarray(i, i + chunkSize))
  }
  return btoa(binary)
}

function formatDuration(ms) {
  if (!Number.isFinite(ms) || ms <= 0) return ""
  if (ms < 1000) return `${Math.round(ms)} ms`
  return `${(ms / 1000).toFixed(2)} s`
}

function syncManualControlStates() {
  const textarea = document.querySelector("#sourceInput")
  const charCount = document.querySelector("#charCount")
  const clearBtn = document.querySelector("#clearBtn")
  const importImageBtn = document.querySelector("#importImageBtn")
  const translateBtn = document.querySelector("#doTranslateBtn")
  const text = state.manualTranslate.sourceText || ""
  const hasText = Boolean(text.trim())
  const isBusy =
    state.manualTranslate.isLoading || state.manualTranslate.ocrLoading

  if (textarea && textarea.value !== text) {
    textarea.value = text
  }
  if (charCount) charCount.textContent = `${text.length} 字符`
  if (clearBtn) clearBtn.disabled = !hasText || isBusy
  if (importImageBtn) importImageBtn.disabled = isBusy
  if (translateBtn) {
    translateBtn.disabled = !hasText || isBusy
    translateBtn.innerHTML = state.manualTranslate.isLoading
      ? '<span class="loading-spinner"></span><span>翻译中...</span>'
      : state.manualTranslate.ocrLoading
        ? '<span class="loading-spinner"></span><span>识别中...</span>'
        : '<i data-lucide="arrow-right-left"></i><span>立即翻译</span>'
    createIcons({ icons, attrs: { "stroke-width": 1.8 } })
  }
}

async function handleImageOCRFile(file) {
  if (state.manualTranslate.ocrLoading || state.manualTranslate.isLoading) return
  if (!isSupportedImageFile(file)) {
    state.manualTranslate.error =
      "不支持的文件类型，请选择 PNG、JPG、WebP、BMP 或 GIF 图片"
    updateManualResultBox()
    return
  }
  if (file.size > MAX_OCR_IMAGE_BYTES) {
    state.manualTranslate.error = "图片大小超过 10MB 限制，请压缩后重试"
    updateManualResultBox()
    return
  }

  state.manualTranslate.ocrLoading = true
  state.manualTranslate.error = ""
  state.manualTranslate.notice = ""
  state.manualTranslate.translatedText = ""
  state.manualTranslate.elapsedText = ""
  syncManualControlStates()
  updateManualResultBox()

  let recognizedText = ""
  let ocrElapsedMs = 0
  try {
    if (window.go?.wailsui?.App?.OCRImage) {
      const base64 = await fileToBase64(file)
      const res = await OCRImage(base64)
      recognizedText = (res?.text || "").trim()
      ocrElapsedMs = Number(res?.elapsed_ms) || 0
    } else {
      await new Promise((resolve) => setTimeout(resolve, 450))
      recognizedText = `Recognized text from ${file.name || "clipboard image"}`
      ocrElapsedMs = 450
    }
  } catch (error) {
    state.manualTranslate.ocrLoading = false
    state.manualTranslate.error = `图片文字识别失败: ${readableError(error)}`
    syncManualControlStates()
    updateManualResultBox()
    return
  }

  state.manualTranslate.ocrLoading = false
  if (!recognizedText) {
    state.manualTranslate.notice = "未从图片中识别到文字，可换张更清晰的截图试试"
    syncManualControlStates()
    updateManualResultBox()
    return
  }

  state.manualTranslate.sourceText = recognizedText
  const engineReady =
    Boolean(state.health?.translator_ready) || !window.go?.wailsui?.App?.Translate
  if (!engineReady) {
    const ocrLabel = formatDuration(ocrElapsedMs)
    state.manualTranslate.elapsedText = ocrLabel ? `OCR ${ocrLabel}` : ""
    state.manualTranslate.notice =
      "已识别图片文字并填入原文区，AI 引擎仍在加载中，就绪后点击“立即翻译”即可"
    syncManualControlStates()
    updateManualResultBox()
    return
  }

  syncManualControlStates()
  await handleManualTranslate({ ocrElapsedMs })
}

async function handleManualTranslate({ ocrElapsedMs = null } = {}) {
  const text = (state.manualTranslate.sourceText || "").trim()
  if (!text || state.manualTranslate.isLoading || state.manualTranslate.ocrLoading)
    return

  state.manualTranslate.isLoading = true
  state.manualTranslate.error = ""
  state.manualTranslate.notice = ""
  state.manualTranslate.elapsedText = ""
  syncManualControlStates()
  updateManualResultBox()

  const startedAt = performance.now()
  try {
    let resultText = ""
    let elapsedMs = 0
    if (window.go?.wailsui?.App?.Translate) {
      const res = await Translate(text)
      resultText = (res?.result || "").trim()
      elapsedMs = Number(res?.elapsed_ms) || Math.round(performance.now() - startedAt)
    } else {
      // Mock for Vite local preview
      await new Promise((resolve) => setTimeout(resolve, 420))
      resultText = `好的设计让工具退居幕后，专注于创造价值。（模拟译文：${text}）`
      elapsedMs = Math.round(performance.now() - startedAt)
    }
    state.manualTranslate.translatedText = resultText.trim()
    const trLabel = formatDuration(elapsedMs)
    const ocrLabel =
      Number.isFinite(ocrElapsedMs) && ocrElapsedMs > 0
        ? formatDuration(ocrElapsedMs)
        : ""
    if (ocrLabel && trLabel) {
      state.manualTranslate.elapsedText = `OCR ${ocrLabel} · 翻译 ${trLabel}`
    } else if (trLabel) {
      state.manualTranslate.elapsedText = `耗时 ${trLabel}`
    }
  } catch (error) {
    state.manualTranslate.error = readableError(error)
  } finally {
    state.manualTranslate.isLoading = false
    syncManualControlStates()
    updateManualResultBox()
  }
}

function updateManualResultBox() {
  const box = document.querySelector("#manualResultBox")
  if (box) {
    box.innerHTML = renderResultBoxContent()
    createIcons({ icons, attrs: { "stroke-width": 1.8 } })
    document
      .querySelector("#manualRetryBtn")
      ?.addEventListener("click", () => handleManualTranslate())
  }

  const elapsedBadge = document.querySelector("#manualElapsed")
  if (elapsedBadge) {
    const label = state.manualTranslate.elapsedText || ""
    elapsedBadge.classList.toggle("hidden", !label)
    const textSpan = elapsedBadge.querySelector("span")
    if (textSpan) textSpan.textContent = label
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
  return `
    <div class="settings-content">
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
        <div class="port-field source-sub-field">
          <label class="field-label" for="cpuThreads">线程</label>
          <input
            class="number-input"
            id="cpuThreads"
            type="number"
            inputmode="numeric"
            min="0"
            max="256"
            step="1"
            value="${Number.isInteger(Number(state.config.cpu_threads)) ? Math.max(0, Number(state.config.cpu_threads)) : 0}"
            aria-describedby="cpuThreadsHelp"
          />
          <p class="field-help" id="cpuThreadsHelp">CPU 推理线程数（0 表示自动检测核心数）</p>
        </div>
      </section>

      <div class="section-rule"></div>

      <section class="settings-section" id="modelDownloadSection" aria-labelledby="downloadHeading">
        <div class="section-heading">
          <div class="section-icon"><i data-lucide="download"></i></div>
          <div>
            <h2 id="downloadHeading">模型下载源</h2>
            <p>首次下载 Hy-MT2 模型时的下载通道</p>
          </div>
        </div>
        <div class="theme-options" role="radiogroup" aria-labelledby="downloadHeading">
          ${modelSourceOption("modelscope", "globe", "魔搭 ModelScope")}
          ${modelSourceOption("huggingface", "cloud", "HuggingFace")}
          ${modelSourceOption("custom", "link", "自定义镜像")}
        </div>
        <div class="source-field ${(state.config.model_download_source || "modelscope") === "custom" ? "" : "hidden"}" id="customSourceField">
          <label class="field-label" for="modelDownloadBaseUrl">镜像根地址或 .gguf 直链</label>
          <input
            class="text-input"
            id="modelDownloadBaseUrl"
            type="url"
            placeholder="https://hf-mirror.com 或完整 .gguf 链接"
            value="${escapeHtml(state.config.model_download_base_url || "")}"
          />
          <p class="field-help">支持 HF 镜像根地址或直接指向 .gguf 文件的 URL</p>
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

      <section class="settings-section" aria-labelledby="uiResidentHeading">
        <div class="section-heading with-toggle">
          <div class="section-icon"><i data-lucide="monitor"></i></div>
          <div>
            <h2 id="uiResidentHeading">界面常驻内存</h2>
            <p>关闭窗口后保留托盘与界面进程，再次唤醒零等待</p>
          </div>
          <label class="switch" title="界面常驻内存">
            <input
              id="uiResidentToggle"
              type="checkbox"
              data-last-minutes="${Number(state.config.ui_idle_minutes) > 0 ? Number(state.config.ui_idle_minutes) : 5}"
              ${(Number.isInteger(Number(state.config.ui_idle_minutes)) ? Number(state.config.ui_idle_minutes) : 5) === 0 ? "checked" : ""}
            />
            <span class="switch-track"><span class="switch-thumb"></span></span>
            <span class="sr-only">界面常驻内存</span>
          </label>
        </div>
      </section>

      <div class="section-rule"></div>

      <section class="settings-section" aria-labelledby="aiIdleHeading">
        <div class="section-heading with-toggle">
          <div class="section-icon"><i data-lucide="cpu"></i></div>
          <div>
            <h2 id="aiIdleHeading">AI 引擎常驻内存</h2>
            <p>开启后模型常驻后台；关闭则按空闲时间自动回收</p>
          </div>
          <label class="switch" title="AI 引擎常驻内存">
            <input
              id="aiResidentToggle"
              type="checkbox"
              data-last-minutes="${Number(state.config.ai_idle_minutes) > 0 ? Number(state.config.ai_idle_minutes) : 15}"
              ${(Number.isInteger(Number(state.config.ai_idle_minutes)) ? Number(state.config.ai_idle_minutes) : 15) === 0 ? "checked" : ""}
            />
            <span class="switch-track"><span class="switch-thumb"></span></span>
            <span class="sr-only">AI 引擎常驻内存</span>
          </label>
        </div>
        <div id="aiIdleField" class="port-field ocr-field ${(Number.isInteger(Number(state.config.ai_idle_minutes)) ? Number(state.config.ai_idle_minutes) : 15) === 0 ? "is-disabled" : ""}">
          <label class="field-label" for="aiIdleMinutes">分钟</label>
          <input
            class="number-input"
            id="aiIdleMinutes"
            type="number"
            inputmode="numeric"
            min="0"
            step="1"
            ${(Number.isInteger(Number(state.config.ai_idle_minutes)) ? Number(state.config.ai_idle_minutes) : 15) === 0 ? "disabled" : ""}
            value="${Number.isInteger(Number(state.config.ai_idle_minutes)) ? Number(state.config.ai_idle_minutes) : 15}"
            aria-describedby="aiIdleHelp"
          />
          <p class="field-help" id="aiIdleHelp">关闭常驻后的自动休眠等待分钟数（0 等同常驻）</p>
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
    </div>
  `
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
  document
    .querySelectorAll('input[name="theme"]')
    .forEach((input) => input.addEventListener("change", previewTheme))
  document
    .querySelector("#ocrEnabled")
    ?.addEventListener("change", toggleOcrField)
  const aiResidentToggle = document.querySelector("#aiResidentToggle")
  const aiIdleField = document.querySelector("#aiIdleField")
  const aiIdleInput = document.querySelector("#aiIdleMinutes")
  aiResidentToggle?.addEventListener("change", () => {
    const resident = Boolean(aiResidentToggle.checked)
    if (aiIdleField) aiIdleField.classList.toggle("is-disabled", resident)
    if (aiIdleInput) {
      if (resident) {
        const cur = Number(aiIdleInput.value)
        if (Number.isInteger(cur) && cur > 0) {
          aiResidentToggle.dataset.lastMinutes = String(cur)
        }
        aiIdleInput.value = "0"
        aiIdleInput.disabled = true
      } else {
        aiIdleInput.disabled = false
        aiIdleInput.value = aiResidentToggle.dataset.lastMinutes || "15"
      }
    }
  })
  aiIdleInput?.addEventListener("change", () => {
    const val = Number(aiIdleInput.value)
    if (Number.isInteger(val) && val > 0 && aiResidentToggle) {
      aiResidentToggle.dataset.lastMinutes = String(val)
    } else if (val === 0 && aiResidentToggle) {
      aiResidentToggle.checked = true
      aiIdleInput.disabled = true
      aiIdleField?.classList.add("is-disabled")
    }
  })
  document
    .querySelectorAll('input[name="model_download_source"]')
    .forEach((input) =>
      input.addEventListener("change", () => applyDownloadSourceChange(true)),
    )
  document
    .querySelector("#modelDownloadBaseUrl")
    ?.addEventListener("change", () => applyDownloadSourceChange(false))
  document.querySelector("#cancelSettingsBtn")?.addEventListener("click", () => {
    setHotkeyRecording(false)
    state.active_tab = "translate"
    render()
  })
  document.querySelector("#saveBtn")?.addEventListener("click", saveSettings)
}

function isHttpUrl(raw) {
  return /^https?:\/\/[^/\s]+/i.test((raw || "").trim())
}

async function applyDownloadSourceChange(fromRadio) {
  const source =
    document.querySelector('input[name="model_download_source"]:checked')
      ?.value || "modelscope"
  const customField = document.querySelector("#customSourceField")
  const urlInput = document.querySelector("#modelDownloadBaseUrl")
  const status = document.querySelector("#saveStatus")
  if (customField) {
    customField.classList.toggle("hidden", source !== "custom")
  }

  const baseUrl = (urlInput?.value || "").trim().replace(/\/+$/, "")
  if (source === "custom" && !baseUrl) {
    if (fromRadio) {
      urlInput?.focus()
      if (status) showStatus(status, "请输入自定义镜像地址或 .gguf 直链", "pending")
    }
    return
  }
  if (source === "custom" && !isHttpUrl(baseUrl)) {
    if (status)
      showStatus(status, "自定义地址需要以 http:// 或 https:// 开头", "error")
    urlInput?.focus()
    return
  }

  const next = {
    ...state.config,
    model_download_source: source,
    model_download_base_url: baseUrl,
  }
  if (!window.go?.wailsui?.App) {
    state.config = next
    return
  }

  try {
    await SaveConfig(next)
    const synced = await State()
    state.config = synced.config || next
    if (synced.health) state.health = synced.health
    if (urlInput) {
      urlInput.value = state.config.model_download_base_url || ""
    }
    updateEngineIndicators()
    if (status) {
      showStatus(status, "下载源已切换，如模型未完成将自动重试", "success")
    }
  } catch (error) {
    if (status) showStatus(status, `切换下载源失败: ${readableError(error)}`, "error")
  }
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

function radioCardOption(name, selectedValue, value, icon, label) {
  const selected = selectedValue === value
  return `
    <label class="theme-option">
      <input type="radio" name="${name}" value="${value}" ${selected ? "checked" : ""} />
      <span class="theme-option-content">
        <i data-lucide="${icon}"></i>
        <span>${label}</span>
      </span>
    </label>
  `
}

function themeOption(value, icon, label) {
  return radioCardOption("theme", state.config.theme || "system", value, icon, label)
}

function deviceOption(value, icon, label) {
  return radioCardOption(
    "acceleration_device",
    state.config.acceleration_device || "auto",
    value,
    icon,
    label,
  )
}

function modelSourceOption(value, icon, label) {
  return radioCardOption(
    "model_download_source",
    state.config.model_download_source || "modelscope",
    value,
    icon,
    label,
  )
}

function renderCurrentDeviceBadge() {
  const info = state.health?.device_info
  if (!info) return ""
  const actual = String(info.actual || "cpu")
  const isGpu = actual.toLowerCase().includes("gpu")
  const threads = Number(info.threads) > 0 ? ` · ${Number(info.threads)} 线程` : ""
  const label = `${isGpu ? "当前已生效：显卡 (GPU) 加速" : "当前运行模式：纯 CPU 推理"}${threads}`
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
  const hotkeyDisplay =
    document.querySelector("#hotkeyDisplay")?.value.trim() || ""
  const ocrHotkeyDisplay =
    document.querySelector("#ocrHotkeyDisplay")?.value.trim() || ""
  const serverPort = Number(document.querySelector("#serverPort")?.value)
  const cpuThreads = Number(document.querySelector("#cpuThreads")?.value)
  const uiResidentToggle = document.querySelector("#uiResidentToggle")
  const aiResident = Boolean(document.querySelector("#aiResidentToggle")?.checked)
  const aiIdleMinutes = aiResident
    ? 0
    : Number(document.querySelector("#aiIdleMinutes")?.value)
  const uiIdleMinutes = uiResidentToggle?.checked
    ? 0
    : Number(uiResidentToggle?.dataset.lastMinutes) > 0
      ? Number(uiResidentToggle.dataset.lastMinutes)
      : 5
  const modelDownloadSource =
    document.querySelector('input[name="model_download_source"]:checked')
      ?.value || "modelscope"
  const modelDownloadBaseUrl = (
    document.querySelector("#modelDownloadBaseUrl")?.value || ""
  )
    .trim()
    .replace(/\/+$/, "")

  if (!isValidHotkey(hotkeyDisplay) || !isValidHotkey(ocrHotkeyDisplay)) {
    if (status) showStatus(status, "快捷键需要包含修饰键和普通按键", "error")
    return
  }
  if (modelDownloadSource === "custom" && !isHttpUrl(modelDownloadBaseUrl)) {
    if (status)
      showStatus(status, "自定义下载地址需要以 http:// 或 https:// 开头", "error")
    document.querySelector("#modelDownloadBaseUrl")?.focus()
    return
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
  if (
    !Number.isInteger(cpuThreads) ||
    cpuThreads < 0 ||
    cpuThreads > 256
  ) {
    if (status)
      showStatus(status, "CPU 线程数必须是 0 到 256 之间的整数", "error")
    document.querySelector("#cpuThreads")?.focus()
    return
  }
  if (!Number.isInteger(aiIdleMinutes) || aiIdleMinutes < 0) {
    if (status)
      showStatus(status, "AI 空闲时间必须是大于等于 0 的整数", "error")
    document.querySelector("#aiIdleMinutes")?.focus()
    return
  }

  const next = {
    ...state.config,
    hotkey: displayHotkeyToExpr(hotkeyDisplay),
    hotkey_display: normalizeDisplay(hotkeyDisplay),
    ocr_enabled: Boolean(document.querySelector("#ocrEnabled")?.checked),
    ocr_hotkey: displayHotkeyToExpr(ocrHotkeyDisplay),
    ocr_hotkey_display: normalizeDisplay(ocrHotkeyDisplay),
    logging_enabled: Boolean(
      document.querySelector("#loggingEnabled")?.checked,
    ),
    launch_at_login: Boolean(
      document.querySelector("#launchAtLogin")?.checked,
    ),
    server_port: serverPort,
    cpu_threads: cpuThreads,
    ui_idle_minutes: uiIdleMinutes,
    ai_idle_minutes: aiIdleMinutes,
    acceleration_device:
      document.querySelector('input[name="acceleration_device"]:checked')?.value || "auto",
    theme:
      document.querySelector('input[name="theme"]:checked')?.value || "system",
    model_download_source: modelDownloadSource,
    model_download_base_url: modelDownloadBaseUrl,
  }

  if (button) button.disabled = true
  if (status) showStatus(status, "正在保存...", "pending")
  try {
    await SaveConfig(next)
    const synced = await State()
    state.config = synced.config || next
    if (synced.health) state.health = synced.health
    updateEngineIndicators()
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
  if (window.runtime?.WindowSetSize) {
    if (isMain) {
      if (!mainWindowSized) {
        WindowSetSize(458, 640)
        if (window.runtime?.WindowCenter) WindowCenter()
        mainWindowSized = true
      }
    } else if (!startupWindowSized) {
      WindowSetSize(360, 220)
      if (window.runtime?.WindowCenter) WindowCenter()
      startupWindowSized = true
    }
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
    state.health = next.health || {}
    state.startup_status = next.startup_status ?? ""
    state.startup_error = next.startup_error ?? ""
    updateEngineIndicators()
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
  resultShowTimestamp = 0
  windowShown = false
  lastResultHeight = 0
  resultSignature = ""
  state.manualTranslate = emptyManualTranslateState()
  if (document.querySelector(".result-shell")) {
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
    if (state.mode === "result") {
      event.preventDefault()
      closeWindow()
      return
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

window.addEventListener("dragover", (event) => {
  event.preventDefault()
})
window.addEventListener("drop", (event) => {
  if (!event.target?.closest?.("#sourceCard")) {
    event.preventDefault()
  }
})
window.addEventListener("keydown", handleWindowKeydown)
systemTheme.addEventListener("change", () => {
  if (activeTheme === "system") applyTheme("system")
})
bindStateEvents()
loadState()
