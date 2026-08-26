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
	"github.com/lipipi/game-ai/internal/input"
	"github.com/lipipi/game-ai/internal/log"
	"github.com/lipipi/game-ai/internal/llm"
	"github.com/lipipi/game-ai/internal/screenshot"
	"github.com/lipipi/game-ai/internal/tool"
	tools "github.com/lipipi/game-ai/internal/tool/tools"
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
		log.Warn("GetConfigStatus: agentInstance is nil")
		status["initialized"] = false
		status["configured"] = false
		return status
	}

	status["initialized"] = true

	llmProvider := agentInstance.GetLLMProvider()
	if llmProvider == nil {
		log.Warn("GetConfigStatus: llmProvider is nil")
		status["configured"] = false
		return status
	}

	status["provider"] = llmProvider.Name()
	status["configured"] = llmProvider.IsConfigured()
	log.Debugf("GetConfigStatus: provider=%s, configured=%v", llmProvider.Name(), llmProvider.IsConfigured())

	// Don't expose API key!
	if llmProvider.IsConfigured() {
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
	log.Debugf("SaveLLMConfig called: provider=%s, model=%s, baseURL=%s, apiKey=%s",
		provider, model, baseURL, maskAPIKeyForLog(apiKey))

	cfg := config.Global()
	cfg.LLM.Provider = provider
	cfg.LLM.OpenAI.Model = model
	if baseURL != "" {
		cfg.LLM.OpenAI.BaseURL = baseURL
	}
	// Don't overwrite with asterisks - only update if it's not the placeholder
	// Frontend sends '********' when user didn't change the existing API Key
	if apiKey != "" && apiKey != "********" {
		cfg.LLM.OpenAI.APIKey = apiKey
		log.Debugf("SaveLLMConfig: API Key updated (len=%d)", len(apiKey))
	} else {
		log.Debug("SaveLLMConfig: API Key NOT updated (placeholder or empty)")
	}

	log.Debugf("SaveLLMConfig: config state before save: provider=%s, model=%s, baseURL=%s, hasApiKey=%v",
		cfg.LLM.Provider, cfg.LLM.OpenAI.Model, cfg.LLM.OpenAI.BaseURL, cfg.LLM.OpenAI.APIKey != "")

	err := config.Save()
	if err != nil {
		log.Errorf("SaveLLMConfig: failed to save config: %v", err)
		return err
	}
	log.Debug("SaveLLMConfig: config saved to disk successfully")

	// Re-create LLM provider with new configuration
	if agentInstance != nil {
		log.Debug("SaveLLMConfig: agentInstance exists, hot-reloading LLM provider")
		newLLMProvider, err := llm.CreateProvider(cfg.LLM)
		if err != nil {
			log.Errorf("SaveLLMConfig: failed to re-create LLM provider: %v", err)
			return err
		}

		agentInstance.mu.Lock()
		agentInstance.llmProvider = newLLMProvider
		// Rebuild system prompt with tools
		agentInstance.systemPrompt = buildSystemPromptWithTools(defaultSystemPrompt, agentInstance.toolRegistry)
		agentInstance.mu.Unlock()

		log.Debugf("SaveLLMConfig: LLM provider hot-reloaded successfully, IsConfigured=%v", newLLMProvider.IsConfigured())
	} else {
		// agentInstance is nil (e.g. first launch without API Key)
		// Need to fully initialize the agent now that we have a valid config
		log.Warn("SaveLLMConfig: agentInstance is nil, initializing agent from scratch")

		newLLMProvider, err := llm.CreateProvider(cfg.LLM)
		if err != nil {
			log.Errorf("SaveLLMConfig: failed to create LLM provider during init: %v", err)
			return err
		}

		screenshotProvider, _ := screenshot.CreateProvider(cfg.Screenshot)

		toolRegistry, _ := tool.CreateRegistry(cfg.Tool)

		// Inject screenshot provider into ScreenshotTool if available
		if screenshotProvider != nil && toolRegistry != nil {
			if screenshotTool, exists := toolRegistry.GetTool("capture_screenshot"); exists {
				if st, ok := screenshotTool.(*tools.ScreenshotTool); ok {
					st.SetScreenshotProvider(screenshotProvider)
				}
			}
		}

		systemPrompt := buildSystemPromptWithTools(defaultSystemPrompt, toolRegistry)

		agentInstance = &Agent{
			llmProvider:        newLLMProvider,
			screenshotProvider: screenshotProvider,
			toolRegistry:       toolRegistry,
			enableTools:        toolRegistry != nil && toolRegistry.Count() > 0,
			systemPrompt:       systemPrompt,
		}
		agentInstance.newConversation()
		input.Current().RegisterCallback(agentInstance.onUserInput)

		log.Debugf("SaveLLMConfig: agent initialized from scratch, IsReady=%v", agentInstance.IsReady())
	}

	return nil
}

// maskAPIKeyForLog 脱敏 API Key 用于日志输出
func maskAPIKeyForLog(key string) string {
	if key == "" {
		return "(empty)"
	}
	if key == "********" {
		return "********"
	}
	if len(key) <= 8 {
		return key[:2] + "***"
	}
	return key[:4] + "***" + key[len(key)-4:]
}

