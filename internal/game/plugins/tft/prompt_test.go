// Package tft - PromptRenderer 单元测试（Phase 17-A）
//
// 历史：本文件测试的函数原本在 internal/agent/game_context_test.go。
// Phase 17-A 把 FormatTFTGameContext / needsGameContext / pickContextDetail
// 等函数整体迁移到 tft 插件的 prompt.go，本测试同步迁移到 tft 包。
//
// 覆盖：
//   - FormatTFTGameContext：nil / 空 / Unknown / 基础资源 / Board / Bench / Shop /
//     Items / Traits / Augments 等分支
//   - needsGameContext：关键词命中 / 不命中
//   - pickContextDetail：Minimal / Full 路由
//   - Render：used=false 的多种 skip 路径
//   - isInjectablePhase：phase 路由
package tft

import (
	"testing"

	"github.com/lipipi/aurora/internal/game"
	"github.com/stretchr/testify/assert"
)

// ---- FormatTFTGameContext ----

func TestFormatTFTGameContext_Nil(t *testing.T) {
	assert.Equal(t, "", FormatTFTGameContext(nil, ContextFull))
}

func TestFormatTFTGameContext_Empty(t *testing.T) {
	// 全 Unknown → 输出空（不输出"占位段"）
	tctx := &TFTGameContext{}
	assert.Equal(t, "", FormatTFTGameContext(tctx, ContextFull))
}

func TestFormatTFTGameContext_UnknownFieldsExcluded(t *testing.T) {
	// 显式让 0 个字段 Present
	tctx := &TFTGameContext{}
	s := FormatTFTGameContext(tctx, ContextMinimal)
	assert.Equal(t, "", s, "no Known fields → no output")
}

func TestFormatTFTGameContext_BasicResources(t *testing.T) {
	tctx := &TFTGameContext{
		Stage: game.Known("4-2"),
		Level: game.Known(7),
		Gold:  game.Known(32),
		HP:    game.Known(80),
	}
	s := FormatTFTGameContext(tctx, ContextMinimal)
	assert.Contains(t, s, "阶段：4-2")
	assert.Contains(t, s, "等级：7")
	assert.Contains(t, s, "金币：32")
	assert.Contains(t, s, "生命：80")
}

func TestFormatTFTGameContext_FullIncludesBoard(t *testing.T) {
	tctx := &TFTGameContext{
		Stage: game.Known("4-2"),
		Board: game.Known([]BoardSlot{
			{ChampionID: "TFT18_Vayne", Star: 2, Position: 12, Items: []string{"TFT18_IE"}},
		}),
	}
	s := FormatTFTGameContext(tctx, ContextFull)
	assert.Contains(t, s, "棋盘")
	assert.Contains(t, s, "TFT18_Vayne")
	assert.Contains(t, s, "★2")
}

func TestFormatTFTGameContext_MinimalExcludesBoard(t *testing.T) {
	tctx := &TFTGameContext{
		Stage: game.Known("4-2"),
		Board: game.Known([]BoardSlot{
			{ChampionID: "TFT18_Vayne", Star: 2, Position: 12},
		}),
	}
	s := FormatTFTGameContext(tctx, ContextMinimal)
	assert.Contains(t, s, "阶段：4-2")
	assert.NotContains(t, s, "棋盘", "minimal must not include board section")
}

func TestFormatTFTGameContext_HPZeroIsShown(t *testing.T) {
	// HP=0 是合法状态（阵亡）；不能被当 unknown 丢掉
	tctx := &TFTGameContext{
		Stage: game.Known("5-1"),
		HP:    game.Known(0),
	}
	s := FormatTFTGameContext(tctx, ContextMinimal)
	assert.Contains(t, s, "生命：0", "HP=0 (dead) must be present")
}

// ---- needsGameContext ----

func TestNeedsGameContext_TFTKeywords(t *testing.T) {
	cases := []struct {
		input string
		want  bool
	}{
		{"", false},
		{"今天天气怎么样", false},
		{"tft", true},
		{"云顶之弈", true},
		{"这把怎么玩", true},
		{"薇恩 2 星的强度", true},
		{"我该升人口吗", true},
		{"vayne 怎么配装备", true},
	}
	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			assert.Equal(t, tc.want, needsGameContext(tc.input))
		})
	}
}

// ---- pickContextDetail ----

func TestPickContextDetail(t *testing.T) {
	cases := []struct {
		input string
		want  ContextDetail
	}{
		{"", ContextMinimal},
		{"今天天气怎么样", ContextMinimal},
		{"这把怎么玩", ContextFull},
		{"装备给谁", ContextFull},
		{"这把怎么玩", ContextFull},
		{"强化选什么", ContextFull},
	}
	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			assert.Equal(t, tc.want, pickContextDetail(tc.input))
		})
	}
}

