package uidaemon

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const (
	EndpointHealth        = "/health"
	EndpointShowSettings  = "/show/settings"
	EndpointShowTranslate = "/show/translate"
	EndpointShowResult    = "/show/result"
	EndpointShowOverlay   = "/show/overlay"
	EndpointShowLive      = "/show/live"
	EndpointHide          = "/hide"
	EndpointShutdown      = "/shutdown"
	EndpointNotifyHidden  = "/notify/hidden"
	EndpointNotifyConfig  = "/notify/config-saved"

	// Callback endpoints posted by the UI process back to the host.
	EndpointNotifyHotkeyHold      = "/notify/hotkey-hold"
	EndpointNotifyOverlayClosed   = "/notify/overlay-closed"
	EndpointNotifyOverlayRefresh  = "/notify/overlay-refresh"
	EndpointNotifyLiveControl     = "/notify/live-control"

	headerToken = "X-PopTrans-Token"
)

// Endpoint is the on-disk discovery record written by the UI process.
type Endpoint struct {
	URL       string    `json:"url"`
	Token     string    `json:"token"`
	PID       int       `json:"pid"`
	StartedAt time.Time `json:"started_at"`
}

// Status describes the current UI presentation state.
type Status struct {
	Ready         bool   `json:"ready"`
	Mode          string `json:"mode"`
	Visible       bool   `json:"visible"`
	SettingsOpen  bool   `json:"settings_open"`
	HotkeysPaused bool   `json:"hotkeys_paused"`
}

// ResultPayload is pushed by the host to update the result window.
type ResultPayload struct {
	Source    string `json:"source"`
	Result    string `json:"result"`
	Error     string `json:"error"`
	Loading   bool   `json:"loading"`
	LatencyMs int64  `json:"latency_ms"`
}

// ConfigSavedPayload is sent from UI to host after settings are saved.
type ConfigSavedPayload struct {
	OK bool `json:"ok"`
}

// HotkeyHoldPayload temporarily suspends global hotkeys while the settings UI
// is recording a new combination.
type HotkeyHoldPayload struct {
	Paused bool `json:"paused"`
}

// LiveControlPayload tells the host live-translation loop what to do.
// Action is one of: "pause", "resume", "stop".
type LiveControlPayload struct {
	Action string `json:"action"`
}

// ScreenRect is a rectangle in physical screen pixels.
type ScreenRect struct {
	X      int `json:"x"`
	Y      int `json:"y"`
	Width  int `json:"width"`
	Height int `json:"height"`
}

// GlossaryHit marks a glossary term found in a block.
type GlossaryHit struct {
	Term        string `json:"term"`
	Translation string `json:"translation"`
	Source      string `json:"source"`
	Note        string `json:"note,omitempty"`
}

// OverlayStyle carries the user's translation text appearance so the separate
// overlay process does not need to poll settings. Text colour is chosen per
// block from the blurred background luminance.
type OverlayStyle struct {
	FontFamily string `json:"font_family"`
	FontWeight int    `json:"font_weight"`
}

// TranslatedBlock is one OCR line with its position in the captured image.
type TranslatedBlock struct {
	Text         string        `json:"text"`
	Translation  string        `json:"translation"`
	X            float64       `json:"x"`
	Y            float64       `json:"y"`
	W            float64       `json:"w"`
	H            float64       `json:"h"`
	Score        float64       `json:"score"`
	GlossaryHits []GlossaryHit `json:"glossary_hits,omitempty"`
	// Rotated line geometry from OCR; Angle is degrees (negative = rising to the
	// right), CX/CY is the line centre, LineWidth/LineHeight are edge lengths.
	Angle      float64 `json:"angle,omitempty"`
	CX         float64 `json:"cx,omitempty"`
	CY         float64 `json:"cy,omitempty"`
	LineWidth  float64 `json:"line_width,omitempty"`
	LineHeight float64 `json:"line_height,omitempty"`
	// SourceHeight is the original OCR line height, kept for font sizing even
	// when the visible box is clipped to avoid overlapping its neighbour.
	SourceHeight float64 `json:"source_h,omitempty"`
	// TextColor is auto-selected (#FFFFFF or #101010) from the blurred patch
	// luminance so the translation stays readable on any background.
	TextColor string `json:"text_color,omitempty"`
	// BackgroundImage is a data URL of the screen behind this line, blurred by
	// the host so translated text sits on frosted glass without hiding the
	// desktop elsewhere.
	BackgroundImage string `json:"background_image,omitempty"`
}

