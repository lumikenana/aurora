package tools

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"image/jpeg"

	"github.com/lipipi/aurora/internal/errors"
	"github.com/lipipi/aurora/internal/screenshot"
	"github.com/lipipi/aurora/internal/tool"
)

// ScreenshotTool 全屏截图工具
// 捕获全屏并返回 JPEG base64 图片供 LLM 分析
type ScreenshotTool struct {
	screenshotProvider screenshot.ScreenshotProvider
	defaultQuality     int
	defaultMaxWidth    int
	defaultMaxHeight   int
}

// NewScreenshotTool 创建 ScreenshotTool
func NewScreenshotTool(cfg tool.Config) (tool.Tool, errors.AppError) {
	return &ScreenshotTool{
		defaultQuality:   80,
		defaultMaxWidth:  1920,
		defaultMaxHeight: 1080,
	}, nil
}

// SetScreenshotProvider 设置截图 provider（由 Agent 注入）
func (t *ScreenshotTool) SetScreenshotProvider(p screenshot.ScreenshotProvider) {
	t.screenshotProvider = p
}

func (t *ScreenshotTool) Name() string {
	return "capture_screenshot"
}

func (t *ScreenshotTool) Description() string {
	return "Capture a full screen screenshot and return it as a JPEG image. Use this when you need to see what's on the screen to answer the user's question."
}

func (t *ScreenshotTool) InputSchema() string {
	return `{
	"type": "object",
	"properties": {},
	"required": []
}`
}

func (t *ScreenshotTool) Execute(ctx context.Context, input json.RawMessage) (*tool.ToolResult, errors.AppError) {
	if t.screenshotProvider == nil || !t.screenshotProvider.IsAvailable() {
		err := errors.New(errors.CodeScreenshotFailed, "screenshot provider not available")
		return &tool.ToolResult{
			Success: false,
			Error:   err.Message(),
		}, err
	}

	req := screenshot.ScreenshotRequest{
		Mode:       screenshot.CaptureModeFullScreen,
		Quality:    t.defaultQuality,
		MaxWidth:   t.defaultMaxWidth,
		MaxHeight:  t.defaultMaxHeight,
	}

	result, err := t.screenshotProvider.Capture(ctx, req)
	if err != nil {
		return &tool.ToolResult{
			Success: false,
			Error:   err.Message(),
		}, err
	}

	// Encode image to JPEG base64 data URI
	var buf bytes.Buffer
	encodeErr := jpeg.Encode(&buf, result.OriginalImage, &jpeg.Options{Quality: t.defaultQuality})
	if encodeErr != nil {
		appErr := errors.Wrap(errors.CodeScreenshotFailed, "failed to encode JPEG", encodeErr)
		return &tool.ToolResult{
			Success: false,
			Error:   appErr.Message(),
		}, appErr
	}

	base64Data := base64.StdEncoding.EncodeToString(buf.Bytes())
	dataURI := "data:image/jpeg;base64," + base64Data

	return &tool.ToolResult{
		Type:    tool.ToolResultTypeImage,
		Image:   dataURI,
		Success: true,
	}, nil
}

func (t *ScreenshotTool) IsAvailable() bool {
	return t.screenshotProvider != nil && t.screenshotProvider.IsAvailable()
}
