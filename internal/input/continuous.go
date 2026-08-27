package input

import (
	"context"
	"time"

	"github.com/lipipi/aurora/internal/errors"
	"github.com/lipipi/aurora/internal/log"
	"github.com/lipipi/aurora/internal/speech"
)

// startContinuousSession 启动一个连续模式的 ASR 会话。
//
// 流程：
//  1. 启动 ASR session（喂入 asrAudioCh）
//  2. 启动 copyAudioToASR goroutine 把 audioCh 音频复制到 asrAudioCh
//  3. 启动 timeout 监控 goroutine，30s 无活动就退出到 PTT
//
// ASR final 由 handleTranscript 统一处理，会调用 onFinalResult 提交到 Agent。
// 30s 计时器在每次 final transcript 时通过 resetContinuousActivity 重置。
func (vc *VoiceController) startContinuousSession() error {
	vc.mu.Lock()
	if vc.mode != ModeContinuous {
		vc.mu.Unlock()
		return nil
	}
	if vc.speechRecognizer == nil {
		vc.mu.Unlock()
		return errors.New(errors.CodeSpeechFailed, "speech recognizer not available")
	}
	if !vc.audioCap.IsRecording() {
		vc.mu.Unlock()
		return errors.New(errors.CodeAudioFailed, "audio capture not running")
	}

	// 重置活动计时
	vc.continuousLastActive = time.Now()

	// 启动 ASR session
	vc.mu.Unlock()
	err := vc.speechRecognizer.StartSession(vc.asrAudioCh, vc.handleTranscript)
	vc.mu.Lock()
	if err != nil {
		return errors.Wrap(errors.CodeSpeechFailed, "failed to start continuous ASR", err)
	}

	// 状态机：连续模式跳过 WakeDetected，直接进入 SpeechListening
	vc.transitionTo(StateSpeechListening)

	// 启动 audio → ASR 复制 goroutine
	go vc.copyAudioToASR()

	// 启动超时监控
	go vc.continuousTimeoutLoop()

	log.Info("voice controller: continuous ASR session started")
	return nil
}

// resetContinuousActivity 重置连续模式活动计时（每次 final transcript 时调用）
func (vc *VoiceController) resetContinuousActivity() {
	vc.mu.Lock()
	vc.continuousLastActive = time.Now()
	vc.mu.Unlock()
}

// continuousTimeoutLoop 监控连续模式超时
// 在最后一次活动后 timeout 秒内无新 final，就退出连续模式
func (vc *VoiceController) continuousTimeoutLoop() {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-vc.ctx.Done():
			return
		case <-ticker.C:
			vc.mu.Lock()
			if vc.mode != ModeContinuous {
				vc.mu.Unlock()
				return
			}
			elapsed := time.Since(vc.continuousLastActive)
			timeout := vc.continuousTimeout
			vc.mu.Unlock()

			if elapsed > timeout {
				log.Infof("voice controller: continuous mode timeout (%v), switching to PTT", elapsed)
				if err := vc.SetMode(context.Background(), ModePTT); err != nil {
					log.Errorf("voice controller: failed to switch to PTT on timeout: %v", err)
				}
				return
			}
		}
	}
}

// transcriptHandler 连续模式专用的 transcript 处理器（占位，未来可扩展）
func (vc *VoiceController) transcriptHandler(t speech.Transcript) {
	vc.handleTranscript(t)
}
