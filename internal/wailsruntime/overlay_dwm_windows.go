//go:build windows

// overlay_dwm_windows.go — Overlay 窗口的 DWM 级去边处理（Windows only）。
//
// 为什么需要（2026-09-07 Overlay 白边根因 B）：
//
//	Overlay 窗口是 Frameless + BackgroundTypeTransparent，页面层（html/body +
//	WebView2 default background）已全透明，但 Windows 11 的 DWM 会对顶层窗口
//	（即使 WS_POPUP + WS_THICKFRAME）额外合成：
//	  1. 1px 系统色窗口边框（浅色主题下 ≈ 白色）
//	  2. 圆角 + 圆角抗锯齿
//	叠加在透明像素之上 → 面板四周 8px 透明 margin 带透出的不是桌面，
//	而是 DWM 画的白边。此外 Wails v3 对 Frameless 窗口默认执行
//	DwmExtendFrameIntoClientArea（"frameless decorations"），进一步强制启用
//	DWM frame 外观（main.go 已设 DisableFramelessWindowDecorations 关闭）。
//
// 本函数在窗口 HWND 上设置两个 DWM 属性（均幂等、可重复调用）：
//   - DWMWA_WINDOW_CORNER_PREFERENCE = DWMWCP_DONOTROUND：关闭 DWM 圆角
//   - DWMWA_BORDER_COLOR             = DWMWA_COLOR_NONE：关闭 DWM 1px 边框
//
// 面板自身的圆角 / 描边完全由前端 CSS（border-radius + border）绘制。
package wailsruntime

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	dwmapi               = windows.NewLazySystemDLL("dwmapi.dll")
	procDwmSetWindowAttr = dwmapi.NewProc("DwmSetWindowAttribute")
)

const (
	dwmWindowCornerPref  = 33          // DWMWA_WINDOW_CORNER_PREFERENCE
	dwmWindowBorderColor = 34          // DWMWA_BORDER_COLOR
	dwmBorderColorNone   = ^uintptr(1) // DWMWA_COLOR_NONE (0xFFFFFFFE)
)

// applyOverlayDWMTransparency 关闭 Overlay 窗口的 DWM 圆角与 1px 系统边框。
// 窗口未创建（HWND 不可用）时 no-op；属性设置失败静默忽略
// （旧系统无 DWMWA_BORDER_COLOR 时窗口退回默认外观，不影响功能）。
func applyOverlayDWMTransparency() {
	if OverlayWindow == nil {
		return
	}
	hwnd := OverlayWindow.NativeWindow()
	if hwnd == nil {
		return
	}
	doNotRound := uintptr(1) // DWMWCP_DONOTROUND
	hr, _, _ := procDwmSetWindowAttr.Call(
		uintptr(hwnd),
		dwmWindowCornerPref,
		uintptr(unsafe.Pointer(&doNotRound)),
		4, // sizeof(DWORD)
	)
	if hr != 0 {
		return
	}
	borderNone := dwmBorderColorNone
	procDwmSetWindowAttr.Call(
		uintptr(hwnd),
		dwmWindowBorderColor,
		uintptr(unsafe.Pointer(&borderNone)),
		4, // sizeof(DWORD)
	)
}
