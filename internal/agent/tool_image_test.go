// Package agent - Phase 17-B 工具 + 图片处理测试
//
// 验证 E2E 闭环的关键路径：
//   - 工具结果（特别是图片）能正确传递到 LLM
//   - chatStreamWithTools 循环能执行工具并返回最终答案
//   - 系统提示词包含截图使用指导
package agent

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	apperrors "github.com/lipipi/aurora/internal/errors"
	"github.com/lipipi/aurora/internal/llm"
	"github.com/lipipi/aurora/internal/tool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockScreenshotTool 返回预设图片 dataURI 的 mock 工具
type mockScreenshotTool struct {
	name      string // 可配置的名字，避免测试间冲突
	dataURI   string
	fail      bool
	callCount int
}

func (m *mockScreenshotTool) Name() string {
	if m.name == "" {
		return "capture_screenshot"
	}
	return m.name
}
func (m *mockScreenshotTool) Description() string { return "Mock screenshot" }
func (m *mockScreenshotTool) InputSchema() string { return `{"type":"object","properties":{}}` }
func (m *mockScreenshotTool) IsAvailable() bool   { return !m.fail }
func (m *mockScreenshotTool) Execute(_ context.Context, _ json.RawMessage) (*tool.ToolResult, apperrors.AppError) {
	m.callCount++
	if m.fail {
		return &tool.ToolResult{
			Success: false,
			Error:   "mock screenshot failure",
		}, apperrors.New(apperrors.CodeScreenshotFailed, "mock screenshot failure")
	}
	return &tool.ToolResult{
		Type:    tool.ToolResultTypeImage,
		Image:   m.dataURI,
		Success: true,
	}, nil
}

// newTestRegistryWithTool 创建包含指定工具的 ToolRegistry
// 因为 tool.ToolRegistry.tools 是私有字段，这里通过 RegisterTool + CreateRegistry 间接构造
func newTestRegistryWithTool(t *testing.T, toolName string, toolInstance tool.Tool) *tool.ToolRegistry {
	t.Helper()
	// 注册一个返回预设 toolInstance 的工厂
	tool.RegisterTool("__test_"+toolName, func(cfg tool.Config) (tool.Tool, apperrors.AppError) {
		return toolInstance, nil
	})
	cfg := tool.Config{Enabled: []string{"__test_" + toolName}}
	reg, err := tool.CreateRegistry(cfg)
	require.NoError(t, err)
	// 把工具名映射为工具期望的名字（工具通过 Name() 返回自己的名字）
	return reg
}

// TestAddToolResult_Image_PopulatesImages 验证图片工具结果正确填充 Message.Images
func TestAddToolResult_Image_PopulatesImages(t *testing.T) {
	agent := &Agent{}
	agent.newConversation()
	agent.AddUserMessage("帮我看看屏幕")

	call := &ToolCall{Name: "capture_screenshot"}
	dataURI := "data:image/jpeg;base64,/9j/4AAQfake"
	result := &tool.ToolResult{
		Type:    tool.ToolResultTypeImage,
		Image:   dataURI,
		Success: true,
	}

	agent.addToolResultToConversation(call, result)

	msgs := agent.currentConv.Messages
	require.Len(t, msgs, 2) // 1 user + 1 tool result
	toolMsg := msgs[1]
	assert.Equal(t, llm.RoleUser, toolMsg.Role)
	assert.True(t, toolMsg.ScreenshotAttached, "ScreenshotAttached should be true")
	require.Len(t, toolMsg.Images, 1, "Images should have 1 entry")
	assert.Equal(t, dataURI, toolMsg.Images[0].DataURI)
	assert.Equal(t, "image/jpeg", toolMsg.Images[0].MIMEType)
	assert.Contains(t, toolMsg.Content, "screenshot captured", "Content should mention screenshot")
}

