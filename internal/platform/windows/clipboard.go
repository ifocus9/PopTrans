package windows

import (
	"fmt"
	"log"
	"time"
	"unsafe"

	xwindows "golang.org/x/sys/windows"
)

const (
	cfUnicodeText = 13
	gmemMoveable  = 0x0002
)

func ClipboardText() (string, error) {
	if err := openClipboard(); err != nil {
		return "", err
	}
	defer closeClipboard()

	handle, _, _ := procGetClipboardData.Call(uintptr(cfUnicodeText))
	if handle == 0 {
		return "", fmt.Errorf("clipboard has no unicode text")
	}

	ptr, _, _ := procGlobalLock.Call(handle)
	if ptr == 0 {
		return "", fmt.Errorf("global lock clipboard failed")
	}
	defer procGlobalUnlock.Call(handle)

	return xwindows.UTF16PtrToString((*uint16)(unsafe.Pointer(ptr))), nil
}

func SetClipboardText(text string) error {
	if err := openClipboard(); err != nil {
		return err
	}
	defer closeClipboard()

	if ret, _, err := procEmptyClipboard.Call(); ret == 0 {
		return fmt.Errorf("empty clipboard: %w", err)
	}

	utf16, err := xwindows.UTF16FromString(text)
	if err != nil {
		return err
	}

	size := uintptr(len(utf16) * 2)
	handle, _, err := procGlobalAlloc.Call(gmemMoveable, size)
	if handle == 0 {
		return fmt.Errorf("global alloc failed: %w", err)
	}

	ptr, _, _ := procGlobalLock.Call(handle)
	if ptr == 0 {
		return fmt.Errorf("global lock alloc failed")
	}

	dst := unsafe.Slice((*uint16)(unsafe.Pointer(ptr)), len(utf16))
	copy(dst, utf16)
	procGlobalUnlock.Call(handle)

	if ret, _, err := procSetClipboardData.Call(uintptr(cfUnicodeText), handle); ret == 0 {
		return fmt.Errorf("set clipboard data: %w", err)
	}

	return nil
}

func CaptureSelectedText() (string, error) {
	fgTitle, fgClass := ForegroundWindowInfo()
	log.Printf("capture native: foreground window title=%q class=%q", fgTitle, fgClass)
	fgHWND := ForegroundWindowHWND()

	original, originalErr := ClipboardText()
	hadOriginal := originalErr == nil
	sentinel := fmt.Sprintf("__translate_plugin_clipboard_sentinel_%d__", time.Now().UnixNano())
	log.Printf("capture native: original clipboard available=%t", hadOriginal)

	if err := SetClipboardText(sentinel); err != nil {
		log.Printf("capture native: failed to set sentinel clipboard: %v", err)
		return "", err
	}
	sequenceBeforeCopy := ClipboardSequenceNumber()
	log.Printf("capture native: sentinel set sequence=%d", sequenceBeforeCopy)

	time.Sleep(150 * time.Millisecond)

	// 在超时窗口内反复注入 Ctrl+C 并重试：浏览器（尤其 Chrome）偶发会丢弃一次
	// 合成按键，或某一瞬间焦点状态不对；多次重试可显著提升浏览器中捕获的成功率。
	deadline := time.Now().Add(1800 * time.Millisecond)
	lastSend := time.Time{}
	attempts := 0
	const maxAttempts = 4
	for time.Now().Before(deadline) {
		if attempts < maxAttempts && time.Since(lastSend) >= 150*time.Millisecond {
			sendCtrlC(fgHWND)
			attempts++
			lastSend = time.Now()
			log.Printf("capture native: ctrl+c sent (attempt %d)", attempts)
		}

		currentSequence := ClipboardSequenceNumber()
		if currentSequence != sequenceBeforeCopy {
			value, err := ClipboardText()
			if err == nil {
				log.Printf("capture native: clipboard updated sequence=%d preview=%q", currentSequence, previewClipboardText(value, 120))
				if value != sentinel {
					restoreClipboard(original, hadOriginal)
					return value, nil
				}
			} else {
				log.Printf("capture native: clipboard read failed after sequence change: %v", err)
			}
		}
		time.Sleep(50 * time.Millisecond)
	}

	log.Printf("capture native: clipboard did not change from sentinel within deadline (attempts=%d)", attempts)
	restoreClipboard(original, hadOriginal)
	return "", fmt.Errorf("no selected text detected")
}

func restoreClipboard(original string, hadOriginal bool) {
	if hadOriginal {
		_ = SetClipboardText(original)
		return
	}
	_ = SetClipboardText("")
}

func previewClipboardText(text string, limit int) string {
	if len(text) <= limit {
		return text
	}
	return text[:limit] + "..."
}
