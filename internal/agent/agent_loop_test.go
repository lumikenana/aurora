// Package agent - Agent Loop 通用回归测试（Phase 17-A 重写）
//
// 历史：原 internal/agent/tft_tool_loop_test.go 同时含
//   - TestParseToolCall（通用）
//   - TestExecuteTool_TFTTools（TFT 工具集成）
//   - TestAgentLoop_ToolCallFlow（通用 Agent Loop）
//
// Phase 17-A：tft_tool_loop_test.go 删除；TFT 工具测试已随工具下放
// 到 internal/game/plugins/tft/tools/。本文件保留：
//   - TestParseToolCall（验证 tool call 解析）
//   - TestAgentLoop_ToolCallFlow（验证 Agent Loop 完整性）
// 目的：证明 Agent Loop 本身对游戏无知（不依赖 tft / lol / 任何插件）。
package agent

import (
	"context"
	"encoding/json"
	"sync"
	"testing"

	apperrors "github.com/lipipi/aurora/internal/errors"
	"github.com/lipipi/aurora/internal/llm"
	"github.com/lipipi/aurora/internal/tool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockLLMProvider 简单 LLM provider mock（按调用顺序返回预设回复）
type mockLLMProvider struct {
	mu    sync.Mutex
	queue []string // 每次 ChatCompletion 弹出队首
}

func (m *mockLLMProvider) Name() string              { return "mock" }
func (m *mockLLMProvider) IsConfigured() bool        { return true }
func (m *mockLLMProvider) SupportedModels() []string { return []string{"mock"} }

func (m *mockLLMProvider) ChatCompletion(_ context.Context, _ llm.ChatCompletionRequest) (*llm.ChatCompletionResponse, apperrors.AppError) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.queue) == 0 {
		return &llm.ChatCompletionResponse{Content: ""}, nil
	}
	content := m.queue[0]
	m.queue = m.queue[1:]
	return &llm.ChatCompletionResponse{Content: content}, nil
}

func (m *mockLLMProvider) ChatCompletionStream(_ context.Context, _ llm.ChatCompletionRequest) (<-chan llm.StreamChunk, apperrors.AppError) {
	ch := make(chan llm.StreamChunk, 1)
	ch <- llm.StreamChunk{Content: "stream", Finished: true}
	close(ch)
	return ch, nil
}

// TestParseToolCall 验证 LLM 响应中 ToolCall 标记的解析
func TestParseToolCall(t *testing.T) {
	cases := []struct {
		name      string
		response  string
		wantNil   bool
		wantName  string
		wantIDArg string
	}{
		{
			name:     "no tool call",
			response: "just a plain text answer",
			wantNil:  true,
		},
		{
			name:     "tool call with single arg",
			response: `thinking...<|ToolCall|>[{"name":"get_tft_champion","parameters":{"id":"TFT18_Vayne"}}]<|/ToolCall|>`,
			wantName: "get_tft_champion",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			call := parseToolCall(tc.response)
			if tc.wantNil {
				assert.Nil(t, call)
				return
			}
			require.NotNil(t, call)
			assert.Equal(t, tc.wantName, call.Name)
			if tc.wantIDArg != "" {
				var params map[string]string
				require.NoError(t, json.Unmarshal(call.Parameters, &params))
				assert.Equal(t, tc.wantIDArg, params["id"])
			}
		})
	}
}

// TestAgentLoop_ToolCallFlow 验证 Agent Loop 在 mock LLM 下能正常完成 final answer
//
// Phase 17-A 验收：Agent Loop 本身对游戏无知（不依赖 tft / lol / 任何插件）。
// 这里用 mock LLM 走完整 ChatWithTools，验证它不会因为没有插件就 panic 或卡住。
func TestAgentLoop_ToolCallFlow(t *testing.T) {
	mock := &mockLLMProvider{
		queue: []string{"直接回答：薇恩是 1 费英雄。"},
	}

	prevInstance := agentInstance
	agentInstance = &Agent{
		llmProvider:  mock,
		toolRegistry: &tool.ToolRegistry{},
		enableTools:  true,
	}
	t.Cleanup(func() { agentInstance = prevInstance })
	agentInstance.newConversation()

	final, err := agentInstance.ChatWithTools(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "直接回答：薇恩是 1 费英雄。", final)
}
