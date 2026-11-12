package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	DefaultServerPort    = 8989
	MinServerPort        = 1024
	MaxServerPort        = 65535
	DefaultUIIdleMinutes = 5
	DefaultAIIdleMinutes = 15
)

// ModelEntry is one translation model the user can activate. GGUF files live
// under <baseDir>/models/<dir>/<file>.
type ModelEntry struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Source  string `json:"source"`
	Repo    string `json:"repo"`
	File    string `json:"file"`
	Size    int64  `json:"size,omitempty"`
	Dir     string `json:"dir,omitempty"`
	Builtin bool   `json:"builtin,omitempty"`
}

// GlossaryEntry overrides how a specific term is translated.
type GlossaryEntry struct {
	Term        string `json:"term"`
	Translation string `json:"translation"`
	Source      string `json:"source"`
	Note        string `json:"note,omitempty"`
}

type Config struct {
	Hotkey           string `json:"hotkey"`
	HotkeyDisplay    string `json:"hotkey_display"`
	OcrEnabled       bool   `json:"ocr_enabled"`
	OcrHotkey        string `json:"ocr_hotkey"`
	OcrHotkeyDisplay string `json:"ocr_hotkey_display"`
	// LiveEnabled controls the "select a region and translate it continuously"
	// hotkey. It is independent from the one-shot screenshot hotkey.
	LiveEnabled       bool   `json:"live_enabled"`
	LiveHotkey        string `json:"live_hotkey"`
	LiveHotkeyDisplay string `json:"live_hotkey_display"`
	LoggingEnabled    bool   `json:"logging_enabled"`
	ServerPort        int    `json:"server_port"`
	Theme             string `json:"theme"`
	// AccelerationDevice: "auto" | "gpu" | "cpu"
	AccelerationDevice string `json:"acceleration_device"`
	// UIIdleMinutes is how long the UI process stays alive after the window is hidden.
	// 0 means never auto-exit. Missing values fall back to DefaultUIIdleMinutes.
	UIIdleMinutes int `json:"ui_idle_minutes"`
	// AIIdleMinutes is how long the AI backend stays alive after its last request.
	// 0 means never auto-exit. Missing values fall back to DefaultAIIdleMinutes.
	AIIdleMinutes int `json:"ai_idle_minutes"`
	// AIKeepAlive keeps the AI engine resident in the background so screenshot
	// translation never waits for a cold model start. When enabled it overrides
	// AIIdleMinutes.
	AIKeepAlive bool `json:"ai_keep_alive"`
	// AIThreads is the number of CPU threads used by llama.cpp. 0 = automatic.
	AIThreads int `json:"ai_threads"`
	// LaunchAtLogin enables starting the app automatically when the user logs in.
	// The host process writes its own executable path to the HKCU Run key.
	LaunchAtLogin bool `json:"launch_at_login"`
	// ModelDownloadSource selects where the first-run GGUF model is fetched from:
	// "modelscope" (default, recommended in mainland China), "huggingface" or "custom".
	ModelDownloadSource string `json:"model_download_source"`
	// ModelDownloadBaseURL is used when ModelDownloadSource is "custom". It may be
	// a HuggingFace-style mirror root (e.g. https://hf-mirror.com) or a complete
	// .gguf file URL.
	ModelDownloadBaseURL string `json:"model_download_base_url"`
	// Models is the user's translation model catalogue and ActiveModelID picks
	// the one the engine loads.
	Models        []ModelEntry `json:"models"`
	ActiveModelID string       `json:"active_model_id"`
	// SourceLanguage is "auto" or one of SupportedLanguages.
	SourceLanguage string `json:"source_language"`
	// TargetLanguage is "auto" (zh<->en automatic) or one of SupportedLanguages.
	TargetLanguage string `json:"target_language"`
	// OverlayFontFamily is one of the keys accepted by NormalizeOverlayFontFamily.
	OverlayFontFamily string `json:"overlay_font_family"`
	// OverlayFontWeight is 300..800 (CSS numeric weight). Text size is derived
	// from each OCR line box so translations match the source text size.
	OverlayFontWeight int `json:"overlay_font_weight"`
	// Glossary is the user's term -> translation dictionary.
	Glossary []GlossaryEntry `json:"glossary"`
}

const (
	ModelSourceModelScope  = "modelscope"
	ModelSourceHuggingFace = "huggingface"
	ModelSourceCustom      = "custom"
)

