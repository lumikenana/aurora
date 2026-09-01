package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"

	"github.com/lipipi/aurora/internal/audio"
	"github.com/lipipi/aurora/internal/errors"
	"github.com/lipipi/aurora/internal/game"
	"github.com/lipipi/aurora/internal/input"
	"github.com/lipipi/aurora/internal/log"
	"github.com/lipipi/aurora/internal/llm"
	"github.com/lipipi/aurora/internal/screenshot"
	"github.com/lipipi/aurora/internal/speech"
	"github.com/lipipi/aurora/internal/tool"
	"github.com/lipipi/aurora/internal/tts"
	"github.com/lipipi/aurora/internal/wakeword"
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
	// 游戏插件配置
	Game game.Config `json:"game"`
	// 语音识别配置
	Speech speech.Config `json:"speech"`
	// 语音合成配置
	TTS tts.Config `json:"tts"`
	// 音频采集配置
	Audio audio.Config `json:"audio"`
	// 唤醒词检测配置
	WakeWord wakeword.Config `json:"wakeword"`
	// 当前语音模式: "ptt" / "wakeword" / "continuous"
	VoiceMode string `json:"voice_mode"`
	// 连续模式无输入自动退出秒数
	ContinuousTimeoutSeconds int `json:"continuous_timeout_seconds"`
	// 连续模式退出关键词
	ContinuousExitCommands []string `json:"continuous_exit_commands"`
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
		Hotkey:            input.DefaultHotkeyConfig(),
		LLM:               llm.DefaultConfig(),
		Screenshot:         screenshot.DefaultConfig(),
		Tool:              tool.DefaultConfig(),
		Game:              game.DefaultConfig(),
		Speech:            speech.DefaultConfig(),
		TTS:               tts.DefaultConfig(),
		Audio:             audio.DefaultConfig(),
		WakeWord:          wakeword.DefaultConfig(),
		VoiceMode:         "wakeword",
		ContinuousTimeoutSeconds: 30,
		ContinuousExitCommands:   []string{"结束对话", "停止", "退出对话", "exit", "stop"},
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

// legacyAppDataDir 返回旧版 GameAI 数据目录路径。
// 用于一次性迁移到 Aurora 目录；迁移完成后不再被调用。
func legacyAppDataDir() string {
	switch runtime.GOOS {
	case "windows":
		if appData := os.Getenv("APPDATA"); appData != "" {
			return filepath.Join(appData, "GameAI")
		}
		home, _ := os.UserHomeDir()
		return filepath.Join(home, "AppData", "Roaming", "GameAI")
	case "darwin":
		home, _ := os.UserHomeDir()
		return filepath.Join(home, "Library", "Application Support", "GameAI")
	default:
		if xdgData := os.Getenv("XDG_DATA_HOME"); xdgData != "" {
			return filepath.Join(xdgData, "gameai")
		}
		home, _ := os.UserHomeDir()
		return filepath.Join(home, ".local", "share", "gameai")
	}
}

// migrateLegacyAppDataDir 尝试一次性将旧 GameAI 数据目录重命名为 Aurora 目录。
// 任何失败都保留旧目录完整，并退回使用旧路径，确保用户数据不丢失。
// 迁移成功后后续调用直接命中新目录，不会重复迁移。
func migrateLegacyAppDataDir(newDir string) string {
	legacy := legacyAppDataDir()
	if legacy == newDir {
		return newDir
	}
	// 新目录已存在：直接使用，不触碰旧目录（避免覆盖用户手动创建的内容）。
	if _, err := os.Stat(newDir); err == nil {
		return newDir
	}
	// 旧目录不存在：全新安装，直接使用新目录。
	if _, err := os.Stat(legacy); err != nil {
		return newDir
	}
	// 尝试 rename；跨卷 / 权限失败时退回旧路径。
	log.Info("Migrating legacy data directory: %s -> %s", legacy, newDir)
	if err := os.Rename(legacy, newDir); err != nil {
		log.Warn("Failed to migrate legacy data directory (%s -> %s): %v; continuing with legacy path", legacy, newDir, err)
		return legacy
	}
	log.Info("Legacy data directory migrated successfully: %s", newDir)
	return newDir
}

// GetAppDataDir 获取应用数据目录。
// 首次运行时若检测到旧 GameAI 目录会自动一次性迁移到 Aurora；
// 迁移失败则继续使用旧目录，保证用户数据（config.json / knowledge.db /
// 日志 / 唤醒模型 等）不丢失。
func GetAppDataDir() string {
	var newDir string
	switch runtime.GOOS {
	case "windows":
		// Windows: %APPDATA%\Aurora
		if appData := os.Getenv("APPDATA"); appData != "" {
			newDir = filepath.Join(appData, "Aurora")
		} else {
			home, _ := os.UserHomeDir()
			newDir = filepath.Join(home, "AppData", "Roaming", "Aurora")
		}
	case "darwin":
		// macOS: ~/Library/Application Support/Aurora
		home, _ := os.UserHomeDir()
		newDir = filepath.Join(home, "Library", "Application Support", "Aurora")
	default:
		// Linux/Unix: ~/.local/share/aurora
		if xdgData := os.Getenv("XDG_DATA_HOME"); xdgData != "" {
			newDir = filepath.Join(xdgData, "aurora")
		} else {
			home, _ := os.UserHomeDir()
			newDir = filepath.Join(home, ".local", "share", "aurora")
		}
	}
	return migrateLegacyAppDataDir(newDir)
}

// GetAppDir 获取应用程序目录
func GetAppDir() string {
	exePath, err := os.Executable()
	if err != nil {
		return "."
	}
	return filepath.Dir(exePath)
}
