package input

import (
	"context"
	"sync"
	"time"

	"github.com/lipipi/aurora/internal/audio"
	"github.com/lipipi/aurora/internal/errors"
	"github.com/lipipi/aurora/internal/log"
	"github.com/lipipi/aurora/internal/speech"
	"github.com/lipipi/aurora/internal/tts"
	"github.com/lipipi/aurora/internal/wakeword"
)

// VoiceState defines the possible states of the voice controller.
type VoiceState int

const (
	// StateIdle idle state:
	// - ModePTT: waiting for PTT hotkey
	// - ModeWakeWord: wakeword detector running, listening for wakeword
	// - ModeContinuous: ASR session active, listening for any speech
	StateIdle VoiceState = iota

	// StateWakeDetected wakeword just detected, starting ASR session (WakeWord mode)
	StateWakeDetected

	// StateSpeechListening ASR is running, listening to user speech
	StateSpeechListening

	// StateProcessing ASR finished, Agent is processing the query and generating response
	StateProcessing

	// StateSpeaking TTS is playing the response
	StateSpeaking
)

// String returns the string representation of the state.
func (s VoiceState) String() string {
	switch s {
	case StateIdle:
		return "Idle"
	case StateWakeDetected:
		return "WakeDetected"
	case StateSpeechListening:
		return "SpeechListening"
	case StateProcessing:
		return "Processing"
	case StateSpeaking:
		return "Speaking"
	default:
		return "Unknown"
	}
}

// VoiceMode 语音输入模式
type VoiceMode int

const (
	// ModePTT 按键说话模式：只有按住 Ctrl+Shift+R 才能语音输入
	ModePTT VoiceMode = iota
	// ModeWakeWord 唤醒模式：检测到唤醒词后开始识别
	ModeWakeWord
	// ModeContinuous 连续对话模式：免唤醒，循环识别，30s 无输入自动退出
	ModeContinuous
)

// String 返回模式标识符（用于持久化 / 事件）
func (m VoiceMode) String() string {
	switch m {
	case ModePTT:
		return "ptt"
	case ModeWakeWord:
		return "wakeword"
	case ModeContinuous:
		return "continuous"
	default:
		return "unknown"
	}
}

// DisplayName 返回模式中文显示名
func (m VoiceMode) DisplayName() string {
	switch m {
	case ModePTT:
		return "按键说话"
	case ModeWakeWord:
		return "唤醒模式"
	case ModeContinuous:
		return "连续对话"
	default:
		return "未知"
	}
}

// ParseVoiceMode 从字符串解析模式
func ParseVoiceMode(s string) VoiceMode {
	switch s {
	case "ptt":
		return ModePTT
	case "wakeword":
		return ModeWakeWord
	case "continuous":
		return ModeContinuous
	default:
		return ModeWakeWord // 默认
	}
}

// VoiceController is the unified controller for voice processing.
// It manages three voice modes (PTT / WakeWord / Continuous) and TTS playback.
//
// 模式行为：
//   - ModePTT：仅在 PTT 热键按下时启动 ASR；VoiceController 本身不监听
//   - ModeWakeWord：后台持续运行 wakeword + ASR（唤醒后一次识别）
//   - ModeContinuous：跳过 wakeword，持续运行 ASR；识别 final → Agent → TTS → 循环；30s 无输入退出
//
// TTS 播放期间 ttsPlaying=true，echo cancellation 抑制 ASR 接收音频
// Agent 处理期间 agentBusy=true，wakeword/continuous 模式下新输入被忽略（PTT 仍可抢占）
type VoiceController struct {
	wakeDetector     wakeword.WakeWordDetector
	speechRecognizer speech.SpeechRecognizer
	audioCap         audio.AudioCapture
	ttsProvider      tts.TTSProvider // 可选注入

	// 状态保护 - 所有字段访问都需 mu
	mu           sync.Mutex
	mode         VoiceMode
	state        VoiceState
	enabled      bool
	lastWakeTime time.Time
	cooldown     time.Duration

	// Agent / TTS 状态
	agentBusy  bool
	ttsPlaying bool

	// 连续模式专用
	continuousCancel     context.CancelFunc
	continuousExitCmds   []string
	continuousTimeout    time.Duration
	continuousLastActive time.Time

	// Channels
	audioCh     chan []byte
	asrAudioCh  chan []byte
	wakeEventCh <-chan wakeword.WakeEvent

	// 上下文
	ctx    context.Context
	cancel context.CancelFunc

	// Callbacks
	onStateChange  func(VoiceState)
	onTranscript   func(speech.Transcript)
	onWakeDetected func()
	onFinalResult  func(string)
	onModeChange   func(VoiceMode) // 模式变更通知（前端 + 托盘同步）
}

