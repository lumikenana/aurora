package tft

import (
	"testing"

	"github.com/lipipi/aurora/internal/game"
)

// TestTFTGameContext_MergeFrom_BasicFields 测试字段级合并基础资源
func TestTFTGameContext_MergeFrom_BasicFields(t *testing.T) {
	// 初始状态：全部 Known
	dst := &TFTGameContext{
		Gold:  game.Known(42),
		HP:    game.Known(67),
		Level: game.Known(7),
		Stage: game.Known("4-2"),
	}
	// 第二轮：只识别 Gold
	src := &TFTGameContext{
		Gold: game.Detected(43, game.SourceOCR, 0.9),
		// HP/Level/Stage 保持 Unknown
	}
	dst.MergeFrom(src)

	// Gold 应该更新
	if v := dst.Gold.OrZero(); v != 43 {
		t.Errorf("Gold = %d, want 43", v)
	}
	// HP/Level/Stage 应该保留
	if v := dst.HP.OrZero(); v != 67 {
		t.Errorf("HP = %d, want 67 (preserved)", v)
	}
	if v := dst.Level.OrZero(); v != 7 {
		t.Errorf("Level = %d, want 7 (preserved)", v)
	}
	if v := dst.Stage.OrZero(); v != "4-2" {
		t.Errorf("Stage = %q, want %q (preserved)", v, "4-2")
	}
}

// TestTFTGameContext_MergeFrom_AllFieldsUnknown 验证 src 全 Unknown 时不覆盖
func TestTFTGameContext_MergeFrom_AllFieldsUnknown(t *testing.T) {
	dst := &TFTGameContext{
		Gold:  game.Known(42),
		HP:    game.Known(67),
		Level: game.Known(7),
		Stage: game.Known("4-2"),
	}
	src := &TFTGameContext{} // 全部 Unknown
	dst.MergeFrom(src)

	// 全部保持
	if v := dst.Gold.OrZero(); v != 42 {
		t.Errorf("Gold = %d, want 42 (preserved)", v)
	}
	if v := dst.HP.OrZero(); v != 67 {
		t.Errorf("HP = %d, want 67 (preserved)", v)
	}
	if v := dst.Level.OrZero(); v != 7 {
		t.Errorf("Level = %d, want 7 (preserved)", v)
	}
	if v := dst.Stage.OrZero(); v != "4-2" {
		t.Errorf("Stage = %q, want %q (preserved)", v, "4-2")
	}
}

// TestTFTGameContext_MergeFrom_AllFieldsKnown 全字段更新
func TestTFTGameContext_MergeFrom_AllFieldsKnown(t *testing.T) {
	dst := &TFTGameContext{
		Gold:  game.Known(42),
		HP:    game.Known(67),
		Level: game.Known(7),
		Stage: game.Known("4-2"),
	}
	src := &TFTGameContext{
		Gold:  game.Detected(50, game.SourceOCR, 0.9),
		HP:    game.Detected(80, game.SourceOCR, 0.9),
		Level: game.Detected(8, game.SourceOCR, 0.9),
		Stage: game.Detected("5-1", game.SourceOCR, 0.9),
	}
	dst.MergeFrom(src)

	if v := dst.Gold.OrZero(); v != 50 {
		t.Errorf("Gold = %d, want 50", v)
	}
	if v := dst.HP.OrZero(); v != 80 {
		t.Errorf("HP = %d, want 80", v)
	}
	if v := dst.Level.OrZero(); v != 8 {
		t.Errorf("Level = %d, want 8", v)
	}
	if v := dst.Stage.OrZero(); v != "5-1" {
		t.Errorf("Stage = %q, want %q", v, "5-1")
	}
}

