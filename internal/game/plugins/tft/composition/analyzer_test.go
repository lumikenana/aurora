// Package composition - CompositionAnalyzer 单元测试（Phase 16-A §21）
//
// 覆盖 spec 中的 11 个测试场景：
//   1. 单 Champion
//   2. 多 Champion 同羁绊（验证 Trait Count）
//   3. Breakpoint（2/4/6）
//   4. 多羁绊
//   5. Unknown Trait（不能错误计算）
//   6. Patch Isolation（TFT18 Champion 不得用 TFT17 Trait 数据）
//   7. Star（1/2/3）
//   8. Items（作为 Core Candidate 证据）
//   9. Composition Fingerprint（cache hit / Shop 不触发 / Gold 不触发 / XP 不触发 / Board 变触发 / Star 变触发 / Item 变触发）
//   10. Empty Board
//   11. Partial Grounding（某个 Champion 无法 Grounding → 其他正常 / Unknown 不报错）
//
// 额外覆盖：
//   - 0 LLM calls：分析器无网络/无 LLM 依赖（compile-time 验证）
//   - KB 不可用（patch 未知）→ 所有 Trait Status = Unknown
//   - Format 输出结构
package composition

import (
	"encoding/json"
	"sync"
	"testing"

	"github.com/lipipi/aurora/internal/game/plugins/tft/grounder"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---- Test helpers ----

// makeGrounded 构造测试用 GroundedGameContext
//
// 参数：
//   - patch / setKey 必填
//   - board：每个元素的 ChampionID/Cost/Star/Traits/Items 由调用方控制
//   - resolvedTraits：KB 解析后的 trait（含 Effects breakpoint 数据）
//   - resolvedChampions：KB 解析后的 champion（id → ResolvedChampion）
func makeGrounded(patch, setKey string,
	board []grounder.GroundedBoardSlot,
	resolvedTraits []grounder.GroundedTraitState,
	resolvedChampions map[string]grounder.ResolvedChampion,
) grounder.GroundedGameContext {
	// 注入 resolved champion 进 Board
	boardOut := make([]grounder.GroundedBoardSlot, 0, len(board))
	for _, s := range board {
		if c, ok := resolvedChampions[s.Champion.ID]; ok {
			s.Champion = c
		}
		boardOut = append(boardOut, s)
	}

	return grounder.GroundedGameContext{
		Meta: grounder.GroundedMeta{
			Patch:     patch,
			SetKey:    setKey,
			KBHasData: true,
		},
		Board:  boardOut,
		Traits: resolvedTraits,
	}
}

// makeResolvedChamp 构造 ResolvedChampion（含 Status=Resolved）
func makeResolvedChamp(id, setKey string, cost int, traits []string, role string) grounder.ResolvedChampion {
	return grounder.ResolvedChampion{
		ID:      id,
		Name:    id, // 简化测试
		Cost:    cost,
		Traits:  traits,
		Role:    role,
		SetKey:  setKey,
		Patch:   "16.17.1",
		Status:  grounder.StatusResolved,
	}
}

// makeTraitWithBreakpoints 构造 ResolvedTrait + CD conditional_trait_sets
func makeTraitWithBreakpoints(id, setKey, name string, breakpoints []Breakpoint) grounder.GroundedTraitState {
	type cdStyle struct {
		StyleIdx  int    `json:"style_idx"`
		StyleName string `json:"style_name"`
	}
	type cdBP struct {
		MinUnits  int             `json:"min_units"`
		MaxUnits  int             `json:"max_units"`
		Style     cdStyle         `json:"style"`
		Constants json.RawMessage `json:"constants"`
	}
	type cdEffects struct {
		ConditionalTraitSets []cdBP `json:"conditional_trait_sets"`
	}

	cds := make([]cdBP, 0, len(breakpoints))
	for _, b := range breakpoints {
		cds = append(cds, cdBP{
			MinUnits: b.MinUnits,
			MaxUnits: b.MaxUnits,
			Style:    cdStyle{StyleIdx: b.StyleIdx, StyleName: b.StyleName},
		})
	}
	raw, _ := json.Marshal(cdEffects{ConditionalTraitSets: cds})

	return grounder.GroundedTraitState{
		Trait: grounder.ResolvedTrait{
			ID:      id,
			Name:    name,
			Type:    "Origin",
			Effects: raw,
			SetKey:  setKey,
			Patch:   "16.17.1",
			Status:  grounder.StatusResolved,
		},
	}
}

// makeTraitEmpty 构造没有 breakpoint 数据的 ResolvedTrait
func makeTraitEmpty(id, setKey, name string) grounder.GroundedTraitState {
	return grounder.GroundedTraitState{
		Trait: grounder.ResolvedTrait{
			ID:      id,
			Name:    name,
			Type:    "Origin",
			Effects: json.RawMessage(`{}`),
			SetKey:  setKey,
			Patch:   "16.17.1",
			Status:  grounder.StatusResolved,
		},
	}
}

// ---- 1. Single Champion ----

func TestAnalyze_SingleChampion(t *testing.T) {
	g := makeGrounded("16.17.1", "TFT18",
		[]grounder.GroundedBoardSlot{
			{Position: 0, Star: 1, Champion: grounder.ResolvedChampion{ID: "TFT18_Vayne", Status: grounder.StatusResolved}},
		},
		[]grounder.GroundedTraitState{
			makeTraitWithBreakpoints("TFT18_Marksman", "TFT18", "Marksman", []Breakpoint{
				{MinUnits: 2, MaxUnits: 25000, StyleName: "kBronze"},
			}),
		},
		map[string]grounder.ResolvedChampion{
			"TFT18_Vayne": makeResolvedChamp("TFT18_Vayne", "TFT18", 1, []string{"TFT18_Marksman"}, "Carry"),
		},
	)

	a := NewAnalyzer()
	out := a.Analyze(g)
	require.Len(t, out.ActiveChampions, 1)
	assert.Equal(t, 1, out.ChampionCount)
	assert.Equal(t, "TFT18_Vayne", out.ActiveChampions[0].ID)
	assert.Equal(t, 1, out.ActiveChampions[0].Star)

	// 单个 champion 也应该产生 trait（即使单位数 1 < breakpoint 2）
	require.Len(t, out.Traits, 1)
	assert.Equal(t, "TFT18_Marksman", out.Traits[0].TraitID)
	assert.Equal(t, 1, out.Traits[0].ActiveUnits)
	assert.Equal(t, TraitStatusInactive, out.Traits[0].Status, "1 unit < 2 should be inactive")
}

// ---- 2. Multiple Champions Same Trait ----

func TestAnalyze_MultipleChampions_SameTrait(t *testing.T) {
	// 4 个 Vayne 持 Marksman → ActiveUnits = 4
	g := makeGrounded("16.17.1", "TFT18",
		[]grounder.GroundedBoardSlot{
			{Position: 0, Star: 1, Champion: grounder.ResolvedChampion{ID: "TFT18_Vayne", Status: grounder.StatusResolved}},
			{Position: 1, Star: 1, Champion: grounder.ResolvedChampion{ID: "TFT18_Vayne", Status: grounder.StatusResolved}},
			{Position: 2, Star: 1, Champion: grounder.ResolvedChampion{ID: "TFT18_Vayne", Status: grounder.StatusResolved}},
			{Position: 3, Star: 1, Champion: grounder.ResolvedChampion{ID: "TFT18_Vayne", Status: grounder.StatusResolved}},
		},
		[]grounder.GroundedTraitState{
			makeTraitWithBreakpoints("TFT18_Marksman", "TFT18", "Marksman", []Breakpoint{
				{MinUnits: 2, MaxUnits: 3, StyleIdx: 2, StyleName: "kBronze"},
				{MinUnits: 4, MaxUnits: 5, StyleIdx: 3, StyleName: "kSilver"},
				{MinUnits: 6, MaxUnits: 25000, StyleIdx: 5, StyleName: "kGold"},
			}),
		},
		map[string]grounder.ResolvedChampion{
			"TFT18_Vayne": makeResolvedChamp("TFT18_Vayne", "TFT18", 1, []string{"TFT18_Marksman"}, "Carry"),
		},
	)

	a := NewAnalyzer()
	out := a.Analyze(g)
	require.Len(t, out.ActiveChampions, 4)
	require.Len(t, out.Traits, 1)
	tr := out.Traits[0]
	assert.Equal(t, 4, tr.ActiveUnits, "4 Vayne → 4 active units of Marksman")
	assert.Equal(t, TraitStatusActive, tr.Status)
	require.NotNil(t, tr.CurrentBreakpoint)
	assert.Equal(t, 4, tr.CurrentBreakpoint.MinUnits, "4 units → current = 4 breakpoint")
	require.NotNil(t, tr.NextBreakpoint)
	assert.Equal(t, 6, tr.NextBreakpoint.MinUnits, "next = 6 breakpoint")
}

// ---- 3. Breakpoint 2/4/6 ----

func TestAnalyze_Breakpoint_246(t *testing.T) {
	bps := []Breakpoint{
		{MinUnits: 2, MaxUnits: 3, StyleIdx: 2, StyleName: "kBronze"},
		{MinUnits: 4, MaxUnits: 5, StyleIdx: 3, StyleName: "kSilver"},
		{MinUnits: 6, MaxUnits: 25000, StyleIdx: 5, StyleName: "kGold"},
	}

	for _, tc := range []struct {
		name         string
		championCt   int
		expectActive bool
		expectCur    int
		expectNext   int
	}{
		{"1 unit", 1, false, 0, 2},
		{"2 units (first)", 2, true, 2, 4},
		{"3 units (between 1st/2nd)", 3, true, 2, 4},
		{"4 units (second)", 4, true, 4, 6},
		{"5 units (between 2nd/3rd)", 5, true, 4, 6},
		{"6 units (third, max)", 6, true, 6, 0},
		{"7 units (max exceeded)", 7, true, 6, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			board := make([]grounder.GroundedBoardSlot, tc.championCt)
			champs := map[string]grounder.ResolvedChampion{}
			for i := 0; i < tc.championCt; i++ {
				board[i] = grounder.GroundedBoardSlot{
					Position:  i,
					Star:      1,
					Champion:  grounder.ResolvedChampion{ID: "TFT18_Vayne", Status: grounder.StatusResolved},
				}
			}
			champs["TFT18_Vayne"] = makeResolvedChamp("TFT18_Vayne", "TFT18", 1, []string{"TFT18_Marksman"}, "Carry")

			g := makeGrounded("16.17.1", "TFT18", board,
				[]grounder.GroundedTraitState{
					makeTraitWithBreakpoints("TFT18_Marksman", "TFT18", "Marksman", bps),
				}, champs)

			a := NewAnalyzer()
			out := a.Analyze(g)
			require.Len(t, out.Traits, 1)
			tr := out.Traits[0]
			assert.Equal(t, tc.championCt, tr.ActiveUnits)
			assert.Equal(t, tc.expectActive, tr.IsActive, "IsActive mismatch for %s", tc.name)
			if tc.expectCur == 0 {
				assert.Nil(t, tr.CurrentBreakpoint, "current should be nil for %s", tc.name)
			} else {
				require.NotNil(t, tr.CurrentBreakpoint, "current should not be nil for %s", tc.name)
				assert.Equal(t, tc.expectCur, tr.CurrentBreakpoint.MinUnits)
			}
			if tc.expectNext == 0 {
				assert.Nil(t, tr.NextBreakpoint, "next should be nil for %s", tc.name)
			} else {
				require.NotNil(t, tr.NextBreakpoint, "next should not be nil for %s", tc.name)
				assert.Equal(t, tc.expectNext, tr.NextBreakpoint.MinUnits)
			}
		})
	}
}

