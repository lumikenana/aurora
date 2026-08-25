// Package agent Agent Core - 对话管理和 LLM 调度
package agent

import (
	"context"
	"sync"

	"github.com/lipipi/game-ai/internal/config"
	"github.com/lipipi/game-ai/internal/errors"
	"github.com/lipipi/game-ai/internal/input"
	"github.com/lipipi/game-ai/internal/log"
	"github.com/lipipi/game-ai/internal/llm"
	"github.com/lipipi/game-ai/internal/screenshot"

	// 导入 provider 以触发 init 注册
	_ "github.com/lipipi/game-ai/internal/llm/providers/openai"
)

// Message 对话消息
type Message struct {
	Role    llm.Role `json:"role"`
	Content string   `json:"content"`
}

// Conversation 对话
type Conversation struct {
	ID      string    `json:"id"`
	Messages []Message `json:"messages"`
}

// Agent Agent 核心 - 管理对话状态，调度 LLM
type Agent struct {
	llmProvider   llm.LLMProvider
	screenshotProvider screenshot.ScreenshotProvider
	conversations []Conversation
	currentConv   *Conversation
	systemPrompt  string
	mu            sync.RWMutex
}

var (
	agentInstance *Agent
)

// Init 初始化 Agent
func Init(cfg config.Config) errors.AppError {
	log.Debug("Initializing Agent Core...")

	llmProvider, err := llm.CreateProvider(cfg.LLM)
	if err != nil {
		return err
	}

	screenshotProvider, err := screenshot.CreateProvider(cfg.Screenshot)
	if err != nil {
		log.Warnf("Failed to create screenshot provider: %v", err)
		// Screenshot is optional, continue without it
		screenshotProvider = nil
	}

	agentInstance = &Agent{
		llmProvider:        llmProvider,
		screenshotProvider:  screenshotProvider,
		systemPrompt:        defaultSystemPrompt,
	}

	// 创建初始对话
	agentInstance.newConversation()

	// 注册输入回调
	input.Current().RegisterCallback(agentInstance.onUserInput)

	if screenshotProvider != nil && screenshotProvider.IsAvailable() {
		log.Debugf("Agent Core initialized with LLM: %s, Screenshot: %s", llmProvider.Name(), screenshotProvider.Name())
	} else {
		log.Debugf("Agent Core initialized with provider: %s (screenshot not available)", llmProvider.Name())
	}
	return nil
}

// Shutdown 关闭 Agent
func Shutdown() {
	log.Debug("Shutting down Agent Core...")
	agentInstance = nil
}

// Current 获取 Agent 实例
func Current() *Agent {
	return agentInstance
}

// IsReady 检查 Agent 是否就绪
func (a *Agent) IsReady() bool {
	return a.llmProvider != nil && a.llmProvider.IsConfigured()
}

// GetLLMProvider 获取 LLM Provider
func (a *Agent) GetLLMProvider() llm.LLMProvider {
	return a.llmProvider
}

// GetConversation 获取当前对话
func (a *Agent) GetConversation() []Message {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if a.currentConv == nil {
		return nil
	}
	return a.currentConv.Messages
}

// NewConversation 开始新对话
func (a *Agent) NewConversation() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.newConversation()
}

// newConversation 创建新对话（内部，不锁）
func (a *Agent) newConversation() {
	conv := Conversation{
		ID:         "conv-" + log.RandomID(8),
		Messages:   make([]Message, 0),
	}

	// 添加系统提示
	if a.systemPrompt != "" {
		conv.Messages = append(conv.Messages, Message{
			Role:    llm.RoleSystem,
			Content: a.systemPrompt,
		})
	}

	a.conversations = append(a.conversations, conv)
	a.currentConv = &a.conversations[len(a.conversations)-1]

	log.Debugf("New conversation created: %s", a.currentConv.ID)
}

// AddUserMessage 添加用户消息
func (a *Agent) AddUserMessage(content string) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.currentConv == nil {
		a.newConversation()
	}

	a.currentConv.Messages = append(a.currentConv.Messages, Message{
		Role:    llm.RoleUser,
		Content: content,
	})

	log.Debugf("Added user message: %d chars", len(content))
}

