package input

import (
	"testing"

	"github.com/lipipi/aurora/internal/speech"
)

// 空 final 必须触发 onEmptyFinal 回调，且不提交 UserInput
// （InputManager 未初始化：若 handleTranscript 走到 Current().NotifyInput
// 会 nil panic，测试自然失败 —— 即"不提交输入"的隐式断言）
func TestPTTManager_EmptyFinalTriggersCallback(t *testing.T) {
	m := &PTTManager{}

	emptyFinalCount := 0
	m.RegisterEmptyFinalCallback(func() { emptyFinalCount++ })

	var forwarded []speech.Transcript
	m.RegisterTranscriptCallback(func(tr speech.Transcript) {
		forwarded = append(forwarded, tr)
	})

	// interim（含空文本）不触发
	m.handleTranscript(speech.Transcript{IsFinal: false, Text: ""})
	if emptyFinalCount != 0 {
		t.Fatalf("interim transcript must not trigger empty-final callback, got %d", emptyFinalCount)
	}

	// 空 final 触发回调一次（用户按了 PTT 但没说出可识别内容）
	m.handleTranscript(speech.Transcript{IsFinal: true, Text: ""})
	if emptyFinalCount != 1 {
		t.Errorf("empty final should trigger callback exactly once, got %d", emptyFinalCount)
	}

	// transcript 转发不受空 final 分支影响（前端显示用）
	if len(forwarded) != 2 {
		t.Errorf("all transcripts should still be forwarded to onTranscript, got %d", len(forwarded))
	}
}
