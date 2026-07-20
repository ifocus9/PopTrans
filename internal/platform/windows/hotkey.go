package windows

import (
	"fmt"
	"strconv"
	"strings"
)

type Hotkey struct {
	Modifiers uint32
	KeyCode   uint32
}

const (
	ModAlt     = 0x0001
	ModControl = 0x0002
	ModShift   = 0x0004
	ModWin     = 0x0008
)

func ParseHotkey(expr string) (Hotkey, error) {
	var hk Hotkey

	parts := strings.Split(strings.ToLower(strings.TrimSpace(expr)), "+")
	if len(parts) == 0 {
		return hk, fmt.Errorf("empty hotkey")
	}

	var mainKey string
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		switch part {
		case "<ctrl>", "ctrl":
			hk.Modifiers |= ModControl
		case "<alt>", "alt":
			hk.Modifiers |= ModAlt
		case "<shift>", "shift":
			hk.Modifiers |= ModShift
		case "<cmd>", "win":
			hk.Modifiers |= ModWin
		default:
			mainKey = strings.Trim(part, "<>")
		}
	}

	if hk.Modifiers == 0 {
		return hk, fmt.Errorf("hotkey must include modifiers: %s", expr)
	}
	if mainKey == "" {
		return hk, fmt.Errorf("hotkey missing key: %s", expr)
	}

	keyCode, err := parseVirtualKey(mainKey)
	if err != nil {
		return hk, err
	}
	hk.KeyCode = keyCode
	return hk, nil
}

func parseVirtualKey(key string) (uint32, error) {
	if len(key) == 1 {
		r := key[0]
		if r >= 'a' && r <= 'z' {
			return uint32(r - 32), nil
		}
		if r >= '0' && r <= '9' {
			return uint32(r), nil
		}
	}

	switch key {
	case "space":
		return 0x20, nil
	case "enter", "return":
		return 0x0D, nil
	case "tab":
		return 0x09, nil
	case "esc", "escape":
		return 0x1B, nil
	case "up":
		return 0x26, nil
	case "down":
		return 0x28, nil
	case "left":
		return 0x25, nil
	case "right":
		return 0x27, nil
	case "delete":
		return 0x2E, nil
	}

	if strings.HasPrefix(key, "f") {
		n, err := strconv.Atoi(strings.TrimPrefix(key, "f"))
		if err == nil && n >= 1 && n <= 24 {
			return uint32(0x70 + n - 1), nil
		}
	}

	return 0, fmt.Errorf("unsupported hotkey key: %s", key)
}
