package tool

import (
	"context"
	"encoding/json"

	"github.com/lipipi/game-ai/internal/errors"
)

// ToolResultType 工具结果类型
type ToolResultType string

const (
	// ToolResultTypeText 文本结果
	ToolResultTypeText ToolResultType = "text"
	// ToolResultTypeImage 图片结果（base64 编码）
	ToolResultTypeImage ToolResultType = "image"
	// ToolResultTypeJSON JSON 结构化结果
	ToolResultTypeJSON ToolResultType = "json"
)

// ToolResult 工具执行结果，包含结构化数据
type ToolResult struct {
	Type    ToolResultType `json:"type"`
	Text    string          `json:"text,omitempty"`    // 文本结果
	Image   string          `json:"image,omitempty"`   // base64 编码图片（data URI 格式）
	Data    json.RawMessage `json:"data,omitempty"`   // 任意 JSON 结构化数据
	Success bool            `json:"success"`           // 是否成功
	Error   string          `json:"error,omitempty"`   // 错误信息（如果失败）
}

// Tool 统一工具接口
// Agent Core 依赖此接口，不依赖具体实现
type Tool interface {
	// Name 返回工具名称（唯一标识）
	Name() string
	// Description 返回工具描述，供 LLM 识别
	Description() string
	// InputSchema 返回 JSON Schema 格式的输入参数定义
	InputSchema() string
	// Execute 执行工具
	// ctx 包含超时控制，input 是已解析的 JSON 输入
	Execute(ctx context.Context, input json.RawMessage) (*ToolResult, errors.AppError)
	// IsAvailable 检查工具是否可用
	IsAvailable() bool
}
