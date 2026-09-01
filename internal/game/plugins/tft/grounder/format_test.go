// Package grounder - Format(GroundedGameContext) 单元测试（Phase 15 §8.2）
//
// 4 个 case：
//   - 空 GroundedGameContext → ""
//   - 完整 Board/Items/Traits/Augments → golden string section 顺序
//   - 长 Augment 触发 3000 token 截断
//   - WrongSet annotation 出现
package grounder

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---- 14. Empty meta → "" ----

func TestFormat_EmptyMeta(t *testing.T) {
	out := GroundedGameContext{}
	assert.Equal(t, "", Format(out))

	// 即使有 GameState 但无 patch / setKey / 任何槽位 → IsEmpty
	onlyState := GroundedGameContext{
		GameState: GroundedGameState{Stage: "4-2", Level: 7, Gold: 20},
	}
	// GameState 非空但 Patch/SetKey 都空 → 仍输出
	formatted := Format(onlyState)
	assert.NotEmpty(t, formatted)
	assert.Contains(t, formatted, "## 知识库对照")
}

// ---- 15. Full Board → golden structure ----

func TestFormat_FullBoard(t *testing.T) {
	gc := GroundedGameContext{
		GameState: GroundedGameState{Stage: "4-2", Level: 7, Gold: 20, HP: 80},
		Meta: GroundedMeta{
			Patch:     "16.17.1",
			SetKey:    "TFT18",
			KBHasData: true,
		},
		Board: []GroundedBoardSlot{
			{
				Position: 12,
				Star:     2,
				Champion: ResolvedChampion{
					ID: "TFT18_Vayne", Name: "Vayne", Cost: 1,
					Traits: []string{"Bulwark", "Marksman"},
					Role:   "Carry", Status: StatusResolved,
					SetKey: "TFT18", Patch: "16.17.1",
				},
				Items: []ResolvedItem{
					{
						ID: "TFT18_IE", Name: "Infinity Edge",
						Description: "Crit chance doubled",
						From:        []string{"TFT18_BF", "TFT18_Recurve"},
						Status:      StatusResolved,
						SetKey:      "TFT18", Patch: "16.17.1",
					},
				},
			},
		},
		Traits: []GroundedTraitState{
			{
				Trait: ResolvedTrait{
					ID: "TFT18_Bulwark", Name: "Bulwark",
					Type: "Origin", Status: StatusResolved,
					SetKey: "TFT18", Patch: "16.17.1",
				},
				Active: 4, Style: 2,
			},
		},
		Augments: []GroundedAugmentSlot{
			{
				Augment: ResolvedAugment{
					ID: "TFT18_AugX", Name: "Combat Medic",
					Description: "Restore 25% HP after win",
					Status:      StatusResolved,
					SetKey:      "TFT18", Patch: "16.17.1",
				},
				Tier: "gold",
			},
		},
	}

	s := Format(gc)

	// 顺序：Header → Board → Traits → Augments
	headerIdx := strings.Index(s, "## 知识库对照")
	boardIdx := strings.Index(s, "### 棋盘")
	traitsIdx := strings.Index(s, "### 羁绊")
	augsIdx := strings.Index(s, "### 强化符文")
	require.GreaterOrEqual(t, headerIdx, 0)
	require.GreaterOrEqual(t, boardIdx, headerIdx)
	require.GreaterOrEqual(t, traitsIdx, boardIdx)
	require.GreaterOrEqual(t, augsIdx, traitsIdx)

	// 内容
	assert.Contains(t, s, "patch=16.17.1, set=TFT18")
	assert.Contains(t, s, "Vayne")
	assert.Contains(t, s, "★2")
	assert.Contains(t, s, "pos=12")
	assert.Contains(t, s, "cost=1")
	assert.Contains(t, s, "Infinity Edge")
	assert.Contains(t, s, "Bulwark")
	assert.Contains(t, s, "active=4")
	assert.Contains(t, s, "[gold]")
	assert.Contains(t, s, "Combat Medic")
}

// ---- 16. Budget truncation: 12 long augments + big board → ≤ 3000 tokens ----

