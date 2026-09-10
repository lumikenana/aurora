// Package tools - Aurora 内置通用工具（Phase 17-A 重定位）
//
// 通用工具：跨游戏可用，不依赖任何具体游戏插件。
//   - capture_screenshot：全屏截图（供 LLM 视觉分析）
//   - get_current_time：当前时间
//
// 历史：Phase 11-1 曾在此包注册 5 个 TFT 工具（get_tft_game_state /
// get_tft_champion / get_tft_item / get_tft_trait / get_tft_augment）。
// Phase 17-A 把它们下放到 internal/game/plugins/tft/tools/，由该包
// 的 init() 自注册。本包不再包含任何 TFT 工具。
//
// 新增游戏时不要在此加游戏专用工具——到你的 plugin/tools/ 包里注册。
package tools

import (
	"github.com/lipipi/aurora/internal/tool"
)

func init() {
	tool.RegisterTool("capture_screenshot", NewScreenshotTool)
	tool.RegisterTool("get_current_time", NewCurrentTimeTool)
	tool.RegisterTool("exit_conversation_mode", NewExitConversationTool)
}
