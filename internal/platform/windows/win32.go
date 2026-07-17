package windows

import (
	"fmt"
	"log"
	"syscall"
	"time"
	"unsafe"

	xwindows "golang.org/x/sys/windows"
)

type HWND = xwindows.Handle

type Point struct {
	X int32
	Y int32
}

type Rect struct {
	Left   int32
	Top    int32
	Right  int32
	Bottom int32
}

type MonitorInfo struct {
	Size    uint32
	Monitor Rect
	Work    Rect
	Flags   uint32
}

type Msg struct {
	HWnd     HWND
	Message  uint32
	WParam   uintptr
	LParam   uintptr
	Time     uint32
	Pt       Point
	LPrivate uint32
}

type WndClassEx struct {
	Size       uint32
	Style      uint32
	WndProc    uintptr
	ClsExtra   int32
	WndExtra   int32
	Instance   HWND
	Icon       HWND
	Cursor     HWND
	Background HWND
	MenuName   *uint16
	ClassName  *uint16
	IconSm     HWND
}

type NotifyIconData struct {
	CbSize           uint32
	HWnd             HWND
	UID              uint32
	UFlags           uint32
	UCallbackMessage uint32
	HIcon            HWND
	SzTip            [128]uint16
	DwState          uint32
	DwStateMask      uint32
	SzInfo           [256]uint16
	UVersion         uint32
	SzInfoTitle      [64]uint16
	DwInfoFlags      uint32
	GuidItem         syscall.GUID
	HBalloonIcon     HWND
}

type KeybdInput struct {
	WVk         uint16
	WScan       uint16
	DwFlags     uint32
	Time        uint32
	DwExtraInfo uintptr
}

type Input struct {
	Type uint32
	_    uint32
	Ki   KeybdInput
	_    [8]byte
}

const (
	CWUseDefault     = 0x80000000
	SWHide           = 0
	SWShowNoActivate = 4
	SWShow           = 5
	SWShownormal     = 1
	SWPNoSize        = 0x0001
	SWPNoMove        = 0x0002
	SWPShowWindow    = 0x0040

	WSOverlapped = 0x00000000
	WSPopup      = 0x80000000
	WSCaption    = 0x00C00000
	WSSysMenu    = 0x00080000
	WSVisible    = 0x10000000
	WSChild      = 0x40000000
	WSVScroll    = 0x00200000
	WSTabStop    = 0x00010000
	WSBorder     = 0x00800000
	WSDisabled   = 0x08000000

	WsExClientEdge = 0x00000200
	WsExToolWindow = 0x00000080
	WsExTopmost    = 0x00000008
	WsExLayered    = 0x00080000

	ESMultiline   = 0x0004
	ESAutoVScroll = 0x0040
	ESReadonly    = 0x0800

	BSCheckbox = 0x00000002
	BMSetCheck = 0x00F1
	BMGetCheck = 0x00F0
	BSTChecked = 0x0001

	MFString    = 0x00000000
	MFSeparator = 0x00000800
	MFDisabled  = 0x00000002
	MFGrayED    = 0x00000001

	TPMRightButton = 0x0002
	TPMBottomAlign = 0x0020
	TPMLeftAlign   = 0x0000

	NIMAdd     = 0x00000000
	NIMModify  = 0x00000001
	NIMDelete  = 0x00000002
	NIFMessage = 0x00000001
	NIFIcon    = 0x00000002
	NIFTip     = 0x00000004

	ImageIcon      = 1
	LrLoadFromFile = 0x00000010
	LrDefaultSize  = 0x00000040

	WmApp         = 0x8000
	WmCommand     = 0x0111
	WmDestroy     = 0x0002
	WmClose       = 0x0010
	WmEraseBkgnd  = 0x0014
	WmPaint       = 0x000F
	WmKeydown     = 0x0100
	WmMouseMove   = 0x0200
	WmLButtonDown = 0x0201
	WmLButtonUp   = 0x0202
	WmSize        = 0x0005
	WmRButtonUp   = 0x0205
	WmHotkey      = 0x0312

	SMCxScreen        = 0
	SMCyScreen        = 1
	SMXVirtualScreen  = 76
	SMYVirtualScreen  = 77
	SMCXVirtualScreen = 78
	SMCYVirtualScreen = 79

	VkControl         = 0x11
	VkLControl        = 0xA2
	VkMenu            = 0x12
	VkLMenu           = 0xA4
	VkShift           = 0x10
	VkLShift          = 0xA0
	VkC               = 0x43
	VkEscape          = 0x1B
	VkLWin            = 0x5B
	VkRWin            = 0x5C
	KeyeventfKeyup    = 0x0002
	KeyeventfScancode = 0x0008
	InputKeyboard     = 1

	GwlpUserData = -21

	IdiApplication = 32512
	IdcArrow       = 32512

	LwaColorKey = 0x00000001
	LwaAlpha    = 0x00000002
	Transparent = 1

	monitorDefaultToNearest = 2
)

