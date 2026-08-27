// Package openai OpenAI 兼容 API Provider
// 支持标准 OpenAI 接口以及兼容 OpenAI 的第三方服务（如火山引擎豆包）
package openai

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/lipipi/aurora/internal/errors"
	"github.com/lipipi/aurora/internal/log"
	"github.com/lipipi/aurora/internal/llm"
)

// Config OpenAI Provider 配置
type Config struct {
	llm.ProviderConfig `json:",inline"`
}

// OpenAIProvider OpenAI 兼容 API 实现
type OpenAIProvider struct {
	config Config
	client *http.Client
}

// New 创建新的 OpenAI Provider
func New(cfg any) (llm.LLMProvider, errors.AppError) {
	openaiCfg, ok := cfg.(Config)
	if !ok {
		// 尝试从 llm.ProviderConfig 转换
		pc, ok := cfg.(llm.ProviderConfig)
		if !ok {
			return nil, errors.New(errors.CodeInvalidConfig, "invalid config type for OpenAI provider")
		}
		openaiCfg = Config{ProviderConfig: pc}
	}

	if err := openaiCfg.Validate(); err != nil {
		return nil, err
	}

	client := &http.Client{
		Timeout: time.Duration(openaiCfg.TimeoutSec) * time.Second,
	}
	return &OpenAIProvider{
		config: openaiCfg,
		client: client,
	}, nil
}

// 自动注册
func init() {
	llm.RegisterProvider("openai", New)
}

// Name 返回 Provider 名称
func (p *OpenAIProvider) Name() string {
	return "openai-compatible"
}

// IsConfigured 检查是否已正确配置
func (p *OpenAIProvider) IsConfigured() bool {
	return p.config.APIKey != "" && p.config.Model != ""
}

// buildURL 拼接请求 URL：BaseURL + path
// 若 BaseURL 为空则使用 OpenAI 默认地址；若已包含完整路径（以 /chat/completions 结尾）则不再重复拼接
func (p *OpenAIProvider) buildURL(path string) string {
	base := p.config.BaseURL
	if base == "" {
		return "https://api.openai.com/v1" + path
	}
	if strings.HasSuffix(base, path) {
		return base
	}
	return strings.TrimRight(base, "/") + path
}

// openAIRequest OpenAI 请求格式
type openAIRequest struct {
	Model            string         `json:"model"`
	Messages         []openAIMessage `json:"messages"`
	Temperature      float32        `json:"temperature,omitempty"`
	TopP             float32        `json:"top_p,omitempty"`
	MaxTokens        int            `json:"max_tokens,omitempty"`
	PresencePenalty  float32        `json:"presence_penalty,omitempty"`
	FrequencyPenalty float32        `json:"frequency_penalty,omitempty"`
	Stream           bool           `json:"stream"`
	// 火山引擎 ARK 豆包等思考模型：显式关闭 thinking，避免返回空 content
	Thinking *thinkingConfig `json:"thinking,omitempty"`
}

// thinkingConfig 思考模式配置（火山引擎 ARK 豆包等模型使用）
type thinkingConfig struct {
	Type string `json:"type"` // "enabled" / "disabled" / "auto"
}

type openAIMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// openAIResponse 非流式响应
type openAIResponse struct {
	ID      string       `json:"id"`
	Object  string       `json:"object"`
	Created int64        `json:"created"`
	Model   string       `json:"model"`
	Choices []openAIChoice `json:"choices"`
	Usage   openAIUsage  `json:"usage"`
}

type openAIChoice struct {
	Index        int           `json:"index"`
	Message       openAIMessage `json:"message"`
	FinishReason string        `json:"finish_reason"`
}

type openAIUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// ChatCompletion 非流式聊天补全
func (p *OpenAIProvider) ChatCompletion(ctx context.Context, req llm.ChatCompletionRequest) (*llm.ChatCompletionResponse, errors.AppError) {
	// 转换请求格式
	aiReq := p.convertRequest(req)

	reqBody, err := json.Marshal(aiReq)
	if err != nil {
		return nil, errors.Wrap(errors.CodeLLMRequest, "Failed to marshal request", err)
	}

	// 创建 HTTP 请求
	url := p.buildURL("/chat/completions")

	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(reqBody))
	if err != nil {
		return nil, errors.Wrap(errors.CodeLLMRequest, "Failed to create request", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+p.config.APIKey)

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, errors.Wrap(errors.CodeLLMNetwork, "Failed to send request to LLM API", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		log.Errorf("LLM API error: status=%d, body=%s", resp.StatusCode, string(bodyBytes))
		return nil, errors.Newf(errors.CodeLLMAPIError, "LLM API returned error: status %d", resp.StatusCode)
	}

	var aiResp openAIResponse
	if err := json.NewDecoder(resp.Body).Decode(&aiResp); err != nil {
		return nil, errors.Wrap(errors.CodeLLMResponse, "Failed to decode response", err)
	}

	if len(aiResp.Choices) == 0 {
		return nil, errors.New(errors.CodeLLMResponse, "No choices returned in response")
	}

	result := &llm.ChatCompletionResponse{
		ID:    aiResp.ID,
		Model: aiResp.Model,
		Content: aiResp.Choices[0].Message.Content,
		Usage: llm.TokenUsage{
			PromptTokens:     aiResp.Usage.PromptTokens,
			CompletionTokens: aiResp.Usage.CompletionTokens,
			TotalTokens:      aiResp.Usage.TotalTokens,
		},
	}

	log.Debugf("LLM request completed: %d prompt tokens, %d completion tokens",
		result.Usage.PromptTokens, result.Usage.CompletionTokens)

	return result, nil
}

