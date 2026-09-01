package storage

import (
	"database/sql"
	"os"
	"path/filepath"

	_ "github.com/mattn/go-sqlite3"

	"github.com/lipipi/aurora/internal/errors"
	"github.com/lipipi/aurora/internal/log"
)

// DB represents the SQLite database connection
type DB struct {
	db  *sql.DB
	path string
}

// New creates a new database connection and initializes migrations
func New(dbPath string) (*DB, error) {
	// Ensure directory exists
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, errors.Wrap(errors.CodeStorageError, "failed to create database directory", err)
	}

	// Open database connection
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, errors.Wrap(errors.CodeStorageError, "failed to open database", err)
	}

	// Enable foreign keys
	if _, err := db.Exec("PRAGMA foreign_keys = ON;"); err != nil {
		db.Close()
		return nil, errors.Wrap(errors.CodeStorageError, "failed to enable foreign keys", err)
	}

	d := &DB{
		db:  db,
		path: dbPath,
	}

	// Run migrations
	if err := d.runMigrations(); err != nil {
		db.Close()
		return nil, errors.Wrap(errors.CodeStorageError, "failed to run migrations", err)
	}

	log.Info("Database initialized at %s", dbPath)
	return d, nil
}

// Close closes the database connection
func (d *DB) Close() error {
	if d.db != nil {
		return d.db.Close()
	}
	return nil
}

// DB returns the underlying sql.DB connection
func (d *DB) DB() *sql.DB {
	return d.db
}

// Path returns the database file path
func (d *DB) Path() string {
	return d.path
}

// Begin starts a new transaction
func (d *DB) Begin() (*sql.Tx, error) {
	return d.db.Begin()
}