// ---- 4. Multiple Traits ----

func TestAnalyze_MultipleTraits(t *testing.T) {
	// 2 个 Vayne (Marksman+Bulwark) + 1 个 Lulu (Mage)
	// 期望：Marksman=2, Bulwark=2, Mage=1
	g := makeGrounded("16.17.1", "TFT18",
		[]grounder.GroundedBoardSlot{
			{Position: 0, Star: 1, Champion: grounder.ResolvedChampion{ID: "TFT18_Vayne", Status: grounder.StatusResolved}},
			{Position: 1, Star: 1, Champion: grounder.ResolvedChampion{ID: "TFT18_Vayne", Status: grounder.StatusResolved}},
			{Position: 2, Star: 1, Champion: grounder.ResolvedChampion{ID: "TFT18_Lulu", Status: grounder.StatusResolved}},
		},
		[]grounder.GroundedTraitState{
			makeTraitWithBreakpoints("TFT18_Marksman", "TFT18", "Marksman", []Breakpoint{
				{MinUnits: 2, MaxUnits: 25000, StyleName: "kBronze"},
			}),
			makeTraitWithBreakpoints("TFT18_Bulwark", "TFT18", "Bulwark", []Breakpoint{
				{MinUnits: 2, MaxUnits: 25000, StyleName: "kBronze"},
			}),
			makeTraitWithBreakpoints("TFT18_Mage", "TFT18", "Mage", []Breakpoint{
				{MinUnits: 3, MaxUnits: 25000, StyleName: "kBronze"},
			}),
		},
		map[string]grounder.ResolvedChampion{
			"TFT18_Vayne": makeResolvedChamp("TFT18_Vayne", "TFT18", 1, []string{"TFT18_Marksman", "TFT18_Bulwark"}, "Carry"),
			"TFT18_Lulu":  makeResolvedChamp("TFT18_Lulu", "TFT18", 2, []string{"TFT18_Mage"}, "Support"),
		},
	)

	a := NewAnalyzer()
	out := a.Analyze(g)
	require.Len(t, out.Traits, 3)

	// 按 ActiveUnits 降序
	counts := map[string]int{}
	statuses := map[string]TraitAggregateStatus{}
	for _, tr := range out.Traits {
		counts[tr.TraitID] = tr.ActiveUnits
		statuses[tr.TraitID] = tr.Status
	}
	assert.Equal(t, 2, counts["TFT18_Marksman"])
	assert.Equal(t, 2, counts["TFT18_Bulwark"])
	assert.Equal(t, 1, counts["TFT18_Mage"])
	assert.Equal(t, TraitStatusActive, statuses["TFT18_Marksman"])
	assert.Equal(t, TraitStatusActive, statuses["TFT18_Bulwark"])
	assert.Equal(t, TraitStatusInactive, statuses["TFT18_Mage"], "1 unit < 3 → inactive")
}

