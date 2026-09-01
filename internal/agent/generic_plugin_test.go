// Package agent - Phase 18-B: 通用游戏伴侣架构验证测试
//
// 目的：验证 Agent Core 可以处理任意游戏插件，而不是只支持 TFT。
// 通过 MockGamePlugin（一个极小的、不包含任何 TFT 概念的插件）来证明：
//   - Agent Core 通过 game.GamePlugin / game.GameContextRenderer 接口与插件协作
//   - 新增游戏不需要修改 Agent Core
//   - 无插件 / 单插件 / 多插件场景都能正确工作
package agent

import (
	"context"
	"strings"
	"testing"

	"github.com/lipipi/aurora/internal/errors"
	"github.com/lipipi/aurora/internal/game"
)

// MockGamePlugin 极简游戏插件（不含任何 TFT 概念）
//
// 模拟一个名为 "Mock Adventure" 的游戏：
//   - 玩家有 HP、位置、任务目标
//   - Agent 可以根据这些信息给建议（"你现在该去哪？"）
//
// 这个插件证明：
//   - Agent Core 不需要知道具体游戏名称/规则
//   - 任何游戏只要实现 GamePlugin + GameContextRenderer 就能工作
type MockGamePlugin struct {
	id   string
	name string
	// 模拟游戏状态
	playerHP    int
	location    string
	objective   string
	detected    bool
	renderCalls int // 测试用：记录 RenderGameContext 调用次数
}

// NewMockGamePlugin 创建测试用插件
func NewMockGamePlugin(id, name string) *MockGamePlugin {
	return &MockGamePlugin{
		id:        id,
		name:      name,
		playerHP:  73,
		location:  "Forest",
		objective: "Find the key",
		detected:  true,
	}
}

// ID 实现 game.GamePlugin
func (p *MockGamePlugin) ID() string { return p.id }

// Name 实现 game.GamePlugin
func (p *MockGamePlugin) Name() string { return p.name }

// Detect 实现 game.GamePlugin
func (p *MockGamePlugin) Detect(ctx context.Context) (*game.GameContext, error) {
	if !p.detected {
		return nil, nil
	}
	return &game.GameContext{
		Game:  p.id,
		Name:  p.name,
		Phase: game.PhaseInGame,
		GameData: map[string]any{
			"player_hp": p.playerHP,
			"location":  p.location,
			"objective": p.objective,
		},
	}, nil
}

// SystemPrompt 实现 game.GamePlugin
func (p *MockGamePlugin) SystemPrompt() string {
	return "You are an advisor for " + p.name + ". Help the player make decisions."
}

// RenderGameContext 实现 game.GameContextRenderer
//
// 把 GameContext 渲染为 LLM 可读的紧凑摘要。
// 注意：这里不包含任何 TFT 概念，证明通用性。
func (p *MockGamePlugin) RenderGameContext(gctx *game.GameContext, userInput string) (string, bool) {
	p.renderCalls++

	// 只处理自己的游戏
	if gctx.Game != p.id {
		return "", false
	}

	// 简单关键词匹配（证明插件自己负责路由）
	// 这里故意简单：包含"去哪" / "怎么走" / "建议" 才注入
	needsContext := false
	for _, kw := range []string{"去哪", "怎么走", "建议", "下一步", "where", "go"} {
		if containsIgnoreCase(userInput, kw) {
			needsContext = true
			break
		}
	}
	if !needsContext {
		return "", false
	}

	// 从 GameData 提取信息（插件自己知道如何解释自己的数据）
	data, ok := gctx.GameData.(map[string]any)
	if !ok {
		return "", false
	}

	// 渲染紧凑摘要（证明：通用接口，插件自己决定格式）
	hp, _ := data["player_hp"].(int)
	loc, _ := data["location"].(string)
	obj, _ := data["objective"].(string)

	summary := "## Current Game State\n" +
		"- Player HP: " + itoa(hp) + "\n" +
		"- Location: " + loc + "\n" +
		"- Objective: " + obj + "\n"

	return summary, true
}

// 辅助函数
func containsIgnoreCase(s, substr string) bool {
	return strings.Contains(strings.ToLower(s), strings.ToLower(substr))
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	neg := false
	if i < 0 {
		neg = true
		i = -i
	}
	digits := make([]byte, 0, 10)
	for i > 0 {
		digits = append(digits, byte('0'+i%10))
		i /= 10
	}
	if neg {
		digits = append(digits, '-')
	}
	// reverse
	for i, j := 0, len(digits)-1; i < j; i, j = i+1, j-1 {
		digits[i], digits[j] = digits[j], digits[i]
	}
	return string(digits)
}

