package cmd

import (
	"fmt"
	"os"

	"github.com/lipipi/aurora/internal/log"
)

var (
	version = "dev"
)

// SetVersion 设置版本信息
func SetVersion(v string) {
	version = v
}

// Execute 执行命令
func Execute() error {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "-v", "--version", "version":
			fmt.Printf("Aurora version %s\n", version)
			return nil
		case "-h", "--help", "help":
			printHelp()
			return nil
		default:
			fmt.Printf("Unknown command: %s\n", os.Args[1])
			printHelp()
			return nil
		}
	}
	log.Info("Starting Aurora...")
	// 应用启动逻辑会在这里调用
	return nil
}

func printHelp() {
	fmt.Println("Aurora - Windows桌面AI游戏助手")
	fmt.Println()
	fmt.Println("Usage:")
	fmt.Println("  aurora          Start the application")
	fmt.Println("  aurora -v       Show version")
	fmt.Println("  aurora -h       Show help")
	fmt.Println()
}