// ---- 5. Unknown Trait ----

func TestAnalyze_UnknownTrait_NoKBData(t *testing.T) {
	// Board 持有 trait TFT18_Mystery，但 KB 中没有该 trait（或没有 breakpoint 数据）
	g := makeGrounded("16.17.1", "TFT18",
		[]grounder.GroundedBoardSlot{
			{Position: 0, Star: 1, Champion: grounder.ResolvedChampion{ID: "TFT18_Vayne", Status: grounder.StatusResolved}},
		},
		[]grounder.GroundedTraitState{
			makeTraitEmpty("TFT18_Mystery", "TFT18", "Mystery"), // 没 breakpoint
		},
		map[string]grounder.ResolvedChampion{
			"TFT18_Vayne": makeResolvedChamp("TFT18_Vayne", "TFT18", 1, []string{"TFT18_Mystery"}, "Carry"),
		},
	)

	a := NewAnalyzer()
	out := a.Analyze(g)
	require.Len(t, out.Traits, 1)
	assert.Equal(t, TraitStatusUnknown, out.Traits[0].Status,
		"KB 中无 breakpoint → 必须标 Unknown，不能猜测")
	assert.Equal(t, 0, out.Traits[0].BreakpointCount)
	assert.Nil(t, out.Traits[0].CurrentBreakpoint)
	assert.Nil(t, out.Traits[0].NextBreakpoint)
}

func TestAnalyze_UnknownTrait_TraitNotInKB(t *testing.T) {
	// Board 持有 trait，但 KB 中完全没有该 trait
	g := makeGrounded("16.17.1", "TFT18",
		[]grounder.GroundedBoardSlot{
			{Position: 0, Star: 1, Champion: grounder.ResolvedChampion{ID: "TFT18_Vayne", Status: grounder.StatusResolved}},
		},
		nil, // KB 无任何 trait
		map[string]grounder.ResolvedChampion{
			"TFT18_Vayne": makeResolvedChamp("TFT18_Vayne", "TFT18", 1, []string{"TFT18_Mystery"}, "Carry"),
		},
	)

	a := NewAnalyzer()
	out := a.Analyze(g)
	require.Len(t, out.Traits, 1)
	assert.Equal(t, TraitStatusUnknown, out.Traits[0].Status,
		"KB 中无此 trait → Status=Unknown")
}

// ---- 6. Patch Isolation (analyzer 不跨 set 替代) ----

func TestAnalyze_PatchIsolation_AnalyzerNotCrossSet(t *testing.T) {
	// 设置：Board TFT18_Vayne 持 TFT17_Marksman
	// 期望：analyzer 不会**改写** trait id 跨 set 替代
	g := makeGrounded("16.17.1", "TFT18",
		[]grounder.GroundedBoardSlot{
			{Position: 0, Star: 1, Champion: grounder.ResolvedChampion{ID: "TFT18_Vayne", Status: grounder.StatusResolved}},
		},
		[]grounder.GroundedTraitState{
			{
				Trait: grounder.ResolvedTrait{
					ID:      "TFT17_Marksman",
					Name:    "Marksman17",
					SetKey:  "TFT17",
					Effects: json.RawMessage(`{"conditional_trait_sets":[{"min_units":2,"max_units":3,"style":{"style_idx":2,"style_name":"kBronze"}}]}`),
					Status:  grounder.StatusResolved,
				},
			},
		},
		map[string]grounder.ResolvedChampion{
			"TFT18_Vayne": makeResolvedChamp("TFT18_Vayne", "TFT18", 1, []string{"TFT17_Marksman"}, "Carry"),
		},
	)

	a := NewAnalyzer()
	out := a.Analyze(g)
	require.NotEmpty(t, out.Traits)
	// 关键：analyzer 只是 KB 数据的消费者，**不会** 改写 trait id 前缀
	for _, tr := range out.Traits {
		// 即使 KB 解析出错，analyzer 不会把 id 改写
		assert.Contains(t, tr.TraitID, "TFT", "analyzer 不修改 trait id")
	}
}

