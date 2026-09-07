package input

import (
	"github.com/lipipi/aurora/internal/errors"
)

// HotkeyID 热键ID类型
type HotkeyID int

const (
	// HotkeyToggleQuickInput 切换快速输入热键（默认 Ctrl+Alt+R）
	HotkeyToggleQuickInput HotkeyID = 1
	// HotkeyPTT Push-To-Talk 热键（默认 Ctrl+Shift+R）
	HotkeyPTT HotkeyID = 2
	// HotkeyCycleVoiceMode 循环切换语音模式热键（默认 Ctrl+Alt+V：PTT → WakeWord → Continuous → PTT）
	HotkeyCycleVoiceMode HotkeyID = 3
)

// Modifier 修饰键定义（Windows虚拟键码）
type Modifier int

const (
	// ModAlt Alt键
	ModAlt Modifier = 1
	// ModCtrl Ctrl键
	ModCtrl Modifier = 2
	// ModShift Shift键
	ModShift Modifier = 4
	// ModWin Win键
	ModWin Modifier = 8
)

// Hotkey 热键定义
type Hotkey struct {
	ID       HotkeyID // 唯一ID
	Modifier Modifier // 修饰键
	Key      int      // 虚拟键码
	Callback func()   // 触发回调
}

// HotkeyListener 热键监听器接口
type HotkeyListener interface {
	// Init 初始化监听器（含消息循环线程）
	Init() errors.AppError
	// Shutdown 关闭监听器，释放所有热键
	Shutdown()
	// RegisterHotkey 注册热键
	RegisterHotkey(hotkey Hotkey) errors.AppError
	// UnregisterHotkey 注销热键
	UnregisterHotkey(id HotkeyID) errors.AppError
	// UnregisterAll 注销所有热键
	UnregisterAll()
	// IsRegistered 热键是否已注册
	IsRegistered(id HotkeyID) bool
}
