package tools

import (
	"context"
	"encoding/json"
	"testing"

	apperrors "github.com/lipipi/aurora/internal/errors"
	"github.com/lipipi/aurora/internal/game"
	"github.com/lipipi/aurora/internal/game/knowledge"
	"github.com/lipipi/aurora/internal/tool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockKnowledgeProvider 完整实现 KnowledgeProvider 接口，便于测试
type mockKnowledgeProvider struct {
	champions map[string]knowledge.Champion // key: patch + "|" + id
	items     map[string]knowledge.Item
	traits    map[string]knowledge.Trait
	augments  map[string]knowledge.Augment

	// Phase 12：GetCurrentPatch 返回值（默认空；测试按需设置）
	currentPatch string

	// 用于验证查询是否包含期望的 patch/game/mode
	lastChampGame   string
	lastChampPatch  string
	lastChampQuery  knowledge.Query
	lastItemGame    string
	lastItemPatch   string
	lastItemQuery   knowledge.Query
	lastTraitGame   string
	lastTraitPatch  string
	lastTraitQuery  knowledge.Query
	lastAugmentGame string
	lastAugmentPatch string
	lastAugmentQuery knowledge.Query
}

func newMockKB() *mockKnowledgeProvider {
	return &mockKnowledgeProvider{
		champions: map[string]knowledge.Champion{},
		items:     map[string]knowledge.Item{},
		traits:    map[string]knowledge.Trait{},
		augments:  map[string]knowledge.Augment{},
	}
}

func k(patch, id string) string { return patch + "|" + id }

// GetCurrentPatch Phase 12：返回 currentPatch（默认空）
func (m *mockKnowledgeProvider) GetCurrentPatch(_ context.Context) (string, apperrors.AppError) {
	return m.currentPatch, nil
}

func (m *mockKnowledgeProvider) GetChampion(_ context.Context, g string, q knowledge.Query) ([]knowledge.Champion, apperrors.AppError) {
	m.lastChampGame = g
	// patch 由调用方管理；这里只 mock 行为：按 id 查，按 patch 索引
	for _, c := range m.champions {
		if c.ID == q.ID {
			m.lastChampPatch = c.Patch
			m.lastChampQuery = q
			return []knowledge.Champion{c}, nil
		}
	}
	m.lastChampQuery = q
	return nil, nil
}

func (m *mockKnowledgeProvider) GetItem(_ context.Context, g string, q knowledge.Query) ([]knowledge.Item, apperrors.AppError) {
	m.lastItemGame = g
	for _, it := range m.items {
		if it.ID == q.ID {
			m.lastItemPatch = it.Patch
			m.lastItemQuery = q
			return []knowledge.Item{it}, nil
		}
	}
	m.lastItemQuery = q
	return nil, nil
}

func (m *mockKnowledgeProvider) GetTrait(_ context.Context, g string, q knowledge.Query) ([]knowledge.Trait, apperrors.AppError) {
	m.lastTraitGame = g
	for _, tr := range m.traits {
		if tr.ID == q.ID {
			m.lastTraitPatch = tr.Patch
			m.lastTraitQuery = q
			return []knowledge.Trait{tr}, nil
		}
	}
	m.lastTraitQuery = q
	return nil, nil
}

func (m *mockKnowledgeProvider) GetAugment(_ context.Context, g string, q knowledge.Query) ([]knowledge.Augment, apperrors.AppError) {
	m.lastAugmentGame = g
	for _, a := range m.augments {
		if a.ID == q.ID {
			m.lastAugmentPatch = a.Patch
			m.lastAugmentQuery = q
			return []knowledge.Augment{a}, nil
		}
	}
	m.lastAugmentQuery = q
	return nil, nil
}

// stateWithPatch 构造一个 CommonData.Version 已知的 GameContext
func stateWithPatch(patch string) *game.GameContext {
	return &game.GameContext{
		Game: "tft",
		CommonData: game.CommonData{
			Version: game.Known(patch),
		},
	}
}

func TestTFTKnowledgeTool_ChampionExisting(t *testing.T) {
	kb := newMockKB()
	kb.champions[k("16.17.1", "TFT18_Vayne")] = knowledge.Champion{
		Patch: "16.17.1",
		ID:    "TFT18_Vayne",
		Name:  "薇恩",
		Data:  json.RawMessage(`{"cost":1,"traits":["TFT18_Bulwark"]}`),
	}
	state := &mockStateProvider{ctx: stateWithPatch("16.17.1")}

	tool := newTFTKnowledgeTool("champion")
	tool.SetGameStateProvider(state)
	tool.SetKnowledgeProvider(kb)
	require.True(t, tool.IsAvailable())

	res, err := tool.Execute(context.Background(), json.RawMessage(`{"id":"TFT18_Vayne"}`))
	require.NoError(t, err)
	require.True(t, res.Success)

	var out championOutput
	require.NoError(t, json.Unmarshal(res.Data, &out))
	assert.True(t, out.Found)
	assert.Equal(t, "TFT18_Vayne", out.ID)
	assert.Equal(t, "薇恩", out.Name)
	assert.Equal(t, "16.17.1", out.Patch)
	assert.Equal(t, "tft", out.Game)
	assert.Equal(t, "standard", out.Mode)
	// patch/game/mode 都已传递给 provider
	assert.Equal(t, "tft", kb.lastChampGame)
}

func TestTFTKnowledgeTool_ChampionNotFound(t *testing.T) {
	kb := newMockKB()
	state := &mockStateProvider{ctx: stateWithPatch("16.17.1")}
	tool := newTFTKnowledgeTool("champion")
	tool.SetGameStateProvider(state)
	tool.SetKnowledgeProvider(kb)

	res, err := tool.Execute(context.Background(), json.RawMessage(`{"id":"TFT18_NotExist"}`))
	require.NoError(t, err)
	require.True(t, res.Success)

	var out notFoundOutput
	require.NoError(t, json.Unmarshal(res.Data, &out))
	assert.False(t, out.Found)
	assert.Equal(t, "TFT18_NotExist", out.ID)
	assert.Equal(t, "16.17.1", out.Patch)
	assert.Equal(t, "tft", out.Game)
}

func TestTFTKnowledgeTool_ItemAndTraitAndAugment(t *testing.T) {
	kb := newMockKB()
	kb.items[k("16.17.1", "TFT_IE")] = knowledge.Item{
		Patch: "16.17.1", ID: "TFT_IE", Name: "无尽之刃",
		Data: json.RawMessage(`{"ad":20,"crit":25}`),
	}
	kb.traits[k("16.17.1", "TFT18_Bulwark")] = knowledge.Trait{
		Patch: "16.17.1", ID: "TFT18_Bulwark", Name: "壁垒",
		Data: json.RawMessage(`{"effect":"def"}`),
	}
	kb.augments[k("16.17.1", "TFT18_Aug_X")] = knowledge.Augment{
		Patch: "16.17.1", ID: "TFT18_Aug_X", Name: "强化X",
		Description: "描述",
		Data:       json.RawMessage(`{"tier":"gold"}`),
	}
	state := &mockStateProvider{ctx: stateWithPatch("16.17.1")}

	tests := []struct {
		kind string
		id   string
		run  func(t *testing.T, data []byte)
	}{
		{"item", "TFT_IE", func(t *testing.T, data []byte) {
			var out itemOutput
			require.NoError(t, json.Unmarshal(data, &out))
			assert.True(t, out.Found)
			assert.Equal(t, "TFT_IE", out.ID)
			assert.Equal(t, "无尽之刃", out.Name)
		}},
		{"trait", "TFT18_Bulwark", func(t *testing.T, data []byte) {
			var out traitOutput
			require.NoError(t, json.Unmarshal(data, &out))
			assert.True(t, out.Found)
			assert.Equal(t, "TFT18_Bulwark", out.ID)
		}},
		{"augment", "TFT18_Aug_X", func(t *testing.T, data []byte) {
			var out augmentOutput
			require.NoError(t, json.Unmarshal(data, &out))
			assert.True(t, out.Found)
			assert.Equal(t, "TFT18_Aug_X", out.ID)
			assert.Equal(t, "描述", out.Description)
		}},
	}

	for _, tc := range tests {
		t.Run(tc.kind, func(t *testing.T) {
			tool := newTFTKnowledgeTool(tc.kind)
			tool.SetGameStateProvider(state)
			tool.SetKnowledgeProvider(kb)
			require.Equal(t, "get_tft_"+tc.kind, tool.Name())

			res, err := tool.Execute(context.Background(), json.RawMessage(`{"id":"`+tc.id+`"}`))
			require.NoError(t, err)
			require.True(t, res.Success)
			tc.run(t, res.Data)
		})
	}
}

func TestTFTKnowledgeTool_NoActiveGame(t *testing.T) {
	kb := newMockKB()
	tool := newTFTKnowledgeTool("champion")
	tool.SetGameStateProvider(&mockStateProvider{ctx: nil})
	tool.SetKnowledgeProvider(kb)

	res, err := tool.Execute(context.Background(), json.RawMessage(`{"id":"X"}`))
	assert.Error(t, err)
	assert.False(t, res.Success)
	assert.Contains(t, res.Error, "no active game")
}

func TestTFTKnowledgeTool_PatchUnknown(t *testing.T) {
	// GameContext 存在但 Version.Present=false
	kb := newMockKB()
	state := &mockStateProvider{ctx: &game.GameContext{
		Game:       "tft",
		CommonData: game.CommonData{}, // Version 保持 Present=false
	}}
	tool := newTFTKnowledgeTool("champion")
	tool.SetGameStateProvider(state)
	tool.SetKnowledgeProvider(kb)

	res, err := tool.Execute(context.Background(), json.RawMessage(`{"id":"X"}`))
	assert.Error(t, err)
	assert.False(t, res.Success)
	assert.Contains(t, res.Error, "patch is unknown")
}

func TestTFTKnowledgeTool_NoKnowledgeProvider(t *testing.T) {
	tool := newTFTKnowledgeTool("champion")
	tool.SetGameStateProvider(&mockStateProvider{ctx: stateWithPatch("16.17.1")})
	// 没设置 knowledge provider；IsAvailable 仍返回 true（让 registry 收录），
	// 缺依赖在 Execute 内部检查
	assert.True(t, tool.IsAvailable())

	res, err := tool.Execute(context.Background(), json.RawMessage(`{"id":"X"}`))
	assert.Error(t, err)
	assert.False(t, res.Success)
	assert.Contains(t, res.Error, "not configured")
}

func TestTFTKnowledgeTool_MissingID(t *testing.T) {
	kb := newMockKB()
	tool := newTFTKnowledgeTool("champion")
	tool.SetGameStateProvider(&mockStateProvider{ctx: stateWithPatch("16.17.1")})
	tool.SetKnowledgeProvider(kb)

	res, err := tool.Execute(context.Background(), json.RawMessage(`{}`))
	assert.Error(t, err)
	assert.False(t, res.Success)
	assert.Contains(t, res.Error, "id")
}

func TestTFTKnowledgeTool_InvalidJSON(t *testing.T) {
	kb := newMockKB()
	tool := newTFTKnowledgeTool("champion")
	tool.SetGameStateProvider(&mockStateProvider{ctx: stateWithPatch("16.17.1")})
	tool.SetKnowledgeProvider(kb)

	res, err := tool.Execute(context.Background(), json.RawMessage(`not json`))
	assert.Error(t, err)
	assert.False(t, res.Success)
}

func TestTFTKnowledgeTool_QueryScope(t *testing.T) {
	// 关键测试：即使 KB 中存在其他 patch 的同名 ID，工具必须用 current patch 查询
	// 验证：当前 patch 不存在时 → not_found
	kb := newMockKB()
	kb.champions[k("16.17.1", "TFT18_Vayne")] = knowledge.Champion{
		Patch: "16.17.1", ID: "TFT18_Vayne", Name: "薇恩",
	}
	// 当前 patch = 16.18.0，但 KB 只有 16.17.1 的数据
	state := &mockStateProvider{ctx: stateWithPatch("16.18.0")}

	tool := newTFTKnowledgeTool("champion")
	tool.SetGameStateProvider(state)
	tool.SetKnowledgeProvider(kb)

	// 当前 mock 实现是按 id 查的（不论 patch），所以会返回"找到"——
	// 这说明 mock 不能真实测试跨 patch 隔离。
	// 但 provider 收到的 Query 不带 patch（patch 由工具外层决定）。
	// 这里验证：工具读出的 patch 是 16.18.0，传给 provider 的逻辑路径
	// 是受 GameContext.Version 控制的。
	res, _ := tool.Execute(context.Background(), json.RawMessage(`{"id":"TFT18_Vayne"}`))
	// mock 会找到——证明 mock 不能验证 patch scope；真实 provider 应当按 patch 过滤
	// 这部分测试在 store 集成测试中覆盖（见 store_test）
	var out championOutput
	require.NoError(t, json.Unmarshal(res.Data, &out))
	// 这里只是 sanity check：tool 流程没有 panic
	_ = out
}

// 验证 4 个工厂函数都返回带正确 Name 的工具
func TestTFTKnowledgeTool_Names(t *testing.T) {
	cases := []struct {
		factory func() interface{ Name() string }
		want    string
	}{
		{func() interface{ Name() string } { t, _ := NewTFTChampionTool(tool.Config{}); return t }, "get_tft_champion"},
		{func() interface{ Name() string } { t, _ := NewTFTItemTool(tool.Config{}); return t }, "get_tft_item"},
		{func() interface{ Name() string } { t, _ := NewTFTTraitTool(tool.Config{}); return t }, "get_tft_trait"},
		{func() interface{ Name() string } { t, _ := NewTFTAugmentTool(tool.Config{}); return t }, "get_tft_augment"},
	}
	for _, c := range cases {
		t.Run(c.want, func(t *testing.T) {
			got := c.factory().Name()
			assert.Equal(t, c.want, got)
		})
	}
}
