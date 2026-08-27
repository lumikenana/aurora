//go:build windows
// +build windows

package screenshot

import (
	"bytes"
	"context"
	"image"
	"image/jpeg"
	"math"
	"syscall"
	"unsafe"

	"image/color"

	"github.com/lipipi/aurora/internal/errors"
	"github.com/lipipi/aurora/internal/log"
)

// 一些在 golang.org/x/sys/windows 中没有导出的 GDI32 API 和结构
const (
	// BitBlt copy operation
	sRCCOPY uint32 = 0x00CC0020
	// PrintWindow flags
	pW_CLIENTONLY uint32 = 1
)

type windowsScreenshotProvider struct {
	cfg    Config
	displays []displayInfo
}

type displayInfo struct {
	left   int32
	top    int32
	width  int32
	height int32
	pelsWidth int32
	pelsHeight int32
}

// NewWindowsScreenshotProvider 创建 Windows 截图 Provider
func NewWindowsScreenshotProvider(cfg Config) (ScreenshotProvider, errors.AppError) {
	p := &windowsScreenshotProvider{
		cfg:    cfg,
		displays: make([]displayInfo, 0),
	}

	// 枚举显示器
	err := p.enumerateDisplays()
	if err != nil {
		return nil, errors.Wrap(errors.CodeScreenshotFailed, "failed to enumerate displays", err)
	}

	return p, nil
}

func (p *windowsScreenshotProvider) Name() string {
	return "windows-gdi"
}

func (p *windowsScreenshotProvider) IsAvailable() bool {
	return len(p.displays) > 0
}

func (p *windowsScreenshotProvider) GetDisplayCount() int {
	return len(p.displays)
}

// Some function prototypes we need that are not exported by golang.org/x/sys/windows
var (
	user32                  = syscall.NewLazyDLL("user32.dll")
	gdi32                   = syscall.NewLazyDLL("gdi32.dll")
	procGetSystemMetrics    = user32.NewProc("GetSystemMetrics")
	procGetDC               = user32.NewProc("GetDC")
	procReleaseDC           = user32.NewProc("ReleaseDC")
	procCreateCompatibleDC  = gdi32.NewProc("CreateCompatibleDC")
	procDeleteDC            = gdi32.NewProc("DeleteDC")
	procCreateCompatibleBitmap = gdi32.NewProc("CreateCompatibleBitmap")
	procSelectObject        = gdi32.NewProc("SelectObject")
	procBitBlt              = gdi32.NewProc("BitBlt")
	procDeleteObject        = gdi32.NewProc("DeleteObject")
	procGetDIBits           = gdi32.NewProc("GetDIBits")
	procGetForegroundWindow = user32.NewProc("GetForegroundWindow")
	procGetWindowRect       = user32.NewProc("GetWindowRect")
	procGetWindowDC         = user32.NewProc("GetWindowDC")
	procPrintWindow         = user32.NewProc("PrintWindow")
)

// GetSystemMetrics 索引
const (
	sM_CXSCREEN = 0
	sM_CYSCREEN = 1
)

// enumerateDisplays 枚举显示器（当前仅支持主显示器）
func (p *windowsScreenshotProvider) enumerateDisplays() error {
	width, _, _ := procGetSystemMetrics.Call(sM_CXSCREEN)
	height, _, _ := procGetSystemMetrics.Call(sM_CYSCREEN)

	w := int32(width)
	h := int32(height)
	if w <= 0 || h <= 0 {
		log.Warnf("invalid primary display size: %dx%d", w, h)
		return nil
	}

	// 这里简化处理，只支持主显示器。多显示器需要 EnumDisplayMonitors
	p.displays = append(p.displays, displayInfo{
		left:       0,
		top:        0,
		width:      w,
		height:     h,
		pelsWidth:  w,
		pelsHeight: h,
	})

	return nil
}