// TestTFTGameContext_MergeFrom_EconomicFields 验证经济流字段也能增量更新
func TestTFTGameContext_MergeFrom_EconomicFields(t *testing.T) {
	dst := &TFTGameContext{
		Gold:       game.Known(42),
		WinStreak:  game.Known(3),
		LoseStreak: game.Known(0),
	}
	src := &TFTGameContext{
		WinStreak: game.Detected(4, game.SourceOCR, 0.85),
	}
	dst.MergeFrom(src)

	if v := dst.Gold.OrZero(); v != 42 {
		t.Errorf("Gold = %d, want 42 (preserved)", v)
	}
	if v := dst.WinStreak.OrZero(); v != 4 {
		t.Errorf("WinStreak = %d, want 4", v)
	}
	if v := dst.LoseStreak.OrZero(); v != 0 {
		t.Errorf("LoseStreak = %d, want 0 (preserved)", v)
	}
}

// TestTFTGameContext_MergeFrom_NilOther 验证 nil 入参安全
func TestTFTGameContext_MergeFrom_NilOther(t *testing.T) {
	dst := &TFTGameContext{
		Gold: game.Known(42),
	}
	dst.MergeFrom(nil) // 不应 panic
	if v := dst.Gold.OrZero(); v != 42 {
		t.Errorf("Gold = %d, want 42 (preserved after nil merge)", v)
	}
}

// TestTFTGameContext_MergeFrom_TypeMismatch 验证类型不匹配时安全降级
func TestTFTGameContext_MergeFrom_TypeMismatch(t *testing.T) {
	dst := &TFTGameContext{
		Gold: game.Known(42),
	}
	dst.MergeFrom("not a TFTGameContext")
	if v := dst.Gold.OrZero(); v != 42 {
		t.Errorf("Gold = %d, want 42 (preserved on type mismatch)", v)
	}
}

// TestTFTGameContext_ImplementsGameDataMerger 编译期验证 TFTGameContext 实现 GameDataMerger
func TestTFTGameContext_ImplementsGameDataMerger(t *testing.T) {
	var _ game.GameDataMerger = (*TFTGameContext)(nil)
}

// --- Phase 10-8 新增字段的合并测试 ---

// TestTFTGameContext_MergeFrom_XP 验证 XP 字段增量更新
func TestTFTGameContext_MergeFrom_XP(t *testing.T) {
	dst := &TFTGameContext{XP: game.Known(20)}
	src := &TFTGameContext{XP: game.Detected(30, game.SourceOCR, 0.9)}
	dst.MergeFrom(src)
	if v := dst.XP.OrZero(); v != 30 {
		t.Errorf("XP = %d, want 30", v)
	}

	// src 中 XP Unknown → 保留
	dst2 := &TFTGameContext{XP: game.Known(20)}
	src2 := &TFTGameContext{}
	dst2.MergeFrom(src2)
	if v := dst2.XP.OrZero(); v != 20 {
		t.Errorf("XP after Unknown merge = %d, want 20 (preserved)", v)
	}
}

// TestTFTGameContext_MergeFrom_Board 验证 Board 字段增量更新
func TestTFTGameContext_MergeFrom_Board(t *testing.T) {
	// 旧数据：棋盘有 2 个英雄
	old := []BoardSlot{
		{ChampionID: "TFT18_Vayne", Star: 1, Position: 0},
		{ChampionID: "TFT18_Ahri", Star: 2, Position: 1},
	}
	dst := &TFTGameContext{Board: game.Known(old)}

	// 新一轮：棋盘只有 1 个英雄
	newBoard := []BoardSlot{
		{ChampionID: "TFT18_Vayne", Star: 2, Position: 0},
	}
	src := &TFTGameContext{Board: game.Detected(newBoard, game.SourceOCR, 0.8)}
	dst.MergeFrom(src)

	got := dst.Board.OrZero()
	if len(got) != 1 {
		t.Fatalf("Board length = %d, want 1", len(got))
	}
	if got[0].ChampionID != "TFT18_Vayne" || got[0].Star != 2 {
		t.Errorf("Board[0] = %+v, want Vayne with star 2", got[0])
	}
}

