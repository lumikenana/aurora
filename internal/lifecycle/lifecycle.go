// Package lifecycle 应用生命周期管理
package lifecycle

import (
	"context"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/lipipi/aurora/internal/log"
)

var (
	exitHooks []func()
	quitCh    chan struct{} // 关闭即代表应用开始退出（中心化退出信号）
	quitOnce  sync.Once     // 保护 quitCh 只被 close 一次
)

// init 初始化退出信号 channel
func init() {
	quitCh = make(chan struct{})
	// 注册信号处理
	go handleSignals()
}

// RegisterOnExit 注册退出钩子
func RegisterOnExit(hook func()) {
	exitHooks = append(exitHooks, hook)
}

// Exit 触发应用退出。
// 顺序：先广播退出信号（让长 goroutine 有机会退出）→ 跑 hooks → 强制 os.Exit。
// 双重兜底：即使 hook 卡住或 goroutine 没退，os.Exit 仍会强制结束进程。
func Exit(code int) {
	CloseQuitCh() // 中心化退出信号：所有监听 QuitCh() 的 goroutine 立即收到
	runExitHooks()
	os.Exit(code)
}

// CloseQuitCh 关闭退出信号 channel（幂等）。
// 由 Exit() 内部调用；外部代码一般无需直接调，调用 Exit() 即可。
func CloseQuitCh() {
	quitOnce.Do(func() {
		close(quitCh)
	})
}

// QuitCh 返回一个 channel，关闭时表示应用正在退出。
// 所有长 goroutine 应 select 此 channel 以实现优雅退出：
//
//	for {
//	    select {
//	    case <-lifecycle.QuitCh():
//	        return
//	    case <-time.After(5 * time.Second):
//	        // do periodic work
//	    }
//	}
//
// 关闭后 channel 可被多次读取（皆为 zero value），不会 panic。
func QuitCh() <-chan struct{} {
	return quitCh
}

// QuitContext 返回一个 ctx，在应用退出时 cancel。
// 用于替代 context.Background() 让下游（HTTP 请求 / 文件下载 / WebSocket 等）可被退出信号取消。
// 返回的 cancel 必须由调用方持有并在适当时机调用，避免内部 watcher goroutine 泄露。
//
// 内部实现：启动一个 watcher goroutine 监听 quitCh，触发时调用 cancel。
// watcher 自身也会监听 ctx.Done()，所以调用方提前 cancel 时 watcher 同步退出。
func QuitContext() (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		select {
		case <-quitCh:
			cancel()
		case <-ctx.Done():
		}
	}()
	return ctx, cancel
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
