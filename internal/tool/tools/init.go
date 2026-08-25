package tools

import (
	"github.com/lipipi/game-ai/internal/tool"
)

func init() {
	tool.RegisterTool("capture_screenshot", NewScreenshotTool)
	tool.RegisterTool("get_current_time", NewCurrentTimeTool)
}