// ChatCompletionStream 流式聊天补全
func (p *OpenAIProvider) ChatCompletionStream(ctx context.Context, req llm.ChatCompletionRequest) (<-chan llm.StreamChunk, errors.AppError) {
	req.Stream = true
	aiReq := p.convertRequest(req)

	reqBody, err := json.Marshal(aiReq)
	if err != nil {
		return nil, errors.Wrap(errors.CodeLLMRequest, "Failed to marshal streaming request", err)
	}

	url := p.buildURL("/chat/completions")

	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(reqBody))
	if err != nil {
		return nil, errors.Wrap(errors.CodeLLMRequest, "Failed to create streaming request", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+p.config.APIKey)
	httpReq.Header.Set("Accept", "text/event-stream")
	httpReq.Header.Set("Cache-Control", "no-cache")
	httpReq.Header.Set("Connection", "keep-alive")

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, errors.Wrap(errors.CodeLLMNetwork, "Failed to send streaming request", err)
	}

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		log.Errorf("LLM streaming API error: status=%d, body=%s", resp.StatusCode, string(bodyBytes))
		resp.Body.Close()
		return nil, errors.Newf(errors.CodeLLMAPIError, "LLM API returned error: status %d", resp.StatusCode)
	}

	ch := make(chan llm.StreamChunk)

	go p.readStream(ctx, resp.Body, ch)

	return ch, nil
}

// readStream 读取流式响应并发送到通道
func (p *OpenAIProvider) readStream(ctx context.Context, body io.ReadCloser, ch chan<- llm.StreamChunk) {
	defer body.Close()
	defer close(ch)

	scanner := bufio.NewScanner(body)
	lineCount := 0
	chunkCount := 0

	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return
		default:
		}

		line := scanner.Text()
		lineCount++

		if line == "" {
			continue
		}

		log.Debugf("SSE raw line %d: %s", lineCount, line)

		// SSE 格式: "data: ..." 或 "data:..."
		if len(line) < 5 || line[:4] != "data" {
			log.Debugf("Skipping non-data line: %q", line)
			continue
		}
		// 跳过 "data"，再剥离可选的 ":" 和前导空格
		rest := line[4:]
		if len(rest) > 0 && rest[0] == ':' {
			rest = rest[1:]
		}
		if len(rest) > 0 && rest[0] == ' ' {
			rest = rest[1:]
		}
		data := rest
		if data == "[DONE]" {
			log.Debugf("Stream [DONE] after %d chunks", chunkCount)
			ch <- llm.StreamChunk{
				Finished: true,
			}
			return
		}

		var streamResp struct {
			ID      string          `json:"id"`
			Choices []streamChoice  `json:"choices"`
		}

		if err := json.Unmarshal([]byte(data), &streamResp); err != nil {
			log.Debugf("Failed to parse stream chunk: %v, raw: %s", err, data)
			continue
		}

		if len(streamResp.Choices) == 0 {
			log.Debugf("Stream chunk with no choices: %s", data)
			continue
		}

		delta := streamResp.Choices[0].Delta
		if delta.Content == "" {
			continue
		}

		chunkCount++
		ch <- llm.StreamChunk{
			ID:       streamResp.ID,
			Content:  delta.Content,
			Finished: streamResp.Choices[0].FinishReason != "",
		}
	}

	if err := scanner.Err(); err != nil {
		log.Errorf("Stream reading error: %v", err)
	}

	log.Debugf("Stream ended: %d lines read, %d chunks emitted", lineCount, chunkCount)
	ch <- llm.StreamChunk{
		Finished: true,
	}
}

// streamChoice 流式响应选择
type streamChoice struct {
	Delta        streamDelta `json:"delta"`
	FinishReason string      `json:"finish_reason"`
}

type streamDelta struct {
	Content string `json:"content"`
}

// convertRequest 转换内部请求到 OpenAI 格式
func (p *OpenAIProvider) convertRequest(req llm.ChatCompletionRequest) openAIRequest {
	aiReq := openAIRequest{
		Model:            p.config.Model,
		Temperature:      req.Temperature,
		TopP:             req.TopP,
		MaxTokens:        req.MaxTokens,
		PresencePenalty:  req.PresencePenalty,
		FrequencyPenalty: req.FrequencyPenalty,
		Stream:           req.Stream,
		Thinking:         &thinkingConfig{Type: "disabled"}, // 关闭豆包等模型的思考模式，避免返回空 content
	}

	aiReq.Messages = make([]openAIMessage, len(req.Messages))
	for i, msg := range req.Messages {
		aiReq.Messages[i] = openAIMessage{
			Role:    string(msg.Role),
			Content: msg.Content,
		}
	}

	return aiReq
}
