package input

import (
	"github.com/lipipi/game-ai/internal/errors"
	"github.com/lipipi/game-ai/internal/log"
)

// QuickInputState 快速输入窗口状态
type QuickInputState int

const (
	// QuickInputHidden 隐藏状态
	QuickInputHidden QuickInputState = 0
	// QuickInputVisible 可见状态
	QuickInputVisible QuickInputState = 1
)

// QuickInputManager 快速输入窗口管理器
type QuickInputManager struct {
	state     QuickInputState
	callbacks []func(UserInput)
	onToggle  []func(bool) // 切换窗口状态回调，参数为是否显示
}

var (
	quickInputManager *QuickInputManager
)

// InitQuickInput 初始化快速输入管理器
func InitQuickInput() *QuickInputManager {
	quickInputManager = &QuickInputManager{
		state:     QuickInputHidden,
		callbacks: make([]func(UserInput), 0),
		onToggle:  make([]func(bool), 0),
	}
	return quickInputManager
}

// GetQuickInput 获取快速输入管理器实例
func GetQuickInput() *QuickInputManager {
	return quickInputManager
}

// Toggle 切换快速输入窗口显示/隐藏
func (m *QuickInputManager) Toggle() {
	if m.state == QuickInputHidden {
		m.Show()
	} else {
		m.Hide()
	}
}

// Show 显示快速输入窗口
func (m *QuickInputManager) Show() {
	if m.state == QuickInputVisible {
		return
	}
	m.state = QuickInputVisible
	log.Debug("QuickInput window shown")
	m.notifyToggle(true)
}

// Hide 隐藏快速输入窗口
func (m *QuickInputManager) Hide() {
	if m.state == QuickInputHidden {
		return
	}
	m.state = QuickInputHidden
	log.Debug("QuickInput window hidden")
	m.notifyToggle(false)
}

// IsVisible 判断窗口是否可见
func (m *QuickInputManager) IsVisible() bool {
	return m.state == QuickInputVisible
}

// Submit 提交用户输入
func (m *QuickInputManager) Submit(text string) errors.AppError {
	if len(text) == 0 {
		m.Hide()
		return nil
	}

	input := UserInput{
		Text:   text,
		Source: SourceText,
	}

	// 通知所有回调
	for _, cb := range m.callbacks {
		cb(input)
	}

	// 提交后隐藏窗口
	m.Hide()
	return nil
}

// Cancel 取消输入
func (m *QuickInputManager) Cancel() {
	log.Debug("QuickInput canceled")
	m.Hide()
}

// RegisterSubmitCallback 注册提交回调
func (m *QuickInputManager) RegisterSubmitCallback(callback func(UserInput)) {
	m.callbacks = append(m.callbacks, callback)
}

// RegisterToggleCallback 注册窗口切换回调
func (m *QuickInputManager) RegisterToggleCallback(callback func(bool)) {
	m.onToggle = append(m.onToggle, callback)
}

func (m *QuickInputManager) notifyToggle(show bool) {
	for _, cb := range m.onToggle {
		cb(show)
	}
}
