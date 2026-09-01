//go:build windows
// +build windows

package platform

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"unsafe"

	"github.com/lipipi/aurora/internal/errors"
	"github.com/lipipi/aurora/internal/log"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

// windowsPlatform Windows平台实现
type windowsPlatform struct {
}

const (
	autoStartKeyPath = `Software\Microsoft\Windows\CurrentVersion\Run`
	autoStartValue   = "Aurora"
	// legacyAutoStartValue 旧版 GameAI 注册表值名；切换自启动时主动清理，
	// 避免升级后出现 "GameAI → 旧 exe" 与 "Aurora → 新 exe" 两条并存。
	legacyAutoStartValue = "GameAI"
)

func createPlatform() Platform {
	return &windowsPlatform{}
}

func (p *windowsPlatform) Init() error {
	log.Info("Running on Windows platform")
	return nil
}

func (p *windowsPlatform) Shutdown() error {
	return nil
}

func (p *windowsPlatform) IsWindows() bool {
	return true
}

func (p *windowsPlatform) GetPlatformName() string {
	return "windows"
}

func (p *windowsPlatform) SetAutoStart(enable bool) error {
	key, err := registry.OpenKey(registry.CURRENT_USER, autoStartKeyPath, registry.SET_VALUE)
	if err != nil {
		return errors.Wrap(errors.CodePlatformError, "Failed to open registry key", err)
	}
	defer key.Close()

	// 无论启用 / 禁用，都先清理旧版 "GameAI" 值，避免升级后两条自启项并存。
	deleteValueSilently(key, legacyAutoStartValue)

	if !enable {
		// 禁用时同时清理新版 "Aurora"。
		deleteValueSilently(key, autoStartValue)
		return nil
	}

	exePath, err := os.Executable()
	if err != nil {
		return errors.Wrap(errors.CodePlatformError, "Failed to get executable path", err)
	}
	exePath, err = filepath.Abs(exePath)
	if err != nil {
		return errors.Wrap(errors.CodePlatformError, "Failed to get absolute path", err)
	}

	err = key.SetStringValue(autoStartValue, fmt.Sprintf(`"%s"`, exePath))
	if err != nil {
		return errors.Wrap(errors.CodePlatformError, "Failed to set auto-start registry value", err)
	}

	log.Debugf("Auto-start enabled: %s=%s", autoStartValue, exePath)
	return nil
}

func (p *windowsPlatform) IsAutoStart() (bool, error) {
	key, err := registry.OpenKey(registry.CURRENT_USER, autoStartKeyPath, registry.QUERY_VALUE)
	if err != nil {
		if isRegistryKeyNotFound(err) {
			return false, nil
		}
		return false, errors.Wrap(errors.CodePlatformError, "Failed to open registry key", err)
	}
	defer key.Close()

	// 优先查新版 "Aurora"；兼容旧版 "GameAI"，避免升级后用户看到自启状态跳变。
	if ok, err := valueExists(key, autoStartValue); err != nil {
		return false, errors.Wrap(errors.CodePlatformError, "Failed to read registry value", err)
	} else if ok {
		return true, nil
	}
	if ok, err := valueExists(key, legacyAutoStartValue); err != nil {
		return false, errors.Wrap(errors.CodePlatformError, "Failed to read registry value", err)
	} else if ok {
		return true, nil
	}
	return false, nil
}

// deleteValueSilently 删除注册表值；不存在时静默忽略。
func deleteValueSilently(key registry.Key, name string) {
	if err := key.DeleteValue(name); err != nil && !isRegistryKeyNotFound(err) {
		log.Warnf("Failed to delete registry value %q: %v", name, err)
	}
}

// valueExists 查询指定注册表值是否存在（仅用于自启状态判定）。
func valueExists(key registry.Key, name string) (bool, error) {
	_, _, err := key.GetStringValue(name)
	if err == nil {
		return true, nil
	}
	if isRegistryKeyNotFound(err) {
		return false, nil
	}
	return false, err
}

func (p *windowsPlatform) OpenURL(url string) error {
	// 使用ShellExecute打开URL
	user32 := syscall.NewLazyDLL("shell32.dll")
	procShellExecute := user32.NewProc("ShellExecuteW")

	uri, err := windows.UTF16PtrFromString(url)
	if err != nil {
		return errors.Wrap(errors.CodePlatformError, "Failed to encode URL", err)
	}

	ret, _, _ := procShellExecute.Call(
		0,
		uintptr(unsafe.Pointer(windows.StringToUTF16Ptr("open"))),
		uintptr(unsafe.Pointer(uri)),
		0,
		0,
		1,
	)

	if ret <= 32 {
		return errors.Newf(errors.CodePlatformError, "ShellExecute failed with return code %d", ret)
	}

	return nil
}

func (p *windowsPlatform) MinimizeToTray() error {
	log.Debug("MinimizeToTray called on Windows")
	// 实际实现需要与Wails窗口交互，这里只做占位
	return nil
}

func (p *windowsPlatform) RestoreFromTray() error {
	log.Debug("RestoreFromTray called on Windows")
	// 实际实现需要与Wails窗口交互，这里只做占位
	return nil
}

// isRegistryKeyNotFound 检查是否是键不存在错误
func isRegistryKeyNotFound(err error) bool {
	if err == nil {
		return false
	}
	return err == registry.ErrNotExist
}