var (
	modUser32   = xwindows.NewLazySystemDLL("user32.dll")
	modShcore   = xwindows.NewLazySystemDLL("shcore.dll")
	modShell32  = xwindows.NewLazySystemDLL("shell32.dll")
	modKernel32 = xwindows.NewLazySystemDLL("kernel32.dll")

	procRegisterClassExW              = modUser32.NewProc("RegisterClassExW")
	procCreateWindowExW               = modUser32.NewProc("CreateWindowExW")
	procDefWindowProcW                = modUser32.NewProc("DefWindowProcW")
	procDestroyWindow                 = modUser32.NewProc("DestroyWindow")
	procShowWindow                    = modUser32.NewProc("ShowWindow")
	procUpdateWindow                  = modUser32.NewProc("UpdateWindow")
	procGetMessageW                   = modUser32.NewProc("GetMessageW")
	procTranslateMessage              = modUser32.NewProc("TranslateMessage")
	procDispatchMessageW              = modUser32.NewProc("DispatchMessageW")
	procPostQuitMessage               = modUser32.NewProc("PostQuitMessage")
	procPostMessageW                  = modUser32.NewProc("PostMessageW")
	procRegisterHotKey                = modUser32.NewProc("RegisterHotKey")
	procUnregisterHotKey              = modUser32.NewProc("UnregisterHotKey")
	procLoadImageW                    = modUser32.NewProc("LoadImageW")
	procLoadIconW                     = modUser32.NewProc("LoadIconW")
	procLoadCursorW                   = modUser32.NewProc("LoadCursorW")
	procMapVirtualKeyW                = modUser32.NewProc("MapVirtualKeyW")
	procShellNotifyIconW              = modShell32.NewProc("Shell_NotifyIconW")
	procCreatePopupMenu               = modUser32.NewProc("CreatePopupMenu")
	procAppendMenuW                   = modUser32.NewProc("AppendMenuW")
	procTrackPopupMenu                = modUser32.NewProc("TrackPopupMenu")
	procSetForegroundWindow           = modUser32.NewProc("SetForegroundWindow")
	procGetCursorPos                  = modUser32.NewProc("GetCursorPos")
	procGetSystemMetrics              = modUser32.NewProc("GetSystemMetrics")
	procMonitorFromRect               = modUser32.NewProc("MonitorFromRect")
	procGetMonitorInfoW               = modUser32.NewProc("GetMonitorInfoW")
	procFindWindowW                   = modUser32.NewProc("FindWindowW")
	procGetWindowRect                 = modUser32.NewProc("GetWindowRect")
	procGetDpiForWindow               = modUser32.NewProc("GetDpiForWindow")
	procSetWindowPos                  = modUser32.NewProc("SetWindowPos")
	procMoveWindow                    = modUser32.NewProc("MoveWindow")
	procCreateWindowTextW             = modUser32.NewProc("SetWindowTextW")
	procSetWindowLongPtrW             = modUser32.NewProc("SetWindowLongPtrW")
	procGetWindowLongPtrW             = modUser32.NewProc("GetWindowLongPtrW")
	procSetLayeredWindowAttributes    = modUser32.NewProc("SetLayeredWindowAttributes")
	procMessageBoxW                   = modUser32.NewProc("MessageBoxW")
	procOpenClipboard                 = modUser32.NewProc("OpenClipboard")
	procCloseClipboard                = modUser32.NewProc("CloseClipboard")
	procGetClipboardData              = modUser32.NewProc("GetClipboardData")
	procGetClipboardSeqNum            = modUser32.NewProc("GetClipboardSequenceNumber")
	procSetClipboardData              = modUser32.NewProc("SetClipboardData")
	procEmptyClipboard                = modUser32.NewProc("EmptyClipboard")
	procGlobalAlloc                   = modKernel32.NewProc("GlobalAlloc")
	procGlobalLock                    = modKernel32.NewProc("GlobalLock")
	procGlobalUnlock                  = modKernel32.NewProc("GlobalUnlock")
	procGetModuleHandleW              = modKernel32.NewProc("GetModuleHandleW")
	procSendInput                     = modUser32.NewProc("SendInput")
	procGetForegroundWindow           = modUser32.NewProc("GetForegroundWindow")
	procGetWindowTextW                = modUser32.NewProc("GetWindowTextW")
	procGetWindowTextLengthW          = modUser32.NewProc("GetWindowTextLengthW")
	procGetClassNameW                 = modUser32.NewProc("GetClassNameW")
	procSendMessageW                  = modUser32.NewProc("SendMessageW")
	procGetAsyncKeyState              = modUser32.NewProc("GetAsyncKeyState")
	procAttachThreadInput             = modUser32.NewProc("AttachThreadInput")
	procGetWindowThreadProcessId      = modUser32.NewProc("GetWindowThreadProcessId")
	procSetFocus                      = modUser32.NewProc("SetFocus")
	procGetFocus                      = modUser32.NewProc("GetFocus")
	procSetProcessDpiAwarenessContext = modUser32.NewProc("SetProcessDpiAwarenessContext")
	procSetProcessDpiAwareness        = modShcore.NewProc("SetProcessDpiAwareness")
	procSetCapture                    = modUser32.NewProc("SetCapture")
	procReleaseCapture                = modUser32.NewProc("ReleaseCapture")
	procInvalidateRect                = modUser32.NewProc("InvalidateRect")
	procBeginPaint                    = modUser32.NewProc("BeginPaint")
	procEndPaint                      = modUser32.NewProc("EndPaint")
	procFillRect                      = modUser32.NewProc("FillRect")
	procFrameRect                     = modUser32.NewProc("FrameRect")
	procGetCurrentThreadId            = modKernel32.NewProc("GetCurrentThreadId")
)