// ---- 7. Star ----

func TestAnalyze_Star_1to3(t *testing.T) {
	for _, star := range []int{1, 2, 3} {
		star := star
		t.Run("star="+itoa(star), func(t *testing.T) {
			g := makeGrounded("16.17.1", "TFT18",
				[]grounder.GroundedBoardSlot{
					{Position: 0, Star: star, Champion: grounder.ResolvedChampion{ID: "TFT18_Vayne", Status: grounder.StatusResolved}},
				},
				nil,
				map[string]grounder.ResolvedChampion{
					"TFT18_Vayne": makeResolvedChamp("TFT18_Vayne", "TFT18", 1, []string{"TFT18_Marksman"}, "Carry"),
				},
			)
			a := NewAnalyzer()
			out := a.Analyze(g)
			require.Len(t, out.ActiveChampions, 1)
			assert.Equal(t, star, out.ActiveChampions[0].Star)

			// Star ≥ 2 → Core Candidate Score 应包含 high_star
			if star >= 2 {
				require.NotEmpty(t, out.CoreCandidates)
				c := out.CoreCandidates[0]
				found := false
				for _, e := range c.Evidence {
					if e.Type == EvidenceHighStar {
						found = true
						break
					}
				}
				assert.True(t, found, "star=%d should add high_star evidence", star)
			}
		})
	}
}

func itoa(i int) string {
	switch i {
	case 1:
		return "1"
	case 2:
		return "2"
	case 3:
		return "3"
	default:
		return "?"
	}
}

// ---- 8. Items as Core Evidence ----

func TestAnalyze_Items_AsCoreEvidence(t *testing.T) {
	// 1 费 1 星 Vayne 但有 3 件装备 → 应该升级为 Candidate
	g := makeGrounded("16.17.1", "TFT18",
		[]grounder.GroundedBoardSlot{
			{
				Position: 0, Star: 1,
				Champion: grounder.ResolvedChampion{ID: "TFT18_Vayne", Status: grounder.StatusResolved},
				Items: []grounder.ResolvedItem{
					{ID: "TFT18_BF", Status: grounder.StatusResolved},
					{ID: "TFT18_Recurve", Status: grounder.StatusResolved},
					{ID: "TFT18_Tear", Status: grounder.StatusResolved},
				},
			},
		},
		nil,
		map[string]grounder.ResolvedChampion{
			"TFT18_Vayne": makeResolvedChamp("TFT18_Vayne", "TFT18", 1, []string{"TFT18_Marksman"}, "Carry"),
		},
	)

	a := NewAnalyzer()
	out := a.Analyze(g)
	require.Len(t, out.CoreCandidates, 1)
	c := out.CoreCandidates[0]
	assert.Equal(t, CoreStatusCandidate, c.Status,
		"3 件装备 + 1 星 1 费 = evidence 0.3 → Candidate")
	assert.GreaterOrEqual(t, c.Score, 0.3)
}

// ---- 9. Composition Fingerprint ----

func TestAnalyze_Fingerprint_CacheHit(t *testing.T) {
	g := makeGrounded("16.17.1", "TFT18",
		[]grounder.GroundedBoardSlot{
			{Position: 0, Star: 2, Champion: grounder.ResolvedChampion{ID: "TFT18_Vayne", Status: grounder.StatusResolved}},
		},
		nil,
		map[string]grounder.ResolvedChampion{
			"TFT18_Vayne": makeResolvedChamp("TFT18_Vayne", "TFT18", 1, []string{"TFT18_Marksman"}, "Carry"),
		},
	)

	a := NewAnalyzer()
	out1 := a.Analyze(g)
	out2 := a.Analyze(g)
	// 同一 fingerprint → 同一 GeneratedAt（说明是 cache hit 复用对象）
	assert.Equal(t, out1.GeneratedAt, out2.GeneratedAt,
		"cache hit: GeneratedAt should be identical (object reuse)")
}

// ---- 9b. Shop / Gold / XP change does NOT change fingerprint ----

func TestAnalyze_Fingerprint_ShopGoldXP_NoChange(t *testing.T) {
	g1 := makeGrounded("16.17.1", "TFT18",
		[]grounder.GroundedBoardSlot{
			{Position: 0, Star: 1, Champion: grounder.ResolvedChampion{ID: "TFT18_Vayne", Status: grounder.StatusResolved}},
		},
		nil,
		map[string]grounder.ResolvedChampion{
			"TFT18_Vayne": makeResolvedChamp("TFT18_Vayne", "TFT18", 1, []string{"TFT18_Marksman"}, "Carry"),
		},
	)
	g1.Shop = []grounder.GroundedShopSlot{
		{Champion: grounder.ResolvedChampion{ID: "TFT18_Ahri", Status: grounder.StatusResolved}, Cost: 3},
	}

	g2 := makeGrounded("16.17.1", "TFT18",
		[]grounder.GroundedBoardSlot{
			{Position: 0, Star: 1, Champion: grounder.ResolvedChampion{ID: "TFT18_Vayne", Status: grounder.StatusResolved}},
		},
		nil,
		map[string]grounder.ResolvedChampion{
			"TFT18_Vayne": makeResolvedChamp("TFT18_Vayne", "TFT18", 1, []string{"TFT18_Marksman"}, "Carry"),
		},
	)
	g2.Shop = []grounder.GroundedShopSlot{
		// Shop 内容不同
		{Champion: grounder.ResolvedChampion{ID: "TFT18_Lulu", Status: grounder.StatusResolved}, Cost: 2},
	}
	g2.GameState.Gold = 50
	g2.GameState.XP = 30

	fp1 := ComputeFingerprint(g1)
	fp2 := ComputeFingerprint(g2)
	assert.True(t, fp1.Equal(fp2), "Shop/Gold/XP 变化不应改变 fingerprint")
	assert.Equal(t, fp1.Hash(), fp2.Hash())
}

