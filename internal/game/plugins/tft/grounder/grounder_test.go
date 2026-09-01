// Package grounder - Grounder.Resolve 单元测试（Phase 15 §8.1）
//
// 13 个 case，覆盖：
//   - Champion / Item / Trait / Augment 4 类实体 Hit
//   - Champion / Item 2 类 Miss
//   - Patch Isolation（WrongSet + NoLeakage）
//   - UnknownID 保留
//   - Mixed（Known + Unknown + 不同状态）
//   - Partial（仅部分字段 Known）
//   - Empty（repo == nil）
//   - Dedup 跨槽位
package grounder

import (
	"context"
	"encoding/json"
	"sync"
	"testing"

	"github.com/lipipi/aurora/internal/errors"
	"github.com/lipipi/aurora/internal/game"
	"github.com/lipipi/aurora/internal/game/knowledge"
	"github.com/lipipi/aurora/internal/game/plugins/tft/tfttypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---- Fake KB ----

// fakeKB GroundingRepository 的测试实现
//
// 行为：
//   - 当前 patch = "16.17.1"（TFT18）
//   - Get* 按 Query.ID 精确匹配预设数据
//   - 调用计数（champion / item / trait / augment 各计）
//   - 调用过的 ID 列表（用于断言 "NotCalled"）
type fakeKB struct {
	mu sync.Mutex

	currentPatch string
	champions    map[string]knowledge.Champion
	items        map[string]knowledge.Item
	traits       map[string]knowledge.Trait
	augments     map[string]knowledge.Augment
	errOnCham    errors.AppError
	errOnItem    errors.AppError
	errOnTrait   errors.AppError
	errOnAug     errors.AppError

	// 计数
	champCalls   int
	itemCalls    int
	traitCalls   int
	augmentCalls int

	// 记录被调用的 ID
	champIDsCalled   []string
	itemIDsCalled    []string
	traitIDsCalled   []string
	augmentIDsCalled []string
}

func newFakeKB() *fakeKB {
	return &fakeKB{
		currentPatch: "16.17.1",
		champions:    map[string]knowledge.Champion{},
		items:        map[string]knowledge.Item{},
		traits:       map[string]knowledge.Trait{},
		augments:     map[string]knowledge.Augment{},
	}
}

func (f *fakeKB) GetCurrentPatch(ctx context.Context) (string, errors.AppError) {
	return f.currentPatch, nil
}

func (f *fakeKB) GetChampion(ctx context.Context, game string, q knowledge.Query) ([]knowledge.Champion, errors.AppError) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.champCalls++
	if f.errOnCham != nil {
		return nil, f.errOnCham
	}
	if q.ID == "" {
		// 全部返回（无 ID），按 Map 顺序前 50
		out := make([]knowledge.Champion, 0, len(f.champions))
		for _, c := range f.champions {
			out = append(out, c)
			if len(out) >= 50 {
				break
			}
		}
		return out, nil
	}
	f.champIDsCalled = append(f.champIDsCalled, q.ID)
	if c, ok := f.champions[q.ID]; ok {
		return []knowledge.Champion{c}, nil
	}
	return nil, nil
}

func (f *fakeKB) GetItem(ctx context.Context, game string, q knowledge.Query) ([]knowledge.Item, errors.AppError) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.itemCalls++
	if f.errOnItem != nil {
		return nil, f.errOnItem
	}
	if q.ID != "" {
		f.itemIDsCalled = append(f.itemIDsCalled, q.ID)
		if it, ok := f.items[q.ID]; ok {
			return []knowledge.Item{it}, nil
		}
	}
	return nil, nil
}

func (f *fakeKB) GetTrait(ctx context.Context, game string, q knowledge.Query) ([]knowledge.Trait, errors.AppError) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.traitCalls++
	if f.errOnTrait != nil {
		return nil, f.errOnTrait
	}
	if q.ID != "" {
		f.traitIDsCalled = append(f.traitIDsCalled, q.ID)
		if t, ok := f.traits[q.ID]; ok {
			return []knowledge.Trait{t}, nil
		}
	}
	return nil, nil
}

func (f *fakeKB) GetAugment(ctx context.Context, game string, q knowledge.Query) ([]knowledge.Augment, errors.AppError) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.augmentCalls++
	if f.errOnAug != nil {
		return nil, f.errOnAug
	}
	if q.ID != "" {
		f.augmentIDsCalled = append(f.augmentIDsCalled, q.ID)
		if a, ok := f.augments[q.ID]; ok {
			return []knowledge.Augment{a}, nil
		}
	}
	return nil, nil
}