func RegisterClass(className string, wndProc uintptr) error {
	instance, err := currentModuleHandle()
	if err != nil {
		return err
	}

	classNamePtr, err := xwindows.UTF16PtrFromString(className)
	if err != nil {
		return err
	}

	cursor, _, _ := procLoadCursorW.Call(0, IdiApplication)
	arrow, _, _ := procLoadCursorW.Call(0, IdcArrow)
	if arrow != 0 {
		cursor = arrow
	}

	wc := WndClassEx{
		Size:      uint32(unsafe.Sizeof(WndClassEx{})),
		WndProc:   wndProc,
		Instance:  instance,
		Cursor:    HWND(cursor),
		ClassName: classNamePtr,
	}

	ret, _, callErr := procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))
	if ret == 0 && callErr != syscall.Errno(1410) {
		return fmt.Errorf("register class %s: %w", className, callErr)
	}
	return nil
}

func CreateWindow(exStyle uint32, className, title string, style uint32, x, y, width, height int32, parent HWND, menu uintptr, param uintptr) (HWND, error) {
	instance, err := currentModuleHandle()
	if err != nil {
		return 0, err
	}
	classPtr, err := xwindows.UTF16PtrFromString(className)
	if err != nil {
		return 0, err
	}
	titlePtr, err := xwindows.UTF16PtrFromString(title)
	if err != nil {
		return 0, err
	}

	ret, _, callErr := procCreateWindowExW.Call(
		uintptr(exStyle),
		uintptr(unsafe.Pointer(classPtr)),
		uintptr(unsafe.Pointer(titlePtr)),
		uintptr(style),
		uintptr(x),
		uintptr(y),
		uintptr(width),
		uintptr(height),
		uintptr(parent),
		menu,
		uintptr(instance),
		param,
	)
	if ret == 0 {
		return 0, fmt.Errorf("create window %s: %w", className, callErr)
	}
	return HWND(ret), nil
}

func DefWindowProc(hwnd HWND, msg uint32, wParam, lParam uintptr) uintptr {
	ret, _, _ := procDefWindowProcW.Call(uintptr(hwnd), uintptr(msg), wParam, lParam)
	return ret
}