// AddAssistantMessage 添加助手消息
func (a *Agent) AddAssistantMessage(content string) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.currentConv == nil {
		return
	}

	a.currentConv.Messages = append(a.currentConv.Messages, Message{
		Role:    llm.RoleAssistant,
		Content: content,
	})

	log.Debugf("Added assistant message: %d chars", len(content))
}

// Chat 发送当前对话到 LLM 并获取回复（非流式）
func (a *Agent) Chat(ctx context.Context) (string, errors.AppError) {
	a.mu.RLock()
	messages := a.convertMessages()
	a.mu.RUnlock()

	req := llm.ChatCompletionRequest{
		Messages:    messages,
		Temperature: 0.7,
		Stream:      false,
	}

	resp, err := a.llmProvider.ChatCompletion(ctx, req)
	if err != nil {
		return "", err
	}

	a.mu.Lock()
	a.currentConv.Messages = append(a.currentConv.Messages, Message{
		Role:    llm.RoleAssistant,
		Content: resp.Content,
	})
	a.mu.Unlock()

	return resp.Content, nil
}

// ChatStream 发送当前对话到 LLM 并获取流式回复
func (a *Agent) ChatStream(ctx context.Context) (<-chan llm.StreamChunk, errors.AppError) {
	a.mu.RLock()
	messages := a.convertMessages()
	a.mu.RUnlock()

	req := llm.ChatCompletionRequest{
		Messages:    messages,
		Temperature: 0.7,
		Stream:      true,
	}

	ch, err := a.llmProvider.ChatCompletionStream(ctx, req)
	if err != nil {
		return nil, err
	}

	return ch, nil
}

// convertMessages 转换为 LLM 请求消息格式
func (a *Agent) convertMessages() []llm.ChatMessage {
	if a.currentConv == nil {
		return nil
	}

	result := make([]llm.ChatMessage, len(a.currentConv.Messages))
	for i, msg := range a.currentConv.Messages {
		result[i] = llm.ChatMessage{
			Role:    msg.Role,
			Content: msg.Content,
		}
	}
	return result
}

// onUserInput 处理来自输入模块的用户输入
func (a *Agent) onUserInput(in input.UserInput) {
	log.Debugf("Agent received user input from %s", in.Source)

	if !a.IsReady() {
		log.Error("Agent not ready: LLM provider not configured")
		// TODO: notify UI about error
		return
	}

	a.AddUserMessage(in.Text)

	ctx := context.Background()
	resp, err := a.Chat(ctx)
	if err != nil {
		log.Errorf("Failed to get LLM response: %v", err)
		// TODO: notify UI about error
		return
	}

	log.Debugf("Agent got response from LLM: %d chars", len(resp))
	// Response will be added to conversation by Chat()
	// TODO: notify UI about completion
}

// ClearHistory 清空对话历史
func (a *Agent) ClearHistory() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.newConversation()
	log.Debug("Conversation history cleared")
}

// CaptureScreenshot 捕获屏幕截图
func (a *Agent) CaptureScreenshot(ctx context.Context, req screenshot.ScreenshotRequest) (*screenshot.Screenshot, errors.AppError) {
	if a.screenshotProvider == nil || !a.screenshotProvider.IsAvailable() {
		return nil, errors.New(errors.CodeScreenshotFailed, "screenshot provider not available")
	}
	return a.screenshotProvider.Capture(ctx, req)
}

// GetScreenshotProvider 获取截图 Provider
func (a *Agent) GetScreenshotProvider() screenshot.ScreenshotProvider {
	return a.screenshotProvider
}

// IsScreenshotAvailable 检查截图是否可用
func (a *Agent) IsScreenshotAvailable() bool {
	return a.screenshotProvider != nil && a.screenshotProvider.IsAvailable()
}

// 默认系统提示词
const defaultSystemPrompt = `你是 GameAI，一个专业的 PC 游戏助手。
你帮助玩家解答游戏相关问题，提供游戏策略、装备搭配、任务攻略等建议。

请遵守以下规则：
1. 回答简洁明了，重点突出
2. 如果不知道答案，直接说不知道，不要编造
3. 只提供游戏攻略和知识，不提供作弊方法
4. 保持友好专业的语气
`;
