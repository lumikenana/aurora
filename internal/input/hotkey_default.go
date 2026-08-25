//go:build !windows
// +build !windows

package input

import (
	"github.com/lipipi/game-ai/internal/errors"
	"github.com/lipipi/game-ai/internal/log"
)

// defaultHotkeyListener 非Windows平台热键监听器空实现
type defaultHotkeyListener struct {
	hotkeys map[HotkeyID]Hotkey
	running bool
}

func createHotkeyListener() HotkeyListener {
	return &defaultHotkeyListener{
		hotkeys: make(map[HotkeyID]Hotkey),
		running: false,
	}
}

func (h *defaultHotkeyListener) Init() errors.AppError {
	log.Debug("Hotkey listener not implemented on this platform (stub)")
	h.running = true
	return nil
}

func (h *defaultHotkeyListener) Shutdown() {
	if !h.running {
		return
	}
	h.UnregisterAll()
	h.running = false
	log.Debug("Hotkey listener stub shutdown complete")
}

func (h *defaultHotkeyListener) RegisterHotkey(hotkey Hotkey) errors.AppError {
	log.Debugf("RegisterHotkey called (not implemented): ID=%d", hotkey.ID)
	h.hotkeys[hotkey.ID] = hotkey
	return nil
}

func (h *defaultHotkeyListener) UnregisterHotkey(id HotkeyID) errors.AppError {
	delete(h.hotkeys, id)
	return nil
}

func (h *defaultHotkeyListener) UnregisterAll() {
	h.hotkeys = make(map[HotkeyID]Hotkey)
}

func (h *defaultHotkeyListener) IsRegistered(id HotkeyID) bool {
	_, exists := h.hotkeys[id]
	return exists
}