// VoiceControllerOption is a functional option for VoiceController.
type VoiceControllerOption func(*VoiceController)

// WithOnStateChange sets the state change callback.
func WithOnStateChange(cb func(VoiceState)) VoiceControllerOption {
	return func(vc *VoiceController) { vc.onStateChange = cb }
}

// WithOnTranscript sets the transcript callback.
func WithOnTranscript(cb func(speech.Transcript)) VoiceControllerOption {
	return func(vc *VoiceController) { vc.onTranscript = cb }
}

// WithOnWakeDetected sets the wake detected callback.
func WithOnWakeDetected(cb func()) VoiceControllerOption {
	return func(vc *VoiceController) { vc.onWakeDetected = cb }
}

// WithOnFinalResult sets the final result callback.
func WithOnFinalResult(cb func(string)) VoiceControllerOption {
	return func(vc *VoiceController) { vc.onFinalResult = cb }
}

// WithOnModeChange sets the mode change callback.
func WithOnModeChange(cb func(VoiceMode)) VoiceControllerOption {
	return func(vc *VoiceController) { vc.onModeChange = cb }
}

// WithEnabled sets whether wakeword is enabled (legacy compatibility).
func WithEnabled(enabled bool) VoiceControllerOption {
	return func(vc *VoiceController) { vc.enabled = enabled }
}

// WithCooldown sets the cooldown after detection.
func WithCooldown(cooldown time.Duration) VoiceControllerOption {
	return func(vc *VoiceController) { vc.cooldown = cooldown }
}

// WithContinuousSettings sets continuous mode parameters.
func WithContinuousSettings(timeout time.Duration, exitCmds []string) VoiceControllerOption {
	return func(vc *VoiceController) {
		vc.continuousTimeout = timeout
		if len(exitCmds) > 0 {
			vc.continuousExitCmds = exitCmds
		} else {
			vc.continuousExitCmds = []string{"结束对话", "停止", "退出对话", "exit", "stop"}
		}
	}
}

// NewVoiceController creates a new VoiceController.
func NewVoiceController(
	wakeDetector wakeword.WakeWordDetector,
	speechRecognizer speech.SpeechRecognizer,
	audioCap audio.AudioCapture,
	options ...VoiceControllerOption,
) *VoiceController {
	vc := &VoiceController{
		wakeDetector:        wakeDetector,
		speechRecognizer:    speechRecognizer,
		audioCap:            audioCap,
		state:               StateIdle,
		mode:                ModePTT, // 启动前默认 PTT
		enabled:             false,
		cooldown:            1000 * time.Millisecond,
		continuousTimeout:   30 * time.Second,
		continuousExitCmds:  []string{"结束对话", "停止", "退出对话", "exit", "stop"},
		audioCh:             make(chan []byte, 16),
		asrAudioCh:          make(chan []byte, 16),
	}

	for _, opt := range options {
		opt(vc)
	}

	return vc
}

// SetTTS injects a TTS provider. Optional; if not set, SpeakTTS becomes a no-op.
func (vc *VoiceController) SetTTS(provider tts.TTSProvider) {
	vc.mu.Lock()
	defer vc.mu.Unlock()
	vc.ttsProvider = provider
}

// Start starts the voice controller (initializes context).
// Mode is set separately via SetMode.
func (vc *VoiceController) Start(ctx context.Context) errors.AppError {
	vc.mu.Lock()
	defer vc.mu.Unlock()

	if vc.ctx != nil {
		log.Warn("voice controller: already started")
		return nil
	}

	vc.ctx, vc.cancel = context.WithCancel(ctx)
	vc.state = StateIdle

	log.Info("voice controller started, mode will be set via SetMode")
	vc.notifyStateChange()
	return nil
}

