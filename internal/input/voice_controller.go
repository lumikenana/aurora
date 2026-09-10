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
	// ModeWakeWord 唤醒模式：检测到唤醒词后进入 Conversation（多轮对话）
	ModeWakeWord
)

// String 返回模式标识符（用于持久化 / 事件）
func (m VoiceMode) String() string {
	switch m {
	case ModePTT:
		return "ptt"
	case ModeWakeWord:
		return "wakeword"
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
	default:
		return "未知"
	}
}

// ParseVoiceMode 从字符串解析模式
func ParseVoiceMode(s string) VoiceMode {
	switch s {
	case "ptt":
		return ModePTT
	case "continuous":
		// legacy 配置兼容（Phase 3 起 ModeContinuous 已移除）：
		// 旧 continuous 用户落到唤醒模式 —— 多轮能力由 Conversation 生命周期承载。
		// 完整配置迁移属 Phase 5。
		return ModeWakeWord
	default:
		return ModeWakeWord // 默认（含 "wakeword"）
	}
}

// VoiceController is the unified controller for voice processing.
// It manages two voice modes (PTT / WakeWord) and TTS playback.
//
// 模式行为（Voice Mode Refactoring Phase 3 起）：
//   - ModePTT：仅在 PTT 热键按下时启动 ASR；PTT 触发独立的 standalone Turn
//     （Turn 完成 → Idle，不进入 Conversation / Waiting）
//   - ModeWakeWord：后台持续运行 wakeword detector；唤醒 → Start Conversation
//     → 多轮 Turn（每轮结束进入 Waiting，30s 无输入 Conversation 结束 → Idle）
//
// 旧 ModeContinuous（免唤醒循环识别）已于 Phase 3 移除 ——
// 多轮对话能力由 Conversation 生命周期承载，不再是独立 Mode。
//
// Conversation / Turn 生命周期由 convMgr（ConversationManager）管理；
// WakeWord detector 只负责在 Idle 时触发 Conversation Start，不参与生命周期。
// TTS 播放期间 ttsPlaying=true，echo cancellation 抑制 ASR 接收音频
// Agent 处理期间 agentBusy=true，wakeword 模式下新输入被忽略（PTT 仍可抢占）
type VoiceController struct {
	wakeDetector     wakeword.WakeWordDetector
	speechRecognizer speech.SpeechRecognizer
	audioCap         audio.AudioCapture
	ttsProvider      tts.TTSProvider // 可选注入

	// Conversation / Turn 生命周期（Phase 2）
	convMgr    *ConversationManager
	convConfig ConversationConfig

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

// WithConversationConfig sets the conversation lifecycle configuration
// (e.g. Waiting timeout). Mainly used by tests to shorten timeouts.
func WithConversationConfig(cfg ConversationConfig) VoiceControllerOption {
	return func(vc *VoiceController) { vc.convConfig = cfg }
}

// WithEnabled sets whether wakeword is enabled (legacy compatibility).
func WithEnabled(enabled bool) VoiceControllerOption {
	return func(vc *VoiceController) { vc.enabled = enabled }
}

// WithCooldown sets the cooldown after detection.
func WithCooldown(cooldown time.Duration) VoiceControllerOption {
	return func(vc *VoiceController) { vc.cooldown = cooldown }
}

// NewVoiceController creates a new VoiceController.
func NewVoiceController(
	wakeDetector wakeword.WakeWordDetector,
	speechRecognizer speech.SpeechRecognizer,
	audioCap audio.AudioCapture,
	options ...VoiceControllerOption,
) *VoiceController {
	vc := &VoiceController{
		wakeDetector:     wakeDetector,
		speechRecognizer: speechRecognizer,
		audioCap:         audioCap,
		state:            StateIdle,
		mode:             ModePTT, // 启动前默认 PTT
		enabled:          false,
		cooldown:         1000 * time.Millisecond,
		convConfig:       DefaultConversationConfig(),
		audioCh:          make(chan []byte, 16),
		asrAudioCh:       make(chan []byte, 16),
	}

	for _, opt := range options {
		opt(vc)
	}

	// Conversation / Turn 生命周期管理（Phase 2）。
	// onConvEnd → 停止 Conversation ASR session 并回到 Idle（恢复唤醒监听）。
	vc.convMgr = NewConversationManager(vc.convConfig)
	vc.convMgr.SetCallbacks(
		nil, nil, nil,
		func() { log.Info("voice controller: conversation started") },
		func() { vc.handleConversationEnded() },
	)

	return vc
}

// GetConversationManager returns the conversation manager (lifecycle introspection).
func (vc *VoiceController) GetConversationManager() *ConversationManager {
	return vc.convMgr
}

// handleConversationEnded is the ConversationManager onConvEnd callback.
// Stops the conversation ASR session (if still running) and returns to Idle
// so the wake detector (kept running in the background) can trigger again.
func (vc *VoiceController) handleConversationEnded() {
	vc.mu.Lock()
	defer vc.mu.Unlock()

	if vc.mode != ModeWakeWord {
		return
	}
	if vc.state != StateIdle {
		vc.transitionTo(StateIdle)
	}
	log.Debug("voice controller: conversation ended, back to wake listening")
}

// SetTTS injects a TTS provider. Optional; if not set, SpeakTTS becomes a no-op.
func (vc *VoiceController) SetTTS(provider tts.TTSProvider) {
	vc.mu.Lock()
	defer vc.mu.Unlock()
	vc.ttsProvider = provider
}

// SetWakeDetector injects a wake word detector after construction.
// Used when WakeWord files are downloaded asynchronously after VoiceController
// is already running (for TTS and other voice modes).
func (vc *VoiceController) SetWakeDetector(detector wakeword.WakeWordDetector) {
	vc.mu.Lock()
	defer vc.mu.Unlock()
	vc.wakeDetector = detector
	log.Info("voice controller: wake word detector injected")
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

	// 停止 Conversation 生命周期（状态清理，不触发回调）
	vc.convMgr.Stop()

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

// CycleMode cycles through modes: PTT → WakeWord → PTT
// Returns the new mode.
func (vc *VoiceController) CycleMode(ctx context.Context) VoiceMode {
	vc.mu.Lock()
	var next VoiceMode
	switch vc.mode {
	case ModePTT:
		next = ModeWakeWord
	default:
		next = ModePTT
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

	// Conversation lifecycle: agent finished generating → response phase.
	// Defensive: if the final transcript raced ahead of PTTFinished, close the
	// input phase first (InputComplete no-ops with a warning otherwise).
	cm := vc.convMgr
	if cm.IsStandaloneTurnActive() || cm.IsConversationActive() {
		if cm.GetTurnState() == TurnInput {
			cm.InputComplete()
		}
		cm.ProcessingComplete()
	}

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
// Clears ttsPlaying and advances the Conversation/Turn lifecycle:
//   - standalone (PTT) turn → Turn Complete → Idle
//   - WakeWord Conversation → Waiting + restart ASR session for the next turn
//   - 兜底（文本输入 TTS / stale 回调）→ 状态归 Idle
//
// May also be invoked as a stale callback after PTTPreempt stopped playback
// (ttsProvider.Stop() triggers OnPlaybackEnd asynchronously); the lifecycle
// calls below are state-guarded and no-op in that case.
func (vc *VoiceController) NotifyTTSEnd() {
	vc.mu.Lock()
	vc.ttsPlaying = false
	mode := vc.mode
	vc.mu.Unlock()
	log.Debug("voice controller: tts playing = false")

	cm := vc.convMgr

	// Standalone (PTT) turn: response finished → Turn Complete → Idle.
	if cm.IsStandaloneTurnActive() {
		if cm.GetTurnState() == TurnResponding {
			cm.ResponseComplete()
		}
		vc.mu.Lock()
		if vc.state != StateIdle {
			vc.state = StateIdle
			vc.notifyStateChange()
		}
		vc.mu.Unlock()
		return
	}

	// WakeWord Conversation: response finished → Waiting → listen for next turn.
	if mode == ModeWakeWord && cm.IsConversationActive() {
		if cm.GetTurnState() == TurnResponding {
			cm.ResponseComplete()
		}
		if err := vc.startConversationSession(); err != nil {
			log.Errorf("voice controller: failed to restart conversation ASR: %v", err)
			cm.EndConversation()
		}
		return
	}

	// 兜底：无 standalone Turn、无活跃 Conversation（文本输入 TTS / stale 回调）
	// → TTS 结束回 Idle
	vc.mu.Lock()
	if vc.state == StateSpeaking {
		vc.state = StateIdle
		vc.notifyStateChange()
	}
	vc.mu.Unlock()
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

// PTTPreempt is called when PTT is activated (also by StopSpeaking shortcut
// and overlay interrupt). Cancels any ongoing session and yields to PTT:
//   - interrupts any active Conversation or standalone Turn (Conversation 被打断后
//     不恢复，直接结束 → Idle)
//   - stops TTS playback (yields audio)
//   - ends the conversation ASR session (transitionTo Idle)
func (vc *VoiceController) PTTPreempt() {
	vc.mu.Lock()
	defer vc.mu.Unlock()

	// 打断任何进行中的 Conversation / standalone Turn。
	// onConvEnd（异步）→ handleConversationEnded 是幂等的，此处状态已归 Idle。
	vc.convMgr.Interrupt()

	// 停止 TTS 播放（让出音频）。
	// 注意：ttsProvider.Stop() 会异步触发 OnPlaybackEnd → NotifyTTSEnd（stale），
	// NotifyTTSEnd 内部按 turn/conversation 状态判断，此处已 Interrupt，stale 回调为 no-op。
	if vc.ttsPlaying {
		if vc.ttsProvider != nil {
			vc.ttsProvider.Stop()
		}
		vc.ttsPlaying = false
	}

	// 状态机：到 Idle（从 SpeechListening/WakeDetected/Speaking 离开时结束 ASR session）
	if vc.state != StateIdle {
		vc.transitionTo(StateIdle)
	}
}

// StartPTTTurn begins a standalone PTT turn in the conversation lifecycle.
// Must be called after PTTPreempt (which interrupts any active conversation).
// PTT 语义：一次性单轮 Turn —— 完成后回 Idle，不进入 Conversation / Waiting。
func (vc *VoiceController) StartPTTTurn() {
	vc.convMgr.StartStandaloneTurn()
}

// PTTFinished is called when PTT recording completes.
// PTT 松开 = 本轮用户输入结束（等待 ASR final → Agent → TTS → Turn Complete）。
// 完成后回到 Idle（WakeWord 模式继续监听唤醒词）。
func (vc *VoiceController) PTTFinished() {
	// Standalone turn：输入阶段结束（ASR final 随后通过 InputManager → Agent）
	if vc.convMgr.IsStandaloneTurnActive() && vc.convMgr.GetTurnState() == TurnInput {
		vc.convMgr.InputComplete()
	}

	vc.mu.Lock()
	vc.state = StateIdle
	vc.notifyStateChange()
	vc.mu.Unlock()
}

// stopCurrentModeResources stops resources for the current mode (must be called with mu held).
func (vc *VoiceController) stopCurrentModeResources() {
	// 打断任何进行中的 Conversation / standalone Turn（模式切换 = 生命周期终止）
	vc.convMgr.Interrupt()

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

// startConversationSession (re)starts the ASR session that listens for the
// user's next turn inside an active Conversation (WakeWord mode).
// The session runs while the Conversation is in Listening/Waiting and ends
// when the Conversation ends (timeout / PTT interrupt / mode switch).
func (vc *VoiceController) startConversationSession() error {
	vc.mu.Lock()
	if vc.mode != ModeWakeWord || !vc.convMgr.IsConversationActive() {
		vc.mu.Unlock()
		return nil
	}
	if vc.speechRecognizer == nil {
		vc.mu.Unlock()
		return errors.New(errors.CodeSpeechFailed, "speech recognizer not available")
	}
	if vc.audioCap == nil || !vc.audioCap.IsRecording() {
		vc.mu.Unlock()
		return errors.New(errors.CodeAudioFailed, "audio capture not running")
	}
	vc.mu.Unlock()

	// StartSession 可能阻塞，持锁外调用
	err := vc.speechRecognizer.StartSession(vc.asrAudioCh, vc.handleTranscript)

	vc.mu.Lock()
	defer vc.mu.Unlock()
	if err != nil {
		return errors.Wrap(errors.CodeSpeechFailed, "failed to start conversation ASR", err)
	}

	vc.transitionTo(StateSpeechListening)
	go vc.copyAudioToASR()
	log.Debug("voice controller: conversation ASR session (re)started")
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

	// Conversation lifecycle gates：wake detector 在后台持续运行，
	// Conversation 进行中或 PTT standalone Turn 进行中时事件必须被忽略
	// （等效于 detector 的 Pause/Resume —— 复用现有 event gating 能力，
	// 不重写 wake detector）。
	if vc.convMgr.IsConversationActive() {
		log.Debug("wakeword detected but conversation already active, ignoring")
		return
	}
	if vc.convMgr.IsTurnInProgress() {
		log.Debug("wakeword detected but a turn is in progress, ignoring")
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

	// WakeWord trigger → Start Conversation + 第一个 Turn。
	// Wake detector 继续在后台运行，但其事件会被上面的 lifecycle gates 忽略，
	// 直到 Conversation 结束（onConvEnd → handleConversationEnded → Idle）。
	vc.convMgr.StartConversation()
	vc.convMgr.StartTurn()

	// Transition to WakeDetected state and start ASR
	vc.transitionTo(StateWakeDetected)

	// Start ASR session (need to release lock because start can block)
	vc.mu.Unlock()
	err := vc.speechRecognizer.StartSession(vc.asrAudioCh, vc.handleTranscript)
	vc.mu.Lock()

	if err != nil {
		log.Errorf("failed to start ASR session after wake detection: %v", err)
		vc.convMgr.EndConversation()
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

	cm := vc.convMgr

	// Interim result: inside a Conversation this means the user started
	// speaking the next turn — begin the turn now so the Waiting timer stops
	// （超时语义：等待用户"开始说话"，不是等待"说完"）。
	if !t.IsFinal {
		if mode == ModeWakeWord && t.Text != "" && cm.IsConversationActive() {
			cs := cm.GetState()
			ts := cm.GetTurnState()
			if (cs == ConvWaiting || cs == ConvListening) && (ts == TurnInactive || ts == TurnComplete) {
				cm.StartTurn()
			}
		}
		return
	}

	text := t.Text
	log.Infof("voice controller: final transcript (mode=%s): %q", mode, text)

	// 唤醒词模式：提交到 Agent
	if text != "" && vc.onFinalResult != nil {
		vc.onFinalResult(text)
	}

	// WakeWord Conversation：关闭当前 Turn 的输入阶段。
	if mode == ModeWakeWord && cm.IsConversationActive() {
		if text == "" {
			// ASR 判定语句结束但没有识别出文本 —— 这一轮不算数，
			// 丢弃 Turn，继续监听（Conversation 保持活跃）。
			cm.CancelTurn()
			return
		}
		ts := cm.GetTurnState()
		if ts == TurnInactive || ts == TurnComplete {
			cm.StartTurn()
		}
		cm.InputComplete()
	}

	vc.mu.Lock()
	vc.transitionTo(StateProcessing)
	vc.mu.Unlock()
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
