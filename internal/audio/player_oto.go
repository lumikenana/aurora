package audio

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/ebitengine/oto/v3"
	"github.com/lipipi/aurora/internal/log"
)

// OtoPlayer 基于 Oto v3.4.1 的 PCM 流式播放器
type OtoPlayer struct {
	ctx       *oto.Context
	ctxCancel context.CancelFunc

	mu            sync.Mutex
	currentPlayer *oto.Player
	currentCancel context.CancelFunc
}

// NewOtoPlayer 创建 Oto 播放器
func NewOtoPlayer() (*OtoPlayer, error) {
	// Oto v3.4.1: 一个进程只能有一个 Context
	// 使用固定的 24kHz mono s16le 格式
	ctx, readyCh, err := oto.NewContext(&oto.NewContextOptions{
		SampleRate:   PCM16Mono24kHz.SampleRate,
		ChannelCount: PCM16Mono24kHz.Channels,
		Format:       oto.FormatSignedInt16LE,
	})
	if err != nil {
		return nil, fmt.Errorf("create oto context: %w", err)
	}

	// 等待 context 就绪
	<-readyCh

	log.Debugf("OtoPlayer initialized: %dHz/%dch/s16le",
		PCM16Mono24kHz.SampleRate, PCM16Mono24kHz.Channels)

	// 创建取消函数
	cancel := func() {
		// Oto Context 没有 Close 方法，但我们可以跟踪状态
		// 实际的清理会在进程结束时进行
	}

	return &OtoPlayer{
		ctx:       ctx,
		ctxCancel: cancel,
	}, nil
}

// PlayStream 流式播放音频
// 返回的 session 用于控制播放（Stop/Wait）
func (p *OtoPlayer) PlayStream(ctx context.Context, stream *Stream, onEnd func()) (*PlaybackSession, error) {
	// 验证格式
	if !p.isFormatSupported(stream.Format) {
		return nil, fmt.Errorf("unsupported audio format: %dHz/%dch, only %dHz/%dch/s16le supported",
			stream.Format.SampleRate, stream.Format.Channels,
			PCM16Mono24kHz.SampleRate, PCM16Mono24kHz.Channels)
	}

	// 创建播放会话
	session := &PlaybackSession{
		player:    p,
		stream:    stream,
		onEnd:     onEnd,
		doneCh:    make(chan struct{}),
		startTime: time.Now(),
	}

	// 创建 session context
	sessionCtx, sessionCancel := context.WithCancel(ctx)
	session.cancel = sessionCancel

	// 创建 Oto player
	otoPlayer := p.ctx.NewPlayer(stream.Reader)

	// 保存当前播放器引用
	p.mu.Lock()
	// 如果有之前的播放器，先停止
	if p.currentPlayer != nil {
		p.currentPlayer.Pause()
		p.currentPlayer.Close()
	}
	p.currentPlayer = otoPlayer
	p.currentCancel = sessionCancel
	p.mu.Unlock()

	// 启动播放
	otoPlayer.Play()
	log.Debugf("Oto playback started")

	// 启动监控 goroutine
	go session.monitor(sessionCtx, otoPlayer)

	return session, nil
}

// Stop 停止当前播放
func (p *OtoPlayer) Stop() {
	p.mu.Lock()
	player := p.currentPlayer
	cancel := p.currentCancel
	p.mu.Unlock()

	if player != nil {
		player.Pause()
	}
	if cancel != nil {
		cancel()
	}
}

// Close 关闭播放器
func (p *OtoPlayer) Close() error {
	p.ctxCancel()
	return nil
}

// isFormatSupported 检查格式是否支持
func (p *OtoPlayer) isFormatSupported(f Format) bool {
	return f.SampleRate == PCM16Mono24kHz.SampleRate &&
		f.Channels == PCM16Mono24kHz.Channels &&
		f.SampleFormat == PCM16Mono24kHz.SampleFormat
}

// PlaybackSession 播放会话
type PlaybackSession struct {
	player    *OtoPlayer
	stream    *Stream
	onEnd     func()
	cancel    context.CancelFunc
	doneCh    chan struct{}
	startTime time.Time

	onEndOnce sync.Once // 每个 session 独立的 sync.Once
}

// Stop 停止播放
func (s *PlaybackSession) Stop() {
	if s.cancel != nil {
		s.cancel()
	}
	if s.player != nil {
		s.player.Stop()
	}
}

// Wait 等待播放完成
func (s *PlaybackSession) Wait() {
	<-s.doneCh
}

// monitor 监控播放状态
// 负责检测播放完成并调用 onEnd（exactly once）
func (s *PlaybackSession) monitor(ctx context.Context, otoPlayer *oto.Player) {
	defer func() {
		// 清理 stream
		if s.stream.Close != nil {
			s.stream.Close()
		}

		// 确保 onEnd 只调用一次
		s.onEndOnce.Do(func() {
			if s.onEnd != nil {
				s.onEnd()
			}
		})

		// 通知 Wait() 完成
		close(s.doneCh)

		log.Debugf("Playback session ended: %v", time.Since(s.startTime))
	}()

	// 轮询检查播放状态
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			// Context 被取消（用户 Stop 或上层取消）
			log.Debugf("Playback cancelled by context")
			otoPlayer.Pause()
			return

		case <-ticker.C:
			// 检查播放状态
			if !otoPlayer.IsPlaying() {
				// 检查是否真的播放完成（而不是暂停）
				// Oto v3.4.1: IsPlaying() == false 意味着 EOF + buffer drained
				if otoPlayer.Err() != nil {
					log.Warnf("Playback error: %v", otoPlayer.Err())
				} else {
					log.Debugf("Playback completed naturally")
				}
				return
			}
		}
	}
}
