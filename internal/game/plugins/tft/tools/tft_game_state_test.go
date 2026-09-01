package tools

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/lipipi/aurora/internal/game"
	"github.com/lipipi/aurora/internal/game/plugins/tft/tfttypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockStateProvider 简单 GameStateProvider mock
type mockStateProvider struct {
	ctx *game.GameContext
}

func (m *mockStateProvider) Snapshot() *game.GameContext { return m.ctx }

func TestTFTGameStateTool_NilGameContext(t *testing.T) {
	tool := &TFTGameStateTool{state: &mockStateProvider{ctx: nil}}
	require.True(t, tool.IsAvailable())

	res, err := tool.Execute(context.Background(), nil)
	require.NoError(t, err)
	require.True(t, res.Success)
	require.NotNil(t, res.Data)

	var out gameStateOutput
	require.NoError(t, json.Unmarshal(res.Data, &out))
	assert.False(t, out.Available)
	assert.Equal(t, "", out.Game)
	assert.False(t, out.PatchKnown)
	assert.Nil(t, out.TFT)
}

func TestTFTGameStateTool_NonTFTGameContext(t *testing.T) {
	// GameData 是 string 而不是 *TFTGameContext：标记 not_tft=true
	gctx := &game.GameContext{
		Game: "other",
		Name: "Other",
		Window: game.WindowInfo{
			Title: "Other Game",
		},
		GameData: "raw non-tft data",
	}
	tool := &TFTGameStateTool{state: &mockStateProvider{ctx: gctx}}

	res, err := tool.Execute(context.Background(), nil)
	require.NoError(t, err)
	require.True(t, res.Success)

	var out gameStateOutput
	require.NoError(t, json.Unmarshal(res.Data, &out))
	assert.True(t, out.Available)
	assert.Equal(t, "other", out.Game)
	assert.True(t, out.NotTFT)
	assert.Nil(t, out.TFT)
}

func TestTFTGameStateTool_TFTUnknownFields(t *testing.T) {
	// TFT 上下文但所有 Value 都是 Unknown
	gctx := &game.GameContext{
		Game:  "tft",
		Name:  "云顶之弈",
		Phase: game.PhaseInGame,
		Window: game.WindowInfo{
			Title: "League of Legends (TM) Client",
			Class: "RiotWindowClass",
		},
		GameData: &tfttypes.TFTGameContext{},
	}
	tool := &TFTGameStateTool{state: &mockStateProvider{ctx: gctx}}

	res, err := tool.Execute(context.Background(), nil)
	require.NoError(t, err)
	require.True(t, res.Success)

	var out gameStateOutput
	require.NoError(t, json.Unmarshal(res.Data, &out))
	assert.True(t, out.Available)
	assert.Equal(t, "tft", out.Game)
	assert.False(t, out.PatchKnown)
	assert.Equal(t, "", out.Patch)
	require.NotNil(t, out.TFT)

	// 所有字段 known=false
	assert.False(t, out.TFT.Level.Known)
	assert.False(t, out.TFT.Gold.Known)
	assert.False(t, out.TFT.HP.Known)
	assert.False(t, out.TFT.Board.Known)
	assert.False(t, out.TFT.Bench.Known)
	assert.False(t, out.TFT.Shop.Known)
	assert.False(t, out.TFT.Traits.Known)
	assert.False(t, out.TFT.Augments.Known)
}