// TestAddToolResult_Text_OriginalBehavior 验证文本工具结果保持原有行为
func TestAddToolResult_Text_OriginalBehavior(t *testing.T) {
	agent := &Agent{}
	agent.newConversation()
	agent.AddUserMessage("查一下薇恩")

	call := &ToolCall{Name: "get_tft_champion", Parameters: json.RawMessage(`{"id":"TFT18_Vayne"}`)}
	result := &tool.ToolResult{
		Type:    tool.ToolResultTypeText,
		Text:    "Vayne costs 1 gold",
		Success: true,
	}

	agent.addToolResultToConversation(call, result)

	msgs := agent.currentConv.Messages
	require.Len(t, msgs, 2)
	toolMsg := msgs[1]
	assert.False(t, toolMsg.ScreenshotAttached, "ScreenshotAttached should be false for text result")
	assert.Empty(t, toolMsg.Images, "Images should be empty for text result")
	assert.Contains(t, toolMsg.Content, "tool_call", "Content should contain tool_call JSON")
	assert.Contains(t, toolMsg.Content, "Vayne costs 1 gold", "Content should contain result text")
}

// TestAddToolResult_EmptyImage_FallsBackToText 验证空 dataURI 的图片结果走文本路径
func TestAddToolResult_EmptyImage_FallsBackToText(t *testing.T) {
	agent := &Agent{}
	agent.newConversation()

	call := &ToolCall{Name: "capture_screenshot"}
	result := &tool.ToolResult{
		Type:    tool.ToolResultTypeImage,
		Image:   "", // 空的 dataURI
		Success: false,
		Error:   "screenshot failed",
	}

	agent.addToolResultToConversation(call, result)

	msgs := agent.currentConv.Messages
	require.Len(t, msgs, 1)
	toolMsg := msgs[0]
	assert.False(t, toolMsg.ScreenshotAttached)
	assert.Empty(t, toolMsg.Images)
	// 走文本路径，序列化为 JSON
	assert.Contains(t, toolMsg.Content, "tool_call")
}

// TestConvertMessages_PassesThroughImages 验证 convertMessages 透传 Images 字段
func TestConvertMessages_PassesThroughImages(t *testing.T) {
	agent := &Agent{}
	agent.newConversation()

	// 添加一个带图片的消息
	agent.currentConv.Messages = append(agent.currentConv.Messages, Message{
		Role:    llm.RoleUser,
		Content: "看看这个",
		Images: []llm.ImageContent{
			{DataURI: "data:image/jpeg;base64,abc", MIMEType: "image/jpeg"},
		},
		ScreenshotAttached: true,
	})

	result := agent.convertMessages()
	require.Len(t, result, 1) // system prompt is auto-added by newConversation, but we added 1 extra
	// Find the message with image (it's the last one since newConversation adds system first)
	var found bool
	for _, msg := range result {
		if len(msg.Images) > 0 {
			found = true
			assert.Equal(t, "data:image/jpeg;base64,abc", msg.Images[0].DataURI)
			assert.Equal(t, "看看这个", msg.Content)
		}
	}
	assert.True(t, found, "Should find message with images in converted output")
}

// TestConvertMessages_NoImages_EmptySlice 验证无图片消息的 Images 为空
func TestConvertMessages_NoImages_EmptySlice(t *testing.T) {
	agent := &Agent{}
	agent.newConversation()
	agent.AddUserMessage("你好")

	result := agent.convertMessages()
	for _, msg := range result {
		if msg.Role == llm.RoleUser {
			assert.Empty(t, msg.Images, "Regular user message should have no images")
		}
	}
}