// Stop stops the voice controller and releases all resources.
func (vc *VoiceController) Stop() {
	vc.mu.Lock()
	defer vc.mu.Unlock()

	// 停止连续模式
	if vc.continuousCancel != nil {
		vc.continuousCancel()
		vc.continuousCancel = nil
	}

	if vc.cancel != nil {
		vc.cancel()
	}

	if vc.audioCap != nil && vc.audioCap.IsRecording() {
		vc.audioCap.Stop()
	}

	if vc.wakeDetector != nil {
		vc.wakeDetector.Stop()
	}

	if vc.state == StateSpeechListening || vc.state == StateProcessing {
		if vc.speechRecognizer != nil {
			_ = vc.speechRecognizer.CancelSession()
		}
	}

	vc.state = StateIdle
	vc.ttsPlaying = false
	vc.agentBusy = false
	vc.notifyStateChange()
	log.Info("voice controller stopped")
}

// GetState returns the current state.
func (vc *VoiceController) GetState() VoiceState {
	vc.mu.Lock()
	defer vc.mu.Unlock()
	return vc.state
}

// GetMode returns the current voice mode.
func (vc *VoiceController) GetMode() VoiceMode {
	vc.mu.Lock()
	defer vc.mu.Unlock()
	return vc.mode
}

// SetMode switches to a new voice mode (thread-safe).
// Properly tears down the old mode's resources and starts the new mode.
func (vc *VoiceController) SetMode(ctx context.Context, mode VoiceMode) error {
	vc.mu.Lock()
	defer vc.mu.Unlock()

	if vc.mode == mode {
		log.Debugf("voice controller: already in mode %s", mode)
		return nil
	}

	log.Infof("voice controller: switching mode %s → %s", vc.mode, mode)
	oldMode := vc.mode

	// 1. 停止当前模式的所有资源
	vc.stopCurrentModeResources()

	// 2. 切换模式
	vc.mode = mode

	// 3. 启动新模式
	switch mode {
	case ModePTT:
		// 不启动任何后台监听；只 PTT 可用
		vc.state = StateIdle
		vc.notifyStateChange()
	case ModeWakeWord:
		if err := vc.startWakewordInternal(); err != nil {
			log.Errorf("voice controller: failed to start wakeword mode: %v", err)
			vc.mode = oldMode // 回滚
			return err
		}
		vc.state = StateIdle
		vc.notifyStateChange()
	case ModeContinuous:
		if err := vc.startContinuousInternal(); err != nil {
			log.Errorf("voice controller: failed to start continuous mode: %v", err)
			vc.mode = oldMode
			return err
		}
		// startContinuousInternal 内部会 transitionTo(StateSpeechListening)
	}

	// 4. 通知 mode 变更
	if vc.onModeChange != nil {
		// 释放锁调用回调（避免死锁）
		cb := vc.onModeChange
		vc.mu.Unlock()
		cb(mode)
		vc.mu.Lock()
	}

	return nil
}

// CycleMode cycles through modes: PTT → WakeWord → Continuous → PTT
// Returns the new mode.
func (vc *VoiceController) CycleMode(ctx context.Context) VoiceMode {
	vc.mu.Lock()
	var next VoiceMode
	switch vc.mode {
	case ModePTT:
		next = ModeWakeWord
	case ModeWakeWord:
		next = ModeContinuous
	case ModeContinuous:
		next = ModePTT
	default:
		next = ModeWakeWord
	}
	vc.mu.Unlock()

	if err := vc.SetMode(ctx, next); err != nil {
		log.Errorf("voice controller: cycle mode failed: %v", err)
	}
	return next
}

// IsAgentBusy returns whether the agent is currently processing.
func (vc *VoiceController) IsAgentBusy() bool {
	vc.mu.Lock()
	defer vc.mu.Unlock()
	return vc.agentBusy
}