// BuiltinModelID identifies the model that ships with the installer and is
// auto-downloaded on first run.
const BuiltinModelID = "hy-mt2-1.8b-q4km"

// BuiltinModel is the default translation model (ModelScope, mainland-China
// friendly). Its Dir matches the legacy download location so existing installs
// keep using their 1.1 GB file without re-downloading.
func BuiltinModel() ModelEntry {
	return ModelEntry{
		ID:      BuiltinModelID,
		Name:    "混元 Hy-MT2 1.8B (Q4_K_M)",
		Source:  ModelSourceModelScope,
		Repo:    "Tencent-Hunyuan/Hy-MT2-1.8B-GGUF",
		File:    "Hy-MT2-1.8B-Q4_K_M.gguf",
		Size:    1133080448,
		Dir:     "Hy-MT2-1.8B-GGUF",
		Builtin: true,
	}
}

var Default = Config{
	Hotkey:               "<ctrl>+<alt>+q",
	HotkeyDisplay:        "Ctrl+Alt+Q",
	OcrEnabled:           true,
	OcrHotkey:            "<ctrl>+<alt>+e",
	OcrHotkeyDisplay:     "Ctrl+Alt+E",
	LiveEnabled:          true,
	LiveHotkey:           "<ctrl>+<alt>+r",
	LiveHotkeyDisplay:    "Ctrl+Alt+R",
	LoggingEnabled:       false,
	ServerPort:           DefaultServerPort,
	Theme:                "system",
	AccelerationDevice:   "auto",
	UIIdleMinutes:        DefaultUIIdleMinutes,
	AIIdleMinutes:        DefaultAIIdleMinutes,
	AIKeepAlive:          true,
	AIThreads:            0,
	LaunchAtLogin:        false,
	ModelDownloadSource:  "modelscope",
	ModelDownloadBaseURL: "",
	Models:               []ModelEntry{BuiltinModel()},
	ActiveModelID:        BuiltinModelID,
	SourceLanguage:       "auto",
	TargetLanguage:       "auto",
	OverlayFontFamily:    "system",
	OverlayFontWeight:    500,
}

// SupportedLanguages mirrors backend/translator.py's LANG_NAMES keys.
var SupportedLanguages = []string{
	"zh", "en", "zh-hant", "yue", "vi", "ja", "ko", "th", "it", "id", "ms", "tl",
	"hi", "fr", "es", "de", "pt", "ru", "ar", "pl", "cs", "nl", "km", "my", "fa",
	"gu", "ur", "te", "mr", "he", "bn", "ta", "uk", "bo", "kk", "mn", "ug",
}

// NormalizeLanguage returns "auto" or a supported ISO code.
func NormalizeLanguage(value string) string {
	normalized := strings.ToLower(strings.TrimSpace(value))
	if normalized == "" || normalized == "auto" {
		return "auto"
	}
	for _, code := range SupportedLanguages {
		if normalized == code {
			return code
		}
	}
	return "auto"
}