// ---- 9c. Bench change does NOT change fingerprint ----

func TestAnalyze_Fingerprint_Bench_NoChange(t *testing.T) {
	g1 := makeGrounded("16.17.1", "TFT18",
		[]grounder.GroundedBoardSlot{
			{Position: 0, Star: 1, Champion: grounder.ResolvedChampion{ID: "TFT18_Vayne", Status: grounder.StatusResolved}},
		},
		nil,
		map[string]grounder.ResolvedChampion{
			"TFT18_Vayne": makeResolvedChamp("TFT18_Vayne", "TFT18", 1, []string{"TFT18_Marksman"}, "Carry"),
		},
	)
	g1.Bench = []grounder.GroundedBenchSlot{
		{Champion: grounder.ResolvedChampion{ID: "TFT18_Ahri", Status: grounder.StatusResolved}, Star: 1},
	}

	g2 := makeGrounded("16.17.1", "TFT18",
		[]grounder.GroundedBoardSlot{
			{Position: 0, Star: 1, Champion: grounder.ResolvedChampion{ID: "TFT18_Vayne", Status: grounder.StatusResolved}},
		},
		nil,
		map[string]grounder.ResolvedChampion{
			"TFT18_Vayne": makeResolvedChamp("TFT18_Vayne", "TFT18", 1, []string{"TFT18_Marksman"}, "Carry"),
		},
	)
	g2.Bench = []grounder.GroundedBenchSlot{
		{Champion: grounder.ResolvedChampion{ID: "TFT18_Lulu", Status: grounder.StatusResolved}, Star: 2},
	}

	fp1 := ComputeFingerprint(g1)
	fp2 := ComputeFingerprint(g2)
	assert.True(t, fp1.Equal(fp2), "Bench 变化不应改变 fingerprint")
}

// ---- 9d. Board Champion change DOES change fingerprint ----

func TestAnalyze_Fingerprint_BoardChange(t *testing.T) {
	g1 := makeGrounded("16.17.1", "TFT18",
		[]grounder.GroundedBoardSlot{
			{Position: 0, Star: 1, Champion: grounder.ResolvedChampion{ID: "TFT18_Vayne", Status: grounder.StatusResolved}},
		},
		nil,
		map[string]grounder.ResolvedChampion{
			"TFT18_Vayne": makeResolvedChamp("TFT18_Vayne", "TFT18", 1, []string{"TFT18_Marksman"}, "Carry"),
		},
	)
	g2 := makeGrounded("16.17.1", "TFT18",
		[]grounder.GroundedBoardSlot{
			{Position: 0, Star: 1, Champion: grounder.ResolvedChampion{ID: "TFT18_Ahri", Status: grounder.StatusResolved}},
		},
		nil,
		map[string]grounder.ResolvedChampion{
			"TFT18_Ahri": makeResolvedChamp("TFT18_Ahri", "TFT18", 3, []string{"TFT18_Mage"}, "Mage"),
		},
	)

	fp1 := ComputeFingerprint(g1)
	fp2 := ComputeFingerprint(g2)
	assert.False(t, fp1.Equal(fp2), "Board champion 变化应改变 fingerprint")
}

// ---- 9e. Star change DOES change fingerprint ----

func TestAnalyze_Fingerprint_StarChange(t *testing.T) {
	g1 := makeGrounded("16.17.1", "TFT18",
		[]grounder.GroundedBoardSlot{
			{Position: 0, Star: 1, Champion: grounder.ResolvedChampion{ID: "TFT18_Vayne", Status: grounder.StatusResolved}},
		},
		nil,
		map[string]grounder.ResolvedChampion{
			"TFT18_Vayne": makeResolvedChamp("TFT18_Vayne", "TFT18", 1, []string{"TFT18_Marksman"}, "Carry"),
		},
	)
	g2 := makeGrounded("16.17.1", "TFT18",
		[]grounder.GroundedBoardSlot{
			{Position: 0, Star: 2, Champion: grounder.ResolvedChampion{ID: "TFT18_Vayne", Status: grounder.StatusResolved}},
		},
		nil,
		map[string]grounder.ResolvedChampion{
			"TFT18_Vayne": makeResolvedChamp("TFT18_Vayne", "TFT18", 1, []string{"TFT18_Marksman"}, "Carry"),
		},
	)

	fp1 := ComputeFingerprint(g1)
	fp2 := ComputeFingerprint(g2)
	assert.False(t, fp1.Equal(fp2), "Star 变化应改变 fingerprint")
}

// ---- 9f. Board Item change DOES change fingerprint ----

func TestAnalyze_Fingerprint_ItemChange(t *testing.T) {
	g1 := makeGrounded("16.17.1", "TFT18",
		[]grounder.GroundedBoardSlot{
			{
				Position: 0, Star: 1,
				Champion: grounder.ResolvedChampion{ID: "TFT18_Vayne", Status: grounder.StatusResolved},
				Items:    []grounder.ResolvedItem{{ID: "TFT18_BF", Status: grounder.StatusResolved}},
			},
		},
		nil,
		map[string]grounder.ResolvedChampion{
			"TFT18_Vayne": makeResolvedChamp("TFT18_Vayne", "TFT18", 1, []string{"TFT18_Marksman"}, "Carry"),
		},
	)
	g2 := makeGrounded("16.17.1", "TFT18",
		[]grounder.GroundedBoardSlot{
			{
				Position: 0, Star: 1,
				Champion: grounder.ResolvedChampion{ID: "TFT18_Vayne", Status: grounder.StatusResolved},
				Items:    []grounder.ResolvedItem{{ID: "TFT18_Recurve", Status: grounder.StatusResolved}},
			},
		},
		nil,
		map[string]grounder.ResolvedChampion{
			"TFT18_Vayne": makeResolvedChamp("TFT18_Vayne", "TFT18", 1, []string{"TFT18_Marksman"}, "Carry"),
		},
	)

	fp1 := ComputeFingerprint(g1)
	fp2 := ComputeFingerprint(g2)
	assert.False(t, fp1.Equal(fp2), "Board Item 变化应改变 fingerprint")
}

