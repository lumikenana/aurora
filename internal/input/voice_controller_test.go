package input

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/lipipi/aurora/internal/audio"
	"github.com/lipipi/aurora/internal/speech"
	"github.com/lipipi/aurora/internal/wakeword"
)

// ---- Fakes（仅实现 VoiceController 用到的接口方法） ----

type fakeWakeDetector struct {
	events  chan wakeword.WakeEvent
	started atomic.Bool
	stopped atomic.Bool
}

func newFakeWakeDetector() *fakeWakeDetector {
	return &fakeWakeDetector{events: make(chan wakeword.WakeEvent, 8)}
}

func (f *fakeWakeDetector) Start(ctx context.Context, audioCh <-chan []byte) error {
	f.started.Store(true)
	// 持续消费音频（模拟真实 detector），直到 ctx 取消
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case _, ok := <-audioCh:
				if !ok {
					return
				}
			}
		}
	}()
	return nil
}

func (f *fakeWakeDetector) Stop() error                       { f.stopped.Store(true); return nil }
func (f *fakeWakeDetector) Events() <-chan wakeword.WakeEvent { return f.events }
func (f *fakeWakeDetector) IsConfigured() bool                { return true }
func (f *fakeWakeDetector) Name() string                      { return "fake" }

type fakeRecognizer struct {
	mu           sync.Mutex
	startCount   int
	endCount     int
	cancelCount  int
	lastCallback speech.TranscriptCallback
}

func (f *fakeRecognizer) StartSession(audioCh <-chan []byte, cb speech.TranscriptCallback) error {
	f.mu.Lock()
	f.startCount++
	f.lastCallback = cb
	f.mu.Unlock()
	// 消费音频直到 channel 关闭（真实 ASR 行为）
	go func() {
		for range audioCh {
		}
	}()
	return nil
}

func (f *fakeRecognizer) EndSession() error {
	f.mu.Lock()
	f.endCount++
	f.mu.Unlock()
	return nil
}

func (f *fakeRecognizer) CancelSession() error {
	f.mu.Lock()
	f.cancelCount++
	f.mu.Unlock()
	return nil
}

func (f *fakeRecognizer) IsConfigured() bool { return true }
func (f *fakeRecognizer) Name() string       { return "fake" }

func (f *fakeRecognizer) sessionCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.startCount
}

// emit 模拟 ASR 返回一条识别结果
func (f *fakeRecognizer) emit(text string, isFinal bool) {
	f.mu.Lock()
	cb := f.lastCallback
	f.mu.Unlock()
	if cb != nil {
		cb(speech.Transcript{Text: text, IsFinal: isFinal})
	}
}

type fakeAudioCap struct {
	mu        sync.Mutex
	recording bool
	cb        audio.AudioCallback
}

func (f *fakeAudioCap) Start(cb audio.AudioCallback) error {
	f.mu.Lock()
	f.recording = true
	f.cb = cb
	f.mu.Unlock()
	return nil
}

func (f *fakeAudioCap) Stop() {
	f.mu.Lock()
	f.recording = false
	f.mu.Unlock()
}

func (f *fakeAudioCap) IsRecording() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.recording
}

func (f *fakeAudioCap) Name() string      { return "fake" }
func (f *fakeAudioCap) IsAvailable() bool { return true }

type fakeTTS struct {
	playing atomic.Bool
	stopped atomic.Int32
	endCB   func()
}

func (f *fakeTTS) Synthesize(text string) error {
	f.playing.Store(true)
	return nil
}

func (f *fakeTTS) Stop() {
	f.stopped.Add(1)
	f.playing.Store(false)
	// 与真实 provider 一致：Stop 异步触发 playback end（stale callback 场景）
	if f.endCB != nil {
		go f.endCB()
	}
}

func (f *fakeTTS) IsConfigured() bool { return true }
func (f *fakeTTS) IsPlaying() bool    { return f.playing.Load() }
func (f *fakeTTS) Name() string       { return "fake" }
func (f *fakeTTS) SetOnPlaybackEnd(cb func()) {
	f.endCB = cb
}

// finishPlayback 模拟播放自然结束
func (f *fakeTTS) finishPlayback() {
	f.playing.Store(false)
	if f.endCB != nil {
		f.endCB()
	}
}

// ---- helpers ----

func newTestVoiceController(wd *fakeWakeDetector, rec *fakeRecognizer, ac *fakeAudioCap, timeout time.Duration) *VoiceController {
	// 避免 typed-nil：nil *fakeWakeDetector 不能直接进入 interface
	var detector wakeword.WakeWordDetector
	if wd != nil {
		detector = wd
	}
	vc := NewVoiceController(
		detector, rec, ac,
		WithConversationConfig(ConversationConfig{WaitingTimeout: timeout}),
		WithCooldown(0), // 测试不需要冷却
	)
	return vc
}

