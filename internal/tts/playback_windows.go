//go:build windows
// +build windows

package tts

import (
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"unsafe"

	"github.com/lipipi/aurora/internal/log"
)

var (
	winmm                  = syscall.NewLazyDLL("winmm.dll")
	procMCISendStringW     = winmm.NewProc("mciSendStringW")
	procMCIGetErrorStringW = winmm.NewProc("mciGetErrorStringW")
)

// mciGetErrorString 将 MCI 错误码转为人类可读字符串。
// 便于日志排查（如 289 = MCIERR_CANNOT_LOAD_DRIVER）。
func mciGetErrorString(code uintptr) string {
	buf := make([]uint16, 256)
	procMCIGetErrorStringW.Call(
		code,
		uintptr(unsafe.Pointer(&buf[0])),
		uintptr(len(buf)),
	)
	return syscall.UTF16ToString(buf)
}

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
		errMsg := mciGetErrorString(ret)
		return fmt.Errorf("mciSendString failed: command=%q, error_code=%d (%s)", cmd, ret, errMsg)
	}
	return nil
}

// mciSendStringSimple 不取返回值的简化版本
func mciSendStringSimple(cmd string) {
	if err := mciSendString(cmd); err != nil {
		log.Debugf("mciSendString error: %v (cmd: %s)", err, cmd)
	}
}

// probeMPEGVideo 检测 mpegvideo MCI 驱动是否可用。
// 在 Windows N/KN 版本或驱动注册表损坏时返回 false。
// 用于初始化阶段选择可靠的音频格式（mp3 → wav 降级）。
func probeMPEGVideo() bool {
	// 尝试打开一个不存在的文件，只检测驱动是否能加载。
	// 直接调用 syscall 取原始错误码，避免被 mciSendString 的错误包装干扰。
	const mciErrCannotLoadDriver = uintptr(289)
	cmd := `open "aurora_probe_null_.mp3" type mpegvideo alias auroraprobe`
	cmdPtr, _ := syscall.UTF16PtrFromString(cmd)
	var retBuf [128]uint16
	ret, _, _ := procMCISendStringW.Call(
		uintptr(unsafe.Pointer(cmdPtr)),
		uintptr(unsafe.Pointer(&retBuf[0])),
		uintptr(len(retBuf)),
		0,
	)
	if ret == 0 {
		// 极少见：probe 文件恰好存在并打开了，关闭它
		mciSendStringSimple("close auroraprobe")
		return true
	}
	if ret == mciErrCannotLoadDriver {
		log.Debugf("MCI mpegvideo probe: driver cannot be loaded (error 289)")
		return false
	}
	// 其他错误（如文件不存在 = 276）说明驱动本身可用，只是文件路径无效
	log.Debugf("MCI mpegvideo probe: driver available (non-fatal error %d: %s)",
		ret, mciGetErrorString(ret))
	return true
}

// mciTypeForFormat 返回音频格式对应的 MCI 设备类型。
// wav → waveaudio（基础驱动，几乎所有 Windows 版本都有）
// mp3 → mpegvideo（需要 MPEG 驱动，Windows N/KN 版本可能缺失）
// 兼容 filepath.Ext 输出（带前导点）和裸格式名（如 config 中的 "wav"）。
func mciTypeForFormat(format string) string {
	format = strings.TrimPrefix(format, ".")
	format = strings.ToLower(format)
	if format == "wav" {
		return "waveaudio"
	}
	return "mpegvideo"
}

// windowsPlayer Windows MCI 播放器实现
//
// 使用 winmm.dll 的 mciSendStringW API 播放 MP3/WAV 文件。
// 优点：无需新依赖；Windows 内置；支持 mpegvideo（MP3）。
// 初始化时探测 mpegvideo 驱动可用性，不可用则自动降级为 WAV。
type windowsPlayer struct {
	BasePlayer
	stopChan        chan struct{} // 通知播放 goroutine 退出
	aliasMu         sync.Mutex    // 保护 aliasOpen
	aliasOpen       bool          // auroratts alias 当前是否打开
	preferredFormat string        // "mp3" 或 "wav"（根据 mpegvideo 驱动可用性）
}

// newWindowsPlayer 创建 Windows 播放器
func newWindowsPlayer() *windowsPlayer {
	return &windowsPlayer{}
}

// NewPlatformPlayer 在 Windows 平台返回 Windows 播放器（mciSendString 实现）。
// 初始化时探测 mpegvideo 驱动：
// - 可用 → preferredFormat = "mp3"
// - 不可用（如 Windows N/KN 版本） → preferredFormat = "wav"，调用方应据此调整音频输出格式
func NewPlatformPlayer() Player {
	p := newWindowsPlayer()
	if probeMPEGVideo() {
		p.preferredFormat = "mp3"
		log.Debugf("TTS player: mpegvideo driver available, preferred format=mp3")
	} else {
		p.preferredFormat = "wav"
		log.Warnf("TTS player: mpegvideo driver NOT available (error 289 MCIERR_CANNOT_LOAD_DRIVER). " +
			"Falling back to wav/waveaudio. This is common on Windows N/KN editions. " +
			"Install 'Media Feature Pack' to restore MP3 playback.")
	}
	return p
}

// PreferredFormat 返回播放器偏好的音频格式（"mp3" 或 "wav"）。
// 调用方（如 volcengine TTS provider）应在初始化时读取此值并据此设置音频输出格式，
// 避免合成出播放器无法解码的格式。
func (p *windowsPlayer) PreferredFormat() string {
	if p.preferredFormat == "" {
		return "mp3" // 未初始化时的安全默认
	}
	return p.preferredFormat
}

// closeAliasIfOpen 幂等关闭 auroratts alias，避免对未打开的设备发命令触发 263
func (p *windowsPlayer) closeAliasIfOpen() {
	p.aliasMu.Lock()
	open := p.aliasOpen
	p.aliasOpen = false
	p.aliasMu.Unlock()
	if open {
		mciSendStringSimple("close auroratts")
	}
}

// stopAliasIfOpen 同上，先 stop 再 close（用于中断正在播放的情况）
func (p *windowsPlayer) stopAliasIfOpen() {
	p.aliasMu.Lock()
	open := p.aliasOpen
	p.aliasOpen = false
	p.aliasMu.Unlock()
	if open {
		mciSendStringSimple("stop auroratts")
		mciSendStringSimple("close auroratts")
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

	// 根据文件扩展名选择 MCI 设备类型：
	// .wav → waveaudio（基础驱动，几乎所有 Windows 版本都有）
	// .mp3 → mpegvideo（需要 MPEG 驱动，Windows N/KN 可能缺失）
	ext := filepath.Ext(filePath) // mciTypeForFormat 内部处理点前缀和大小写
	mciType := mciTypeForFormat(ext)

	// 打开文件
	openCmd := fmt.Sprintf(`open "%s" type %s alias auroratts`, filePath, mciType)
	if err := mciSendString(openCmd); err != nil {
		p.SetPlaying(false)
		return fmt.Errorf("failed to open audio file (%s): %w", mciType, err)
	}
	p.aliasMu.Lock()
	p.aliasOpen = true
	p.aliasMu.Unlock()

	// 启动播放 goroutine
	go func() {
		// play with wait 阻塞直到播放完成或 stop
		done := make(chan error, 1)
		go func() {
			done <- mciSendString("play auroratts wait")
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