func ShowWindow(hwnd HWND, cmd int32) {
	procShowWindow.Call(uintptr(hwnd), uintptr(cmd))
}

func UpdateWindow(hwnd HWND) {
	procUpdateWindow.Call(uintptr(hwnd))
}

func DestroyWindow(hwnd HWND) {
	procDestroyWindow.Call(uintptr(hwnd))
}

func PostQuitMessage(code int32) {
	procPostQuitMessage.Call(uintptr(code))
}

func PostMessage(hwnd HWND, msg uint32, wParam, lParam uintptr) {
	procPostMessageW.Call(uintptr(hwnd), uintptr(msg), wParam, lParam)
}

func MessageLoop() (int32, error) {
	var msg Msg
	for {
		ret, _, err := procGetMessageW.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
		switch int32(ret) {
		case -1:
			return -1, err
		case 0:
			return int32(msg.WParam), nil
		default:
			procTranslateMessage.Call(uintptr(unsafe.Pointer(&msg)))
			procDispatchMessageW.Call(uintptr(unsafe.Pointer(&msg)))
		}
	}
}

func EnableDPIAwareness() {
	const perMonitorAware = 2

	// DPI_AWARENESS_CONTEXT_PER_MONITOR_AWARE_V2 is documented as ((HANDLE)-4).
	if ret, _, _ := procSetProcessDpiAwarenessContext.Call(^uintptr(3)); ret != 0 {
		return
	}

	procSetProcessDpiAwareness.Call(perMonitorAware)
}

func RegisterHotKey(hwnd HWND, id int32, hotkey Hotkey) error {
	ret, _, err := procRegisterHotKey.Call(uintptr(hwnd), uintptr(id), uintptr(hotkey.Modifiers), uintptr(hotkey.KeyCode))
	if ret == 0 {
		return fmt.Errorf("register hotkey id=%d: %w", id, err)
	}
	return nil
}

func UnregisterHotKey(hwnd HWND, id int32) {
	procUnregisterHotKey.Call(uintptr(hwnd), uintptr(id))
}

func LoadTrayIcon(iconPath string) HWND {
	if iconPath != "" {
		pathPtr, err := xwindows.UTF16PtrFromString(iconPath)
		if err == nil {
			ret, _, _ := procLoadImageW.Call(0, uintptr(unsafe.Pointer(pathPtr)), ImageIcon, 0, 0, LrLoadFromFile|LrDefaultSize)
			if ret != 0 {
				return HWND(ret)
			}
		}
	}

	ret, _, _ := procLoadIconW.Call(0, IdiApplication)
	return HWND(ret)
}

func AddTrayIcon(hwnd HWND, id uint32, icon HWND, tip string, callbackMessage uint32) error {
	var nid NotifyIconData
	nid.CbSize = uint32(unsafe.Sizeof(nid))
	nid.HWnd = hwnd
	nid.UID = id
	nid.UFlags = NIFMessage | NIFIcon | NIFTip
	nid.UCallbackMessage = callbackMessage
	nid.HIcon = icon
	copyWideString(nid.SzTip[:], tip)

	ret, _, err := procShellNotifyIconW.Call(NIMAdd, uintptr(unsafe.Pointer(&nid)))
	if ret == 0 {
		return fmt.Errorf("tray add: %w", err)
	}
	return nil
}

func ModifyTrayIcon(hwnd HWND, id uint32, icon HWND, tip string, callbackMessage uint32) error {
	var nid NotifyIconData
	nid.CbSize = uint32(unsafe.Sizeof(nid))
	nid.HWnd = hwnd
	nid.UID = id
	nid.UFlags = NIFMessage | NIFIcon | NIFTip
	nid.UCallbackMessage = callbackMessage
	nid.HIcon = icon
	copyWideString(nid.SzTip[:], tip)

	ret, _, err := procShellNotifyIconW.Call(NIMModify, uintptr(unsafe.Pointer(&nid)))
	if ret == 0 {
		return fmt.Errorf("tray modify: %w", err)
	}
	return nil
}

func DeleteTrayIcon(hwnd HWND, id uint32) {
	var nid NotifyIconData
	nid.CbSize = uint32(unsafe.Sizeof(nid))
	nid.HWnd = hwnd
	nid.UID = id
	procShellNotifyIconW.Call(NIMDelete, uintptr(unsafe.Pointer(&nid)))
}

