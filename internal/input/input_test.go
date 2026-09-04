package input

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestSetCallback_ReplacesPreviousCallback(t *testing.T) {
	m := &InputManager{}

	var first, second atomic.Int32
	m.SetCallback(func(input UserInput) { first.Add(1) })
	m.SetCallback(func(input UserInput) { second.Add(1) })

	m.NotifyInput(UserInput{Text: "hello", Source: SourceText})

	assert.Equal(t, int32(0), first.Load(), "old callback should not fire")
	assert.Equal(t, int32(1), second.Load(), "only latest callback fires")
}

// TestSetCallback_SameCallbackReSet_Idempotent 验证同一回调重复 Set 只触发一次
// （对应 SaveLLMConfig 多次保存配置的场景）。
func TestSetCallback_SameCallbackReSet_Idempotent(t *testing.T) {
	m := &InputManager{}

	var calls atomic.Int32
	cb := func(input UserInput) { calls.Add(1) }
	m.SetCallback(cb)
	m.SetCallback(cb)
	m.SetCallback(cb)

	m.NotifyInput(UserInput{Text: "hello", Source: SourceText})

	assert.Equal(t, int32(1), calls.Load(), "repeated Set of same callback must not accumulate")
}

// TestNotifyInput_NoCallback_NoPanic 验证未注册回调时 NotifyInput 不 panic。
func TestNotifyInput_NoCallback_NoPanic(t *testing.T) {
	m := &InputManager{}
	assert.NotPanics(t, func() {
		m.NotifyInput(UserInput{Text: "hello", Source: SourceText})
	})
}

// TestSetCallback_ConcurrentWithNotifyInput 验证 SetCallback 与 NotifyInput
// 并发执行无数据竞争（NotifyInput 在热键/PTT 消息线程，SetCallback 在
// SaveLLMConfig 的 binding goroutine —— 修复前这两个访问是裸的）。
// 数据竞争需 go test -race 才能真正检出。
func TestSetCallback_ConcurrentWithNotifyInput(t *testing.T) {
	m := &InputManager{}

	var calls atomic.Int32
	stop := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		writer := func(input UserInput) { calls.Add(1) }
		for {
			select {
			case <-stop:
				return
			default:
				m.SetCallback(writer)
			}
		}
	}()
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
				m.NotifyInput(UserInput{Text: "concurrent", Source: SourceText})
			}
		}
	}()
	time.Sleep(50 * time.Millisecond)
	close(stop)
	wg.Wait()
}
