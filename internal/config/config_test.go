package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestThemeDefaultsToSystem(t *testing.T) {
	cfg, err := Load(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Theme != "system" {
		t.Fatalf("theme = %q, want system", cfg.Theme)
	}
}

func TestThemeSettingRoundTrip(t *testing.T) {
	dir := t.TempDir()
	cfg := Default
	cfg.Theme = "light"
	if err := Save(dir, cfg); err != nil {
		t.Fatal(err)
	}

	loaded, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Theme != "light" {
		t.Fatalf("theme = %q, want light", loaded.Theme)
	}
}

func TestInvalidThemeFallsBackToSystem(t *testing.T) {
	dir := t.TempDir()
	cfg := Default
	cfg.Theme = "unknown"
	if err := Save(dir, cfg); err != nil {
		t.Fatal(err)
	}

	loaded, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Theme != "system" {
		t.Fatalf("theme = %q, want system", loaded.Theme)
	}
}

func TestLoggingDefaultsToDisabled(t *testing.T) {
	cfg, err := Load(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if cfg.LoggingEnabled {
		t.Fatal("logging should require explicit opt-in")
	}
}

func TestLoggingSettingRoundTrip(t *testing.T) {
	dir := t.TempDir()
	cfg := Default
	cfg.LoggingEnabled = true
	if err := Save(dir, cfg); err != nil {
		t.Fatal(err)
	}

	loaded, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !loaded.LoggingEnabled {
		t.Fatal("logging_enabled was not persisted")
	}
}

func TestServerPortDefaultsForLegacySettings(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(dir, "settings.json"),
		[]byte(`{"hotkey":"<ctrl>+<alt>+q","theme":"system"}`),
		0o644,
	); err != nil {
		t.Fatal(err)
	}

	loaded, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.ServerPort != DefaultServerPort {
		t.Fatalf("server port = %d, want %d", loaded.ServerPort, DefaultServerPort)
	}
}

func TestServerPortSettingRoundTrip(t *testing.T) {
	dir := t.TempDir()
	cfg := Default
	cfg.ServerPort = 19090
	if err := Save(dir, cfg); err != nil {
		t.Fatal(err)
	}

	loaded, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.ServerPort != 19090 {
		t.Fatalf("server port = %d, want 19090", loaded.ServerPort)
	}
	if got := BackendURL(loaded.ServerPort); got != "http://127.0.0.1:19090" {
		t.Fatalf("backend URL = %q", got)
	}
}

func TestInvalidPersistedServerPortFallsBackToDefault(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(dir, "settings.json"),
		[]byte(`{"server_port":80,"theme":"system"}`),
		0o644,
	); err != nil {
		t.Fatal(err)
	}

	loaded, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.ServerPort != DefaultServerPort {
		t.Fatalf("server port = %d, want %d", loaded.ServerPort, DefaultServerPort)
	}
}

func TestSaveRejectsInvalidServerPort(t *testing.T) {
	cfg := Default
	cfg.ServerPort = 70000
	err := Save(t.TempDir(), cfg)
	if err == nil || !strings.Contains(err.Error(), "1024") {
		t.Fatalf("expected port validation error, got %v", err)
	}
}

func TestUIIdleMinutesDefault(t *testing.T) {
	cfg, err := Load(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if cfg.UIIdleMinutes != DefaultUIIdleMinutes {
		t.Fatalf("ui idle minutes = %d, want %d", cfg.UIIdleMinutes, DefaultUIIdleMinutes)
	}
	if cfg.UIIdleTimeout() != time.Duration(DefaultUIIdleMinutes)*time.Minute {
		t.Fatalf("timeout = %s", cfg.UIIdleTimeout())
	}
}

func TestUIIdleMinutesZeroMeansNever(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(dir, "settings.json"),
		[]byte(`{"ui_idle_minutes":0}`),
		0o644,
	); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.UIIdleMinutes != 0 {
		t.Fatalf("ui idle minutes = %d, want 0", loaded.UIIdleMinutes)
	}
	if loaded.UIIdleTimeout() != 0 {
		t.Fatalf("timeout = %s, want 0", loaded.UIIdleTimeout())
	}
}

func TestUIIdleMinutesMissingUsesDefault(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(dir, "settings.json"),
		[]byte(`{"theme":"system"}`),
		0o644,
	); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.UIIdleMinutes != DefaultUIIdleMinutes {
		t.Fatalf("ui idle minutes = %d, want %d", loaded.UIIdleMinutes, DefaultUIIdleMinutes)
	}
}

