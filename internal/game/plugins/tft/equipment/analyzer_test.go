// Package equipment - Analyzer 行为测试（Phase 16-B）
//
// 覆盖：
//   1. 基础装备聚合（Board 装备 + Pocket 散件）
//   2. 装备类型分类（Completed / Component / Unknown）
//   3. KB 状态透传（KBUnavailable / Resolved / WrongSet）
//   4. Core champion linkage（依赖 CompositionAnalysis）
//   5. nil/空上下文 → 零值
//   6. Fingerprint 缓存命中 / 装备变化失效
//   7. Patch 隔离（WrongSet 不污染）
package equipment

import (
	"testing"

	"github.com/lipipi/aurora/internal/game/plugins/tft/composition"
	"github.com/lipipi/aurora/internal/game/plugins/tft/grounder"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// makeItemCtx 构造一个装备分析用的 GroundedGameContext
func makeItemCtx() grounder.GroundedGameContext {
	return grounder.GroundedGameContext{
		Meta: grounder.GroundedMeta{Patch: "14.1", SetKey: "TFT18", KBHasData: true},
		Board: []grounder.GroundedBoardSlot{
			{
				Position: 0, Star: 2,
				Champion: grounder.ResolvedChampion{ID: "TFT18_Vayne", Name: "Vayne", Cost: 1, Status: grounder.StatusResolved},
				Items: []grounder.ResolvedItem{
					{ID: "TFT18_InfinityEdge", Name: "Infinity Edge", From: []string{"TFT18_BF", "TFT18_Recurve"}, Status: grounder.StatusResolved},
					{ID: "TFT18_LastWhisper", Name: "Last Whisper", From: []string{"TFT18_Glove", "TFT18_Recurve"}, Status: grounder.StatusResolved},
				},
			},
			{
				Position: 5, Star: 1,
				Champion: grounder.ResolvedChampion{ID: "TFT18_Jinx", Name: "Jinx", Cost: 4, Status: grounder.StatusResolved},
				Items: []grounder.ResolvedItem{
					{ID: "TFT18_JG", Name: "Jeweled Gauntlet", From: []string{"TFT18_Rod", "TFT18_Glove"}, Status: grounder.StatusResolved},
				},
			},
		},
		Items: []grounder.GroundedItemSlot{
			// 散件口袋
			{Item: grounder.ResolvedItem{ID: "TFT18_BF", Name: "B.F. Sword", Status: grounder.StatusResolved}, Count: 2},
			{Item: grounder.ResolvedItem{ID: "TFT18_Recurve", Name: "Recurve Bow", Status: grounder.StatusResolved}, Count: 1},
		},
	}
}

// makeCoreComp 构造一个简化的 CompositionAnalysis（Vayne 标记为 Core）
func makeCoreComp(vayneID string) *composition.CompositionAnalysis {
	return &composition.CompositionAnalysis{
		CoreCandidates: []composition.CoreCandidate{
			{ChampionID: vayneID, Name: "Vayne", Score: 0.85, Status: composition.CoreStatusLikely},
		},
	}
}

func TestAnalyzer_EmptyContext_NoItems(t *testing.T) {
	a := NewAnalyzer()
	out := a.Analyze(grounder.GroundedGameContext{}, nil)
	assert.Empty(t, out.Champions)
	assert.Empty(t, out.ItemIndex)
	assert.Equal(t, 0, out.Summary.TotalItems)
}

func TestAnalyzer_BoardItems_AggregateToItemIndex(t *testing.T) {
	a := NewAnalyzer()
	g := makeItemCtx()
	out := a.Analyze(g, nil)

	require.Contains(t, out.ItemIndex, "TFT18_InfinityEdge")
	ie := out.ItemIndex["TFT18_InfinityEdge"]
	assert.Equal(t, "Infinity Edge", ie.Name)
	assert.Equal(t, ItemTypeCompleted, ie.Type)
	assert.Equal(t, 1, ie.TotalCount)
	assert.Equal(t, 1, ie.BoardCount)
	assert.Equal(t, 0, ie.PocketCount)
	require.Len(t, ie.Holders, 1)
	assert.Equal(t, "TFT18_Vayne", ie.Holders[0].ChampionID)
	assert.Equal(t, 2, ie.Holders[0].Star)
	assert.Equal(t, 0, ie.Holders[0].Position)
}

func TestAnalyzer_PocketItems_TaggedAsComponent(t *testing.T) {
	a := NewAnalyzer()
	g := makeItemCtx()
	out := a.Analyze(g, nil)

	require.Contains(t, out.ItemIndex, "TFT18_BF")
	bf := out.ItemIndex["TFT18_BF"]
	assert.Equal(t, ItemTypeComponent, bf.Type)
	assert.Equal(t, 2, bf.TotalCount, "B.F. Sword 出现 2 次")
	assert.Equal(t, 0, bf.BoardCount, "B.F. Sword 在 Board 上 0 次")
	assert.Equal(t, 2, bf.PocketCount, "B.F. Sword 在 Pocket 中 2 次")
}

func TestAnalyzer_ChampionEquipment_ListsItems(t *testing.T) {
	a := NewAnalyzer()
	g := makeItemCtx()
	out := a.Analyze(g, nil)

	// 找到 Vayne
	var vayne *ChampionEquipment
	for i := range out.Champions {
		if out.Champions[i].ChampionID == "TFT18_Vayne" {
			vayne = &out.Champions[i]
		}
	}
	require.NotNil(t, vayne)
	assert.Equal(t, 2, vayne.Star)
	assert.Len(t, vayne.Items, 2)
	assert.Equal(t, "TFT18_InfinityEdge", vayne.Items[0].ItemID)
}

func TestAnalyzer_CoreChampionLinkage_FromComposition(t *testing.T) {
	a := NewAnalyzer()
	g := makeItemCtx()
	comp := makeCoreComp("TFT18_Vayne")

	out := a.Analyze(g, comp)

	// 找到 Vayne
	var vayne *ChampionEquipment
	for i := range out.Champions {
		if out.Champions[i].ChampionID == "TFT18_Vayne" {
			vayne = &out.Champions[i]
		}
	}
	require.NotNil(t, vayne)
	assert.True(t, vayne.IsCore, "Composition Analysis 标记 Vayne 为 Likely Core → 装备分析继承 IsCore")
	assert.Equal(t, composition.CoreStatusLikely, vayne.CoreStatus)

	// Jinx 不在 Composition 里 → IsCore = false
	var jinx *ChampionEquipment
	for i := range out.Champions {
		if out.Champions[i].ChampionID == "TFT18_Jinx" {
			jinx = &out.Champions[i]
		}
	}
	require.NotNil(t, jinx)
	assert.False(t, jinx.IsCore)
}

func TestAnalyzer_CoreEquipment_FilteredByComposition(t *testing.T) {
	a := NewAnalyzer()
	g := makeItemCtx()
	comp := makeCoreComp("TFT18_Vayne")

	out := a.Analyze(g, comp)

	// CoreEquipment 只包含 Likely / Candidate
	require.NotEmpty(t, out.CoreEquipment)
	for _, ce := range out.CoreEquipment {
		assert.Equal(t, "TFT18_Vayne", ce.ChampionID)
	}
}

func TestAnalyzer_WrongSet_ItemStatusPreserved(t *testing.T) {
	a := NewAnalyzer()
	g := grounder.GroundedGameContext{
		Meta: grounder.GroundedMeta{Patch: "14.1", SetKey: "TFT18"},
		Board: []grounder.GroundedBoardSlot{
			{
				Position: 0, Star: 1,
				Champion: grounder.ResolvedChampion{ID: "TFT18_Vayne", Status: grounder.StatusResolved},
				Items: []grounder.ResolvedItem{
					// 旧 set 的装备 ID → KB 找不到
					{ID: "TFT16_OldItem", Status: grounder.StatusKBUnavailable},
				},
			},
		},
	}

	out := a.Analyze(g, nil)
	require.Contains(t, out.ItemIndex, "TFT16_OldItem")
	old := out.ItemIndex["TFT16_OldItem"]
	assert.Equal(t, ItemTypeUnknown, old.Type, "KB 找不到 → Unknown 类型")
	assert.Equal(t, "TFT18_Vayne", old.Holders[0].ChampionID) // sanity
}

func TestAnalyzer_Fingerprint_CacheHit(t *testing.T) {
	a := NewAnalyzer()
	g := makeItemCtx()
	out1 := a.Analyze(g, nil)
	ts1 := out1.GeneratedAt

	out2 := a.Analyze(g, nil)
	assert.Equal(t, ts1, out2.GeneratedAt, "完全相同输入应命中缓存")
}

func TestAnalyzer_Fingerprint_BoardItemChangeInvalidates(t *testing.T) {
	a := NewAnalyzer()
	g1 := makeItemCtx()
	_ = a.Analyze(g1, nil)

	// 改变 Vayne 的装备
	g2 := makeItemCtx()
	g2.Board[0].Items = append(g2.Board[0].Items, grounder.ResolvedItem{
		ID: "TFT18_GuardianAngel", Name: "Guardian Angel", From: []string{"TFT18_BF", "TFT18_Chain"}, Status: grounder.StatusResolved,
	})

	out1 := a.Analyze(g1, nil)
	out2 := a.Analyze(g2, nil)
	assert.NotEqual(t, out1.GeneratedAt.UnixNano(), out2.GeneratedAt.UnixNano(), "装备变化 → 重算")
	assert.Greater(t, out2.Summary.TotalItems, out1.Summary.TotalItems, "TotalItems 增长")
}

func TestAnalyzer_Fingerprint_EconomyChangeDoesNotInvalidate(t *testing.T) {
	a := NewAnalyzer()
	g1 := makeItemCtx()
	_ = a.Analyze(g1, nil)

	// 改变 Gold（经济字段，装备分析不依赖）
	g2 := makeItemCtx()
	g2.GameState.Gold = 9999

	out1 := a.Analyze(g1, nil)
	ts1 := out1.GeneratedAt
	out2 := a.Analyze(g2, nil)
	assert.Equal(t, ts1, out2.GeneratedAt, "Gold 变化不触发装备分析重算（独立缓存）")
}

func TestAnalyzer_Format_Empty(t *testing.T) {
	s := Format(EquipmentAnalysis{ItemIndex: map[string]ItemOwnership{}})
	assert.Equal(t, "", s)
}

func TestAnalyzer_Format_WithItems(t *testing.T) {
	a := NewAnalyzer()
	g := makeItemCtx()
	out := a.Analyze(g, makeCoreComp("TFT18_Vayne"))

	s := Format(out)
	assert.Contains(t, s, "## 装备分析")
	assert.Contains(t, s, "Infinity Edge")
	assert.Contains(t, s, "[核心]", "Vayne 应当被标为核心")
}
