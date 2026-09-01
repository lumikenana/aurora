// Package tools - TFT 游戏的工具集（Phase 17-A 重定位）
//
// 历史：本包工具原本位于 internal/tool/tools/，是 Tool Registry 中央
// 硬编码的 5 个 TFT 工具。Phase 17-A 把它们下放到本包（tft 插件自有
// 工具），消除 internal/tool/tools 对 internal/game/plugins/tft 的
// 反向依赖（违反 CLAUDE.md §15 反例清单）。
//
// 副作用：
//   - internal/tool/tools/init.go 不再注册这 5 个工具（只剩
//     capture_screenshot / get_current_time 两个通用工具）
//   - 这 5 个工具由本包的 init() 通过 tool.RegisterTool 注册
//   - 工具的 GameStateProvider / KnowledgeProvider 注入由本包
//     的 AttachDeps 函数 + Plugin 的 SetupReceiver 实现负责
//   - Agent Core 不知道这 5 个工具的存在，也不通过工具名查找
package tools

import (
	"github.com/lipipi/aurora/internal/tool"
)

// init 注册 5 个 TFT 工具到全局 tool factory 表
//
// 与原 internal/tool/tools/init.go 不同：这里只注册 TFT 工具；
// 通用工具（screenshot / current_time）仍在 internal/tool/tools/init.go
// 中注册。
func init() {
	tool.RegisterTool("get_tft_game_state", NewTFTGameStateTool)
	tool.RegisterTool("get_tft_champion", NewTFTChampionTool)
	tool.RegisterTool("get_tft_item", NewTFTItemTool)
	tool.RegisterTool("get_tft_trait", NewTFTTraitTool)
	tool.RegisterTool("get_tft_augment", NewTFTAugmentTool)
}