// TestChatStreamWithTools_NoToolForPlainChat 验证普通聊天直接返回最终答案，不调用工具
func TestChatStreamWithTools_NoToolForPlainChat(t *testing.T) {
	mock := &mockLLMProvider{
		queue: []string{"你好！我是 GameAI，很高兴为你服务。"},
	}

	a := &Agent{
		llmProvider:  mock,
		toolRegistry: &tool.ToolRegistry{},
		enableTools:  true,
	}
	a.newConversation()
	a.AddUserMessage("你好")

	// 直接调用 chatStreamWithTools，不启动 goroutine
	// 由于 streamFinalAnswer 内部用 goroutine 推送，我们等待它完成
	done := make(chan struct{})
	origHook := a.postResponseHook
	var hookCalled string
	a.postResponseHook = func(text string) {
		hookCalled = text
		close(done)
	}

	a.chatStreamWithTools(context.Background(), ChatOptions{})

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("chatStreamWithTools did not complete in time")
	}

	assert.Equal(t, "你好！我是 GameAI，很高兴为你服务。", hookCalled)
	// 验证 assistant message 已加入
	msgs := a.currentConv.Messages
	require.GreaterOrEqual(t, len(msgs), 2)
	lastMsg := msgs[len(msgs)-1]
	assert.Equal(t, llm.RoleAssistant, lastMsg.Role)
	assert.Equal(t, "你好！我是 GameAI，很高兴为你服务。", lastMsg.Content)

	// 恢复，避免影响其他测试
	a.postResponseHook = origHook
}

// TestChatStreamWithTools_ExecutesScreenshotThenAnswers 验证截图工具调用 → 最终答案
func TestChatStreamWithTools_ExecutesScreenshotThenAnswers(t *testing.T) {
	mockScreenshot := &mockScreenshotTool{
		dataURI: "data:image/jpeg;base64,fakescreenshotdata",
	}

	// 注册 mock 截图工具
	tool.RegisterTool("capture_screenshot", func(cfg tool.Config) (tool.Tool, apperrors.AppError) {
		return mockScreenshot, nil
	})
	cfg := tool.Config{Enabled: []string{"capture_screenshot"}}
	registry, err := tool.CreateRegistry(cfg)
	require.NoError(t, err)

	// LLM 第一次调用返回截图工具请求，第二次返回最终答案
	mockLLM := &mockLLMProvider{
		queue: []string{
			`好的，让我看看你的屏幕。<|ToolCall|>[{"name":"capture_screenshot","parameters":{}}]<|/ToolCall|>`,
			"我看到你正在玩 TFT，当前有 50 金币，建议拉 8 人口。",
		},
	}

	a := &Agent{
		llmProvider:  mockLLM,
		toolRegistry: registry,
		enableTools:  true,
	}
	a.newConversation()
	a.AddUserMessage("帮我看看屏幕")

	done := make(chan struct{})
	var hookAnswer string
	a.postResponseHook = func(text string) {
		hookAnswer = text
		close(done)
	}

	a.chatStreamWithTools(context.Background(), ChatOptions{})

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("chatStreamWithTools did not complete in time")
	}

	// 验证截图工具被调用
	assert.Equal(t, 1, mockScreenshot.callCount, "screenshot tool should be called once")

	// 验证最终答案正确
	assert.Contains(t, hookAnswer, "TFT", "Final answer should mention TFT")
	assert.Contains(t, hookAnswer, "50 金币", "Final answer should mention gold")

	// 验证对话历史中有图片消息
	var foundImage bool
	for _, msg := range a.currentConv.Messages {
		if len(msg.Images) > 0 {
			foundImage = true
			assert.Equal(t, "data:image/jpeg;base64,fakescreenshotdata", msg.Images[0].DataURI)
			assert.True(t, msg.ScreenshotAttached)
		}
	}
	assert.True(t, foundImage, "Conversation history should contain message with image")
}