func (p *windowsScreenshotProvider) Capture(ctx context.Context, req ScreenshotRequest) (*Screenshot, errors.AppError) {
	select {
	case <-ctx.Done():
		return nil, errors.New(errors.CodeScreenshotFailed, "capture cancelled")
	default:
	}

	var di displayInfo
	var displayIndex int

	switch req.Mode {
	case CaptureModeFullScreen, CaptureModePrimaryDisplay:
		displayIndex = 0
	case CaptureModeSpecifiedDisplay:
		if req.DisplayIndex < 0 || req.DisplayIndex >= len(p.displays) {
			return nil, errors.Newf(errors.CodeScreenshotFailed, "invalid display index: %d, total displays: %d", req.DisplayIndex, len(p.displays))
		}
		displayIndex = req.DisplayIndex
	case CaptureModeActiveWindow:
		return p.captureActiveWindow(req)
	default:
		displayIndex = 0
	}

	di = p.displays[displayIndex]
	return p.captureDisplay(di, req)
}

func (p *windowsScreenshotProvider) captureDisplay(di displayInfo, req ScreenshotRequest) (*Screenshot, errors.AppError) {
	width := int(di.width)
	height := int(di.height)

	// Get screen DC
	hScreenDC, _, err := procGetDC.Call(0)
	if hScreenDC == 0 {
		return nil, errors.Wrap(errors.CodeScreenshotFailed, "GetDC failed", err)
	}
	defer procReleaseDC.Call(0, hScreenDC)

	// Create compatible DC
	hMemDC, _, err := procCreateCompatibleDC.Call(hScreenDC)
	if hMemDC == 0 {
		return nil, errors.Wrap(errors.CodeScreenshotFailed, "CreateCompatibleDC failed", err)
	}
	defer procDeleteDC.Call(hMemDC)

	// Create compatible bitmap
	hBitmap, _, err := procCreateCompatibleBitmap.Call(hScreenDC, uintptr(width), uintptr(height))
	if hBitmap == 0 {
		return nil, errors.Wrap(errors.CodeScreenshotFailed, "CreateCompatibleBitmap failed", err)
	}
	defer procDeleteObject.Call(hBitmap)

	// Select bitmap into memory DC
	oldBitmap, _, err := procSelectObject.Call(hMemDC, hBitmap)
	if oldBitmap == 0 {
		return nil, errors.Wrap(errors.CodeScreenshotFailed, "SelectObject failed", err)
	}
	defer procSelectObject.Call(hMemDC, oldBitmap)

	// BitBlt from screen to memory
	ret, _, _ := procBitBlt.Call(
		hMemDC,
		0,
		0,
		uintptr(width),
		uintptr(height),
		hScreenDC,
		uintptr(di.left),
		uintptr(di.top),
		uintptr(sRCCOPY),
	)
	if ret == 0 {
		return nil, errors.New(errors.CodeScreenshotFailed, "BitBlt failed")
	}

	// GetDIBits 要求位图不能被选入 DC，先恢复旧位图
	procSelectObject.Call(hMemDC, oldBitmap)

	// Get bitmap bits into image.RGBA
	img, err := bitmapToImage(hScreenDC, hBitmap, width, height)
	if err != nil {
		return nil, errors.Wrap(errors.CodeScreenshotFailed, "failed to convert bitmap to image", err)
	}

	// Resize if needed
	resized := p.resizeImage(img, req.MaxWidth, req.MaxHeight)

	// Compress
	result, err := p.compressImage(resized, req.Quality)
	if err != nil {
		return nil, errors.Wrap(errors.CodeScreenshotFailed, "failed to compress image", err)
	}

	result.Mode = req.Mode

	return result, nil
}

// BITMAPINFOHEADER for GetDIBits
type bitmapInfoHeader struct {
	biSize          uint32
	biWidth         int32
	biHeight        int32
	biPlanes        uint16
	biBitCount       uint16
	biCompression    uint32
	biSizeImage      uint32
	biXPelsPerMeter  int32
	biYPelsPerMeter  int32
	biClrUsed        uint32
	biClrImportant   uint32
}

type bitmapInfo struct {
	bmiHeader bitmapInfoHeader
	bmiColors [2]uint32
}