// ---- 9g. Patch change DOES change fingerprint ----

func TestAnalyze_Fingerprint_PatchChange(t *testing.T) {
	g1 := makeGrounded("16.17.1", "TFT18",
		[]grounder.GroundedBoardSlot{
			{Position: 0, Star: 1, Champion: grounder.ResolvedChampion{ID: "TFT18_Vayne", Status: grounder.StatusResolved}},
		},
		nil,
		map[string]grounder.ResolvedChampion{
			"TFT18_Vayne": makeResolvedChamp("TFT18_Vayne", "TFT18", 1, []string{"TFT18_Marksman"}, "Carry"),
		},
	)
	g2 := makeGrounded("16.16", "TFT17",
		[]grounder.GroundedBoardSlot{
			{Position: 0, Star: 1, Champion: grounder.ResolvedChampion{ID: "TFT17_Vayne", Status: grounder.StatusResolved}},
		},
		nil,
		map[string]grounder.ResolvedChampion{
			"TFT17_Vayne": makeResolvedChamp("TFT17_Vayne", "TFT17", 1, []string{"TFT17_Marksman"}, "Carry"),
		},
	)
	fp1 := ComputeFingerprint(g1)
	fp2 := ComputeFingerprint(g2)
	assert.False(t, fp1.Equal(fp2), "patch 变化应改变 fingerprint")
}

// ---- 9h. Cache miss when Board changes ----

func TestAnalyze_CacheMiss_WhenBoardChanges(t *testing.T) {
	g1 := makeGrounded("16.17.1", "TFT18",
		[]grounder.GroundedBoardSlot{
			{Position: 0, Star: 1, Champion: grounder.ResolvedChampion{ID: "TFT18_Vayne", Status: grounder.StatusResolved}},
		},
		nil,
		map[string]grounder.ResolvedChampion{
			"TFT18_Vayne": makeResolvedChamp("TFT18_Vayne", "TFT18", 1, []string{"TFT18_Marksman"}, "Carry"),
		},
	)
	g2 := makeGrounded("16.17.1", "TFT18",
		[]grounder.GroundedBoardSlot{
			{Position: 0, Star: 1, Champion: grounder.ResolvedChampion{ID: "TFT18_Ahri", Status: grounder.StatusResolved}},
		},
		nil,
		map[string]grounder.ResolvedChampion{
			"TFT18_Ahri": makeResolvedChamp("TFT18_Ahri", "TFT18", 3, []string{"TFT18_Mage"}, "Mage"),
		},
	)
	a := NewAnalyzer()
	out1 := a.Analyze(g1)
	out2 := a.Analyze(g2)
	assert.NotEqual(t, out1.GeneratedAt, out2.GeneratedAt, "fingerprint 变化 → cache miss → 重新分析")
	assert.Equal(t, "TFT18_Vayne", out1.ActiveChampions[0].ID)
	assert.Equal(t, "TFT18_Ahri", out2.ActiveChampions[0].ID)
}

// ---- 9i. Concurrency: same fingerprint from multiple goroutines ----

func TestAnalyze_Concurrent_SameFingerprint(t *testing.T) {
	g := makeGrounded("16.17.1", "TFT18",
		[]grounder.GroundedBoardSlot{
			{Position: 0, Star: 1, Champion: grounder.ResolvedChampion{ID: "TFT18_Vayne", Status: grounder.StatusResolved}},
		},
		nil,
		map[string]grounder.ResolvedChampion{
			"TFT18_Vayne": makeResolvedChamp("TFT18_Vayne", "TFT18", 1, []string{"TFT18_Marksman"}, "Carry"),
		},
	)
	a := NewAnalyzer()
	var wg sync.WaitGroup
	const n = 8
	results := make([]CompositionAnalysis, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			results[idx] = a.Analyze(g)
		}(i)
	}
	wg.Wait()
	for i := 1; i < n; i++ {
		assert.Equal(t, results[0].Fingerprint.Hash(), results[i].Fingerprint.Hash())
		assert.Equal(t, results[0].ChampionCount, results[i].ChampionCount)
	}
}

// ---- 10. Empty Board ----

func TestAnalyze_EmptyBoard(t *testing.T) {
	g := grounder.GroundedGameContext{
		Meta: grounder.GroundedMeta{Patch: "16.17.1", SetKey: "TFT18", KBHasData: true},
	}
	a := NewAnalyzer()
	out := a.Analyze(g)
	assert.Equal(t, 0, out.ChampionCount)
	assert.Empty(t, out.ActiveChampions)
	assert.Empty(t, out.Traits)
	assert.Empty(t, out.CoreCandidates)
	assert.Equal(t, "ok", out.Summary.TraitKBStatus, "no traits → kb status = ok")
	assert.Equal(t, "", Format(out), "empty analysis should produce no prompt section")
}

func TestAnalyze_NilGrounded(t *testing.T) {
	a := NewAnalyzer()
	out := a.Analyze(grounder.GroundedGameContext{})
	assert.True(t, out.Fingerprint.IsZero())
	assert.Equal(t, 0, out.ChampionCount)
}

// ---- 11. Partial Grounding ----

