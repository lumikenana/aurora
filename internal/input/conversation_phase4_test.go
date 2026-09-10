package input

import (
	"sync/atomic"
	"testing"
	"time"
)

// ---- Phase 4: Listening Timeout（WakeWord Conversation 专属） ----

// Listening 超时：唤醒后用户一直不说话 → Conversation 结束 → Idle
func TestConversation_ListeningTimeout_EndsConversation(t *testing.T) {
	cm := NewConversationManager(ConversationConfig{
		WaitingTimeout:   time.Second,
		ListeningTimeout: 60 * time.Millisecond,
	})
	convEnds := atomic.Int32{}
	cm.SetCallbacks(nil, nil, nil, nil, func() { convEnds.Add(1) })

	cm.StartConversation()
	cm.StartTurn()

	if cm.GetState() != ConvListening {
		t.Fatalf("expected ConvListening after start, got %s", cm.GetState())
	}

	// 用户一直不说话 → Listening 超时 → Conversation 结束
	waitFor(t, time.Second, func() bool {
		return !cm.IsConversationActive()
	}, "conversation should end on listening timeout")

	if cm.GetState() != ConvIdle {
		t.Errorf("expected ConvIdle after listening timeout, got %s", cm.GetState())
	}
	if got := convEnds.Load(); got != 1 {
		t.Errorf("expected exactly 1 onConvEnd, got %d", got)
	}
}

// Listening 超时重置：interim 活动（ResetListeningTimer）推迟超时
func TestConversation_ListeningTimeout_ResetOnInput(t *testing.T) {
	cm := NewConversationManager(ConversationConfig{
		WaitingTimeout:   time.Second,
		ListeningTimeout: 150 * time.Millisecond,
	})
	cm.SetCallbacks(nil, nil, nil, nil, nil)

	cm.StartConversation()
	cm.StartTurn()

	// t=100ms：用户开始说话（interim）→ 重置 timer（原 timer 将在 t=150ms 触发）
	time.Sleep(100 * time.Millisecond)
	cm.ResetListeningTimer()

	// t=200ms：已超过原触发点，但重置后 timer 到 t=250ms —— 必须仍活跃
	time.Sleep(100 * time.Millisecond)
	if !cm.IsConversationActive() {
		t.Fatal("conversation must stay active — listening timer should have been reset by input")
	}

	// 之后不再有输入 → 超时正常结束
	waitFor(t, time.Second, func() bool {
		return !cm.IsConversationActive()
	}, "conversation should end after input stops")
}

// Listening 超时不影响 Processing：InputComplete 后 timer 停止
func TestConversation_ListeningTimeout_StopsOnInputComplete(t *testing.T) {
	cm := NewConversationManager(ConversationConfig{
		WaitingTimeout:   time.Second,
		ListeningTimeout: 60 * time.Millisecond,
	})
	cm.SetCallbacks(nil, nil, nil, nil, nil)

	cm.StartConversation()
	cm.StartTurn()
	cm.InputComplete() // → Processing，Listening timer 应停止

	// 超过 Listening timeout → Conversation 必须仍活跃（Processing 中）
	time.Sleep(120 * time.Millisecond)
	if !cm.IsConversationActive() {
		t.Error("conversation must not end on listening timeout after input completed")
	}
	if cm.GetState() != ConvProcessing {
		t.Errorf("expected ConvProcessing, got %s", cm.GetState())
	}
}

// Listening 超时与 Waiting 超时互不干扰：Waiting 期间 Listening timer 不生效
func TestConversation_ListeningTimeout_DoesNotAffectWaiting(t *testing.T) {
	cm := NewConversationManager(ConversationConfig{
		WaitingTimeout:   250 * time.Millisecond,
		ListeningTimeout: 60 * time.Millisecond,
	})
	convEnds := atomic.Int32{}
	cm.SetCallbacks(nil, nil, nil, nil, func() { convEnds.Add(1) })

	// 完整一轮 → Waiting
	cm.StartConversation()
	cm.StartTurn()
	cm.InputComplete()
	cm.ProcessingComplete()
	cm.ResponseComplete()

	if cm.GetState() != ConvWaiting {
		t.Fatalf("expected ConvWaiting, got %s", cm.GetState())
	}

	// 超过 Listening timeout（60ms）但未达 Waiting timeout（250ms）→ 必须仍活跃
	time.Sleep(120 * time.Millisecond)
	if !cm.IsConversationActive() {
		t.Error("listening timeout must not fire during Waiting")
	}
	if cm.GetState() != ConvWaiting {
		t.Errorf("expected ConvWaiting, got %s", cm.GetState())
	}

	// Waiting 超时仍正常结束
	waitFor(t, time.Second, func() bool {
		return !cm.IsConversationActive()
	}, "conversation should end on waiting timeout")
	if got := convEnds.Load(); got != 1 {
		t.Errorf("expected exactly 1 onConvEnd, got %d", got)
	}
}