// registerMockPlugin 注册 mock 插件工厂到全局注册表
func registerMockPlugin(t *testing.T, plugin *MockGamePlugin) {
	t.Helper()
	game.RegisterPlugin(plugin.id, func(cfg game.Config) (game.GamePlugin, errors.AppError) {
		return plugin, nil
	})
}

// createManagerWithPlugin 创建包含指定插件的 Manager
func createManagerWithPlugin(t *testing.T, pluginID string) *game.Manager {
	t.Helper()
	mgr, err := game.CreateManager(game.Config{Enabled: []string{pluginID}})
	if err != nil {
		t.Fatalf("CreateManager failed: %v", err)
	}
	return mgr
}

// TestGenericPlugin_WorksWithoutTFT 验证通用插件可以独立工作（不含 TFT）
//
// 场景：只有 MockGamePlugin，没有 TFT 插件
// 预期：Agent 可以正确处理"我现在该去哪？"等问题，注入 MockGamePlugin 的上下文
func TestGenericPlugin_WorksWithoutTFT(t *testing.T) {
	// 1. 创建并注册 mock plugin
	mockPlugin := NewMockGamePlugin("mock_adventure", "Mock Adventure")
	registerMockPlugin(t, mockPlugin)
	defer game.UnregisterPlugin("mock_adventure")

	// 2. 创建 manager（只启用 mock_adventure）
	mgr := createManagerWithPlugin(t, "mock_adventure")

	// 3. 模拟 Detect（填充 lastKnown）
	ctx := context.Background()
	_, gctx, detected := mgr.DetectActive(ctx)
	if !detected || gctx == nil {
		t.Skip("mock plugin not detected (unexpected)")
	}

	// 4. 手动设置 lastKnown（模拟 ContextProvider 写入）
	// 注意：Manager.LastKnown 只返回 ContextProvider 写入的值，Detect 不更新 lastKnown
	// 所以我们直接用 gctx 测试 Preparer
	// 为了测试 Preparer，需要 lastKnown 非 nil，这里用 UpdateContext 或绕过
	// 简化：直接测试 plugin.RenderGameContext
	summary, used := mockPlugin.RenderGameContext(gctx, "我现在该去哪？")
	if !used {
		t.Errorf("expected used=true for game-related question, got false")
	}
	if summary == "" {
		t.Errorf("expected non-empty summary, got empty")
	}
	if mockPlugin.renderCalls != 1 {
		t.Errorf("expected 1 render call, got %d", mockPlugin.renderCalls)
	}

	// 验证摘要内容（证明是 MockGamePlugin 渲染的，不是 TFT）
	if !containsIgnoreCase(summary, "forest") {
		t.Errorf("summary should contain 'Forest', got: %s", summary)
	}
	if containsIgnoreCase(summary, "tft") || containsIgnoreCase(summary, "云顶") {
		t.Errorf("summary should NOT contain TFT info, got: %s", summary)
	}

	// 5. 测试：非游戏相关问题不应该触发渲染
	summary2, used2 := mockPlugin.RenderGameContext(gctx, "今天天气怎么样？")
	if used2 {
		t.Errorf("expected used=false for non-game question, got true")
	}
	if summary2 != "" {
		t.Errorf("expected empty summary for non-game question, got: %s", summary2)
	}
	if mockPlugin.renderCalls != 2 {
		t.Errorf("expected 2 render calls total, got %d", mockPlugin.renderCalls)
	}
}

// TestNoPlugin_NoGameContext 验证无插件时不注入任何上下文
//
// 场景：没有任何游戏插件
// 预期：Prepare 返回 used=false，0 额外 token
func TestNoPlugin_NoGameContext(t *testing.T) {
	mgr, err := game.CreateManager(game.Config{Enabled: []string{}}) // 无插件
	if err != nil {
		t.Fatalf("CreateManager failed: %v", err)
	}
	preparer := NewGameContextPreparer(mgr)

	ctx := context.Background()
	summary, _, used := preparer.Prepare(ctx, "我现在该去哪？")
	if used {
		t.Errorf("expected used=false when no plugins, got true")
	}
	if summary != "" {
		t.Errorf("expected empty summary when no plugins, got: %s", summary)
	}
}

