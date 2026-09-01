// Package game 配置
package game

// KnowledgeConfig 游戏知识库配置（ADR-022）
type KnowledgeConfig struct {
	// Enabled 是否启用知识库
	Enabled bool `json:"enabled"`
	// Patch 版本覆盖（空 = 使用最新已下载版本）
	Patch string `json:"patch"`
	// Locale 数据语言（communitydragon locale，如 zh_cn / en_us）
	Locale string `json:"locale"`
	// AutoDownloadOnStart 启动时自动检查并下载最新版本数据
	AutoDownloadOnStart bool `json:"auto_download_on_start"`
	// Source 在线数据源标识（当前仅 communitydragon）
	Source string `json:"source"`
}

// OCRConfig OCR 引擎配置
type OCRConfig struct {
	// Engine OCR 引擎名称（"auto" / "tesseract" / "mock"）
	Engine string `json:"engine"`
	// Language 语言（tesseract 使用，如 "chi_sim+eng"）
	Language string `json:"language"`
}

// Config 游戏模块配置
type Config struct {
	// Enabled 启用的游戏插件列表（插件 ID）
	Enabled []string `json:"enabled"`
	// ScreenshotOnDetect 检测到游戏时是否自动附带屏幕截图给 LLM
	ScreenshotOnDetect bool `json:"screenshot_on_detect"`
	// Knowledge 游戏知识库配置
	Knowledge KnowledgeConfig `json:"knowledge"`
	// OCR OCR 引擎配置
	OCR OCRConfig `json:"ocr"`
}

// DefaultConfig 默认配置
func DefaultConfig() Config {
	return Config{
		Enabled:            []string{"tft"},
		ScreenshotOnDetect: true,
		Knowledge: KnowledgeConfig{
			Enabled:             true,
			Patch:               "",
			Locale:              "zh_cn",
			AutoDownloadOnStart: true,
			Source:              "communitydragon",
		},
		OCR: OCRConfig{
			Engine:   "auto",
			Language: "chi_sim+eng",
		},
	}
}
