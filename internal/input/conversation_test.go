package input

import (
	"sync"
	"testing"
	"time"
)

func TestConversationManager_NormalLifecycle(t *testing.T) {
	config := ConversationConfig{
		WaitingTimeout: 100 * time.Millisecond, // Short timeout for testing
	}
	cm := NewConversationManager(config)

	// Track state changes
	var stateChanges []ConversationState
	var mu sync.Mutex
	cm.SetCallbacks(
		func(state ConversationState) {
			mu.Lock()
			stateChanges = append(stateChanges, state)
			mu.Unlock()
		},
		nil, nil, nil, nil,
	)

	// Initial state
	if cm.GetState() != ConvIdle {
		t.Errorf("expected initial state ConvIdle, got %s", cm.GetState())
	}

	// Start conversation
	cm.StartConversation()
	time.Sleep(10 * time.Millisecond) // Allow callback to execute

	if !cm.IsConversationActive() {
		t.Error("conversation should be active after StartConversation")
	}
	if cm.GetState() != ConvListening {
		t.Errorf("expected state ConvListening after start, got %s", cm.GetState())
	}

	// Start turn
	cm.StartTurn()
	time.Sleep(10 * time.Millisecond)
	if cm.GetTurnState() != TurnInput {
		t.Errorf("expected turn state TurnInput, got %s", cm.GetTurnState())
	}

	// Input complete
	cm.InputComplete()
	time.Sleep(10 * time.Millisecond)
	if cm.GetTurnState() != TurnProcessing {
		t.Errorf("expected turn state TurnProcessing, got %s", cm.GetTurnState())
	}
	if cm.GetState() != ConvProcessing {
		t.Errorf("expected conv state ConvProcessing, got %s", cm.GetState())
	}

	// Processing complete
	cm.ProcessingComplete()
	time.Sleep(10 * time.Millisecond)
	if cm.GetTurnState() != TurnResponding {
		t.Errorf("expected turn state TurnResponding, got %s", cm.GetTurnState())
	}
	if cm.GetState() != ConvResponding {
		t.Errorf("expected conv state ConvResponding, got %s", cm.GetState())
	}

	// Response complete
	cm.ResponseComplete()
	time.Sleep(10 * time.Millisecond)
	if cm.GetTurnState() != TurnComplete {
		t.Errorf("expected turn state TurnComplete, got %s", cm.GetTurnState())
	}
	if cm.GetState() != ConvWaiting {
		t.Errorf("expected conv state ConvWaiting, got %s", cm.GetState())
	}

	// Start another turn
	cm.StartTurn()
	time.Sleep(10 * time.Millisecond)
	if cm.GetTurnState() != TurnInput {
		t.Errorf("expected turn state TurnInput for second turn, got %s", cm.GetTurnState())
	}

	// End conversation
	cm.EndConversation()
	time.Sleep(10 * time.Millisecond)
	if cm.IsConversationActive() {
		t.Error("conversation should not be active after EndConversation")
	}
	if cm.GetState() != ConvIdle {
		t.Errorf("expected state ConvIdle after end, got %s", cm.GetState())
	}

	// Verify state changes
	mu.Lock()
	if len(stateChanges) == 0 {
		t.Error("expected state change callbacks")
	}
	mu.Unlock()
}

func TestConversationManager_Timeout(t *testing.T) {
	config := ConversationConfig{
		WaitingTimeout: 100 * time.Millisecond,
	}
	cm := NewConversationManager(config)

	var convEnded bool
	var mu sync.Mutex
	cm.SetCallbacks(nil, nil, nil, nil, func() {
		mu.Lock()
		convEnded = true
		mu.Unlock()
	})

	// Start conversation
	cm.StartConversation()
	cm.StartTurn()
	cm.InputComplete()
	cm.ProcessingComplete()
	cm.ResponseComplete()

	// Should be in Waiting state
	if cm.GetState() != ConvWaiting {
		t.Errorf("expected state ConvWaiting, got %s", cm.GetState())
	}

	// Wait for timeout
	time.Sleep(200 * time.Millisecond)

	// Should have ended
	mu.Lock()
	ended := convEnded
	mu.Unlock()

	if !ended {
		t.Error("conversation should have ended after timeout")
	}
	if cm.IsConversationActive() {
		t.Error("conversation should not be active after timeout")
	}
	if cm.GetState() != ConvIdle {
		t.Errorf("expected state ConvIdle after timeout, got %s", cm.GetState())
	}
}

