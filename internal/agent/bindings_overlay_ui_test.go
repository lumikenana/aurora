package agent

// Overlay Edit Mode / 几何持久化的单元测试（UI 重构 2026-09-04：Native
// Window → Go-side 300ms debounce → 落盘 路径覆盖）。
//
// 覆盖：
//   - PersistOverlayGeometry 的 clamp 行为（max / min / 0,0 语义）
//   - changed=false 短路（不 Save 不广播）
//   - SchedulePersistOverlayGeometry 静默窗口合并 + 边界 300ms 触发
//   - FlushPendingOverlayGeometry 立即落盘 + 清空 pending + 无 pending no-op
//   - SetOverlayLockedAndBroadcast 进程内状态 + 广播 overlayLockChanged
//
// 不覆盖：Wails native 拖拽 / resize（OS + WebView2 行为，需 Windows E2E）；
// wailsruntime.SetOverlayResizable / SetIgnoreMouseEvents 在测试环境
// OverlayWindow == nil 时是 no-op，由 wailsruntime 自己的测试或 E2E 覆盖。

import (
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/lipipi/aurora/internal/config"
	"github.com/lipipi/aurora/internal/wailsruntime"
)

// TestMain 引导 config 到临时目录，让 config.Global() 返回稳定的全局指针
// （默认行为：globalConfig == nil → 每次返回新 DefaultConfig，跨调用
// 不可观察状态变化，无法测试 cfg 写入副作用）。
//
// 用 t.TempDir() 替代真实用户配置目录，避免污染用户数据。
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "aurora-agent-test-")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(dir)

	// 覆盖 OS-specific config 路径查找
	switch runtime.GOOS {
	case "windows":
		os.Setenv("APPDATA", dir)
	default:
		// darwin + linux 都通过 HOME + XDG_DATA_HOME 解析
		os.Setenv("HOME", dir)
		os.Setenv("XDG_DATA_HOME", filepath.Join(dir, ".local", "share"))
	}

	if err := config.Init(); err != nil {
		panic("config init failed: " + err.Error())
	}

	code := m.Run()
	os.Exit(code)
}

// resetOverlayDebounce 把进程内 debounce 状态清零（供 test 间隔离用）。
// 同时清掉 timer 防止它在 test 结束后才 fire 污染下一轮。
func resetOverlayDebounce(t *testing.T) {
	t.Helper()
	overlayGeomMu.Lock()
	if overlayGeomTimer != nil {
		overlayGeomTimer.Stop()
		overlayGeomTimer = nil
	}
	overlayGeomPending = false
	overlayGeomX, overlayGeomY = 0, 0
	overlayGeomW, overlayGeomH = 0, 0
	overlayGeomMu.Unlock()
}

// snapshotOverlayConfig 快照 / 还原 cfg.Overlay 的几何字段。PersistOverlayGeometry
// 会调 config.Save() 写 config 目录（test env 走 temp dir，但 t.Cleanup
// 仍应恢复成快照值，保持 test 间隔离）。
func snapshotOverlayConfig(t *testing.T) {
	t.Helper()
	cfg := config.Global()
	ox, oy, ow, oh := cfg.Overlay.X, cfg.Overlay.Y, cfg.Overlay.Width, cfg.Overlay.Height
	t.Cleanup(func() {
		cfg.Overlay.X = ox
		cfg.Overlay.Y = oy
		cfg.Overlay.Width = ow
		cfg.Overlay.Height = oh
		if err := config.Save(); err != nil {
			t.Logf("cleanup: config.Save failed (non-fatal): %v", err)
		}
	})
}

// ---- PersistOverlayGeometry ----

