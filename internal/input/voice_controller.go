package input

import (
	"context"
	"sync"
	"time"

	"github.com/lipipi/aurora/internal/audio"
	"github.com/lipipi/aurora/internal/errors"
	"github.com/lipipi/aurora/internal/log"
	"github.com/lipipi/aurora/internal/speech"
	"github.com/lipipi/aurora/internal/wakeword"
)

// VoiceState defines the possible states of the voice controller.
type VoiceState int

const (
	// StateIdle idle state:
	// - If wakeword enabled: wakeword detector running, listening for wakeword
	// - If PTT enabled: idle, waiting for hotkey press
	StateIdle VoiceState = iota

	// StateWakeDetected wakeword just detected, starting ASR session
	StateWakeDetected

	// StateSpeechListening ASR is running, listening to user speech after wakeword
	StateSpeechListening

	// StateProcessing ASR finished, Agent is processing the query and generating response
	StateProcessing

	// StateSpeaking TTS is playing the response (future-proof for TTS)
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

// VoiceController is the unified controller for voice processing.
// It manages:
// - Wake word detection (continuous background listening)
// - ASR session after wake detection
// - State transitions
// - Mutual exclusion with PTT
type VoiceController struct {
	wakeDetector   wakeword.WakeWordDetector
	speechRecognizer speech.SpeechRecognizer
	audioCap        audio.AudioCapture

	// Current state, guarded by mu
	mu            sync.Mutex
	state         VoiceState
	lastWakeTime  time.Time
	cooldown      time.Duration

	// Channels
	audioCh        chan []byte          // Audio from capture → wakeword / ASR
	asrAudioCh     chan []byte          // Audio → ASR (only after wake)
	wakeEventCh    <-chan wakeword.WakeEvent

	// Context for cancellation
	ctx           context.Context
	cancel        context.CancelFunc

	// Callbacks
	onStateChange  func(VoiceState)
	onTranscript  func(speech.Transcript)
	onWakeDetected func()
	onFinalResult func(string)

	// Configuration
	enabled       bool
}

// VoiceControllerOption is a functional option for VoiceController.
type VoiceControllerOption func(*VoiceController)

// WithOnStateChange sets the state change callback.
func WithOnStateChange(cb func(VoiceState)) VoiceControllerOption {
	return func(vc *VoiceController) {
		vc.onStateChange = cb
	}
}

// WithOnTranscript sets the transcript callback.
func WithOnTranscript(cb func(speech.Transcript)) VoiceControllerOption {
	return func(vc *VoiceController) {
		vc.onTranscript = cb
	}
}

// WithOnWakeDetected sets the wake detected callback.
func WithOnWakeDetected(cb func()) VoiceControllerOption {
	return func(vc *VoiceController) {
		vc.onWakeDetected = cb
	}
}

// WithOnFinalResult sets the final result callback.
func WithOnFinalResult(cb func(string)) VoiceControllerOption {
	return func(vc *VoiceController) {
		vc.onFinalResult = cb
	}
}

// WithEnabled sets whether wakeword is enabled.
func WithEnabled(enabled bool) VoiceControllerOption {
	return func(vc *VoiceController) {
		vc.enabled = enabled
	}
}

// WithCooldown sets the cooldown after detection.
func WithCooldown(cooldown time.Duration) VoiceControllerOption {
	return func(vc *VoiceController) {
		vc.cooldown = cooldown
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
		wakeDetector:     wakeDetector,
		speechRecognizer: speechRecognizer,
		audioCap:         audioCap,
		state:            StateIdle,
		enabled:          false,
		cooldown:         1000 * time.Millisecond,
		audioCh:          make(chan []byte, 16),
		asrAudioCh:       make(chan []byte, 16),
	}

	for _, opt := range options {
		opt(vc)
	}

	return vc
}

// Start starts the voice controller.
// If wakeword is enabled, starts continuous wakeword detection.
func (vc *VoiceController) Start(ctx context.Context) errors.AppError {
	vc.mu.Lock()
	defer vc.mu.Unlock()

	if !vc.enabled || !vc.wakeDetector.IsConfigured() {
		log.Info("voice controller: wakeword disabled or not configured, not starting")
		vc.state = StateIdle
		return nil
	}

	if vc.state != StateIdle {
		log.Warnf("voice controller: already started, state: %s", vc.state)
		return nil
	}

	vc.ctx, vc.cancel = context.WithCancel(ctx)

	// Start audio capture
	audioCallback := func(data []byte) {
		// Non-blocking send to prevent blocking audio callback
		select {
		case vc.audioCh <- data:
		default:
			log.Debug("voice controller: audio channel full, dropping frame")
		}
	}

	if err := vc.audioCap.Start(audioCallback); err != nil {
		vc.cancel()
		return errors.Wrap(errors.CodeWakeWordFailed, "failed to start audio capture", err)
	}

	// Start wakeword detector
	if err := vc.wakeDetector.Start(vc.ctx, vc.audioCh); err != nil {
		vc.audioCap.Stop()
		vc.cancel()
		return errors.Wrap(errors.CodeWakeWordFailed, "failed to start wakeword detector", err)
	}

	vc.wakeEventCh = vc.wakeDetector.Events()

	// Start the main event loop
	go vc.eventLoop()

	vc.state = StateIdle
	log.Infof("voice controller started, wakeword enabled: %v, state: %s", vc.enabled, vc.state)

	vc.notifyStateChange()
	return nil
}

// Stop stops the voice controller and releases all resources.
func (vc *VoiceController) Stop() {
	vc.mu.Lock()
	defer vc.mu.Unlock()

	if vc.cancel != nil {
		vc.cancel()
	}

	if vc.audioCap.IsRecording() {
		vc.audioCap.Stop()
	}

	if vc.state == StateSpeechListening || vc.state == StateProcessing {
		// If we're in ASR, cancel the session
		_ = vc.speechRecognizer.CancelSession()
	}

	vc.wakeDetector.Stop()
	vc.state = StateIdle
	vc.notifyStateChange()
	log.Info("voice controller stopped")
}

// GetState returns the current state.
func (vc *VoiceController) GetState() VoiceState {
	vc.mu.Lock()
	defer vc.mu.Unlock()
	return vc.state
}

// IsEnabled returns whether wakeword detection is enabled.
func (vc *VoiceController) IsEnabled() bool {
	vc.mu.Lock()
	defer vc.mu.Unlock()
	return vc.enabled
}

// SetEnabled enables or disables wakeword detection.
// If disabling, stops everything. If enabling, starts if not already started.
func (vc *VoiceController) SetEnabled(enabled bool, ctx context.Context) errors.AppError {
	vc.mu.Lock()
	defer vc.mu.Unlock()

	if vc.enabled == enabled {
		return nil
	}

	vc.enabled = enabled

	if !enabled {
		if vc.cancel != nil {
			vc.cancel()
		}
		if vc.audioCap.IsRecording() {
			vc.audioCap.Stop()
		}
		vc.wakeDetector.Stop()
		vc.state = StateIdle
		vc.notifyStateChange()
		return nil
	}

	// Enabling: already started if context exists
	if vc.ctx != nil && vc.state == StateIdle {
		return nil
	}

	// Need to start
	// Start will be done in the main context
	return nil
}

// PTTPreempt is called when PTT is activated.
// We need to cancel any ongoing wakeword → ASR session and yield to PTT.
func (vc *VoiceController) PTTPreempt() {
	vc.mu.Lock()
	defer vc.mu.Unlock()

	if vc.state != StateIdle {
		log.Debug("voice controller: PTT preempting current session")
		vc.transitionTo(StateIdle)
		// ASR session is canceled in transitionTo if needed
	}
}

// PTTFinished is called when PTT recording completes.
// If wakeword is enabled, we go back to idle listening.
func (vc *VoiceController) PTTFinished() {
	vc.mu.Lock()
	defer vc.mu.Unlock()

	if vc.enabled && vc.state == StateIdle {
		// Already idle, nothing to do
		return
	}

	log.Debug("voice controller: PTT finished, returning to idle wakeword listening")
	vc.transitionTo(StateIdle)
}

// eventLoop is the main event loop running in a separate goroutine.
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
	// Check current state - can only accept detection from idle
	if vc.state != StateIdle {
		log.Debugf("wakeword detected but state is %s, ignoring", vc.state)
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

	// Start ASR session
	// We need to release the mutex before starting ASR because start can block
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

	// Now we need to start copying audio from audioCh to asrAudioCh
	// This happens in a separate goroutine because we need to copy
	// until the session ends.
	go vc.copyAudioToASR()
}

// copyAudioToASR copies audio chunks from the main audio channel to the ASR channel.
// It stops when we leave StateSpeechListening.
func (vc *VoiceController) copyAudioToASR() {
	for {
		vc.mu.Lock()
		if vc.state != StateSpeechListening {
			vc.mu.Unlock()
			return
		}
		vc.mu.Unlock()

		select {
		case data, ok := <-vc.audioCh:
			if !ok {
				return
			}
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
	defer vc.mu.Unlock()

	// Forward transcript to callback
	if vc.onTranscript != nil {
		vc.onTranscript(t)
	}

	// Only handle final transcript here
	if t.IsFinal {
		if vc.onFinalResult != nil && t.Text != "" {
			vc.onFinalResult(t.Text)
		}
		// Transition to processing
		vc.transitionTo(StateProcessing)

		// After processing completes, we'll transition back to idle
		// This happens when the agent completes its response. We expect
		// that the higher level will call OnAgentComplete when done.
	}
}

// OnAgentComplete is called when the agent has finished processing the final result.
// We transition back to idle to resume wakeword listening.
func (vc *VoiceController) OnAgentComplete() {
	vc.mu.Lock()
	defer vc.mu.Unlock()

	if vc.state == StateProcessing {
		// If we had TTS, we'd go to StateSpeaking here
		// But since we don't have TTS yet, go directly back to idle
		vc.transitionTo(StateIdle)
	}
}

// OnTTSComplete is called when TTS has finished playing the response.
// Only used if TTS is enabled in the future.
func (vc *VoiceController) OnTTSComplete() {
	vc.mu.Lock()
	defer vc.mu.Unlock()

	if vc.state == StateSpeaking {
		vc.transitionTo(StateIdle)
	}
}

// transitionTo transitions to a new state, handling cleanup of the old state.
// Must be called with mu locked.
func (vc *VoiceController) transitionTo(newState VoiceState) {
	oldState := vc.state

	// Cleanup based on old state
	switch oldState {
	case StateSpeechListening, StateWakeDetected:
		// End ASR session
		if err := vc.speechRecognizer.EndSession(); err != nil {
			log.Errorf("failed to end ASR session: %v", err)
		}
		// Drain the asrAudioCh to clear old data
		vc.drainASRAudioCh()
	}

	// If we're transitioning TO Idle FROM any non-Idle state,
	// we need to ensure audio capture is still running because we need
	// to keep listening for wake word continuously.
	// Audio capture is only stopped when VoiceController.Stop() is called.
	if newState == StateIdle && !vc.audioCap.IsRecording() {
		// Restart audio capture for continuous wake word listening
		audioCallback := func(data []byte) {
			select {
			case vc.audioCh <- data:
			default:
				log.Debug("voice controller: audio channel full, dropping frame")
			}
		}
		if err := vc.audioCap.Start(audioCallback); err != nil {
			log.Errorf("failed to restart audio capture for wake word listening: %v", err)
		} else {
			log.Debug("voice controller: audio capture restarted for continuous wake listening")
		}
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
		vc.onStateChange(vc.state)
	}
}