func TestAIIdleMinutesDefault(t *testing.T) {
	cfg, err := Load(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if cfg.AIIdleMinutes != DefaultAIIdleMinutes {
		t.Fatalf("ai idle minutes = %d, want %d", cfg.AIIdleMinutes, DefaultAIIdleMinutes)
	}
	// The engine is resident by default now, which overrides the idle timeout.
	if !cfg.AIKeepAlive {
		t.Fatal("ai_keep_alive should default to true")
	}
	if cfg.AIIdleTimeout() != 0 {
		t.Fatalf("timeout = %s, want 0 while keep-alive is on", cfg.AIIdleTimeout())
	}
}

func TestAIIdleMinutesZeroMeansNever(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(dir, "settings.json"),
		[]byte(`{"ai_idle_minutes":0}`),
		0o644,
	); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.AIIdleMinutes != 0 {
		t.Fatalf("ai idle minutes = %d, want 0", loaded.AIIdleMinutes)
	}
	if loaded.AIIdleTimeout() != 0 {
		t.Fatalf("timeout = %s, want 0", loaded.AIIdleTimeout())
	}
}

func TestAIIdleMinutesMissingUsesDefault(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(dir, "settings.json"),
		[]byte(`{"theme":"system"}`),
		0o644,
	); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.AIIdleMinutes != DefaultAIIdleMinutes {
		t.Fatalf("ai idle minutes = %d, want %d", loaded.AIIdleMinutes, DefaultAIIdleMinutes)
	}
}

