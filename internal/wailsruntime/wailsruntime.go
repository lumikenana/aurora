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