const (
	bI_RGB         uint32 = 0
	dIB_RGB_COLORS uint32 = 0
)

// bitmapToImage 将 Windows 位图转换为 Go image.RGBA
func bitmapToImage(hdc, hBitmap uintptr, width, height int) (*image.RGBA, error) {
	bmi := bitmapInfo{
		bmiHeader: bitmapInfoHeader{
			biSize:        uint32(unsafe.Sizeof(bitmapInfoHeader{})),
			biWidth:       int32(width),
			biHeight:      -int32(height), // negative for top-down DIB
			biPlanes:      1,
			biBitCount:     32,
			biCompression:  bI_RGB,
		},
	}

	img := image.NewRGBA(image.Rect(0, 0, width, height))

	ret, _, err := procGetDIBits.Call(
		hdc,
		hBitmap,
		0,
		uintptr(height),
		uintptr(unsafe.Pointer(&img.Pix[0])),
		uintptr(unsafe.Pointer(&bmi)),
		uintptr(dIB_RGB_COLORS),
	)
	if ret == 0 {
		if err != nil && err != syscall.Errno(0) {
			return nil, err
		}
		return nil, syscall.Errno(0)
	}

	// BGRA -> RGBA conversion
	for i := 0; i < len(img.Pix); i += 4 {
		b := img.Pix[i]
		g := img.Pix[i+1]
		r := img.Pix[i+2]
		a := img.Pix[i+3]
		img.Pix[i] = r
		img.Pix[i+1] = g
		img.Pix[i+2] = b
		img.Pix[i+3] = a
	}

	return img, nil
}

func (p *windowsScreenshotProvider) resizeImage(img *image.RGBA, maxWidth, maxHeight int) image.Image {
	if maxWidth <= 0 && maxHeight <= 0 {
		return img
	}

	origW := img.Bounds().Dx()
	origH := img.Bounds().Dy()

	if (maxWidth <= 0 || origW <= maxWidth) && (maxHeight <= 0 || origH <= maxHeight) {
		return img // no resize needed
	}

	// Calculate new dimensions maintaining aspect ratio
	var newW, newH int
	if maxWidth > 0 && maxHeight > 0 {
		ratio := math.Min(float64(maxWidth)/float64(origW), float64(maxHeight)/float64(origH))
		newW = int(math.Round(float64(origW) * ratio))
		newH = int(math.Round(float64(origH) * ratio))
	} else if maxWidth > 0 {
		ratio := float64(maxWidth)/float64(origW)
		newW = maxWidth
		newH = int(math.Round(float64(origH) * ratio))
	} else {
		ratio := float64(maxHeight)/float64(origH)
		newH = maxHeight
		newW = int(math.Round(float64(origW) * ratio))
	}

	// Simple nearest neighbor resize
	resized := image.NewRGBA(image.Rect(0, 0, newW, newH))
	scaleX := float64(origW)/float64(newW)
	scaleY := float64(origH)/float64(newH)

	for y := 0; y < newH; y++ {
		for x := 0; x < newW; x++ {
			srcX := int(math.Floor(float64(x) * scaleX))
			srcY := int(math.Floor(float64(y) * scaleY))
			r, g, b, a := img.At(srcX, srcY).RGBA()
			resized.SetRGBA(x, y, color.RGBA{R: uint8(r >> 8), G: uint8(g >> 8), B: uint8(b >> 8), A: uint8(a >> 8)})
		}
	}

	return resized
}

func (p *windowsScreenshotProvider) compressImage(img image.Image, quality int) (*Screenshot, errors.AppError) {
	bounds := img.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()

	if quality <= 0 {
		quality = p.cfg.DefaultQuality
	}
	if quality < 1 {
		quality = 1
	}
	if quality > 100 {
		quality = 100
	}

	// We'll use JPEG compression by default
	var buf []byte
	// Use bytes.Buffer to compress into memory
	// Actually we need to create a buffer and encode directly
	// Go standard library does this efficiently
	var m = new(bytes.Buffer)
	err := jpeg.Encode(m, img, &jpeg.Options{Quality: quality})
	if err != nil {
		return nil, errors.Wrap(errors.CodeScreenshotFailed, "JPEG encode failed", err)
	}
	buf = m.Bytes()

	return &Screenshot{
		OriginalImage: img,
		Width:         width,
		Height:        height,
		Bytes:         buf,
		Format:        "jpeg",
	}, nil
}

