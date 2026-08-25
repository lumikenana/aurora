package tools

import (
	"context"
	"encoding/json"
	"time"

	"github.com/lipipi/game-ai/internal/errors"
	"github.com/lipipi/game-ai/internal/tool"
)

// CurrentTimeTool 获取当前服务器时间工具
// 返回当前时间的 ISO 8601 格式字符串
type CurrentTimeTool struct {
}

// NewCurrentTimeTool 创建 CurrentTimeTool
func NewCurrentTimeTool(cfg tool.Config) (tool.Tool, errors.AppError) {
	return &CurrentTimeTool{}, nil
}

func (t *CurrentTimeTool) Name() string {
	return "get_current_time"
}

func (t *CurrentTimeTool) Description() string {
	return "Get the current server time in ISO 8601 (RFC3339) format. Use this when you need to know the current date or time."
}

func (t *CurrentTimeTool) InputSchema() string {
	return `{
	"type": "object",
	"properties": {},
	"required": []
}`
}

func (t *CurrentTimeTool) Execute(ctx context.Context, input json.RawMessage) (*tool.ToolResult, errors.AppError) {
	now := time.Now().Format(time.RFC3339)
	return &tool.ToolResult{
		Type:    tool.ToolResultTypeText,
		Text:    now,
		Success: true,
	}, nil
}

func (t *CurrentTimeTool) IsAvailable() bool {
	return true
}