func TestPersistOverlayGeometry_ClampsToMax(t *testing.T) {
	snapshotOverlayConfig(t)
	resetOverlayDebounce(t)

	cfg := config.Global()
	cfg.Overlay.X = 100
	cfg.Overlay.Y = 100
	cfg.Overlay.Width = 0
	cfg.Overlay.Height = 0

	// Width 2000 超过 max 960 → clamp 到 960
	PersistOverlayGeometry(100, 100, 2000, 500)
	if cfg.Overlay.Width != overlayMaxWidth {
		t.Errorf("expected Width=%d (clamped from 2000), got %d", overlayMaxWidth, cfg.Overlay.Width)
	}
	if cfg.Overlay.Height != 500 {
		t.Errorf("expected Height=500 (unchanged), got %d", cfg.Overlay.Height)
	}
}

func TestPersistOverlayGeometry_ClampsToMin(t *testing.T) {
	snapshotOverlayConfig(t)
	resetOverlayDebounce(t)

	cfg := config.Global()
	cfg.Overlay.X = 0
	cfg.Overlay.Y = 0
	cfg.Overlay.Width = 0
	cfg.Overlay.Height = 0

	// Width 100 低于 min 240 → clamp 到 240
	PersistOverlayGeometry(0, 0, 100, 300)
	if cfg.Overlay.Width != overlayMinWidth {
		t.Errorf("expected Width=%d (clamped from 100), got %d", overlayMinWidth, cfg.Overlay.Width)
	}
}

func TestPersistOverlayGeometry_ZeroMeansDefault(t *testing.T) {
	snapshotOverlayConfig(t)
	resetOverlayDebounce(t)

	cfg := config.Global()
	cfg.Overlay.X = 0
	cfg.Overlay.Y = 0
	cfg.Overlay.Width = 0
	cfg.Overlay.Height = 0

	// (0, 0) 语义："未自定义" → 走默认位置（wailsruntime.setDefaultOverlayPosition
	// 硬编码 bottom-right + 16px margin），width/height 也归零
	// （CommitOverlayGeometry 的约定，保持一致；2026-09-07 Anchor 移除后改名为 Default）
	PersistOverlayGeometry(0, 0, 0, 0)
	if cfg.Overlay.X != 0 || cfg.Overlay.Y != 0 {
		t.Errorf("expected X/Y=0,0, got (%d, %d)", cfg.Overlay.X, cfg.Overlay.Y)
	}
	if cfg.Overlay.Width != 0 || cfg.Overlay.Height != 0 {
		t.Errorf("expected W/H=0,0, got (%d, %d)", cfg.Overlay.Width, cfg.Overlay.Height)
	}
}

func TestPersistOverlayGeometry_NoChangeSkipsSave(t *testing.T) {
	snapshotOverlayConfig(t)
	resetOverlayDebounce(t)

	cfg := config.Global()
	// 已知初值（0,0,0,0 + 一个固定 (100,200,360,420)）
	cfg.Overlay.X = 100
	cfg.Overlay.Y = 200
	cfg.Overlay.Width = 360
	cfg.Overlay.Height = 420

	// 相同值 → changed=false → 不 Save（无可观察副作用，
	// 通过"不 panic + cfg 不变"间接验证）
	PersistOverlayGeometry(100, 200, 360, 420)
	if cfg.Overlay.X != 100 || cfg.Overlay.Y != 200 ||
		cfg.Overlay.Width != 360 || cfg.Overlay.Height != 420 {
		t.Errorf("expected cfg unchanged on no-op, got (%d,%d,%d,%d)",
			cfg.Overlay.X, cfg.Overlay.Y, cfg.Overlay.Width, cfg.Overlay.Height)
	}
}

func TestPersistOverlayGeometry_DetectsChange(t *testing.T) {
	snapshotOverlayConfig(t)
	resetOverlayDebounce(t)

	cfg := config.Global()
	cfg.Overlay.X = 0
	cfg.Overlay.Y = 0
	cfg.Overlay.Width = 360
	cfg.Overlay.Height = 420

	// Width 变化 → 应该写 cfg
	PersistOverlayGeometry(0, 0, 480, 420)
	if cfg.Overlay.Width != 480 {
		t.Errorf("expected Width=480, got %d", cfg.Overlay.Width)
	}
}

