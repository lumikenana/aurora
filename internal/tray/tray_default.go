//go:build !windows
// +build !windows

package tray

import (
	"github.com/lipipi/aurora/internal/errors"
	"github.com/lipipi/aurora/internal/log"
)

// defaultTray 非Windows平台默认托盘实现（空实现）
type defaultTray struct {
	onShow            func()
	onExit            func()
	onToggleQuickInput func()
	onToggleOverlay    func()
}

func createTray() Tray {
	return &defaultTray{}
}

func (t *defaultTray) Init() error {
	log.Info("System tray not supported on this platform (empty implementation)")
	return nil
}

func (t *defaultTray) Shutdown() error {
	return nil
}

func (t *defaultTray) Show() error {
	log.Debug("Tray.Show called (not supported)")
	return errors.New(errors.CodeTrayError, "System tray not supported on this platform")
}

func (t *defaultTray) Hide() error {
	log.Debug("Tray.Hide called (not supported)")
	return errors.New(errors.CodeTrayError, "System tray not supported on this platform")
}

func (t *defaultTray) SetTooltip(tooltip string) error {
	log.Debugf("Tray.SetTooltip: %s (not supported)", tooltip)
	return nil
}

func (t *defaultTray) AddMenuItem(label string, onClick func()) error {
	log.Debugf("Tray.AddMenuItem: %s (not supported)", label)
	return nil
}

func (t *defaultTray) AddSeparator() error {
	log.Debug("Tray.AddSeparator called (not supported)")
	return nil
}

func (t *defaultTray) AddSubMenu(parentLabel, childLabel string, onClick func()) error {
	log.Debugf("Tray.AddSubMenu: %s/%s (not supported)", parentLabel, childLabel)
	return nil
}

func (t *defaultTray) SetMenuItemChecked(parentLabel, childLabel string, checked bool) error {
	log.Debugf("Tray.SetMenuItemChecked: %s/%s=%v (not supported)", parentLabel, childLabel, checked)
	return nil
}

func (t *defaultTray) SetOnShowHandler(onShow func()) {
	t.onShow = onShow
}

func (t *defaultTray) SetOnExitHandler(onExit func()) {
	t.onExit = onExit
}

func (t *defaultTray) SetOnToggleQuickInputHandler(onToggle func()) {
	t.onToggleQuickInput = onToggle
}

// SetOnToggleOverlayHandler 设置 Overlay 切换回调（Phase 3 MVP，stub）
func (t *defaultTray) SetOnToggleOverlayHandler(onToggle func()) {
	t.onToggleOverlay = onToggle
	// 空实现
}

// SetOverlayMenuState 非 Windows stub（空实现）
func (t *defaultTray) SetOverlayMenuState(visible bool) {
	// 空实现
}

// SetRecordingIcon changes the tray icon to indicate recording/listening state.
// Empty implementation on non-Windows platforms.
func (t *defaultTray) SetRecordingIcon(isRecording bool) {
	// Empty implementation
}