// NormalizeGlossary trims entries, drops empty rows, de-duplicates by term
// (case-insensitive, last entry wins) and caps the list size.
func NormalizeGlossary(entries []GlossaryEntry) []GlossaryEntry {
	if len(entries) == 0 {
		return nil
	}
	const maxEntries = 500
	result := make([]GlossaryEntry, 0, len(entries))
	for _, entry := range entries {
		term := strings.TrimSpace(entry.Term)
		translation := strings.TrimSpace(entry.Translation)
		source := strings.TrimSpace(entry.Source)
		note := strings.TrimSpace(entry.Note)
		if term == "" || translation == "" {
			continue
		}
		term = truncateRunes(term, 100)
		translation = truncateRunes(translation, 200)
		source = truncateRunes(source, 300)
		note = truncateRunes(note, 300)

		replaced := false
		for index := range result {
			if strings.EqualFold(result[index].Term, term) {
				result[index] = GlossaryEntry{Term: term, Translation: translation, Source: source, Note: note}
				replaced = true
				break
			}
		}
		if replaced {
			continue
		}
		result = append(result, GlossaryEntry{Term: term, Translation: translation, Source: source, Note: note})
		if len(result) >= maxEntries {
			break
		}
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

// NormalizeModels trims and validates the model catalogue, guarantees the
// builtin model is present, de-duplicates IDs and caps the list size.
func NormalizeModels(entries []ModelEntry) []ModelEntry {
	const maxModels = 50
	result := make([]ModelEntry, 0, len(entries)+1)
	seen := map[string]bool{}
	builtinPresent := false

	appendEntry := func(entry ModelEntry) {
		id := strings.TrimSpace(entry.ID)
		if id == "" {
			id = makeModelID(entry.Repo, entry.File)
		}
		base := id
		for suffix := 2; seen[id]; suffix++ {
			id = fmt.Sprintf("%s-%d", base, suffix)
		}
		seen[id] = true
		entry.ID = id
		result = append(result, entry)
	}

	for _, raw := range entries {
		source := NormalizeModelSource(raw.Source)
		repo := NormalizeModelRepo(raw.Repo)
		file := NormalizeModelFile(raw.File)
		if repo == "" || file == "" {
			continue
		}
		name := strings.TrimSpace(raw.Name)
		if name == "" {
			name = repo
		}
		entry := ModelEntry{
			ID:      strings.TrimSpace(raw.ID),
			Name:    truncateRunes(name, 80),
			Source:  source,
			Repo:    repo,
			File:    file,
			Size:    raw.Size,
			Dir:     NormalizeModelDir(raw.Dir, repo),
			Builtin: raw.Builtin,
		}
		if entry.Size < 0 {
			entry.Size = 0
		}
		if entry.Builtin {
			builtinPresent = true
			entry.ID = BuiltinModelID
			entry.Name = BuiltinModel().Name
			entry.Source = BuiltinModel().Source
			entry.Repo = BuiltinModel().Repo
			entry.File = BuiltinModel().File
			entry.Dir = BuiltinModel().Dir
		}
		appendEntry(entry)
		if len(result) >= maxModels {
			break
		}
	}
	if !builtinPresent {
		result = append([]ModelEntry{BuiltinModel()}, result...)
		if len(result) > maxModels {
			result = result[:maxModels]
		}
	}
	return result
}

// NormalizeActiveModelID returns a valid model ID, falling back to the first
// configured model (always the builtin at minimum).
func NormalizeActiveModelID(id string, models []ModelEntry) string {
	trimmed := strings.TrimSpace(id)
	for _, entry := range models {
		if entry.ID == trimmed {
			return trimmed
		}
	}
	if len(models) > 0 {
		return models[0].ID
	}
	return BuiltinModelID
}

// NormalizeModelSource accepts only the sources the downloader understands.
func NormalizeModelSource(source string) string {
	switch strings.ToLower(strings.TrimSpace(source)) {
	case ModelSourceHuggingFace:
		return ModelSourceHuggingFace
	default:
		return ModelSourceModelScope
	}
}

// NormalizeModelRepo accepts "namespace/name" or "name".
func NormalizeModelRepo(repo string) string {
	repo = strings.TrimSpace(strings.ReplaceAll(repo, "\\", "/"))
	repo = strings.Trim(repo, "/")
	parts := strings.Split(repo, "/")
	if len(parts) == 0 || len(parts) > 2 {
		return ""
	}
	cleaned := make([]string, 0, len(parts))
	for _, part := range parts {
		part = sanitizeModelSegment(part)
		if part == "" || part == "." || part == ".." {
			return ""
		}
		cleaned = append(cleaned, part)
	}
	return strings.Join(cleaned, "/")
}

// NormalizeModelFile validates a repo-relative .gguf path
// ("file.gguf" or "Q4_K_M/file.gguf"). Characters are preserved because the
// same value is used to build the download URL; LocalModelFile produces the
// Windows-safe path on disk.
func NormalizeModelFile(file string) string {
	file = strings.TrimSpace(strings.ReplaceAll(file, "\\", "/"))
	file = strings.Trim(file, "/")
	if file == "" || strings.Contains(file, "..") {
		return ""
	}
	parts := strings.Split(file, "/")
	if len(parts) > 4 {
		return ""
	}
	for _, part := range parts {
		if part == "" || part == "." || part == ".." {
			return ""
		}
		for _, char := range part {
			if char < 0x20 || char == 0x7f || char == ':' {
				return ""
			}
		}
	}
	if !strings.EqualFold(filepath.Ext(file), ".gguf") {
		return ""
	}
	return file
}

// LocalModelFile maps a repo-relative GGUF path to a Windows-safe relative
// path, preserving spaces and other characters that are legal on disk.
func LocalModelFile(file string) string {
	file = strings.ReplaceAll(file, "\\", "/")
	parts := strings.Split(file, "/")
	cleaned := make([]string, 0, len(parts))
	for _, part := range parts {
		var builder strings.Builder
		for _, char := range part {
			switch {
			case char < 0x20, char == 0x7f, strings.ContainsRune(`<>:"\|?*`, char):
				builder.WriteByte('_')
			default:
				builder.WriteRune(char)
			}
		}
		segment := strings.TrimRight(builder.String(), ". ")
		if segment == "" {
			segment = "_"
		}
		cleaned = append(cleaned, segment)
	}
	return filepath.FromSlash(strings.Join(cleaned, "/"))
}

// NormalizeModelDir returns a safe directory name for a model's files.
func NormalizeModelDir(dir, repo string) string {
	dir = sanitizeModelSegment(strings.TrimSpace(dir))
	if dir == "" || dir == "." || dir == ".." {
		dir = sanitizeModelSegment(strings.ReplaceAll(repo, "/", "-"))
	}
	if dir == "" || dir == "." || dir == ".." {
		dir = "models"
	}
	return dir
}

// ModelDirName is the on-disk folder name for a model entry.
func ModelDirName(entry ModelEntry) string {
	return NormalizeModelDir(entry.Dir, entry.Repo)
}

// ModelPath is the absolute GGUF path for a model entry.
func ModelPath(baseDir string, entry ModelEntry) string {
	relative := LocalModelFile(entry.File)
	return filepath.Join(baseDir, "models", ModelDirName(entry), relative)
}

// FindModel returns the entry with the given ID.
func FindModel(models []ModelEntry, id string) (ModelEntry, bool) {
	for _, entry := range models {
		if entry.ID == id {
			return entry, true
		}
	}
	return ModelEntry{}, false
}

// ActiveModel returns the configured active model, falling back to the builtin.
func ActiveModel(cfg Config) ModelEntry {
	if entry, ok := FindModel(cfg.Models, cfg.ActiveModelID); ok {
		return entry
	}
	return BuiltinModel()
}

func makeModelID(repo, file string) string {
	raw := strings.ToLower(repo + "-" + strings.TrimSuffix(file, filepath.Ext(file)))
	var builder strings.Builder
	lastDash := false
	for _, char := range raw {
		switch {
		case char >= 'a' && char <= 'z', char >= '0' && char <= '9':
			builder.WriteRune(char)
			lastDash = false
		case !lastDash:
			builder.WriteByte('-')
			lastDash = true
		}
	}
	id := strings.Trim(builder.String(), "-")
	if id == "" {
		return "model"
	}
	return truncateRunes(id, 64)
}

func sanitizeModelSegment(value string) string {
	var builder strings.Builder
	for _, char := range strings.TrimSpace(value) {
		switch {
		case char >= 'a' && char <= 'z',
			char >= 'A' && char <= 'Z',
			char >= '0' && char <= '9',
			char == '-', char == '_', char == '.':
			builder.WriteRune(char)
		default:
			builder.WriteByte('-')
		}
	}
	return strings.Trim(builder.String(), "-.")
}

func truncateRunes(value string, limit int) string {
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit])
}

