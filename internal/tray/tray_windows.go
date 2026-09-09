//go:build windows
// +build windows

// Phase 8D：Wails v3 native app.SystemTray 实现。
// 旧 getlantern/systray 全部移除；v3 SystemTray 由 application 拥有，
// BuildMenuAndAttach 在 main.go v3App 创建后调一次。
package tray

import (
	_ "embed"
	"sync"

	"github.com/lipipi/aurora/internal/log"
	"github.com/wailsapp/wails/v3/pkg/application"
)

//go:embed build/windows/icon.ico
var defaultIcon []byte

// v3Tray Wails v3 native SystemTray 实现的薄包装。
// 内部持有 v3 *application.SystemTray + 已构建的 *application.Menu
// 以及 radio items 的 parent → label → *MenuItem 映射（用于 SetMenuItemChecked）。
type v3Tray struct {
	mu          sync.Mutex
	sysTray     *application.SystemTray
	menu        *application.Menu
	radioIndex  map[string]map[string]*application.MenuItem // parent → label → item
	isRecording bool
	hasMicIcon  bool
	overlayItem *application.MenuItem
}

func newV3Tray() *v3Tray {
	return &v3Tray{
		radioIndex: make(map[string]map[string]*application.MenuItem),
	}
}

// BuildMenuAndAttach 构造 v3 Menu 并 SetMenu 到 sysTray。
// 在 main.go v3App 创建后调一次（windows-only）。sysTray 由 application 拥有。
// 构造完成后 currentTray 切换为 v3Tray。
func BuildMenuAndAttach(sysTray *application.SystemTray) error {
	if sysTray == nil {
		return nil
	}
	menu := application.NewMenu()
	vt := newV3Tray()
	vt.sysTray = sysTray
	vt.menu = menu

	// 顶部菜单项
	if onShowHandler != nil {
		menu.Add("显示主窗口").OnClick(func(_ *application.Context) { onShowHandler() })
	}
	// 快速提问菜单（始终添加；handler nil 时不绑 callback）
	quickInputItem := menu.Add("快速提问")
	if onQuickInputHandler != nil {
		quickInputItem.OnClick(func(_ *application.Context) { onQuickInputHandler() })
	}
	// Overlay 切换菜单项 — 初始文本取最近一次 SetOverlayMenuState 的值
	vt.overlayItem = menu.Add(initialOverlayLabel())
	if onOverlayHandler != nil {
		cb := onOverlayHandler
		vt.overlayItem.OnClick(func(_ *application.Context) { cb() })
	}

	// 语音模式 submenu + radio items
	buildRadioSubmenus(vt, menu)

	// 退出（放最下面）
	menu.AddSeparator()
	if onExitHandler != nil {
		menu.Add("退出").OnClick(func(_ *application.Context) { onExitHandler() })
	}

	// 挂回 sysTray
	sysTray.SetMenu(menu)
	sysTray.SetIcon(defaultIcon)
	sysTray.SetTooltip("Aurora - AI 助手")

	// 切到 v3Tray
	mu.Lock()
	currentTray = vt
	mu.Unlock()

	log.Debug("System tray (Wails v3 native) built and attached")
	return nil
}

func buildRadioSubmenus(vt *v3Tray, root *application.Menu) {
	// 按 parentLabel 分组
	byParent := make(map[string][]RadioItem)
	order := []string{}
	for _, ri := range radioItems {
		if _, ok := byParent[ri.ParentLabel]; !ok {
			order = append(order, ri.ParentLabel)
		}
		byParent[ri.ParentLabel] = append(byParent[ri.ParentLabel], ri)
	}
	for _, parent := range order {
		items := byParent[parent]
		sub := root.AddSubmenu(parent)
		vt.radioIndex[parent] = make(map[string]*application.MenuItem, len(items))
		for _, ri := range items {
			mi := sub.AddRadio(ri.ItemLabel, false)
			vt.radioIndex[parent][ri.ItemLabel] = mi
			if ri.OnClick != nil {
				cb := ri.OnClick
				mi.OnClick(func(_ *application.Context) { cb() })
			}
		}
	}
}

func initialOverlayLabel() string {
	if overlayVisible {
		return "隐藏 Overlay"
	}
	return "显示 Overlay"
}

// ---- Tray interface impl ----

func (t *v3Tray) SetOnShowHandler(f func())           { setHandler(&onShowHandler, f) }
func (t *v3Tray) SetOnExitHandler(f func())           { setHandler(&onExitHandler, f) }
func (t *v3Tray) SetOnToggleQuickInputHandler(f func()) {
	setHandler(&onQuickInputHandler, f)
}
func (t *v3Tray) SetOnToggleOverlayHandler(f func())  { setHandler(&onOverlayHandler, f) }

func (t *v3Tray) SetOverlayMenuState(visible bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	overlayVisible = visible
	if t.overlayItem == nil {
		return
	}
	if visible {
		t.overlayItem.SetLabel("隐藏 Overlay")
	} else {
		t.overlayItem.SetLabel("显示 Overlay")
	}
}

// SetRecordingIcon 切换托盘图标（v3 实现：仅切 tooltip，mic 图标字节空
// 与旧 getlantern 行为保持一致）。
func (t *v3Tray) SetRecordingIcon(isRecording bool) {
	t.mu.Lock()
	t.isRecording = isRecording
	t.mu.Unlock()
	if t.sysTray == nil {
		return
	}
	if isRecording {
		t.sysTray.SetTooltip("Aurora - 持续监听唤醒词...")
	} else {
		t.sysTray.SetTooltip("Aurora - AI 助手")
	}
}

// setRadioChecked 内部方法（SetMenuItemChecked 调用）
func (t *v3Tray) setRadioChecked(parentLabel, itemLabel string, checked bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	items, ok := t.radioIndex[parentLabel]
	if !ok {
		return
	}
	mi, ok := items[itemLabel]
	if !ok {
		return
	}
	mi.SetChecked(checked)
}
