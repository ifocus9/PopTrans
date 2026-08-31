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
	EndpointHide          = "/hide"
	EndpointShutdown      = "/shutdown"
	EndpointNotifyHidden  = "/notify/hidden"
	EndpointNotifyConfig  = "/notify/config-saved"

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
	Source  string `json:"source"`
	Result  string `json:"result"`
	Error   string `json:"error"`
	Loading bool   `json:"loading"`
}

// ConfigSavedPayload is sent from UI to host after settings are saved.
type ConfigSavedPayload struct {
	OK bool `json:"ok"`
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