func (f *fakeKB) AssertNotCalled(t *testing.T, id string) {
	t.Helper()
	for _, called := range f.champIDsCalled {
		assert.NotEqual(t, id, called, "champion %q should NOT have been queried", id)
	}
	for _, called := range f.itemIDsCalled {
		assert.NotEqual(t, id, called, "item %q should NOT have been queried", id)
	}
}

// ---- Helpers ----

// makeGameContext 构造带 Version 的 GameContext
func makeGameContext(patch string) *game.GameContext {
	if patch == "" {
		return nil
	}
	return &game.GameContext{
		CommonData: game.CommonData{
			Version: game.Known(patch),
		},
	}
}

// makeTFTContext 构造最小可用的 TFTGameContext
func makeTFTContext() *tfttypes.TFTGameContext {
	return &tfttypes.TFTGameContext{
		Stage: game.Known("4-2"),
		Level: game.Known(7),
		Gold:  game.Known(20),
		HP:    game.Known(80),
	}
}

// makeChampionJSON 构造 CommunityDragon 格式 champion Data
func makeChampionJSON(cost int, traits []string, role string) json.RawMessage {
	m := map[string]interface{}{"cost": cost, "traits": traits, "role": role}
	raw, _ := json.Marshal(m)
	return raw
}

// makeItemJSON 构造 CommunityDragon 格式 item Data
func makeItemJSON(desc string, from []string) json.RawMessage {
	m := map[string]interface{}{"desc": desc, "from": from}
	raw, _ := json.Marshal(m)
	return raw
}

// makeTraitJSON 构造 CommunityDragon 格式 trait Data
func makeTraitJSON(t, typ string) json.RawMessage {
	m := map[string]interface{}{"desc": t, "type": typ}
	raw, _ := json.Marshal(m)
	return raw
}

// ---- 1. Champion Hit ----

func TestResolve_Champion_Hit(t *testing.T) {
	fkb := newFakeKB()
	fkb.champions["TFT18_Vayne"] = knowledge.Champion{
		Patch: "16.17.1",
		ID:    "TFT18_Vayne",
		Name:  "Vayne",
		Data:  makeChampionJSON(1, []string{"Bulwark", "Marksman"}, "Carry"),
	}
	g := New(fkb)
	tctx := makeTFTContext()
	tctx.Board = game.Known([]tfttypes.BoardSlot{
		{ChampionID: "TFT18_Vayne", Star: 2, Position: 12},
	})

	out := g.Resolve(context.Background(), makeGameContext("16.17.1"), tctx)

	require.Len(t, out.Board, 1)
	c := out.Board[0].Champion
	assert.Equal(t, StatusResolved, c.Status)
	assert.Equal(t, "TFT18_Vayne", c.ID)
	assert.Equal(t, "Vayne", c.Name)
	assert.Equal(t, 1, c.Cost)
	assert.Equal(t, []string{"Bulwark", "Marksman"}, c.Traits)
	assert.Equal(t, "Carry", c.Role)
	assert.Equal(t, "TFT18", c.SetKey)
	assert.Equal(t, "16.17.1", c.Patch)
}

// ---- 2. Champion Miss ----

func TestResolve_Champion_Miss(t *testing.T) {
	fkb := newFakeKB()
	// 不注册任何 champion
	g := New(fkb)
	tctx := makeTFTContext()
	tctx.Board = game.Known([]tfttypes.BoardSlot{
		{ChampionID: "TFT18_NonExistent", Star: 1},
	})

	out := g.Resolve(context.Background(), makeGameContext("16.17.1"), tctx)

	require.Len(t, out.Board, 1)
	c := out.Board[0].Champion
	assert.Equal(t, StatusUnknown, c.Status)
	assert.Equal(t, "TFT18_NonExistent", c.ID)
	assert.Empty(t, c.Name)
}

// ---- 3. Item Hit ----

func TestResolve_Item_Hit(t *testing.T) {
	fkb := newFakeKB()
	fkb.items["TFT18_IE"] = knowledge.Item{
		Patch: "16.17.1",
		ID:    "TFT18_IE",
		Name:  "Infinity Edge",
		Data:  makeItemJSON("Crit chance doubled; deals bonus true damage", []string{"TFT18_BF", "TFT18_Recurve"}),
	}
	g := New(fkb)
	tctx := makeTFTContext()
	tctx.Items = game.Known([]tfttypes.ItemSlot{
		{ItemID: "TFT18_IE", Count: 1},
	})

	out := g.Resolve(context.Background(), makeGameContext("16.17.1"), tctx)

	require.Len(t, out.Items, 1)
	it := out.Items[0].Item
	assert.Equal(t, StatusResolved, it.Status)
	assert.Equal(t, "TFT18_IE", it.ID)
	assert.Equal(t, "Infinity Edge", it.Name)
	assert.Contains(t, it.Description, "Crit chance")
	assert.Equal(t, []string{"TFT18_BF", "TFT18_Recurve"}, it.From)
}

