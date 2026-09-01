// Package llm 定义大语言模型 Provider 接口
package llm

import (
	"context"

	"github.com/lipipi/aurora/internal/errors"
)

// Role 对话角色
type Role string

const (
	// RoleSystem 系统提示词
	RoleSystem Role = "system"
	// RoleUser 用户消息
	RoleUser Role = "user"
	// RoleAssistant AI 助手回复
	RoleAssistant Role = "assistant"
)

// ImageContent 图片输入（多模态）
type ImageContent struct {
	// DataURI 完整 data URI，如 "data:image/jpeg;base64,..."
	DataURI string `json:"url"`
	// MIMEType 图片 MIME 类型（如 "image/jpeg"），冗余字段便于日志
	MIMEType string `json:"mime_type,omitempty"`
}

// ChatMessage 单条对话消息
type ChatMessage struct {
	Role    Role           `json:"role"`
	Content string         `json:"content"`
	// Images 多模态图片输入，仅随本次请求发送，不影响纯文本路径
	Images []ImageContent `json:"images,omitempty"`
}

// ChatCompletionRequest 聊天补全请求
type ChatCompletionRequest struct {
	Messages         []ChatMessage  `json:"messages"`
	Model            string         `json:"model"`
	Temperature      float32        `json:"temperature,omitempty"`
	TopP             float32        `json:"top_p,omitempty"`
	MaxTokens        int            `json:"max_tokens,omitempty"`
	PresencePenalty  float32        `json:"presence_penalty,omitempty"`
	FrequencyPenalty float32        `json:"frequency_penalty,omitempty"`
	Stream           bool           `json:"stream"`
}

// TokenUsage Token 使用统计
type TokenUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// ChatCompletionResponse 聊天补全响应（非流式）
type ChatCompletionResponse struct {
	ID      string        `json:"id"`
	Model   string        `json:"model"`
	Content string        `json:"content"`
	Usage   TokenUsage    `json:"usage"`
}

// StreamChunk 流式响应块
type StreamChunk struct {
	ID        string `json:"id"`
	Content   string `json:"content"`
	Finished  bool   `json:"finished"`
	Usage     TokenUsage `json:"usage,omitempty"`
}

// LLMProvider 大语言模型 Provider 接口
// Agent Core 依赖此接口，不依赖具体实现
type LLMProvider interface {
	// ChatCompletion 非流式聊天补全
	ChatCompletion(ctx context.Context, req ChatCompletionRequest) (*ChatCompletionResponse, errors.AppError)
	// ChatCompletionStream 流式聊天补全，返回可读通道
	// 调用方需要关闭 ctx 或读完所有内容后关闭通道
	ChatCompletionStream(ctx context.Context, req ChatCompletionRequest) (<-chan StreamChunk, errors.AppError)
	// IsConfigured 检查 Provider 是否已正确配置
	IsConfigured() bool
	// Name 返回 Provider 名称
	Name() string
}

// ProviderConfig Provider 基础配置
type ProviderConfig struct {
	APIKey     string `json:"api_key"`
	BaseURL    string `json:"base_url,omitempty"`
	Model      string `json:"model"`
	TimeoutSec int    `json:"timeout_sec,omitempty"`
}

// Validate 验证配置是否合法
func (c *ProviderConfig) Validate() errors.AppError {
	if c.APIKey == "" {
		return errors.New(errors.CodeInvalidConfig, "API key is required")
	}
	if c.Model == "" {
		return errors.New(errors.CodeInvalidConfig, "model is required")
	}
	if c.TimeoutSec <= 0 {
		c.TimeoutSec = 60
	}
	return nil
}
