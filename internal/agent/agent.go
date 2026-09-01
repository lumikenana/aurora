// Package agent Agent Core - 对话管理和 LLM 调度
//
// Phase 17-A：Agent Core 不再 import 任何 internal/game/plugins/* 子包。
//   - gameManager 上的插件通过 game.GameContextRenderer / game.ToolContributor
//     / game.SetupReceiver 等可选接口做协作
//   - 知识库以 any 形式存放在 knowledgeProvider，注入插件时由插件自行
//     type-assert 到自己需要的接口（保持 #2.2 边界）
//   - GameContextPreparer 简化为 ~30 行包装，委托给插件的 Renderer
package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/lipipi/aurora/internal/config"
	"github.com/lipipi/aurora/internal/errors"
	"github.com/lipipi/aurora/internal/game"
	"github.com/lipipi/aurora/internal/input"
	"github.com/lipipi/aurora/internal/log"
	"github.com/lipipi/aurora/internal/llm"
	"github.com/lipipi/aurora/internal/screenshot"
	"github.com/lipipi/aurora/internal/tool"
	tools "github.com/lipipi/aurora/internal/tool/tools"
)

// Message 对话消息
type Message struct {
	Role    llm.Role `json:"role"`
	Content string   `json:"content"`
	// ScreenshotAttached true 表示本轮附带截图（追问轮重新截图，图片不写入历史）
	// Phase 12 引入；游戏检测路径使用。Phase 17-B 工具调用也使用此字段。
	ScreenshotAttached bool `json:"screenshot_attached,omitempty"`
	// Images 多模态图片输入（Phase 17-B）。
	// 工具调用（如 capture_screenshot）返回图片时填充此字段；
	// convertMessages 会透传到 llm.ChatMessage.Images。
	// json:"-" 避免序列化到对话历史 JSON（图片数据太大）。
	Images []llm.ImageContent `json:"-"`
}

// ChatOptions 单次 LLM 请求的附加选项
//
// 不同于 llm.ChatCompletionRequest 本身；这是 Agent 层累积的"额外上下文"。
// onUserInput → ChatStream 时把 ExtraSystemPrompt 拼到单次 system 副本，图片
// 挂到最后一条 user 消息副本。不影响对话历史。
type ChatOptions struct {
	// ExtraSystemPrompt 拼接到 system prompt 尾部（单次副本）
	ExtraSystemPrompt string
	// ImageDataURIs 截图 data URI 列表（追问轮重新截新图）
	ImageDataURIs []string
}

// Conversation 对话
type Conversation struct {
	ID      string    `json:"id"`
	Messages []Message `json:"messages"`
}

// PostResponseHook 响应完成后的钩子（用于触发 TTS 等）
type PostResponseHook func(text string)

// Agent Agent 核心 - 管理对话状态，调度 LLM
type Agent struct {
	llmProvider        llm.LLMProvider
	screenshotProvider  screenshot.ScreenshotProvider
	toolRegistry       *tool.ToolRegistry
	conversations       []Conversation
	currentConv         *Conversation
	systemPrompt        string
	enableTools         bool
	mu                  sync.RWMutex
	postResponseHook    PostResponseHook // 流式响应完成时调用，传入完整文本

	// Phase 12 起引入的 game-side wiring：
	// gameManager → 所有游戏插件注册中心；GetGameContext() 走它
	// knowledgeProvider → 上游 KB（Phase 17-A 改为 any；具体接口由插件 type-assert）
	// gameContextPreparer → 薄包装，委托给实现 game.GameContextRenderer 的插件
	// screenshotOnDetect → 触发截图的开关（PT 等游戏检测路径）
	gameManager         *game.Manager
	screenshotOnDetect  bool
	knowledgeProvider   any
	gameContextPreparer *GameContextPreparer
}

var (
	agentInstance *Agent
)

