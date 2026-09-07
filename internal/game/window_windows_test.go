//go:build windows

// 窗口枚举回归测试：修复 enumWindows 每次调用 syscall.NewCallback
// 耗尽 runtime callback 槽位（上限 2000）导致 "fatal error: too many callback functions"。
//
// 修复前：TestEnumWindowsRepeatedCalls 运行约 1000 次枚举后进程直接 fatal。
// 修复后：callback 固化为包级单例，枚举上下文经 lParam 传入，可无限复用。
package game

import (
	"strings"
	"sync"
	"testing"
)

// lParam 通路冒烟：桌面上必然存在可见窗口，宽松匹配必须命中。
// 同时验证经 lParam 传入的 match/found 上下文确实生效（而非静默返回未找到）。
func TestEnumWindowsFindsVisibleWindow(t *testing.T) {
	w, ok := enumWindows(func(wi WindowInfo) bool {
		return wi.Class != ""
	})
	if !ok {
		t.Fatal("枚举可见窗口未命中（lParam 上下文通路可能失效）")
	}
	if w.HWND == 0 || w.Class == "" {
		t.Fatalf("返回了无效窗口: %+v", w)
	}
}

// A. 重复调用：连续大量枚举，修复前 2000 次左右即 fatal，
// 此测试本身通过即证明没有新 callback 被持续注册。
func TestEnumWindowsRepeatedCalls(t *testing.T) {
	for i := 0; i < 3000; i++ {
		_, _ = FindWindowByClassAndTitle("League of Legends", "RiotWindowClass")
		_, _ = FindWindowByClass("Riot Window")
	}
}

// B. 并发调用：多个 goroutine 同时枚举，
// 验证不 panic、结果与自身查询条件一致（context 串扰即会命中 Errorf），
// 需配合 -race 运行。
func TestEnumWindowsConcurrentNoCrossTalk(t *testing.T) {
	const (
		goroutines = 16
		iters      = 50
	)
	// 桌面上不存在的类名/标题：预期未找到；若因串扰返回，
	// 结果必须仍与本次查询条件一致，否则报错。
	class := "AuroraTestNoSuchClass"
	var wg sync.WaitGroup
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for i := 0; i < iters; i++ {
				w, ok := FindWindowByClassAndTitle("AuroraTestTitle", class)
				if ok && (!strings.EqualFold(w.Class, class) ||
					!strings.Contains(strings.ToLower(w.Title), "auroratesttitle")) {
					t.Errorf("goroutine %d: 结果与查询条件不匹配（context 串扰）: %+v", g, w)
					return
				}
			}
		}(g)
	}
	wg.Wait()
}
