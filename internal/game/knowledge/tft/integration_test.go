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

// TestData represents the test data structure
type TestData struct {
	Game         string                   `json:"game"`
	Patch        string                   `json:"patch"`
	Champions    []TestChampion          `json:"champions"`
	Items        []TestItem              `json:"items"`
	Traits       []TestTrait             `json:"traits"`
	Compositions []json.RawMessage      `json:"compositions"`
}

type TestChampion struct {
	ID   string          `json:"id"`
	Name string          `json:"name"`
	Data json.RawMessage `json:"data"`
}

type TestItem struct {
	ID   string          `json:"id"`
	Name string          `json:"name"`
	Data json.RawMessage `json:"data"`
}

type TestTrait struct {
	ID   string          `json:"id"`
	Name string          `json:"name"`
	Data json.RawMessage `json:"data"`
}

// Test_IntegrationWithTestData tests the full flow with external test data file
func Test_IntegrationWithTestData(t *testing.T) {
	// Load test data - try multiple locations to handle different working directories
	var dataBytes []byte
	var err error
	dataBytes, err = os.ReadFile("../../../scripts/testdata/tft_test_data.json")
	if err != nil {
		dataBytes, err = os.ReadFile("scripts/testdata/tft_test_data.json")
		if err != nil {
			dataBytes, err = os.ReadFile("../../../../scripts/testdata/tft_test_data.json")
		}
	}
	require.NoError(t, err)

	var testData TestData
	err = json.Unmarshal(dataBytes, &testData)
	require.NoError(t, err)

	// Create temp DB
	tmpFile, err := os.CreateTemp("", "gameai-tft-integration-test-*.db")
	require.NoError(t, err)
	tmpFile.Close()
	defer os.Remove(tmpFile.Name())

	db, err := storage.New(tmpFile.Name())
	require.NoError(t, err)
	defer db.Close()

	// Insert test patch
	_, err = db.DB().Exec(`
		INSERT INTO patches (version, release_date, notes, created_at)
		VALUES (?, ?, ?, strftime('%s', 'now'))
	`, testData.Patch, 1717209600, "Integration test data")
	require.NoError(t, err)

	var patchID int64
	err = db.DB().QueryRow(`SELECT id FROM patches WHERE version = ?`, testData.Patch).Scan(&patchID)
	require.NoError(t, err)

	// Insert champions
	for _, c := range testData.Champions {
		var cost int
		if len(c.Data) > 0 {
			var dataMap map[string]interface{}
			_ = json.Unmarshal(c.Data, &dataMap)
			if c, ok := dataMap["cost"].(float64); ok {
				cost = int(c)
			}
		}
		_, err := db.DB().Exec(`
			INSERT INTO champions (patch_id, key, name, cost, created_at, data)
			VALUES (?, ?, ?, ?, strftime('%s', 'now'), ?)
		`, patchID, c.ID, c.Name, cost, string(c.Data))
		require.NoError(t, err)
	}

	// Insert items
	for _, it := range testData.Items {
		_, err := db.DB().Exec(`
			INSERT INTO items (patch_id, key, name, created_at, data)
			VALUES (?, ?, ?, strftime('%s', 'now'), ?)
		`, patchID, it.ID, it.Name, string(it.Data))
		require.NoError(t, err)
	}

	// Insert traits
	for _, tr := range testData.Traits {
		_, err := db.DB().Exec(`
			INSERT INTO traits (patch_id, key, name, created_at, data)
			VALUES (?, ?, ?, strftime('%s', 'now'), ?)
		`, patchID, tr.ID, tr.Name, string(tr.Data))
		require.NoError(t, err)
	}

	// Test queries
	repo := NewSQLiteRepository(db)
	provider := New(repo)
	ctx := context.Background()

	// Test HasPatch
	has, err := provider.HasPatch(ctx, testData.Patch)
	require.NoError(t, err)
	assert.True(t, has)

	// Test GetChampion by name
	champ, err := provider.GetChampion(ctx, testData.Patch, "金克丝")
	require.NoError(t, err)
	require.NotNil(t, champ)
	assert.Equal(t, "TFT14_Jinx", champ.ID)
	assert.Equal(t, "金克丝", champ.Name)
	assert.NotEmpty(t, champ.Data)

	// Test GetChampion by key
	champ, err = provider.GetChampion(ctx, testData.Patch, "TFT14_Vi")
	require.NoError(t, err)
	require.NotNil(t, champ)
	assert.Equal(t, "蔚", champ.Name)

	// Test fuzzy matching
	champ, err = provider.GetChampion(ctx, testData.Patch, "凯特")
	require.NoError(t, err)
	require.NotNil(t, champ)
	assert.Equal(t, "凯特琳", champ.Name)

	// Test GetItem
	item, err := provider.GetItem(ctx, testData.Patch, "无尽")
	require.NoError(t, err)
	require.NotNil(t, item)
	assert.Equal(t, "InfinityEdge", item.ID)
	assert.Equal(t, "无尽之刃", item.Name)

	// Test GetTrait
	trait, err := provider.GetTrait(ctx, testData.Patch, "狙神")
	require.NoError(t, err)
	require.NotNil(t, trait)
	assert.Equal(t, "Sniper", trait.ID)
	assert.Equal(t, "狙神", trait.Name)

	// Test ListChampions
	champs, err := provider.ListChampions(ctx, testData.Patch)
	require.NoError(t, err)
	assert.Len(t, champs, 3)
	// Should be ordered by cost descending
	assert.Equal(t, "金克丝", champs[0].Name) // cost 4
	assert.Equal(t, "蔚", champs[1].Name)       // cost 2
	assert.Equal(t, "凯特琳", champs[2].Name)   // cost 1

	// Test ListItems
	items, err := provider.ListItems(ctx, testData.Patch)
	require.NoError(t, err)
	assert.Len(t, items, 2)

	// Test ListTraits
	traits, err := provider.ListTraits(ctx, testData.Patch)
	require.NoError(t, err)
	assert.Len(t, traits, 2)

	// Test GetPatch
	patchInfo, err := provider.GetPatch(ctx, testData.Patch)
	require.NoError(t, err)
	require.NotNil(t, patchInfo)
	assert.Equal(t, "14.1", patchInfo.Version)
	assert.Equal(t, int64(1717209600), patchInfo.ReleaseDate)

	// Test not found
	notFound, err := provider.GetChampion(ctx, testData.Patch, "NotFound")
	require.NoError(t, err)
	assert.Nil(t, notFound)
}
