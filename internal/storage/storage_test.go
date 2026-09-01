package storage

import (
	"os"
	"testing"
	"path/filepath"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDatabaseInitialization(t *testing.T) {
	// Create temp directory for test
	tmpDir, err := os.MkdirTemp("", "gameai-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, "gameai.db")
	db, err := New(dbPath)
	require.NoError(t, err)
	defer db.Close()

	// Check current version
	version, err := db.GetCurrentVersion()
	require.NoError(t, err)
	assert.Equal(t, 1, version)
}

func TestTablesExist(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "gameai-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, "gameai.db")
	db, err := New(dbPath)
	require.NoError(t, err)
	defer db.Close()

	// Check that all tables exist
	tables := []string{"migrations", "patches", "champions", "traits", "items"}
	for _, table := range tables {
		var exists int
		err := db.DB().QueryRow(`
			SELECT COUNT(*) FROM sqlite_master
			WHERE type='table' AND name=?`, table).Scan(&exists)
		require.NoError(t, err)
		assert.Equal(t, 1, exists, "table %s should exist", table)
	}
}

func TestIndexesExist(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "gameai-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, "gameai.db")
	db, err := New(dbPath)
	require.NoError(t, err)
	defer db.Close()

	// Check indexes
	indexes := []string{
		"idx_champions_patch_id",
		"idx_champions_key",
		"idx_traits_patch_id",
		"idx_traits_key",
		"idx_items_patch_id",
		"idx_items_key",
	}

	for _, idx := range indexes {
		var exists int
		err := db.DB().QueryRow(`
			SELECT COUNT(*) FROM sqlite_master
			WHERE type='index' AND name=?`, idx).Scan(&exists)
		require.NoError(t, err)
		assert.Equal(t, 1, exists, "index %s should exist", idx)
	}
}

func TestForeignKeysEnabled(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "gameai-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, "gameai.db")
	db, err := New(dbPath)
	require.NoError(t, err)
	defer db.Close()

	var enabled int
	err = db.DB().QueryRow("PRAGMA foreign_keys").Scan(&enabled)
	require.NoError(t, err)
	assert.Equal(t, 1, enabled, "foreign keys should be enabled")
}

func TestInsertPatch(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "gameai-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, "gameai.db")
	db, err := New(dbPath)
	require.NoError(t, err)
	defer db.Close()

	// Insert a test patch
	res, err := db.DB().Exec(`
		INSERT INTO patches (version, release_date, notes, created_at)
		VALUES (?, ?, ?, strftime('%s', 'now'))`,
		"14.1", 1735689600, "Test patch notes")
	require.NoError(t, err)

	id, err := res.LastInsertId()
	require.NoError(t, err)
	assert.True(t, id > 0)
}

func TestInsertChampion(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "gameai-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, "gameai.db")
	db, err := New(dbPath)
	require.NoError(t, err)
	defer db.Close()

	// Insert patch first
	res, err := db.DB().Exec(`
		INSERT INTO patches (version, release_date, notes, created_at)
		VALUES (?, ?, ?, strftime('%s', 'now'))`,
		"14.1", 1735689600, "Test patch")
	require.NoError(t, err)
	patchID, _ := res.LastInsertId()

	// Insert champion
	res, err = db.DB().Exec(`
		INSERT INTO champions (patch_id, key, name, cost, created_at)
		VALUES (?, ?, ?, ?, strftime('%s', 'now'))`,
		patchID, "ahri", "Ahri", 4)
	require.NoError(t, err)

	id, err := res.LastInsertId()
	require.NoError(t, err)
	assert.True(t, id > 0)
}
