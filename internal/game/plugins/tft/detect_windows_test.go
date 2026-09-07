//go:build windows

// TFT 窗口检测链路回归测试（GetActiveGame → DetectActive → Detect →
// detectTFTWindow → FindWindowByClassAndTitle）。
// 修复前周期执行会在 ~2000 次枚举后触发 fatal error: too many callback functions。
package tft

import (
	"sync"
	"testing"
)

// C. 实际链路重复调用：本测试通过即证明真实检测链路可持续运行
func TestDetectTFTWindowRepeated(t *testing.T) {
	for i := 0; i < 3000; i++ {
		_, _ = detectTFTWindow()
	}
}

// C. 实际链路并发调用：配合 -race 验证无数据竞争
func TestDetectTFTWindowConcurrent(t *testing.T) {
	var wg sync.WaitGroup
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 100; i++ {
				_, _ = detectTFTWindow()
			}
		}()
	}
	wg.Wait()
}
