import "./styles.css"
import {
  createIcons,
  Check,
  Clipboard,
  FileText,
  Keyboard,
  Languages,
  Monitor,
  MonitorUp,
  Moon,
  Network,
  Sun,
  X,
} from "lucide"
import {
  SaveConfig,
  State,
} from "../wailsjs/go/wailsui/App"
import {
  Quit,
  WindowSetDarkTheme,
  WindowSetLightTheme,
  WindowSetSize,
  WindowSetSystemDefaultTheme,
  WindowShow,
} from "../wailsjs/runtime/runtime"

const app = document.querySelector("#app")

const icons = {
  Check,
  Clipboard,
  FileText,
  Keyboard,
  Languages,
  Monitor,
  MonitorUp,
  Moon,
  Network,
  Sun,
  X,
}
const systemTheme = window.matchMedia("(prefers-color-scheme: dark)")
let healthTimer
let resultTimer
let resultSignature = ""
let activeTheme = "system"
let state = {
  mode: "settings",
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
  },
  health: {},
}

async function loadState() {
  if (!window.go?.wailsui?.App) {
    state = previewState()
    applyTheme(state.config?.theme)
    render()
    return
  }

  try {
    state = await State()
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
  if (new URLSearchParams(window.location.search).get("mode") === "result") {
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
  return { ...state, health: { translator_ready: true } }
}

function render() {
  clearInterval(healthTimer)
  clearTimeout(resultTimer)
  document.documentElement.classList.toggle(
    "result-mode",
    state.mode === "result",
  )
  state.mode === "result" ? renderResult() : renderSettings()
  createIcons({ icons, attrs: { "stroke-width": 1.8 } })
  showWindowAfterRender()
}

function showWindowAfterRender() {
  if (!window.runtime?.WindowShow) return

  requestAnimationFrame(() => {
    requestAnimationFrame(() => WindowShow())
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

  app.innerHTML = `
    <main class="result-shell ${hasError ? "is-error" : ""}">
      <header class="app-header result-header">
        <div class="brand-mark" aria-hidden="true"><i data-lucide="languages"></i></div>
        <div class="header-copy">
          <h1>${isRecognizing ? "正在识别" : isLoading ? "翻译中" : hasError ? "翻译失败" : "选中翻译"}</h1>
        </div>
        <div class="result-header-actions">
          <span id="copyStatus" class="result-copy-status" aria-live="polite"></span>
          <button class="icon-button result-copy-button" id="copyBtn" type="button" title="${hasError ? "复制错误详情" : "复制译文"}" aria-label="${hasError ? "复制错误详情" : "复制译文"}" ${copyText ? "" : "disabled"}>
            <i data-lucide="clipboard"></i>
          </button>
          <button class="icon-button close-button" id="closeBtn" type="button" title="关闭窗口" aria-label="关闭窗口">
            <i data-lucide="x"></i>
          </button>
        </div>
      </header>

      <div class="result-content">
        ${
          hasError
            ? `
          <section class="error-panel" role="alert">
            <span class="section-label">错误详情</span>
            <div class="result-text error-text">${escapeHtml(result.error || "")}</div>
          </section>
        `
            : `
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
      </div>
    </main>
  `

  document.querySelector("#closeBtn").addEventListener("click", closeWindow)
  document
    .querySelector("#copyBtn")
    .addEventListener("click", () => copyResult(copyText))
  requestAnimationFrame(fitResultWindow)
  if (isLoading) scheduleResultRefresh()
}

function scheduleResultRefresh() {
  clearTimeout(resultTimer)
  resultTimer = setTimeout(refreshResultState, 120)
}

async function refreshResultState() {
  try {
    const next = await State()
    if (JSON.stringify(next.result || {}) !== resultSignature) {
      state = next
      render()
      return
    }
  } catch {
    // A state write may be in progress; the next read will retry.
  }

  if (state.result?.loading) scheduleResultRefresh()
}

function renderSettings() {
  app.innerHTML = `
    <main class="settings-shell">
      <header class="app-header settings-header">
        <div class="header-copy">
          <h1>设置</h1>
        </div>
        <button class="icon-button close-button" id="settingsCloseBtn" type="button" title="关闭窗口" aria-label="关闭窗口">
          <i data-lucide="x"></i>
        </button>
      </header>

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

      <footer class="settings-footer">
        <span id="saveStatus" class="action-status" aria-live="polite">保存后立即应用设置</span>
        <button id="saveBtn" class="primary-button" type="button">
          <span>保存</span>
        </button>
      </footer>
    </main>
  `

  bindHotkeyInput(document.querySelector("#hotkeyDisplay"))
  bindHotkeyInput(document.querySelector("#ocrHotkeyDisplay"))
  document
    .querySelectorAll('input[name="theme"]')
    .forEach((input) => input.addEventListener("change", previewTheme))
  document
    .querySelector("#ocrEnabled")
    .addEventListener("change", toggleOcrField)
  document.querySelector("#saveBtn").addEventListener("click", saveSettings)
  document
    .querySelector("#settingsCloseBtn")
    .addEventListener("click", closeWindow)
  healthTimer = setInterval(refreshHealth, 2500)
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
  field.classList.toggle("is-disabled", !enabled)
  input.disabled = !enabled
}

async function saveSettings() {
  const status = document.querySelector("#saveStatus")
  const button = document.querySelector("#saveBtn")
  const hotkeyDisplay = document.querySelector("#hotkeyDisplay").value.trim()
  const ocrHotkeyDisplay = document
    .querySelector("#ocrHotkeyDisplay")
    .value.trim()
  const serverPort = Number(document.querySelector("#serverPort").value)

  if (!isValidHotkey(hotkeyDisplay) || !isValidHotkey(ocrHotkeyDisplay)) {
    showStatus(status, "快捷键需要包含修饰键和普通按键", "error")
    return
  }
  if (!Number.isInteger(serverPort) || serverPort < 1024 || serverPort > 65535) {
    showStatus(status, "端口必须是 1024 到 65535 之间的整数", "error")
    document.querySelector("#serverPort").focus()
    return
  }

  const next = {
    ...state.config,
    hotkey: displayHotkeyToExpr(hotkeyDisplay),
    hotkey_display: normalizeDisplay(hotkeyDisplay),
    ocr_enabled: document.querySelector("#ocrEnabled").checked,
    ocr_hotkey: displayHotkeyToExpr(ocrHotkeyDisplay),
    ocr_hotkey_display: normalizeDisplay(ocrHotkeyDisplay),
    logging_enabled: document.querySelector("#loggingEnabled").checked,
    server_port: serverPort,
    theme:
      document.querySelector('input[name="theme"]:checked')?.value || "system",
  }

  button.disabled = true
  showStatus(status, "正在保存...", "pending")
  try {
    await SaveConfig(next)
    state.config = next
    showStatus(status, "设置已保存", "success")
    setTimeout(closeWindow, 450)
  } catch (error) {
    showStatus(status, readableError(error), "error")
  } finally {
    button.disabled = false
  }
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
    // The next interval retries while the local service starts.
  }
}

async function copyResult(text) {
  const status = document.querySelector("#copyStatus")
  try {
    await navigator.clipboard.writeText(text || "")
    status.textContent = "已复制"
    status.className = "result-copy-status success"
    const button = document.querySelector("#copyBtn")
    button.innerHTML = '<i data-lucide="check"></i>'
    button.classList.add("is-success")
    button.title = "已复制"
    createIcons({ icons, attrs: { "stroke-width": 1.8 } })
  } catch (error) {
    status.textContent = "复制失败"
    status.className = "result-copy-status error"
  }
}

function closeWindow() {
  try {
    Quit()
  } catch {
    window.close()
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
loadState()
