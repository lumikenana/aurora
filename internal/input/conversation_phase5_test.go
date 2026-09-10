package input

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/lipipi/aurora/internal/wakeword"
)

// Phase 5：Conversation 生命周期审计测试。
// 覆盖 spec 场景：
//   - RequestEnd 标记后 ASR 空结果走 CancelTurn → 直接结束（不回 Waiting 等 30s）
//   - PTT standalone Turn 超过 Listening 超时不被取消
//   - Text standalone Turn 超过 Listening 超时不受影响
//   - VC 依赖缺失（无 detector / 无 audio capture / 全 nil）时 Text/PTT 仍安全

// RequestEnd（Turn 输入阶段）→ ASR 空结果 CancelTurn → Conversation 立即结束，
// onConvEnd exactly once，且不进入 Waiting / 不重启 waiting timer。
func TestConversation_CancelTurnConsumesPendingExit(t *testing.T) {
	cm := NewConversationManager(ConversationConfig{
		WaitingTimeout:   100 * time.Millisecond,
		ListeningTimeout: time.Minute, // 本测试不触发 listening 超时
	})

	var convEnds atomic.Int32
	cm.SetCallbacks(nil, nil, nil, nil, func() { convEnds.Add(1) })

	cm.StartConversation()
	cm.StartTurn() // Turn 进入 Input（ASR 流式中）

	// 用户在输入阶段表达退出（exit_conversation_mode → RequestEnd）
	if !cm.RequestEnd() {
		t.Fatal("RequestEnd should return true with active conversation")
	}
	if cm.GetState() != ConvListening {
		t.Fatalf("RequestEnd must not end conversation immediately, state=%s", cm.GetState())
	}

	// ASR 判定语句结束但无文本 → CancelTurn（异常结束路径）
	cm.CancelTurn()

	if cm.IsConversationActive() {
		t.Fatal("conversation must end on CancelTurn with pending exit request")
	}
	if cm.GetState() != ConvIdle {
		t.Fatalf("state must be Idle (not Waiting), got %s", cm.GetState())
	}
	if cm.GetTurnState() != TurnInactive {
		t.Fatalf("turn state must be Inactive after conversation end, got %s", cm.GetTurnState())
	}

	// 等待超过 waiting timeout：不得有第二次 onConvEnd（timer 未重启）
	time.Sleep(250 * time.Millisecond)
	if got := convEnds.Load(); got != 1 {
		t.Fatalf("onConvEnd must fire exactly once, got %d", got)
	}
	if cm.IsConversationActive() || cm.GetState() != ConvIdle {
		t.Fatal("conversation must stay ended (no waiting timer restart)")
	}
}

// 对照组：无 pending exit 时 CancelTurn 保持原语义 —— 回 Waiting 并重启 timer。
func TestConversation_CancelTurnWithoutExitRequestStillWaits(t *testing.T) {
	cm := NewConversationManager(ConversationConfig{
		WaitingTimeout:   time.Minute,
		ListeningTimeout: time.Minute,
	})
	cm.StartConversation()
	cm.StartTurn()
	cm.CancelTurn()

	if !cm.IsConversationActive() {
		t.Fatal("conversation must stay active without exit request")
	}
	if cm.GetState() != ConvWaiting {
		t.Fatalf("state must be Waiting after CancelTurn, got %s", cm.GetState())
	}
}

// spec §2.2：PTT standalone Turn 经过 ConvListening 但不参与 Listening 超时 ——
// 按住 PTT 超过 15s（测试缩放为 80ms）不会自动取消。
func TestVoiceController_PTTTurnIgnoresListeningTimeout(t *testing.T) {
	rec := &fakeRecognizer{}
	ac := &fakeAudioCap{}

	vc := newTestVoiceControllerWithConfig(nil, rec, ac, ConversationConfig{
		WaitingTimeout:   time.Minute,
		ListeningTimeout: 80 * time.Millisecond,
	})
	_ = vc.Start(context.Background())
	defer vc.Stop()

	vc.PTTPreempt()
	vc.StartPTTTurn()

	// 等待远超 listening timeout：turn 必须保持 Input，不被取消
	time.Sleep(250 * time.Millisecond)
	if !vc.convMgr.IsStandaloneTurnActive() {
		t.Fatal("PTT standalone turn must not be cancelled by listening timeout")
	}
	if ts := vc.convMgr.GetTurnState(); ts != TurnInput {
		t.Fatalf("turn must stay in Input (user controls PTT release), got %s", ts)
	}

	// 正常收尾：松开 PTT → Agent → TTS End → Idle
	vc.PTTFinished()
	vc.NotifyAgentComplete()
	vc.NotifyTTSEnd()
	waitFor(t, time.Second, func() bool {
		return !vc.convMgr.IsStandaloneTurnActive() && vc.convMgr.GetState() == ConvIdle
	}, "PTT turn should complete to Idle")
}

