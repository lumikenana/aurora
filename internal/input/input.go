// Package input 输入模块 - 处理全局快捷键、语音唤醒、用户输入
package input

import (
	"sync"

	"github.com/lipipi/aurora/internal/errors"
	"github.com/lipipi/aurora/internal/log"
)

// SourceType 输入来源类型
type SourceType string

const (
	// SourceText 文本输入（快捷键呼出输入框）
	SourceText SourceType = "text"
	// SourceVoice 语音输入（唤醒词或PTT）
	SourceVoice SourceType = "voice"
)

// UserInput 用户输入结构
type UserInput struct {
	// Text 输入文本
	Text string `json:"text"`
	// Source 输入来源
	Source SourceType `json:"source"`
}

// InputCallback 用户输入完成回调
type InputCallback func(input UserInput)

// InputManager 输入管理器 - 统一管理所有输入方式
type InputManager struct {
	hotkeyListener HotkeyListener
	// callbacks 保护 cbMu：NotifyInput 在热键/PTT 消息线程触发，
	// SetCallback 可能在 Wails binding goroutine（SaveLLMConfig）触发。
	cbMu      sync.RWMutex
	callbacks []InputCallback
	isRunning bool
}

var (
	manager *InputManager
)

// Init 初始化输入管理器
func Init() errors.AppError {
	log.Debug("Initializing input manager...")

	manager = &InputManager{
		hotkeyListener: createHotkeyListener(),
		callbacks:      make([]InputCallback, 0),
	}

	if err := manager.hotkeyListener.Init(); err != nil {
		return errors.Wrap(errors.CodeUnknown, "Failed to initialize hotkey listener", err)
	}

	manager.isRunning = true
	log.Debug("Input manager initialized")
	return nil
}

// Shutdown 关闭输入管理器
func Shutdown() {
	if manager == nil {
		return
	}

	log.Debug("Shutting down input manager...")
	manager.hotkeyListener.Shutdown()
	manager.isRunning = false
	log.Debug("Input manager shutdown complete")
}

// Current 获取输入管理器实例
func Current() *InputManager {
	return manager
}

// SetCallback 设置输入完成回调（替换语义，非追加）。
//
// 当前唯一的消费者是 Agent Core，且 Agent 可被 SaveLLMConfig 整体重建。
// 必须用"替换"保证运行时始终只有当前有效 Agent 接收用户输入 ——
// 若用追加语义，重建后的旧 Agent 回调会残留，一条输入被多个 Agent
// 重复处理（双 LLM 调用 / 双 chatUserMessage / Overlay 重复显示）。
func (m *InputManager) SetCallback(callback InputCallback) {
	m.cbMu.Lock()
	defer m.cbMu.Unlock()
	m.callbacks = []InputCallback{callback}
}

// NotifyInput 通知有新输入
func (m *InputManager) NotifyInput(input UserInput) {
	log.Debugf("New user input from %s: %d chars", input.Source, len(input.Text))
	m.cbMu.RLock()
	callbacks := make([]InputCallback, len(m.callbacks))
	copy(callbacks, m.callbacks)
	m.cbMu.RUnlock()
	for _, cb := range callbacks {
		cb(input)
	}
}

// GetHotkeyListener 获取快捷键监听器
func (m *InputManager) GetHotkeyListener() HotkeyListener {
	return m.hotkeyListener
}

// IsRunning 是否正在运行
func (m *InputManager) IsRunning() bool {
	return m.isRunning
}
