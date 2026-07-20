package app

import (
	"fmt"
	"log"

	win "translate-plugin/internal/platform/windows"
)

type SelectionOverlay struct {
	hwnd       win.HWND
	origin     win.Point
	width      int32
	height     int32
	selecting  bool
	completed  bool
	start      win.Point
	current    win.Point
	onSelected func(win.Rect)
	onCancel   func()
}

func NewSelectionOverlay(onSelected func(win.Rect), onCancel func()) *SelectionOverlay {
	return &SelectionOverlay{
		onSelected: onSelected,
		onCancel:   onCancel,
	}
}

func (o *SelectionOverlay) Show(owner win.HWND) error {
	screen := win.VirtualScreenRect()
	o.origin = win.Point{X: screen.Left, Y: screen.Top}
	o.width = screen.Width()
	o.height = screen.Height()

	hwnd, err := win.CreateWindow(
		win.WsExTopmost|win.WsExToolWindow|win.WsExLayered,
		mainWindowClass,
		"OCR 框选",
		win.WSPopup|win.WSVisible,
		screen.Left,
		screen.Top,
		o.width,
		o.height,
		owner,
		0,
		0,
	)
	if err != nil {
		return err
	}
	o.hwnd = hwnd

	if err := win.SetLayeredWindowAlphaWithColorKey(hwnd, 92, win.SelectionTransparentColor); err != nil {
		win.DestroyWindow(hwnd)
		o.hwnd = 0
		return err
	}

	win.ShowWindow(hwnd, win.SWShow)
	win.SetForegroundWindow(hwnd)
	win.UpdateWindow(hwnd)
	return nil
}

func (o *SelectionOverlay) IsWindow(hwnd win.HWND) bool {
	return o != nil && o.hwnd != 0 && o.hwnd == hwnd
}

func (o *SelectionOverlay) HandleMessage(hwnd win.HWND, msg uint32, wParam, lParam uintptr) (uintptr, bool) {
	switch msg {
	case win.WmEraseBkgnd:
		return 1, true
	case win.WmPaint:
		return win.PaintSelectionOverlay(hwnd, o.width, o.height, o.selectionRect(), o.selecting), true
	case win.WmLButtonDown:
		pt := win.LParamPoint(lParam)
		o.start = pt
		o.current = pt
		o.selecting = true
		win.SetCapture(hwnd)
		win.InvalidateWindow(hwnd)
		return 0, true
	case win.WmMouseMove:
		if o.selecting {
			o.current = win.LParamPoint(lParam)
			win.InvalidateWindow(hwnd)
		}
		return 0, true
	case win.WmLButtonUp:
		if o.selecting {
			o.current = win.LParamPoint(lParam)
			o.selecting = false
			win.ReleaseCapture()
			o.finishSelection()
		}
		return 0, true
	case win.WmKeydown:
		if wParam == win.VkEscape {
			o.cancel()
			return 0, true
		}
	case win.WmClose:
		o.cancel()
		return 0, true
	case win.WmDestroy:
		o.hwnd = 0
		return 0, true
	}
	return 0, false
}

func (o *SelectionOverlay) selectionRect() win.Rect {
	return win.NormalizeRect(win.Rect{
		Left:   o.start.X,
		Top:    o.start.Y,
		Right:  o.current.X,
		Bottom: o.current.Y,
	})
}

func (o *SelectionOverlay) finishSelection() {
	if o.completed {
		return
	}
	o.completed = true

	relative := o.selectionRect()
	absolute := win.Rect{
		Left:   relative.Left + o.origin.X,
		Top:    relative.Top + o.origin.Y,
		Right:  relative.Right + o.origin.X,
		Bottom: relative.Bottom + o.origin.Y,
	}

	win.DestroyWindow(o.hwnd)
	if absolute.Width() < 5 || absolute.Height() < 5 {
		o.callCancel()
		return
	}

	log.Printf("ocr selection completed: %+v", absolute)
	if o.onSelected != nil {
		o.onSelected(absolute)
	}
}

func (o *SelectionOverlay) cancel() {
	if o.completed {
		return
	}
	o.completed = true
	if o.hwnd != 0 {
		win.DestroyWindow(o.hwnd)
	}
	o.callCancel()
}

func (o *SelectionOverlay) callCancel() {
	log.Printf("ocr selection canceled")
	if o.onCancel != nil {
		o.onCancel()
	}
}

func (o *SelectionOverlay) Close() {
	if o.hwnd != 0 {
		win.DestroyWindow(o.hwnd)
		o.hwnd = 0
	}
}

func (o *SelectionOverlay) String() string {
	return fmt.Sprintf("SelectionOverlay(hwnd=%d)", o.hwnd)
}
