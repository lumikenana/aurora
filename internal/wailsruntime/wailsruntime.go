// Package wailsruntime is the single bridge between Aurora and Wails v3.
//
// Why a single bridge:
//
//   - `wails/v3/...` is imported by exactly two files in the whole repo:
//     this file (the bridge) and main.go (the bootstrap). All other code
//     that needs to talk to the Wails application goes through the
//     exported helpers below.
//   - This keeps the v3 import fan-out small and makes future v3 changes
//     (e.g. if Wails v3 GA re-shapes EventManager) require touching one
//     file instead of a dozen.
//   - The bridge is intentionally no-op before App is set (i.e. before
//     application.New(...) returns and main.go assigns App). This lets
//     tray callbacks, hotkey callbacks, and lifecycle hooks be wired up
//     earlier in main() without needing to know the v3 lifecycle details.
package wailsruntime

import (
	"context"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// App is the Wails v3 application singleton. It is set exactly once by
// main.go immediately after `application.New(...)` returns. Reads of App
// are only safe after that point; before that, App is nil and the
// helpers below return safe no-op values.
var App *application.App

// MainWindow is the primary WebviewWindow for Aurora. Set once by main.go
// after `app.Window.NewWithOptions(...)`. Before it is set, helpers that
// touch the main window (Show / Hide) are no-ops.
var MainWindow *application.WebviewWindow

// QuickInputWindow is the frameless, always-on-top, system-level quick
// input window (Phase 8E). Set once by main.go after creation; before that
// the helpers below are no-ops. Hidden by default at startup; ShowQuickInputWindow
// is the only path that brings it up (Ctrl+Alt+R via v3 GlobalShortcut,
// tray menu, or programmatic toggle).
var QuickInputWindow *application.WebviewWindow

// OverlayWindow is the frameless, always-on-top, transparent game overlay
// window (Phase 8F). Set once by main.go after creation; before that the
// helpers below are no-ops. Hidden by default; visibility follows the agent's
// overlayVisibleState via the overlayVisibilityChanged event (main.go
// subscription), so the single source of truth stays in the agent package.
var OverlayWindow *application.WebviewWindow

// EmitEvent broadcasts a custom event to all windows and Go-side
// listeners. Returns false if the app is not yet running.
func EmitEvent(name string, data ...any) bool {
	if App == nil {
		return false
	}
	return App.Event.Emit(name, data...)
}

// SubscribeEvent subscribes to a custom event. The callback receives the
// payload that was passed to EmitEvent. The returned function unsubscribes
// (caller can ignore it; v3 cleans up on shutdown). Safe to call before
// App is set: returns a no-op unsubscribe.
func SubscribeEvent(name string, cb func(data ...any)) func() {
	if App == nil {
		return func() {}
	}
	return App.Event.On(name, func(e *application.CustomEvent) {
		cb(e.Data)
	})
}

// GetContext returns the long-lived app context. Before App is set,
// returns context.Background so callers never block on a nil ctx.
func GetContext() context.Context {
	if App == nil {
		return context.Background()
	}
	return App.Context()
}

// Quit requests a graceful application shutdown. No-op if App is not yet
// running.
func Quit() {
	if App != nil {
		App.Quit()
	}
}

// ShowMainWindow brings the main window to the front, un-minimising it
// if needed. No-op before the main window is created.
func ShowMainWindow() {
	if MainWindow == nil {
		return
	}
	MainWindow.UnMinimise()
	MainWindow.Show()
}

// HideMainWindow hides the main window (Aurora stays alive in the tray).
func HideMainWindow() {
	if MainWindow == nil {
		return
	}
	MainWindow.Hide()
}

// ShowQuickInputWindow shows the quick input window and gives it focus.
// No-op if the window is not yet created.
func ShowQuickInputWindow() {
	if QuickInputWindow == nil {
		return
	}
	QuickInputWindow.UnMinimise()
	QuickInputWindow.Show()
	QuickInputWindow.Focus()
}

// HideQuickInputWindow hides the quick input window (singleton — never
// destroyed; Show is the only path to bring it back up).
func HideQuickInputWindow() {
	if QuickInputWindow == nil {
		return
	}
	QuickInputWindow.Hide()
}

// ToggleQuickInputWindow flips the quick input window's visibility.
func ToggleQuickInputWindow() {
	if QuickInputWindow == nil {
		return
	}
	if QuickInputWindow.IsVisible() {
		HideQuickInputWindow()
	} else {
		ShowQuickInputWindow()
	}
}

// overlayWindowWidth/Height 与 main.go 的 WebviewWindowOptions 保持一致
// （默认 preset；config.Overlay.Width/Height 非零时被自定义尺寸覆盖）。
const (
	overlayWindowWidth  = 360
	overlayWindowHeight = 420
	// overlayAnchorMargin overlay 面板与屏幕工作区边缘的间距（与旧 CSS anchor 16px 一致）
	overlayAnchorMargin = 16
)

// ShowOverlayWindow shows the overlay window without destroying it.
// No-op before the window is created.
func ShowOverlayWindow() {
	if OverlayWindow == nil {
		return
	}
	OverlayWindow.Show()
}

// HideOverlayWindow hides the overlay window (singleton — never destroyed).
func HideOverlayWindow() {
	if OverlayWindow == nil {
		return
	}
	OverlayWindow.Hide()
}

// SetOverlayAnchorPosition positions the overlay window at one of the four
// corners of the primary screen's work area (Phase 8F: Anchor 语义为
// "主屏 WorkArea 四角")。
//
// 不做：多显示器跟随（MVP 固定主屏）。anchor 值非法时 no-op。
func SetOverlayAnchorPosition(anchor string) {
	if OverlayWindow == nil {
		return
	}
	if App == nil || App.Screen == nil {
		return
	}
	screen := App.Screen.GetPrimary()
	if screen == nil {
		return
	}
	work := screen.WorkArea
	w, h := OverlayWindow.Size()
	if w <= 0 {
		w = overlayWindowWidth
	}
	if h <= 0 {
		h = overlayWindowHeight
	}
	var x, y int
	switch anchor {
	case "top-left":
		x, y = work.X+overlayAnchorMargin, work.Y+overlayAnchorMargin
	case "top-right":
		x, y = work.X+work.Width-w-overlayAnchorMargin, work.Y+overlayAnchorMargin
	case "bottom-left":
		x, y = work.X+overlayAnchorMargin, work.Y+work.Height-h-overlayAnchorMargin
	case "bottom-right":
		x, y = work.X+work.Width-w-overlayAnchorMargin, work.Y+work.Height-h-overlayAnchorMargin
	default:
		// Unknown anchor：保持当前位置（数据保真，不猜）
		return
	}
	OverlayWindow.SetPosition(x, y)
}

// SetOverlayGeometry 按 overlay 配置应用窗口几何：
//   - w/h > 0 时应用自定义尺寸（config.Overlay.Width/Height），否则保持当前
//   - (x, y) != (0, 0) 时应用自定义位置，否则按 anchor 定位到主屏 WorkArea 四角
//
// 由 main.go 在 overlay 显示 / 配置变更时调用（传 cfg.Overlay 字段）。
func SetOverlayGeometry(anchor string, x, y, w, h int) {
	if OverlayWindow == nil {
		return
	}
	if w > 0 && h > 0 {
		if cw, ch := OverlayWindow.Size(); cw != w || ch != h {
			OverlayWindow.SetSize(w, h)
		}
	}
	if x != 0 || y != 0 {
		OverlayWindow.SetPosition(x, y)
		return
	}
	SetOverlayAnchorPosition(anchor)
}

// MoveOverlayWindow 实时移动 overlay 窗口（Edit Mode 拖拽用，不持久化）。
func MoveOverlayWindow(x, y int) {
	if OverlayWindow == nil {
		return
	}
	OverlayWindow.SetPosition(x, y)
}

// ResizeOverlayWindow 实时调整 overlay 窗口尺寸（Edit Mode resize 用，不持久化）。
func ResizeOverlayWindow(w, h int) {
	if OverlayWindow == nil {
		return
	}
	OverlayWindow.SetSize(w, h)
}

// GetOverlayWindowBounds 返回 overlay 窗口当前几何（物理像素）。
// 窗口未创建时返回 (0,0,0,0)。
func GetOverlayWindowBounds() (x, y, w, h int) {
	if OverlayWindow == nil {
		return 0, 0, 0, 0
	}
	x, y = OverlayWindow.Position()
	w, h = OverlayWindow.Size()
	return x, y, w, h
}

// EmergencyHideAll 紧急隐藏全部 Aurora 窗口（主窗口 / Overlay / QuickInput）。
// 只隐藏 UI，不停止后台 AI / 语音 / 快捷键服务（紧急隐藏的语义边界）。
func EmergencyHideAll() {
	HideOverlayWindow()
	HideQuickInputWindow()
	HideMainWindow()
}
