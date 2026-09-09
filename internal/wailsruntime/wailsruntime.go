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

	"github.com/lipipi/aurora/internal/log"
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
	// overlayDefaultMargin overlay 面板与屏幕工作区边缘的间距
	// （2026-09-07 Anchor 移除后硬编码 bottom-right 位置 + 16px margin）
	overlayDefaultMargin = 16
)

// ShowOverlayWindow shows the overlay window without destroying it.
// No-op before the window is created.
func ShowOverlayWindow() {
	if OverlayWindow == nil {
		return
	}
	// DWM 去边（Win11 1px 系统边框 + 圆角）：幂等，每次显示前保障生效
	// （2026-09-07 Overlay 白边根因 B，见 overlay_dwm_windows.go）。
	applyOverlayDWMTransparency()

	beforeVisible := OverlayWindow.IsVisible()
	beforeW, beforeH := OverlayWindow.Size()

	// Workaround for Wails v3 beta.16 race: if the window hasn't been
	// created yet when Show() is first called at startup, two consecutive
	// Show() calls may still leave the window hidden (second Show returns
	// but WS_VISIBLE isn't actually set).
	//
	// Strategy: explicitly Run() first to guarantee the HWND exists
	// (idempotent — no-op if already created), then Show().
	OverlayWindow.Run()
	afterRunVisible := OverlayWindow.IsVisible()
	afterRunW, afterRunH := OverlayWindow.Size()

	OverlayWindow.Show()
	afterShowVisible := OverlayWindow.IsVisible()
	afterShowW, afterShowH := OverlayWindow.Size()

	log.Debugf("[DIAG-BugB] ShowOverlayWindow: before(vis=%v %dx%d) → afterRun(vis=%v %dx%d) → afterShow(vis=%v %dx%d)",
		beforeVisible, beforeW, beforeH,
		afterRunVisible, afterRunW, afterRunH,
		afterShowVisible, afterShowW, afterShowH)
}

// HideOverlayWindow hides the overlay window (singleton — never destroyed).
func HideOverlayWindow() {
	if OverlayWindow == nil {
		return
	}
	OverlayWindow.Hide()
}

// setDefaultOverlayPosition positions the overlay window at the default
// location: primary screen work area 的 bottom-right + 16px margin。
//
// 历史：2026-09-07 移除 Anchor 字段前，此函数是 SetOverlayAnchorPosition，
// 支持 top-left/right/bottom-left/right 四角；现在硬编码 bottom-right。
// 保留函数名/职责（"默认位置"），调用方仅 SetOverlayGeometry。
//
// 不做：多显示器跟随（MVP 固定主屏）。
func setDefaultOverlayPosition() {
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
	x := work.X + work.Width - w - overlayDefaultMargin
	y := work.Y + work.Height - h - overlayDefaultMargin
	OverlayWindow.SetPosition(x, y)
}

// SetOverlayGeometry 按 overlay 配置应用窗口几何：
//   - w/h > 0 时应用自定义尺寸（config.Overlay.Width/Height），否则保持当前
//   - (x, y) != (0, 0) 时应用自定义位置，否则走默认位置（setDefaultOverlayPosition：
//     bottom-right + 16px margin）
//
// 由 main.go 在 overlay 显示 / 配置变更时调用（传 cfg.Overlay 字段）。
// 2026-09-07 Anchor 字段移除后，签名不再带 anchor 参数。
func SetOverlayGeometry(x, y, w, h int) {
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
	setDefaultOverlayPosition()
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

// SetOverlayResizable 切换 overlay 窗口是否可调尺寸（Edit Mode = true，HUD = false）。
// 包装 v3 SetResizable，绕过 Wails 内部 options 比较（确保每次切换都生效）。
// 窗口未创建时 no-op。
func SetOverlayResizable(b bool) {
	if OverlayWindow == nil {
		return
	}
	OverlayWindow.SetResizable(b)
}

// SetOverlayIgnoreMouseEvents 切换 overlay 窗口是否 click-through：
//   - true（HUD / Locked）：鼠标事件穿透到下层（游戏画面）
//   - false（Edit Mode）：正常接收鼠标事件（drag / resize / 按钮）
//
// 窗口未创建时 no-op。
func SetOverlayIgnoreMouseEvents(ignore bool) {
	if OverlayWindow == nil {
		return
	}
	OverlayWindow.SetIgnoreMouseEvents(ignore)
}

// EmergencyHideAll 紧急隐藏全部 Aurora 窗口（主窗口 / Overlay / QuickInput）。
// 只隐藏 UI，不停止后台 AI / 语音 / 快捷键服务（紧急隐藏的语义边界）。
func EmergencyHideAll() {
	HideOverlayWindow()
	HideQuickInputWindow()
	HideMainWindow()
}
