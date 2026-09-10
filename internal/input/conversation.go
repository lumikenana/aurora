package input

import (
	"sync"
	"time"

	"github.com/lipipi/aurora/internal/log"
)

// ConversationState represents the state of a conversation
type ConversationState int

const (
	// ConvIdle - no active conversation
	ConvIdle ConversationState = iota
	// ConvListening - waiting for user input (ASR active or waiting for PTT)
	ConvListening
	// ConvProcessing - ASR finished, LLM is processing
	ConvProcessing
	// ConvResponding - TTS is playing the response
	ConvResponding
	// ConvWaiting - response complete, waiting for next user input
	ConvWaiting
)

// String returns the string representation of the conversation state
func (s ConversationState) String() string {
	switch s {
	case ConvIdle:
		return "Idle"
	case ConvListening:
		return "Listening"
	case ConvProcessing:
		return "Processing"
	case ConvResponding:
		return "Responding"
	case ConvWaiting:
		return "Waiting"
	default:
		return "Unknown"
	}
}

// TurnState represents the state of a single turn
type TurnState int

const (
	// TurnInactive - turn not started
	TurnInactive TurnState = iota
	// TurnInput - receiving user input (ASR streaming)
	TurnInput
	// TurnProcessing - input complete, LLM processing
	TurnProcessing
	// TurnResponding - LLM response complete, TTS playing
	TurnResponding
	// TurnComplete - turn finished
	TurnComplete
)

// String returns the string representation of the turn state
func (s TurnState) String() string {
	switch s {
	case TurnInactive:
		return "Inactive"
	case TurnInput:
		return "Input"
	case TurnProcessing:
		return "Processing"
	case TurnResponding:
		return "Responding"
	case TurnComplete:
		return "Complete"
	default:
		return "Unknown"
	}
}

// ConversationConfig holds configuration for conversation behavior
type ConversationConfig struct {
	// WaitingTimeout is the duration to wait for next input before ending conversation
	WaitingTimeout time.Duration
	// ListeningTimeout 是「已唤醒但用户一直没有说话」的兜底（Phase 4）。
	// 与 WaitingTimeout 语义不同：Waiting 等待"下一轮开始"，Listening 等待
	// "本轮开始说话"。说话期间 ASR interim 会持续重置该 timer（ResetListeningTimer），
	// 不会截断正常语音；超时 → Conversation 结束 → Idle。
	// 仅作用于 WakeWord Conversation；PTT/Text standalone Turn 不参与（手动控制输入）。
	// <=0 禁用。
	ListeningTimeout time.Duration
}

// DefaultConversationConfig returns default conversation configuration
func DefaultConversationConfig() ConversationConfig {
	return ConversationConfig{
		WaitingTimeout: 30 * time.Second,
		// 15s：唤醒后用户需要几秒反应并组织语言；正常说话会被 interim 重置，
		// 服务端 VAD 在语音结束后 ~1-2s 内返回 final，15s 只是"完全没说话"的兜底。
		ListeningTimeout: 15 * time.Second,
	}
}

// ConversationManager manages the lifecycle of conversations and turns
type ConversationManager struct {
	config ConversationConfig

	// State
	mu                 sync.Mutex
	convState          ConversationState
	turnState          TurnState
	activeConversation bool
	standaloneTurn     bool   // PTT/Text standalone turn active (no conversation)
	endingRequested    bool   // 自然语言退出请求（exit_conversation_mode）：当前 Turn 完成后结束 Conversation
	convGeneration     uint64 // generation counter to prevent stale timers

	// Timer management
	waitingTimer     *time.Timer
	waitingTimerStop chan struct{} // signal to stop timer goroutine
	listeningTimer   *time.Timer   // Listening 超时（仅 WakeWord Conversation）

	// Callbacks
	onStateChange func(ConversationState)
	onTurnStart   func()
	onTurnEnd     func()
	onConvStart   func()
	onConvEnd     func()
}

// NewConversationManager creates a new conversation manager
func NewConversationManager(config ConversationConfig) *ConversationManager {
	return &ConversationManager{
		config:    config,
		convState: ConvIdle,
		turnState: TurnInactive,
	}
}

