package windows

import (
	"bytes"
	"fmt"
	"image"
	"image/png"
	"unsafe"

	xwindows "golang.org/x/sys/windows"
)

type bitmapInfoHeader struct {
	Size          uint32
	Width         int32
	Height        int32
	Planes        uint16
	BitCount      uint16
	Compression   uint32
	SizeImage     uint32
	XPelsPerMeter int32
	YPelsPerMeter int32
	ClrUsed       uint32
	ClrImportant  uint32
}

type bitmapInfo struct {
	Header bitmapInfoHeader
	Colors [1]uint32
}

const (
	biRGB        = 0
	dibRGBColors = 0
	srccopy      = 0x00CC0020
	captureBLT   = 0x40000000
)

var (
	modGDI32 = xwindows.NewLazySystemDLL("gdi32.dll")

	procGetDC                  = modUser32.NewProc("GetDC")
	procReleaseDC              = modUser32.NewProc("ReleaseDC")
	procCreateCompatibleDC     = modGDI32.NewProc("CreateCompatibleDC")
	procDeleteDC               = modGDI32.NewProc("DeleteDC")
	procCreateCompatibleBitmap = modGDI32.NewProc("CreateCompatibleBitmap")
	procSelectObject           = modGDI32.NewProc("SelectObject")
	procBitBlt                 = modGDI32.NewProc("BitBlt")
	procGetDIBits              = modGDI32.NewProc("GetDIBits")
	procCreateSolidBrush       = modGDI32.NewProc("CreateSolidBrush")
	procDeleteObject           = modGDI32.NewProc("DeleteObject")
)

func CaptureScreenPNG(rect Rect) ([]byte, error) {
	rect = NormalizeRect(rect)
	width := rect.Width()
	height := rect.Height()
	if width <= 0 || height <= 0 {
		return nil, fmt.Errorf("invalid capture rect: %+v", rect)
	}

	screenDC, _, err := procGetDC.Call(0)
	if screenDC == 0 {
		return nil, fmt.Errorf("get screen dc: %w", err)
	}
	defer procReleaseDC.Call(0, screenDC)

	memDC, _, err := procCreateCompatibleDC.Call(screenDC)
	if memDC == 0 {
		return nil, fmt.Errorf("create compatible dc: %w", err)
	}
	defer procDeleteDC.Call(memDC)

	bitmap, _, err := procCreateCompatibleBitmap.Call(screenDC, uintptr(width), uintptr(height))
	if bitmap == 0 {
		return nil, fmt.Errorf("create compatible bitmap: %w", err)
	}
	defer deleteObject(bitmap)

	oldObj, _, _ := procSelectObject.Call(memDC, bitmap)
	if oldObj != 0 {
		defer procSelectObject.Call(memDC, oldObj)
	}

	ok, _, err := procBitBlt.Call(
		memDC,
		0,
		0,
		uintptr(width),
		uintptr(height),
		screenDC,
		uintptr(rect.Left),
		uintptr(rect.Top),
		srccopy|captureBLT,
	)
	if ok == 0 {
		return nil, fmt.Errorf("bitblt: %w", err)
	}
	// GetDIBits requires the bitmap not to be selected into any device context.
	if oldObj != 0 {
		procSelectObject.Call(memDC, oldObj)
	}

	stride := int(width) * 4
	pixels := make([]byte, stride*int(height))
	info := bitmapInfo{
		Header: bitmapInfoHeader{
			Size:        uint32(unsafe.Sizeof(bitmapInfoHeader{})),
			Width:       width,
			Height:      -height,
			Planes:      1,
			BitCount:    32,
			Compression: biRGB,
			SizeImage:   uint32(len(pixels)),
		},
	}

	lines, _, err := procGetDIBits.Call(
		memDC,
		bitmap,
		0,
		uintptr(height),
		uintptr(unsafe.Pointer(&pixels[0])),
		uintptr(unsafe.Pointer(&info)),
		dibRGBColors,
	)
	if lines == 0 {
		return nil, fmt.Errorf("get dibits: %w", err)
	}

	img := image.NewRGBA(image.Rect(0, 0, int(width), int(height)))
	for y := 0; y < int(height); y++ {
		for x := 0; x < int(width); x++ {
			src := y*stride + x*4
			dst := y*img.Stride + x*4
			img.Pix[dst+0] = pixels[src+2]
			img.Pix[dst+1] = pixels[src+1]
			img.Pix[dst+2] = pixels[src+0]
			img.Pix[dst+3] = 0xFF
		}
	}

	var out bytes.Buffer
	if err := png.Encode(&out, img); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}