// TestTFTGameContext_MergeFrom_EmptyBenchOverwrites 验证空 Bench 覆盖旧数据
//
// 关键不变量：识别到空 Bench 应该清空旧数据，而不是保留。
// Present=true + empty slice 表示"已识别为空"；Present=false 表示"未识别"。
func TestTFTGameContext_MergeFrom_EmptyBenchOverwrites(t *testing.T) {
	// 旧数据：备战席有 2 个英雄
	old := []BenchSlot{
		{ChampionID: "TFT18_Vayne", Star: 1},
		{ChampionID: "TFT18_Ahri", Star: 2},
	}
	dst := &TFTGameContext{Bench: game.Known(old)}

	// 新一轮：识别到备战席为空（empty slice + Present=true）
	src := &TFTGameContext{Bench: game.Detected([]BenchSlot{}, game.SourceOCR, 0.8)}
	dst.MergeFrom(src)

	got := dst.Bench.OrZero()
	if len(got) != 0 {
		t.Errorf("Bench length = %d, want 0 (empty should overwrite)", len(got))
	}
	if !dst.Bench.IsKnown() {
		t.Error("Bench should be marked as known (Present=true)")
	}
}

// TestTFTGameContext_MergeFrom_UnknownBenchPreserves 验证未识别 Bench 保留旧数据
func TestTFTGameContext_MergeFrom_UnknownBenchPreserves(t *testing.T) {
	old := []BenchSlot{
		{ChampionID: "TFT18_Vayne", Star: 1},
	}
	dst := &TFTGameContext{Bench: game.Known(old)}

	// 新一轮：Bench 未识别（Present=false）
	src := &TFTGameContext{Bench: game.Unknown[[]BenchSlot]()}
	dst.MergeFrom(src)

	got := dst.Bench.OrZero()
	if len(got) != 1 {
		t.Errorf("Bench length = %d, want 1 (Unknown should not overwrite)", len(got))
	}
	if got[0].ChampionID != "TFT18_Vayne" {
		t.Errorf("Bench[0].ChampionID = %q, want TFT18_Vayne", got[0].ChampionID)
	}
}

// TestTFTGameContext_MergeFrom_Shop 验证 Shop 字段增量更新 + 空覆盖
func TestTFTGameContext_MergeFrom_Shop(t *testing.T) {
	// 旧：5 格商店
	old := []ShopSlot{
		{ChampionID: "TFT18_Vayne", Cost: 1},
		{ChampionID: "TFT18_Ahri", Cost: 2},
		{ChampionID: "TFT18_Xayah", Cost: 3},
		{ChampionID: "TFT18_Jinx", Cost: 4},
		{ChampionID: "TFT18_Lulu", Cost: 5},
	}
	dst := &TFTGameContext{Shop: game.Known(old)}

	// 新：识别到空商店
	src := &TFTGameContext{Shop: game.Detected([]ShopSlot{}, game.SourceOCR, 0.8)}
	dst.MergeFrom(src)

	got := dst.Shop.OrZero()
	if len(got) != 0 {
		t.Errorf("Shop length = %d, want 0 (empty should overwrite)", len(got))
	}
}

// TestTFTGameContext_MergeFrom_Items 验证 Items 字段增量更新
func TestTFTGameContext_MergeFrom_Items(t *testing.T) {
	dst := &TFTGameContext{Items: game.Known([]ItemSlot{{ItemID: "TFT18_BF", Count: 1}})}
	src := &TFTGameContext{Items: game.Detected([]ItemSlot{
		{ItemID: "TFT18_BF", Count: 1},
		{ItemID: "TFT18_IE", Count: 1},
	}, game.SourceOCR, 0.7)}
	dst.MergeFrom(src)

	got := dst.Items.OrZero()
	if len(got) != 2 {
		t.Errorf("Items length = %d, want 2", len(got))
	}
}

// TestTFTGameContext_MergeFrom_Traits 验证 Traits 字段增量更新
func TestTFTGameContext_MergeFrom_Traits(t *testing.T) {
	dst := &TFTGameContext{Traits: game.Known([]TraitState{
		{TraitID: "Arcanist", Active: 2, Style: 4},
	})}
	src := &TFTGameContext{Traits: game.Detected([]TraitState{
		{TraitID: "Arcanist", Active: 3, Style: 4},
		{TraitID: "Frontline", Active: 2, Style: 2},
	}, game.SourceOCR, 0.7)}
	dst.MergeFrom(src)

	got := dst.Traits.OrZero()
	if len(got) != 2 {
		t.Errorf("Traits length = %d, want 2", len(got))
	}
}