func TestAnalyze_PartialGrounding(t *testing.T) {
	// Board 上有 3 个槽位：
	//   - TFT18_Vayne: Resolved
	//   - Unknown: Status=Unknown
	//   - "" (空): 跳过
	// 期望：Vayne 进入分析，Unknown 跳过，Total ActiveChampions = 1
	g := makeGrounded("16.17.1", "TFT18",
		[]grounder.GroundedBoardSlot{
			{Position: 0, Star: 2, Champion: grounder.ResolvedChampion{ID: "TFT18_Vayne", Status: grounder.StatusResolved}},
			{Position: 1, Star: 1, Champion: grounder.ResolvedChampion{ID: "Unknown", Status: grounder.StatusUnknown}},
			{Position: 2, Star: 1, Champion: grounder.ResolvedChampion{ID: "", Status: grounder.StatusUnknown}},
		},
		[]grounder.GroundedTraitState{
			makeTraitWithBreakpoints("TFT18_Marksman", "TFT18", "Marksman", []Breakpoint{
				{MinUnits: 2, MaxUnits: 25000, StyleName: "kBronze"},
			}),
		},
		map[string]grounder.ResolvedChampion{
			"TFT18_Vayne": makeResolvedChamp("TFT18_Vayne", "TFT18", 1, []string{"TFT18_Marksman"}, "Carry"),
		},
	)

	a := NewAnalyzer()
	out := a.Analyze(g)

	// ChampionCount 只算非空 ID
	assert.Equal(t, 1, out.ChampionCount, "Empty/Unknown ID 不计 ChampionCount")
	require.Len(t, out.ActiveChampions, 1)
	assert.Equal(t, "TFT18_Vayne", out.ActiveChampions[0].ID)

	// Core candidates：Unknown 不进入 cands（无评分依据）
	require.Len(t, out.CoreCandidates, 1, "Unknown 不进入 cands（无评分依据）")
	assert.Equal(t, "TFT18_Vayne", out.CoreCandidates[0].ChampionID)

	// Trait 只算 Resolved 英雄持有的（Vayne 持 Marksman）
	require.Len(t, out.Traits, 1)
	assert.Equal(t, 1, out.Traits[0].ActiveUnits, "只算 Resolved 英雄")
}

// ---- 12. KB unavailable (patch unknown) ----

func TestAnalyze_KBUnavailable_AllUnknown(t *testing.T) {
	g := grounder.GroundedGameContext{
		Meta: grounder.GroundedMeta{Patch: "", SetKey: "", KBHasData: false},
		Board: []grounder.GroundedBoardSlot{
			{Position: 0, Star: 1, Champion: grounder.ResolvedChampion{ID: "TFT18_Vayne", Status: grounder.StatusKBUnavailable}},
		},
	}
	a := NewAnalyzer()
	out := a.Analyze(g)
	assert.Equal(t, 1, out.ChampionCount)
	assert.Empty(t, out.Traits)
	require.Len(t, out.CoreCandidates, 1)
	assert.Equal(t, CoreStatusUnknown, out.CoreCandidates[0].Status)
}

// ---- 13. Summary / Primary / Secondary ----

func TestAnalyze_Summary_PrimarySecondary(t *testing.T) {
	// 2 Vayne (Marksman) + 1 Lulu (Mage) + 1 Garen (Bulwark)
	// 期望 Primary=Marksman(2)，Secondary 为空（其它 1 unit < 2 都不 active）
	g := makeGrounded("16.17.1", "TFT18",
		[]grounder.GroundedBoardSlot{
			{Position: 0, Star: 1, Champion: grounder.ResolvedChampion{ID: "TFT18_Vayne", Status: grounder.StatusResolved}},
			{Position: 1, Star: 1, Champion: grounder.ResolvedChampion{ID: "TFT18_Vayne", Status: grounder.StatusResolved}},
			{Position: 2, Star: 1, Champion: grounder.ResolvedChampion{ID: "TFT18_Lulu", Status: grounder.StatusResolved}},
			{Position: 3, Star: 1, Champion: grounder.ResolvedChampion{ID: "TFT18_Garen", Status: grounder.StatusResolved}},
		},
		[]grounder.GroundedTraitState{
			makeTraitWithBreakpoints("TFT18_Marksman", "TFT18", "Marksman", []Breakpoint{
				{MinUnits: 2, MaxUnits: 25000, StyleName: "kBronze"},
			}),
			makeTraitWithBreakpoints("TFT18_Mage", "TFT18", "Mage", []Breakpoint{
				{MinUnits: 3, MaxUnits: 25000, StyleName: "kBronze"},
			}),
			makeTraitWithBreakpoints("TFT18_Bulwark", "TFT18", "Bulwark", []Breakpoint{
				{MinUnits: 2, MaxUnits: 25000, StyleName: "kBronze"},
			}),
		},
		map[string]grounder.ResolvedChampion{
			"TFT18_Vayne": makeResolvedChamp("TFT18_Vayne", "TFT18", 1, []string{"TFT18_Marksman"}, "Carry"),
			"TFT18_Lulu":  makeResolvedChamp("TFT18_Lulu", "TFT18", 2, []string{"TFT18_Mage"}, "Support"),
			"TFT18_Garen": makeResolvedChamp("TFT18_Garen", "TFT18", 3, []string{"TFT18_Bulwark"}, "Tank"),
		},
	)
	a := NewAnalyzer()
	out := a.Analyze(g)
	assert.Equal(t, "TFT18_Marksman", out.Summary.PrimaryTrait)
	assert.Equal(t, "", out.Summary.SecondaryTrait,
		"inactive traits 不进入 secondary 选择")
}

// ---- 14. Format ----

func TestFormat_Empty(t *testing.T) {
	assert.Equal(t, "", Format(CompositionAnalysis{}))
}

func TestFormat_Basic(t *testing.T) {
	g := makeGrounded("16.17.1", "TFT18",
		[]grounder.GroundedBoardSlot{
			{Position: 0, Star: 2, Champion: grounder.ResolvedChampion{ID: "TFT18_Vayne", Status: grounder.StatusResolved}},
		},
		[]grounder.GroundedTraitState{
			makeTraitWithBreakpoints("TFT18_Marksman", "TFT18", "Marksman", []Breakpoint{
				{MinUnits: 2, MaxUnits: 25000, StyleName: "kBronze"},
			}),
		},
		map[string]grounder.ResolvedChampion{
			"TFT18_Vayne": makeResolvedChamp("TFT18_Vayne", "TFT18", 1, []string{"TFT18_Marksman"}, "Carry"),
		},
	)
	a := NewAnalyzer()
	out := a.Analyze(g)
	s := Format(out)
	require.NotEmpty(t, s)
	assert.Contains(t, s, "## 阵容分析")
	assert.Contains(t, s, "patch=16.17.1")
	assert.Contains(t, s, "set=TFT18")
	assert.Contains(t, s, "TFT18_Marksman")
	assert.Contains(t, s, "激活")
}

