//go:build cgo
// +build cgo

package tft

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/lipipi/aurora/internal/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSQLiteRepository_Queries(t *testing.T) {
	// Create temporary in-memory database (actually file-based for testing)
	tmpFile, err := os.CreateTemp("", "gameai-tft-test-*.db")
	require.NoError(t, err)
	tmpFile.Close()
	defer os.Remove(tmpFile.Name())

	// Open database and run migrations
	db, err := storage.New(tmpFile.Name())
	require.NoError(t, err)
	defer db.Close()

	// Insert test patch
	_, err = db.DB().Exec(`
		INSERT INTO patches (version, release_date, notes, created_at)
		VALUES (?, ?, ?, strftime('%s', 'now'))
	`, "14.1", 1717209600, "Test patch")
	require.NoError(t, err)

	// Get patch ID
	var patchID int64
	err = db.DB().QueryRow(`SELECT id FROM patches WHERE version = ?`, "14.1").Scan(&patchID)
	require.NoError(t, err)

	// Insert test champions
	champions := []struct {
		key  string
		name string
		cost int
		data string
	}{
		{"TFT14_Jinx", "金克丝", 4, `{"health": 1000, "attack": 100}`},
		{"TFT14_Vi", "蔚", 2, `{"health": 800, "attack": 60}`},
		{"TFT14_Caitlyn", "凯特琳", 1, `{"health": 600, "attack": 50}`},
	}
	for _, c := range champions {
		_, err := db.DB().Exec(`
			INSERT INTO champions (patch_id, key, name, cost, created_at, data)
			VALUES (?, ?, ?, ?, strftime('%s', 'now'), ?)
		`, patchID, c.key, c.name, c.cost, c.data)
		require.NoError(t, err)
	}

	// Insert test items
	items := []struct {
		key  string
		name string
		data string
	}{
		{"InfinityEdge", "无尽之刃", `{"ad": 20, "crit": 25}`},
		{"Bloodthirster", "饮血剑", `{"ad": 15, "lifesteal": 20}`},
	}
	for _, it := range items {
		_, err := db.DB().Exec(`
			INSERT INTO items (patch_id, key, name, created_at, data)
			VALUES (?, ?, ?, strftime('%s', 'now'), ?)
		`, patchID, it.key, it.name, it.data)
		require.NoError(t, err)
	}

	// Insert test traits
	traits := []struct {
		key  string
		name string
		data string
	}{
		{"RocketInnovation", "火箭创新", `{"bonus": "攻击速度加成"}`},
		{"Sniper", "狙神", `{"bonus": "暴击伤害加成"}`},
	}
	for _, tr := range traits {
		_, err := db.DB().Exec(`
			INSERT INTO traits (patch_id, key, name, created_at, data)
			VALUES (?, ?, ?, strftime('%s', 'now'), ?)
		`, patchID, tr.key, tr.name, tr.data)
		require.NoError(t, err)
	}

	repo := NewSQLiteRepository(db)
	ctx := context.Background()

	// Test GetChampion by key
	t.Run("GetChampion by key", func(t *testing.T) {
		c, err := repo.GetChampion(ctx, "14.1", "TFT14_Jinx")
		require.NoError(t, err)
		require.NotNil(t, c)
		assert.Equal(t, "14.1", c.Patch)
		assert.Equal(t, "TFT14_Jinx", c.ID)
		assert.Equal(t, "金克丝", c.Name)
		var data map[string]interface{}
		jsonErr := json.Unmarshal(c.Data, &data)
		require.NoError(t, jsonErr)
		assert.Equal(t, float64(1000), data["health"])
	})

	// Test GetChampion by fuzzy name
	t.Run("GetChampion by fuzzy name", func(t *testing.T) {
		c, err := repo.GetChampion(ctx, "14.1", "金")
		require.NoError(t, err)
		require.NotNil(t, c)
		assert.Equal(t, "金克丝", c.Name)
	})

	// Test GetChampion not found
	t.Run("GetChampion not found", func(t *testing.T) {
		c, err := repo.GetChampion(ctx, "14.1", "NotFound")
		require.NoError(t, err)
		assert.Nil(t, c)
	})

	// Test GetItem by key
	t.Run("GetItem by key", func(t *testing.T) {
		it, err := repo.GetItem(ctx, "14.1", "InfinityEdge")
		require.NoError(t, err)
		require.NotNil(t, it)
		assert.Equal(t, "14.1", it.Patch)
		assert.Equal(t, "InfinityEdge", it.ID)
		assert.Equal(t, "无尽之刃", it.Name)
	})

	// Test GetTrait by name
	t.Run("GetTrait by name", func(t *testing.T) {
		tr, err := repo.GetTrait(ctx, "14.1", "狙神")
		require.NoError(t, err)
		require.NotNil(t, tr)
		assert.Equal(t, "Sniper", tr.ID)
		assert.Equal(t, "狙神", tr.Name)
	})

	// Test ListChampions
	t.Run("ListChampions", func(t *testing.T) {
		list, err := repo.ListChampions(ctx, "14.1")
		require.NoError(t, err)
		assert.Len(t, list, 3)
		// Ordered by cost descending then name ascending
		// Jinx (4), Vi (2), Caitlyn (1)
		assert.Equal(t, "金克丝", list[0].Name)
		assert.Equal(t, "蔚", list[1].Name)
		assert.Equal(t, "凯特琳", list[2].Name)
	})

	// Test ListItems
	t.Run("ListItems", func(t *testing.T) {
		list, err := repo.ListItems(ctx, "14.1")
		require.NoError(t, err)
		assert.Len(t, list, 2)
	})

	// Test ListTraits
	t.Run("ListTraits", func(t *testing.T) {
		list, err := repo.ListTraits(ctx, "14.1")
		require.NoError(t, err)
		assert.Len(t, list, 2)
	})

	// Test HasPatch
	t.Run("HasPatch existing", func(t *testing.T) {
		exists, err := repo.HasPatch(ctx, "14.1")
		require.NoError(t, err)
		assert.True(t, exists)
	})

	t.Run("HasPatch non-existing", func(t *testing.T) {
		exists, err := repo.HasPatch(ctx, "99.9")
		require.NoError(t, err)
		assert.False(t, exists)
	})

	// Test GetPatch
	t.Run("GetPatch", func(t *testing.T) {
		p, err := repo.GetPatch(ctx, "14.1")
		require.NoError(t, err)
		require.NotNil(t, p)
		assert.Equal(t, "14.1", p.Version)
		assert.Equal(t, int64(1717209600), p.ReleaseDate)
		assert.Equal(t, "Test patch", p.Notes)
	})

	t.Run("GetPatch not found", func(t *testing.T) {
		p, err := repo.GetPatch(ctx, "99.9")
		require.NoError(t, err)
		assert.Nil(t, p)
	})
}

