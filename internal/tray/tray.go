// Package tray 系统托盘管理
//
// Phase 8D：从 getlantern/systray 迁移到 Wails v3 native app.SystemTray。
//
// 设计：
//   - main.go 在 v3App 创建之前调用本包的 SetOn*Handler / RegisterRadioItem
//     注册所有回调（早期返回 stubTray，stubTray 仅把回调存到包级变量，
//     BuildMenuAndAttach 时再实际绑定到 v3 MenuItem.OnClick）
//   - main.go 在 v3App 创建后调一次 BuildMenuAndAttach（仅 windows），
//     真正构造 v3 Menu 并 SetMenu 到 SystemTray
//   - BuildMenuAndAttach 之后 SetOverlayMenuState / SetRecordingIcon
//     通过 currentTray 实际生效
//
// macOS dev：BuildMenuAndAttach 是 no-op（不存在 windows-only 文件），
// currentTray 始终为 stubTray，所有方法为 no-op；与旧 default 行为一致。
package tray

import (
	"sync"

	"github.com/lipipi/aurora/internal/log"
)

// RadioItem 注册的 radio 子菜单项描述（仅 windows 实际生效）。
type RadioItem struct {
	ParentLabel string
	ItemLabel   string
	OnClick     func()
}

// Tray 系统托盘接口。
// main.go 在 v3App 创建前通过 Current().SetOnXxxHandler 注册回调；
// BuildMenuAndAttach 之后回调会绑到真实 v3 MenuItem。
type Tray interface {
	SetOnShowHandler(onShow func())
	SetOnExitHandler(onExit func())
	SetOnToggleQuickInputHandler(onToggle func())
	SetOnToggleOverlayHandler(onToggle func())
	// SetOverlayMenuState 动态更新 Overlay 切换菜单项的显示文本
	// visible=true → "隐藏 Overlay"，false → "显示 Overlay"
	SetOverlayMenuState(visible bool)
	// SetRecordingIcon 切换托盘图标（录音/唤醒态 ↔ 默认）
	// 历史实现里 mic 图标字节为空时仅切 tooltip，行为保持。
	SetRecordingIcon(isRecording bool)
}

var (
	mu sync.Mutex

	currentTray Tray

	// 由 stubTray.SetOn*Handler / Current() 返回 stub 之前累积；BuildMenuAndAttach 时读出
	onShowHandler       func()
	onExitHandler       func()
	onQuickInputHandler func()
	onOverlayHandler    func()

	// 由 main.go 在 voice mode 注册阶段调用 RegisterRadioItem 累积；
	// BuildMenuAndAttach 时按 ParentLabel 分组建 submenu
	radioItems []RadioItem

	// 最近一次 SetOverlayMenuState 的值；BuildMenuAndAttach 时作为初始文本
	overlayVisible bool
)

// Init 兼容旧调用（getlantern/systray 时代需要启动独立 goroutine + 消息循环）。
// v3 native 下空实现：v3 SystemTray 由 application 拥有，无需单独 init。
// 保留签名以避免修改 main.go 调用面。
func Init() error {
	log.Debug("tray.Init: no-op under Wails v3 (BuildMenuAndAttach handles setup)")
	return nil
}

// Shutdown 兼容旧调用。v3 SystemTray 由 v3App 销毁，无需手动 shutdown。
func Shutdown() error {
	log.Debug("tray.Shutdown: no-op under Wails v3 (app shutdown destroys tray)")
	return nil
}

// Current 返回当前 Tray。
// v3App 创建前返回 stubTray（仅累积回调，方法全为 no-op）；
// BuildMenuAndAttach 后返回真实 v3 实现。
// 永不返回 nil。
func Current() Tray {
	mu.Lock()
	defer mu.Unlock()
	if currentTray != nil {
		return currentTray
	}
	return stubTray{}
}

// SetRecordingIcon 包级快捷方式（与旧 API 兼容）。
func SetRecordingIcon(isRecording bool) {
	Current().SetRecordingIcon(isRecording)
}

// RegisterRadioItem 注册一个 radio 子菜单项。
// 同一 ParentLabel 多次调用会在同一 submenu 下创建多个 radio item。
// 仅在 BuildMenuAndAttach 时实际生效（windows-only）。
func RegisterRadioItem(parentLabel, itemLabel string, onClick func()) {
	mu.Lock()
	defer mu.Unlock()
	radioItems = append(radioItems, RadioItem{parentLabel, itemLabel, onClick})
}

// SetMenuItemChecked 设置已构建 submenu 中某 radio item 的勾选状态。
// BuildMenuAndAttach 之前的调用是 no-op（menu 还未构建）。
// 仅 windows 实现在 v3 MenuItem.SetChecked 上生效。
func SetMenuItemChecked(parentLabel, itemLabel string, checked bool) {
	mu.Lock()
	t := currentTray
	mu.Unlock()
	if t == nil {
		// menu 尚未构建（BuildMenuAndAttach 未调）；保持 silent no-op，
		// 等 BuildMenuAndAttach 之后由调用方（main.go voice 启动）再次设置。
		return
	}
	if r, ok := t.(interface {
		setRadioChecked(parentLabel, itemLabel string, checked bool)
	}); ok {
		r.setRadioChecked(parentLabel, itemLabel, checked)
	}
}

// stubTray v3App 创建前的占位实现：
//  - SetOn*Handler 累积回调到包级变量
//  - SetOverlayMenuState 累积状态
//  - SetRecordingIcon no-op
type stubTray struct{}

func (stubTray) SetOnShowHandler(f func())            { setHandler(&onShowHandler, f) }
func (stubTray) SetOnExitHandler(f func())            { setHandler(&onExitHandler, f) }
func (stubTray) SetOnToggleQuickInputHandler(f func()) {
	setHandler(&onQuickInputHandler, f)
}
func (stubTray) SetOnToggleOverlayHandler(f func())  { setHandler(&onOverlayHandler, f) }
func (stubTray) SetOverlayMenuState(visible bool)    { overlayVisible = visible }
func (stubTray) SetRecordingIcon(bool)               {}

func setHandler(dst *func(), f func()) {
	mu.Lock()
	*dst = f
	mu.Unlock()
}
