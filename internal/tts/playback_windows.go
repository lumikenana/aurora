//go:build windows
// +build windows

package tts

import (
	"fmt"
	"sync"
	"syscall"
	"unsafe"

	"github.com/lipipi/aurora/internal/log"
)

var (
	winmm             = syscall.NewLazyDLL("winmm.dll")
	procMCISendStringW = winmm.NewProc("mciSendStringW")
)

// mciSendString 调用 Windows MCI API 执行命令
// 返回 MCI 错误码（0 表示成功）
func mciSendString(cmd string) error {
	cmdPtr, err := syscall.UTF16PtrFromString(cmd)
	if err != nil {
		return fmt.Errorf("mciSendString: utf16 conversion failed: %w", err)
	}
	// 返回 buffer（很多命令返回空，所以用一个小的 buffer）
	var retBuf [128]uint16

	ret, _, _ := procMCISendStringW.Call(
		uintptr(unsafe.Pointer(cmdPtr)),
		uintptr(unsafe.Pointer(&retBuf[0])),
		uintptr(len(retBuf)),
		0,
	)
	if ret != 0 {
		return fmt.Errorf("mciSendString failed: command=%q, error_code=%d", cmd, ret)
	}
	return nil
}

// mciSendStringSimple 不取返回值的简化版本
func mciSendStringSimple(cmd string) {
	if err := mciSendString(cmd); err != nil {
		log.Debugf("mciSendString error: %v (cmd: %s)", err, cmd)
	}
}

// windowsPlayer Windows MCI 播放器实现
//
// 使用 winmm.dll 的 mciSendStringW API 播放 MP3/WAV 文件。
// 优点：无需新依赖；Windows 内置；支持 mpegvideo（MP3）。
type windowsPlayer struct {
	BasePlayer
	stopChan chan struct{} // 通知播放 goroutine 退出
	aliasMu  sync.Mutex    // 保护 aliasOpen
	aliasOpen bool         // gameaitts alias 当前是否打开
}

// newWindowsPlayer 创建 Windows 播放器
func newWindowsPlayer() *windowsPlayer {
	return &windowsPlayer{}
}

// NewPlatformPlayer 在 Windows 平台返回 Windows 播放器（mciSendString 实现）
func NewPlatformPlayer() Player {
	return newWindowsPlayer()
}

// closeAliasIfOpen 幂等关闭 gameaitts alias，避免对未打开的设备发命令触发 263
func (p *windowsPlayer) closeAliasIfOpen() {
	p.aliasMu.Lock()
	open := p.aliasOpen
	p.aliasOpen = false
	p.aliasMu.Unlock()
	if open {
		mciSendStringSimple("close gameaitts")
	}
}

// stopAliasIfOpen 同上，先 stop 再 close（用于中断正在播放的情况）
func (p *windowsPlayer) stopAliasIfOpen() {
	p.aliasMu.Lock()
	open := p.aliasOpen
	p.aliasOpen = false
	p.aliasMu.Unlock()
	if open {
		mciSendStringSimple("stop gameaitts")
		mciSendStringSimple("close gameaitts")
	}
}

// Play 异步播放文件
func (p *windowsPlayer) Play(filePath string) error {
	p.mu.Lock()
	// 如果正在播放，先停
	if p.playing {
		p.mu.Unlock()
		_ = p.Stop()
		p.mu.Lock()
	}
	p.playing = true
	p.stopChan = make(chan struct{})
	p.mu.Unlock()

	// 关闭之前可能打开的 alias（幂等）
	p.closeAliasIfOpen()

	// 打开文件
	openCmd := fmt.Sprintf(`open "%s" type mpegvideo alias gameaitts`, filePath)
	if err := mciSendString(openCmd); err != nil {
		p.SetPlaying(false)
		return fmt.Errorf("failed to open audio file: %w", err)
	}
	p.aliasMu.Lock()
	p.aliasOpen = true
	p.aliasMu.Unlock()

	// 启动播放 goroutine
	go func() {
		// play with wait 阻塞直到播放完成或 stop
		done := make(chan error, 1)
		go func() {
			done <- mciSendString("play gameaitts wait")
		}()

		select {
		case <-p.stopChan:
			// 被打断：立即停止（幂等）
			p.stopAliasIfOpen()
			p.NotifyEnd()
		case err := <-done:
			// 正常结束（幂等 close）
			p.closeAliasIfOpen()
			if err != nil {
				log.Debugf("TTS play ended with: %v", err)
			}
			p.NotifyEnd()
		}
	}()

	return nil
}

// Stop 停止当前播放
func (p *windowsPlayer) Stop() error {
	p.mu.Lock()
	if !p.playing {
		p.mu.Unlock()
		return nil
	}
	ch := p.stopChan
	p.mu.Unlock()

	// 通知播放 goroutine 退出
	select {
	case <-ch:
		// 已经关闭
	default:
		close(ch)
	}

	// 兜底：幂等 stop + close
	p.stopAliasIfOpen()
	p.SetPlaying(false)
	return nil
}
