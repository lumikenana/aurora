// Package agent - Phase 18-A GameContext 注入链路集成测试
//
// 验证 Phase 17-B 重构时丢失的 GameContext → LLM 注入链路已恢复：
//   - prepareGameOptions 委托 gameContextPreparer.Prepare
//   - chatStreamWithTools 接受 ChatOptions 并把 ExtraSystemPrompt 注入每轮 LLM 请求
//   - chatWithOptions 把 ExtraSystemPrompt 附加到 system 消息，不改对话历史
//   - 完整链路：mock plugin (GameContextRenderer) → Preparer → prepareGameOptions
//     → chatStreamWithTools → LLM 请求的 system 消息包含 ExtraSystemPrompt
//
// 0 真实 LLM 调用；0 真实游戏插件；0 网络访问。
package agent

import (
	"context"
	"sync"
	"testing"

	"github.com/lipipi/aurora/internal/errors"
	"github.com/lipipi/aurora/internal/game"
	"github.com/lipipi/aurora/internal/llm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// capturingLLMProvider mock LLM：记录收到的 ChatCompletionRequest 用于断言
type capturingLLMProvider struct {
	mu       sync.Mutex
	requests []llm.ChatCompletionRequest
	// 按顺序返回的响应队列（Content）
	responses []string
}

func (c *capturingLLMProvider) Name() string       { return "capturing-mock" }
func (c *capturingLLMProvider) IsConfigured() bool { return true }

func (c *capturingLLMProvider) ChatCompletion(_ context.Context, req llm.ChatCompletionRequest) (*llm.ChatCompletionResponse, errors.AppError) {
	c.mu.Lock()
	c.requests = append(c.requests, req)
	idx := len(c.requests) - 1
	c.mu.Unlock()

	content := ""
	if idx < len(c.responses) {
		content = c.responses[idx]
	}
	return &llm.ChatCompletionResponse{Content: content}, nil
}

func (c *capturingLLMProvider) ChatCompletionStream(_ context.Context, _ llm.ChatCompletionRequest) (<-chan llm.StreamChunk, errors.AppError) {
	ch := make(chan llm.StreamChunk, 1)
	ch <- llm.StreamChunk{Content: "stream", Finished: true}
	close(ch)
	return ch, nil
}

func (c *capturingLLMProvider) lastRequest() (llm.ChatCompletionRequest, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.requests) == 0 {
		return llm.ChatCompletionRequest{}, false
	}
	return c.requests[len(c.requests)-1], true
}

func (c *capturingLLMProvider) requestCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.requests)
}

// mockGamePlugin 同时实现 game.GamePlugin + game.GameContextRenderer
type mockGamePlugin struct {
	id      string
	name    string
	summary string // RenderGameContext 返回的 summary
	used    bool   // RenderGameContext 返回的 used
	rendered bool  // 标记 RenderGameContext 是否被调用
}

func (m *mockGamePlugin) ID() string   { return m.id }
func (m *mockGamePlugin) Name() string { return m.name }
func (m *mockGamePlugin) Detect(_ context.Context) (*game.GameContext, error) {
	return &game.GameContext{
		Game:  m.id,
		Name:  m.name,
		Phase: game.PhaseInGame,
	}, nil
}
func (m *mockGamePlugin) SystemPrompt() string { return "" }

// RenderGameContext implements game.GameContextRenderer
func (m *mockGamePlugin) RenderGameContext(_ *game.GameContext, _ string) (string, bool) {
	m.rendered = true
	return m.summary, m.used
}

// newManagerWithPlugin 构造一个只含给定 plugin 的 game.Manager（绕过全局 pluginFactories）
//
// 通过 game.RegisterPlugin 注册临时 plugin + CreateManager 实例化，
// 完成后 t.Cleanup 注销。plugin ID 使用唯一值避免与其他测试冲突。
func newManagerWithPlugin(t *testing.T, plugin game.GamePlugin) *game.Manager {
	t.Helper()
	id := plugin.ID()
	game.RegisterPlugin(id, func(_ game.Config) (game.GamePlugin, errors.AppError) {
		return plugin, nil
	})
	t.Cleanup(func() {
		// 注销（避免影响其他测试）
		game.UnregisterPlugin(id)
	})
	mgr, err := game.CreateManager(game.Config{Enabled: []string{id}})
	require.NoError(t, err)
	return mgr
}