// TestTFTGameContext_MergeFrom_Augments 验证 Augments 字段增量更新
func TestTFTGameContext_MergeFrom_Augments(t *testing.T) {
	dst := &TFTGameContext{Augments: game.Known([]AugmentSlot{
		{AugmentID: "Combat Medic", Tier: "silver"},
	})}
	src := &TFTGameContext{Augments: game.Detected([]AugmentSlot{
		{AugmentID: "Combat Medic", Tier: "silver"},
		{AugmentID: "Built Different", Tier: "gold"},
		{AugmentID: "Cybernetic Implants", Tier: "prismatic"},
	}, game.SourceOCR, 0.8)}
	dst.MergeFrom(src)

	got := dst.Augments.OrZero()
	if len(got) != 3 {
		t.Errorf("Augments length = %d, want 3", len(got))
	}
}

// TestTFTGameContext_MergeFrom_Carousel 验证 Carousel 字段增量更新
func TestTFTGameContext_MergeFrom_Carousel(t *testing.T) {
	old := &CarouselInfo{
		Round: "1-1",
		Offerings: []string{"TFT18_Spat", "TFT18_Tear"},
	}
	dst := &TFTGameContext{Carousel: game.Known(old)}

	newC := &CarouselInfo{
		Round: "1-2",
		Offerings: []string{"TFT18_BF"},
	}
	src := &TFTGameContext{Carousel: game.Detected(newC, game.SourceOCR, 0.7)}
	dst.MergeFrom(src)

	got := dst.Carousel.OrZero()
	if got == nil {
		t.Fatal("Carousel should be non-nil")
	}
	if got.Round != "1-2" {
		t.Errorf("Carousel.Round = %q, want %q", got.Round, "1-2")
	}
	if len(got.Offerings) != 1 {
		t.Errorf("Carousel.Offerings length = %d, want 1", len(got.Offerings))
	}
}

// TestTFTGameContext_MergeFrom_AllNewFields 综合测试 Phase 10-8 所有新字段
func TestTFTGameContext_MergeFrom_AllNewFields(t *testing.T) {
	// 初始：所有 Phase 10-8 字段都 Known
	dst := &TFTGameContext{
		XP:    game.Known(10),
		Board: game.Known([]BoardSlot{{ChampionID: "TFT18_Vayne", Position: 0}}),
		Bench: game.Known([]BenchSlot{{ChampionID: "TFT18_Ahri"}}),
		Shop:  game.Known([]ShopSlot{{ChampionID: "TFT18_Xayah", Cost: 1}}),
		Items: game.Known([]ItemSlot{{ItemID: "TFT18_BF", Count: 1}}),
		Traits: game.Known([]TraitState{{TraitID: "Arcanist", Active: 2, Style: 4}}),
		Augments: game.Known([]AugmentSlot{{AugmentID: "A", Tier: "silver"}}),
	}

	// 第二轮：所有字段 Unknown → 应该全部保留
	src := &TFTGameContext{} // 全部 Unknown
	dst.MergeFrom(src)

	if v := dst.XP.OrZero(); v != 10 {
		t.Errorf("XP = %d, want 10 (preserved)", v)
	}
	if got := dst.Board.OrZero(); len(got) != 1 {
		t.Errorf("Board length = %d, want 1 (preserved)", len(got))
	}
	if got := dst.Bench.OrZero(); len(got) != 1 {
		t.Errorf("Bench length = %d, want 1 (preserved)", len(got))
	}
	if got := dst.Shop.OrZero(); len(got) != 1 {
		t.Errorf("Shop length = %d, want 1 (preserved)", len(got))
	}
	if got := dst.Items.OrZero(); len(got) != 1 {
		t.Errorf("Items length = %d, want 1 (preserved)", len(got))
	}
	if got := dst.Traits.OrZero(); len(got) != 1 {
		t.Errorf("Traits length = %d, want 1 (preserved)", len(got))
	}
	if got := dst.Augments.OrZero(); len(got) != 1 {
		t.Errorf("Augments length = %d, want 1 (preserved)", len(got))
	}
}