// ---- 4. Item Miss ----

func TestResolve_Item_Miss(t *testing.T) {
	fkb := newFakeKB()
	g := New(fkb)
	tctx := makeTFTContext()
	tctx.Items = game.Known([]tfttypes.ItemSlot{
		{ItemID: "TFT18_DoesNotExist", Count: 2},
	})

	out := g.Resolve(context.Background(), makeGameContext("16.17.1"), tctx)

	require.Len(t, out.Items, 1)
	assert.Equal(t, StatusUnknown, out.Items[0].Item.Status)
}

// ---- 5. Trait Hit ----

func TestResolve_Trait_Hit(t *testing.T) {
	fkb := newFakeKB()
	fkb.traits["TFT18_Bulwark"] = knowledge.Trait{
		Patch: "16.17.1",
		ID:    "TFT18_Bulwark",
		Name:  "Bulwark",
		Data:  makeTraitJSON("Gain shield equal to 15/30/50% max HP", "Origin"),
	}
	g := New(fkb)
	tctx := makeTFTContext()
	tctx.Traits = game.Known([]tfttypes.TraitState{
		{TraitID: "TFT18_Bulwark", Active: 4, Style: 2},
	})

	out := g.Resolve(context.Background(), makeGameContext("16.17.1"), tctx)

	require.Len(t, out.Traits, 1)
	tr := out.Traits[0].Trait
	assert.Equal(t, StatusResolved, tr.Status)
	assert.Equal(t, "TFT18_Bulwark", tr.ID)
	assert.Equal(t, "Bulwark", tr.Name)
	assert.Equal(t, "Origin", tr.Type)
	// 透传 active/style
	assert.Equal(t, 4, out.Traits[0].Active)
	assert.Equal(t, 2, out.Traits[0].Style)
}

// ---- 6. Augment Hit (with Tier) ----

func TestResolve_Augment_Hit(t *testing.T) {
	fkb := newFakeKB()
	fkb.augments["TFT18_AugX"] = knowledge.Augment{
		Patch:       "16.17.1",
		ID:          "TFT18_AugX",
		Name:        "Combat Medic",
		Description: "Restore 25% of max HP after winning combat",
	}
	g := New(fkb)
	tctx := makeTFTContext()
	tctx.Augments = game.Known([]tfttypes.AugmentSlot{
		{AugmentID: "TFT18_AugX", Tier: "gold"},
	})

	out := g.Resolve(context.Background(), makeGameContext("16.17.1"), tctx)

	require.Len(t, out.Augments, 1)
	a := out.Augments[0].Augment
	assert.Equal(t, StatusResolved, a.Status)
	assert.Equal(t, "TFT18_AugX", a.ID)
	assert.Equal(t, "Combat Medic", a.Name)
	assert.Contains(t, a.Description, "25%")
	// Tier 透传
	assert.Equal(t, "gold", out.Augments[0].Tier)
}

// ---- 7. Patch Isolation — WrongSet ----

func TestResolve_PatchIsolation_WrongSet(t *testing.T) {
	fkb := newFakeKB()
	// KB 中**有** TFT17_Vayne，但当前 context 是 TFT18
	fkb.champions["TFT17_Vayne"] = knowledge.Champion{
		Patch: "16.16", // TFT17
		ID:    "TFT17_Vayne",
		Name:  "OldVayne",
		Data:  makeChampionJSON(1, []string{"Marksman"}, "Carry"),
	}
	g := New(fkb)
	tctx := makeTFTContext()
	tctx.Bench = game.Known([]tfttypes.BenchSlot{
		{ChampionID: "TFT17_Vayne", Star: 1},
	})

	out := g.Resolve(context.Background(), makeGameContext("16.17.1"), tctx)

	require.Len(t, out.Bench, 1)
	c := out.Bench[0].Champion
	assert.Equal(t, StatusWrongSet, c.Status, "TFT17_Vayne in TFT18 context must be WrongSet")
	assert.Equal(t, "TFT17_Vayne", c.ID, "ID should still be preserved")
	// 关键不变量：fakeKB **没有**被调用 GetChampion
	fkb.AssertNotCalled(t, "TFT17_Vayne")
	assert.Equal(t, 0, fkb.champCalls, "prefix mismatch should not hit KB at all")
}

// ---- 8. Patch Isolation — NoLeakage (two patches independent) ----