// TestChatWithOptions_ExtraSystemPromptAppended 验证 chatWithOptions 把 ExtraSystemPrompt
// 正确附加到 system 消息，且不改对话历史
func TestChatWithOptions_ExtraSystemPromptAppended(t *testing.T) {
	capLLM := &capturingLLMProvider{responses: []string{"final answer"}}
	a := &Agent{llmProvider: capLLM}
	a.newConversation()
	a.AddUserMessage("帮我看看棋盘")

	// 调用 chatWithOptions 注入 ExtraSystemPrompt
	resp, err := a.chatWithOptions(context.Background(), ChatOptions{
		ExtraSystemPrompt: "## GameContext\n当前金币=100",
	})
	require.NoError(t, err)
	assert.Equal(t, "final answer", resp)

	// 验证 LLM 请求的 system 消息包含附加的 ExtraSystemPrompt
	req, ok := capLLM.lastRequest()
	require.True(t, ok)
	require.NotEmpty(t, req.Messages)
	require.Equal(t, llm.RoleSystem, req.Messages[0].Role)
	assert.Contains(t, req.Messages[0].Content, "## GameContext")
	assert.Contains(t, req.Messages[0].Content, "当前金币=100")
	// 同时保留 base system prompt 内容（来自 newConversation 的 defaultSystemPrompt 如果有的话）
	// 这里 newConversation 没有 system prompt，所以 system 消息可能为空或不存在
	// 我们只断言 ExtraSystemPrompt 被附加

	// 验证对话历史未被修改（chatWithOptions 不写入历史）
	messages := a.GetConversation()
	// 期望：[user: "帮我看看棋盘"]
	// 不包含 assistant 回复（chatWithOptions 不写历史）
	assistantCount := 0
	for _, m := range messages {
		if m.Role == llm.RoleAssistant {
			assistantCount++
		}
	}
	assert.Equal(t, 0, assistantCount, "chatWithOptions must not write assistant reply to history")
}

// TestChatWithOptions_ZeroOpts_NoSystemMutation 验证 opts 为零值时 system 消息不变
func TestChatWithOptions_ZeroOpts_NoSystemMutation(t *testing.T) {
	capLLM := &capturingLLMProvider{responses: []string{"ok"}}
	a := &Agent{llmProvider: capLLM}
	a.newConversation()
	a.AddUserMessage("hello")

	_, err := a.chatWithOptions(context.Background(), ChatOptions{})
	require.NoError(t, err)

	req, ok := capLLM.lastRequest()
	require.True(t, ok)
	// system 消息（如果存在）不应被添加 "\n\n" 后缀
	for _, m := range req.Messages {
		if m.Role == llm.RoleSystem {
			assert.NotContains(t, m.Content, "\n\n\n", "zero opts must not inject extra separator")
		}
	}
}

// TestPrepareGameOptions_WithPluginRenderer 验证 prepareGameOptions 正确委托给
// gameContextPreparer，而 preparer 委托给 plugin 的 RenderGameContext
func TestPrepareGameOptions_WithPluginRenderer(t *testing.T) {
	plugin := &mockGamePlugin{
		id:      "mock-game-test-1",
		name:    "Mock Game",
		summary: "## Mock Game State\nHP=100, Gold=50",
		used:    true,
	}
	mgr := newManagerWithPlugin(t, plugin)

	// 让 manager 持有 lastKnown（UpdateContext 内部调用 DetectActive 并写入 lastKnown）
	gctx := mgr.UpdateContext(context.Background(), game.ContextInput{})
	require.NotNil(t, gctx, "UpdateContext should populate lastKnown via DetectActive")

	preparer := NewGameContextPreparer(mgr)
	a := &Agent{
		gameManager:         mgr,
		gameContextPreparer: preparer,
	}

	opts := a.prepareGameOptions(context.Background(), "帮我看看现在怎么玩")
	assert.Equal(t, "## Mock Game State\nHP=100, Gold=50", opts.ExtraSystemPrompt)
	assert.True(t, plugin.rendered, "plugin RenderGameContext should have been called")
}

// TestPrepareGameOptions_PluginReturnsUnused 验证插件返回 used=false 时不注入
func TestPrepareGameOptions_PluginReturnsUnused(t *testing.T) {
	plugin := &mockGamePlugin{
		id:      "mock-game-test-2",
		name:    "Mock Game",
		summary: "",
		used:    false,
	}
	mgr := newManagerWithPlugin(t, plugin)
	_ = mgr.UpdateContext(context.Background(), game.ContextInput{})

	preparer := NewGameContextPreparer(mgr)
	a := &Agent{
		gameManager:         mgr,
		gameContextPreparer: preparer,
	}

	opts := a.prepareGameOptions(context.Background(), "今天天气怎么样")
	assert.Empty(t, opts.ExtraSystemPrompt)
}

// TestPrepareGameOptions_NoGameContext 验证没有 GameContext 时不注入
func TestPrepareGameOptions_NoGameContext(t *testing.T) {
	plugin := &mockGamePlugin{
		id:      "mock-game-test-3",
		name:    "Mock Game",
		summary: "should not appear",
		used:    true,
	}
	mgr := newManagerWithPlugin(t, plugin)
	// 不调用 DetectActive → lastKnown == nil

	preparer := NewGameContextPreparer(mgr)
	a := &Agent{
		gameManager:         mgr,
		gameContextPreparer: preparer,
	}

	opts := a.prepareGameOptions(context.Background(), "帮我看看棋盘")
	assert.Empty(t, opts.ExtraSystemPrompt, "no GameContext → no injection")
}