func TestTFTGameStateTool_TFTKnownValues(t *testing.T) {
	// 部分字段已识别，含 Known(0) 的 HP=0
	now := time.Now()
	tctx := &tfttypes.TFTGameContext{
		Stage:    game.Known("4-2"),
		Level:    game.Known(7),
		XP:       game.Known(12),
		XPTolevel: game.Known(20),
		Gold:     game.Detected(42, game.SourceOCR, 0.98),
		Interest: game.Known(2),
		HP:       game.Known(0), // 重要：HP=0 是合法值（阵亡），必须 known=true
		WinStreak: game.Known(3),
		LoseStreak: game.Known(0), // 同样：连胜 3 败场 0 合法
		Board:    game.Known([]tfttypes.BoardSlot{
			{ChampionID: "TFT18_Vayne", Star: 2, Items: []string{"TFT_IE"}, Position: 7},
		}),
		Bench: game.Known([]tfttypes.BenchSlot{
			{ChampionID: "TFT18_Jinx", Star: 1},
		}),
		Shop: game.Known([]tfttypes.ShopSlot{
			{ChampionID: "TFT18_Ahri", Cost: 3},
		}),
		Items: game.Known([]tfttypes.ItemSlot{
			{ItemID: "TFT_IE", Count: 1},
		}),
		Traits: game.Known([]tfttypes.TraitState{
			{TraitID: "TFT18_Bulwark", Active: 2, Style: 2},
		}),
		Augments: game.Known([]tfttypes.AugmentSlot{
			{AugmentID: "TFT18_Aug_X", Tier: "silver"},
		}),
	}
	_ = now

	gctx := &game.GameContext{
		Game:  "tft",
		Name:  "云顶之弈",
		Phase: game.PhaseInGame,
		Window: game.WindowInfo{
			Title: "LoL",
		},
		CommonData: game.CommonData{
			Version: game.Detected("16.17.1", game.SourceOCR, 0.95),
		},
		GameData: tctx,
	}
	tool := &TFTGameStateTool{state: &mockStateProvider{ctx: gctx}}

	res, err := tool.Execute(context.Background(), nil)
	require.NoError(t, err)
	require.True(t, res.Success)

	var out gameStateOutput
	require.NoError(t, json.Unmarshal(res.Data, &out))
	assert.True(t, out.Available)
	assert.Equal(t, "tft", out.Game)
	assert.True(t, out.PatchKnown)
	assert.Equal(t, "16.17.1", out.Patch)
	assert.False(t, out.NotTFT)
	require.NotNil(t, out.TFT)

	// 关键：HP=0 必须 known=true
	assert.True(t, out.TFT.HP.Known)
	assert.Equal(t, 0, out.TFT.HP.Value)
	assert.True(t, out.TFT.LoseStreak.Known)
	assert.Equal(t, 0, out.TFT.LoseStreak.Value)

	// 数值字段
	assert.Equal(t, 7, out.TFT.Level.Value)
	assert.Equal(t, 42, out.TFT.Gold.Value)
	assert.InDelta(t, 0.98, out.TFT.Gold.Confidence, 0.001)
	assert.Equal(t, "ocr", out.TFT.Gold.Source)
	assert.Equal(t, "4-2", out.TFT.Stage)

	// 列表字段
	require.True(t, out.TFT.Board.Known)
	require.Len(t, out.TFT.Board.Items, 1)
	assert.Equal(t, "TFT18_Vayne", out.TFT.Board.Items[0].ChampionID)
	assert.Equal(t, 2, out.TFT.Board.Items[0].Star)
	assert.Equal(t, []string{"TFT_IE"}, out.TFT.Board.Items[0].Items)

	require.True(t, out.TFT.Traits.Known)
	require.Len(t, out.TFT.Traits.Items, 1)
	assert.Equal(t, "TFT18_Bulwark", out.TFT.Traits.Items[0].TraitID)

	require.True(t, out.TFT.Augments.Known)
	require.Len(t, out.TFT.Augments.Items, 1)
	assert.Equal(t, "silver", out.TFT.Augments.Items[0].Tier)
}

func TestTFTGameStateTool_TFTEmptySlices(t *testing.T) {
	// Present=true 但切片为空
	tctx := &tfttypes.TFTGameContext{
		Board:    game.Known([]tfttypes.BoardSlot{}),
		Bench:    game.Known([]tfttypes.BenchSlot{}),
		Shop:     game.Known([]tfttypes.ShopSlot{}),
		Items:    game.Known([]tfttypes.ItemSlot{}),
		Traits:   game.Known([]tfttypes.TraitState{}),
		Augments: game.Known([]tfttypes.AugmentSlot{}),
	}
	gctx := &game.GameContext{Game: "tft", GameData: tctx}
	tool := &TFTGameStateTool{state: &mockStateProvider{ctx: gctx}}

	res, err := tool.Execute(context.Background(), nil)
	require.NoError(t, err)
	var out gameStateOutput
	require.NoError(t, json.Unmarshal(res.Data, &out))
	require.NotNil(t, out.TFT)
	assert.True(t, out.TFT.Board.Known)
	assert.Empty(t, out.TFT.Board.Items)
	assert.True(t, out.TFT.Bench.Known)
	assert.Empty(t, out.TFT.Bench.Items)
}

func TestTFTGameStateTool_NoStateProvider(t *testing.T) {
	tool := &TFTGameStateTool{}
	// 始终可用（缺依赖由 Execute 内部检查）
	assert.True(t, tool.IsAvailable())

	res, err := tool.Execute(context.Background(), nil)
	assert.Error(t, err)
	assert.False(t, res.Success)
	assert.Contains(t, res.Error, "not configured")
}

func TestTFTGameStateTool_Description(t *testing.T) {
	tool := &TFTGameStateTool{}
	assert.Equal(t, "get_tft_game_state", tool.Name())
	desc := tool.Description()
	assert.Contains(t, desc, "TFT")
	assert.Contains(t, desc, "patch")
	assert.Contains(t, desc, "known")
}