func ShowTrayMenu(hwnd HWND, items []MenuItem) {
	menu, _, _ := procCreatePopupMenu.Call()
	if menu == 0 {
		return
	}

	for _, item := range items {
		if item.Separator {
			procAppendMenuW.Call(menu, MFSeparator, 0, 0)
			continue
		}

		flags := uintptr(MFString)
		if item.Disabled {
			flags |= MFDisabled | MFGrayED
		}

		textPtr, _ := xwindows.UTF16PtrFromString(item.Text)
		procAppendMenuW.Call(menu, flags, uintptr(item.ID), uintptr(unsafe.Pointer(textPtr)))
	}

	var pt Point
	procGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
	procSetForegroundWindow.Call(uintptr(hwnd))
	procTrackPopupMenu.Call(menu, TPMRightButton|TPMBottomAlign|TPMLeftAlign, uintptr(pt.X), uintptr(pt.Y), 0, uintptr(hwnd), 0)
}

type MenuItem struct {
	ID        uint16
	Text      string
	Disabled  bool
	Separator bool
}

func SetWindowPosTopMost(hwnd HWND, x, y, width, height int32) {
	procSetWindowPos.Call(uintptr(hwnd), ^uintptr(1), uintptr(x), uintptr(y), uintptr(width), uintptr(height), SWPShowWindow)
}

func MoveWindow(hwnd HWND, x, y, width, height int32, repaint bool) {
	repaintValue := uintptr(0)
	if repaint {
		repaintValue = 1
	}
	procMoveWindow.Call(uintptr(hwnd), uintptr(x), uintptr(y), uintptr(width), uintptr(height), repaintValue)
}

func SetWindowText(hwnd HWND, text string) {
	textPtr, _ := xwindows.UTF16PtrFromString(text)
	procCreateWindowTextW.Call(uintptr(hwnd), uintptr(unsafe.Pointer(textPtr)))
}

func GetWindowText(hwnd HWND) string {
	length, _, _ := procGetWindowTextLengthW.Call(uintptr(hwnd))
	if length == 0 {
		return ""
	}
	buf := make([]uint16, length+1)
	n, _, _ := procGetWindowTextW.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	return xwindows.UTF16ToString(buf[:n])
}

func SetCheck(hwnd HWND, checked bool) {
	value := uintptr(0)
	if checked {
		value = BSTChecked
	}
	procSendMessageW.Call(uintptr(hwnd), BMSetCheck, value, 0)
}

func IsChecked(hwnd HWND) bool {
	ret, _, _ := procSendMessageW.Call(uintptr(hwnd), BMGetCheck, 0, 0)
	return ret == BSTChecked
}

func SetForegroundWindow(hwnd HWND) {
	procSetForegroundWindow.Call(uintptr(hwnd))
}

func MessageBox(hwnd HWND, text, title string) {
	textPtr, _ := xwindows.UTF16PtrFromString(text)
	titlePtr, _ := xwindows.UTF16PtrFromString(title)
	procMessageBoxW.Call(uintptr(hwnd), uintptr(unsafe.Pointer(textPtr)), uintptr(unsafe.Pointer(titlePtr)), 0)
}

func GetCursorPos() Point {
	var pt Point
	procGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
	return pt
}

func ScreenSize() (int32, int32) {
	width, _, _ := procGetSystemMetrics.Call(SMCxScreen)
	height, _, _ := procGetSystemMetrics.Call(SMCyScreen)
	return int32(width), int32(height)
}

func WorkAreaForPoint(point Point) Rect {
	pointRect := Rect{Left: point.X, Top: point.Y, Right: point.X + 1, Bottom: point.Y + 1}
	monitor, _, _ := procMonitorFromRect.Call(uintptr(unsafe.Pointer(&pointRect)), monitorDefaultToNearest)
	if monitor != 0 {
		info := MonitorInfo{Size: uint32(unsafe.Sizeof(MonitorInfo{}))}
		if ok, _, _ := procGetMonitorInfoW.Call(monitor, uintptr(unsafe.Pointer(&info))); ok != 0 {
			return info.Work
		}
	}

	width, height := ScreenSize()
	return Rect{Right: width, Bottom: height}
}

