package input

import (
	"fmt"

	"github.com/lipipi/aurora/internal/log"
)

// Bindings Wails bindings 供前端调用
type Bindings struct {
	// emitEvent 事件发射器（main.go 注入 wailsruntime.EmitEvent 包装；
	// internal/input 不 import wails，保持解耦）
	emitEvent func(name string, data ...any)
	// persistShortcuts 快捷键持久化回调（main.go 注入，写 config.Shortcuts
	// 并 Save；internal/input 不 import config，避免循环依赖）
	persistShortcuts func(combos map[string]string)
}

// NewBindings 创建新的 bindings 实例
func NewBindings() *Bindings {
	return &Bindings{}
}

// SetEventEmitter 注入事件发射器（main.go 调用）
func (b *Bindings) SetEventEmitter(emit func(name string, data ...any)) {
	b.emitEvent = emit
}

// SetShortcutPersistence 注入快捷键持久化回调（main.go 调用）
func (b *Bindings) SetShortcutPersistence(persist func(combos map[string]string)) {
	b.persistShortcuts = persist
}

func (b *Bindings) emit(name string, data ...any) {
	if b.emitEvent != nil {
		b.emitEvent(name, data...)
	}
}

// FrontendEcho 前端回执（诊断用）：前端收到事件后调用，用于确认前后端事件链路
func (b *Bindings) FrontendEcho(event string) {
	log.Debugf("Frontend echo: %s", event)
}

// ToggleQuickInput 切换快速输入窗口
func (b *Bindings) ToggleQuickInput() bool {
	mgr := GetQuickInput()
	mgr.Toggle()
	return mgr.IsVisible()
}

// ShowQuickInput 显示快速输入窗口
func (b *Bindings) ShowQuickInput() {
	mgr := GetQuickInput()
	mgr.Show()
}

// HideQuickInput 隐藏快速输入窗口
func (b *Bindings) HideQuickInput() {
	mgr := GetQuickInput()
	mgr.Hide()
}

// IsQuickInputVisible 获取快速输入窗口是否可见
func (b *Bindings) IsQuickInputVisible() bool {
	mgr := GetQuickInput()
	return mgr.IsVisible()
}

// SubmitQuickInput 提交快速输入
func (b *Bindings) SubmitQuickInput(text string) error {
	mgr := GetQuickInput()
	return mgr.Submit(text)
}

// CancelQuickInput 取消快速输入
func (b *Bindings) CancelQuickInput() {
	mgr := GetQuickInput()
	mgr.Cancel()
}

// ---- 快捷键系统（UI 重构：统一 Registry + 可配置 + 冲突检测） ----

// GetShortcuts 返回当前生效的快捷键组合（action → combo）
func (b *Bindings) GetShortcuts() map[string]string {
	mgr := GetShortcutManager()
	if mgr == nil {
		// manager 未就绪（启动早期查询）：返回默认值
		return DefaultShortcuts()
	}
	return mgr.Combos()
}

// SetShortcut 设置单个快捷键（校验 + 冲突检测 + 重注册 + 持久化）。
// 冲突时返回错误（前端展示"已分配给 xxx"），不落盘。
func (b *Bindings) SetShortcut(action string, combo string) (map[string]string, error) {
	mgr := GetShortcutManager()
	if mgr == nil {
		return nil, fmt.Errorf("shortcut manager not ready")
	}
	if !IsValidShortcutAction(action) {
		return nil, fmt.Errorf("unknown shortcut action: %s", action)
	}

	if err := mgr.SetShortcut(ShortcutAction(action), combo); err != nil {
		return nil, err
	}

	// PTT 组合变更 → 同步释放轮询
	if action == string(ShortcutPTT) {
		if ptt := GetPTT(); ptt != nil {
			if mods, vk, ok := mgr.PTTCombo(); ok {
				ptt.SetPTTCombo(mods, vk)
			}
		}
	}

	combos := mgr.Combos()
	if b.persistShortcuts != nil {
		b.persistShortcuts(combos)
	}
	b.emit("shortcutsChanged", combos)
	return combos, nil
}

// ResetShortcuts 恢复全部快捷键为默认组合（重注册 + 持久化）
func (b *Bindings) ResetShortcuts() (map[string]string, error) {
	mgr := GetShortcutManager()
	if mgr == nil {
		return nil, fmt.Errorf("shortcut manager not ready")
	}
	for _, action := range AllShortcutActions {
		if err := mgr.ResetShortcut(action); err != nil {
			log.Warnf("Reset shortcut %s failed: %v", action, err)
		}
	}
	// PTT 释放轮询同步
	if ptt := GetPTT(); ptt != nil {
		if mods, vk, ok := mgr.PTTCombo(); ok {
			ptt.SetPTTCombo(mods, vk)
		}
	}

	if b.persistShortcuts != nil {
		b.persistShortcuts(DefaultShortcuts())
	}

	combos := mgr.Combos()
	b.emit("shortcutsChanged", combos)
	return combos, nil
}
