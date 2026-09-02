package tts

import "sync"

// PlaybackEndCallback 播放结束回调
type PlaybackEndCallback func()

// Player 音频播放接口
//
// 抽象播放器实现：Windows 用 mciSendString，
// 其他平台可空实现或用其他方式。
type Player interface {
	// Play 播放指定路径的音频文件，异步执行
	// 播放完成时调用 onEnd（如果已注册）
	// 如果当前正在播放，会先停止
	Play(filePath string) error

	// Stop 停止当前播放
	Stop() error

	// IsPlaying 是否正在播放
	IsPlaying() bool

	// SetOnEnd 设置播放结束回调（异步播放结束时调用）
	SetOnEnd(cb PlaybackEndCallback)

	// PreferredFormat 返回播放器偏好的音频格式（如 "mp3" / "wav"）。
	// 调用方（如 TTS provider）应在初始化时读取此值，
	// 按此格式生成音频文件，避免合成出播放器无法解码的格式。
	PreferredFormat() string
}

// BasePlayer 提供共享的 mutex + 状态 + onEnd 回调机制
type BasePlayer struct {
	mu      sync.Mutex
	playing bool
	OnEnd   PlaybackEndCallback
}

// SetOnEnd 设置播放结束回调
func (p *BasePlayer) SetOnEnd(cb PlaybackEndCallback) {
	p.mu.Lock()
	p.OnEnd = cb
	p.mu.Unlock()
}

// SetPlaying 设置播放状态
func (p *BasePlayer) SetPlaying(v bool) {
	p.mu.Lock()
	p.playing = v
	p.mu.Unlock()
}

// IsPlaying 获取播放状态
func (p *BasePlayer) IsPlaying() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.playing
}

// NotifyEnd 通知播放结束（播放 goroutine 调用）
func (p *BasePlayer) NotifyEnd() {
	p.SetPlaying(false)
	if p.OnEnd != nil {
		p.OnEnd()
	}
}