func TestResolve_PatchIsolation_NoLeakage(t *testing.T) {
	fkb := newFakeKB()
	fkb.champions["TFT18_Vayne"] = knowledge.Champion{Patch: "16.17.1", ID: "TFT18_Vayne", Name: "Vayne18", Data: makeChampionJSON(1, nil, "")}
	fkb.champions["TFT17_Vayne"] = knowledge.Champion{Patch: "16.16", ID: "TFT17_Vayne", Name: "Vayne17", Data: makeChampionJSON(1, nil, "")}
	g := New(fkb)

	// 第一次：TFT18
	tctx18 := makeTFTContext()
	tctx18.Board = game.Known([]tfttypes.BoardSlot{{ChampionID: "TFT18_Vayne", Star: 1}})
	out18 := g.Resolve(context.Background(), makeGameContext("16.17.1"), tctx18)
	require.Len(t, out18.Board, 1)
	assert.Equal(t, StatusResolved, out18.Board[0].Champion.Status)
	assert.Equal(t, "Vayne18", out18.Board[0].Champion.Name)

	// 第二次：TFT17
	tctx17 := makeTFTContext()
	tctx17.Board = game.Known([]tfttypes.BoardSlot{{ChampionID: "TFT17_Vayne", Star: 1}})
	out17 := g.Resolve(context.Background(), makeGameContext("16.16"), tctx17)
	require.Len(t, out17.Board, 1)
	assert.Equal(t, StatusResolved, out17.Board[0].Champion.Status)
	assert.Equal(t, "Vayne17", out17.Board[0].Champion.Name)
	assert.Equal(t, "TFT17", out17.Board[0].Champion.SetKey)
}

// ---- 9. UnknownID StaysUnknown (literal "Unknown") ----

func TestResolve_UnknownID_StaysUnknown(t *testing.T) {
	fkb := newFakeKB()
	// 即使 KB 中**有** "Unknown" 也不应被查询
	fkb.champions["Unknown"] = knowledge.Champion{Patch: "16.17.1", ID: "Unknown", Name: "ShouldNot", Data: makeChampionJSON(0, nil, "")}
	g := New(fkb)
	tctx := makeTFTContext()
	tctx.Board = game.Known([]tfttypes.BoardSlot{{ChampionID: "Unknown"}})

	out := g.Resolve(context.Background(), makeGameContext("16.17.1"), tctx)

	require.Len(t, out.Board, 1)
	assert.Equal(t, StatusUnknown, out.Board[0].Champion.Status)
	assert.Equal(t, "Unknown", out.Board[0].Champion.ID)
	assert.Equal(t, 0, fkb.champCalls, "Unknown ID must not query KB")
}

// ---- 10. Mixed (Known + Unknown + Lucian) ----

func TestResolve_Mixed_KnownAndUnknown(t *testing.T) {
	fkb := newFakeKB()
	fkb.champions["TFT18_Vayne"] = knowledge.Champion{Patch: "16.17.1", ID: "TFT18_Vayne", Name: "Vayne", Data: makeChampionJSON(1, nil, "")}
	fkb.champions["TFT18_Lucian"] = knowledge.Champion{Patch: "16.17.1", ID: "TFT18_Lucian", Name: "Lucian", Data: makeChampionJSON(2, nil, "")}
	// "Unknown" ID 不在 KB
	g := New(fkb)
	tctx := makeTFTContext()
	tctx.Board = game.Known([]tfttypes.BoardSlot{
		{ChampionID: "TFT18_Vayne"},
		{ChampionID: "Unknown"},
		{ChampionID: "TFT18_Lucian"},
	})

	out := g.Resolve(context.Background(), makeGameContext("16.17.1"), tctx)

	require.Len(t, out.Board, 3)
	assert.Equal(t, StatusResolved, out.Board[0].Champion.Status)
	assert.Equal(t, "Vayne", out.Board[0].Champion.Name)
	assert.Equal(t, StatusUnknown, out.Board[1].Champion.Status)
	assert.Equal(t, StatusResolved, out.Board[2].Champion.Status)
	assert.Equal(t, "Lucian", out.Board[2].Champion.Name)
	// 关键：只查了 2 次 KB（Vayne, Lucian），Unknown 没查
	assert.Equal(t, 2, fkb.champCalls, "Unknown slot must not trigger KB query")
}

// ---- 11. Partial — Only Board Known ----