func openClipboard() error {
	for range 10 {
		ret, _, err := procOpenClipboard.Call(0)
		if ret != 0 {
			return nil
		}
		_ = err
		time.Sleep(20 * time.Millisecond)
	}
	return fmt.Errorf("open clipboard failed")
}

func closeClipboard() {
	procCloseClipboard.Call()
}

// ForegroundWindowInfo 返回当前前台窗口的标题与类名。
// 用于诊断按键注入：Ctrl+C 会被发往前台窗口，若这里不是用户选中文本的应用
// （而是本程序的弹窗/托盘或某个受保护窗口），即可解释为何捕获不到选中文本。
func ForegroundWindowInfo() (title, className string) {
	hwnd, _, _ := procGetForegroundWindow.Call()
	if hwnd == 0 {
		return "", ""
	}

	titleBuf := make([]uint16, 256)
	n, _, _ := procGetWindowTextW.Call(hwnd, uintptr(unsafe.Pointer(&titleBuf[0])), uintptr(len(titleBuf)))
	title = xwindows.UTF16ToString(titleBuf[:n])

	classBuf := make([]uint16, 256)
	n, _, _ = procGetClassNameW.Call(hwnd, uintptr(unsafe.Pointer(&classBuf[0])), uintptr(len(classBuf)))
	className = xwindows.UTF16ToString(classBuf[:n])
	return title, className
}

// ForegroundWindowHWND 返回当前前台窗口句柄（用于把输入线程附加到目标进程）。
func ForegroundWindowHWND() HWND {
	ret, _, _ := procGetForegroundWindow.Call()
	return HWND(ret)
}

// GetWindowThreadProcessId 返回指定窗口所属线程与进程 ID。
func GetWindowThreadProcessId(hwnd HWND) (threadID, processID uint32) {
	var pid uint32
	tid, _, _ := procGetWindowThreadProcessId.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&pid)))
	return uint32(tid), pid
}

// GetCurrentThreadId 返回调用线程的 ID。
func GetCurrentThreadId() uint32 {
	ret, _, _ := procGetCurrentThreadId.Call()
	return uint32(ret)
}

// AttachThreadInput 把本线程的输入处理附加/脱离到目标线程，
// 这样跨进程 SendInput 才能可靠地投递到目标窗口的焦点控件（浏览器渲染线程）。
func AttachThreadInput(targetThread, currentThread uint32, attach bool) error {
	attachVal := uintptr(0)
	if attach {
		attachVal = 1
	}
	ret, _, err := procAttachThreadInput.Call(uintptr(targetThread), uintptr(currentThread), attachVal)
	if ret == 0 {
		return fmt.Errorf("attach thread input: %w", err)
	}
	return nil
}

// SetFocusHWND 将键盘焦点设到指定窗口（尽力而为，失败不影响流程）。
func SetFocusHWND(hwnd HWND) HWND {
	ret, _, _ := procSetFocus.Call(uintptr(hwnd))
	return HWND(ret)
}

// GetFocusHWND 返回当前线程前台窗口中拥有键盘焦点的子窗口。
func GetFocusHWND() HWND {
	ret, _, _ := procGetFocus.Call()
	return HWND(ret)
}