func TestFormat_BudgetTruncation(t *testing.T) {
	// 构造 12 个超长 description 的 augment + 28 个 board slot
	gc := GroundedGameContext{
		Meta: GroundedMeta{Patch: "16.17.1", SetKey: "TFT18", KBHasData: true},
		GameState: GroundedGameState{Stage: "5-1", Level: 9, Gold: 50, HP: 100},
	}
	// 28 board slots
	for i := 0; i < 28; i++ {
		gc.Board = append(gc.Board, GroundedBoardSlot{
			Position: i,
			Star:     2,
			Champion: ResolvedChampion{
				ID: "TFT18_Vayne", Name: "Vayne", Cost: 1,
				Status: StatusResolved, SetKey: "TFT18", Patch: "16.17.1",
			},
		})
	}
	// 12 个超长 augment
	longDesc := strings.Repeat("very long augment description text. ", 50) // ~1700 chars
	for i := 0; i < 12; i++ {
		gc.Augments = append(gc.Augments, GroundedAugmentSlot{
			Augment: ResolvedAugment{
				ID: "TFT18_LongAug" + string(rune('A'+i)),
				Name: "LongAug",
				Description: longDesc,
				Status:      StatusResolved,
				SetKey:      "TFT18", Patch: "16.17.1",
			},
			Tier: "gold",
		})
	}

	s := Format(gc)
	maxChars := FormatMaxTokens * charsPerToken
	// 允许追加 [truncated for length...] 后缀
	truncated := strings.Contains(s, "[truncated")
	if !truncated {
		assert.LessOrEqual(t, len(s), maxChars, "should be within budget when not truncated")
	} else {
		// 截断后应远小于 budget
		assert.Less(t, len(s), maxChars+200, "truncated output should be near budget")
	}
	// 棋盘永远不被丢
	assert.Contains(t, s, "### 棋盘")
}

// ---- 17. WrongSet annotation ----

func TestFormat_WrongSetAnnotation(t *testing.T) {
	gc := GroundedGameContext{
		Meta: GroundedMeta{Patch: "16.17.1", SetKey: "TFT18", KBHasData: true},
		GameState: GroundedGameState{Stage: "4-2"},
		Board: []GroundedBoardSlot{
			{
				Position: 0,
				Champion: ResolvedChampion{
					ID: "TFT17_Vayne", Status: StatusWrongSet,
					SetKey: "TFT18", Patch: "16.17.1",
				},
			},
		},
		Items: []GroundedItemSlot{
			{
				Item: ResolvedItem{
					ID: "TFT16_IE", Status: StatusWrongSet,
					SetKey: "TFT18", Patch: "16.17.1",
				},
			},
		},
	}

	s := Format(gc)

	assert.Contains(t, s, "## 知识库对照")
	assert.Contains(t, s, "⚠️")
	assert.Contains(t, s, "wrong_set")
	assert.Contains(t, s, "TFT17_Vayne")
	assert.Contains(t, s, "TFT16_IE")
	assert.True(t, gc.HasWrongSet())
}

// ---- Bonus: Unknown + KBUnavailable tags ----

func TestFormat_StatusTags(t *testing.T) {
	gc := GroundedGameContext{
		Meta: GroundedMeta{Patch: "16.17.1", SetKey: "TFT18", KBHasData: true},
		GameState: GroundedGameState{Stage: "4-2"},
		Board: []GroundedBoardSlot{
			{Champion: ResolvedChampion{ID: "Unknown", Status: StatusUnknown, SetKey: "TFT18"}},
			{Champion: ResolvedChampion{ID: "TFT18_Vayne", Status: StatusKBUnavailable, SetKey: "TFT18"}},
		},
	}
	s := Format(gc)
	assert.Contains(t, s, "[unknown: 名称未识别]")
	assert.Contains(t, s, "[kb_unavailable]")
}

// ---- Bonus: GameState empty vs non-empty ----

func TestFormat_GameState(t *testing.T) {
	gc := GroundedGameContext{
		Meta:      GroundedMeta{Patch: "16.17.1", SetKey: "TFT18", KBHasData: true},
		GameState: GroundedGameState{Stage: "5-1", Level: 8, XP: 12, XPTolevel: 24, Gold: 35, Interest: 5, HP: 65, WinStreak: 3},
	}
	s := Format(gc)
	assert.Contains(t, s, "### 对局资源")
	assert.Contains(t, s, "5-1")
	assert.Contains(t, s, "Lv.8")
	assert.Contains(t, s, "35g")
	assert.Contains(t, s, "利息 5")
	assert.Contains(t, s, "65")
	assert.Contains(t, s, "连胜: 3")
}

// ---- Bonus: PatchFallback flag ----

func TestFormat_PatchFallback(t *testing.T) {
	gc := GroundedGameContext{
		Meta: GroundedMeta{
			Patch: "16.17.1", SetKey: "TFT18", KBHasData: true, PatchFallback: true,
		},
		GameState: GroundedGameState{Stage: "4-2"},
	}
	s := Format(gc)
	assert.Contains(t, s, "(fallback)")
}

// ---- 编码一致性（sanity：Decoded RawMessage 不为 nil） ----

func TestDecodeRawField_NestedAbility(t *testing.T) {
	data := json.RawMessage(`{"name":"Vayne","ability":{"name":"Tumble","desc":"dashes"},"cost":1}`)
	ab := decodeRawField(data, "ability")
	require.NotNil(t, ab)
	assert.Contains(t, string(ab), "Tumble")
}