// ---- Schedule / Flush debounce ----

func TestSchedulePersistOverlayGeometry_Debounces(t *testing.T) {
	snapshotOverlayConfig(t)
	resetOverlayDebounce(t)

	cfg := config.Global()
	cfg.Overlay.X = 0
	cfg.Overlay.Y = 0
	cfg.Overlay.Width = 0
	cfg.Overlay.Height = 0

	// 连续多次 schedule（模拟拖拽过程）：在 300ms 静默窗口内合并为最后一次
	SchedulePersistOverlayGeometry(100, 100, 360, 420)
	SchedulePersistOverlayGeometry(200, 200, 400, 450)
	SchedulePersistOverlayGeometry(300, 300, 480, 500)

	// 立刻读：值未写入 cfg（debounce 还没 fire）
	if cfg.Overlay.X != 0 {
		t.Errorf("expected immediate-read X=0, got %d", cfg.Overlay.X)
	}

	// 等待超过 300ms（debounce window）
	time.Sleep(overlayGeometryDebounce + 100*time.Millisecond)

	// 最后一次值应已写入
	if cfg.Overlay.X != 300 || cfg.Overlay.Y != 300 ||
		cfg.Overlay.Width != 480 || cfg.Overlay.Height != 500 {
		t.Errorf("expected (300,300,480,500), got (%d,%d,%d,%d)",
			cfg.Overlay.X, cfg.Overlay.Y, cfg.Overlay.Width, cfg.Overlay.Height)
	}
}

func TestFlushPendingOverlayGeometry_ImmediateWrite(t *testing.T) {
	snapshotOverlayConfig(t)
	resetOverlayDebounce(t)

	cfg := config.Global()
	cfg.Overlay.X = 0
	cfg.Overlay.Y = 0
	cfg.Overlay.Width = 0
	cfg.Overlay.Height = 0

	// schedule 但不等待 debounce
	SchedulePersistOverlayGeometry(50, 60, 360, 420)

	// 立即 flush（WindowClosing / WindowHide 路径）
	wrote := FlushPendingOverlayGeometry()
	if !wrote {
		t.Errorf("expected FlushPendingOverlayGeometry=true when pending exists")
	}
	if cfg.Overlay.X != 50 || cfg.Overlay.Y != 60 ||
		cfg.Overlay.Width != 360 || cfg.Overlay.Height != 420 {
		t.Errorf("expected (50,60,360,420) after flush, got (%d,%d,%d,%d)",
			cfg.Overlay.X, cfg.Overlay.Y, cfg.Overlay.Width, cfg.Overlay.Height)
	}

	// 等待原本 debounce 时间，确认 timer 已被 stop（不会二次写入）
	cfg.Overlay.X = 0
	time.Sleep(overlayGeometryDebounce + 100*time.Millisecond)
	if cfg.Overlay.X != 0 {
		t.Errorf("expected X=0 after flush+timer-stop, got %d", cfg.Overlay.X)
	}
}

func TestFlushPendingOverlayGeometry_NoPendingIsNoop(t *testing.T) {
	snapshotOverlayConfig(t)
	resetOverlayDebounce(t)

	cfg := config.Global()
	cfg.Overlay.X = 999
	cfg.Overlay.Y = 0
	cfg.Overlay.Width = 0
	cfg.Overlay.Height = 0

	// 无 pending → flush 返回 false 且不动 cfg
	wrote := FlushPendingOverlayGeometry()
	if wrote {
		t.Errorf("expected FlushPendingOverlayGeometry=false when nothing pending")
	}
	if cfg.Overlay.X != 999 {
		t.Errorf("expected X=999 unchanged, got %d", cfg.Overlay.X)
	}
}