// TestPrepareGameOptions_NilPreparer 验证 preparer 为 nil 时不 panic
func TestPrepareGameOptions_NilPreparer(t *testing.T) {
	a := &Agent{gameManager: nil, gameContextPreparer: nil}
	assert.NotPanics(t, func() {
		opts := a.prepareGameOptions(context.Background(), "any question")
		assert.Empty(t, opts.ExtraSystemPrompt)
	})
}

// TestChatStreamWithTools_GameContextInjected 验证完整链路：
// GameContext → prepareGameOptions → chatStreamWithTools → LLM 请求 system 包含 ExtraSystemPrompt
func TestChatStreamWithTools_GameContextInjected(t *testing.T) {
	capLLM := &capturingLLMProvider{
		responses: []string{"基于你的棋盘，建议升 8 人口。"},
	}
	plugin := &mockGamePlugin{
		id:      "mock-game-test-4",
		name:    "Mock Game",
		summary: "## TFT State\nLevel=8, Gold=32",
		used:    true,
	}
	mgr := newManagerWithPlugin(t, plugin)
	_ = mgr.UpdateContext(context.Background(), game.ContextInput{})

	preparer := NewGameContextPreparer(mgr)
	a := &Agent{
		llmProvider:         capLLM,
		gameManager:         mgr,
		gameContextPreparer: preparer,
		systemPrompt:        "You are a helpful game assistant.",
	}
	a.newConversation()
	a.AddUserMessage("帮我看看现在应该怎么升级")

	// 模拟 onUserInput 流程：prepareGameOptions → chatStreamWithTools
	ctx := context.Background()
	opts := a.prepareGameOptions(ctx, "帮我看看现在应该怎么升级")
	a.chatStreamWithTools(ctx, opts)

	// 验证 LLM 收到包含 ExtraSystemPrompt 的请求
	req, ok := capLLM.lastRequest()
	require.True(t, ok)
	require.NotEmpty(t, req.Messages)

	// 找到 system 消息并验证包含 GameContext 摘要
	var systemContent string
	for _, m := range req.Messages {
		if m.Role == llm.RoleSystem {
			systemContent = m.Content
			break
		}
	}
	assert.Contains(t, systemContent, "## TFT State", "LLM request should contain GameContext summary")
	assert.Contains(t, systemContent, "Level=8, Gold=32", "LLM request should contain specific state")
	assert.Contains(t, systemContent, "You are a helpful game assistant.", "base system prompt should be preserved")
}

// TestChatStreamWithTools_ToolLoopPreservesExtraSystemPrompt 验证工具循环期间
// ExtraSystemPrompt 在每一轮 LLM 调用中都被保留
func TestChatStreamWithTools_ToolLoopPreservesExtraSystemPrompt(t *testing.T) {
	capLLM := &capturingLLMProvider{
		responses: []string{
			`先看屏幕<|ToolCall|>[{"name":"__test_screenshot_5","parameters":{}}]<|/ToolCall|>`,
			"基于截图，建议购买 2 星 Vayne。",
		},
	}

	// 注册一个 mock screenshot 工具
	screenshotTool := &mockScreenshotTool{
		name:    "__test_screenshot_5",
		dataURI: "data:image/jpeg;base64,fake",
	}
	registry := newTestRegistryWithTool(t, "screenshot_5", screenshotTool)

	a := &Agent{
		llmProvider:  capLLM,
		toolRegistry: registry,
		enableTools:  true,
	}
	a.newConversation()
	a.AddUserMessage("帮我看看屏幕")

	// 直接带 ExtraSystemPrompt 进入工具循环
	opts := ChatOptions{ExtraSystemPrompt: "## Custom Context\nGold=100"}
	a.chatStreamWithTools(context.Background(), opts)

	// 期望 2 次 LLM 调用：第一次返回 tool call，第二次返回 final
	assert.Equal(t, 2, capLLM.requestCount(), "should have made 2 LLM calls")

	// 验证两次调用的 system 消息都包含 ExtraSystemPrompt
	capLLM.mu.Lock()
	defer capLLM.mu.Unlock()
	for i, req := range capLLM.requests {
		var systemContent string
		for _, m := range req.Messages {
			if m.Role == llm.RoleSystem {
				systemContent = m.Content
				break
			}
		}
		assert.Contains(t, systemContent, "## Custom Context",
			"LLM call %d should contain ExtraSystemPrompt", i)
		assert.Contains(t, systemContent, "Gold=100",
			"LLM call %d should contain GameContext data", i)
	}
}
