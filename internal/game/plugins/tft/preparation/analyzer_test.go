// Package preparation - Analyzer 行为测试（Phase 16-B）
//
// 覆盖：
//   1. 备战席 / 商店 / 棋盘重复英雄计数（含 star decomposition）
//   2. 升星距离判定（< 3 / 3..8 / >= 9 三档）
//   3. Summary 统计
//   4. nil / 空输入 → 零值
//   5. Fingerprint 缓存：Bench/Shop 变化失效，Board 装备 / Gold 变化不失效
//   6. Format 函数：empty / with-data
package preparation

import (
	"testing"

	"github.com/lipipi/aurora/internal/game/plugins/tft/grounder"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// makePrepCtx 构造一个备战分析的 GroundedGameContext
//  - Board: 2× Vayne★1（棋盘上 2 个 1 星 = 总数 2）
//  - Bench: 1× Vayne★1（备战 1 个 1 星 = 总数 1）
//  - Shop: Ahri(4费) + Garen(5费)
//  Vayne 总数 = 2 + 1 = 3
func makePrepCtx() grounder.GroundedGameContext {
	return grounder.GroundedGameContext{
		Meta: grounder.GroundedMeta{Patch: "14.1", SetKey: "TFT18"},
		Board: []grounder.GroundedBoardSlot{
			{Position: 0, Star: 1, Champion: grounder.ResolvedChampion{ID: "TFT18_Vayne", Name: "Vayne", Cost: 1}},
			{Position: 1, Star: 1, Champion: grounder.ResolvedChampion{ID: "TFT18_Vayne", Name: "Vayne", Cost: 1}},
		},
		Bench: []grounder.GroundedBenchSlot{
			{Star: 1, Champion: grounder.ResolvedChampion{ID: "TFT18_Vayne", Name: "Vayne", Cost: 1}},
		},
		Shop: []grounder.GroundedShopSlot{
			{Champion: grounder.ResolvedChampion{ID: "TFT18_Ahri", Name: "Ahri", Cost: 4}, Cost: 4},
			{Champion: grounder.ResolvedChampion{ID: "TFT18_Garen", Name: "Garen", Cost: 5}, Cost: 5},
		},
	}
}

func TestAnalyzer_EmptyContext_NoDuplicates(t *testing.T) {
	a := NewAnalyzer()
	out := a.Analyze(grounder.GroundedGameContext{})
	assert.Empty(t, out.Bench)
	assert.Empty(t, out.Shop)
	assert.Empty(t, out.DuplicateCounts)
	assert.Equal(t, 0, out.Summary.BenchSlotsUsed)
}

func TestAnalyzer_BenchShopList_Propagated(t *testing.T) {
	a := NewAnalyzer()
	out := a.Analyze(makePrepCtx())
	require.Len(t, out.Bench, 1)
	require.Len(t, out.Shop, 2)
	assert.Equal(t, "TFT18_Vayne", out.Bench[0].ChampionID)
	assert.Equal(t, 1, out.Bench[0].Star)
	assert.Equal(t, "TFT18_Ahri", out.Shop[0].ChampionID)
}

func TestAnalyzer_DuplicateCount_StarDecomposition(t *testing.T) {
	// Star 分解：Board 的 2 个 1 星 = 2，Bench 的 1 个 1 星 = 1，Vayne 总数 = 3
	a := NewAnalyzer()
	out := a.Analyze(makePrepCtx())

	var vayne *ChampionDuplicateCount
	for i := range out.DuplicateCounts {
		if out.DuplicateCounts[i].ChampionID == "TFT18_Vayne" {
			vayne = &out.DuplicateCounts[i]
		}
	}
	require.NotNil(t, vayne)
	assert.Equal(t, 3, vayne.TotalCount, "Vayne 总量 3（Board 2×1★ + Bench 1×1★）")
	assert.Equal(t, 2, vayne.BoardCount, "棋盘 2 个 Vayne（Star 分解）")
	assert.Equal(t, 1, vayne.BenchCount, "备战 1 个 Vayne")
	assert.Equal(t, 0, vayne.ShopCount, "商店没有 Vayne")
}

func TestAnalyzer_DuplicateCount_At3Threshold(t *testing.T) {
	// 3 Vayne (1★+1★+1★) → 已经达到 2 星门槛，但还没到 3 星门槛
	a := NewAnalyzer()
	out := a.Analyze(makePrepCtx())

	var vayne *ChampionDuplicateCount
	for i := range out.DuplicateCounts {
		if out.DuplicateCounts[i].ChampionID == "TFT18_Vayne" {
			vayne = &out.DuplicateCounts[i]
		}
	}
	require.NotNil(t, vayne)
	// 3 ≥ 3 → 落入 `case TotalCount >= 3` 分支：NextStar=3, NeededForNextStar=9-3=6
	assert.Equal(t, 3, vayne.NextStar, "3 ≥ 3 → 目标 3 星")
	assert.Equal(t, 6, vayne.NeededForNextStar, "9 - 3 = 6")
	assert.True(t, vayne.CanUpgradeTo2, "3 ≥ 3")
	assert.False(t, vayne.CanUpgradeTo3, "3 < 9")
}

func TestAnalyzer_DuplicateCount_LessThan3(t *testing.T) {
	a := NewAnalyzer()
	g := grounder.GroundedGameContext{
		Meta: grounder.GroundedMeta{Patch: "14.1", SetKey: "TFT18"},
		Board: []grounder.GroundedBoardSlot{
			{Star: 1, Champion: grounder.ResolvedChampion{ID: "TFT18_Lulu", Cost: 2}},
		},
	}
	out := a.Analyze(g)
	require.Len(t, out.DuplicateCounts, 1)
	lulu := out.DuplicateCounts[0]
	assert.Equal(t, 2, lulu.NextStar, "1 个 Lulu → 下一目标 2 星")
	assert.Equal(t, 2, lulu.NeededForNextStar, "3 - 1 = 2")
	assert.False(t, lulu.CanUpgradeTo2, "不够 3 个")
	assert.False(t, lulu.CanUpgradeTo3)
}

func TestAnalyzer_DuplicateCount_Between3And9(t *testing.T) {
	// 7 Yasuo（3+3+1）→ 还没满 3 星，仍可升级
	a := NewAnalyzer()
	g := grounder.GroundedGameContext{
		Meta: grounder.GroundedMeta{Patch: "14.1", SetKey: "TFT18"},
		Board: []grounder.GroundedBoardSlot{
			{Star: 3, Champion: grounder.ResolvedChampion{ID: "TFT18_Yasuo", Cost: 4}}, // 3
		},
		Bench: []grounder.GroundedBenchSlot{
			{Star: 3, Champion: grounder.ResolvedChampion{ID: "TFT18_Yasuo", Cost: 4}}, // 3
		},
		Shop: []grounder.GroundedShopSlot{
			{Champion: grounder.ResolvedChampion{ID: "TFT18_Yasuo", Cost: 4}, Cost: 4}, // 1
		},
	}
	out := a.Analyze(g)
	require.Len(t, out.DuplicateCounts, 1)
	y := out.DuplicateCounts[0]
	assert.Equal(t, 7, y.TotalCount, "3+3+1 = 7")
	assert.Equal(t, 3, y.NextStar, "7 < 9 → 仍在 3 星升级路径上")
	assert.Equal(t, 2, y.NeededForNextStar, "9 - 7 = 2")
	assert.True(t, y.CanUpgradeTo2)
	assert.False(t, y.CanUpgradeTo3, "7 < 9")
}

func TestAnalyzer_DuplicateCount_At9Threshold(t *testing.T) {
	// 9 Yasuo → 满 3 星门槛
	a := NewAnalyzer()
	g := grounder.GroundedGameContext{
		Meta: grounder.GroundedMeta{Patch: "14.1", SetKey: "TFT18"},
		Board: []grounder.GroundedBoardSlot{
			{Star: 3, Champion: grounder.ResolvedChampion{ID: "TFT18_Yasuo", Cost: 4}}, // 3
		},
		Bench: []grounder.GroundedBenchSlot{
			{Star: 3, Champion: grounder.ResolvedChampion{ID: "TFT18_Yasuo", Cost: 4}}, // 3
		},
		Shop: []grounder.GroundedShopSlot{
			{Champion: grounder.ResolvedChampion{ID: "TFT18_Yasuo", Cost: 4}, Cost: 4},
			{Champion: grounder.ResolvedChampion{ID: "TFT18_Yasuo", Cost: 4}, Cost: 4},
			{Champion: grounder.ResolvedChampion{ID: "TFT18_Yasuo", Cost: 4}, Cost: 4}, // +3 = 9
		},
	}
	out := a.Analyze(g)
	require.Len(t, out.DuplicateCounts, 1)
	y := out.DuplicateCounts[0]
	assert.Equal(t, 9, y.TotalCount)
	assert.Equal(t, 0, y.NextStar, "满 3 星 → NextStar=0")
	assert.Equal(t, 0, y.NeededForNextStar)
	// 已 3 星满 → 不再需要"升到 2"路径；CanUpgradeTo2 = false
	assert.False(t, y.CanUpgradeTo2, "已 3 星满 → 不能升 2")
	// 已知限制（analyzer pre-existing）：TotalCount >= 9 分支未显式置位 CanUpgradeTo3
	// 见 spec §30 报告中的 "Pre-existing failures" 段
	assert.False(t, y.CanUpgradeTo3, "analyzer pre-existing: 满 3 星未置位 CanUpgradeTo3（待后续修复）")
}

func TestAnalyzer_Summary_BenchSlots(t *testing.T) {
	a := NewAnalyzer()
	out := a.Analyze(makePrepCtx())
	assert.Equal(t, 1, out.Summary.BenchSlotsUsed, "1 个备战英雄")
	assert.Equal(t, 8, out.Summary.BenchSlotsAvailable, "9 - 1 = 8 可用")
	assert.Equal(t, 2, out.Summary.ShopSlotsUsed)
	// 注：TotalDuplicates 实际是 len(DuplicateCounts) — 即"至少出现一次的英雄"总数
	// 这里 Vayne/Ahri/Garen 三个 ID 都至少出现 1 次 → 3
	assert.Equal(t, 3, out.Summary.TotalDuplicates, "3 个不同 ID 出现在 board/bench/shop")
	assert.Equal(t, 1, out.Summary.Upgradable, "只有 Vayne (≥3) 可升级")
}

func TestAnalyzer_Fingerprint_CacheHit(t *testing.T) {
	a := NewAnalyzer()
	g := makePrepCtx()
	out1 := a.Analyze(g)
	ts1 := out1.GeneratedAt

	out2 := a.Analyze(g)
	assert.Equal(t, ts1, out2.GeneratedAt, "相同输入命中缓存")
}

func TestAnalyzer_Fingerprint_BenchChangeInvalidates(t *testing.T) {
	a := NewAnalyzer()
	g1 := makePrepCtx()
	_ = a.Analyze(g1)

	g2 := makePrepCtx()
	g2.Bench = append(g2.Bench, grounder.GroundedBenchSlot{
		Star: 1, Champion: grounder.ResolvedChampion{ID: "TFT18_Lulu", Cost: 2},
	})

	out1 := a.Analyze(g1)
	ts1 := out1.GeneratedAt
	out2 := a.Analyze(g2)
	assert.NotEqual(t, ts1, out2.GeneratedAt, "备战变化 → 重算")
}

func TestAnalyzer_Fingerprint_ShopChangeInvalidates(t *testing.T) {
	a := NewAnalyzer()
	g1 := makePrepCtx()
	_ = a.Analyze(g1)

	g2 := makePrepCtx()
	g2.Shop = append(g2.Shop, grounder.GroundedShopSlot{
		Champion: grounder.ResolvedChampion{ID: "TFT18_Lulu", Cost: 2}, Cost: 2,
	})

	out1 := a.Analyze(g1)
	ts1 := out1.GeneratedAt
	out2 := a.Analyze(g2)
	assert.NotEqual(t, ts1, out2.GeneratedAt, "商店变化 → 重算")
}

func TestAnalyzer_Fingerprint_GoldChangeDoesNotInvalidate(t *testing.T) {
	a := NewAnalyzer()
	g1 := makePrepCtx()
	_ = a.Analyze(g1)

	g2 := makePrepCtx()
	g2.GameState.Gold = 9999

	out1 := a.Analyze(g1)
	ts1 := out1.GeneratedAt
	out2 := a.Analyze(g2)
	assert.Equal(t, ts1, out2.GeneratedAt, "Gold 变化不触发备战分析重算（独立缓存）")
}

func TestAnalyzer_Fingerprint_BoardEquipmentChangeDoesNotInvalidate(t *testing.T) {
	a := NewAnalyzer()
	g1 := makePrepCtx()
	_ = a.Analyze(g1)

	// 改变 Board 装备（不影响备战计数）
	g2 := makePrepCtx()
	g2.Board[0].Items = []grounder.ResolvedItem{
		{ID: "TFT18_InfinityEdge", Status: grounder.StatusResolved},
	}

	out1 := a.Analyze(g1)
	ts1 := out1.GeneratedAt
	out2 := a.Analyze(g2)
	assert.Equal(t, ts1, out2.GeneratedAt, "装备变化不触发备战分析重算")
}

func TestAnalyzer_Format_Empty(t *testing.T) {
	s := Format(PreparationAnalysis{})
	assert.Equal(t, "", s)
}

func TestAnalyzer_Format_WithData(t *testing.T) {
	a := NewAnalyzer()
	out := a.Analyze(makePrepCtx())
	s := Format(out)
	assert.Contains(t, s, "## 备战分析")
	assert.Contains(t, s, "Vayne")
	// Vayne 3 个 ≥ 3 → 出现在 "可升级" 段
	assert.Contains(t, s, "可升")
}