// 并发 stress：模拟 Wails event 在多个 goroutine 同时 fire
func TestSchedulePersistOverlayGeometry_ConcurrentSafe(t *testing.T) {
	snapshotOverlayConfig(t)
	resetOverlayDebounce(t)
	// 防止前序 suppress 测试残留标志影响本 test
	waitSuppressClear(t, 2*time.Second)

	cfg := config.Global()
	cfg.Overlay.X = 0
	cfg.Overlay.Y = 0
	cfg.Overlay.Width = 0
	cfg.Overlay.Height = 0

	const N = 50
	var wg sync.WaitGroup
	var done atomic.Bool
	for i := 0; i < N; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			for !done.Load() {
				SchedulePersistOverlayGeometry(i, i, 360+i, 420+i)
			}
		}(i)
	}

	time.Sleep(overlayGeometryDebounce + 50*time.Millisecond)
	done.Store(true)
	wg.Wait()

	// 最后一次：X 必须在 [0, N) 范围（任一 goroutine 的最后写入）
	overlayGeomMu.Lock()
	lastX := overlayGeomX
	overlayGeomMu.Unlock()
	if lastX < 0 || lastX >= N {
		t.Errorf("expected lastX in [0,%d), got %d", N, lastX)
	}
}

// waitSuppressClear 轮询等待 wailsruntime 的 suppress 标志复位。
// 用于隔离设置过 suppress 的 test，避免 500ms 屏蔽期污染后续 test。
func waitSuppressClear(t *testing.T, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if !wailsruntime.IsGeometryPersistSuppressed() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	if wailsruntime.IsGeometryPersistSuppressed() {
		t.Fatal("timed out waiting for geometry persist suppress to clear")
	}
}

// ---- SetOverlayLockedAndBroadcast ----

func TestSetOverlayLockedAndBroadcast_UpdatesState(t *testing.T) {
	// wailsruntime.OverlayWindow 在 test env 是 nil，native helper
	// 内部判 nil → no-op，状态切换本身仍可验证
	setOverlayLockedState(true)
	if !getOverlayLockedState() {
		t.Fatal("precondition: expected locked=true")
	}

	SetOverlayLockedAndBroadcast(false)
	if getOverlayLockedState() {
		t.Errorf("expected locked=false after SetOverlayLockedAndBroadcast(false)")
	}

	SetOverlayLockedAndBroadcast(true)
	if !getOverlayLockedState() {
		t.Errorf("expected locked=true after SetOverlayLockedAndBroadcast(true)")
	}
}

// ---- SchedulePersistOverlayGeometry suppress 机制 ----
//
// 程序主动搬窗（SetOverlayGeometry / ShowOverlayWindow）期间，wailsruntime
// 屏蔽标志置 true。SchedulePersistOverlayGeometry 在调度前检查该标志，
// true 时直接 return —— 防止 OS 触发的瞬态 WindowDidMove/Resize 被当成
// 用户拖拽结果落盘覆盖正确配置（2026-09-09 间歇性几何失效根因修复）。

func TestSchedulePersistOverlayGeometry_SuppressSkipsSchedule(t *testing.T) {
	snapshotOverlayConfig(t)
	resetOverlayDebounce(t)
	// 等待前序 test 残留的 suppress 复位（如有），并保证本 test 结束后
	// suppress 也复位，避免污染后续 test（特别是 ConcurrentSafe）
	waitSuppressClear(t, 2*time.Second)
	t.Cleanup(func() { waitSuppressClear(t, 2*time.Second) })

	cfg := config.Global()
	cfg.Overlay.X = 100
	cfg.Overlay.Y = 100
	cfg.Overlay.Width = 360
	cfg.Overlay.Height = 420

	// 进入屏蔽期（模拟 SetOverlayGeometry / ShowOverlayWindow 刚调用过）
	wailsruntime.SuppressGeometryPersist(500 * time.Millisecond)

	// 屏蔽期内的 schedule 调用应被丢弃：不写 cfg，不启动 debounce timer
	SchedulePersistOverlayGeometry(999, 999, 800, 600)

	// 立即检查：cfg 不应被修改
	if cfg.Overlay.X != 100 || cfg.Overlay.Y != 100 ||
		cfg.Overlay.Width != 360 || cfg.Overlay.Height != 420 {
		t.Errorf("expected cfg unchanged during suppress, got (%d,%d,%d,%d)",
			cfg.Overlay.X, cfg.Overlay.Y, cfg.Overlay.Width, cfg.Overlay.Height)
	}

	// 等待 debounce 窗口过去：仍不应有 timer 触发写入
	time.Sleep(overlayGeometryDebounce + 100*time.Millisecond)
	if cfg.Overlay.X != 100 {
		t.Errorf("expected X=100 after debounce window, got %d (suppress did not block schedule)", cfg.Overlay.X)
	}
}

