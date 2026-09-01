package game

import (
	"context"
	"testing"
)

// TestManager_SnapshotEmpty 验证未设置 lastKnown 时 Snapshot 返回 nil
func TestManager_SnapshotEmpty(t *testing.T) {
	m := &Manager{}
	if got := m.Snapshot(); got != nil {
		t.Fatalf("Empty Manager.Snapshot() should be nil, got %+v", got)
	}
}

// TestManager_Clear 验证 Clear 后 Snapshot 返回 nil
func TestManager_Clear(t *testing.T) {
	m := &Manager{}
	m.lastKnown = &GameContext{Game: "tft", Name: "云顶之弈"}
	if got := m.Snapshot(); got == nil {
		t.Fatal("Snapshot should not be nil after setting lastKnown")
	}
	m.Clear()
	if got := m.Snapshot(); got != nil {
		t.Fatalf("Snapshot should be nil after Clear, got %+v", got)
	}
}

// TestManager_SnapshotIsCopy 验证 Snapshot 返回副本而非内部指针
func TestManager_SnapshotIsCopy(t *testing.T) {
	m := &Manager{}
	original := &GameContext{Game: "tft", Name: "云顶之弈"}
	m.lastKnown = original

	got := m.Snapshot()
	if got == nil {
		t.Fatal("Snapshot should not be nil")
	}
	// 修改返回的副本
	got.Name = "MODIFIED"
	// 验证原始 lastKnown 未被影响
	if m.lastKnown.Name != "云顶之弈" {
		t.Fatalf("Snapshot must be a copy, lastKnown was modified: %s", m.lastKnown.Name)
	}
	if original.Name != "云顶之弈" {
		t.Fatalf("Original must be unchanged, got: %s", original.Name)
	}
}

// TestManager_UpdateContextNoProvider 验证无 Provider 时 UpdateContext 仍可调用
// Phase 10-6 行为：若无 lastKnown 且 DetectActive 返回 false，UpdateContext 返回 nil
func TestManager_UpdateContextNoProvider(t *testing.T) {
	m := &Manager{} // 无 plugins，无 lastKnown

	ctx := context.Background()
	got := m.UpdateContext(ctx, ContextInput{})
	if got != nil {
		t.Fatalf("UpdateContext with no plugins and no lastKnown should return nil, got %+v", got)
	}
}

// TestManager_UpdateContextPreservesLastKnown 验证 UpdateContext 不破坏 lastKnown
// Phase 10-6 行为：lastKnown 已存在时，UpdateContext 应在其基础上更新
func TestManager_UpdateContextPreservesLastKnown(t *testing.T) {
	m := &Manager{}
	existing := &GameContext{Game: "tft", Name: "云顶之弈", Phase: PhaseInGame}
	m.lastKnown = existing

	ctx := context.Background()
	got := m.UpdateContext(ctx, ContextInput{})
	if got == nil {
		t.Fatal("UpdateContext should return updated context when lastKnown exists")
	}
	if got.Game != "tft" {
		t.Fatalf("Game mismatch: %s", got.Game)
	}
	if got.Phase != PhaseInGame {
		t.Fatalf("Phase mismatch: %s", got.Phase)
	}
}