// SetCallbacks sets the callback functions for state changes
func (cm *ConversationManager) SetCallbacks(
	onStateChange func(ConversationState),
	onTurnStart func(),
	onTurnEnd func(),
	onConvStart func(),
	onConvEnd func(),
) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	cm.onStateChange = onStateChange
	cm.onTurnStart = onTurnStart
	cm.onTurnEnd = onTurnEnd
	cm.onConvStart = onConvStart
	cm.onConvEnd = onConvEnd
}

// GetState returns the current conversation state
func (cm *ConversationManager) GetState() ConversationState {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	return cm.convState
}

// GetTurnState returns the current turn state
func (cm *ConversationManager) GetTurnState() TurnState {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	return cm.turnState
}

// IsConversationActive returns whether a conversation is currently active
func (cm *ConversationManager) IsConversationActive() bool {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	return cm.activeConversation
}

// StartConversation starts a new conversation
// This should be called when WakeWord is detected
func (cm *ConversationManager) StartConversation() {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	if cm.activeConversation {
		log.Warn("conversation already active, ignoring start request")
		return
	}

	cm.activeConversation = true
	cm.endingRequested = false
	cm.convGeneration++
	cm.setConvStateLocked(ConvListening)
	cm.setTurnStateLocked(TurnInactive)

	log.Infof("conversation started (generation=%d)", cm.convGeneration)

	if cm.onConvStart != nil {
		// Release lock before calling callback to prevent deadlocks
		go cm.onConvStart()
	}
}

// EndConversation ends the current conversation
func (cm *ConversationManager) EndConversation() {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	if !cm.activeConversation {
		log.Warn("no active conversation to end")
		return
	}

	cm.endConversationLocked()
}

// endConversationLocked ends the active conversation (must be called with lock held).
// Fires onConvEnd exactly once per call.
func (cm *ConversationManager) endConversationLocked() {
	cm.stopWaitingTimerLocked()
	cm.stopListeningTimerLocked()
	cm.activeConversation = false
	cm.endingRequested = false
	cm.setConvStateLocked(ConvIdle)
	cm.setTurnStateLocked(TurnInactive)

	log.Infof("conversation ended (generation=%d)", cm.convGeneration)

	if cm.onConvEnd != nil {
		go cm.onConvEnd()
	}
}

// RequestEnd 请求结束当前 Conversation（自然语言退出，由 exit_conversation_mode
// LLM 工具驱动 —— Phase 4，替代旧的本地退出关键词匹配）。
//
// 语义：不立即打断当前 Turn —— 标记 endingRequested，当前 Turn 正常完成
// （ResponseComplete）后 Conversation 才结束并回 Idle（不进入 Waiting）。
// 若 Conversation 活跃但无进行中的 Turn（如 Waiting），立即结束。
//
// 无活跃 Conversation 时（含 PTT/Text standalone Turn）安全 no-op，返回 false
// —— standalone Turn 的生命周期不受影响。幂等：多次调用最多触发一次 onConvEnd。
func (cm *ConversationManager) RequestEnd() bool {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	if !cm.activeConversation {
		log.Debug("RequestEnd: no active conversation, ignoring")
		return false
	}

	// 无进行中的 Turn（Waiting 等）：没有需要等待完成的回复，立即结束
	if cm.turnState == TurnInactive || cm.turnState == TurnComplete {
		log.Info("RequestEnd: no turn in progress, ending conversation now")
		cm.endConversationLocked()
		return true
	}

	cm.endingRequested = true
	log.Infof("RequestEnd: will end conversation after current turn completes (turn=%s)", cm.turnState)
	return true
}

// StartStandaloneTurn starts a standalone turn outside any conversation
// (PTT trigger semantics). A standalone turn runs
// Input → Processing → Responding → Complete and then returns directly to
// ConvIdle — it never enters ConvWaiting and never starts the waiting timer.
// Returns false if a conversation is currently active (caller must interrupt
// or end it first).
func (cm *ConversationManager) StartStandaloneTurn() bool {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	if cm.activeConversation {
		log.Warn("cannot start standalone turn: conversation active")
		return false
	}

	if cm.standaloneTurn {
		// Previous standalone turn never completed (e.g. PTT was cancelled
		// before the agent ran). Reset it so a new turn can start.
		log.Warnf("resetting stale standalone turn (state=%s)", cm.turnState)
		cm.standaloneTurn = false
		cm.setTurnStateLocked(TurnInactive)
		cm.setConvStateLocked(ConvIdle)
	}

	cm.standaloneTurn = true
	cm.convGeneration++ // invalidate any stale timers/callbacks from a previous lifecycle
	cm.setTurnStateLocked(TurnInput)
	cm.setConvStateLocked(ConvListening)

	log.Debugf("standalone turn started (generation=%d)", cm.convGeneration)

	if cm.onTurnStart != nil {
		go cm.onTurnStart()
	}
	return true
}