func waitFor(t *testing.T, deadline time.Duration, cond func() bool, msg string) {
	t.Helper()
	start := time.Now()
	for time.Since(start) < deadline {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("condition not met within %v: %s", deadline, msg)
}

// ---- Tests ----

// WakeWord → Conversation → Turn → Waiting →（超时）→ Idle
func TestVoiceController_WakeStartsConversation(t *testing.T) {
	wd := newFakeWakeDetector()
	rec := &fakeRecognizer{}
	ac := &fakeAudioCap{}
	tts := &fakeTTS{}

	var finalTexts []string
	var mu sync.Mutex

	vc := newTestVoiceController(wd, rec, ac, 80*time.Millisecond)
	vc.SetTTS(tts)
	vc.convMgr.SetCallbacks(nil, nil, nil,
		func() { log0() },
		func() { vc.handleConversationEnded() },
	)
	_ = vc.Start(context.Background())
	defer vc.Stop()

	if err := vc.SetMode(context.Background(), ModeWakeWord); err != nil {
		t.Fatalf("SetMode(ModeWakeWord) failed: %v", err)
	}
	vc.mu.Lock()
	vc.onFinalResult = func(text string) {
		mu.Lock()
		finalTexts = append(finalTexts, text)
		mu.Unlock()
	}
	vc.mu.Unlock()

	// 触发唤醒
	wd.events <- wakeword.WakeEvent{Type: wakeword.WakeEventDetected, ModelName: "test", Score: 0.9}

	waitFor(t, time.Second, func() bool {
		return vc.convMgr.IsConversationActive() && vc.GetState() == StateSpeechListening
	}, "conversation should start and ASR should be listening")

	// Turn 1：ASR final → Agent → TTS → Waiting
	rec.emit("你好", true)
	waitFor(t, time.Second, func() bool {
		return vc.convMgr.GetTurnState() == TurnProcessing
	}, "turn should enter processing after final transcript")

	vc.NotifyAgentComplete()
	vc.SpeakTTS("回复一")
	waitFor(t, time.Second, func() bool { return vc.GetState() == StateSpeaking }, "should be speaking")
	tts.finishPlayback()

	waitFor(t, time.Second, func() bool {
		return vc.convMgr.GetState() == ConvWaiting
	}, "conversation should enter Waiting after TTS")

	// Waiting 期间 ASR session 已重启（第二次 StartSession）
	if rec.sessionCount() != 2 {
		t.Errorf("expected ASR session restarted for next turn (2 sessions), got %d", rec.sessionCount())
	}

	// 超时 → Conversation 结束 → Idle
	waitFor(t, 2*time.Second, func() bool {
		return !vc.convMgr.IsConversationActive() && vc.GetState() == StateIdle
	}, "conversation should end on waiting timeout")

	mu.Lock()
	if len(finalTexts) != 1 || finalTexts[0] != "你好" {
		t.Errorf("expected one final result 你好, got %v", finalTexts)
	}
	mu.Unlock()
}

// Conversation 多轮：Waiting 中用户继续说话 → 新 Turn（timer 停止）
func TestVoiceController_ConversationMultiTurn(t *testing.T) {
	wd := newFakeWakeDetector()
	rec := &fakeRecognizer{}
	ac := &fakeAudioCap{}
	tts := &fakeTTS{}

	vc := newTestVoiceController(wd, rec, ac, 200*time.Millisecond)
	vc.SetTTS(tts)
	_ = vc.Start(context.Background())
	defer vc.Stop()
	if err := vc.SetMode(context.Background(), ModeWakeWord); err != nil {
		t.Fatalf("SetMode failed: %v", err)
	}

	wd.events <- wakeword.WakeEvent{Type: wakeword.WakeEventDetected}
	waitFor(t, time.Second, func() bool { return vc.convMgr.IsConversationActive() }, "conversation active")

	// Turn 1
	rec.emit("第一个问题", true)
	waitFor(t, time.Second, func() bool { return vc.convMgr.GetTurnState() == TurnProcessing }, "turn1 processing")
	vc.NotifyAgentComplete()
	vc.SpeakTTS("回答一")
	tts.finishPlayback()
	waitFor(t, time.Second, func() bool { return vc.convMgr.GetState() == ConvWaiting }, "waiting")

	// Waiting 中超时前用户开始说话（interim）→ StartTurn 停止 timer
	time.Sleep(100 * time.Millisecond) // 接近但未超过 200ms timeout
	rec.emit("第二个", false)             // interim
	waitFor(t, time.Second, func() bool { return vc.convMgr.GetTurnState() == TurnInput }, "turn2 input")

	// 超过原 timeout 后 Conversation 必须仍然活跃（timer 已停止）
	time.Sleep(250 * time.Millisecond)
	if !vc.convMgr.IsConversationActive() {
		t.Error("conversation must stay active — waiting timer should have stopped on new input")
	}

	// Turn 2 完成
	rec.emit("第二个问题", true)
	waitFor(t, time.Second, func() bool { return vc.convMgr.GetTurnState() == TurnProcessing }, "turn2 processing")
	vc.NotifyAgentComplete()
	vc.SpeakTTS("回答二")
	tts.finishPlayback()
	waitFor(t, time.Second, func() bool { return vc.convMgr.GetState() == ConvWaiting }, "waiting again")
}

// Conversation 期间唤醒词事件必须被忽略（不重复创建 Conversation）
func TestVoiceController_WakeIgnoredDuringConversation(t *testing.T) {
	wd := newFakeWakeDetector()
	rec := &fakeRecognizer{}
	ac := &fakeAudioCap{}

	convStarts := atomic.Int32{}
	vc := newTestVoiceController(wd, rec, ac, time.Second)
	_ = vc.Start(context.Background())
	defer vc.Stop()
	if err := vc.SetMode(context.Background(), ModeWakeWord); err != nil {
		t.Fatalf("SetMode failed: %v", err)
	}
	vc.convMgr.SetCallbacks(nil, nil, nil,
		func() { convStarts.Add(1) },
		func() { vc.handleConversationEnded() },
	)

	wd.events <- wakeword.WakeEvent{Type: wakeword.WakeEventDetected}
	waitFor(t, time.Second, func() bool { return vc.convMgr.IsConversationActive() }, "conversation active")

	// Conversation 进行中再次触发唤醒 —— 必须被忽略
	wd.events <- wakeword.WakeEvent{Type: wakeword.WakeEventDetected}
	time.Sleep(50 * time.Millisecond)

	if got := convStarts.Load(); got != 1 {
		t.Errorf("expected exactly 1 conversation start, got %d", got)
	}
	if rec.sessionCount() != 1 {
		t.Errorf("expected 1 ASR session (no duplicate), got %d", rec.sessionCount())
	}
}

// PTT 打断 Conversation（Responding 中）：停 TTS → standalone Turn → Idle
func TestVoiceController_PTTInterruptsConversation(t *testing.T) {
	wd := newFakeWakeDetector()
	rec := &fakeRecognizer{}
	ac := &fakeAudioCap{}
	tts := &fakeTTS{}

	vc := newTestVoiceController(wd, rec, ac, time.Second)
	vc.SetTTS(tts)
	_ = vc.Start(context.Background())
	defer vc.Stop()
	if err := vc.SetMode(context.Background(), ModeWakeWord); err != nil {
		t.Fatalf("SetMode failed: %v", err)
	}

	// 进入 Responding
	wd.events <- wakeword.WakeEvent{Type: wakeword.WakeEventDetected}
	waitFor(t, time.Second, func() bool { return vc.convMgr.IsConversationActive() }, "conversation active")
	rec.emit("问题", true)
	waitFor(t, time.Second, func() bool { return vc.convMgr.GetTurnState() == TurnProcessing }, "processing")
	vc.NotifyAgentComplete()
	vc.SpeakTTS("很长的回答……")
	waitFor(t, time.Second, func() bool { return vc.GetState() == StateSpeaking }, "speaking")

	// PTT 按下：打断（与 main.go 接线一致：PTTPreempt + StartPTTTurn）
	vc.PTTPreempt()
	vc.StartPTTTurn()

	if vc.convMgr.IsConversationActive() {
		t.Error("conversation should be interrupted by PTT")
	}
	if !vc.convMgr.IsStandaloneTurnActive() {
		t.Error("standalone PTT turn should be active")
	}
	if vc.GetState() != StateIdle {
		t.Errorf("expected StateIdle after preempt, got %s", vc.GetState())
	}
	if tts.stopped.Load() == 0 {
		t.Error("TTS Stop should be called on preempt")
	}

	// provider.Stop() 异步触发的 stale NotifyTTSEnd 不得破坏 standalone turn
	time.Sleep(50 * time.Millisecond)
	if !vc.convMgr.IsStandaloneTurnActive() {
		t.Error("stale NotifyTTSEnd must not clear the standalone turn")
	}
	if vc.convMgr.GetTurnState() != TurnInput {
		t.Errorf("stale NotifyTTSEnd must not advance turn state, got %s", vc.convMgr.GetTurnState())
	}

	// PTT 完成 → Agent → TTS → Turn Complete → Idle（不恢复 Conversation）
	vc.PTTFinished()
	if vc.convMgr.GetTurnState() != TurnProcessing {
		t.Errorf("expected TurnProcessing after PTTFinished, got %s", vc.convMgr.GetTurnState())
	}
	vc.NotifyAgentComplete()
	vc.SpeakTTS("PTT 回复")
	tts.finishPlayback()
	time.Sleep(20 * time.Millisecond)

	if vc.convMgr.IsStandaloneTurnActive() {
		t.Error("standalone turn should complete after TTS")
	}
	if vc.convMgr.IsConversationActive() {
		t.Error("original conversation must NOT be resumed after PTT turn")
	}
	if vc.convMgr.GetState() != ConvIdle {
		t.Errorf("expected ConvIdle after PTT turn, got %s", vc.convMgr.GetState())
	}
}

// PTT 优先：standalone Turn 进行中唤醒词事件被忽略
func TestVoiceController_WakeIgnoredDuringPTTTurn(t *testing.T) {
	wd := newFakeWakeDetector()
	rec := &fakeRecognizer{}
	ac := &fakeAudioCap{}

	vc := newTestVoiceController(wd, rec, ac, time.Second)
	_ = vc.Start(context.Background())
	defer vc.Stop()
	if err := vc.SetMode(context.Background(), ModeWakeWord); err != nil {
		t.Fatalf("SetMode failed: %v", err)
	}

	vc.PTTPreempt()
	vc.StartPTTTurn()

	wd.events <- wakeword.WakeEvent{Type: wakeword.WakeEventDetected}
	time.Sleep(50 * time.Millisecond)

	if vc.convMgr.IsConversationActive() {
		t.Error("conversation must not start during a PTT standalone turn")
	}
	if rec.sessionCount() != 0 {
		t.Errorf("no ASR session should start from wake during PTT turn, got %d", rec.sessionCount())
	}
}

// PTT 单轮（ModePTT）：完整生命周期 → Idle，不进入 Waiting
func TestVoiceController_PTTStandaloneTurnFullCycle(t *testing.T) {
	rec := &fakeRecognizer{}
	ac := &fakeAudioCap{}
	tts := &fakeTTS{}

	vc := newTestVoiceController(nil, rec, ac, 60*time.Millisecond)
	vc.SetTTS(tts)
	_ = vc.Start(context.Background())
	defer vc.Stop()
	// 默认 ModePTT

	vc.PTTPreempt()
	vc.StartPTTTurn()
	if vc.convMgr.GetTurnState() != TurnInput {
		t.Fatalf("expected TurnInput, got %s", vc.convMgr.GetTurnState())
	}

	vc.PTTFinished()
	vc.NotifyAgentStart()
	vc.NotifyAgentComplete()
	if vc.convMgr.GetTurnState() != TurnResponding {
		t.Errorf("expected TurnResponding after agent complete, got %s", vc.convMgr.GetTurnState())
	}

	vc.SpeakTTS("回复")
	tts.finishPlayback()
	time.Sleep(20 * time.Millisecond)

	if vc.convMgr.GetState() != ConvIdle {
		t.Errorf("expected ConvIdle after PTT turn, got %s", vc.convMgr.GetState())
	}

	// 没有 waiting timer：超过 timeout 无任何变化
	time.Sleep(120 * time.Millisecond)
	if vc.convMgr.GetState() != ConvIdle || vc.convMgr.IsConversationActive() {
		t.Error("standalone turn must not enter Waiting or start a conversation")
	}
}

// Conversation 中 ASR 空结果：丢弃 Turn，继续监听，超时仍生效
func TestVoiceController_EmptyFinalInConversation(t *testing.T) {
	wd := newFakeWakeDetector()
	rec := &fakeRecognizer{}
	ac := &fakeAudioCap{}

	vc := newTestVoiceController(wd, rec, ac, 80*time.Millisecond)
	_ = vc.Start(context.Background())
	defer vc.Stop()
	if err := vc.SetMode(context.Background(), ModeWakeWord); err != nil {
		t.Fatalf("SetMode failed: %v", err)
	}

	wd.events <- wakeword.WakeEvent{Type: wakeword.WakeEventDetected}
	waitFor(t, time.Second, func() bool { return vc.convMgr.IsConversationActive() }, "conversation active")

	// 空 final（VAD 触发但没有文本）→ CancelTurn → Waiting
	rec.emit("", true)
	time.Sleep(20 * time.Millisecond)

	if !vc.convMgr.IsConversationActive() {
		t.Error("conversation should stay active after empty final")
	}
	if vc.GetState() == StateProcessing {
		t.Error("must not enter Processing on empty final")
	}

	// timer 重启 → 超时结束
	waitFor(t, 2*time.Second, func() bool {
		return !vc.convMgr.IsConversationActive()
	}, "conversation should end on waiting timeout after empty final")
}

func log0() {}
