package input

import (
	"errors"
	"sync"
	"testing"
	"time"
)

// recordingDriver 记录 Register/Unregister 调用的 mock driver。
// 模拟 Wails v3 GlobalShortcut：可能返回 error。
type recordingDriver struct {
	mu            sync.Mutex
	registered    map[string]func() // 已注册 combo → cb
	unregistered  []string          // Unregister 调用顺序
	unregisterErr error             // 模拟 driver 报错
}

func newRecordingDriver() *recordingDriver {
	return &recordingDriver{registered: map[string]func(){}}
}

func (d *recordingDriver) Register(combo string, cb func()) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.registered[combo] = cb
	return nil
}

func (d *recordingDriver) Unregister(combo string) error {
	d.mu.Lock()
	d.unregistered = append(d.unregistered, combo)
	delete(d.registered, combo)
	d.mu.Unlock()
	return d.unregisterErr
}

func (d *recordingDriver) snapshot() (registered int, unregistered []string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	return len(d.registered), append([]string(nil), d.unregistered...)
}

func newTestShortcutManager() (*ShortcutManager, *recordingDriver) {
	drv := newRecordingDriver()
	m := NewShortcutManager(drv)
	// 给所有 action 注入 noop handler，让 ApplyAll 能成功注册
	handlers := make(map[ShortcutAction]func(), len(AllShortcutActions))
	for _, a := range AllShortcutActions {
		handlers[a] = func() {}
	}
	m.SetHandlers(handlers)
	return m, drv
}

// TestUnregisterAll_Empty 空 manager 调用是 no-op
func TestUnregisterAll_Empty(t *testing.T) {
	m, drv := newTestShortcutManager()
	m.UnregisterAll()
	_, unreg := drv.snapshot()
	if len(unreg) != 0 {
		t.Errorf("empty manager should not call Unregister, got %d calls", len(unreg))
	}
}

// TestUnregisterAll_AllRegistered 注册全部 action → UnregisterAll 应全部注销
func TestUnregisterAll_AllRegistered(t *testing.T) {
	m, drv := newTestShortcutManager()
	m.ApplyAll(DefaultShortcuts())

	regCount, _ := drv.snapshot()
	if regCount != len(AllShortcutActions) {
		t.Fatalf("setup: expected %d registered, got %d", len(AllShortcutActions), regCount)
	}

	m.UnregisterAll()

	// 1) driver.Unregister 对每个已注册 combo 调用 1 次
	_, unreg := drv.snapshot()
	if len(unreg) != len(AllShortcutActions) {
		t.Errorf("expected %d Unregister calls, got %d", len(AllShortcutActions), len(unreg))
	}

	// 2) manager.combos map 已清空
	if got := m.Combos(); len(got) != 0 {
		t.Errorf("after UnregisterAll, Combos() should be empty, got %d entries", len(got))
	}
}

// TestUnregisterAll_Idempotent 重复调用是安全的（第二次 no-op）
func TestUnregisterAll_Idempotent(t *testing.T) {
	m, drv := newTestShortcutManager()
	m.ApplyAll(DefaultShortcuts())

	m.UnregisterAll()
	_, firstUnreg := drv.snapshot()

	m.UnregisterAll()
	_, secondUnreg := drv.snapshot()

	if len(firstUnreg) != len(AllShortcutActions) {
		t.Errorf("first call expected %d, got %d", len(AllShortcutActions), len(firstUnreg))
	}
	if len(secondUnreg) != len(firstUnreg) {
		t.Errorf("second call should be no-op, got %d additional calls", len(secondUnreg)-len(firstUnreg))
	}
}

// TestUnregisterAll_PreservesDriverError driver 返回 error 也不 panic，
// combos map 仍然清空（关键：清空发生在 Unregister 调用前，避免状态不一致）
func TestUnregisterAll_PreservesDriverError(t *testing.T) {
	m, drv := newTestShortcutManager()
	m.ApplyAll(DefaultShortcuts())
	drv.unregisterErr = errors.New("simulated driver failure")

	// 不应 panic
	m.UnregisterAll()

	// map 必须清空 — 否则下一次 ApplyAll 会拿到 stale 数据
	if got := m.Combos(); len(got) != 0 {
		t.Errorf("after UnregisterAll (with driver error), Combos() should still be empty, got %d", len(got))
	}
}

// probeDriver 包装 recordingDriver，在 Unregister 期间回调 probe 函数。
// 用于检测 manager 是否在持锁状态下调用 driver（持锁则 probe 死锁，测试超时）。
type probeDriver struct {
	recordingDriver
	probe func()
}

func (d *probeDriver) Unregister(combo string) error {
	d.mu.Lock()
	d.unregistered = append(d.unregistered, combo)
	delete(d.registered, combo)
	d.mu.Unlock()
	if d.probe != nil {
		d.probe()
	}
	return nil
}

// TestUnregisterAll_DoesNotHoldLockWhileCallingDriver
// 关键不变量：map 清空在持锁内完成，driver 调用在锁外。
// 验证：driver.Unregister 期间如果尝试读 manager.combos 不会死锁。
// 失败模式：若实现错误地"持锁调 driver"，这里会 deadlock（测试 1s 超时）。
func TestUnregisterAll_DoesNotHoldLockWhileCallingDriver(t *testing.T) {
	drv := &probeDriver{
		recordingDriver: recordingDriver{registered: map[string]func(){}},
	}
	m := NewShortcutManager(drv)
	handlers := make(map[ShortcutAction]func(), len(AllShortcutActions))
	for _, a := range AllShortcutActions {
		handlers[a] = func() {}
	}
	m.SetHandlers(handlers)
	m.ApplyAll(DefaultShortcuts())

	// probe：在 driver.Unregister 期间尝试读 m.Combos()。
	// 若 manager 持锁调 driver → probe 阻塞 → 测试 1s 超时 → 失败。
	drv.probe = func() {
		_ = m.Combos()
	}

	done := make(chan struct{})
	go func() {
		m.UnregisterAll()
		close(done)
	}()

	select {
	case <-done:
		// 成功：UnregisterAll 没有持锁调 driver
	case <-time.After(1 * time.Second):
		t.Fatal("UnregisterAll appears to hold lock while calling driver (deadlock detected via probe)")
	}
}