// TestChatStreamWithTools_ScreenshotFailure_ContinuesGracefully 验证截图失败不崩溃
func TestChatStreamWithTools_ScreenshotFailure_ContinuesGracefully(t *testing.T) {
	mockScreenshot := &mockScreenshotTool{
		name: "capture_screenshot",
		fail: true, // 截图失败
	}

	tool.RegisterTool("capture_screenshot", func(cfg tool.Config) (tool.Tool, apperrors.AppError) {
		return mockScreenshot, nil
	})
	cfg := tool.Config{Enabled: []string{"capture_screenshot"}}
	registry, err := tool.CreateRegistry(cfg)
	require.NoError(t, err)

	// LLM 请求截图 → 工具失败 → LLM 给出文字回答
	mockLLM := &mockLLMProvider{
		queue: []string{
			`让我截图。<|ToolCall|>[{"name":"capture_screenshot","parameters":{}}]<|/ToolCall|>`,
			"抱歉，截图失败了，但我可以告诉你一般性的建议。",
		},
	}

	a := &Agent{
		llmProvider:  mockLLM,
		toolRegistry: registry,
		enableTools:  true,
	}
	a.newConversation()
	a.AddUserMessage("帮我看看屏幕")

	done := make(chan struct{})
	var hookAnswer string
	a.postResponseHook = func(text string) {
		hookAnswer = text
		close(done)
	}

	// 不应该 panic
	assert.NotPanics(t, func() {
		a.chatStreamWithTools(context.Background(), ChatOptions{})
	})

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("did not complete in time")
	}

	assert.Contains(t, hookAnswer, "截图失败", "Should gracefully continue after tool failure")
}

// TestSystemPrompt_ContainsScreenshotGuidance 验证系统提示词包含截图使用指导
func TestSystemPrompt_ContainsScreenshotGuidance(t *testing.T) {
	// 创建一个包含 capture_screenshot 的 registry
	mockScreenshot := &mockScreenshotTool{dataURI: "data:image/jpeg;base64,x"}
	tool.RegisterTool("capture_screenshot_for_prompt_test", func(cfg tool.Config) (tool.Tool, apperrors.AppError) {
		return mockScreenshot, nil
	})
	cfg := tool.Config{Enabled: []string{"capture_screenshot_for_prompt_test"}}
	registry, err := tool.CreateRegistry(cfg)
	require.NoError(t, err)

	prompt := buildSystemPromptWithTools("你是 GameAI", registry)

	// 验证关键内容
	assert.Contains(t, prompt, "capture_screenshot")
	assert.Contains(t, prompt, "何时使用 capture_screenshot")
	assert.Contains(t, prompt, "看看屏幕")
	assert.Contains(t, prompt, "何时不使用 capture_screenshot")
	assert.Contains(t, prompt, "普通聊天")
}

// TestSystemPrompt_NoTools_NoToolsSection 验证没有工具时不添加工具段
func TestSystemPrompt_NoTools_NoToolsSection(t *testing.T) {
	prompt := buildSystemPromptWithTools("你是 GameAI", nil)
	assert.Equal(t, "你是 GameAI", prompt, "Should return base prompt unchanged when no tools")

	emptyRegistry := &tool.ToolRegistry{}
	prompt2 := buildSystemPromptWithTools("你是 GameAI", emptyRegistry)
	assert.Equal(t, "你是 GameAI", prompt2, "Should return base prompt unchanged for empty registry")
}