// captureActiveWindow 捕获当前活动窗口
func (p *windowsScreenshotProvider) captureActiveWindow(req ScreenshotRequest) (*Screenshot, errors.AppError) {
	// Get foreground window
	hWnd, _, _ := procGetForegroundWindow.Call()
	if hWnd == 0 {
		return nil, errors.New(errors.CodeScreenshotFailed, "no foreground window found")
	}

	return p.captureWindow(hWnd, req)
}

// captureWindow 捕获指定窗口
func (p *windowsScreenshotProvider) captureWindow(hWnd uintptr, req ScreenshotRequest) (*Screenshot, errors.AppError) {
	var rect struct{ left, top, right, bottom int32 }
	ret, _, _ := procGetWindowRect.Call(hWnd, uintptr(unsafe.Pointer(&rect)))
	if ret == 0 {
		return nil, errors.New(errors.CodeScreenshotFailed, "GetWindowRect failed")
	}

	width := rect.right - rect.left
	height := rect.bottom - rect.top

	if width <= 0 || height <= 0 {
		return nil, errors.New(errors.CodeScreenshotFailed, "invalid window dimensions")
	}

	// Get window DC
	hWindowDC, _, _ := procGetWindowDC.Call(hWnd)
	if hWindowDC == 0 {
		return nil, errors.New(errors.CodeScreenshotFailed, "GetWindowDC failed")
	}
	defer procReleaseDC.Call(hWnd, hWindowDC)

	// Create compatible DC
	hMemDC, _, _ := procCreateCompatibleDC.Call(hWindowDC)
	if hMemDC == 0 {
		return nil, errors.New(errors.CodeScreenshotFailed, "CreateCompatibleDC failed")
	}
	defer procDeleteDC.Call(hMemDC)

	// Create compatible bitmap
	hBitmap, _, _ := procCreateCompatibleBitmap.Call(hWindowDC, uintptr(width), uintptr(height))
	if hBitmap == 0 {
		return nil, errors.New(errors.CodeScreenshotFailed, "CreateCompatibleBitmap failed")
	}
	defer procDeleteObject.Call(hBitmap)

	oldBitmap, _, _ := procSelectObject.Call(hMemDC, hBitmap)
	if oldBitmap == 0 {
		return nil, errors.New(errors.CodeScreenshotFailed, "SelectObject failed")
	}
	defer procSelectObject.Call(hMemDC, oldBitmap)

	// Print window to bitmap
	ret, _, _ = procPrintWindow.Call(hWnd, hMemDC, uintptr(pW_CLIENTONLY))
	if ret == 0 {
		// Fallback to BitBlt
		ret, _, _ = procBitBlt.Call(
			hMemDC,
			0,
			0,
			uintptr(width),
			uintptr(height),
			hWindowDC,
			0,
			0,
			uintptr(sRCCOPY),
		)
		if ret == 0 {
			return nil, errors.New(errors.CodeScreenshotFailed, "failed to capture window content")
		}
	}

	// GetDIBits 要求位图不能被选入 DC，先恢复旧位图
	procSelectObject.Call(hMemDC, oldBitmap)

	img, err := bitmapToImage(hWindowDC, hBitmap, int(width), int(height))
	if err != nil {
		return nil, errors.Wrap(errors.CodeScreenshotFailed, "failed to convert bitmap to image", err)
	}

	resized := p.resizeImage(img, req.MaxWidth, req.MaxHeight)
	result, appErr := p.compressImage(resized, req.Quality)
	if appErr != nil {
		return nil, appErr
	}
	result.Mode = CaptureModeActiveWindow

	return result, nil
}

func init() {
	RegisterProvider("windows", NewWindowsScreenshotProvider)
}