func TestConversationManager_TimeoutReset(t *testing.T) {
	config := ConversationConfig{
		WaitingTimeout: 150 * time.Millisecond,
	}
	cm := NewConversationManager(config)

	var convEnded bool
	var mu sync.Mutex
	cm.SetCallbacks(nil, nil, nil, nil, func() {
		mu.Lock()
		convEnded = true
		mu.Unlock()
	})

	// Start conversation and complete first turn
	cm.StartConversation()
	cm.StartTurn()
	cm.InputComplete()
	cm.ProcessingComplete()
	cm.ResponseComplete()

	// Wait for 80ms (less than timeout)
	time.Sleep(80 * time.Millisecond)

	// Reset timer by simulating new input
	cm.ResetWaitingTimer()

	// Wait for another 80ms (total 160ms, but timer was reset at 80ms)
	time.Sleep(80 * time.Millisecond)

	// Should still be active
	if !cm.IsConversationActive() {
		t.Error("conversation should still be active after timer reset")
	}
	if cm.GetState() != ConvWaiting {
		t.Errorf("expected state ConvWaiting, got %s", cm.GetState())
	}

	// Wait for timeout
	time.Sleep(100 * time.Millisecond)

	// Now should have ended
	mu.Lock()
	ended := convEnded
	mu.Unlock()

	if !ended {
		t.Error("conversation should have ended after final timeout")
	}
}

func TestConversationManager_StaleTimer(t *testing.T) {
	config := ConversationConfig{
		WaitingTimeout: 100 * time.Millisecond,
	}
	cm := NewConversationManager(config)

	var convEndCount int
	var mu sync.Mutex
	cm.SetCallbacks(nil, nil, nil, nil, func() {
		mu.Lock()
		convEndCount++
		mu.Unlock()
	})

	// Start first conversation
	cm.StartConversation()
	cm.StartTurn()
	cm.InputComplete()
	cm.ProcessingComplete()
	cm.ResponseComplete()

	// Wait a bit but not enough for timeout
	time.Sleep(50 * time.Millisecond)

	// End conversation manually
	cm.EndConversation()
	time.Sleep(10 * time.Millisecond)

	// Start second conversation
	cm.StartConversation()
	cm.StartTurn()
	cm.InputComplete()
	cm.ProcessingComplete()
	cm.ResponseComplete()

	// Wait for what would have been the timeout from first conversation
	time.Sleep(100 * time.Millisecond)

	// Second conversation should still be active (stale timer from first should not affect it)
	if !cm.IsConversationActive() {
		t.Error("second conversation should still be active")
	}
	if cm.GetState() != ConvWaiting {
		t.Errorf("expected state ConvWaiting, got %s", cm.GetState())
	}

	// Check that onConvEnd was called exactly twice (once for manual end, will be once more for timeout)
	mu.Lock()
	count := convEndCount
	mu.Unlock()

	if count != 1 {
		t.Errorf("expected convEndCount to be 1 at this point, got %d", count)
	}

	// Wait for second conversation to timeout
	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	count = convEndCount
	mu.Unlock()

	if count != 2 {
		t.Errorf("expected convEndCount to be 2 after second timeout, got %d", count)
	}
}

func TestConversationManager_IllegalStateTransitions(t *testing.T) {
	config := DefaultConversationConfig()
	cm := NewConversationManager(config)

	// Try to start turn without active conversation
	cm.StartTurn()
	if cm.GetTurnState() != TurnInactive {
		t.Error("should not be able to start turn without active conversation")
	}

	// Try to complete input without starting turn
	cm.StartConversation()
	cm.InputComplete()
	if cm.GetTurnState() != TurnInactive {
		t.Error("should not be able to complete input without starting turn")
	}

	// Try to complete processing without completing input
	cm.StartTurn()
	cm.ProcessingComplete()
	if cm.GetTurnState() != TurnInput {
		t.Error("should not be able to complete processing without completing input")
	}

	// Try to complete response without completing processing
	cm.InputComplete()
	cm.ResponseComplete()
	if cm.GetTurnState() != TurnProcessing {
		t.Error("should not be able to complete response without completing processing")
	}

	cm.EndConversation()
}

func TestConversationManager_MultipleTurns(t *testing.T) {
	config := ConversationConfig{
		WaitingTimeout: 100 * time.Millisecond,
	}
	cm := NewConversationManager(config)

	var turnCount int
	var mu sync.Mutex
	cm.SetCallbacks(nil, nil, func() {
		mu.Lock()
		turnCount++
		mu.Unlock()
	}, nil, nil)

	cm.StartConversation()

	// Complete 3 turns
	for i := 0; i < 3; i++ {
		cm.StartTurn()
		cm.InputComplete()
		cm.ProcessingComplete()
		cm.ResponseComplete()

		// Reset timer to prevent timeout
		cm.ResetWaitingTimer()

		// Wait for callbacks to execute
		time.Sleep(10 * time.Millisecond)
	}

	// Should have 3 completed turns
	mu.Lock()
	count := turnCount
	mu.Unlock()

	if count != 3 {
		t.Errorf("expected 3 completed turns, got %d", count)
	}

	cm.EndConversation()
}

