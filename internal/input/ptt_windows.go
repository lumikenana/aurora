//go:build windows
// +build windows

package input

import (
	"syscall"
	"time"

	"github.com/lipipi/game-ai/internal/log"
)

var (
	user32PTT            = syscall.NewLazyDLL("user32.dll")
	procGetAsyncKeyState = user32PTT.NewProc("GetAsyncKeyState")
)

// Windows 虚拟键码
const (
	vkControl = 0x11
	vkShift   = 0x10
	vkR       = 0x52 // 'R'
)

// pollKeyRelease 轮询检测 PTT 组合键释放
// 使用 GetAsyncKeyState 检测 Ctrl+Shift+R 释放
// 只在录音期间运行（50ms 间隔），开销极低
func (m *PTTManager) pollKeyRelease() {
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-m.done:
			return
		case <-ticker.C:
			if !isPTTHeld() {
				log.Debug("PTT key released, stopping recording")
				m.StopRecording()
				return
			}
		}
	}
}

// isPTTHeld 检测 Ctrl+Shift+R 是否仍被按住
// GetAsyncKeyState 返回值的最高位为 1 表示按键处于按下状态
func isPTTHeld() bool {
	ctrl, _, _ := procGetAsyncKeyState.Call(uintptr(vkControl))
	shift, _, _ := procGetAsyncKeyState.Call(uintptr(vkShift))
	r, _, _ := procGetAsyncKeyState.Call(uintptr(vkR))
	return int16(ctrl) < 0 && int16(shift) < 0 && int16(r) < 0
}
