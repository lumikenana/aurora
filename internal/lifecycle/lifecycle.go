// Package lifecycle 应用生命周期管理
package lifecycle

import (
	"os"
	"os/signal"
	"syscall"

	"github.com/lipipi/aurora/internal/log"
)

var (
	exitHooks []func()
)

func init() {
	// 注册信号处理
	go handleSignals()
}

// RegisterOnExit 注册退出钩子
func RegisterOnExit(hook func()) {
	exitHooks = append(exitHooks, hook)
}

// Exit 触发应用退出
func Exit(code int) {
	runExitHooks()
	os.Exit(code)
}

// 运行所有退出钩子
func runExitHooks() {
	log.Debug("Running exit hooks...")
	// 逆序执行钩子，后注册先执行（类似栈）
	for i := len(exitHooks) - 1; i >= 0; i-- {
		func() {
			defer func() {
				if r := recover(); r != nil {
					log.Errorf("Exit hook panicked: %v", r)
				}
			}()
			exitHooks[i]()
		}()
	}
	log.Debug("All exit hooks completed")
}

// 处理信号
func handleSignals() {
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM, syscall.SIGINT)

	sig := <-c
	log.Infof("Received signal: %v", sig)

	Exit(0)
}
