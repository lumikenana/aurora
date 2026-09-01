//go:build cgo

package store

import (
	"context"
	"os"
	"testing"

	"github.com/lipipi/aurora/internal/game/knowledge"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSQLiteStore_QueryAugment 验证新加的 QueryAugment 严格限定 (game, patch, mode)
//
// 关键不变量：不同 patch 的同名 augment 不能跨版本返回。
func TestSQLiteStore_QueryAugment(t *testing.T) {
	tmp, err := os.CreateTemp("", "gameai-store-aug-*.db")
	require.NoError(t, err)
	tmp.Close()
	defer os.Remove(tmp.Name())

	st, err := Open(tmp.Name())
	require.NoError(t, err)
	defer st.Close()

	ctx := context.Background()

	// 导入两个 patch 的 augments
	importPatch := func(patch string) {
		_, perr := st.ImportNormalized(ctx, knowledge.NormalizedPatchData{
			Game:    "tft",
			SetKey:  "set13",
			Patch:   patch,
			Mode:    "standard",
			Source:  "test",
			Augments: []knowledge.Record{
				{ID: "TFT18_Aug_X", Name: "强化X", Data: []byte(`{"tier":"gold"}`)},
				{ID: "TFT18_Aug_Y", Name: "强化Y", Data: []byte(`{"tier":"silver"}`)},
			},
		})
		require.NoError(t, perr)
	}
	importPatch("16.17.1")
	importPatch("16.18.0")

	// 16.17.1 查到
	t.Run("query patch 16.17.1", func(t *testing.T) {
		got, err := st.QueryAugment(ctx, "tft", "16.17.1", knowledge.Query{ID: "TFT18_Aug_X"})
		require.NoError(t, err)
		require.Len(t, got, 1)
		assert.Equal(t, "TFT18_Aug_X", got[0].ID)
		assert.Equal(t, "16.17.1", got[0].Patch)
		assert.Equal(t, "强化X", got[0].Name)
	})

	// 16.18.0 也能查到（同名 ID）
	t.Run("query patch 16.18.0", func(t *testing.T) {
		got, err := st.QueryAugment(ctx, "tft", "16.18.0", knowledge.Query{ID: "TFT18_Aug_X"})
		require.NoError(t, err)
		require.Len(t, got, 1)
		assert.Equal(t, "16.18.0", got[0].Patch)
	})

	// 不存在的 patch → 0 条
	t.Run("query unknown patch", func(t *testing.T) {
		got, err := st.QueryAugment(ctx, "tft", "99.99.9", knowledge.Query{ID: "TFT18_Aug_X"})
		require.NoError(t, err)
		assert.Empty(t, got, "cross-patch data leak must NOT happen")
	})

	// 不存在的 game → 0 条
	t.Run("query wrong game", func(t *testing.T) {
		got, err := st.QueryAugment(ctx, "lol", "16.17.1", knowledge.Query{ID: "TFT18_Aug_X"})
		require.NoError(t, err)
		assert.Empty(t, got, "cross-game data leak must NOT happen")
	})
}

// TestSQLiteStore_QueryAugment_NotFound 验证空查询
func TestSQLiteStore_QueryAugment_NotFound(t *testing.T) {
	tmp, err := os.CreateTemp("", "gameai-store-aug-empty-*.db")
	require.NoError(t, err)
	tmp.Close()
	defer os.Remove(tmp.Name())

	st, err := Open(tmp.Name())
	require.NoError(t, err)
	defer st.Close()

	ctx := context.Background()

	// 先导入一些
	_, perr := st.ImportNormalized(ctx, knowledge.NormalizedPatchData{
		Game: "tft", Patch: "16.17.1", Mode: "standard", SetKey: "set13",
		Augments: []knowledge.Record{
			{ID: "TFT18_Aug_X", Name: "强化X", Data: []byte(`{}`)},
		},
	})
	require.NoError(t, perr)

	got, err := st.QueryAugment(ctx, "tft", "16.17.1", knowledge.Query{ID: "NotExist"})
	require.NoError(t, err)
	assert.Empty(t, got)
}