// IsTTSPlaying returns whether TTS is currently playing.
func (vc *VoiceController) IsTTSPlaying() bool {
	vc.mu.Lock()
	defer vc.mu.Unlock()
	return vc.ttsPlaying
}

// NotifyAgentStart is called when agent starts processing a final result.
// Sets agentBusy=true; new voice input will be ignored (except PTT).
func (vc *VoiceController) NotifyAgentStart() {
	vc.mu.Lock()
	wasBusy := vc.agentBusy
	vc.agentBusy = true
	vc.mu.Unlock()
	if !wasBusy {
		log.Debug("voice controller: agent busy = true")
	}
}

// NotifyAgentComplete is called when agent finishes generating response.
// Clears agentBusy and, if state is still Processing, transitions to Idle.
// This is the canonical way to leave the Processing state — it does NOT
// depend on TTS being configured or successful.
func (vc *VoiceController) NotifyAgentComplete() {
	vc.mu.Lock()
	wasBusy := vc.agentBusy
	vc.agentBusy = false
	// 兜底：若仍在 Processing 状态（TTS 未配 / 失败 / 路径异常），主动切到 Idle
	// 避免 UI 永远卡在 "Agent 处理中"
	if vc.state == StateProcessing {
		vc.state = StateIdle
		vc.notifyStateChange()
		log.Debug("voice controller: forced Processing → Idle on agent complete")
	}
	vc.mu.Unlock()
	if wasBusy {
		log.Debug("voice controller: agent busy = false")
	}
}

// NotifyTTSStart is called when TTS begins playing the response.
// Sets ttsPlaying=true for echo cancellation; transitions to StateSpeaking.
func (vc *VoiceController) NotifyTTSStart() {
	vc.mu.Lock()
	vc.ttsPlaying = true
	vc.mu.Unlock()
	log.Debug("voice controller: tts playing = true")
	vc.transitionTo(StateSpeaking)
}

// NotifyTTSEnd is called when TTS playback finishes.
// Clears ttsPlaying; in continuous mode, restarts ASR session.
func (vc *VoiceController) NotifyTTSEnd() {
	vc.mu.Lock()
	vc.ttsPlaying = false
	mode := vc.mode
	vc.mu.Unlock()
	log.Debug("voice controller: tts playing = false")

	// 唤醒模式：TTS 结束 → 回到 Idle（继续监听唤醒词）
	// 连续模式：TTS 结束 → 重启 ASR session 继续监听
	switch mode {
	case ModeContinuous:
		if err := vc.startContinuousSession(); err != nil {
			log.Errorf("voice controller: failed to restart continuous session: %v", err)
			// 失败则退出连续模式
			_ = vc.SetMode(context.Background(), ModePTT)
		}
	case ModeWakeWord:
		// 回到 Idle（继续监听唤醒词）
		vc.mu.Lock()
		vc.state = StateIdle
		vc.notifyStateChange()
		vc.mu.Unlock()
	}
}

// SpeakTTS triggers TTS playback for the given text.
// If no TTS provider is set or not configured, immediately calls onEnd equivalent (NotifyTTSEnd).
func (vc *VoiceController) SpeakTTS(text string) {
	vc.mu.Lock()
	provider := vc.ttsProvider
	vc.mu.Unlock()

	if provider == nil || !provider.IsConfigured() || text == "" {
		log.Debug("voice controller: TTS not available, skipping")
		vc.NotifyTTSEnd()
		return
	}

	vc.NotifyTTSStart()
	provider.SetOnPlaybackEnd(func() {
		vc.NotifyTTSEnd()
	})
	if err := provider.Synthesize(text); err != nil {
		log.Errorf("voice controller: TTS synthesize failed: %v", err)
		vc.NotifyTTSEnd()
	}
}

// PTTPreempt is called when PTT is activated.
// Cancels any ongoing session and yields to PTT.
func (vc *VoiceController) PTTPreempt() {
	vc.mu.Lock()
	defer vc.mu.Unlock()

	// 停止 TTS 播放（让出音频）
	if vc.ttsPlaying {
		if vc.ttsProvider != nil {
			vc.ttsProvider.Stop()
		}
		vc.ttsPlaying = false
	}

	// 如果有连续模式 session，停掉
	if vc.continuousCancel != nil {
		vc.continuousCancel()
		vc.continuousCancel = nil
	}

	// 状态机：到 Idle
	if vc.state != StateIdle {
		vc.transitionTo(StateIdle)
	}
}

