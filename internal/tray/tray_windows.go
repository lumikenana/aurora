//go:build windows
// +build windows

package tray

import (
	_ "embed"
	"sync"

	"github.com/getlantern/systray"
	"github.com/lipipi/aurora/internal/errors"
	"github.com/lipipi/aurora/internal/log"
)

// windowsTray Windows系统托盘实现
type windowsTray struct {
	items             []*systray.MenuItem
	// subMenus parent label → 父菜单项（用于创建子菜单）
	subMenus map[string]*systray.MenuItem
	// childItems parent label → child label → 子菜单项（用于 Check/Uncheck）
	childItems map[string]map[string]*systray.MenuItem
	mu                sync.Mutex
	onShow            func()
	onToggleQuickInput func()
	onExitHandler     func()
	isRunning         bool
	ready             chan struct{}
	done              chan struct{}
	readyOnce         sync.Once
}

//go:embed build/windows/icon.ico
var defaultIcon []byte

// micIcon is left empty - will use default icon if not available
var micIcon []byte

func createTray() Tray {
	return &windowsTray{
		ready:      make(chan struct{}),
		done:       make(chan struct{}),
		subMenus:   make(map[string]*systray.MenuItem),
		childItems: make(map[string]map[string]*systray.MenuItem),
	}
}

func (t *windowsTray) Init() error {
	log.Debug("Initializing Windows system tray...")

	go func() {
		systray.Run(func() {
			t.onReady()
		}, func() {
			t.handleOnExit()
		})
		close(t.done)
	}()

	t.isRunning = true
	return nil
}

// waitReady 等待托盘就绪（systray 的消息循环在另一个 goroutine 中启动，
// 在就绪前调用 AddMenuItem 会失败）
func (t *windowsTray) waitReady() {
	select {
	case <-t.ready:
	case <-t.done:
	}
}

func (t *windowsTray) Shutdown() error {
	if !t.isRunning {
		return nil
	}
	log.Debug("Shutting down system tray...")
	systray.Quit()
	<-t.done
	t.isRunning = false
	return nil
}

func (t *windowsTray) onReady() {
	// 通知等待者托盘已就绪
	t.readyOnce.Do(func() { close(t.ready) })

	// 设置默认图标
	systray.SetIcon(defaultIcon)
	systray.SetTitle("Aurora")
	systray.SetTooltip("Aurora - AI 助手")

	// 添加默认菜单项
	if t.onShow != nil {
		showItem := systray.AddMenuItem("显示主窗口", "显示主窗口")
		go func() {
			for range showItem.ClickedCh {
				t.onShow()
			}
		}()
	}

	// 总是添加快速提问菜单项，点击时检查回调是否存在
	quickInputItem := systray.AddMenuItem("快速提问", "打开快速提问输入框 (快捷键 Ctrl+Alt+R)")
	go func() {
		for range quickInputItem.ClickedCh {
			if t.onToggleQuickInput != nil {
				t.onToggleQuickInput()
			}
		}
	}()

	systray.AddSeparator()

	if t.onExitHandler != nil {
		exitItem := systray.AddMenuItem("退出", "退出应用")
		go func() {
			for range exitItem.ClickedCh {
				t.onExitHandler()
			}
		}()
	}
}

func (t *windowsTray) handleOnExit() {
	log.Debug("Tray onExit called")
	if t.onExitHandler != nil {
		t.onExitHandler()
	}
}

func (t *windowsTray) Show() error {
	// Systray默认就是显示的
	return nil
}

func (t *windowsTray) Hide() error {
	// 不支持隐藏图标
	return errors.New(errors.CodeTrayError, "Hiding tray icon not supported")
}

func (t *windowsTray) SetTooltip(tooltip string) error {
	systray.SetTooltip(tooltip)
	return nil
}

func (t *windowsTray) AddMenuItem(label string, onClick func()) error {
	t.waitReady()
	item := systray.AddMenuItem(label, label)
	t.items = append(t.items, item)
	go func() {
		for range item.ClickedCh {
			onClick()
		}
	}()
	return nil
}

func (t *windowsTray) AddSubMenu(parentLabel, childLabel string, onClick func()) error {
	t.waitReady()
	t.mu.Lock()
	defer t.mu.Unlock()

	// 获取或创建父菜单（作为顶层菜单项）
	parent, ok := t.subMenus[parentLabel]
	if !ok {
		parent = systray.AddMenuItem(parentLabel, parentLabel)
		t.subMenus[parentLabel] = parent
	}

	// 在父菜单下创建子项
	child := parent.AddSubMenuItem(childLabel, childLabel)
	if t.childItems[parentLabel] == nil {
		t.childItems[parentLabel] = make(map[string]*systray.MenuItem)
	}
	t.childItems[parentLabel][childLabel] = child

	go func() {
		for range child.ClickedCh {
			onClick()
		}
	}()
	return nil
}

func (t *windowsTray) SetMenuItemChecked(parentLabel, childLabel string, checked bool) error {
	t.waitReady()
	t.mu.Lock()
	defer t.mu.Unlock()
	items, ok := t.childItems[parentLabel]
	if !ok {
		return errors.New(errors.CodeTrayError, "sub menu not found: "+parentLabel)
	}
	item, ok := items[childLabel]
	if !ok {
		return errors.New(errors.CodeTrayError, "menu item not found: "+childLabel)
	}
	if checked {
		item.Check()
	} else {
		item.Uncheck()
	}
	return nil
}

func (t *windowsTray) AddSeparator() error {
	systray.AddSeparator()
	return nil
}

func (t *windowsTray) SetOnShowHandler(onShow func()) {
	t.onShow = onShow
}

func (t *windowsTray) SetOnExitHandler(onExit func()) {
	t.onExitHandler = onExit
}

func (t *windowsTray) SetOnToggleQuickInputHandler(onToggle func()) {
	t.onToggleQuickInput = onToggle
}

// SetRecordingIcon changes the tray icon to indicate whether we're actively listening.
// true = change to microphone icon (recording/listening active)
// false = change back to default icon
func (t *windowsTray) SetRecordingIcon(isRecording bool) {
	t.waitReady()
	if isRecording && len(micIcon) > 0 {
		systray.SetIcon(micIcon)
		systray.SetTooltip("Aurora - 持续监听唤醒词...")
	} else {
		systray.SetIcon(defaultIcon)
		systray.SetTooltip("Aurora - AI 助手")
	}
}