// CancelTurn aborts a turn that is still in the input phase (e.g. ASR
// returned an empty final result — the user was heard but said nothing).
// Conversation: the turn is dropped and the conversation returns to Waiting
// (waiting timer restarted). Standalone: the turn is dropped and the manager
// returns to Idle.
func (cm *ConversationManager) CancelTurn() {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	if cm.turnState != TurnInput {
		log.Warnf("cannot cancel turn: turn state is %s, not Input", cm.turnState)
		return
	}

	cm.setTurnStateLocked(TurnComplete)

	if cm.standaloneTurn {
		cm.standaloneTurn = false
		cm.setConvStateLocked(ConvIdle)
		log.Debug("standalone turn cancelled, back to idle")
		return
	}

	if !cm.activeConversation {
		return
	}

	// 消费待处理的退出请求（Phase 5）：RequestEnd 在 Turn 输入阶段标记了
	// endingRequested，随后 ASR 空结果导致 CancelTurn —— 用户已明确表达退出
	// 意图，不能回 Waiting 干等 30s 超时，直接结束 Conversation。
	// RequestEnd 语义不变（正常路径仍等当前 Turn 完成）；这里只覆盖异常结束路径。
	if cm.endingRequested {
		log.Info("turn cancelled with pending exit request, ending conversation")
		cm.endConversationLocked()
		return
	}

	cm.setConvStateLocked(ConvWaiting)
	cm.startWaitingTimerLocked()
	log.Debug("turn cancelled, conversation back to waiting")
}

// Interrupt immediately terminates any active conversation or standalone turn.
// Used when PTT preempts the current voice activity or the voice mode changes.
// Fires onConvEnd only if a conversation was active.
func (cm *ConversationManager) Interrupt() {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	hadConversation := cm.activeConversation
	hadTurn := cm.standaloneTurn || (cm.turnState != TurnInactive && cm.turnState != TurnComplete)

	if !hadConversation && !hadTurn {
		return
	}

	cm.stopWaitingTimerLocked()
	cm.stopListeningTimerLocked()
	cm.activeConversation = false
	cm.standaloneTurn = false
	cm.endingRequested = false
	cm.convGeneration++ // invalidate pending timers/callbacks
	cm.setConvStateLocked(ConvIdle)
	cm.setTurnStateLocked(TurnInactive)

	log.Infof("interrupted (conversation=%v, generation=%d)", hadConversation, cm.convGeneration)

	if hadConversation && cm.onConvEnd != nil {
		go cm.onConvEnd()
	}
}

// IsTurnInProgress returns whether any turn (conversation or standalone) is
// currently between Input and Responding.
func (cm *ConversationManager) IsTurnInProgress() bool {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	return cm.turnState != TurnInactive && cm.turnState != TurnComplete
}

// IsStandaloneTurnActive returns whether a standalone (PTT) turn is active.
func (cm *ConversationManager) IsStandaloneTurnActive() bool {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	return cm.standaloneTurn
}

// StartTurn starts a new turn within the current conversation
// This should be called when user input begins (PTT press or ASR starts)
func (cm *ConversationManager) StartTurn() {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	if !cm.activeConversation {
		log.Warn("cannot start turn: no active conversation")
		return
	}

	if cm.turnState != TurnInactive && cm.turnState != TurnComplete {
		log.Warnf("cannot start turn: current turn state is %s", cm.turnState)
		return
	}

	cm.stopWaitingTimerLocked()
	cm.setTurnStateLocked(TurnInput)
	cm.setConvStateLocked(ConvListening)

	log.Infof("turn started (generation=%d)", cm.convGeneration)

	if cm.onTurnStart != nil {
		go cm.onTurnStart()
	}
}

// InputComplete marks the input phase as complete (ASR finished)
func (cm *ConversationManager) InputComplete() {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	if cm.turnState != TurnInput {
		log.Warnf("cannot complete input: turn state is %s, not Input", cm.turnState)
		return
	}

	cm.setTurnStateLocked(TurnProcessing)
	cm.setConvStateLocked(ConvProcessing)

	log.Debug("input complete, processing")
}