func TestTFTKnowledgeProvider_Integration(t *testing.T) {
	// Create temporary database
	tmpFile, err := os.CreateTemp("", "gameai-tft-provider-test-*.db")
	require.NoError(t, err)
	tmpFile.Close()
	defer os.Remove(tmpFile.Name())

	db, err := storage.New(tmpFile.Name())
	require.NoError(t, err)
	defer db.Close()

	// Insert test data
	_, err = db.DB().Exec(`
		INSERT INTO patches (version, release_date, notes, created_at)
		VALUES (?, ?, ?, strftime('%s', 'now'))
	`, "14.2", 1720000000, "Another test patch")
	require.NoError(t, err)

	var patchID int64
	err = db.DB().QueryRow(`SELECT id FROM patches WHERE version = ?`, "14.2").Scan(&patchID)
	require.NoError(t, err)

	_, err = db.DB().Exec(`
		INSERT INTO champions (patch_id, key, name, cost, created_at, data)
		VALUES (?, ?, ?, ?, strftime('%s', 'now'), '{}')
	`, patchID, "TFT14_Ahri", "阿狸", 3, "{}")
	require.NoError(t, err)

	// Create provider with sqlite repo
	repo := NewSQLiteRepository(db)
	provider := New(repo)
	ctx := context.Background()

	// Test provider interface
	champ, err := provider.GetChampion(ctx, "14.2", "阿狸")
	require.NoError(t, err)
	require.NotNil(t, champ)
	assert.Equal(t, "阿狸", champ.Name)
	assert.Equal(t, "14.2", champ.Patch)

	// List all
	champs, err := provider.ListChampions(ctx, "14.2")
	require.NoError(t, err)
	assert.Len(t, champs, 1)
}
