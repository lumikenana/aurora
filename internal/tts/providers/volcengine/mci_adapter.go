package volcengine

import (
	"github.com/lipipi/aurora/internal/tts"
)

// mciPlayerAdapter 将 tts.Player 适配为 audio.MCIPlayer 接口
// Phase 19: 用于 PlaybackService 的 MCI fallback
type mciPlayerAdapter struct {
	player tts.Player
}

func (a *mciPlayerAdapter) Play(filePath string) error {
	return a.player.Play(filePath)
}

func (a *mciPlayerAdapter) Stop() error {
	return a.player.Stop()
}

func (a *mciPlayerAdapter) SetOnEnd(cb func()) {
	// tts.PlaybackEndCallback 就是 func()，直接转换
	a.player.SetOnEnd(tts.PlaybackEndCallback(cb))
}
