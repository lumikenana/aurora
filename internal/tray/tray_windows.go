//go:build windows
// +build windows

package tray

import (
	"github.com/getlantern/systray"
	"github.com/lipipi/game-ai/internal/errors"
	"github.com/lipipi/game-ai/internal/log"
)

// windowsTray Windows系统托盘实现
type windowsTray struct {
	items             []*systray.MenuItem
	onShow            func()
	onToggleQuickInput func()
	onExitHandler     func()
	isRunning         bool
	done              chan struct{}
}

func createTray() Tray {
	return &windowsTray{
		done: make(chan struct{}),
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
	// 设置默认图标（可以后续替换）
	systray.SetTitle("GameAI")
	systray.SetTooltip("GameAI - AI游戏助手")

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
	item := systray.AddMenuItem(label, label)
	t.items = append(t.items, item)
	go func() {
		for range item.ClickedCh {
			onClick()
		}
	}()
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

	// 如果托盘已经初始化好了，立即添加菜单项
	if t.isRunning && t.onToggleQuickInput != nil {
		quickInputItem := systray.AddMenuItem("快速提问", "打开快速提问输入框 (快捷键 Ctrl+Alt+R)")
		go func() {
			for range quickInputItem.ClickedCh {
				t.onToggleQuickInput()
			}
		}()
	}
}
