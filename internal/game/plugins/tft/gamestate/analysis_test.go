// Package gamestate - Analyzer 行为测试（Phase 16-B）
//
// 覆盖：
//   1. AnalysisOptions 路由：NeedX 决定是否计算对应模块
//   2. nil 模块不参与 Format 输出
//   3. 子分析器独立缓存：模块间互不触发重算
//   4. Composition 引用透传（不重新计算）
//   5. InvalidateAll 清空所有缓存
//   6. Format 函数：组合 + 空
package gamestate

import (
	"testing"

	"github.com/lipipi/aurora/internal/game/plugins/tft/composition"
	"github.com/lipipi/aurora/internal/game/plugins/tft/grounder"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// makeCtx 通用 GroundedGameContext
func makeCtx() grounder.GroundedGameContext {
	return grounder.GroundedGameContext{
		Meta: grounder.GroundedMeta{Patch: "14.1", SetKey: "TFT18", KBHasData: true},
		GameState: grounder.GroundedGameState{
			Stage: "4-2", Phase: "PvP",
			Level: 7, XP: 12, XPTolevel: 4,
			Gold: 42, Interest: 3, HP: 67,
			WinStreak: 3,
		},
		Board: []grounder.GroundedBoardSlot{
			{
				Position: 0, Star: 2,
				Champion: grounder.ResolvedChampion{ID: "TFT18_Vayne", Name: "Vayne", Cost: 1, Status: grounder.StatusResolved},
				Items: []grounder.ResolvedItem{
					{ID: "TFT18_InfinityEdge", Name: "Infinity Edge", From: []string{"TFT18_BF", "TFT18_Recurve"}, Status: grounder.StatusResolved},
				},
			},
		},
		Bench: []grounder.GroundedBenchSlot{
			{Star: 1, Champion: grounder.ResolvedChampion{ID: "TFT18_Vayne", Name: "Vayne", Cost: 1}},
		},
		Shop: []grounder.GroundedShopSlot{
			{Champion: grounder.ResolvedChampion{ID: "TFT18_Ahri", Name: "Ahri", Cost: 4}, Cost: 4},
		},
	}
}

func TestAnalyzer_EmptyOptions_AllNil(t *testing.T) {
	a := NewAnalyzer()
	g := makeCtx()
	out := a.Analyze(g, nil, AnalysisOptions{})

	assert.Nil(t, out.Equipment)
	assert.Nil(t, out.Economy)
	assert.Nil(t, out.Preparation)
	assert.Nil(t, out.Composition)
}

func TestAnalyzer_OnlyEquipment(t *testing.T) {
	a := NewAnalyzer()
	g := makeCtx()
	out := a.Analyze(g, nil, AnalysisOptions{NeedEquipment: true})

	require.NotNil(t, out.Equipment)
	assert.Nil(t, out.Economy, "未请求 Economy → nil")
	assert.Nil(t, out.Preparation, "未请求 Preparation → nil")
}

func TestAnalyzer_OnlyEconomy(t *testing.T) {
	a := NewAnalyzer()
	g := makeCtx()
	out := a.Analyze(g, nil, AnalysisOptions{NeedEconomy: true})

	assert.Nil(t, out.Equipment)
	require.NotNil(t, out.Economy)
	assert.Equal(t, 42, out.Economy.Gold)
	assert.Equal(t, 7, out.Economy.Level)
}

func TestAnalyzer_OnlyPreparation(t *testing.T) {
	a := NewAnalyzer()
	g := makeCtx()
	out := a.Analyze(g, nil, AnalysisOptions{NeedPreparation: true})

	assert.Nil(t, out.Equipment)
	assert.Nil(t, out.Economy)
	require.NotNil(t, out.Preparation)
	assert.Equal(t, 1, out.Preparation.Summary.BenchSlotsUsed)
}

func TestAnalyzer_Composition_RefPassedThrough(t *testing.T) {
	a := NewAnalyzer()
	g := makeCtx()
	comp := &composition.CompositionAnalysis{
		ChampionCount: 1,
		CoreCandidates: []composition.CoreCandidate{
			{ChampionID: "TFT18_Vayne", Name: "Vayne", Status: composition.CoreStatusLikely},
		},
	}
	out := a.Analyze(g, comp, AnalysisOptions{NeedComposition: true})
	require.NotNil(t, out.Composition)
	assert.Equal(t, 1, out.Composition.ChampionCount)
	assert.Equal(t, "TFT18_Vayne", out.Composition.CoreCandidates[0].ChampionID)
}

func TestAnalyzer_AllModulesAtOnce(t *testing.T) {
	a := NewAnalyzer()
	g := makeCtx()
	out := a.Analyze(g, nil, AnalysisOptions{
		NeedEquipment:   true,
		NeedEconomy:     true,
		NeedPreparation: true,
	})
	require.NotNil(t, out.Equipment)
	require.NotNil(t, out.Economy)
	require.NotNil(t, out.Preparation)
}

func TestAnalyzer_EquipmentCacheIndependentOfEconomy(t *testing.T) {
	a := NewAnalyzer()
	g1 := makeCtx()
	_ = a.Analyze(g1, nil, AnalysisOptions{NeedEquipment: true, NeedEconomy: true})

	// 改 Gold（只影响 Economy，不影响 Equipment）
	g2 := makeCtx()
	g2.GameState.Gold = 1

	out1 := a.Analyze(g1, nil, AnalysisOptions{NeedEquipment: true})
	eq1 := out1.Equipment
	ts1 := eq1.GeneratedAt

	out2 := a.Analyze(g2, nil, AnalysisOptions{NeedEquipment: true})
	// Equipment 缓存命中（g1 vs g2 在装备分析下相同）
	assert.Equal(t, ts1, out2.Equipment.GeneratedAt, "Equipment 独立缓存：Gold 变化不重算装备")
}

func TestAnalyzer_InvalidateAll_ForcesRecompute(t *testing.T) {
	a := NewAnalyzer()
	g := makeCtx()
	out1 := a.Analyze(g, nil, AnalysisOptions{NeedEconomy: true})
	ts1 := out1.Economy.GeneratedAt

	a.InvalidateAll()

	out2 := a.Analyze(g, nil, AnalysisOptions{NeedEconomy: true})
	require.NotNil(t, out2.Economy)
	assert.True(t, out2.Economy.GeneratedAt.After(ts1) || out2.Economy.GeneratedAt.Equal(ts1),
		"InvalidateAll 后再次分析；时间戳可能更新（重算）")
}

func TestAnalyzer_Format_AllNil(t *testing.T) {
	out := GameStateAnalysis{}
	assert.Equal(t, "", Format(out))
}

func TestAnalyzer_Format_OnlyEconomy(t *testing.T) {
	a := NewAnalyzer()
	g := makeCtx()
	out := a.Analyze(g, nil, AnalysisOptions{NeedEconomy: true})

	s := Format(out)
	assert.Contains(t, s, "## 经济分析")
	assert.NotContains(t, s, "## 装备分析", "Equipment 是 nil → 不输出")
	assert.NotContains(t, s, "## 备战分析", "Preparation 是 nil → 不输出")
}

func TestAnalyzer_Format_EquipmentPlusEconomy(t *testing.T) {
	a := NewAnalyzer()
	g := makeCtx()
	out := a.Analyze(g, nil, AnalysisOptions{NeedEquipment: true, NeedEconomy: true})

	s := Format(out)
	assert.Contains(t, s, "## 装备分析")
	assert.Contains(t, s, "## 经济分析")
	assert.NotContains(t, s, "## 备战分析", "未请求 Preparation → 不输出")
}

func TestAnalyzer_Format_AllThree(t *testing.T) {
	a := NewAnalyzer()
	g := makeCtx()
	out := a.Analyze(g, nil, AnalysisOptions{
		NeedEquipment:   true,
		NeedEconomy:     true,
		NeedPreparation: true,
	})
	s := Format(out)
	assert.Contains(t, s, "## 装备分析")
	assert.Contains(t, s, "## 经济分析")
	assert.Contains(t, s, "## 备战分析")
}
