package tool

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/lipipi/aurora/internal/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockTool 测试用 tool 实现
type mockTool struct {
	name      string
	available bool
}

func (m *mockTool) Name() string                                         { return m.name }
func (m *mockTool) Description() string                                  { return "mock" }
func (m *mockTool) InputSchema() string                                  { return "{}" }
func (m *mockTool) IsAvailable() bool                                    { return m.available }
func (m *mockTool) Execute(context.Context, json.RawMessage) (*ToolResult, errors.AppError) {
	return &ToolResult{Success: true}, nil
}

// TestFilterUnavailable_RemovesUnavailableTools 验证 FilterUnavailable 移除不可用工具
func TestFilterUnavailable_RemovesUnavailableTools(t *testing.T) {
	// 注册 3 个 mock 工具：2 个可用，1 个不可用
	const (
		avail1    = "test_filter_avail_1"
		avail2    = "test_filter_avail_2"
		unavail   = "test_filter_unavail"
	)
	RegisterTool(avail1, func(cfg Config) (Tool, errors.AppError) {
		return &mockTool{name: avail1, available: true}, nil
	})
	RegisterTool(avail2, func(cfg Config) (Tool, errors.AppError) {
		return &mockTool{name: avail2, available: true}, nil
	})
	RegisterTool(unavail, func(cfg Config) (Tool, errors.AppError) {
		return &mockTool{name: unavail, available: false}, nil
	})
	defer func() {
		delete(toolFactories, avail1)
		delete(toolFactories, avail2)
		delete(toolFactories, unavail)
	}()

	cfg := Config{Enabled: []string{avail1, avail2, unavail}}
	registry, err := CreateRegistry(cfg)
	require.NoError(t, err)
	require.NotNil(t, registry)

	// CreateRegistry 现在包含所有工具（不做 IsAvailable 过滤）
	assert.Equal(t, 3, registry.Count(), "CreateRegistry should include all instantiated tools")

	// FilterUnavailable 移除不可用的
	removed := registry.FilterUnavailable()
	assert.Equal(t, 1, removed, "should remove 1 unavailable tool")
	assert.Equal(t, 2, registry.Count(), "should have 2 tools remaining")

	// 验证剩下的工具是正确的
	_, hasAvail1 := registry.GetTool(avail1)
	_, hasAvail2 := registry.GetTool(avail2)
	_, hasUnavail := registry.GetTool(unavail)
	assert.True(t, hasAvail1)
	assert.True(t, hasAvail2)
	assert.False(t, hasUnavail, "unavailable tool should be removed")
}

// TestFilterUnavailable_NilRegistry 验证对 nil registry 调用不 panic
func TestFilterUnavailable_NilRegistry(t *testing.T) {
	var r *ToolRegistry
	assert.NotPanics(t, func() {
		n := r.FilterUnavailable()
		assert.Equal(t, 0, n)
	})
}

// TestFilterUnavailable_Idempotent 验证多次调用结果稳定
func TestFilterUnavailable_Idempotent(t *testing.T) {
	const name = "test_filter_idem"
	RegisterTool(name, func(cfg Config) (Tool, errors.AppError) {
		return &mockTool{name: name, available: false}, nil
	})
	defer delete(toolFactories, name)

	cfg := Config{Enabled: []string{name}}
	registry, err := CreateRegistry(cfg)
	require.NoError(t, err)

	assert.Equal(t, 1, registry.FilterUnavailable())
	assert.Equal(t, 0, registry.FilterUnavailable(), "second call should remove nothing")
	assert.Equal(t, 0, registry.Count())
}

// TestCreateRegistry_DoesNotFilterByAvailability 回归测试：
// CreateRegistry 不应在初始化阶段按 IsAvailable 过滤工具。
// 这保证了"先创建 → 后注入依赖 → 再过滤"的正确流程。
func TestCreateRegistry_DoesNotFilterByAvailability(t *testing.T) {
	const name = "test_no_filter_at_create"
	RegisterTool(name, func(cfg Config) (Tool, errors.AppError) {
		return &mockTool{name: name, available: false}, nil
	})
	defer delete(toolFactories, name)

	cfg := Config{Enabled: []string{name}}
	registry, err := CreateRegistry(cfg)
	require.NoError(t, err)

	// 修复前这里会是 0（被 CreateRegistry 过滤掉）
	// 修复后应该是 1（延迟到 FilterUnavailable 时过滤）
	assert.Equal(t, 1, registry.Count(),
		"CreateRegistry should NOT filter by IsAvailable; leave that to FilterUnavailable")

	tool, exists := registry.GetTool(name)
	assert.True(t, exists)
	assert.False(t, tool.IsAvailable(), "tool is in registry but not yet available")
}

// TestToolRegistry_Add 验证 Add 直接注入已构造工具（核心生命周期工具路径）：
// 不受 cfg.Tool.Enabled 持久化列表过滤，同名覆盖
func TestToolRegistry_Add(t *testing.T) {
	// 空 Enabled = 启用所有已注册工厂；这里用显式列表模拟既有用户的持久化配置
	registry, err := CreateRegistry(Config{Enabled: []string{}})
	require.NoError(t, err)
	before := registry.Count()

	// Add 一个不在 Enabled 列表中的工具
	registry.Add(&mockTool{name: "test_core_tool", available: true})
	assert.Equal(t, before+1, registry.Count(), "Add should insert the tool directly")

	got, exists := registry.GetTool("test_core_tool")
	assert.True(t, exists)
	assert.Equal(t, "test_core_tool", got.Name())

	// 同名覆盖
	registry.Add(&mockTool{name: "test_core_tool", available: false})
	assert.Equal(t, before+1, registry.Count(), "Add with same name should overwrite")

	// nil 安全
	registry.Add(nil)
	assert.Equal(t, before+1, registry.Count(), "Add(nil) should be a no-op")
}