func TestAccelerationDeviceDefaultsToAuto(t *testing.T) {
	cfg, err := Load(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if cfg.AccelerationDevice != "auto" {
		t.Fatalf("acceleration device = %q, want auto", cfg.AccelerationDevice)
	}
}

func TestAccelerationDeviceRoundTrip(t *testing.T) {
	for _, dev := range []string{"auto", "gpu", "cpu"} {
		dir := t.TempDir()
		cfg := Default
		cfg.AccelerationDevice = dev
		if err := Save(dir, cfg); err != nil {
			t.Fatal(err)
		}
		loaded, err := Load(dir)
		if err != nil {
			t.Fatal(err)
		}
		if loaded.AccelerationDevice != dev {
			t.Fatalf("acceleration device = %q, want %q", loaded.AccelerationDevice, dev)
		}
	}
}

func TestInvalidAccelerationDeviceFallsBackToAuto(t *testing.T) {
	dir := t.TempDir()
	cfg := Default
	cfg.AccelerationDevice = "invalid_gpu"
	if err := Save(dir, cfg); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.AccelerationDevice != "auto" {
		t.Fatalf("acceleration device = %q, want auto", loaded.AccelerationDevice)
	}
}

func TestLaunchAtLoginDefaultsToDisabled(t *testing.T) {
	cfg, err := Load(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if cfg.LaunchAtLogin {
		t.Fatal("launch_at_login should default to false")
	}
}

func TestLaunchAtLoginRoundTrip(t *testing.T) {
	dir := t.TempDir()
	cfg := Default
	cfg.LaunchAtLogin = true
	if err := Save(dir, cfg); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !loaded.LaunchAtLogin {
		t.Fatal("launch_at_login was not persisted")
	}
}

func TestLaunchAtLoginMissingUsesDefault(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(dir, "settings.json"),
		[]byte(`{"theme":"system"}`),
		0o644,
	); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.LaunchAtLogin {
		t.Fatal("legacy settings should not enable launch_at_login")
	}
}

func TestLiveTranslationDefaults(t *testing.T) {
	cfg, err := Load(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.LiveEnabled {
		t.Fatal("live translation should be enabled by default")
	}
	if cfg.LiveHotkey != "<ctrl>+<alt>+r" || cfg.LiveHotkeyDisplay != "Ctrl+Alt+R" {
		t.Fatalf("live hotkey = %q / %q", cfg.LiveHotkey, cfg.LiveHotkeyDisplay)
	}
}

func TestAIKeepAliveDisabledUsesIdleMinutes(t *testing.T) {
	dir := t.TempDir()
	cfg := Default
	cfg.AIKeepAlive = false
	cfg.AIIdleMinutes = 7
	if err := Save(dir, cfg); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.AIIdleTimeout() != 7*time.Minute {
		t.Fatalf("idle timeout = %s, want 7m", loaded.AIIdleTimeout())
	}
}

func TestModelDownloadSourceDefaultsToModelScope(t *testing.T) {
	cfg, err := Load(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ModelDownloadSource != ModelSourceModelScope {
		t.Fatalf("download source = %q, want %q", cfg.ModelDownloadSource, ModelSourceModelScope)
	}
}

func TestModelDownloadSourceRoundTrip(t *testing.T) {
	dir := t.TempDir()
	cfg := Default
	cfg.ModelDownloadSource = ModelSourceCustom
	cfg.ModelDownloadBaseURL = "https://hf-mirror.com/"
	if err := Save(dir, cfg); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.ModelDownloadSource != ModelSourceCustom {
		t.Fatalf("download source = %q", loaded.ModelDownloadSource)
	}
	if loaded.ModelDownloadBaseURL != "https://hf-mirror.com" {
		t.Fatalf("download base url = %q", loaded.ModelDownloadBaseURL)
	}
}

func TestCustomDownloadSourceWithoutURLFallsBackToModelScope(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(dir, "settings.json"),
		[]byte(`{"model_download_source":"custom","model_download_base_url":"ftp://mirror"}`),
		0o644,
	); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.ModelDownloadSource != ModelSourceModelScope {
		t.Fatalf("download source = %q, want modelscope", loaded.ModelDownloadSource)
	}
	if loaded.ModelDownloadBaseURL != "" {
		t.Fatalf("invalid base url should be cleared, got %q", loaded.ModelDownloadBaseURL)
	}
}

func TestInvalidDownloadSourceFallsBackToModelScope(t *testing.T) {
	dir := t.TempDir()
	cfg := Default
	cfg.ModelDownloadSource = "unknown"
	if err := Save(dir, cfg); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.ModelDownloadSource != ModelSourceModelScope {
		t.Fatalf("download source = %q", loaded.ModelDownloadSource)
	}
}

func TestTranslationLanguageDefaults(t *testing.T) {
	cfg, err := Load(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if cfg.SourceLanguage != "auto" || cfg.TargetLanguage != "auto" {
		t.Fatalf("languages = %q/%q, want auto/auto", cfg.SourceLanguage, cfg.TargetLanguage)
	}
}

func TestTranslationLanguageRoundTrip(t *testing.T) {
	dir := t.TempDir()
	cfg := Default
	cfg.SourceLanguage = "en"
	cfg.TargetLanguage = "ja"
	if err := Save(dir, cfg); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.SourceLanguage != "en" || loaded.TargetLanguage != "ja" {
		t.Fatalf("languages = %q/%q", loaded.SourceLanguage, loaded.TargetLanguage)
	}
}

func TestUnsupportedLanguageFallsBackToAuto(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(dir, "settings.json"),
		[]byte(`{"source_language":"klingon","target_language":"xx"}`),
		0o644,
	); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.SourceLanguage != "auto" || loaded.TargetLanguage != "auto" {
		t.Fatalf("languages = %q/%q, want auto/auto", loaded.SourceLanguage, loaded.TargetLanguage)
	}
}

func TestOverlayStyleDefaults(t *testing.T) {
	cfg, err := Load(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if cfg.OverlayFontFamily != "system" || cfg.OverlayFontWeight != 500 {
		t.Fatalf("font = %q/%d", cfg.OverlayFontFamily, cfg.OverlayFontWeight)
	}
}

func TestAIThreadsDefaultsToAuto(t *testing.T) {
	cfg, err := Load(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if cfg.AIThreads != 0 {
		t.Fatalf("ai_threads = %d, want 0 (auto)", cfg.AIThreads)
	}
}

func TestAIThreadsRoundTripAndValidation(t *testing.T) {
	dir := t.TempDir()
	cfg := Default
	cfg.AIThreads = 8
	if err := Save(dir, cfg); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.AIThreads != 8 {
		t.Fatalf("ai_threads = %d, want 8", loaded.AIThreads)
	}

	badDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(badDir, "settings.json"), []byte(`{"ai_threads":9999}`), 0o644); err != nil {
		t.Fatal(err)
	}
	bad, err := Load(badDir)
	if err != nil {
		t.Fatal(err)
	}
	if bad.AIThreads != 0 {
		t.Fatalf("invalid ai_threads = %d, want 0", bad.AIThreads)
	}
}

func TestGlossaryNormalization(t *testing.T) {
	dir := t.TempDir()
	cfg := Default
	cfg.Glossary = []GlossaryEntry{
		{Term: "  GPU  ", Translation: " 图形处理器 ", Source: " 显卡手册 "},
		{Term: "", Translation: "无效", Source: ""},
		{Term: "CPU", Translation: "", Source: ""},
		{Term: "gpu", Translation: "显卡", Source: "覆盖"},
		{Term: "LLM", Translation: "大语言模型", Source: ""},
	}
	if err := Save(dir, cfg); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Glossary) != 2 {
		t.Fatalf("glossary entries = %d, want 2: %+v", len(loaded.Glossary), loaded.Glossary)
	}
	if loaded.Glossary[0].Term != "gpu" || loaded.Glossary[0].Translation != "显卡" {
		t.Fatalf("duplicate term should keep the last entry: %+v", loaded.Glossary[0])
	}
	if loaded.Glossary[1].Term != "LLM" || loaded.Glossary[1].Translation != "大语言模型" {
		t.Fatalf("second entry = %+v", loaded.Glossary[1])
	}
}

func TestGlossaryRoundTrip(t *testing.T) {
	dir := t.TempDir()
	cfg := Default
	cfg.Glossary = []GlossaryEntry{
		{Term: "PopTrans", Translation: "弹出翻译", Source: "项目词汇"},
	}
	if err := Save(dir, cfg); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Glossary) != 1 || loaded.Glossary[0].Source != "项目词汇" {
		t.Fatalf("glossary = %+v", loaded.Glossary)
	}
}

func TestOverlayStyleValidation(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(dir, "settings.json"),
		[]byte(`{"overlay_text_color":"#12345","overlay_font_family":"comic","overlay_font_weight":950,"overlay_background_color":"#GGGGGG","overlay_background_opacity":-5,"overlay_font_scale":999}`),
		0o644,
	); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.OverlayFontFamily != "system" || loaded.OverlayFontWeight != 500 {
		t.Fatalf("font = %q/%d", loaded.OverlayFontFamily, loaded.OverlayFontWeight)
	}
}

