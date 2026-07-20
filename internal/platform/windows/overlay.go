package windows

import (
	"fmt"
	"unsafe"

	xwindows "golang.org/x/sys/windows"
)

const (
	SelectionTransparentColor = 0x00010101
	selectionAccentColor      = 0x00FF3D8B // COLORREF for #8B3DFF.
	selectionLabelWidth       = int32(112)
	selectionLabelHeight      = int32(24)
	selectionLabelGap         = int32(6)
	drawTextCenter            = 0x00000001
	drawTextVCenter           = 0x00000004
	drawTextSingleLine        = 0x00000020
	drawTextNoPrefix          = 0x00000800
)

var (
	procDrawTextW    = modUser32.NewProc("DrawTextW")
	procSetBkMode    = modGDI32.NewProc("SetBkMode")
	procSetTextColor = modGDI32.NewProc("SetTextColor")
)

type PaintStruct struct {
	HDC         uintptr
	Erase       int32
	RcPaint     Rect
	Restore     int32
	IncUpdate   int32
	RGBReserved [32]byte
}

func VirtualScreenRect() Rect {
	x, _, _ := procGetSystemMetrics.Call(SMXVirtualScreen)
	y, _, _ := procGetSystemMetrics.Call(SMYVirtualScreen)
	w, _, _ := procGetSystemMetrics.Call(SMCXVirtualScreen)
	h, _, _ := procGetSystemMetrics.Call(SMCYVirtualScreen)
	return Rect{
		Left:   int32(x),
		Top:    int32(y),
		Right:  int32(x) + int32(w),
		Bottom: int32(y) + int32(h),
	}
}

func SetLayeredWindowAlpha(hwnd HWND, alpha byte) error {
	ret, _, err := procSetLayeredWindowAttributes.Call(uintptr(hwnd), 0, uintptr(alpha), LwaAlpha)
	if ret == 0 {
		return fmt.Errorf("set layered alpha: %w", err)
	}
	return nil
}

func SetLayeredWindowAlphaWithColorKey(hwnd HWND, alpha byte, colorKey uint32) error {
	ret, _, err := procSetLayeredWindowAttributes.Call(
		uintptr(hwnd),
		uintptr(colorKey),
		uintptr(alpha),
		LwaAlpha|LwaColorKey,
	)
	if ret == 0 {
		return fmt.Errorf("set layered alpha and color key: %w", err)
	}
	return nil
}

func SetCapture(hwnd HWND) {
	procSetCapture.Call(uintptr(hwnd))
}

func ReleaseCapture() {
	procReleaseCapture.Call()
}

func InvalidateWindow(hwnd HWND) {
	procInvalidateRect.Call(uintptr(hwnd), 0, 0)
}

func PaintSelectionOverlay(hwnd HWND, width, height int32, selection Rect, hasSelection bool) uintptr {
	var ps PaintStruct
	hdc, _, _ := procBeginPaint.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&ps)))
	if hdc == 0 {
		return 0
	}
	defer procEndPaint.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&ps)))

	paintDC := hdc
	memoryDC, _, _ := procCreateCompatibleDC.Call(hdc)
	if memoryDC != 0 {
		bitmap, _, _ := procCreateCompatibleBitmap.Call(hdc, uintptr(width), uintptr(height))
		if bitmap != 0 {
			oldBitmap, _, _ := procSelectObject.Call(memoryDC, bitmap)
			if oldBitmap != 0 {
				paintDC = memoryDC
				defer func() {
					procBitBlt.Call(hdc, 0, 0, uintptr(width), uintptr(height), memoryDC, 0, 0, srccopy)
					procSelectObject.Call(memoryDC, oldBitmap)
					procDeleteObject.Call(bitmap)
					procDeleteDC.Call(memoryDC)
				}()
			} else {
				procDeleteObject.Call(bitmap)
				procDeleteDC.Call(memoryDC)
				memoryDC = 0
			}
		} else {
			procDeleteDC.Call(memoryDC)
			memoryDC = 0
		}
	}

	bgBrush := createSolidBrush(0x000000)
	if bgBrush != 0 {
		rect := Rect{Left: 0, Top: 0, Right: width, Bottom: height}
		procFillRect.Call(paintDC, uintptr(unsafe.Pointer(&rect)), bgBrush)
		deleteObject(bgBrush)
	}

	if hasSelection {
		transparentBrush := createSolidBrush(SelectionTransparentColor)
		if transparentBrush != 0 {
			rect := selection
			procFillRect.Call(paintDC, uintptr(unsafe.Pointer(&rect)), transparentBrush)
			deleteObject(transparentBrush)
		}

		brush := createSolidBrush(selectionAccentColor)
		if brush != 0 {
			rect := selection
			for i := int32(0); i < 3; i++ {
				frame := Rect{
					Left:   rect.Left - i,
					Top:    rect.Top - i,
					Right:  rect.Right + i,
					Bottom: rect.Bottom + i,
				}
				procFrameRect.Call(paintDC, uintptr(unsafe.Pointer(&frame)), brush)
			}
			deleteObject(brush)
		}

		drawSelectionSizeLabel(paintDC, width, height, selection)
	}

	return 0
}

func drawSelectionSizeLabel(hdc uintptr, overlayWidth, overlayHeight int32, selection Rect) {
	label := fmt.Sprintf("%d x %d", selection.Width(), selection.Height())
	labelUTF16, err := xwindows.UTF16FromString(label)
	if err != nil || len(labelUTF16) < 2 {
		return
	}

	left := selection.Left
	if left < 4 {
		left = 4
	}
	if left+selectionLabelWidth > overlayWidth-4 {
		left = overlayWidth - selectionLabelWidth - 4
	}

	top := selection.Bottom + selectionLabelGap
	if top+selectionLabelHeight > overlayHeight-4 {
		top = selection.Bottom - selectionLabelHeight - selectionLabelGap
	}
	if top < 4 {
		top = 4
	}

	labelRect := Rect{
		Left:   left,
		Top:    top,
		Right:  left + selectionLabelWidth,
		Bottom: top + selectionLabelHeight,
	}
	background := createSolidBrush(selectionAccentColor)
	if background == 0 {
		return
	}
	procFillRect.Call(hdc, uintptr(unsafe.Pointer(&labelRect)), background)
	deleteObject(background)

	procSetBkMode.Call(hdc, Transparent)
	procSetTextColor.Call(hdc, 0x00FFFFFF)
	procDrawTextW.Call(
		hdc,
		uintptr(unsafe.Pointer(&labelUTF16[0])),
		uintptr(len(labelUTF16)-1),
		uintptr(unsafe.Pointer(&labelRect)),
		drawTextCenter|drawTextVCenter|drawTextSingleLine|drawTextNoPrefix,
	)
}

func LParamPoint(lParam uintptr) Point {
	x := int32(int16(lParam & 0xFFFF))
	y := int32(int16((lParam >> 16) & 0xFFFF))
	return Point{X: x, Y: y}
}

func NormalizeRect(rect Rect) Rect {
	if rect.Left > rect.Right {
		rect.Left, rect.Right = rect.Right, rect.Left
	}
	if rect.Top > rect.Bottom {
		rect.Top, rect.Bottom = rect.Bottom, rect.Top
	}
	return rect
}

func (r Rect) Width() int32 {
	return r.Right - r.Left
}

func (r Rect) Height() int32 {
	return r.Bottom - r.Top
}

func createSolidBrush(color uint32) uintptr {
	ret, _, _ := procCreateSolidBrush.Call(uintptr(color))
	return ret
}

func deleteObject(handle uintptr) {
	procDeleteObject.Call(handle)
}
