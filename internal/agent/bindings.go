// Package agent Agent Wails bindings - 供前端调用
//
// 注意：此版本的 Wails 对 context.Context 参数处理有缺陷
// （传 null 会导致 reflect: Call using zero Value argument），
// 因此所有绑定方法都不带 ctx 参数，运行时上下文通过 SetRuntimeContext 注入。
package agent

import (
	"context"
	"sync"

	"github.com/lipipi/aurora/internal/config"
	"github.com/lipipi/aurora/internal/errors"
	"github.com/lipipi/aurora/internal/game"
	"github.com/lipipi/aurora/internal/input"
	"github.com/lipipi/aurora/internal/llm"
	"github.com/lipipi/aurora/internal/log"
	"github.com/lipipi/aurora/internal/screenshot"
	"github.com/lipipi/aurora/internal/tool"
	tools "github.com/lipipi/aurora/internal/tool/tools"
	"github.com/lipipi/aurora/internal/tts"
	"github.com/lipipi/aurora/internal/wailsruntime"
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

// emitEvent 发送事件到前端（app 尚未就绪时由 wailsruntime.EmitEvent 内部忽略）
func emitEvent(name string, data ...interface{}) {
	defer func() {
		// 防止 ctx 失效等意外导致 panic
	}()
	wailsruntime.EmitEvent(name, data...)
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
// 委托给 onUserInput（与语音输入同一管道，含游戏检测 / 截图 / 回退逻辑）
func (b *Bindings) ChatStream(userMessage string) error {
	if agentInstance == nil {
		return errors.New(errors.CodeInvalidConfig, "Agent not initialized")
	}

	if !agentInstance.IsReady() {
		return errors.New(errors.CodeInvalidConfig, "LLM provider not configured")
	}

	agentInstance.onUserInput(input.UserInput{Text: userMessage, Source: input.SourceText})
	return nil
}

// GetActiveGame 获取当前检测到的游戏（TTL 缓存），供前端显示游戏徽章
func (b *Bindings) GetActiveGame() map[string]interface{} {
	if agentInstance == nil {
		return map[string]interface{}{"active": false}
	}
	manager := agentInstance.GetGameManager()
	if manager == nil {
		return map[string]interface{}{"active": false}
	}
	plugin, gctx, ok := manager.DetectActive(context.Background())
	if !ok {
		return map[string]interface{}{"active": false}
	}
	return map[string]interface{}{
		"active": true,
		"id":     plugin.ID(),
		"name":   plugin.Name(),
		"window": gctx.Window.Title,
	}
}

// GameContextResponse 前端可见的游戏上下文响应（Phase 10-6）
//
// 故意使用独立结构而非直接返回 *game.GameContext，理由：
//   - 隐藏 WindowHWND（uintptr 在前端无意义）
//   - 隐藏 Confidence（内部调试用，未来按需开放）
//   - GameData 通过 type 字段暴露"是哪种游戏的上下文"，具体内容由 type 决定
type GameContextResponse struct {
	Available    bool   `json:"available"`
	Game         string `json:"game"`
	Name         string `json:"name"`
	Phase        string `json:"phase"`
	DetectedAt   string `json:"detected_at,omitempty"`
	WindowTitle  string `json:"window_title,omitempty"`
	WindowClass  string `json:"window_class,omitempty"`
	GameDataType string `json:"game_data_type,omitempty"`
}

// GetGameContext 获取当前游戏上下文（供前端调试面板使用）
//
// Phase 10-6 范围：
//   - 仅暴露 Snapshot，不触发检测
//   - 不暴露 WindowHWND / Confidence
//   - GameData 暂时不展开（TFT/其他游戏的内部结构待 UI 设计）
func (b *Bindings) GetGameContext() GameContextResponse {
	if agentInstance == nil {
		return GameContextResponse{Available: false}
	}
	gctx := agentInstance.GetGameContext()
	if gctx == nil {
		return GameContextResponse{Available: false}
	}
	resp := GameContextResponse{
		Available:   true,
		Game:        gctx.Game,
		Name:        gctx.Name,
		Phase:       string(gctx.Phase),
		WindowTitle: gctx.Window.Title,
		WindowClass: gctx.Window.Class,
	}
	if !gctx.DetectedAt.IsZero() {
		resp.DetectedAt = gctx.DetectedAt.Format("2006-01-02T15:04:05Z07:00")
	}
	if gctx.GameData != nil {
		// Phase 17-A：使用 gctx.Game（由插件 Detect() 时填入，e.g. "tft"）。
		// 不再做 *tft.TFTGameContext 类型断言（保持 Agent Core 通用化）。
		resp.GameDataType = gctx.Game
		if resp.GameDataType == "" {
			resp.GameDataType = "unknown"
		}
	}
	return resp
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
		Mode:      screenshot.CaptureModeFullScreen,
		MaxWidth:  1920,
		MaxHeight: 1080,
		Quality:   80,
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

		// exit_conversation_mode：核心生命周期工具，确保存在；
		// 此处 voiceControllerRef 通常已就绪，会一并完成 requester 注入
		// （必须在 buildSystemPromptWithTools 之前）
		ensureExitConversationTool(toolRegistry)

		// Inject screenshot provider into ScreenshotTool if available
		if screenshotProvider != nil && toolRegistry != nil {
			if screenshotTool, exists := toolRegistry.GetTool("capture_screenshot"); exists {
				if st, ok := screenshotTool.(*tools.ScreenshotTool); ok {
					st.SetScreenshotProvider(screenshotProvider)
				}
			}
		}
		// 所有运行时依赖注入完成后，过滤掉仍不可用的工具
		// （见 CreateRegistry 注释：避免"先过滤后注入"的顺序 bug）
		if toolRegistry != nil {
			if removed := toolRegistry.FilterUnavailable(); removed > 0 {
				log.Debugf("SaveLLMConfig: filtered %d unavailable tool(s) after dependency injection", removed)
			}
		}

		systemPrompt := buildSystemPromptWithTools(defaultSystemPrompt, toolRegistry)

		gameManager, err := game.CreateManager(cfg.Game)
		if err != nil {
			log.Warnf("SaveLLMConfig: failed to create game manager: %v", err)
			gameManager = nil
		}

		// Phase 11-1：保留先前的 knowledge provider（main.go 在 Init 之前会注入）
		// Phase 17-A：类型为 any（Agent Core 不再 import 任何具体 KB 接口）
		prevKP := agentInstance.GetKnowledgeProvider()

		agentInstance = &Agent{
			llmProvider:        newLLMProvider,
			screenshotProvider: screenshotProvider,
			toolRegistry:       toolRegistry,
			enableTools:        toolRegistry != nil && toolRegistry.Count() > 0,
			systemPrompt:       systemPrompt,
			gameManager:        gameManager,
			screenshotOnDetect: cfg.Game.ScreenshotOnDetect,
			knowledgeProvider:  prevKP,
		}
		// Phase 17-A：通用插件 wiring（取代 injectTFTToolDeps + rebuildPreparer）
		if gameManager != nil {
			agentInstance.wirePlugins()
		}
		agentInstance.gameContextPreparer = NewGameContextPreparer(gameManager)

		agentInstance.newConversation()
		registerInputCallback(agentInstance)

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

// --- TTS bindings ---

// GetTTSConfig 获取语音合成配置（不返回 API Key）
func (b *Bindings) GetTTSConfig() map[string]interface{} {
	cfg := config.Global()
	return map[string]interface{}{
		"provider":      cfg.TTS.Provider,
		"resource_id":   cfg.TTS.Volcengine.ResourceId,
		"voice_type":    cfg.TTS.Volcengine.Speaker,
		"format":        cfg.TTS.Volcengine.Format,
		"sample_rate":   cfg.TTS.Volcengine.SampleRate,
		"speech_rate":   cfg.TTS.Volcengine.SpeechRate,
		"loudness_rate": cfg.TTS.Volcengine.LoudnessRate,
		"pitch":         cfg.TTS.Volcengine.Pitch,
		"has_api_key":   cfg.TTS.Volcengine.APIKey != "",
	}
}

// SaveTTSConfig 保存语音合成配置
func (b *Bindings) SaveTTSConfig(
	provider string,
	resourceID string,
	speaker string,
	format string,
	sampleRate int,
	speechRate int,
	loudnessRate int,
	pitch int,
	apiKey string,
) error {
	log.Debugf("SaveTTSConfig: provider=%s, speaker=%s, apiKey=%s", provider, speaker, maskAPIKeyForLog(apiKey))

	cfg := config.Global()
	if provider != "" {
		cfg.TTS.Provider = provider
	}
	if resourceID != "" {
		cfg.TTS.Volcengine.ResourceId = resourceID
	}
	if speaker != "" {
		cfg.TTS.Volcengine.Speaker = speaker
	}
	if format != "" {
		cfg.TTS.Volcengine.Format = format
	}
	if sampleRate > 0 {
		cfg.TTS.Volcengine.SampleRate = sampleRate
	}
	// 语速/音量/音调可以是 0 或负值，不做 > 0 判断
	cfg.TTS.Volcengine.SpeechRate = speechRate
	cfg.TTS.Volcengine.LoudnessRate = loudnessRate
	cfg.TTS.Volcengine.Pitch = pitch
	if apiKey != "" && apiKey != "********" {
		cfg.TTS.Volcengine.APIKey = apiKey
		log.Debug("SaveTTSConfig: API Key updated")
	}

	if err := config.Save(); err != nil {
		log.Errorf("SaveTTSConfig: failed to save: %v", err)
		return err
	}
	log.Debug("SaveTTSConfig: saved (restart required for provider reload)")
	return nil
}

// TestTTS 测试 TTS 合成 + 播放
// text: 要朗读的文本
// 返回错误如果合成失败
func (b *Bindings) TestTTS(text string) error {
	log.Debugf("TestTTS called: text=%q", text)
	provider, err := tts.CreateProvider(config.Global().TTS)
	if err != nil {
		return err
	}
	if !provider.IsConfigured() {
		return errors.New(errors.CodeTTSFailed, "TTS provider not configured (API Key missing)")
	}
	return provider.Synthesize(text)
}

// --- Voice bindings ---

// voiceController 引用，由 main.go 在 OnStartup 时注入
var voiceControllerRef *input.VoiceController

// SetVoiceController 注入 voiceController 引用（main.go 调用）
func SetVoiceController(vc *input.VoiceController) {
	voiceControllerRef = vc
	// VC 就绪 → 把 Conversation 生命周期补注入 exit_conversation_mode 工具
	// （Agent Init 先于 VC 创建，工具在 Init 时只建实例未注入 requester）
	if agentInstance != nil {
		ensureExitConversationTool(agentInstance.toolRegistry)
	}
}

// GetVoiceState 获取当前语音状态: "Idle" / "WakeDetected" / "SpeechListening" / "Processing" / "Speaking"
func (b *Bindings) GetVoiceState() string {
	if voiceControllerRef == nil {
		return "Idle"
	}
	return voiceControllerRef.GetState().String()
}

// IsAgentBusy 是否正在处理
func (b *Bindings) IsAgentBusy() bool {
	if voiceControllerRef == nil {
		return false
	}
	return voiceControllerRef.IsAgentBusy()
}

// IsTTSPlaying TTS 是否在播放
func (b *Bindings) IsTTSPlaying() bool {
	if voiceControllerRef == nil {
		return false
	}
	return voiceControllerRef.IsTTSPlaying()
}

// --- Overlay bindings (Phase 3 MVP) ---

// clampOpacity 限制 Opacity 在 [0.30, 1.00] 区间，避免不可读或完全透明
func clampOpacity(opacity float64) float64 {
	if opacity < 0.30 {
		return 0.30
	}
	if opacity > 1.00 {
		return 1.00
	}
	return opacity
}

// normalizeFontSize 仅接受预设值，其他值回退 "medium"
func normalizeFontSize(size string) string {
	switch size {
	case "small", "medium", "large":
		return size
	default:
		return "medium"
	}
}

// GetOverlayConfig 获取 Overlay 配置
func (b *Bindings) GetOverlayConfig() map[string]interface{} {
	cfg := config.Global()
	return map[string]interface{}{
		"enabled":                 cfg.Overlay.Enabled,
		"opacity":                 cfg.Overlay.Opacity,
		"font_size":               cfg.Overlay.FontSize,
		"auto_scroll":             cfg.Overlay.AutoScroll,
		"show_user_messages":      cfg.Overlay.ShowUserMessages,
		"show_assistant_messages": cfg.Overlay.ShowAssistantMessages,
		// anchor 字段已移除（2026-09-07）。默认位置在 wailsruntime.setDefaultOverlayPosition 硬编码。
		"locked": cfg.Overlay.Locked,
		"x":      cfg.Overlay.X,
		"y":      cfg.Overlay.Y,
		"width":  cfg.Overlay.Width,
		"height": cfg.Overlay.Height,
	}
}

// SaveOverlayConfig 保存 Overlay 配置
//
// 热重载策略（任务 §15）：
//   - 所有视觉字段（opacity / fontSize / autoScroll / showUserMessages /
//     showAssistantMessages）保存后立即通过 overlayConfigChanged 事件
//     通知前端，Overlay 组件实时应用，无需重启。
//   - Enabled 仅作为「下次启动默认状态」持久化；运行时是否可见由
//     SetOverlayVisible 控制（避免 Settings 改 Enabled 与 Tray toggle 行为冲突）。
//   - anchor 已移除（2026-09-07），默认位置硬编码。
func (b *Bindings) SaveOverlayConfig(
	enabled bool,
	opacity float64,
	fontSize string,
	autoScroll bool,
	showUserMessages bool,
	showAssistantMessages bool,
) error {
	log.Debugf("SaveOverlayConfig: enabled=%v opacity=%.2f fontSize=%s autoScroll=%v showUser=%v showAssistant=%v",
		enabled, opacity, fontSize, autoScroll, showUserMessages, showAssistantMessages)

	cfg := config.Global()
	cfg.Overlay.Enabled = enabled
	cfg.Overlay.Opacity = clampOpacity(opacity)
	cfg.Overlay.FontSize = normalizeFontSize(fontSize)
	cfg.Overlay.AutoScroll = autoScroll
	cfg.Overlay.ShowUserMessages = showUserMessages
	cfg.Overlay.ShowAssistantMessages = showAssistantMessages

	if err := config.Save(); err != nil {
		log.Errorf("SaveOverlayConfig: failed to save: %v", err)
		return err
	}

	// 双向同步运行态（保证 GetOverlayVisible / 托盘标签 / 窗口显示状态都一致）。
	// SetOverlayVisible 内部会再写一次 Enabled，但因为值相同会 early return，
	// 不会递归 Save。
	setOverlayVisibleState(enabled)

	// 通知前端配置已变更，立即生效
	emitEvent("overlayConfigChanged", nil)
	log.Debug("SaveOverlayConfig: saved and broadcast overlayConfigChanged")
	return nil
}

// SetOverlayVisible 切换 Overlay 当前可见性 + 持久化 Enabled。
//
// 用于：
//   - Tray 菜单的 Show/Hide Overlay
//   - 前端 Overlay 组件的关闭按钮
//
// 同时：① 更新进程内 overlayVisibleState ② 写回 cfg.Overlay.Enabled + Save
// （之前只改运行态不改盘，导致重启后开关显示为开但窗口不显示——
// 用户期望"上次关了/开了，下次还保持"）。
// 通过 overlayVisibilityChanged 事件通知所有订阅者（前端 + 托盘刷新标签）。
func (b *Bindings) SetOverlayVisible(visible bool) {
	log.Debugf("SetOverlayVisible: %v", visible)
	setOverlayVisibleState(visible)
	// 持久化：与运行态保持一致，重启后恢复相同状态
	cfg := config.Global()
	if cfg.Overlay.Enabled != visible {
		cfg.Overlay.Enabled = visible
		if err := config.Save(); err != nil {
			log.Errorf("SetOverlayVisible: failed to save enabled=%v: %v", visible, err)
		}
	}
	emitEvent("overlayVisibilityChanged", visible)
}

// GetOverlayVisible 获取当前 Overlay 应可见状态
// 返回进程内运行时态 overlayVisibleState，与 cfg.Overlay.Enabled 保持同步
// （SetOverlayVisible 会同时写盘）。初始化时从 config.Overlay.Enabled 继承。
// （前端初始化时调用，避免重启后 overlay 状态丢失）
func (b *Bindings) GetOverlayVisible() bool {
	return getOverlayVisibleState()
}

// overlayVisibleState：进程内 Overlay 可见性，与 cfg.Overlay.Enabled 保持同步
// （SetOverlayVisible 同时写盘，不再是纯运行态——用户期望"上次开/关了，
// 下次还保持"）。默认从 config.Overlay.Enabled 继承；
// 由 SetOverlayVisible / SetOverlayVisibleAndBroadcast / initOverlayVisibleState 维护。
// 锁：单独 mutex，避免与 runtimeCtx 锁耦合。
var (
	overlayVisibleState   bool
	overlayVisibleStateMu sync.RWMutex
)

// initOverlayVisibleState 启动时用 config.Overlay.Enabled 初始化一次
func initOverlayVisibleState() {
	cfg := config.Global()
	overlayVisibleStateMu.Lock()
	overlayVisibleState = cfg.Overlay.Enabled
	overlayVisibleStateMu.Unlock()
	log.Debugf("initOverlayVisibleState: initialized to cfg.Overlay.Enabled=%v", cfg.Overlay.Enabled)
}

// InitOverlayVisibleState 公开版，供 main.go 在 agent.Init 之前预初始化
// Overlay 运行时可见性（托盘菜单初始文本依赖它）。
// Idempotent：在 agent.Init 内部会再次调用，结果相同。
func InitOverlayVisibleState() {
	initOverlayVisibleState()
}

func setOverlayVisibleState(visible bool) {
	overlayVisibleStateMu.Lock()
	overlayVisibleState = visible
	overlayVisibleStateMu.Unlock()
}

func getOverlayVisibleState() bool {
	overlayVisibleStateMu.RLock()
	defer overlayVisibleStateMu.RUnlock()
	return overlayVisibleState
}

// SetOverlayVisibleAndBroadcast 更新进程内 Overlay 可见性状态 + 广播事件。
// 供 main.go 路径使用（Tray 菜单点击等不经前端 binding 的场景）。
// 行为等价于 Bindings.SetOverlayVisible，但不需要构造 Bindings 实例。
func SetOverlayVisibleAndBroadcast(visible bool) {
	setOverlayVisibleState(visible)
	// 持久化：与运行态保持一致，重启后恢复相同状态
	// （与 Bindings.SetOverlayVisible 语义一致，只是不需要构造实例）
	cfg := config.Global()
	if cfg.Overlay.Enabled != visible {
		cfg.Overlay.Enabled = visible
		if err := config.Save(); err != nil {
			log.Errorf("SetOverlayVisibleAndBroadcast: failed to save enabled=%v: %v", visible, err)
		}
	}
	emitEvent("overlayVisibilityChanged", visible)
}

// HideOverlayTemporary 临时隐藏 Overlay（不修改持久化配置）。
// 用于 Overlay 窗口关闭按钮：用户点关闭只是临时隐藏，下次启动仍按 cfg.Overlay.Enabled 决定。
// 与 SetOverlayVisibleAndBroadcast(false) 的区别：不写 cfg.Overlay.Enabled。
func HideOverlayTemporary() {
	setOverlayVisibleState(false)
	emitEvent("overlayVisibilityChanged", false)
	log.Debug("HideOverlayTemporary: overlay hidden (runtime only, not persisted)")
}

// GetOverlayVisibleState 读取进程内 Overlay 可见性（供 main.go / Tray 路径使用）。
func GetOverlayVisibleState() bool {
	return getOverlayVisibleState()
}