// NormalizeHexColor returns a #RRGGBB string or fallback.
func NormalizeHexColor(value, fallback string) string {
	candidate := strings.ToUpper(strings.TrimSpace(value))
	if len(candidate) != 7 || candidate[0] != '#' {
		return fallback
	}
	for _, char := range candidate[1:] {
		if (char < '0' || char > '9') && (char < 'A' || char > 'F') {
			return fallback
		}
	}
	return candidate
}

// OverlayFontFamilies lists the font choices accepted for the overlay text.
var OverlayFontFamilies = []string{
	"system", "yahei", "segoe", "simsun", "simhei", "dengxian",
}

// NormalizeOverlayFontFamily returns a supported font key or the default.
func NormalizeOverlayFontFamily(value string) string {
	normalized := strings.ToLower(strings.TrimSpace(value))
	for _, key := range OverlayFontFamilies {
		if normalized == key {
			return key
		}
	}
	return Default.OverlayFontFamily
}

func ResolveBaseDir() string {
	// The single-exe package extracts its UI/engine into a runtime folder and
	// passes the real application directory down through the environment.
	if override := strings.TrimSpace(os.Getenv("POPTRANS_BASE_DIR")); override != "" {
		return filepath.Clean(override)
	}

	exePath, exeErr := os.Executable()
	if exeErr == nil {
		exeDir := filepath.Dir(exePath)
		if isPackagedRoot(exeDir) || isSingleExeRoot(exeDir) {
			return exeDir
		}
	}

	if cwd, err := os.Getwd(); err == nil {
		if dir := findProjectRoot(cwd); dir != "" {
			return dir
		}
	}

	if exeErr != nil {
		return "."
	}

	exeDir := filepath.Dir(exePath)
	if dir := findProjectRoot(exeDir); dir != "" {
		return dir
	}
	return exeDir
}

