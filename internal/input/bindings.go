package input

// Bindings Wails bindings 供前端调用
type Bindings struct {
}

// NewBindings 创建新的 bindings 实例
func NewBindings() *Bindings {
	return &Bindings{}
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
