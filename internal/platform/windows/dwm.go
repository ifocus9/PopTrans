package windows

import (
	"unsafe"

	xwindows "golang.org/x/sys/windows"
)

type dwmBlurBehind struct {
	Flags                 uint32
	Enable                int32
	RegionBlur            uintptr
	TransitionOnMaximized int32
}

const (
	dwmBBEnable = 0x00000001

	dwmwaUseImmersiveDarkMode       = 20
	dwmwaUseImmersiveDarkModeBefore = 19
	dwmwaWindowCornerPreference     = 33
	dwmwcpRound                     = 2
)

var (
	modDwmapi = xwindows.NewLazySystemDLL("dwmapi.dll")

	procDwmEnableBlurBehindWindow = modDwmapi.NewProc("DwmEnableBlurBehindWindow")
	procDwmSetWindowAttribute     = modDwmapi.NewProc("DwmSetWindowAttribute")
)

func ApplyModernFrame(hwnd HWND) {
	enable := int32(1)
	corner := int32(dwmwcpRound)

	// These attributes are unavailable on older Windows builds, so each call is best-effort.
	procDwmSetWindowAttribute.Call(
		uintptr(hwnd),
		dwmwaUseImmersiveDarkMode,
		uintptr(unsafe.Pointer(&enable)),
		unsafe.Sizeof(enable),
	)
	procDwmSetWindowAttribute.Call(
		uintptr(hwnd),
		dwmwaUseImmersiveDarkModeBefore,
		uintptr(unsafe.Pointer(&enable)),
		unsafe.Sizeof(enable),
	)
	procDwmSetWindowAttribute.Call(
		uintptr(hwnd),
		dwmwaWindowCornerPreference,
		uintptr(unsafe.Pointer(&corner)),
		unsafe.Sizeof(corner),
	)

	blur := dwmBlurBehind{
		Flags:  dwmBBEnable,
		Enable: 1,
	}
	procDwmEnableBlurBehindWindow.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&blur)))
}
