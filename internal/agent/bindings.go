// Package agent Agent Wails bindings - 供前端调用
//
// 注意：此版本的 Wails 对 context.Context 参数处理有缺陷
// （传 null 会导致 reflect: Call using zero Value argument），
// 因此所有绑定方法都不带 ctx 参数，运行时上下文通过 SetRuntimeContext 注入。
package agent

import (
	"context"
	"sync"

	"github.com/lipipi/game-ai/internal/config"
	"github.com/lipipi/game-ai/internal/errors"
	"github.com/lipipi/game-ai/internal/log"
	"github.com/lipipi/game-ai/internal/llm"
	"github.com/lipipi/game-ai/internal/screenshot"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// Bindings Agent bindings for frontend
type Bindings struct {
}

// NewBindings creates new bindings instance
func NewBindings() *Bindings {
	return &Bindings{}
}

var (
	runtimeCtx   context.Context
	runtimeCtxMu sync.RWMutex
)

// SetRuntimeContext 设置 Wails 运行时上下文（main 在 OnStartup 时调用）
func SetRuntimeContext(ctx context.Context) {
	runtimeCtxMu.Lock()
	defer runtimeCtxMu.Unlock()
	runtimeCtx = ctx
}

// getRuntimeContext 获取运行时上下文
func getRuntimeContext() context.Context {
	runtimeCtxMu.RLock()
	defer runtimeCtxMu.RUnlock()
	if runtimeCtx == nil {
		return context.Background()
	}
	return runtimeCtx
}

// emitEvent 发送事件到前端（上下文未就绪时忽略）
func emitEvent(name string, data ...interface{}) {
	defer func() {
		// 防止 ctx 失效等意外导致 panic
	}()
	if runtimeCtx != nil {
		runtime.EventsEmit(getRuntimeContext(), name, data...)
	}
}

// IsReady checks if agent is ready and configured
func (b *Bindings) IsReady() bool {
	if agentInstance == nil {
		return false
	}
	return agentInstance.IsReady()
}

// GetConversation gets current conversation history
func (b *Bindings) GetConversation() []Message {
	if agentInstance == nil {
		return nil
	}
	return agentInstance.GetConversation()
}

// ClearHistory clears conversation history and starts new conversation
func (b *Bindings) ClearHistory() {
	if agentInstance == nil {
		return
	}
	agentInstance.ClearHistory()
}

// Chat sends user message and get response (non-streaming)
// This is for compatibility, streaming is preferred
func (b *Bindings) Chat(userMessage string) (string, error) {
	if agentInstance == nil {
		return "", errors.New(errors.CodeInvalidConfig, "Agent not initialized")
	}

	if !agentInstance.IsReady() {
		return "", errors.New(errors.CodeInvalidConfig, "LLM provider not configured")
	}

	agentInstance.AddUserMessage(userMessage)

	resp, err := agentInstance.Chat(context.Background())
	if err != nil {
		return "", err
	}

	return resp, nil
}

// ChatStream starts streaming chat response
// The response chunks are sent via events: "llmChunk" and "llmFinish"
func (b *Bindings) ChatStream(userMessage string) error {
	if agentInstance == nil {
		return errors.New(errors.CodeInvalidConfig, "Agent not initialized")
	}

	if !agentInstance.IsReady() {
		return errors.New(errors.CodeInvalidConfig, "LLM provider not configured")
	}

	agentInstance.AddUserMessage(userMessage)

	ch, err := agentInstance.ChatStream(context.Background())
	if err != nil {
		return err
	}

	var fullResponse string

	go func() {
		for chunk := range ch {
			if chunk.Content != "" {
				fullResponse += chunk.Content
				emitEvent("llmChunk", chunk.Content)
			}

			if chunk.Finished {
				agentInstance.AddAssistantMessage(fullResponse)
				emitEvent("llmFinish", nil)
				return
			}
		}
	}()

	return nil
}

// GetConfigStatus gets LLM configuration status
func (b *Bindings) GetConfigStatus() map[string]interface{} {
	status := make(map[string]interface{})

	if agentInstance == nil {
		status["initialized"] = false
		return status
	}

	status["initialized"] = true
	status["provider"] = agentInstance.GetLLMProvider().Name()
	status["configured"] = agentInstance.GetLLMProvider().IsConfigured()

	// Don't expose API key!
	if agentInstance.GetLLMProvider().IsConfigured() {
		status["has_api_key"] = true
	}

	return status
}

// CaptureFullScreenshot 测试捕获全屏截图
// 返回 JPEG 格式的 base64 图片数据
func (b *Bindings) CaptureFullScreenshot() (map[string]interface{}, error) {
	log.Debug("CaptureFullScreenshot called")

	if agentInstance == nil {
		return nil, errors.New(errors.CodeInvalidConfig, "Agent not initialized")
	}

	if !agentInstance.IsScreenshotAvailable() {
		return nil, errors.New(errors.CodeScreenshotFailed, "screenshot not available on this platform")
	}

	req := screenshot.ScreenshotRequest{
		Mode:       screenshot.CaptureModeFullScreen,
		MaxWidth:   1920,
		MaxHeight:  1080,
		Quality:    80,
	}

	result, err := agentInstance.CaptureScreenshot(context.Background(), req)
	if err != nil {
		log.Errorf("CaptureFullScreenshot failed: %v", err)
		return nil, err
	}

	response := make(map[string]interface{})
	response["width"] = result.Width
	response["height"] = result.Height
	response["format"] = result.Format
	response["data"] = result.Bytes

	log.Debugf("CaptureFullScreenshot done: %dx%d, %d bytes", result.Width, result.Height, len(result.Bytes))
	return response, nil
}

// GetLLMConfig 获取当前 LLM 配置（不返回 API Key）
func (b *Bindings) GetLLMConfig() map[string]interface{} {
	cfg := config.Global()
	result := make(map[string]interface{})
	result["provider"] = cfg.LLM.Provider
	result["model"] = cfg.LLM.OpenAI.Model
	result["base_url"] = cfg.LLM.OpenAI.BaseURL
	result["has_api_key"] = cfg.LLM.OpenAI.APIKey != ""
	return result
}

// SaveLLMConfig 保存 LLM 配置
func (b *Bindings) SaveLLMConfig(provider string, model string, baseURL string, apiKey string) error {
	cfg := config.Global()
	cfg.LLM.Provider = provider
	cfg.LLM.OpenAI.Model = model
	if baseURL != "" {
		cfg.LLM.OpenAI.BaseURL = baseURL
	}
	if apiKey != "" {
		cfg.LLM.OpenAI.APIKey = apiKey
	}
	err := config.Save()
	if err != nil {
		return err
	}

	// Re-create LLM provider with new configuration
	if agentInstance != nil {
		newLLMProvider, err := llm.CreateProvider(cfg.LLM)
		if err != nil {
			log.Errorf("Failed to re-initialize LLM provider: %v", err)
			return err
		}

		agentInstance.mu.Lock()
		agentInstance.llmProvider = newLLMProvider
		// Rebuild system prompt with tools
		agentInstance.systemPrompt = buildSystemPromptWithTools(defaultSystemPrompt, agentInstance.toolRegistry)
		agentInstance.mu.Unlock()

		log.Debug("LLM config saved and LLM provider re-initialized successfully")
	} else {
		log.Debug("LLM config saved successfully (agent not initialized yet)")
	}

	return nil
}

// FrontendEcho 测试前端-后端连通性
func (b *Bindings) FrontendEcho(msg string) string {
	log.Debugf("Frontend echo: %s", msg)
	return "pong: " + msg
}