// Init 初始化 Agent
func Init(cfg config.Config) errors.AppError {
	log.Debug("Initializing Agent Core...")

	llmProvider, err := llm.CreateProvider(cfg.LLM)
	if err != nil {
		// LLM provider creation failed (e.g. missing API Key on first launch)
		// Still create the agent so SaveLLMConfig can hot-reload later
		log.Warnf("LLM provider not available: %v (agent will initialize without it)", err)
		llmProvider = nil
	}

	screenshotProvider, err := screenshot.CreateProvider(cfg.Screenshot)
	if err != nil {
		log.Warnf("Failed to create screenshot provider: %v", err)
		// Screenshot is optional, continue without it
		screenshotProvider = nil
	}

	// Create tool registry
	toolRegistry, err := tool.CreateRegistry(cfg.Tool)
	if err != nil {
		log.Warnf("Failed to create tool registry: %v", err)
		toolRegistry = nil
	}

	// Inject screenshot provider into ScreenshotTool if available
	if screenshotProvider != nil && toolRegistry != nil {
		if screenshotTool, exists := toolRegistry.GetTool("capture_screenshot"); exists {
			if st, ok := screenshotTool.(*tools.ScreenshotTool); ok {
				st.SetScreenshotProvider(screenshotProvider)
			}
		}
	}

	// Build system prompt with tool definitions if tools are available
	systemPrompt := buildSystemPromptWithTools(defaultSystemPrompt, toolRegistry)

	// Create game manager（Phase 12 引入；TFT plugin 在 main.go blank import）
	gameManager, gErr := game.CreateManager(cfg.Game)
	if gErr != nil {
		log.Warnf("Failed to create game manager: %v", gErr)
		gameManager = nil
	}

	agentInstance = &Agent{
		llmProvider:        llmProvider,
		screenshotProvider:  screenshotProvider,
		toolRegistry:       toolRegistry,
		enableTools:        toolRegistry != nil && toolRegistry.Count() > 0,
		systemPrompt:       systemPrompt,
		gameManager:        gameManager,
		screenshotOnDetect: cfg.Game.ScreenshotOnDetect,
	}

	// Phase 17-A：通用插件 wiring
	//   - 每个实现 game.SetupReceiver 的插件接收 GameState / Knowledge 依赖
	//   - 每个实现 game.ToolContributor 的插件把自己的工具挂到 registry
	//   - 不再针对 "tft" / 具体插件名做特殊处理
	if gameManager != nil {
		agentInstance.wirePlugins()
	}

	// 构造 GameContextPreparer（薄包装，委托给插件的 GameContextRenderer）
	agentInstance.gameContextPreparer = NewGameContextPreparer(gameManager)

	// 创建初始对话
	agentInstance.newConversation()

	// 注册输入回调
	input.Current().RegisterCallback(agentInstance.onUserInput)

	// Log initialization summary
	providerName := "none"
	if llmProvider != nil {
		providerName = llmProvider.Name()
	}
	if toolRegistry != nil && toolRegistry.Count() > 0 {
		log.Debugf("Agent Core initialized with LLM: %s, %d tools enabled", providerName, toolRegistry.Count())
	} else if screenshotProvider != nil && screenshotProvider.IsAvailable() {
		log.Debugf("Agent Core initialized with LLM: %s, Screenshot: %s, no tools enabled", providerName, screenshotProvider.Name())
	} else {
		log.Debugf("Agent Core initialized with provider: %s (screenshot not available, no tools enabled)", providerName)
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
// Phase 17-B：透传 Images 字段（工具调用返回的截图等多模态内容）
func (a *Agent) convertMessages() []llm.ChatMessage {
	if a.currentConv == nil {
		return nil
	}

	result := make([]llm.ChatMessage, len(a.currentConv.Messages))
	for i, msg := range a.currentConv.Messages {
		result[i] = llm.ChatMessage{
			Role:    msg.Role,
			Content: msg.Content,
			Images:  msg.Images, // Phase 17-B: 透传图片数据（截图等）
		}
	}
	return result
}

// onUserInput 处理来自输入模块的用户输入（文本 + 语音都走这里）
// Phase 17-B：改用 chatStreamWithTools 支持工具调用（截图等），打通 E2E 闭环
func (a *Agent) onUserInput(in input.UserInput) {
	log.Debugf("Agent received user input from %s: %s", in.Source, in.Text)

	if !a.IsReady() {
		log.Error("Agent not ready: LLM provider not configured")
		emitEvent("llmChunk", "[错误] LLM 未配置，请先在设置中配置 API Key")
		emitEvent("llmFinish", nil)
		return
	}

	a.AddUserMessage(in.Text)

	ctx := context.Background()

	// Phase 17-B：使用工具循环（支持 capture_screenshot 等工具调用）
	// 工具循环期间用户等待；最终答案以模拟流式推送到前端
	a.chatStreamWithTools(ctx)
}

// chatStreamWithTools 执行工具循环，最终答案通过事件推送到前端
// Phase 17-B 核心方法：打通 "听 → 想 → 看 → 回答 → 说" 闭环
//
// 流程：
//  1. 非流式 LLM 调用 → 检查是否需要工具
//  2. 如果需要工具 → 执行 → 把结果加入对话 → 重复
//  3. 如果不需要工具 → 这是最终答案 → 通过事件模拟流式推送
//  4. 调用 postResponseHook（触发 TTS）
//
// Token 策略：
//   - 普通问题：1 次 LLM 调用，0 次工具调用
//   - 视觉问题：2-3 次 LLM 调用（判断+执行+最终回答），1 次截图
func (a *Agent) chatStreamWithTools(ctx context.Context) {
	const maxToolCalls = 5

	for toolCallsMade := 0; toolCallsMade < maxToolCalls; toolCallsMade++ {
		// 非流式调用检查是否需要工具
		resp, err := a.Chat(ctx)
		if err != nil {
			log.Errorf("LLM call failed in tool loop: %v", err)
			emitEvent("llmChunk", "[错误] "+err.Error())
			emitEvent("llmFinish", nil)
			// 即使 fullResponse 为空也要调 hook，恢复 voiceController 状态
			if a.postResponseHook != nil {
				a.postResponseHook("")
			}
			return
		}

		toolCall := parseToolCall(resp)
		if toolCall == nil {
			// 没有工具调用 → 这是最终答案 → 模拟流式推送
			a.streamFinalAnswer(resp)
			return
		}

		// 执行工具（带超时）
		log.Debugf("Executing tool: %s (call %d/%d)", toolCall.Name, toolCallsMade+1, maxToolCalls)
		toolCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		result, err := a.executeTool(toolCtx, toolCall)
		cancel()

		if err != nil {
			log.Errorf("Tool %s failed: %v", toolCall.Name, err)
			// 工具失败仍把错误结果加入对话，让 LLM 知道发生了什么
		} else {
			log.Debugf("Tool %s executed successfully, result type: %v", toolCall.Name, result.Type)
		}

		// 把工具结果加入对话（图片结果会填充 Message.Images）
		a.addToolResultToConversation(toolCall, result)
	}

	// 达到最大工具调用次数
	log.Warnf("Max tool calls (%d) reached", maxToolCalls)
	emitEvent("llmChunk", "[已达到最大工具调用次数，停止执行]")
	emitEvent("llmFinish", nil)
	if a.postResponseHook != nil {
		a.postResponseHook("")
	}
}

// streamFinalAnswer 把最终答案通过事件模拟流式推送到前端
//
// 工具循环结束后调用。把完整答案按小块推送，让前端有流式显示效果，
// 同时触发 postResponseHook（TTS 播放）。
func (a *Agent) streamFinalAnswer(answer string) {
	a.AddAssistantMessage(answer)

	go func() {
		// 按小块推送，模拟流式效果（每块 5 个 rune，间隔 20ms）
		// 对于短回答（< 50 字），总耗时约 200ms，用户几乎感觉不到延迟
		// 对于长回答，有明显的逐字出现效果
		runes := []rune(answer)
		chunkSize := 5
		for i := 0; i < len(runes); i += chunkSize {
			end := i + chunkSize
			if end > len(runes) {
				end = len(runes)
			}
			emitEvent("llmChunk", string(runes[i:end]))
			time.Sleep(20 * time.Millisecond)
		}
		emitEvent("llmFinish", nil)
		log.Debugf("Agent streaming complete: %d chars", len(answer))
		// 触发 post-response hook（用于 TTS 等）
		// 即使 answer 为空也要调，否则 voiceController 状态机永远卡在 Processing
		if a.postResponseHook != nil {
			a.postResponseHook(answer)
		}
	}()
}

// SetPostResponseHook 设置响应完成钩子（流式结束后调用，传入完整文本）
func (a *Agent) SetPostResponseHook(hook PostResponseHook) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.postResponseHook = hook
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

// GetGameManager 返回 game.Manager（Phase 12）
func (a *Agent) GetGameManager() *game.Manager {
	return a.gameManager
}

// GetGameContext 返回最近一次 ContextProvider 写入的 GameContext（Phase 16-B）
//
// 不触发检测；只读快照。Phase 12 起 game.ContextProvider 负责填充。
func (a *Agent) GetGameContext() *game.GameContext {
	if a.gameManager == nil {
		return nil
	}
	return a.gameManager.LastKnown()
}

// GetKnowledgeProvider 返回当前注入的 KnowledgeProvider（Phase 16-A+）
//
// Phase 17-A：返回类型改为 any。调用方需要 type-assert 到具体接口。
// 实际对象通常是 *knowledge.Service（实现 game.KnowledgeProvider + 5 个 Get*）。
func (a *Agent) GetKnowledgeProvider() any {
	return a.knowledgeProvider
}

// SetKnowledgeProvider 注入 KnowledgeProvider（main.go 启动 KB 服务后调用）
//
// Phase 17-A：
//   - 类型改为 any（Agent Core 不再 import 任何具体 KB 接口）
//   - 重新调用 wirePlugins，把 KB 转发到实现 game.SetupReceiver 的插件
//     （插件内部自行 type-assert 到自己需要的接口）
//   - 不再调 rebuildPreparer（Preparer 不再持有 KB / grounder / analyzers）
func (a *Agent) SetKnowledgeProvider(kb any) {
	a.knowledgeProvider = kb
	if a.gameManager != nil {
		a.wirePlugins()
	}
}

// wirePlugins 通用插件协作入口（Phase 17-A）
//
// 两步：
//  1. 对每个实现 game.SetupReceiver 的插件注入 GameState + Knowledge 依赖
//  2. 对每个实现 game.ToolContributor 的插件调用 RegisterTools
//
// 不感知具体插件类型 / 工具名。Agent Core 不再 import internal/game/plugins/*。
//
// 幂等：重复调用安全（SetupReceiver 内部覆盖，ToolContributor 内部覆盖）。
// 何时调用：
//   - Init 后（首次建立 gameManager 时）
//   - SetKnowledgeProvider 后（KB 注入时，让插件刷新 KB 引用）
func (a *Agent) wirePlugins() {
	if a.gameManager == nil {
		return
	}
	plugins := a.gameManager.Plugins()
	if len(plugins) == 0 {
		return
	}

	// 1) SetupReceiver：把 GameState / Knowledge 注入插件
	setupDeps := game.SetupDeps{
		GameState: managerSnapshotProvider{manager: a.gameManager},
		Knowledge: a.knowledgeProvider,
	}
	for _, pl := range plugins {
		if sr, ok := pl.(game.SetupReceiver); ok {
			if err := sr.AttachDependencies(setupDeps); err != nil {
				log.Warnf("agent: plugin %s AttachDependencies failed: %v", pl.ID(), err)
				continue
			}
			log.Debugf("agent: plugin %s attached deps (state+k)", pl.ID())
		}
	}

	// 2) ToolContributor：让插件把自己的工具挂到 registry
	if a.toolRegistry != nil {
		for _, pl := range plugins {
			if tc, ok := pl.(game.ToolContributor); ok {
				if err := tc.RegisterTools(a.toolRegistry); err != nil {
					log.Warnf("agent: plugin %s RegisterTools failed: %v", pl.ID(), err)
					continue
				}
				log.Debugf("agent: plugin %s registered tools", pl.ID())
			}
		}
	}
}

// managerSnapshotProvider 把 *game.Manager 适配为 game.GameStateProvider
//
// Phase 17-A：原本在 internal/agent/tft_tools_wiring.go（已删除）。
// 这是 Agent Core 内的通用工具，类型只依赖 game 包，零游戏特定。
type managerSnapshotProvider struct {
	manager *game.Manager
}

// Snapshot 委托给 manager（manager 为 nil 时返回 nil）
func (p managerSnapshotProvider) Snapshot() *game.GameContext {
	if p.manager == nil {
		return nil
	}
	return p.manager.Snapshot()
}

// prepareGameOptions 根据当前游戏上下文与用户输入，构建单次 LLM 请求的附加选项
//
// Phase 16-B 范围内：仅当 gameManager + Preparer 就位 + 有活跃游戏上下文时才注入；
// 否则返回零值 ChatOptions（保持现有行为不变）。
//
// 严格 0 LLM 调用 — 全部走本地分析器。
func (a *Agent) prepareGameOptions(ctx context.Context, userInput string) ChatOptions {
	if a.gameContextPreparer == nil {
		return ChatOptions{}
	}
	gctx := a.GetGameContext()
	if gctx == nil {
		return ChatOptions{}
	}

	// Phase 16-B 范围内：先把"已实现"的分析层接好；把 GameStateAnalysis
	// 切片 → 拼接到 system prompt 的实现需要 PromptBuilder，超出本阶段
	// Required Scope（属于 Agent Context Scope 阶段 — 后续 Phase 17+）。
	//
	// 因此 prepareGameOptions 当前返回零值；onUserInput 仍走原路径。
	_ = userInput
	_ = gctx
	return ChatOptions{}
}

// ToolCall 工具调用请求
type ToolCall struct {
	Name       string          `json:"name"`
	Parameters json.RawMessage `json:"parameters"`
}

// buildSystemPromptWithTools 构建包含工具定义的系统提示词
// Phase 17-B：增加 capture_screenshot 使用时机指导，让 LLM 知道何时该截图
func buildSystemPromptWithTools(basePrompt string, registry *tool.ToolRegistry) string {
	if registry == nil || registry.Count() == 0 {
		return basePrompt
	}

	var toolsSection strings.Builder
	toolsSection.WriteString("\n\n你可以使用以下工具来获取信息：\n\n")

	for _, t := range registry.ListTools() {
		toolsSection.WriteString(fmt.Sprintf("- **%s**: %s\n", t.Name(), t.Description()))
		toolsSection.WriteString(fmt.Sprintf("  输入参数: %s\n", t.InputSchema()))
	}

	toolsSection.WriteString(`
当你需要使用工具获取信息时，请按照以下格式回应：

<|ToolCall|>[{"name": "tool_name", "parameters": {}}]<|/ToolCall|>

注意：
- 只在你需要获取额外信息时才调用工具
- 如果不需要工具，请直接给出答案
- 一次只调用一个工具
- 参数必须是有效的 JSON 格式

**何时使用 capture_screenshot**：
- 用户询问"看看屏幕"、"帮我看看"、"看看现在"等视觉相关问题
- 用户问"现在应该怎么做"、"接下来怎么走"、"这个界面什么意思"等需要看到画面的问题
- 用户描述当前游戏/应用界面并询问含义
- 任何需要看到用户当前屏幕才能回答的问题

**何时不使用 capture_screenshot**：
- 普通聊天（"你好"、"你是谁"、"谢谢"）
- 解释性文字问题（"解释一下"、"上一句什么意思"）
- 不涉及当前画面的抽象问题或知识问答
- 已经看过当前画面且用户没有要求重新查看的追问
`)

	return basePrompt + toolsSection.String()
}

// ChatWithTools 执行 Agent Loop 处理工具调用
// 流程: 用户提问 → LLM 判断是否需要工具 → 执行工具 → 结果加入上下文 → LLM 回答
func (a *Agent) ChatWithTools(ctx context.Context) (string, errors.AppError) {
	maxToolCalls := 5 // 防止无限循环
	toolCallsMade := 0

	for {
		if toolCallsMade >= maxToolCalls {
			err := errors.New(errors.CodeLLMResponse, "too many tool calls (max 5), stopping")
			return "", err
		}

		// Get response from LLM
		resp, err := a.Chat(ctx)
		if err != nil {
			return "", err
		}

		// Check if response contains tool call
		toolCall := parseToolCall(resp)
		if toolCall == nil {
			// No tool call, return final answer
			return resp, nil
		}

		toolCallsMade++
		log.Debugf("LLM requested tool call: %s", toolCall.Name)

		// Execute tool with timeout
		toolCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()

		result, err := a.executeTool(toolCtx, toolCall)
		if err != nil {
			// Add error result to conversation and continue
			a.addToolResultToConversation(toolCall, result)
			log.Errorf("Tool execution failed: %v", err)
			continue
		}

		// Add tool result to conversation
		a.addToolResultToConversation(toolCall, result)
		log.Debugf("Tool %s executed successfully, result type: %v", toolCall.Name, result.Type)

		// Loop back to send augmented context to LLM
	}
}

// executeTool 执行单个工具调用
func (a *Agent) executeTool(ctx context.Context, call *ToolCall) (*tool.ToolResult, errors.AppError) {
	t, exists := a.toolRegistry.GetTool(call.Name)
	if !exists {
		err := errors.New(errors.CodeInvalidConfig, "tool not found: "+call.Name)
		return &tool.ToolResult{
			Success: false,
			Error:   err.Message(),
		}, err
	}

	return t.Execute(ctx, call.Parameters)
}

// addToolResultToConversation 将工具结果添加到对话
// Phase 17-B：图片类型结果填充 Message.Images 字段，让下次 LLM 调用能看到截图
func (a *Agent) addToolResultToConversation(call *ToolCall, result *tool.ToolResult) {
	a.mu.Lock()
	defer a.mu.Unlock()

	// 图片结果：填充 Images 字段，让下次 LLM 调用能通过 llm.ChatMessage.Images 看到截图
	if result.Type == tool.ToolResultTypeImage && result.Image != "" {
		a.currentConv.Messages = append(a.currentConv.Messages, Message{
			Role:    llm.RoleUser,
			Content: "[Tool result: screenshot captured. See attached image.]",
			Images: []llm.ImageContent{
				{DataURI: result.Image, MIMEType: "image/jpeg"},
			},
			ScreenshotAttached: true,
		})
		return
	}

	// 文本/JSON 结果：原有逻辑（序列化为 JSON 作为 user message）
	resultJSON, _ := json.Marshal(map[string]interface{}{
		"tool_call": call,
		"result":    result,
	})

	a.currentConv.Messages = append(a.currentConv.Messages, Message{
		Role:    llm.RoleUser,
		Content: string(resultJSON),
	})
}

// parseToolCall 从 LLM 响应解析工具调用
// Simple parsing looking for <|ToolCall|>...<|/ToolCall|>
func parseToolCall(response string) *ToolCall {
	startMarker := "<|ToolCall|>"
	endMarker := "<|/ToolCall|>"

	start := strings.Index(response, startMarker)
	if start == -1 {
		return nil
	}

	end := strings.Index(response[start:], endMarker)
	if end == -1 {
		return nil
	}

	jsonContent := strings.TrimSpace(response[start+len(startMarker) : start+end])
	var calls []ToolCall
	err := json.Unmarshal([]byte(jsonContent), &calls)
	if err != nil || len(calls) == 0 {
		log.Debugf("Failed to parse tool call: %v", err)
		return nil
	}

	// For simplicity, handle just the first tool call
	return &calls[0]
}

// 默认系统提示词
const defaultSystemPrompt = `你是 Aurora，一个专业的 PC 游戏 AI 助手。
你帮助玩家解答游戏相关问题，提供游戏策略、装备搭配、任务攻略等建议。

请遵守以下规则：
1. 回答简洁明了，重点突出
2. 如果不知道答案，直接说不知道，不要编造
3. 只提供游戏攻略和知识，不提供作弊方法
4. 保持友好专业的语气
`;