// sendCtrlC 向“前台窗口”注入一次干净的 Ctrl+C 来复制选中文本。
//
// 关于之前的实现与“按慢了抓不到”的现象：
//
//	注册热键 RegisterHotKey(Alt+Q) 是在系统层拦截组合键的，Chrome 根本不会收到 Alt 键，
//	所以“Alt 仍按下导致变成 Ctrl+Alt+C”的旧假设并不成立。浏览器里真正不稳定的是——
//	跨线程的 SendInput 在没有 AttachThreadInput 时，不一定能投递到 Chrome 渲染线程的焦点控件，
//	且原实现只发了一次、没有任何重试，一旦这次合成按键被丢弃或焦点状态不对，就直接失败。
//
//	这里改为：先把本线程输入附加到前台窗口所在线程并尽力置前，清理可能残留的修饰键，
//	再注入干净的 Ctrl+C。具体的“偶发丢弃”由 CaptureSelectedText 在超时窗口内多次重试来兜底。
func sendCtrlC(foregroundHWND HWND) {
	var attached bool
	if foregroundHWND != 0 {
		fgThread, _ := GetWindowThreadProcessId(foregroundHWND)
		selfThread := GetCurrentThreadId()
		if fgThread != 0 && fgThread != selfThread {
			if err := AttachThreadInput(fgThread, selfThread, true); err == nil {
				attached = true
			} else {
				log.Printf("sendCtrlC: attach thread input failed: %v", err)
			}
		}
		// 尽力把前台窗口带回前台；浏览器复制选区通常要求它持有前台/焦点。
		SetForegroundWindow(foregroundHWND)
		time.Sleep(20 * time.Millisecond)
	}
	if attached {
		defer func() {
			fgThread, _ := GetWindowThreadProcessId(foregroundHWND)
			_ = AttachThreadInput(fgThread, GetCurrentThreadId(), false)
		}()
	}

	// 等用户松开物理修饰键，避免 Alt/Ctrl 仍按下时合成出 Ctrl+Alt+C / Ctrl+Shift+C。
	waitForModifiersReleased(1500 * time.Millisecond)

	// 兜底清理可能残留的修饰键抬起状态。
	if err := sendKeyboardInputs([]Input{
		keyboardScanInput(VkLMenu, KeyeventfKeyup),
		keyboardScanInput(VkLControl, KeyeventfKeyup),
		keyboardScanInput(VkLShift, KeyeventfKeyup),
	}); err != nil {
		log.Printf("sendCtrlC: release modifiers via SendInput failed: %v", err)
	}
	time.Sleep(20 * time.Millisecond)

	if err := sendKeyboardInputs([]Input{
		keyboardScanInput(VkLControl, 0),
		keyboardScanInput(VkC, 0),
		keyboardScanInput(VkC, KeyeventfKeyup),
		keyboardScanInput(VkLControl, KeyeventfKeyup),
	}); err != nil {
		log.Printf("sendCtrlC: send ctrl+c via SendInput failed: %v", err)
	}
	time.Sleep(30 * time.Millisecond)
}

// waitForModifiersReleased 轮询等待 Ctrl/Alt/Shift/Win 全部物理松开，最多等 timeout。
// 用户按住热键不放时会等到超时后强行继续（并记录日志），避免卡死。
func waitForModifiersReleased(timeout time.Duration) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if !anyModifierDown() {
			return
		}
		time.Sleep(15 * time.Millisecond)
	}
	log.Printf("sendCtrlC: modifiers still held after %v, proceeding anyway", timeout)
}

func anyModifierDown() bool {
	return keyDown(VkControl) || keyDown(VkMenu) || keyDown(VkShift) ||
		keyDown(VkLWin) || keyDown(VkRWin)
}

func keyDown(vk uint16) bool {
	ret, _, _ := procGetAsyncKeyState.Call(uintptr(vk))
	return ret&0x8000 != 0
}

func copyWideString(dst []uint16, value string) {
	src, _ := xwindows.UTF16FromString(value)
	copy(dst, src)
}

func currentModuleHandle() (HWND, error) {
	ret, _, err := procGetModuleHandleW.Call(0)
	if ret == 0 {
		return 0, fmt.Errorf("get module handle: %w", err)
	}
	return HWND(ret), nil
}

func ClipboardSequenceNumber() uint32 {
	ret, _, _ := procGetClipboardSeqNum.Call()
	return uint32(ret)
}

func keyboardInput(vk uint16, flags uint32) Input {
	return Input{
		Type: InputKeyboard,
		Ki: KeybdInput{
			WVk:     vk,
			DwFlags: flags,
		},
	}
}

func keyboardScanInput(vk uint16, flags uint32) Input {
	scan, _, _ := procMapVirtualKeyW.Call(uintptr(vk), 0)
	return Input{
		Type: InputKeyboard,
		Ki: KeybdInput{
			WScan:   uint16(scan),
			DwFlags: flags | KeyeventfScancode,
		},
	}
}

func sendKeyboardInputs(inputs []Input) error {
	if len(inputs) == 0 {
		return nil
	}

	ret, _, err := procSendInput.Call(
		uintptr(len(inputs)),
		uintptr(unsafe.Pointer(&inputs[0])),
		unsafe.Sizeof(inputs[0]),
	)
	if ret == 0 {
		return fmt.Errorf("SendInput failed: %w", err)
	}
	return nil
}