func Load(baseDir string) (Config, error) {
	cfg := Default
	path := filepath.Join(baseDir, "settings.json")

	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return cfg, nil
	}
	if err != nil {
		return cfg, err
	}

	rawKeys := map[string]json.RawMessage{}
	_ = json.Unmarshal(data, &rawKeys)

	if err := json.Unmarshal(data, &cfg); err != nil {
		return Default, err
	}

	mergeLoadedDefaults(&cfg, rawKeys)
	return cfg, nil
}

func Save(baseDir string, cfg Config) error {
	mergeDefaults(&cfg)
	if err := ValidateServerPort(cfg.ServerPort); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(baseDir, "settings.json"), data, 0o644)
}

func BackendURL(port int) string {
	if ValidateServerPort(port) != nil {
		port = DefaultServerPort
	}
	return fmt.Sprintf("http://127.0.0.1:%d", port)
}

func ValidateServerPort(port int) error {
	if port < MinServerPort || port > MaxServerPort {
		return fmt.Errorf("server port must be between %d and %d", MinServerPort, MaxServerPort)
	}
	return nil
}

func mergeDefaults(cfg *Config) {
	if cfg.Hotkey == "" {
		cfg.Hotkey = Default.Hotkey
	}
	if cfg.HotkeyDisplay == "" {
		cfg.HotkeyDisplay = Default.HotkeyDisplay
	}
	if cfg.OcrHotkey == "" {
		cfg.OcrHotkey = Default.OcrHotkey
	}
	if cfg.OcrHotkeyDisplay == "" {
		cfg.OcrHotkeyDisplay = Default.OcrHotkeyDisplay
	}
	if cfg.LiveHotkey == "" {
		cfg.LiveHotkey = Default.LiveHotkey
	}
	if cfg.LiveHotkeyDisplay == "" {
		cfg.LiveHotkeyDisplay = Default.LiveHotkeyDisplay
	}
	if cfg.ServerPort == 0 {
		cfg.ServerPort = Default.ServerPort
	}
	if cfg.Theme != "light" && cfg.Theme != "dark" && cfg.Theme != "system" {
		cfg.Theme = Default.Theme
	}
	if cfg.AccelerationDevice != "auto" && cfg.AccelerationDevice != "gpu" && cfg.AccelerationDevice != "cpu" {
		cfg.AccelerationDevice = Default.AccelerationDevice
	}
	cfg.ModelDownloadSource = NormalizeModelDownloadSource(cfg.ModelDownloadSource, cfg.ModelDownloadBaseURL)
	cfg.ModelDownloadBaseURL = NormalizeModelDownloadBaseURL(cfg.ModelDownloadBaseURL)
	cfg.SourceLanguage = NormalizeLanguage(cfg.SourceLanguage)
	cfg.TargetLanguage = NormalizeLanguage(cfg.TargetLanguage)
	cfg.OverlayFontFamily = NormalizeOverlayFontFamily(cfg.OverlayFontFamily)
	if cfg.OverlayFontWeight < 300 || cfg.OverlayFontWeight > 800 {
		cfg.OverlayFontWeight = Default.OverlayFontWeight
	}
	cfg.Models = NormalizeModels(cfg.Models)
	// The builtin model follows the global download-source preference, which
	// still offers a custom HF-style mirror for mainland networks.
	for index := range cfg.Models {
		if !cfg.Models[index].Builtin {
			continue
		}
		if cfg.ModelDownloadSource == ModelSourceModelScope {
			cfg.Models[index].Source = ModelSourceModelScope
		} else {
			cfg.Models[index].Source = ModelSourceHuggingFace
		}
	}
	cfg.ActiveModelID = NormalizeActiveModelID(cfg.ActiveModelID, cfg.Models)
	cfg.Glossary = NormalizeGlossary(cfg.Glossary)
	if cfg.AIThreads < 0 || cfg.AIThreads > 256 {
		cfg.AIThreads = 0
	}
	if cfg.UIIdleMinutes < 0 {
		cfg.UIIdleMinutes = Default.UIIdleMinutes
	}
	if cfg.AIIdleMinutes < 0 {
		cfg.AIIdleMinutes = Default.AIIdleMinutes
	}
}