// PTTFinished is called when PTT recording completes.
// In WakeWord mode, returns to idle listening.
// In Continuous mode, resumes the continuous session.
func (vc *VoiceController) PTTFinished() {
	vc.mu.Lock()
	mode := vc.mode
	vc.mu.Unlock()

	switch mode {
	case ModeContinuous:
		// PTT 完成，ASR final 已经通过 InputManager → Agent 处理
		// 重新启动 continuous session 继续监听
		if err := vc.startContinuousSession(); err != nil {
			log.Errorf("voice controller: failed to restart continuous after PTT: %v", err)
		}
	case ModeWakeWord:
		// 继续监听唤醒词
		vc.mu.Lock()
		vc.state = StateIdle
		vc.notifyStateChange()
		vc.mu.Unlock()
	default:
		// PTT 模式：保持 Idle
		vc.mu.Lock()
		vc.state = StateIdle
		vc.notifyStateChange()
		vc.mu.Unlock()
	}
}

// stopCurrentModeResources stops resources for the current mode (must be called with mu held).
func (vc *VoiceController) stopCurrentModeResources() {
	// 停止连续模式
	if vc.continuousCancel != nil {
		vc.continuousCancel()
		vc.continuousCancel = nil
	}

	// 停止 TTS
	if vc.ttsPlaying && vc.ttsProvider != nil {
		vc.ttsProvider.Stop()
		vc.ttsPlaying = false
	}

	// 停止 ASR（如果在听）
	if vc.state == StateSpeechListening || vc.state == StateProcessing {
		if vc.speechRecognizer != nil {
			_ = vc.speechRecognizer.CancelSession()
		}
	}

	// 停止 wakeword
	if vc.wakeDetector != nil {
		vc.wakeDetector.Stop()
	}

	// 停止 audio capture
	if vc.audioCap != nil && vc.audioCap.IsRecording() {
		vc.audioCap.Stop()
	}
}

// startWakewordInternal starts wakeword mode (must be called with mu held).
func (vc *VoiceController) startWakewordInternal() error {
	if vc.wakeDetector == nil || !vc.wakeDetector.IsConfigured() {
		return errors.New(errors.CodeWakeWordFailed, "wakeword detector not configured")
	}
	if vc.audioCap == nil {
		return errors.New(errors.CodeAudioFailed, "audio capture not available")
	}

	// Start audio capture
	audioCallback := func(data []byte) {
		select {
		case vc.audioCh <- data:
		default:
			log.Debug("voice controller: audio channel full, dropping frame")
		}
	}
	if err := vc.audioCap.Start(audioCallback); err != nil {
		return errors.Wrap(errors.CodeAudioFailed, "failed to start audio capture", err)
	}

	// Start wakeword detector
	if err := vc.wakeDetector.Start(vc.ctx, vc.audioCh); err != nil {
		vc.audioCap.Stop()
		return errors.Wrap(errors.CodeWakeWordFailed, "failed to start wakeword detector", err)
	}

	vc.wakeEventCh = vc.wakeDetector.Events()

	// Start event loop
	go vc.eventLoop()

	log.Info("voice controller: wakeword mode started")
	return nil
}

// startContinuousInternal starts continuous mode (must be called with mu held).
// Only sets up the state; startContinuousSession does the actual ASR start.
func (vc *VoiceController) startContinuousInternal() error {
	if vc.audioCap == nil {
		return errors.New(errors.CodeAudioFailed, "audio capture not available")
	}
	if vc.speechRecognizer == nil {
		return errors.New(errors.CodeSpeechFailed, "speech recognizer not available")
	}

	// 重置连续模式状态
	vc.continuousLastActive = time.Now()

	// 启动 audio capture（如果还没启动）
	if !vc.audioCap.IsRecording() {
		audioCallback := func(data []byte) {
			select {
			case vc.audioCh <- data:
			default:
				log.Debug("voice controller: audio channel full, dropping frame")
			}
		}
		if err := vc.audioCap.Start(audioCallback); err != nil {
			return errors.Wrap(errors.CodeAudioFailed, "failed to start audio capture for continuous", err)
		}
	}

	// 启动 session（在持锁外启动，避免死锁）
	// 先解锁启动，启动完成后再加锁
	vc.mu.Unlock()
	err := vc.startContinuousSession()
	vc.mu.Lock()
	if err != nil {
		vc.audioCap.Stop()
		return err
	}

	log.Info("voice controller: continuous mode started")
	return nil
}