func TestSchedulePersistOverlayGeometry_WorksAfterSuppressExpires(t *testing.T) {
	snapshotOverlayConfig(t)
	resetOverlayDebounce(t)
	waitSuppressClear(t, 2*time.Second)
	t.Cleanup(func() { waitSuppressClear(t, 2*time.Second) })

	cfg := config.Global()
	cfg.Overlay.X = 0
	cfg.Overlay.Y = 0
	cfg.Overlay.Width = 0
	cfg.Overlay.Height = 0

	// 短屏蔽期（测试用 50ms，远小于真实 500ms，但足以验证 expire 后恢复工作）
	wailsruntime.SuppressGeometryPersist(50 * time.Millisecond)

	// 屏蔽期内：丢弃
	SchedulePersistOverlayGeometry(111, 222, 360, 420)
	time.Sleep(overlayGeometryDebounce + 50*time.Millisecond)
	if cfg.Overlay.X != 0 {
		t.Fatalf("expected X=0 during suppress, got %d", cfg.Overlay.X)
	}

	// 等屏蔽期过去
	time.Sleep(100 * time.Millisecond)

	// 屏蔽解除后的 schedule：正常工作
	SchedulePersistOverlayGeometry(333, 444, 480, 500)
	time.Sleep(overlayGeometryDebounce + 100*time.Millisecond)
	if cfg.Overlay.X != 333 || cfg.Overlay.Y != 444 ||
		cfg.Overlay.Width != 480 || cfg.Overlay.Height != 500 {
		t.Errorf("expected (333,444,480,500) after suppress expired, got (%d,%d,%d,%d)",
			cfg.Overlay.X, cfg.Overlay.Y, cfg.Overlay.Width, cfg.Overlay.Height)
	}
}

func TestFlushPendingOverlayGeometry_IgnoresSuppress(t *testing.T) {
	snapshotOverlayConfig(t)
	resetOverlayDebounce(t)
	waitSuppressClear(t, 2*time.Second)
	t.Cleanup(func() { waitSuppressClear(t, 2*time.Second) })

	cfg := config.Global()
	cfg.Overlay.X = 0
	cfg.Overlay.Y = 0
	cfg.Overlay.Width = 0
	cfg.Overlay.Height = 0

	// 先 schedule 一个 pending（在屏蔽前）
	SchedulePersistOverlayGeometry(50, 60, 360, 420)

	// 进入屏蔽期（模拟程序化搬窗与 flush 时序重叠）
	wailsruntime.SuppressGeometryPersist(500 * time.Millisecond)

	// Flush 必须无视屏蔽标志：关窗是用户意图，最后一次位置必须落盘
	wrote := FlushPendingOverlayGeometry()
	if !wrote {
		t.Errorf("expected Flush to write even during suppress")
	}
	if cfg.Overlay.X != 50 || cfg.Overlay.Y != 60 ||
		cfg.Overlay.Width != 360 || cfg.Overlay.Height != 420 {
		t.Errorf("expected (50,60,360,420) after flush, got (%d,%d,%d,%d)",
			cfg.Overlay.X, cfg.Overlay.Y, cfg.Overlay.Width, cfg.Overlay.Height)
	}
}