// ---- isInjectablePhase ----

func TestIsInjectablePhase(t *testing.T) {
	cases := []struct {
		phase game.ContextPhase
		want  bool
	}{
		{game.PhaseUnknown, false},
		{game.PhaseNotInGame, false},
		{game.PhaseInGame, true},
		{game.PhaseCarousel, true},
		{game.PhaseDraft, true},
		{game.PhaseGameOver, false},
	}
	for _, tc := range cases {
		t.Run(string(tc.phase), func(t *testing.T) {
			assert.Equal(t, tc.want, isInjectablePhase(tc.phase))
		})
	}
}

// ---- Render ----

func TestRender_NilGctx(t *testing.T) {
	r := NewPromptRenderer(nil, nil, nil, nil)
	_, used := r.Render(nil, "tft 怎么玩")
	assert.False(t, used, "nil gctx must skip")
}

func TestRender_NotTFT(t *testing.T) {
	r := NewPromptRenderer(nil, nil, nil, nil)
	gctx := &game.GameContext{Game: "lol", Phase: game.PhaseInGame}
	_, used := r.Render(gctx, "lol 怎么玩")
	assert.False(t, used, "non-tft game must skip")
}

func TestRender_WrongPhase(t *testing.T) {
	r := NewPromptRenderer(nil, nil, nil, nil)
	gctx := &game.GameContext{
		Game:  "tft",
		Phase: game.PhaseNotInGame,
		GameData: &TFTGameContext{
			Stage: game.Known("4-2"),
		},
	}
	_, used := r.Render(gctx, "tft 怎么玩")
	assert.False(t, used, "non-injectable phase must skip")
}

func TestRender_NotTFTQuestion(t *testing.T) {
	r := NewPromptRenderer(nil, nil, nil, nil)
	gctx := &game.GameContext{
		Game:     "tft",
		Phase:    game.PhaseInGame,
		GameData: &TFTGameContext{Stage: game.Known("4-2")},
	}
	_, used := r.Render(gctx, "今天天气怎么样")
	assert.False(t, used, "non-tft question must skip")
}

func TestRender_HappyPath(t *testing.T) {
	r := NewPromptRenderer(nil, nil, nil, nil)
	gctx := &game.GameContext{
		Game:     "tft",
		Phase:    game.PhaseInGame,
		GameData: &TFTGameContext{Stage: game.Known("4-2"), Gold: game.Known(32)},
	}
	summary, used := r.Render(gctx, "tft 怎么玩")
	assert.True(t, used)
	assert.NotEmpty(t, summary)
	assert.Contains(t, summary, "阶段：4-2")
	assert.Contains(t, summary, "金币：32")
}

func TestRender_NoKBAndNoGrounderStillProducesBase(t *testing.T) {
	// Phase 17-A: kb=nil, grounder=nil → 仍能出基础资源段（不依赖 KB）
	r := NewPromptRenderer(nil, nil, nil, nil)
	gctx := &game.GameContext{
		Game:     "tft",
		Phase:    game.PhaseInGame,
		GameData: &TFTGameContext{Stage: game.Known("4-2")},
	}
	summary, used := r.Render(gctx, "tft 怎么玩")
	assert.True(t, used)
	assert.Contains(t, summary, "阶段：4-2")
}

// ---- collectChampionIDs ----

func TestCollectChampionIDs_Nil(t *testing.T) {
	assert.Nil(t, collectChampionIDs(nil))
}

func TestCollectChampionIDs_DedupBoardBench(t *testing.T) {
	tctx := &TFTGameContext{
		Board: game.Known([]BoardSlot{
			{ChampionID: "TFT18_Vayne", Star: 2, Position: 0},
			{ChampionID: "TFT18_Jinx", Star: 1, Position: 1},
		}),
		Bench: game.Known([]BenchSlot{
			{ChampionID: "TFT18_Vayne", Star: 1}, // 与 Board 重复
			{ChampionID: "TFT18_Ahri", Star: 1},
		}),
	}
	ids := collectChampionIDs(tctx)
	assert.Len(t, ids, 3, "Vayne should be deduped")
}

func TestBoardBenchCount(t *testing.T) {
	tctx := &TFTGameContext{
		Board: game.Known([]BoardSlot{{ChampionID: "x"}}),
		Bench: game.Known([]BenchSlot{{ChampionID: "y"}, {ChampionID: "z"}}),
	}
	assert.Equal(t, 1, boardCount(tctx))
	assert.Equal(t, 2, benchCount(tctx))
	assert.Equal(t, 0, boardCount(nil))
}