// eventLoop is the main event loop for wakeword mode.
func (vc *VoiceController) eventLoop() {
	defer func() {
		if r := recover(); r != nil {
			log.Errorf("voice controller eventLoop panicked: %v", r)
		}
	}()

	for {
		select {
		case <-vc.ctx.Done():
			return
		case event, ok := <-vc.wakeEventCh:
			if !ok {
				return
			}
			vc.handleWakeEvent(event)
		}
	}
}

// handleWakeEvent processes a wake event from the detector.
func (vc *VoiceController) handleWakeEvent(event wakeword.WakeEvent) {
	vc.mu.Lock()
	defer vc.mu.Unlock()

	switch event.Type {
	case wakeword.WakeEventDetected:
		vc.handleWakeDetected(event)
	case wakeword.WakeEventError:
		log.Errorf("wakeword detection error: %s", event.Message)
		vc.transitionTo(StateIdle)
	}
}

// handleWakeDetected handles a successful wake word detection.
func (vc *VoiceController) handleWakeDetected(event wakeword.WakeEvent) {
	// 仅在 WakeWord 模式处理唤醒词
	if vc.mode != ModeWakeWord {
		log.Debugf("voice controller: wakeword detected in non-wakeword mode (%s), ignoring", vc.mode)
		return
	}

	// Check current state - can only accept detection from idle
	if vc.state != StateIdle {
		log.Debugf("wakeword detected but state is %s, ignoring", vc.state)
		return
	}

	// Agent busy 时忽略
	if vc.agentBusy {
		log.Debug("wakeword detected but agent busy, ignoring")
		return
	}

	// TTS playing 时忽略
	if vc.ttsPlaying {
		log.Debug("wakeword detected but TTS playing, ignoring")
		return
	}

	// Check cooldown
	now := time.Now()
	if now.Sub(vc.lastWakeTime) < vc.cooldown {
		log.Debugf("wakeword detected but in cooldown (%v since last), ignoring", now.Sub(vc.lastWakeTime))
		return
	}

	vc.lastWakeTime = now
	log.Infof("voice controller: wake word detected: %s (score: %.3f)", event.ModelName, event.Score)

	// Notify wake detected
	if vc.onWakeDetected != nil {
		vc.onWakeDetected()
	}

	// Transition to WakeDetected state and start ASR
	vc.transitionTo(StateWakeDetected)

	// Start ASR session (need to release lock because start can block)
	vc.mu.Unlock()
	err := vc.speechRecognizer.StartSession(vc.asrAudioCh, vc.handleTranscript)
	vc.mu.Lock()

	if err != nil {
		log.Errorf("failed to start ASR session after wake detection: %v", err)
		vc.transitionTo(StateIdle)
		return
	}

	// Transition to SpeechListening
	vc.transitionTo(StateSpeechListening)

	// Start copying audio from audioCh to asrAudioCh
	go vc.copyAudioToASR()
}

// copyAudioToASR copies audio chunks from the main audio channel to the ASR channel.
// Stops when we leave StateSpeechListening OR when ttsPlaying is true (echo cancellation).
func (vc *VoiceController) copyAudioToASR() {
	for {
		vc.mu.Lock()
		if vc.state != StateSpeechListening {
			vc.mu.Unlock()
			return
		}
		if vc.ttsPlaying {
			// TTS 在播放，丢弃本帧防止回声
			vc.mu.Unlock()
			// 等一帧时间再重试
			time.Sleep(50 * time.Millisecond)
			continue
		}
		vc.mu.Unlock()

		select {
		case data, ok := <-vc.audioCh:
			if !ok {
				return
			}
			// Re-check after receiving
			vc.mu.Lock()
			if vc.ttsPlaying {
				vc.mu.Unlock()
				continue
			}
			vc.mu.Unlock()

			// Non-blocking send
			select {
			case vc.asrAudioCh <- data:
			default:
				log.Debug("voice controller: ASR audio channel full, dropping frame")
			}
		case <-vc.ctx.Done():
			return
		}
	}
}