// PTT/Text standalone Turn 不受 Listening 超时影响（经过 ConvListening 但手动控制输入）
func TestConversation_ListeningTimeout_NotForStandaloneTurn(t *testing.T) {
	cm := NewConversationManager(ConversationConfig{
		WaitingTimeout:   time.Second,
		ListeningTimeout: 60 * time.Millisecond,
	})
	convEnds := atomic.Int32{}
	cm.SetCallbacks(nil, nil, nil, nil, func() { convEnds.Add(1) })

	cm.StartStandaloneTurn()
	if cm.GetState() != ConvListening {
		t.Fatalf("expected ConvListening for standalone turn, got %s", cm.GetState())
	}

	// 超过 Listening timeout → standalone Turn 必须不受影响
	time.Sleep(120 * time.Millisecond)
	if !cm.IsStandaloneTurnActive() {
		t.Error("standalone turn must not be ended by listening timeout")
	}
	if cm.GetTurnState() != TurnInput {
		t.Errorf("expected TurnInput, got %s", cm.GetTurnState())
	}
	if got := convEnds.Load(); got != 0 {
		t.Errorf("no onConvEnd should fire for standalone turn, got %d", got)
	}
}

// Stale timer 安全：Interrupt 后旧 Listening timer 不得结束新 Conversation
func TestConversation_ListeningTimeout_StaleTimerAfterInterrupt(t *testing.T) {
	cm := NewConversationManager(ConversationConfig{
		WaitingTimeout:   time.Second,
		ListeningTimeout: 100 * time.Millisecond,
	})
	convEnds := atomic.Int32{}
	cm.SetCallbacks(nil, nil, nil, nil, func() { convEnds.Add(1) })

	// Conversation A 进入 Listening，t=50ms 时被 Interrupt（timer 原定在 t=100ms 触发）
	cm.StartConversation()
	cm.StartTurn()
	time.Sleep(50 * time.Millisecond)
	cm.Interrupt()
	// onConvEnd 异步触发（goroutine），等待其落地
	waitFor(t, time.Second, func() bool { return convEnds.Load() == 1 },
		"interrupt should fire onConvEnd once")

	// 立即开始 Conversation B（新 generation）
	cm.StartConversation()
	cm.StartTurn()

	// t=150ms：旧 timer 触发点已过，B 必须仍活跃
	time.Sleep(100 * time.Millisecond)
	if !cm.IsConversationActive() {
		t.Error("stale listening timer must not end the new conversation")
	}
	if got := convEnds.Load(); got != 1 {
		t.Errorf("stale timer must not fire onConvEnd, got %d", got)
	}

	// B 自己的 Listening 超时（t≈250ms）正常结束
	waitFor(t, time.Second, func() bool { return convEnds.Load() == 2 },
		"new conversation should end on its own listening timeout")
}

// ---- Phase 4: RequestEnd（exit_conversation_mode 自然语言退出） ----

// RequestEnd：当前 Turn 正常完成后 Conversation 才结束，不进入 Waiting
func TestConversation_RequestEnd_EndsAfterTurnCompletes(t *testing.T) {
	cm := NewConversationManager(ConversationConfig{
		WaitingTimeout: 100 * time.Millisecond,
	})
	convEnds := atomic.Int32{}
	cm.SetCallbacks(nil, nil, nil, nil, func() { convEnds.Add(1) })

	cm.StartConversation()
	cm.StartTurn()
	cm.InputComplete() // → Processing（LLM 处理中，Tool 在此时执行）

	if !cm.RequestEnd() {
		t.Fatal("RequestEnd should return true with active conversation")
	}

	// 关键：Tool 调用不立即打断当前 Turn
	if !cm.IsConversationActive() {
		t.Fatal("RequestEnd must not kill the current turn immediately")
	}
	if cm.GetState() != ConvProcessing {
		t.Errorf("turn should still be Processing, got %s", cm.GetState())
	}

	// 当前 Turn 正常完成
	cm.ProcessingComplete()
	cm.ResponseComplete()

	// → Conversation 结束 → Idle（不进入 Waiting）
	if cm.IsConversationActive() {
		t.Error("conversation should end after turn completion")
	}
	if cm.GetState() != ConvIdle {
		t.Errorf("expected ConvIdle, got %s (must not enter Waiting)", cm.GetState())
	}
	// onConvEnd 异步触发（goroutine），等待其落地
	waitFor(t, time.Second, func() bool { return convEnds.Load() == 1 },
		"expected exactly 1 onConvEnd")

	// 无 waiting timer：超过 waiting timeout 无额外事件
	time.Sleep(150 * time.Millisecond)
	if got := convEnds.Load(); got != 1 {
		t.Errorf("no additional onConvEnd should fire, got %d", got)
	}
}

