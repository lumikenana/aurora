// Package agent Agent Wails bindings - 供前端调用
package agent

import (
	"context"

	"github.com/lipipi/game-ai/internal/errors"
	"github.com/lipipi/game-ai/internal/log"
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

// IsReady checks if agent is ready and configured
func (b *Bindings) IsReady(ctx context.Context) bool {
	if agentInstance == nil {
		return false
	}
	return agentInstance.IsReady()
}

// GetConversation gets current conversation history
func (b *Bindings) GetConversation(ctx context.Context) []Message {
	if agentInstance == nil {
		return nil
	}
	return agentInstance.GetConversation()
}

// ClearHistory clears conversation history and starts new conversation
func (b *Bindings) ClearHistory(ctx context.Context) {
	if agentInstance == nil {
		return
	}
	agentInstance.ClearHistory()
}

// Chat sends user message and get response (non-streaming)
// This is for compatibility, streaming is preferred
func (b *Bindings) Chat(ctx context.Context, userMessage string) (string, error) {
	if agentInstance == nil {
		return "", errors.New(errors.CodeInvalidConfig, "Agent not initialized")
	}

	if !agentInstance.IsReady() {
		return "", errors.New(errors.CodeInvalidConfig, "LLM provider not configured")
	}

	agentInstance.AddUserMessage(userMessage)

	resp, err := agentInstance.Chat(ctx)
	if err != nil {
		return "", err
	}

	return resp, nil
}

// ChatStream starts streaming chat response
// The response chunks are sent via events: "chunk" and "finish"
func (b *Bindings) ChatStream(ctx context.Context, userMessage string) error {
	if agentInstance == nil {
		return errors.New(errors.CodeInvalidConfig, "Agent not initialized")
	}

	if !agentInstance.IsReady() {
		return errors.New(errors.CodeInvalidConfig, "LLM provider not configured")
	}

	agentInstance.AddUserMessage(userMessage)

	ch, err := agentInstance.ChatStream(ctx)
	if err != nil {
		return err
	}

	var fullResponse string

	go func() {
		for chunk := range ch {
			select {
			case <-ctx.Done():
				log.Debug("Chat streaming cancelled")
				return
			default:
			}

			if chunk.Content != "" {
				fullResponse += chunk.Content
				runtime.EventsEmit(ctx, "llmChunk", chunk.Content)
			}

			if chunk.Finished {
				agentInstance.AddAssistantMessage(fullResponse)
				runtime.EventsEmit(ctx, "llmFinish", nil)
				return
			}
		}
	}()

	return nil
}

// GetConfigStatus gets LLM configuration status
func (b *Bindings) GetConfigStatus(ctx context.Context) map[string]interface{} {
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
func (b *Bindings) CaptureFullScreenshot(ctx context.Context) (map[string]interface{}, error) {
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

	result, err := agentInstance.CaptureScreenshot(ctx, req)
	if err != nil {
		return nil, err
	}

	response := make(map[string]interface{})
	response["width"] = result.Width
	response["height"] = result.Height
	response["format"] = result.Format
	response["data"] = result.Bytes

	return response, nil
}
