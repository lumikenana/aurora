// Package platform 提供平台相关抽象
package platform

import (
	"github.com/lipipi/game-ai/internal/log"
)

// Platform 平台接口定义
type Platform interface {
	// Init 初始化平台
	Init() error
	// Shutdown 关闭平台
	Shutdown() error
	// IsWindows 是否Windows平台
	IsWindows() bool
	// GetPlatformName 获取平台名称
	GetPlatformName() string
	// SetAutoStart 设置开机启动
	SetAutoStart(enable bool) error
	// IsAutoStart 是否开机启动
	IsAutoStart() (bool, error)
	// OpenURL 打开URL
	OpenURL(url string) error
	// MinimizeToTray 最小化到托盘
	MinimizeToTray() error
	// RestoreFromTray 从托盘恢复
	RestoreFromTray() error
}

var (
	currentPlatform Platform
)

// Init 初始化平台
func Init() {
	log.Debug("Initializing platform layer...")
	currentPlatform = createPlatform()
	if err := currentPlatform.Init(); err != nil {
		log.Errorf("Failed to initialize platform: %v", err)
	}
}

// Shutdown 关闭平台
func Shutdown() {
	if currentPlatform != nil {
		_ = currentPlatform.Shutdown()
	}
}

// Current 获取当前平台
func Current() Platform {
	return currentPlatform
}

// IsWindows 是否Windows平台
func IsWindows() bool {
	if currentPlatform == nil {
		return false
	}
	return currentPlatform.IsWindows()
}

// GetPlatformName 获取平台名称
func GetPlatformName() string {
	if currentPlatform == nil {
		return "unknown"
	}
	return currentPlatform.GetPlatformName()
}
