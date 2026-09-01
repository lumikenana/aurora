package storage

import (
	"database/sql"
	"fmt"

	"github.com/lipipi/aurora/internal/errors"
	"github.com/lipipi/aurora/internal/log"
)

// Migration represents a single database migration
type Migration struct {
	Version int
	SQL     string
}

// migrations is the list of all migrations in order
var migrations = []Migration{
	{
		Version: 1,
		SQL: `
-- Create patches table
CREATE TABLE IF NOT EXISTS patches (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	version TEXT NOT NULL,
	release_date INTEGER NOT NULL,
	notes TEXT,
	created_at INTEGER NOT NULL,
	UNIQUE(version)
);

-- Create champions table
CREATE TABLE IF NOT EXISTS champions (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	patch_id INTEGER NOT NULL,
	key TEXT NOT NULL,
	name TEXT NOT NULL,
	cost INTEGER NOT NULL,
	data TEXT,
	created_at INTEGER NOT NULL,
	UNIQUE(patch_id, key),
	FOREIGN KEY(patch_id) REFERENCES patches(id) ON DELETE CASCADE
);

-- Create traits table
CREATE TABLE IF NOT EXISTS traits (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	patch_id INTEGER NOT NULL,
	key TEXT NOT NULL,
	name TEXT NOT NULL,
	description TEXT,
	data TEXT,
	created_at INTEGER NOT NULL,
	UNIQUE(patch_id, key),
	FOREIGN KEY(patch_id) REFERENCES patches(id) ON DELETE CASCADE
);

-- Create items table
CREATE TABLE IF NOT EXISTS items (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	patch_id INTEGER NOT NULL,
	key TEXT NOT NULL,
	name TEXT NOT NULL,
	description TEXT,
	data TEXT,
	created_at INTEGER NOT NULL,
	UNIQUE(patch_id, key),
	FOREIGN KEY(patch_id) REFERENCES patches(id) ON DELETE CASCADE
);

-- Create indexes
CREATE INDEX IF NOT EXISTS idx_champions_patch_id ON champions(patch_id);
CREATE INDEX IF NOT EXISTS idx_champions_key ON champions(key);
CREATE INDEX IF NOT EXISTS idx_traits_patch_id ON traits(patch_id);
CREATE INDEX IF NOT EXISTS idx_traits_key ON traits(key);
CREATE INDEX IF NOT EXISTS idx_items_patch_id ON items(patch_id);
CREATE INDEX IF NOT EXISTS idx_items_key ON items(key);
`,
	},
}

func (d *DB) runMigrations() error {
	// Create migration table
	createTableSQL := `
	CREATE TABLE IF NOT EXISTS migrations (
		version INTEGER PRIMARY KEY,
		applied_at INTEGER NOT NULL
	);
	`
	if _, err := d.db.Exec(createTableSQL); err != nil {
		return errors.Wrap(errors.CodeStorageError, "failed to create migrations table", err)
	}

	// Get current version
	var currentVersion int
	err := d.db.QueryRow(`SELECT COALESCE(MAX(version), 0) FROM migrations`).Scan(&currentVersion)
	if err != nil && err != sql.ErrNoRows {
		return errors.Wrap(errors.CodeStorageError, "failed to get current migration version", err)
	}

	log.Info("Current database migration version: %d", currentVersion)

	// Apply pending migrations
	for _, m := range migrations {
		if m.Version > currentVersion {
			log.Info("Applying migration version %d", m.Version)
			if err := d.applyMigration(m); err != nil {
				return err
			}
			log.Info("Applied migration version %d", m.Version)
		}
	}

	return nil
}

func (d *DB) applyMigration(m Migration) error {
	tx, err := d.db.Begin()
	if err != nil {
		return errors.Wrap(errors.CodeStorageError, fmt.Sprintf("failed to begin transaction for migration %d", m.Version), err)
	}

	// Execute migration SQL
	if _, err := tx.Exec(m.SQL); err != nil {
		tx.Rollback()
		return errors.Wrap(errors.CodeStorageError, fmt.Sprintf("failed to execute migration %d", m.Version), err)
	}

	// Record migration
	if _, err := tx.Exec(`INSERT INTO migrations (version, applied_at) VALUES (?, strftime('%s', 'now'))`, m.Version); err != nil {
		tx.Rollback()
		return errors.Wrap(errors.CodeStorageError, fmt.Sprintf("failed to record migration %d", m.Version), err)
	}

	if err := tx.Commit(); err != nil {
		return errors.Wrap(errors.CodeStorageError, fmt.Sprintf("failed to commit migration %d", m.Version), err)
	}

	return nil
}

// GetCurrentVersion returns the current migration version
func (d *DB) GetCurrentVersion() (int, error) {
	var version int
	err := d.db.QueryRow(`SELECT COALESCE(MAX(version), 0) FROM migrations`).Scan(&version)
	if err != nil && err != sql.ErrNoRows {
		return 0, errors.Wrap(errors.CodeStorageError, "failed to get current migration version", err)
	}
	return version, nil
}