// ResponseComplete marks the response phase as complete (TTS finished)
// This transitions to Waiting state and starts the waiting timer
func (cm *ConversationManager) ResponseComplete() {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	if cm.turnState != TurnResponding {
		log.Warnf("cannot complete response: turn state is %s, not Responding", cm.turnState)
		return
	}

	cm.setTurnStateLocked(TurnComplete)

	log.Debug("response complete")

	if cm.onTurnEnd != nil {
		go cm.onTurnEnd()
	}

	// Standalone turn (PTT/Text): no conversation — back to Idle, no waiting timer.
	if cm.standaloneTurn {
		cm.standaloneTurn = false
		cm.setConvStateLocked(ConvIdle)
		log.Debug("standalone turn complete, back to idle")
		return
	}

	// 自然语言退出（exit_conversation_mode → RequestEnd）：当前 Turn 已正常
	// 完成，直接结束 Conversation → Idle，不进入 Waiting、不启动 waiting timer。
	if cm.endingRequested {
		log.Info("ending requested: conversation ends after turn completion")
		cm.endConversationLocked()
		return
	}

	cm.setConvStateLocked(ConvWaiting)

	log.Infof("entering waiting state (timeout=%v, generation=%d)",
		cm.config.WaitingTimeout, cm.convGeneration)

	// Start waiting timer for next input
	cm.startWaitingTimerLocked()
}

// ProcessingComplete marks the LLM processing as complete
func (cm *ConversationManager) ProcessingComplete() {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	if cm.turnState != TurnProcessing {
		log.Warnf("cannot complete processing: turn state is %s, not Processing", cm.turnState)
		return
	}

	cm.setTurnStateLocked(TurnResponding)
	cm.setConvStateLocked(ConvResponding)

	log.Debug("processing complete, responding")
}

// setState helpers (must be called with lock held)
func (cm *ConversationManager) setConvStateLocked(state ConversationState) {
	if cm.convState != state {
		oldState := cm.convState
		cm.convState = state
		log.Debugf("conversation state: %s → %s", oldState, state)

		// Listening timeout 生命周期（单一不变量，集中管理避免遗漏转换点）：
		// timer 仅在「WakeWord Conversation 等待用户输入」期间运行 ——
		// 即进入 ConvListening 且 Conversation 活跃且非 standalone Turn。
		// PTT/Text standalone Turn 也经过 ConvListening，但输入由用户手动控制
		// （按键松开 / 文本已就绪），不参与 Listening timeout（spec §7.3）。
		if state == ConvListening && cm.activeConversation && !cm.standaloneTurn {
			cm.startListeningTimerLocked()
		} else if oldState == ConvListening {
			cm.stopListeningTimerLocked()
		}

		if cm.onStateChange != nil {
			// Copy callback to avoid calling it with lock held
			callback := cm.onStateChange
			go callback(state)
		}
	}
}

func (cm *ConversationManager) setTurnStateLocked(state TurnState) {
	if cm.turnState != state {
		oldState := cm.turnState
		cm.turnState = state
		log.Debugf("turn state: %s → %s", oldState, state)
	}
}

// startWaitingTimerLocked starts the waiting timer (must be called with lock held)
func (cm *ConversationManager) startWaitingTimerLocked() {
	cm.stopWaitingTimerLocked()

	if cm.config.WaitingTimeout <= 0 {
		return
	}

	generation := cm.convGeneration
	cm.waitingTimerStop = make(chan struct{})

	cm.waitingTimer = time.AfterFunc(cm.config.WaitingTimeout, func() {
		cm.handleWaitingTimeout(generation)
	})

	log.Debugf("waiting timer started (timeout=%v, generation=%d)",
		cm.config.WaitingTimeout, generation)
}

// stopWaitingTimerLocked stops the waiting timer (must be called with lock held)
func (cm *ConversationManager) stopWaitingTimerLocked() {
	if cm.waitingTimer != nil {
		cm.waitingTimer.Stop()
		cm.waitingTimer = nil
	}

	if cm.waitingTimerStop != nil {
		select {
		case <-cm.waitingTimerStop:
			// already closed
		default:
			close(cm.waitingTimerStop)
		}
		cm.waitingTimerStop = nil
	}
}

