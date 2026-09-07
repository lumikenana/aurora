//go:build !windows

// overlay_dwm_other.go — 非 Windows 平台 no-op 存根。
// Overlay 的 DWM 去边处理是 Windows 专属（见 overlay_dwm_windows.go）；
// 本存根保证 darwin/linux 上 go build ./... 通过。
package wailsruntime

func applyOverlayDWMTransparency() {}
