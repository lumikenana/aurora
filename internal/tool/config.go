package tool

// Config 工具模块整体配置
type Config struct {
	Enabled []string `json:"enabled"` // 启用的工具列表
}

// DefaultConfig 返回默认工具配置
func DefaultConfig() Config {
	return Config{
		Enabled: []string{
			"capture_screenshot",
			"get_current_time",
			// Phase 11-1：TFT 工具
			"get_tft_game_state",
			"get_tft_champion",
			"get_tft_item",
			"get_tft_trait",
			"get_tft_augment",
		},
	}
}
