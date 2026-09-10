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
}

// DefaultConversationConfig returns default conversation configuration
func DefaultConversationConfig() ConversationConfig {
	return ConversationConfig{
		WaitingTimeout: 30 * time.Second,
	}
}

// ConversationManager manages the lifecycle of conversations and turns
type ConversationManager struct {
	config ConversationConfig

	// State
	mu                  sync.Mutex
	convState           ConversationState
	turnState           TurnState
	activeConversation  bool
	convGeneration      uint64 // generation counter to prevent stale timers

	// Timer management
	waitingTimer     *time.Timer
	waitingTimerStop chan struct{} // signal to stop timer goroutine

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

	cm.stopWaitingTimerLocked()
	cm.activeConversation = false
	cm.setConvStateLocked(ConvIdle)
	cm.setTurnStateLocked(TurnInactive)

	log.Infof("conversation ended (generation=%d)", cm.convGeneration)

	if cm.onConvEnd != nil {
		go cm.onConvEnd()
	}
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

	log.Debug("turn started")

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
	cm.setConvStateLocked(ConvWaiting)

	log.Debug("response complete, entering waiting state")

	if cm.onTurnEnd != nil {
		go cm.onTurnEnd()
	}

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

	if cm.activeConversation {
		cm.activeConversation = false
		cm.setConvStateLocked(ConvIdle)
		cm.setTurnStateLocked(TurnInactive)
		log.Info("conversation manager stopped")
	}
}