func mergeLoadedDefaults(cfg *Config, rawKeys map[string]json.RawMessage) {
	mergeDefaults(cfg)
	if ValidateServerPort(cfg.ServerPort) != nil {
		cfg.ServerPort = Default.ServerPort
	}
	if _, ok := rawKeys["acceleration_device"]; !ok {
		cfg.AccelerationDevice = Default.AccelerationDevice
	}
	if _, ok := rawKeys["ui_idle_minutes"]; !ok {
		cfg.UIIdleMinutes = Default.UIIdleMinutes
	}
	if _, ok := rawKeys["ai_idle_minutes"]; !ok {
		cfg.AIIdleMinutes = Default.AIIdleMinutes
	}
	if cfg.UIIdleMinutes < 0 {
		cfg.UIIdleMinutes = Default.UIIdleMinutes
	}
	if cfg.AIIdleMinutes < 0 {
		cfg.AIIdleMinutes = Default.AIIdleMinutes
	}
}

// NormalizeModelDownloadSource validates the configured download source. An
// unknown value (including an old frontend sending an empty string) falls back
// to ModelScope. "custom" requires a usable http(s) base URL.
func NormalizeModelDownloadSource(source, baseURL string) string {
	source = strings.ToLower(strings.TrimSpace(source))
	switch source {
	case ModelSourceModelScope, ModelSourceHuggingFace:
		return source
	case ModelSourceCustom:
		if NormalizeModelDownloadBaseURL(baseURL) != "" {
			return source
		}
		return ModelSourceModelScope
	default:
		return ModelSourceModelScope
	}
}

// NormalizeModelDownloadBaseURL trims surrounding whitespace and rejects values
// that are not plain http(s) URLs. Empty/invalid values return "".
func NormalizeModelDownloadBaseURL(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}
	parsed, err := url.Parse(trimmed)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return ""
	}
	return strings.TrimRight(trimmed, "/")
}

// ModelInstalled reports whether the active model's GGUF is already present in
// baseDir. It mirrors backend/translator.py's path resolution so config changes
// that only affect downloads can avoid an unnecessary engine restart.
func ModelInstalled(baseDir string, cfg Config) bool {
	_, err := os.Stat(ModelPath(baseDir, ActiveModel(cfg)))
	return err == nil
}

// UIIdleTimeout returns how long the UI should stay alive after hide.
// A zero duration means the UI should not auto-exit.
func (c Config) UIIdleTimeout() time.Duration {
	if c.UIIdleMinutes < 0 {
		return time.Duration(DefaultUIIdleMinutes) * time.Minute
	}
	if c.UIIdleMinutes == 0 {
		return 0
	}
	return time.Duration(c.UIIdleMinutes) * time.Minute
}

// AIIdleTimeout returns how long the backend should stay alive after its last request.
// A zero duration means the backend should not auto-exit.
func (c Config) AIIdleTimeout() time.Duration {
	if c.AIKeepAlive {
		return 0
	}
	if c.AIIdleMinutes < 0 {
		return time.Duration(DefaultAIIdleMinutes) * time.Minute
	}
	if c.AIIdleMinutes == 0 {
		return 0
	}
	return time.Duration(c.AIIdleMinutes) * time.Minute
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func findProjectRoot(start string) string {
	dir := filepath.Clean(start)
	for range 6 {
		if isProjectRoot(dir) {
			return dir
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return ""
}

func isProjectRoot(dir string) bool {
	return isPackagedRoot(dir) ||
		(exists(filepath.Join(dir, "backend", "api_server.py")) &&
			exists(filepath.Join(dir, "backend", "requirements.txt")))
}

func isPackagedRoot(dir string) bool {
	return exists(filepath.Join(dir, "ai_engine.exe")) &&
		exists(filepath.Join(dir, "translate-ui.exe"))
}

// isSingleExeRoot recognises the bundled release layout, where only
// PopTrans.exe is visible and the UI/engine live in runtime/ after first run.
func isSingleExeRoot(dir string) bool {
	if !exists(filepath.Join(dir, "PopTrans.exe")) {
		return false
	}
	return exists(filepath.Join(dir, "models")) || exists(filepath.Join(dir, "runtime"))
}