func TestModelCatalogueNormalization(t *testing.T) {
	entries := []ModelEntry{
		{ID: "a", Name: "A", Source: "huggingface", Repo: "Qwen/Qwen2.5-0.5B-Instruct-GGUF", File: "qwen.gguf"},
		{ID: "a", Name: "dup", Source: "bogus", Repo: "../evil", File: "x.gguf"},
		{Name: "B", Repo: "Tencent-Hunyuan/Hy-MT2-30B-A3B-GGUF", File: "Hy-MT2-30B-A3B-Q4_K_M.gguf"},
		{Name: "bad file", Repo: "ok/repo", File: "notes.txt"},
	}
	models := NormalizeModels(entries)
	if len(models) != 3 {
		t.Fatalf("models = %d, want 3 (%+v)", len(models), models)
	}
	if models[0].ID != BuiltinModelID {
		t.Fatalf("first model = %q, want builtin", models[0].ID)
	}
	ids := map[string]bool{}
	for _, entry := range models {
		if entry.ID == "" || ids[entry.ID] {
			t.Fatalf("invalid or duplicate id %q in %+v", entry.ID, models)
		}
		ids[entry.ID] = true
		if strings.Contains(entry.Repo, "..") || entry.File == "notes.txt" {
			t.Fatalf("unsafe entry survived: %+v", entry)
		}
		if entry.Dir == "" {
			t.Fatalf("empty dir for %+v", entry)
		}
	}
	if models[1].ID != "a" || models[1].Source != "huggingface" {
		t.Fatalf("first user model = %+v", models[1])
	}
	if active := NormalizeActiveModelID("missing", models); active != BuiltinModelID {
		t.Fatalf("fallback active = %q", active)
	}
	if active := NormalizeActiveModelID("a", models); active != "a" {
		t.Fatalf("explicit active = %q", active)
	}
}

func TestModelInstalledUsesActiveModel(t *testing.T) {
	dir := t.TempDir()
	cfg := Default
	path := ModelPath(dir, BuiltinModel())
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !ModelInstalled(dir, cfg) {
		t.Fatalf("builtin model should be installed at %s", path)
	}

	cfg.Models = append(cfg.Models, ModelEntry{
		ID:     "other",
		Name:   "Other",
		Source: ModelSourceModelScope,
		Repo:   "Qwen/Qwen2.5-0.5B-Instruct-GGUF",
		File:   "qwen.gguf",
	})
	cfg.ActiveModelID = "other"
	if ModelInstalled(dir, cfg) {
		t.Fatal("other model should not be installed")
	}
}

func TestModelFileRawAndLocalPaths(t *testing.T) {
	raw := "Q4_K_M/model + extra.gguf"
	if got := NormalizeModelFile(raw); got != raw {
		t.Fatalf("raw file = %q, want %q", got, raw)
	}
	local := LocalModelFile(raw)
	if strings.ContainsAny(local, `<>:"|?*`) {
		t.Fatalf("local path still has Windows-invalid characters: %q", local)
	}
	if !strings.HasSuffix(local, ".gguf") {
		t.Fatalf("local path lost the extension: %q", local)
	}

	for _, bad := range []string{"../x.gguf", "a/../../x.gguf", "C:/x.gguf", "x.txt", ""} {
		if got := NormalizeModelFile(bad); got != "" {
			t.Fatalf("unsafe file %q accepted as %q", bad, got)
		}
	}
}
