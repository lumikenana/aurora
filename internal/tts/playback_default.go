//go:build !windows
// +build !windows

package tts

import "github.com/lipipi/aurora/internal/log"

// defaultPlayer 非 Windows 平台空实现
type defaultPlayer struct {
	BasePlayer
}

func newDefaultPlayer() *defaultPlayer {
	return &defaultPlayer{}
}

// NewPlatformPlayer 在非 Windows 平台返回 no-op 播放器
func NewPlatformPlayer() Player {
	return newDefaultPlayer()
}

// Play 非 Windows 平台不支持 TTS 播放
func (p *defaultPlayer) Play(filePath string) error {
	log.Warnf("TTS playback not supported on this platform, ignoring file: %s", filePath)
	// 仍然通知结束，让上层流程不卡住
	go p.NotifyEnd()
	return nil
}

// Stop 非 Windows 平台空实现
func (p *defaultPlayer) Stop() error {
	return nil
}

// PreferredFormat 非 Windows 平台默认返回 mp3
func (p *defaultPlayer) PreferredFormat() string {
	return "mp3"
}
