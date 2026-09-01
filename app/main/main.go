package main

import (
	"os"

	"github.com/lipipi/aurora/app/cmd"
	"github.com/lipipi/aurora/internal/config"
	"github.com/lipipi/aurora/internal/log"
	"github.com/lipipi/aurora/internal/lifecycle"
	"github.com/lipipi/aurora/internal/platform"
	"github.com/lipipi/aurora/internal/tray"
)

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	// 初始化日志（先基础初始化，配置加载后再完善）
	log.Init()
	log.Infof("Aurora version: %s, commit: %s, built: %s", version, commit, date)

	// 初始化配置
	if err := config.Init(); err != nil {
		log.Fatalf("Failed to initialize config: %v", err)
	}

	// 完善日志初始化，使用配置的日志级别和数据目录
	cfg := config.Global()
	log.SetLevelFromString(cfg.LogLevel)
	log.InitWithLogDir(config.GetAppDataDir())

	// 初始化平台相关服务
	platform.Init()

	// 初始化系统托盘
	if err := tray.Init(); err != nil {
		log.Errorf("Failed to initialize tray: %v", err)
	}

	// 注册生命周期钩子
	lifecycle.RegisterOnExit(cleanup)

	// 启动应用
	if err := cmd.Execute(); err != nil {
		log.Fatalf("Application error: %v", err)
		os.Exit(1)
	}

	// 启动Wails应用
	// This will be handled by Wails template
}

func cleanup() {
	log.Info("Cleaning up...")
	tray.Shutdown()
	platform.Shutdown()
	config.Shutdown()
	log.Info("Shutdown complete")
}