func TestConversationManager_Stop(t *testing.T) {
	config := DefaultConversationConfig()
	cm := NewConversationManager(config)

	cm.StartConversation()
	cm.StartTurn()

	if !cm.IsConversationActive() {
		t.Error("conversation should be active")
	}

	cm.Stop()

	if cm.IsConversationActive() {
		t.Error("conversation should not be active after Stop")
	}
	if cm.GetState() != ConvIdle {
		t.Errorf("expected state ConvIdle after Stop, got %s", cm.GetState())
	}
}

func TestConversationManager_StartWhileActive(t *testing.T) {
	config := DefaultConversationConfig()
	cm := NewConversationManager(config)

	cm.StartConversation()

	// Try to start again
	cm.StartConversation()

	// Should still have only one conversation
	if !cm.IsConversationActive() {
		t.Error("conversation should be active")
	}

	cm.EndConversation()
}

func TestConversationManager_EndWhileInactive(t *testing.T) {
	config := DefaultConversationConfig()
	cm := NewConversationManager(config)

	// Try to end without starting
	cm.EndConversation()

	// Should not panic or error
	if cm.IsConversationActive() {
		t.Error("conversation should not be active")
	}
}

func TestConversationState_String(t *testing.T) {
	tests := []struct {
		state    ConversationState
		expected string
	}{
		{ConvIdle, "Idle"},
		{ConvListening, "Listening"},
		{ConvProcessing, "Processing"},
		{ConvResponding, "Responding"},
		{ConvWaiting, "Waiting"},
		{ConversationState(99), "Unknown"},
	}

	for _, tt := range tests {
		if got := tt.state.String(); got != tt.expected {
			t.Errorf("ConversationState(%d).String() = %s, want %s", tt.state, got, tt.expected)
		}
	}
}

func TestTurnState_String(t *testing.T) {
	tests := []struct {
		state    TurnState
		expected string
	}{
		{TurnInactive, "Inactive"},
		{TurnInput, "Input"},
		{TurnProcessing, "Processing"},
		{TurnResponding, "Responding"},
		{TurnComplete, "Complete"},
		{TurnState(99), "Unknown"},
	}

	for _, tt := range tests {
		if got := tt.state.String(); got != tt.expected {
			t.Errorf("TurnState(%d).String() = %s, want %s", tt.state, got, tt.expected)
		}
	}
}

func TestConversationManager_ConcurrentAccess(t *testing.T) {
	config := ConversationConfig{
		WaitingTimeout: 100 * time.Millisecond,
	}
	cm := NewConversationManager(config)

	cm.StartConversation()

	var wg sync.WaitGroup
	iterations := 100

	// Concurrent state reads
	for i := 0; i < iterations; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = cm.GetState()
			_ = cm.GetTurnState()
			_ = cm.IsConversationActive()
		}()
	}

	// Concurrent state changes
	for i := 0; i < iterations/10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			cm.StartTurn()
			time.Sleep(1 * time.Millisecond)
			cm.InputComplete()
			time.Sleep(1 * time.Millisecond)
			cm.ProcessingComplete()
			time.Sleep(1 * time.Millisecond)
			cm.ResponseComplete()
			time.Sleep(1 * time.Millisecond)
			cm.ResetWaitingTimer()
		}()
	}

	wg.Wait()

	cm.EndConversation()

	// Should not panic or deadlock
	if cm.IsConversationActive() {
		t.Error("conversation should not be active after concurrent operations")
	}
}

func TestConversationManager_ZeroTimeout(t *testing.T) {
	config := ConversationConfig{
		WaitingTimeout: 0, // No timeout
	}
	cm := NewConversationManager(config)

	cm.StartConversation()
	cm.StartTurn()
	cm.InputComplete()
	cm.ProcessingComplete()
	cm.ResponseComplete()

	// Wait a bit
	time.Sleep(100 * time.Millisecond)

	// Should still be in Waiting state (no timeout)
	if cm.GetState() != ConvWaiting {
		t.Errorf("expected state ConvWaiting with zero timeout, got %s", cm.GetState())
	}

	if !cm.IsConversationActive() {
		t.Error("conversation should still be active with zero timeout")
	}

	cm.EndConversation()
}