// OverlayPayload renders a captured screenshot with translated text on top.
// ImageDataURL is only sent on the first update; later updates keep the image.
type OverlayPayload struct {
	// Rect is the captured image area in physical screen pixels.
	Rect ScreenRect `json:"rect"`
	// WindowRect includes the toolbar strip and border outside the image.
	WindowRect ScreenRect `json:"window_rect"`
	// BarSide is "top" or "bottom": which side of the image holds the toolbar.
	BarSide      string            `json:"bar_side"`
	ImageWidth   int               `json:"image_width"`
	ImageHeight  int               `json:"image_height"`
	ImageDataURL string            `json:"image_data_url,omitempty"`
	Blocks       []TranslatedBlock `json:"blocks"`
	Source       string            `json:"source"`
	Translation  string            `json:"translation"`
	Loading      bool              `json:"loading"`
	Error        string            `json:"error"`
	Style        OverlayStyle      `json:"style"`
	// Stage/Progress/Lines drive the translation progress indicator:
	// "ocr" -> "translate" -> "done" ("error" on failure).
	Stage    string `json:"stage,omitempty"`
	Progress int    `json:"progress,omitempty"`
	Lines    int    `json:"lines,omitempty"`
	// LatencyMs is the OCR+translation round-trip for this screenshot.
	LatencyMs int64 `json:"latency_ms"`
}

// LivePayload updates the floating live-translation panel.
type LivePayload struct {
	// Anchor is the monitored region (image area) in physical screen pixels.
	Anchor ScreenRect `json:"anchor"`
	// WindowRect includes the toolbar strip and border outside the region.
	WindowRect   ScreenRect        `json:"window_rect"`
	BarSide      string            `json:"bar_side"`
	ImageDataURL string            `json:"image_data_url,omitempty"`
	ImageWidth   int               `json:"image_width"`
	ImageHeight  int               `json:"image_height"`
	Blocks       []TranslatedBlock `json:"blocks"`
	Source       string            `json:"source"`
	Translation  string            `json:"translation"`
	Status       string            `json:"status"`
	Error        string            `json:"error"`
	Style        OverlayStyle      `json:"style"`
	Version      int64             `json:"version"`
	// Stage/Progress/Lines drive the progress bar while a frame is recognized
	// and translated: "ocr" -> "translate" -> "done" ("error" on failure).
	Stage    string `json:"stage,omitempty"`
	Progress int    `json:"progress,omitempty"`
	Lines    int    `json:"lines,omitempty"`
	// LatencyMs is the OCR+translation round-trip for this live update.
	LatencyMs int64 `json:"latency_ms"`
}

func EndpointPath(baseDir string) string {
	return filepath.Join(baseDir, "translate-ui-endpoint.json")
}

func WriteEndpoint(baseDir string, endpoint Endpoint) error {
	path := EndpointPath(baseDir)
	data, err := json.MarshalIndent(endpoint, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func ReadEndpoint(baseDir string) (Endpoint, error) {
	path := EndpointPath(baseDir)
	data, err := os.ReadFile(path)
	if err != nil {
		return Endpoint{}, err
	}
	var endpoint Endpoint
	if err := json.Unmarshal(data, &endpoint); err != nil {
		return Endpoint{}, err
	}
	if endpoint.URL == "" || endpoint.Token == "" {
		return Endpoint{}, fmt.Errorf("invalid ui endpoint file")
	}
	return endpoint, nil
}

func RemoveEndpoint(baseDir string) {
	_ = os.Remove(EndpointPath(baseDir))
}

func NewToken() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

func TokenHeader() string {
	return headerToken
}