// spec §2.2：Text standalone Turn 超过 Listening 超时不受影响 ——
// 文本已就绪（InputComplete 立即完成），Turn 停在 Processing 直到 Agent 完成。
func TestVoiceController_TextTurnIgnoresListeningTimeout(t *testing.T) {
	rec := &fakeRecognizer{}
	ac := &fakeAudioCap{}

	vc := newTestVoiceControllerWithConfig(nil, rec, ac, ConversationConfig{
		WaitingTimeout:   time.Minute,
		ListeningTimeout: 80 * time.Millisecond,
	})
	_ = vc.Start(context.Background())
	defer vc.Stop()

	vc.StartTextTurn()
	if ts := vc.convMgr.GetTurnState(); ts != TurnProcessing {
		t.Fatalf("text turn must complete input immediately, got %s", ts)
	}

	// 等待远超 listening timeout：turn 不受影响
	time.Sleep(250 * time.Millisecond)
	if !vc.convMgr.IsStandaloneTurnActive() {
		t.Fatal("text standalone turn must not be affected by listening timeout")
	}
	if ts := vc.convMgr.GetTurnState(); ts != TurnProcessing {
		t.Fatalf("text turn must stay in Processing until agent completes, got %s", ts)
	}
	if vc.convMgr.IsConversationActive() {
		t.Fatal("text trigger must not create a conversation")
	}

	// 正常收尾：Agent → TTS End → Idle（不进 Conversation / Waiting）
	vc.NotifyAgentComplete()
	vc.NotifyTTSEnd()
	waitFor(t, time.Second, func() bool {
		return !vc.convMgr.IsStandaloneTurnActive() && vc.convMgr.GetState() == ConvIdle
	}, "text turn should complete to Idle")
}

// spec §5：无 WakeWord detector —— WakeWord Trigger 不可用（不启动后台监听），
// PTT / Text Trigger 不受影响。
func TestVoiceController_NoWakeDetector_TextAndPTTWork(t *testing.T) {
	rec := &fakeRecognizer{}
	ac := &fakeAudioCap{}

	vc := newTestVoiceControllerWithConfig(nil, rec, ac, ConversationConfig{
		WaitingTimeout:   time.Minute,
		ListeningTimeout: time.Minute,
	})
	_ = vc.Start(context.Background())
	defer vc.Stop()

	if vc.wakewordRunning {
		t.Fatal("wakeword trigger must not run without a detector")
	}

	// Text / PTT 不受影响
	vc.StartTextTurn()
	if !vc.convMgr.IsStandaloneTurnActive() {
		t.Fatal("text turn must work without wake detector")
	}
	vc.NotifyAgentComplete()
	vc.NotifyTTSEnd()
	waitFor(t, time.Second, func() bool {
		return !vc.convMgr.IsStandaloneTurnActive()
	}, "text turn should complete without wake detector")

	vc.PTTPreempt()
	vc.StartPTTTurn()
	if ts := vc.convMgr.GetTurnState(); ts != TurnInput {
		t.Fatalf("PTT turn must work without wake detector, got %s", ts)
	}
}

