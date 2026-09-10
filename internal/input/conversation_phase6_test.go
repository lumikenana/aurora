package input

import (
	"context"
	"testing"
	"time"

	"github.com/lipipi/aurora/internal/wakeword"
)

// Phase 6：PTT / WakeWord 是并行 Trigger，不再是互斥 VoiceMode。
// 覆盖 spec 场景：
//   - 两个 Trigger 同时可用（无需任何模式设置）
//   - WakeWord 在 Start 后即可触发 Conversation（无 VoiceMode 依赖）
//   - PTT standalone Turn 期间 WakeWord Trigger 后台监听保持运行，
//     Turn 结束回 Idle 后唤醒立即可用
//   - SetWakeDetector 注入（模型异步下载完成）后立即启动 WakeWord Trigger

// 两个 Trigger 同时可用：Start 后 wake 可直接触发 Conversation；
// Conversation 被 PTT 抢占后回 Idle，wake 又可再次触发 —— 全程无模式切换。
func TestVoiceController_BothTriggersAvailableSimultaneously(t *testing.T) {
	wd := newFakeWakeDetector()
	rec := &fakeRecognizer{}
	ac := &fakeAudioCap{}

	vc := newTestVoiceControllerWithConfig(wd, rec, ac, ConversationConfig{
		WaitingTimeout:   time.Minute,
		ListeningTimeout: time.Minute,
	})
	_ = vc.Start(context.Background())
	defer vc.Stop()

	// WakeWord Trigger 在 Start 后自动运行（无 SetMode / 无 VoiceMode）
	if !vc.wakewordRunning {
		t.Fatal("wakeword trigger should auto-start when detector is configured")
	}

	// Trigger 1: WakeWord → Conversation
	wd.events <- wakeword.WakeEvent{Type: wakeword.WakeEventDetected}
	waitFor(t, time.Second, func() bool { return vc.convMgr.IsConversationActive() },
		"wake should start conversation without any mode setting")

	// Trigger 2: PTT 抢占 → standalone Turn → Idle（不恢复 Conversation）
	vc.PTTPreempt()
	vc.StartPTTTurn()
	if !vc.convMgr.IsStandaloneTurnActive() {
		t.Fatal("PTT trigger should preempt conversation with a standalone turn")
	}
	vc.PTTFinished()
	vc.NotifyAgentComplete()
	vc.NotifyTTSEnd()
	waitFor(t, time.Second, func() bool {
		return !vc.convMgr.IsStandaloneTurnActive() && vc.convMgr.GetState() == ConvIdle
	}, "PTT turn should complete to Idle")

	// PTT Turn 结束后 WakeWord Trigger 仍然可用（后台监听未被 PTT 破坏）
	if !vc.wakewordRunning {
		t.Fatal("wakeword trigger must keep running through a PTT turn")
	}
	wd.events <- wakeword.WakeEvent{Type: wakeword.WakeEventDetected}
	waitFor(t, time.Second, func() bool { return vc.convMgr.IsConversationActive() },
		"wake should start a new conversation after the PTT turn")
}

// 无 detector 时 PTT standalone Turn 完整生命周期 → Idle，不进入 Waiting；
// WakeWord Trigger 不启动，但不阻塞 PTT / Text（无 VoiceMode 依赖）。
func TestVoiceController_PTTStandaloneWithoutDetector(t *testing.T) {
	rec := &fakeRecognizer{}
	ac := &fakeAudioCap{}

	vc := newTestVoiceControllerWithConfig(nil, rec, ac, ConversationConfig{
		WaitingTimeout:   60 * time.Millisecond,
		ListeningTimeout: time.Minute,
	})
	_ = vc.Start(context.Background())
	defer vc.Stop()

	if vc.wakewordRunning {
		t.Fatal("wakeword trigger must not run without a detector")
	}

	vc.PTTPreempt()
	vc.StartPTTTurn()
	vc.PTTFinished()
	vc.NotifyAgentComplete()
	vc.NotifyTTSEnd()
	waitFor(t, time.Second, func() bool {
		return !vc.convMgr.IsStandaloneTurnActive() && vc.convMgr.GetState() == ConvIdle
	}, "PTT turn should complete to Idle")

	// 不进入 Waiting：超过 waiting timeout 无任何变化
	time.Sleep(120 * time.Millisecond)
	if vc.convMgr.IsConversationActive() || vc.convMgr.GetState() != ConvIdle {
		t.Fatal("standalone PTT turn must not enter Waiting or start a conversation")
	}
}

// SetWakeDetector 注入（模型异步下载完成）→ WakeWord Trigger 立即开始后台监听，
// 唤醒可直接触发 Conversation —— 无需重启、无模式切换。
func TestVoiceController_SetWakeDetectorStartsTrigger(t *testing.T) {
	rec := &fakeRecognizer{}
	ac := &fakeAudioCap{}

	vc := newTestVoiceControllerWithConfig(nil, rec, ac, ConversationConfig{
		WaitingTimeout:   time.Minute,
		ListeningTimeout: time.Minute,
	})
	_ = vc.Start(context.Background())
	defer vc.Stop()

	if vc.wakewordRunning {
		t.Fatal("wakeword trigger must not run before detector injection")
	}

	// 下载完成 → 注入 detector → Trigger 自动启动
	wd := newFakeWakeDetector()
	vc.SetWakeDetector(wd)
	if !vc.wakewordRunning {
		t.Fatal("wakeword trigger should start immediately after detector injection")
	}
	if !wd.started.Load() {
		t.Fatal("injected detector should be started")
	}

	wd.events <- wakeword.WakeEvent{Type: wakeword.WakeEventDetected}
	waitFor(t, time.Second, func() bool { return vc.convMgr.IsConversationActive() },
		"wake should start conversation right after detector injection")
}

