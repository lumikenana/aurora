//go:build windows
// +build windows

package input

import (
	"syscall"
	"time"

	"github.com/lipipi/aurora/internal/log"
)

var (
	user32PTT            = syscall.NewLazyDLL("user32.dll")
	procGetAsyncKeyState = user32PTT.NewProc("GetAsyncKeyState")
)

// Windows 虚拟键码（修饰键）
const (
	vkControl = 0x11
	vkShift   = 0x10
	vkMenu    = 0x12 // Alt
	vkLWin    = 0x5B
	vkRWin    = 0x5C
)

// pollKeyRelease 轮询检测 PTT 组合键释放
// 使用 GetAsyncKeyState 检测配置的 PTT 组合（默认 Ctrl+Shift+R）是否释放。
// 只在录音期间运行（50ms 间隔），开销极低。
func (m *PTTManager) pollKeyRelease() {
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-m.done:
			return
		case <-ticker.C:
			if !m.isPTTHeld() {
				log.Debug("PTT key released, stopping recording")
				m.StopRecording()
				return
			}
		}
	}
}

// isPTTHeld 检测 PTT 组合键是否仍被按住
// GetAsyncKeyState 返回值的最高位为 1 表示按键处于按下状态
func (m *PTTManager) isPTTHeld() bool {
	m.mu.Lock()
	mods, vk := m.comboMods, m.comboVK
	m.mu.Unlock()

	if vk == 0 {
		return false
	}
	if mods&ModCtrl != 0 && !isKeyDown(vkControl) {
		return false
	}
	if mods&ModShift != 0 && !isKeyDown(vkShift) {
		return false
	}
	if mods&ModAlt != 0 && !isKeyDown(vkMenu) {
		return false
	}
	if mods&ModWin != 0 && !(isKeyDown(vkLWin) || isKeyDown(vkRWin)) {
		return false
	}
	return isKeyDown(vk)
}

func isKeyDown(vk int) bool {
	state, _, _ := procGetAsyncKeyState.Call(uintptr(vk))
	return int16(state) < 0
}