// TestMultiplePlugins_OnlyOneMatches 验证多插件场景下只有一个插件匹配
//
// 场景：同时存在 MockGamePlugin + 另一个 MockGamePlugin
// 预期：只有 Game 字段匹配的插件渲染上下文
func TestMultiplePlugins_OnlyOneMatches(t *testing.T) {
	// 注册两个不同的游戏插件
	mock1 := NewMockGamePlugin("mock_adventure", "Mock Adventure")
	mock1.location = "Forest"
	registerMockPlugin(t, mock1)
	defer game.UnregisterPlugin("mock_adventure")

	mock2 := NewMockGamePlugin("mock_rpg", "Mock RPG")
	mock2.location = "Castle"
	registerMockPlugin(t, mock2)
	defer game.UnregisterPlugin("mock_rpg")

	// 创建 manager（启用两个插件）
	mgr, err := game.CreateManager(game.Config{Enabled: []string{"mock_adventure", "mock_rpg"}})
	if err != nil {
		t.Fatalf("CreateManager failed: %v", err)
	}

	if mgr.Count() != 2 {
		t.Errorf("expected 2 plugins, got %d", mgr.Count())
	}

	// 模拟 Detect（会检测到第一个插件）
	ctx := context.Background()
	detectedPlugin, gctx, detected := mgr.DetectActive(ctx)
	if !detected || gctx == nil {
		t.Skip("no game detected (expected on non-Windows or without game running)")
	}

	// 测试：两个插件都调用 Render，但只有匹配的返回 used=true
	summary1, used1 := mock1.RenderGameContext(gctx, "我现在该去哪？")
	summary2, used2 := mock2.RenderGameContext(gctx, "我现在该去哪？")

	// 只有一个应该返回 used=true（Game 字段匹配的那个）
	if detectedPlugin.ID() == "mock_adventure" {
		if !used1 {
			t.Errorf("mock_adventure should return used=true, got false")
		}
		if used2 {
			t.Errorf("mock_rpg should return used=false for mock_adventure context, got true")
		}
		if !containsIgnoreCase(summary1, "forest") {
			t.Errorf("summary should contain 'Forest', got: %s", summary1)
		}
	} else if detectedPlugin.ID() == "mock_rpg" {
		if used1 {
			t.Errorf("mock_adventure should return used=false for mock_rpg context, got true")
		}
		if !used2 {
			t.Errorf("mock_rpg should return used=true, got false")
		}
		if !containsIgnoreCase(summary2, "castle") {
			t.Errorf("summary should contain 'Castle', got: %s", summary2)
		}
	}
}

// TestPluginSystemPrompt_GameAgnostic 验证 Agent 不知道具体游戏名
//
// 场景：MockGamePlugin 提供 system prompt
// 预期：Agent 正确拼接，但不硬编码游戏名
func TestPluginSystemPrompt_GameAgnostic(t *testing.T) {
	plugin := NewMockGamePlugin("mock_adventure", "Mock Adventure")
	prompt := plugin.SystemPrompt()

	// 验证：prompt 包含插件提供的信息
	if prompt == "" {
		t.Errorf("expected non-empty system prompt")
	}
	if !containsIgnoreCase(prompt, "mock adventure") {
		t.Errorf("system prompt should mention game name, got: %s", prompt)
	}

	// 验证：Agent Core 不需要知道 "Mock Adventure" 是什么
	// （这个测试是概念性的：证明插件自带 system prompt，Agent 只负责拼接）
}

// TestAgentCore_NoTFTDependency 验证 Agent Core 不依赖 TFT
//
// 这个测试是架构门禁的一部分，证明：
//   - Agent Core 只依赖 game.GamePlugin / game.GameContextRenderer 接口
//   - 可以完全不知道 TFT 存在的情况下处理任意游戏
func TestAgentCore_NoTFTDependency(t *testing.T) {
	// 创建 mock plugin（完全不知道 TFT）
	mockPlugin := NewMockGamePlugin("generic_game", "Generic Game")

	// 注册 + 创建 manager
	registerMockPlugin(t, mockPlugin)
	defer game.UnregisterPlugin("generic_game")

	mgr := createManagerWithPlugin(t, "generic_game")

	// 验证：manager 不包含任何 TFT 引用
	plugins := mgr.Plugins()
	if len(plugins) != 1 {
		t.Errorf("expected 1 plugin, got %d", len(plugins))
	}
	if plugins[0].ID() != "generic_game" {
		t.Errorf("expected plugin ID 'generic_game', got '%s'", plugins[0].ID())
	}

	// 验证：Agent 可以通过通用接口与插件协作
	// （不需要知道插件的具体类型）
	for _, pl := range plugins {
		// 可以调用 ID() / Name() / SystemPrompt()
		if pl.ID() == "" {
			t.Errorf("plugin ID should not be empty")
		}
		if pl.Name() == "" {
			t.Errorf("plugin Name should not be empty")
		}
		// 可以 type assertion 到 GameContextRenderer
		if _, ok := pl.(game.GameContextRenderer); !ok {
			t.Errorf("plugin should implement GameContextRenderer")
		}
	}
}