// FrontendEcho 测试前端-后端连通性
func (b *Bindings) FrontendEcho(msg string) string {
	log.Debugf("Frontend echo: %s", msg)
	return "pong: " + msg
}

// --- PTT / Speech bindings ---

// IsPTTAvailable 检查 PTT 是否可用
func (b *Bindings) IsPTTAvailable() bool {
	ptt := input.GetPTT()
	if ptt == nil {
		return false
	}
	cfg := config.Global()
	return cfg.Speech.Volcengine.APIKey != ""
}

// IsPTTRecording 检查是否正在录音
func (b *Bindings) IsPTTRecording() bool {
	ptt := input.GetPTT()
	if ptt == nil {
		return false
	}
	return ptt.IsRecording()
}

// GetSpeechConfig 获取语音识别配置（不返回 API Key）
func (b *Bindings) GetSpeechConfig() map[string]interface{} {
	cfg := config.Global()
	return map[string]interface{}{
		"provider":    cfg.Speech.Provider,
		"endpoint":    cfg.Speech.Volcengine.Endpoint,
		"resource_id": cfg.Speech.Volcengine.ResourceID,
		"has_api_key": cfg.Speech.Volcengine.APIKey != "",
	}
}

// SaveSpeechConfig 保存语音识别配置
func (b *Bindings) SaveSpeechConfig(provider string, endpoint string, resourceID string, apiKey string) error {
	log.Debugf("SaveSpeechConfig: provider=%s, endpoint=%s, resourceID=%s, apiKey=%s",
		provider, endpoint, resourceID, maskAPIKeyForLog(apiKey))

	cfg := config.Global()
	if provider != "" {
		cfg.Speech.Provider = provider
	}
	if endpoint != "" {
		cfg.Speech.Volcengine.Endpoint = endpoint
	}
	if resourceID != "" {
		cfg.Speech.Volcengine.ResourceID = resourceID
	}
	if apiKey != "" && apiKey != "********" {
		cfg.Speech.Volcengine.APIKey = apiKey
		log.Debug("SaveSpeechConfig: API Key updated")
	}

	if err := config.Save(); err != nil {
		log.Errorf("SaveSpeechConfig: failed to save: %v", err)
		return err
	}

	// Note: hot-reload of speech provider requires re-creating PTT manager
	// which depends on both audio + speech providers. For now, changes take
	// effect on next app restart.
	log.Debug("SaveSpeechConfig: saved (restart required for provider reload)")
	return nil
}

// GetPTTState 获取 PTT 状态（0=idle, 1=recording）
func (b *Bindings) GetPTTState() int {
	ptt := input.GetPTT()
	if ptt == nil {
		return 0
	}
	if ptt.IsRecording() {
		return 1
	}
	return 0
}

// --- Wake Word bindings ---

// WakeWordConfigResponse is returned by GetWakeWordConfig to the frontend.
type WakeWordConfigResponse struct {
	Enabled         bool    `json:"enabled"`
	ModelPath       string  `json:"model_path"`
	Threshold       float32 `json:"threshold"`
	CooldownMs      int     `json:"cooldown_ms"`
	VADEnabled      bool    `json:"vad_enabled"`
	OnnxRuntimePath string  `json:"onnx_runtime_path"`
}

// GetWakeWordConfig 获取唤醒词配置
func (b *Bindings) GetWakeWordConfig() WakeWordConfigResponse {
	cfg := config.Global()
	return WakeWordConfigResponse{
		Enabled:         cfg.WakeWord.Enabled,
		ModelPath:       cfg.WakeWord.ModelPath,
		Threshold:       cfg.WakeWord.Threshold,
		CooldownMs:      cfg.WakeWord.CooldownMs,
		VADEnabled:      cfg.WakeWord.VADEnabled,
		OnnxRuntimePath: cfg.WakeWord.OnnxRuntimePath,
	}
}

// SaveWakeWordConfig 保存唤醒词配置
func (b *Bindings) SaveWakeWordConfig(
	enabled bool,
	modelPath string,
	threshold float32,
	cooldownMs int,
	vadEnabled bool,
	onnxRuntimePath string,
) error {
	log.Debugf("SaveWakeWordConfig: enabled=%v, modelPath=%s, threshold=%.2f, cooldownMs=%d, vadEnabled=%v",
		enabled, modelPath, threshold, cooldownMs, vadEnabled)

	cfg := config.Global()
	cfg.WakeWord.Enabled = enabled
	if modelPath != "" {
		cfg.WakeWord.ModelPath = modelPath
	}
	cfg.WakeWord.Threshold = threshold
	cfg.WakeWord.CooldownMs = cooldownMs
	cfg.WakeWord.VADEnabled = vadEnabled
	if onnxRuntimePath != "" {
		cfg.WakeWord.OnnxRuntimePath = onnxRuntimePath
	}

	if err := config.Save(); err != nil {
		log.Errorf("SaveWakeWordConfig: failed to save: %v", err)
		return err
	}

	log.Debug("SaveWakeWordConfig: saved (restart required for changes to take effect)")
	return nil
}