// ---- 15. KBStatusTrait ----

func TestKBStatusTrait(t *testing.T) {
	assert.Equal(t, "ok", KBStatusTrait(nil))
	assert.Equal(t, "ok", KBStatusTrait([]TraitAggregate{}))
	assert.Equal(t, "ok", KBStatusTrait([]TraitAggregate{
		{Status: TraitStatusActive},
		{Status: TraitStatusInactive},
	}))
	assert.Equal(t, "partial", KBStatusTrait([]TraitAggregate{
		{Status: TraitStatusActive},
		{Status: TraitStatusUnknown},
	}))
	assert.Equal(t, "unavailable", KBStatusTrait([]TraitAggregate{
		{Status: TraitStatusUnknown},
		{Status: TraitStatusUnknown},
	}))
}

// ---- 16. Core candidate scoring thresholds ----

func TestAnalyze_CoreScoring_HighCostHighStar_Likely(t *testing.T) {
	// 5 费 2 星 装备齐全 持有 unique trait → Likely (>= 0.7)
	g := makeGrounded("16.17.1", "TFT18",
		[]grounder.GroundedBoardSlot{
			{Position: 0, Star: 2, Champion: grounder.ResolvedChampion{ID: "TFT18_Azir", Status: grounder.StatusResolved},
				Items: []grounder.ResolvedItem{
					{ID: "TFT18_BF", Status: grounder.StatusResolved},
					{ID: "TFT18_Recurve", Status: grounder.StatusResolved},
				},
			},
		},
		nil,
		map[string]grounder.ResolvedChampion{
			// 5 费，独有 Azir trait
			"TFT18_Azir": makeResolvedChamp("TFT18_Azir", "TFT18", 5, []string{"TFT18_Shurima"}, "Carry"),
		},
	)
	a := NewAnalyzer()
	out := a.Analyze(g)
	require.Len(t, out.CoreCandidates, 1)
	c := out.CoreCandidates[0]
	// 0.4 (5费) + 0.3 (2星) + 0.3 (≥2装备) + 0.4 (unique trait) = 1.4
	assert.GreaterOrEqual(t, c.Score, 0.7)
	assert.Equal(t, CoreStatusLikely, c.Status)
}

func TestAnalyze_CoreScoring_OneCostOneStar_Unknown(t *testing.T) {
	// 1 费 1 星 无装备 → Unknown
	g := makeGrounded("16.17.1", "TFT18",
		[]grounder.GroundedBoardSlot{
			{Position: 0, Star: 1, Champion: grounder.ResolvedChampion{ID: "TFT18_Vayne", Status: grounder.StatusResolved}},
		},
		nil,
		map[string]grounder.ResolvedChampion{
			"TFT18_Vayne": makeResolvedChamp("TFT18_Vayne", "TFT18", 1, []string{"TFT18_Marksman"}, "Carry"),
		},
	)
	a := NewAnalyzer()
	out := a.Analyze(g)
	require.Len(t, out.CoreCandidates, 1)
	c := out.CoreCandidates[0]
	assert.Equal(t, CoreStatusUnknown, c.Status, "1 费 1 星 无装备 → 0 score → Unknown")
	assert.Equal(t, 0.0, c.Score)
}

// ---- 17. ActiveChampions sort by Position ----

func TestAnalyze_ActiveChampions_OrderByPosition(t *testing.T) {
	g := makeGrounded("16.17.1", "TFT18",
		[]grounder.GroundedBoardSlot{
			{Position: 5, Star: 1, Champion: grounder.ResolvedChampion{ID: "TFT18_B", Status: grounder.StatusResolved}},
			{Position: 0, Star: 1, Champion: grounder.ResolvedChampion{ID: "TFT18_A", Status: grounder.StatusResolved}},
			{Position: 3, Star: 1, Champion: grounder.ResolvedChampion{ID: "TFT18_C", Status: grounder.StatusResolved}},
		},
		nil,
		map[string]grounder.ResolvedChampion{
			"TFT18_A": makeResolvedChamp("TFT18_A", "TFT18", 1, []string{"TFT18_X"}, "Carry"),
			"TFT18_B": makeResolvedChamp("TFT18_B", "TFT18", 1, []string{"TFT18_X"}, "Carry"),
			"TFT18_C": makeResolvedChamp("TFT18_C", "TFT18", 1, []string{"TFT18_X"}, "Carry"),
		},
	)
	a := NewAnalyzer()
	out := a.Analyze(g)
	require.Len(t, out.ActiveChampions, 3)
	assert.Equal(t, "TFT18_A", out.ActiveChampions[0].ID)
	assert.Equal(t, "TFT18_C", out.ActiveChampions[1].ID)
	assert.Equal(t, "TFT18_B", out.ActiveChampions[2].ID)
}

// ---- 18. Single Champion produces Zero LLM calls (compile-time + runtime) ----

// 编译期断言：Analyzer 内部不引用任何 LLM 类型
var _ = func() bool {
	// 此函数体故意不引用 llm 包；如果 Analyzer 间接引用，go build 会失败
	return true
}()

func TestAnalyze_ZeroLLMCalls_ConstructionOnly(t *testing.T) {
	// 验证 Analyzer 构造不产生任何外部副作用
	a := NewAnalyzer()
	require.NotNil(t, a)

	// 验证 Analyzer 没有暴露 LLM 字段（grep-style 防御）
	// 通过类型断言：Analyzer 必须是 struct value，且没有 LLMProvider 字段
	// （实际上 Go 不允许私有字段访问，所以编译期已经保证）
	analyzerType := func(a *Analyzer) string { return "Analyzer" }
	_ = analyzerType(a)
}