// handleTranscript handles a transcript from ASR.
func (vc *VoiceController) handleTranscript(t speech.Transcript) {
	vc.mu.Lock()
	mode := vc.mode
	vc.mu.Unlock()

	// Forward transcript to callback
	if vc.onTranscript != nil {
		vc.onTranscript(t)
	}

	// Only handle final transcript here
	if !t.IsFinal {
		return
	}

	text := t.Text
	log.Infof("voice controller: final transcript (mode=%s): %q", mode, text)

	// 连续模式：重置活动计时
	if mode == ModeContinuous {
		vc.resetContinuousActivity()
	}

	// 连续模式：检查退出关键词
	if mode == ModeContinuous && text != "" {
		if vc.matchExitCommand(text) {
			log.Infof("voice controller: continuous mode exit command detected: %q", text)
			// 退出到 PTT 模式
			_ = vc.SetMode(context.Background(), ModePTT)
			return
		}
	}

	// 唤醒词模式 / 连续模式：提交到 Agent
	if text != "" && vc.onFinalResult != nil {
		vc.onFinalResult(text)
	}

	vc.mu.Lock()
	vc.transitionTo(StateProcessing)
	vc.mu.Unlock()
}

// matchExitCommand 检查文本是否包含连续模式退出关键词
func (vc *VoiceController) matchExitCommand(text string) bool {
	vc.mu.Lock()
	cmds := vc.continuousExitCmds
	vc.mu.Unlock()
	for _, cmd := range cmds {
		if contains(text, cmd) {
			return true
		}
	}
	return false
}

// OnAgentComplete is the legacy API (still used by main.go for wakeword mode).
// Kept for backward compatibility - delegates to NotifyAgentComplete.
func (vc *VoiceController) OnAgentComplete() {
	vc.NotifyAgentComplete()
}

// OnTTSComplete is the legacy API.
func (vc *VoiceController) OnTTSComplete() {
	vc.NotifyTTSEnd()
}

// transitionTo transitions to a new state, handling cleanup of the old state.
// Must be called with mu locked.
func (vc *VoiceController) transitionTo(newState VoiceState) {
	oldState := vc.state

	// Cleanup based on old state
	switch oldState {
	case StateSpeechListening, StateWakeDetected, StateSpeaking:
		// End ASR session if active
		if vc.speechRecognizer != nil {
			if err := vc.speechRecognizer.EndSession(); err != nil {
				log.Debugf("voice controller: end ASR session: %v", err)
			}
		}
		// Drain the asrAudioCh to clear old data
		vc.drainASRAudioCh()
	}

	vc.state = newState
	log.Debugf("voice controller: state changed %s → %s", oldState, newState)
	vc.notifyStateChange()
}

// drainASRAudioCh drains all pending audio from the ASR channel.
func (vc *VoiceController) drainASRAudioCh() {
	for {
		select {
		case <-vc.asrAudioCh:
		default:
			return
		}
	}
}

// notifyStateChange notifies listeners of state change.
func (vc *VoiceController) notifyStateChange() {
	if vc.onStateChange != nil {
		cb := vc.onStateChange
		state := vc.state
		// 持锁调用回调需要小心，但 onStateChange 是简单 emit
		go cb(state) // 异步避免持锁太久
	}
}

// contains 是 strings.Contains 的副本（避免 import 循环问题）
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || indexOf(s, substr) >= 0)
}

func indexOf(s, substr string) int {
	n, m := len(s), len(substr)
	if m == 0 {
		return 0
	}
	for i := 0; i+m <= n; i++ {
		if s[i:i+m] == substr {
			return i
		}
	}
	return -1
}