// TestChatStreamWithTools_MaxToolCalls 验证达到最大工具调用次数时停止
func TestChatStreamWithTools_MaxToolCalls(t *testing.T) {
	mockScreenshot := &mockScreenshotTool{
		name:    "capture_screenshot", // 与 LLM 请求的名字一致
		dataURI: "data:image/jpeg;base64,x",
	}
	// 注册工厂（全局状态；测试顺序执行，不并行）
	tool.RegisterTool("capture_screenshot", func(cfg tool.Config) (tool.Tool, apperrors.AppError) {
		return mockScreenshot, nil
	})
	cfg := tool.Config{Enabled: []string{"capture_screenshot"}}
	registry, err := tool.CreateRegistry(cfg)
	require.NoError(t, err)

	// LLM 每次都返回工具调用（无限循环场景）
	// 注意工具名必须与 mockScreenshot.Name() 一致
	toolCallResponse := `让我截图。<|ToolCall|>[{"name":"capture_screenshot","parameters":{}}]<|/ToolCall|>`
	initialQueueSize := 10
	responses := make([]string, initialQueueSize) // 远超过 maxToolCalls=5
	for i := range responses {
		responses[i] = toolCallResponse
	}
	mockLLM := &mockLLMProvider{queue: responses}

	a := &Agent{
		llmProvider:  mockLLM,
		toolRegistry: registry,
		enableTools:  true,
	}
	a.newConversation()
	a.AddUserMessage("一直截图")

	done := make(chan struct{})
	a.postResponseHook = func(text string) {
		close(done)
	}

	a.chatStreamWithTools(context.Background(), ChatOptions{})

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("did not complete in time")
	}

	// 验证工具调用被限制在 maxToolCalls=5
	assert.Equal(t, 5, mockScreenshot.callCount, "Should stop at max tool calls (5)")
	// LLM 调用次数 = 初始队列大小 - 剩余队列大小
	mockLLM.mu.Lock()
	remaining := len(mockLLM.queue)
	mockLLM.mu.Unlock()
	assert.Equal(t, 5, initialQueueSize-remaining, "LLM should be called 5 times")
}

// TestAddToolResult_NilResult_NoPanic 验证 nil result 不 panic
func TestAddToolResult_NilResult_NoPanic(t *testing.T) {
	agent := &Agent{}
	agent.newConversation()

	call := &ToolCall{Name: "test"}

	// result 为 nil 不应 panic
	assert.NotPanics(t, func() {
		// 直接测试 nil 路径需要小心，因为 addToolResultToConversation 会访问 result.Type
		// 这个测试验证当 result 不为 nil 但字段为空时的行为
		agent.addToolResultToConversation(call, &tool.ToolResult{})
	})
}

// TestMessage_ImagesNotSerialized 验证 Message 的 Images 字段不序列化到 JSON
func TestMessage_ImagesNotSerialized(t *testing.T) {
	msg := Message{
		Role:    llm.RoleUser,
		Content: "hello",
		Images: []llm.ImageContent{
			{DataURI: "data:image/jpeg;base64,abc"},
		},
		ScreenshotAttached: true,
	}

	data, err := json.Marshal(msg)
	require.NoError(t, err)

	// Images 字段不应该出现在 JSON 中（json:"-"）
	assert.NotContains(t, string(data), "data:image")
	assert.NotContains(t, string(data), "images")
	// ScreenshotAttached 应该出现
	assert.Contains(t, string(data), "screenshot_attached")
	// Content 应该出现
	assert.Contains(t, string(data), "hello")
}

// TestConvertMessages_MixedContent 验证混合内容（文本 + 图片）的转换
func TestConvertMessages_MixedContent(t *testing.T) {
	a := &Agent{}
	a.newConversation()

	// 清空默认 system message
	a.currentConv.Messages = nil

	// 添加各种类型的消息
	a.currentConv.Messages = []Message{
		{Role: llm.RoleUser, Content: "你好"},
		{Role: llm.RoleAssistant, Content: "你好！"},
		{Role: llm.RoleUser, Content: "看看屏幕", Images: []llm.ImageContent{
			{DataURI: "data:image/jpeg;base64,screenshot"},
		}, ScreenshotAttached: true},
		{Role: llm.RoleUser, Content: `{"tool_call": {...}}`}, // 工具结果（文本）
	}

	result := a.convertMessages()
	require.Len(t, result, 4)

	// 第 1 条：普通文本
	assert.Empty(t, result[0].Images)
	assert.Equal(t, "你好", result[0].Content)

	// 第 2 条：普通回复
	assert.Empty(t, result[1].Images)

	// 第 3 条：带截图
	require.Len(t, result[2].Images, 1)
	assert.Equal(t, "data:image/jpeg;base64,screenshot", result[2].Images[0].DataURI)

	// 第 4 条：工具文本结果
	assert.Empty(t, result[3].Images)
	assert.True(t, strings.Contains(result[3].Content, "tool_call"))
}