// handleWaitingTimeout handles the waiting timeout
func (cm *ConversationManager) handleWaitingTimeout(generation uint64) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	// Check if this is a stale timer from an old conversation
	if generation != cm.convGeneration {
		log.Debugf("ignoring stale timer (timer generation=%d, current generation=%d)",
			generation, cm.convGeneration)
		return
	}

	// Check if we're still in Waiting state
	if cm.convState != ConvWaiting {
		log.Debugf("timer fired but state is %s, not Waiting", cm.convState)
		return
	}

	if !cm.activeConversation {
		log.Debug("timer fired but no active conversation")
		return
	}

	log.Infof("waiting timeout, ending conversation (generation=%d)", generation)
	cm.activeConversation = false
	cm.setConvStateLocked(ConvIdle)
	cm.setTurnStateLocked(TurnInactive)

	if cm.onConvEnd != nil {
		go cm.onConvEnd()
	}
}

// startListeningTimerLocked starts the listening timer (must be called with lock held).
// 复用 Waiting timer 的并发安全模式：generation 快照 + 触发时状态校验，
// 旧 timer 不会错误结束新的 Conversation / Turn。
func (cm *ConversationManager) startListeningTimerLocked() {
	cm.stopListeningTimerLocked()

	if cm.config.ListeningTimeout <= 0 {
		return
	}

	generation := cm.convGeneration
	cm.listeningTimer = time.AfterFunc(cm.config.ListeningTimeout, func() {
		cm.handleListeningTimeout(generation)
	})

	log.Debugf("listening timer started (timeout=%v, generation=%d)",
		cm.config.ListeningTimeout, generation)
}

// stopListeningTimerLocked stops the listening timer (must be called with lock held)
func (cm *ConversationManager) stopListeningTimerLocked() {
	if cm.listeningTimer != nil {
		cm.listeningTimer.Stop()
		cm.listeningTimer = nil
	}
}

// handleListeningTimeout handles the listening timeout：
// WakeWord Conversation 已进入 Listening 但用户一直没有任何有效输入 →
// 结束 Conversation → Idle（等待"开始说话"的兜底，区别于 Waiting 超时）。
func (cm *ConversationManager) handleListeningTimeout(generation uint64) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	// Stale timer（旧 Conversation / 旧生命周期）不得影响当前状态
	if generation != cm.convGeneration {
		log.Debugf("ignoring stale listening timer (timer generation=%d, current generation=%d)",
			generation, cm.convGeneration)
		return
	}

	// 只在「Conversation 活跃且仍在 Listening」时生效；
	// standalone Turn（PTT/Text）经过 Listening 但不参与 Listening timeout
	if cm.convState != ConvListening || !cm.activeConversation || cm.standaloneTurn {
		log.Debugf("listening timer fired but state is %s (active=%v, standalone=%v), ignoring",
			cm.convState, cm.activeConversation, cm.standaloneTurn)
		return
	}

	log.Infof("listening timeout: no user input after wake, ending conversation (generation=%d)", generation)
	cm.endConversationLocked()
}

// ResetListeningTimer 重置 Listening 超时（ASR interim 到达 = 用户正在说话）。
// 仅 Conversation 处于 Listening 时有效；其他状态（含 standalone Turn）no-op。
func (cm *ConversationManager) ResetListeningTimer() {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	if cm.convState != ConvListening || !cm.activeConversation || cm.standaloneTurn {
		return
	}

	cm.stopListeningTimerLocked()
	cm.startListeningTimerLocked()

	log.Debug("listening timer reset")
}

// ResetWaitingTimer resets the waiting timer (call when new input arrives)
func (cm *ConversationManager) ResetWaitingTimer() {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	if cm.convState != ConvWaiting {
		return
	}

	cm.stopWaitingTimerLocked()
	cm.startWaitingTimerLocked()

	log.Debug("waiting timer reset")
}

// Stop stops the conversation manager and cleans up resources
func (cm *ConversationManager) Stop() {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	cm.stopWaitingTimerLocked()
	cm.stopListeningTimerLocked()

	if cm.activeConversation {
		cm.activeConversation = false
		cm.setConvStateLocked(ConvIdle)
		cm.setTurnStateLocked(TurnInactive)
		log.Info("conversation manager stopped")
	}
}
