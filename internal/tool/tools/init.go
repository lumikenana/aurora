package tools

import (
	"github.com/lipipi/aurora/internal/tool"
)

func init() {
	tool.RegisterTool("capture_screenshot", NewScreenshotTool)
	tool.RegisterTool("get_current_time", NewCurrentTimeTool)
}
