package tool

// Config 工具模块整体配置
type Config struct {
	Enabled []string `json:"enabled"` // 启用的工具列表
}

// DefaultConfig 返回默认工具配置
func DefaultConfig() Config {
	return Config{
		Enabled: []string{"capture_screenshot", "get_current_time"},
	}
}
