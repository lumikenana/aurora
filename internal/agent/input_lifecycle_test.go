package agent

import (
	"testing"

	"github.com/lipipi/aurora/internal/input"
	"github.com/lipipi/aurora/internal/llm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRegisterInputCallback_OnlyLatestAgentReceivesInput 复现 Audit F2 场景：
// SaveLLMConfig 重建 Agent 后再注册回调。修复前 RegisterCallback 是追加语义，
// 旧 Agent 的 onUserInput 残留在 InputManager 中，一条输入被多个 Agent 重复
// 处理（双 LLM 调用 / 双 chatUserMessage / Overlay 重复显示 user message）。
// 修复后 SetCallback 为替换语义 —— 只有最新注册的 Agent 处理输入。
func TestRegisterInputCallback_OnlyLatestAgentReceivesInput(t *testing.T) {
	require.NoError(t, input.Init(), "input manager init (stub listener on darwin)")

	// 两个已就绪的 Agent（mock provider），模拟 Init 注册 + SaveLLMConfig 重建再注册
	oldAgent := &Agent{llmProvider: &mockLLMProvider{}, systemPrompt: ""}
	newAgent := &Agent{llmProvider: &mockLLMProvider{}, systemPrompt: ""}
	oldAgent.newConversation()
	newAgent.newConversation()

	registerInputCallback(oldAgent) // 对应 agent.Init
	registerInputCallback(newAgent) // 对应 SaveLLMConfig 重建后的注册

	input.Current().NotifyInput(input.UserInput{Text: "ping", Source: input.SourceText})

	assert.Equal(t, 0, countUserMessages(oldAgent),
		"old agent must NOT receive input after rebuild")
	assert.Equal(t, 1, countUserMessages(newAgent),
		"exactly the latest agent must receive the input exactly once")
}

// countUserMessages 统计 Agent 当前对话中的 user 消息数。
// onUserInput 处理用户输入时会先 AddUserMessage，可作为"该 Agent 收到了输入"的观察点。
func countUserMessages(a *Agent) int {
	n := 0
	for _, msg := range a.GetConversation() {
		if msg.Role == llm.RoleUser {
			n++
		}
	}
	return n
}
