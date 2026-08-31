import "./styles.css"
import {
  createIcons,
  ArrowLeft,
  ArrowRightLeft,
  Check,
  CheckCheck,
  Clipboard,
  FileText,
  Keyboard,
  Languages,
  Monitor,
  MonitorUp,
  Moon,
  Network,
  RotateCcw,
  Sliders,
  Sparkles,
  Sun,
  Trash2,
  X,
} from "lucide"
import {
  HideWindow,
  SaveConfig,
  State,
  Translate,
} from "../wailsjs/go/wailsui/App"
import {
  EventsOn,
  WindowCenter,
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
  FileText,
  Keyboard,
  Languages,
  Monitor,
  MonitorUp,
  Moon,
  Network,
  RotateCcw,
  Sliders,
  Sparkles,
  Sun,
  Trash2,
  X,
}
const systemTheme = window.matchMedia("(prefers-color-scheme: dark)")
let healthTimer
let resultSignature = ""
let lastResultHeight = 0
let windowShown = false
let unbindStateEvent = null
let activeTheme = "system"
let startupWindowSized = false
let mainWindowSized = false

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
    ai_idle_minutes: 15,
  },
  health: {},
  manualTranslate: {
    sourceText: "",
    translatedText: "",
    isLoading: false,
    error: "",
  },
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

    if (nextMode === "settings" || nextMode === "translate") {
      state.active_tab = nextMode
    }

    const nextTheme = state.config?.theme
    if (nextTheme !== activeTheme) {
      applyTheme(nextTheme)
    }

    if (!windowShown || prevMode !== state.mode) {
      if (!windowShown) {
        state.manualTranslate = {
          sourceText: "",
          translatedText: "",
          isLoading: false,
          error: "",
        }
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

    if (
      prevStartupLoading !== state.startup_loading ||
      prevStartupStatus !== state.startup_status
    ) {
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

    // Main window (translate/settings) can refresh engine badge without full rerender
    const node = document.querySelector("#engineState")
    const textNode = document.querySelector("#engineText")
    if (!node || !textNode) {
      render()
      return
    }
    const ready = Boolean(state.health?.translator_ready)
    node.classList.toggle("is-ready", ready)
    node.classList.toggle("is-loading", !ready)
    textNode.textContent = ready ? "引擎就绪" : "正在启动"
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
  const forceShow = options.forceShow !== false
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
  if (forceShow || !windowShown) {
    showWindowAfterRender()
  }
}

function showWindowAfterRender() {
  if (!window.runtime?.WindowShow) return

  requestAnimationFrame(() => {
    requestAnimationFrame(() => {
      WindowShow()
      windowShown = true
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

function renderMainWindow() {
  const isTranslateTab = state.active_tab === "translate"
  const isSettingsTab = state.active_tab === "settings"
  const isReady = Boolean(state.health?.translator_ready)
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
        <div class="engine-state ${isReady ? "is-ready" : "is-loading"}" id="engineState" title="AI 引擎状态">
          <span class="state-dot"></span>
          <span id="engineText">${isReady ? "引擎就绪" : "正在启动"}</span>
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
            placeholder="输入需要翻译的文本，按 Ctrl + Enter 快速翻译..."
            spellcheck="false"
          >${escapeHtml(source)}</textarea>
        </div>
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
  const { isLoading, error, translatedText } = state.manualTranslate

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
  updateManualResultBox()

  const translateBtn = document.querySelector("#doTranslateBtn")
  if (translateBtn) {
    translateBtn.disabled = true
    translateBtn.innerHTML =
      '<span class="loading-spinner"></span><span>翻译中...</span>'
  }

  try {
    let resultText = ""
    if (window.go?.wailsui?.App?.Translate) {
      const res = await Translate(text)
      resultText = (res?.result || "").trim()
    } else {
      // Mock for Vite local preview
      await new Promise((resolve) => setTimeout(resolve, 600))
      resultText = `好的设计让工具退居幕后，专注于创造价值。（模拟译文：${text}）`
    }
    state.manualTranslate.translatedText = resultText.trim()
  } catch (error) {
    state.manualTranslate.error = readableError(error)
  } finally {
    state.manualTranslate.isLoading = false
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

function updateManualResultBox() {
  const box = document.querySelector("#manualResultBox")
  if (box) {
    box.innerHTML = renderResultBoxContent()
    createIcons({ icons, attrs: { "stroke-width": 1.8 } })
    document
      .querySelector("#manualRetryBtn")
      ?.addEventListener("click", handleManualTranslate)
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

      <section class="settings-section" aria-labelledby="aiIdleHeading">
        <div class="section-heading">
          <div class="section-icon"><i data-lucide="languages"></i></div>
          <div>
            <h2 id="aiIdleHeading">AI 空闲回收</h2>
            <p>空闲一段时间后自动关闭 AI 服务，下次翻译再重新拉起</p>
          </div>
        </div>
        <div class="port-field">
          <label class="field-label" for="aiIdleMinutes">分钟</label>
          <input
            class="number-input"
            id="aiIdleMinutes"
            type="number"
            inputmode="numeric"
            min="0"
            step="1"
            value="${Number.isInteger(Number(state.config.ai_idle_minutes)) ? Number(state.config.ai_idle_minutes) : 15}"
            aria-describedby="aiIdleHelp"
          />
          <p class="field-help" id="aiIdleHelp">0 表示常驻，默认 15 分钟</p>
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
  document.querySelector("#cancelSettingsBtn")?.addEventListener("click", () => {
    state.active_tab = "translate"
    render()
  })
  document.querySelector("#saveBtn")?.addEventListener("click", saveSettings)
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

  if (!window.runtime) return
  if (selected === "light") WindowSetLightTheme()
  else if (selected === "dark") WindowSetDarkTheme()
  else WindowSetSystemDefaultTheme()
}

function bindHotkeyInput(input) {
  if (!input) return
  input.addEventListener("focus", () => {
    input.dataset.previous = input.value
    input.value = "请按下组合键"
    input.classList.add("is-recording")
  })

  input.addEventListener("blur", () => {
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
  const aiIdleMinutes = Number(document.querySelector("#aiIdleMinutes")?.value)

  if (!isValidHotkey(hotkeyDisplay) || !isValidHotkey(ocrHotkeyDisplay)) {
    if (status) showStatus(status, "快捷键需要包含修饰键和普通按键", "error")
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
    server_port: serverPort,
    ai_idle_minutes: aiIdleMinutes,
    theme:
      document.querySelector('input[name="theme"]:checked')?.value || "system",
  }

  if (button) button.disabled = true
  if (status) showStatus(status, "正在保存...", "pending")
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
  app.innerHTML = `
    <main class="startup-shell" role="status" aria-live="polite">
      <h1>选中翻译</h1>
      <p class="startup-status">${escapeHtml(status)}</p>
      ${error ? `<p class="startup-error">${escapeHtml(error)}</p>` : ""}
      <div class="startup-progress" aria-hidden="true"><span></span></div>
      <p class="startup-hint">首次启动可能需要下载模型，请稍候</p>
    </main>
  `
}

async function refreshHealth() {
  if (!window.go?.wailsui?.App) return
  try {
    const next = await State()
    const ready = Boolean(next.health?.translator_ready)
    state.health = next.health
    if (ready) clearInterval(healthTimer)
    const node = document.querySelector("#engineState")
    const text = document.querySelector("#engineText")
    if (!node || !text) return
    node.classList.toggle("is-ready", ready)
    node.classList.toggle("is-loading", !ready)
    text.textContent = ready ? "引擎就绪" : "正在启动"
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
  windowShown = false
  lastResultHeight = 0
  resultSignature = ""
  state.manualTranslate = {
    sourceText: "",
    translatedText: "",
    isLoading: false,
    error: "",
  }
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
  if (state.mode !== "result" || event.key !== "Escape" || event.repeat) return

  event.preventDefault()
  closeWindow()
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
systemTheme.addEventListener("change", () => {
  if (activeTheme === "system") applyTheme("system")
})
bindStateEvents()
loadState()
