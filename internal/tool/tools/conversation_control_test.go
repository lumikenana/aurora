package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

type fakeExitRequester struct {
	called int
	result bool
}

func (f *fakeExitRequester) RequestEnd() bool {
	f.called++
	return f.result
}

// 未注入 requester（VoiceController 未创建）：安全 no-op，不报错
func TestExitConversationTool_NoRequester(t *testing.T) {
	tool := &ExitConversationTool{}

	result, err := tool.Execute(context.Background(), json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("Execute should not fail without requester: %v", err)
	}
	if !result.Success {
		t.Error("result should be success (graceful no-op)")
	}
	if !strings.Contains(result.Text, "没有进行中的连续对话") {
		t.Errorf("unexpected result text: %q", result.Text)
	}
}

// 无活跃 Conversation（含 PTT / 文本单轮场景）：RequestEnd 返回 false → 安全 no-op
func TestExitConversationTool_NoActiveConversation(t *testing.T) {
	req := &fakeExitRequester{result: false}
	tool := &ExitConversationTool{}
	tool.SetConversationExitRequester(req)

	result, err := tool.Execute(context.Background(), json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("Execute should not fail: %v", err)
	}
	if req.called != 1 {
		t.Errorf("RequestEnd should be called once, got %d", req.called)
	}
	if !result.Success {
		t.Error("result should be success (graceful no-op)")
	}
}

// Conversation 活跃：转发 RequestEnd 并告知将在本轮完成后结束
func TestExitConversationTool_RequestsEnd(t *testing.T) {
	req := &fakeExitRequester{result: true}
	tool := &ExitConversationTool{}
	tool.SetConversationExitRequester(req)

	result, err := tool.Execute(context.Background(), json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("Execute should not fail: %v", err)
	}
	if req.called != 1 {
		t.Errorf("RequestEnd should be called once, got %d", req.called)
	}
	if !result.Success {
		t.Error("result should be success")
	}
	if !strings.Contains(result.Text, "本轮回复完成后") {
		t.Errorf("result should explain deferred end, got %q", result.Text)
	}
}

// IsAvailable 始终为 true（requester 延迟注入，不可用过滤会导致工具永久丢失）
func TestExitConversationTool_AlwaysAvailable(t *testing.T) {
	tool := &ExitConversationTool{}
	if !tool.IsAvailable() {
		t.Error("tool must always be available (requester is injected lazily)")
	}
}