func TestResolve_Partial_OnlyBoardKnown(t *testing.T) {
	fkb := newFakeKB()
	fkb.champions["TFT18_Vayne"] = knowledge.Champion{Patch: "16.17.1", ID: "TFT18_Vayne", Name: "Vayne", Data: makeChampionJSON(1, nil, "")}
	g := New(fkb)
	tctx := &tfttypes.TFTGameContext{
		// 基础资源未 Known
		Board: game.Known([]tfttypes.BoardSlot{{ChampionID: "TFT18_Vayne", Star: 1}}),
	}

	out := g.Resolve(context.Background(), makeGameContext("16.17.1"), tctx)

	assert.Len(t, out.Board, 1)
	assert.Equal(t, StatusResolved, out.Board[0].Champion.Status)
	assert.Empty(t, out.Bench)
	assert.Empty(t, out.Shop)
	assert.Empty(t, out.Items)
	assert.Empty(t, out.Traits)
	assert.Empty(t, out.Augments)
}

// ---- 12. Empty — no KB available (repo == nil) ----

func TestResolve_Empty_NoKBAvailable(t *testing.T) {
	g := New(nil)
	tctx := makeTFTContext()
	tctx.Board = game.Known([]tfttypes.BoardSlot{{ChampionID: "TFT18_Vayne"}})
	tctx.Items = game.Known([]tfttypes.ItemSlot{{ItemID: "TFT18_IE"}})

	out := g.Resolve(context.Background(), makeGameContext("16.17.1"), tctx)

	require.NotNil(t, out)
	assert.Equal(t, StatusKBUnavailable, out.Board[0].Champion.Status)
	assert.Equal(t, StatusKBUnavailable, out.Items[0].Item.Status)
	assert.False(t, out.Meta.KBHasData)
}

// ---- 13. Dedup across slots ----

func TestResolve_DedupAcrossSlots(t *testing.T) {
	fkb := newFakeKB()
	fkb.champions["TFT18_Vayne"] = knowledge.Champion{
		Patch: "16.17.1", ID: "TFT18_Vayne", Name: "Vayne",
		Data: makeChampionJSON(1, []string{"Bulwark"}, "Carry"),
	}
	g := New(fkb)
	tctx := makeTFTContext()
	tctx.Board = game.Known([]tfttypes.BoardSlot{{ChampionID: "TFT18_Vayne", Star: 2}})
	tctx.Bench = game.Known([]tfttypes.BenchSlot{{ChampionID: "TFT18_Vayne", Star: 1}})

	out := g.Resolve(context.Background(), makeGameContext("16.17.1"), tctx)

	require.Len(t, out.Board, 1)
	require.Len(t, out.Bench, 1)
	assert.Equal(t, StatusResolved, out.Board[0].Champion.Status)
	assert.Equal(t, StatusResolved, out.Bench[0].Champion.Status)
	// 关键：fakeKB.GetChampion 只被调 1 次（dedup 生效）
	assert.Equal(t, 1, fkb.champCalls, "Vayne on Board+Bench should dedup to 1 KB query")
}

// ---- Bonus: tctx == nil ----

func TestResolve_NilTContext(t *testing.T) {
	fkb := newFakeKB()
	g := New(fkb)
	out := g.Resolve(context.Background(), makeGameContext("16.17.1"), nil)
	assert.Empty(t, out.Board)
	assert.Empty(t, out.Bench)
	// patch 已知、KB 有数据
	assert.Equal(t, "16.17.1", out.Meta.Patch)
	assert.True(t, out.Meta.KBHasData)
}

// ---- Bonus: patch unresolvable (gctx nil + KB returns error) ----

func TestResolve_PatchUnresolvable(t *testing.T) {
	fkb := newFakeKB()
	fkb.currentPatch = "" // KB 也无 patch
	g := New(fkb)
	tctx := makeTFTContext()
	tctx.Board = game.Known([]tfttypes.BoardSlot{{ChampionID: "TFT18_Vayne"}})

	out := g.Resolve(context.Background(), nil, tctx)

	require.Len(t, out.Board, 1)
	assert.Equal(t, StatusKBUnavailable, out.Board[0].Champion.Status)
	assert.False(t, out.Meta.KBHasData)
}

// ---- Bonus: KB query error ----

func TestResolve_KBQueryError(t *testing.T) {
	fkb := newFakeKB()
	fkb.errOnCham = errors.New(errors.CodeUnknown, "kb broken")
	g := New(fkb)
	tctx := makeTFTContext()
	tctx.Board = game.Known([]tfttypes.BoardSlot{{ChampionID: "TFT18_Vayne"}})

	out := g.Resolve(context.Background(), makeGameContext("16.17.1"), tctx)
	require.Len(t, out.Board, 1)
	assert.Equal(t, StatusKBUnavailable, out.Board[0].Champion.Status)
}
