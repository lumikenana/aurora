package input

import (
	"github.com/lipipi/aurora/internal/errors"
	"github.com/lipipi/aurora/internal/log"
)

// QuickInputState 快速输入窗口状态（由 driver 决定真实窗口的可见性）
type QuickInputState int

const (
	// QuickInputHidden 隐藏状态
	QuickInputHidden QuickInputState = 0
	// QuickInputVisible 可见状态
	QuickInputVisible QuickInputState = 1
)

// QuickInputDriver 实际驱动 native window 的接口。
//
// Phase 8E：QuickInputManager 本身不知道任何 Wails v3 / native window 细节；
// 真实窗口的 Show/Hide 由注入的 driver 实现（main.go 提供，桥接到
// internal/wailsruntime 的 QuickInputWindow 句柄）。
//
// 这样：
//   - internal/input 不依赖 wails/v3（CLAUDE.md §2 分层）
//   - QuickInputManager 的状态机（Show/Hide/Toggle/IsVisible/Submit/Cancel）
//     与 native window 之间的耦合只发生在 main.go 的 wiring
type QuickInputDriver interface {
	Show()
	Hide()
	IsVisible() bool
}

var (
	quickInputManager *QuickInputManager
)

// InitQuickInput 初始化快速输入管理器（state machine 自身；不创建窗口）
func InitQuickInput() *QuickInputManager {
	quickInputManager = &QuickInputManager{
		state: QuickInputHidden,
	}
	return quickInputManager
}

// GetQuickInput 获取快速输入管理器实例
func GetQuickInput() *QuickInputManager {
	return quickInputManager
}

// QuickInputManager 快速输入窗口管理器
//
// 职责：
//   - 维护状态机（Hidden/Visible）
//   - 接受外部 Show/Hide/Toggle/Submit/Cancel 调用
//   - 把 Show/Hide 委派给 driver（由 main.go 注入）
//   - 触发 onToggle 回调（前端不再监听，保留给其他可能的订阅者）
type QuickInputManager struct {
	state     QuickInputState
	driver    QuickInputDriver
	callbacks []func(UserInput)
	onToggle  []func(bool) // 切换窗口状态回调，参数为是否显示
}

// SetDriver 注入 native window driver。main.go 在 QuickInputWindow 创建后调一次。
// 未注入 driver 时 Show/Hide/Toggle/IsVisible 退化为只改状态、不动窗口。
func (m *QuickInputManager) SetDriver(d QuickInputDriver) {
	m.driver = d
}

// NewFuncDriver 便于 main.go 用三个函数直接拼出一个 QuickInputDriver。
// 避免在 main.go 写一个空结构体再为每个方法赋函数字面量。
//
// 用法（main.go）：
//
//	quickInput.SetDriver(input.NewFuncDriver(
//	    wailsruntime.ShowQuickInputWindow,
//	    wailsruntime.HideQuickInputWindow,
//	    func() bool { return wailsruntime.QuickInputWindow != nil && wailsruntime.QuickInputWindow.IsVisible() },
//	))
func NewFuncDriver(show, hide func(), isVisible func() bool) QuickInputDriver {
	return &funcDriver{show: show, hide: hide, isVisible: isVisible}
}

type funcDriver struct {
	show      func()
	hide      func()
	isVisible func() bool
}

func (d *funcDriver) Show()          { d.show() }
func (d *funcDriver) Hide()          { d.hide() }
func (d *funcDriver) IsVisible() bool { return d.isVisible() }

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
	if m.driver != nil {
		m.driver.Show()
	}
	log.Debug("QuickInput window shown")
	m.notifyToggle(true)
}

// Hide 隐藏快速输入窗口
func (m *QuickInputManager) Hide() {
	if m.state == QuickInputHidden {
		return
	}
	m.state = QuickInputHidden
	if m.driver != nil {
		m.driver.Hide()
	}
	log.Debug("QuickInput window hidden")
	m.notifyToggle(false)
}

// IsVisible 判断窗口是否可见
func (m *QuickInputManager) IsVisible() bool {
	// 优先以 driver 真实窗口状态为准（与 native 保持单一真相），
	// driver 未注入时退回到状态机自身值。
	if m.driver != nil {
		return m.driver.IsVisible()
	}
	return m.state == QuickInputVisible
}

// Submit 提交用户输入。
//
// QuickInputManager.Submit 仅作为 native window 隐藏入口；真正的 LLM 提交
// 由前端 QuickInput 在 native webview 内直接调用 chatStream（与 8E 之前
// 架构不同：之前 QuickInput 通过 emit('submit') → App.vue.handleSubmit →
// chatStream；8E 之后 QuickInput 渲染在独立 webview，emit 链路不存在，
// 改为直接 chatStream）。
//
// 这里 Submit 仍保留是为了让 submitQuickInput / cancelQuickInput binding
// 路径仍能触发 native window 关闭。
func (m *QuickInputManager) Submit(text string) errors.AppError {
	if len(text) == 0 {
		m.Hide()
		return nil
	}

	// 通知所有回调（兼容旧 RegisterSubmitCallback 路径，main.go 当前未注册）
	input := UserInput{
		Text:   text,
		Source: SourceText,
	}
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

// RegisterSubmitCallback 注册提交回调（保留 API 兼容；当前 main.go 未使用）
func (m *QuickInputManager) RegisterSubmitCallback(callback func(UserInput)) {
	m.callbacks = append(m.callbacks, callback)
}

// RegisterToggleCallback 注册窗口切换回调（保留 API 兼容）
func (m *QuickInputManager) RegisterToggleCallback(callback func(bool)) {
	m.onToggle = append(m.onToggle, callback)
}

func (m *QuickInputManager) notifyToggle(show bool) {
	for _, cb := range m.onToggle {
		cb(show)
	}
}