// PTT 借走并停止共享采集后，PTTFinished 必须恢复 WakeWord Trigger 的音频源
// （真机 bug：PTTManager 与 VC 共享同一 AudioCapture，ptt.StopRecording 里
// audioCap.Stop() 后无人重启采集，detector 永久断粮但 wakewordRunning 仍为 true）
func TestVoiceController_WakewordAudioResumesAfterPTT(t *testing.T) {
	wd := newFakeWakeDetector()
	rec := &fakeRecognizer{}
	ac := &fakeAudioCap{}

	vc := newTestVoiceControllerWithConfig(wd, rec, ac, ConversationConfig{
		WaitingTimeout:   time.Minute,
		ListeningTimeout: time.Minute,
	})
	_ = vc.Start(context.Background())
	defer vc.Stop()

	if !ac.IsRecording() || ac.getStartCount() != 1 {
		t.Fatalf("wakeword trigger should own the capture after Start (recording=%v, starts=%d)",
			ac.IsRecording(), ac.getStartCount())
	}

	// 模拟 PTT 抢占并停止共享采集（ptt.StopRecording 的硬件层效果：
	// Start 顶掉 VC 回调 → Stop 停采集，之后无人重启）
	ac.Stop()

	vc.PTTFinished()

	if !ac.IsRecording() {
		t.Fatal("audio capture should be resumed after PTT")
	}
	if got := ac.getStartCount(); got != 2 {
		t.Errorf("capture should be restarted exactly once after PTT, got %d starts", got)
	}
	if !vc.wakewordRunning {
		t.Error("wakeword trigger must still be running")
	}

	// VC 回调已重新安装：喂帧经 vc.audioCh 被 fake detector 消费（不阻塞即通过）
	ac.feed([]byte{1, 2, 3})

	// 唤醒事件仍能开 Conversation
	wd.events <- wakeword.WakeEvent{Type: wakeword.WakeEventDetected}
	waitFor(t, time.Second, func() bool { return vc.convMgr.IsConversationActive() },
		"wake should start conversation after PTT audio resume")
}

// 无 detector（WakeWord Trigger 不可用）时 PTTFinished 不得触碰共享采集
func TestVoiceController_NoResumeWhenWakewordUnavailable(t *testing.T) {
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

	vc.PTTFinished()

	if got := ac.getStartCount(); got != 0 {
		t.Errorf("PTTFinished must not touch audio capture when wakeword unavailable, got %d starts", got)
	}
	if ac.IsRecording() {
		t.Error("audio capture must not be recording")
	}
}

// PTT 空 ASR 结果：standalone Turn 取消回 Idle；随后的 PTTFinished 幂等无副作用
// （真机 bug：空 final 不提交 UserInput → Agent 不运行 → NotifyAgentComplete
// 不触发 → Turn 永久卡 TurnProcessing）
func TestVoiceController_PTTEmptyResultCancelsTurn(t *testing.T) {
	rec := &fakeRecognizer{}
	ac := &fakeAudioCap{}

	vc := newTestVoiceControllerWithConfig(nil, rec, ac, ConversationConfig{
		WaitingTimeout:   time.Minute,
		ListeningTimeout: time.Minute,
	})
	_ = vc.Start(context.Background())
	defer vc.Stop()

	vc.PTTPreempt()
	vc.StartPTTTurn()
	if vc.convMgr.GetTurnState() != TurnInput {
		t.Fatalf("expected TurnInput, got %s", vc.convMgr.GetTurnState())
	}

	// ASR 空 final（用户按了键但没说出可识别内容）→ 取消 Turn
	vc.PTTEmptyResult()

	if vc.convMgr.IsStandaloneTurnActive() {
		t.Error("standalone turn should be cancelled on empty final")
	}
	if vc.convMgr.GetTurnState() != TurnComplete {
		t.Errorf("expected TurnComplete after cancel, got %s", vc.convMgr.GetTurnState())
	}
	if vc.convMgr.GetState() != ConvIdle {
		t.Errorf("expected ConvIdle after cancel, got %s", vc.convMgr.GetState())
	}
	if vc.GetState() != StateIdle {
		t.Errorf("expected StateIdle, got %s", vc.GetState())
	}

	// 随后的 PTTFinished 必须幂等：不得重复 InputComplete（Turn 不得进入 Processing）
	vc.PTTFinished()
	if vc.convMgr.GetTurnState() != TurnComplete {
		t.Errorf("PTTFinished after empty result must not advance the turn, got %s", vc.convMgr.GetTurnState())
	}
	if vc.convMgr.GetState() != ConvIdle {
		t.Errorf("PTTFinished after empty result must not change conv state, got %s", vc.convMgr.GetState())
	}
}