// spec §5：无 Audio Capture —— WakeWord Trigger 不可用（启动失败仅记日志），Text 不 crash。
func TestVoiceController_NoAudioCapture_TextSafe(t *testing.T) {
	wd := newFakeWakeDetector()
	rec := &fakeRecognizer{}

	// 直接构造：nil audioCap 必须保持 interface-nil（typed-nil 会绕过 nil 检查）
	vc := NewVoiceController(wd, rec, nil,
		WithConversationConfig(ConversationConfig{
			WaitingTimeout:   time.Minute,
			ListeningTimeout: time.Minute,
		}),
		WithCooldown(0),
	)
	_ = vc.Start(context.Background())
	defer vc.Stop()

	if vc.wakewordRunning {
		t.Fatal("wakeword trigger must not run without audio capture")
	}

	vc.StartTextTurn() // 必须不 panic
	if !vc.convMgr.IsStandaloneTurnActive() {
		t.Fatal("text turn must work without audio capture")
	}
	vc.NotifyAgentComplete()
	vc.NotifyTTSEnd()
	waitFor(t, time.Second, func() bool {
		return !vc.convMgr.IsStandaloneTurnActive() && vc.convMgr.GetState() == ConvIdle
	}, "text turn should complete to Idle without audio capture")
}

// spec §5：全部依赖缺失（无 detector / recognizer / audio capture / TTS）——
// Text 与 PTT 生命周期仍完整走完，不 crash。
func TestVoiceController_AllNilDeps_TextAndPTTSafe(t *testing.T) {
	vc := NewVoiceController(nil, nil, nil, WithCooldown(0))
	_ = vc.Start(context.Background())
	defer vc.Stop()

	// Text turn 完整生命周期
	vc.StartTextTurn()
	vc.SpeakTTS("没有 TTS provider 时立即结束") // no-op 路径：直接 NotifyTTSEnd
	vc.NotifyAgentComplete()
	vc.NotifyTTSEnd()
	waitFor(t, time.Second, func() bool {
		return !vc.convMgr.IsStandaloneTurnActive() && vc.convMgr.GetState() == ConvIdle
	}, "text turn should complete to Idle with all deps nil")

	// PTT turn 完整生命周期
	vc.PTTPreempt()
	vc.StartPTTTurn()
	vc.PTTFinished()
	vc.NotifyAgentComplete()
	vc.NotifyTTSEnd()
	waitFor(t, time.Second, func() bool {
		return !vc.convMgr.IsStandaloneTurnActive() && vc.convMgr.GetState() == ConvIdle
	}, "PTT turn should complete to Idle with all deps nil")
}

// 防回归：PTT 打断进行中 Conversation 后，wake 事件门控在 Turn 期间仍生效，
// Turn 完成后（回 Idle）wake 可再次触发新 Conversation。
func TestVoiceController_WakeWorksAgainAfterPTTInterrupt(t *testing.T) {
	wd := newFakeWakeDetector()
	rec := &fakeRecognizer{}
	ac := &fakeAudioCap{}

	vc := newTestVoiceControllerWithConfig(wd, rec, ac, ConversationConfig{
		WaitingTimeout:   time.Minute,
		ListeningTimeout: time.Minute,
	})
	_ = vc.Start(context.Background())
	defer vc.Stop()

	// Conversation 开始
	wd.events <- wakeword.WakeEvent{Type: wakeword.WakeEventDetected}
	waitFor(t, time.Second, func() bool { return vc.convMgr.IsConversationActive() }, "conversation active")

	// PTT 打断 → Conversation 结束，standalone Turn
	vc.PTTPreempt()
	vc.StartPTTTurn()

	// PTT Turn 期间 wake 被忽略
	wd.events <- wakeword.WakeEvent{Type: wakeword.WakeEventDetected}
	time.Sleep(50 * time.Millisecond)
	if vc.convMgr.IsConversationActive() {
		t.Fatal("wake during PTT turn must be ignored")
	}

	// PTT Turn 完成 → Idle → wake 可触发新 Conversation
	vc.PTTFinished()
	vc.NotifyAgentComplete()
	vc.NotifyTTSEnd()
	waitFor(t, time.Second, func() bool {
		return !vc.convMgr.IsStandaloneTurnActive() && vc.GetState() == StateIdle
	}, "back to idle after PTT turn")

	wd.events <- wakeword.WakeEvent{Type: wakeword.WakeEventDetected}
	waitFor(t, time.Second, func() bool {
		return vc.convMgr.IsConversationActive()
	}, "wake after PTT turn should start a new conversation")
}
