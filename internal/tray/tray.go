// Package tray 系统托盘管理
package tray

import (
	"github.com/lipipi/aurora/internal/errors"
	"github.com/lipipi/aurora/internal/log"
)

// Tray 系统托盘接口
type Tray interface {
	// Init 初始化托盘
	Init() error
	// Shutdown 关闭托盘
	Shutdown() error
	// Show 显示托盘图标
	Show() error
	// Hide 隐藏托盘图标
	Hide() error
	// SetTooltip 设置提示文字
	SetTooltip(tooltip string) error
	// AddMenuItem 添加菜单项
	AddMenuItem(label string, onClick func()) error
	// AddSeparator 添加分隔线
	AddSeparator() error
	// SetOnShowHandler 设置显示窗口回调
	SetOnShowHandler(onShow func())
	// SetOnToggleQuickInputHandler 设置快速提问切换回调
	SetOnToggleQuickInputHandler(onToggle func())
	// SetOnExitHandler 设置退出回调
	SetOnExitHandler(onExit func())
	// SetRecordingIcon changes the tray icon to indicate recording/listening state.
	SetRecordingIcon(isRecording bool)
}

var (
	currentTray Tray
)

// Init 初始化系统托盘
func Init() errors.AppError {
	log.Debug("Initializing Windows system tray...")

	currentTray = createTray()
	if err := currentTray.Init(); err != nil {
		return errors.Wrap(errors.CodeTrayError, "Failed to initialize tray", err)
	}

	log.Debug("System tray initialized")
	return nil
}

// Shutdown 关闭系统托盘
func Shutdown() {
	if currentTray != nil {
		_ = currentTray.Shutdown()
	}
}

// Current 获取当前托盘实例
func Current() Tray {
	return currentTray
}

// SetRecordingIcon changes the tray icon to indicate whether we're actively listening.
func SetRecordingIcon(isRecording bool) {
	if currentTray != nil {
		currentTray.SetRecordingIcon(isRecording)
	}
}
