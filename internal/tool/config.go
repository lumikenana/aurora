package tool

// Config 工具模块整体配置
type Config struct {
	Enabled []string `json:"enabled"` // 启用的工具列表
}

// DefaultConfig 返回默认工具配置
// Phase 18-B：移除游戏特定工具名，改为通用工具。
// 游戏特定工具由各插件在 init() 中自注册，并自行管理启用状态。
func DefaultConfig() Config {
	return Config{
		Enabled: []string{
			"capture_screenshot",
			"get_current_time",
			// 游戏特定工具（如 get_tft_*）不再在此硬编码。
			// 各 GamePlugin 通过 ToolContributor 接口注册自己的工具，
			// 默认启用所有已注册工具（见 tool.Registry.EnabledTools）。
		},
	}
}
