//go:build !windows
// +build !windows

package tray

import (
	"github.com/lipipi/game-ai/internal/errors"
	"github.com/lipipi/game-ai/internal/log"
)

// defaultTray 非Windows平台默认托盘实现（空实现）
type defaultTray struct {
	onShow func()
	onExit func()
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

func (t *defaultTray) SetOnShowHandler(onShow func()) {
	t.onShow = onShow
}

func (t *defaultTray) SetOnExitHandler(onExit func()) {
	t.onExit = onExit
}

func (t *defaultTray) SetOnToggleQuickInputHandler(onToggle func()) {
	// 空实现
}