// RequestEnd：无活跃 Conversation 时安全 no-op（含 standalone Turn 场景）
func TestConversation_RequestEnd_NoConversation(t *testing.T) {
	cm := NewConversationManager(ConversationConfig{
		WaitingTimeout: 100 * time.Millisecond,
	})
	convEnds := atomic.Int32{}
	cm.SetCallbacks(nil, nil, nil, nil, func() { convEnds.Add(1) })

	// 完全 Idle
	if cm.RequestEnd() {
		t.Error("RequestEnd should return false with no conversation")
	}

	// PTT/Text standalone Turn 中（LLM 误调用场景）：no-op，不影响 Turn 生命周期
	cm.StartStandaloneTurn()
	cm.InputComplete()
	if cm.RequestEnd() {
		t.Error("RequestEnd should return false during standalone turn")
	}
	if !cm.IsStandaloneTurnActive() {
		t.Error("standalone turn must not be affected by RequestEnd")
	}

	// standalone Turn 正常完成 → Idle
	cm.ProcessingComplete()
	cm.ResponseComplete()
	if cm.GetState() != ConvIdle {
		t.Errorf("expected ConvIdle after standalone turn, got %s", cm.GetState())
	}
	if got := convEnds.Load(); got != 0 {
		t.Errorf("no onConvEnd should fire, got %d", got)
	}
}

// RequestEnd：Waiting 中（无进行中的 Turn）立即结束
func TestConversation_RequestEnd_InWaitingEndsImmediately(t *testing.T) {
	cm := NewConversationManager(ConversationConfig{
		WaitingTimeout: time.Second,
	})
	convEnds := atomic.Int32{}
	cm.SetCallbacks(nil, nil, nil, nil, func() { convEnds.Add(1) })

	// 完整一轮 → Waiting
	cm.StartConversation()
	cm.StartTurn()
	cm.InputComplete()
	cm.ProcessingComplete()
	cm.ResponseComplete()
	if cm.GetState() != ConvWaiting {
		t.Fatalf("expected ConvWaiting, got %s", cm.GetState())
	}

	if !cm.RequestEnd() {
		t.Fatal("RequestEnd should return true in Waiting")
	}
	if cm.IsConversationActive() {
		t.Error("conversation should end immediately in Waiting (no turn to wait for)")
	}
	if cm.GetState() != ConvIdle {
		t.Errorf("expected ConvIdle, got %s", cm.GetState())
	}
	waitFor(t, time.Second, func() bool { return convEnds.Load() == 1 },
		"expected exactly 1 onConvEnd")
}

// RequestEnd：多次调用幂等，最多触发一次 onConvEnd
func TestConversation_RequestEnd_Idempotent(t *testing.T) {
	cm := NewConversationManager(ConversationConfig{
		WaitingTimeout: 100 * time.Millisecond,
	})
	convEnds := atomic.Int32{}
	cm.SetCallbacks(nil, nil, nil, nil, func() { convEnds.Add(1) })

	cm.StartConversation()
	cm.StartTurn()
	cm.InputComplete()

	if !cm.RequestEnd() || !cm.RequestEnd() {
		t.Fatal("RequestEnd should return true while conversation active")
	}

	cm.ProcessingComplete()
	cm.ResponseComplete()

	// Conversation 已结束：后续 RequestEnd 返回 false，onConvEnd 仅一次
	waitFor(t, time.Second, func() bool { return convEnds.Load() == 1 },
		"multiple RequestEnd must produce exactly 1 onConvEnd")
	if cm.RequestEnd() {
		t.Error("RequestEnd after conversation ended should return false")
	}
	time.Sleep(50 * time.Millisecond)
	if got := convEnds.Load(); got != 1 {
		t.Errorf("no additional onConvEnd should fire, got %d", got)
	}
}
