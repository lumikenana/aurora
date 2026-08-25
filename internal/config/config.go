package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"

	"github.com/lipipi/game-ai/internal/errors"
	"github.com/lipipi/game-ai/internal/input"
	"github.com/lipipi/game-ai/internal/log"
	"github.com/lipipi/game-ai/internal/llm"
	"github.com/lipipi/game-ai/internal/screenshot"
	"github.com/lipipi/game-ai/internal/tool"
)

var (
	globalConfig *Config
)

// Config 全局配置
type Config struct {
	// 日志级别: debug, info, warn, error
	LogLevel string `json:"log_level"`
	// 是否开启开机启动
	AutoStart bool `json:"auto_start"`
	// 启动时最小化到托盘
	StartMinimized bool `json:"start_minimized"`
	// 主题: light, dark, auto
	Theme string `json:"theme"`
	// 窗口宽度
	WindowWidth int `json:"window_width"`
	// 窗口高度
	WindowHeight int `json:"window_height"`
	// 是否记住窗口位置
	RememberWindowPos bool `json:"remember_window_pos"`
	// 窗口X坐标
	WindowX int `json:"window_x"`
	// 窗口Y坐标
	WindowY int `json:"window_y"`
	// 快捷键配置
	Hotkey input.HotkeyConfig `json:"hotkey"`
	// LLM 配置
	LLM llm.Config `json:"llm"`
	// 截图配置
	Screenshot screenshot.Config `json:"screenshot"`
	// 工具配置
	Tool tool.Config `json:"tool"`
}

// DefaultConfig 返回默认配置
func DefaultConfig() *Config {
	return &Config{
		LogLevel:          "info",
		AutoStart:         false,
		StartMinimized:    false,
		Theme:             "dark",
		WindowWidth:       800,
		WindowHeight:      600,
		RememberWindowPos: false,
		WindowX:           0,
		WindowY:           0,
		LLM:               llm.DefaultConfig(),
		Screenshot:         screenshot.DefaultConfig(),
		Tool:              tool.DefaultConfig(),
	}
}

// Init 初始化配置
func Init() errors.AppError {
	log.Debug("Initializing configuration...")

	cfgPath := getConfigPath()
	var cfg *Config

	// 检查配置文件是否存在
	if _, err := os.Stat(cfgPath); os.IsNotExist(err) {
		// 使用默认配置
		cfg = DefaultConfig()
		if err := saveConfig(cfg); err != nil {
			return errors.Wrap(errors.CodeConfigError, "Failed to save default config", err)
		}
	} else {
		// 读取现有配置
		data, err := os.ReadFile(cfgPath)
		if err != nil {
			return errors.Wrap(errors.CodeConfigError, "Failed to read config file", err)
		}
		cfg = DefaultConfig()
		if err := json.Unmarshal(data, cfg); err != nil {
			log.Warnf("Failed to parse config file, using defaults: %v", err)
			cfg = DefaultConfig()
		}
	}

	globalConfig = cfg
	log.Debug("Configuration loaded successfully")
	return nil
}

// Shutdown 关闭配置
func Shutdown() {
	if globalConfig != nil {
		_ = Save()
	}
}

// Global 获取全局配置
func Global() *Config {
	if globalConfig == nil {
		return DefaultConfig()
	}
	return globalConfig
}

// Save 保存配置到文件
func Save() errors.AppError {
	if globalConfig == nil {
		return errors.New(errors.CodeConfigError, "Config not initialized")
	}
	return saveConfig(globalConfig)
}

// saveConfig 保存配置
func saveConfig(cfg *Config) errors.AppError {
	cfgPath := getConfigPath()
	dir := filepath.Dir(cfgPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return errors.Wrap(errors.CodeConfigError, "Failed to create config directory", err)
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return errors.Wrap(errors.CodeConfigError, "Failed to marshal config", err)
	}

	if err := os.WriteFile(cfgPath, data, 0644); err != nil {
		return errors.Wrap(errors.CodeConfigError, "Failed to write config file", err)
	}

	return nil
}

// GetConfigPath 获取配置文件路径
func getConfigPath() string {
	dir := GetAppDataDir()
	return filepath.Join(dir, "config.json")
}

// GetAppDataDir 获取应用数据目录
func GetAppDataDir() string {
	switch runtime.GOOS {
	case "windows":
		// Windows: %APPDATA%\GameAI
		if appData := os.Getenv("APPDATA"); appData != "" {
			return filepath.Join(appData, "GameAI")
		}
		home, _ := os.UserHomeDir()
		return filepath.Join(home, "AppData", "Roaming", "GameAI")
	case "darwin":
		// macOS: ~/Library/Application Support/GameAI
		home, _ := os.UserHomeDir()
		return filepath.Join(home, "Library", "Application Support", "GameAI")
	default:
		// Linux/Unix: ~/.local/share/gameai
		if xdgData := os.Getenv("XDG_DATA_HOME"); xdgData != "" {
			return filepath.Join(xdgData, "gameai")
		}
		home, _ := os.UserHomeDir()
		return filepath.Join(home, ".local", "share", "gameai")
	}
}

// GetAppDir 获取应用程序目录
func GetAppDir() string {
	exePath, err := os.Executable()
	if err != nil {
		return "."
	}
	return filepath.Dir(exePath)
}
