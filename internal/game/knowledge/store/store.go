//go:build cgo

// Package store 知识库 SQLite 持久化实现
//
// 本包是整个代码树中唯一 import mattn/go-sqlite3 的包（ADR-022），
// 只被 main.go import —— agent / tool 等包依赖 knowledge 包接口，保持可交叉编译。
package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/lipipi/aurora/internal/errors"
	"github.com/lipipi/aurora/internal/game/knowledge"
	"github.com/lipipi/aurora/internal/log"

	_ "github.com/mattn/go-sqlite3" // SQLite 驱动注册
)

// queryLimit 无条件查询（Name/ID 都为空）的返回上限
const queryLimit = 50

// SQLiteStore 知识库存储
type SQLiteStore struct {
	db *sql.DB
}

// Open 打开（或创建）知识库数据库
// 损坏恢复：quick_check 探针失败 → 旧库改名 .corrupt-<ts> → 重建空库
func Open(path string) (*SQLiteStore, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return nil, fmt.Errorf("create knowledge db dir: %w", err)
	}

	// 损坏探针（在真正打开前用内存方式验证不了文件，直接打开后跑 quick_check）
	db, err := sql.Open("sqlite3", path+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		return nil, fmt.Errorf("open knowledge db: %w", err)
	}
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, recoverFromCorrupt(path, err)
	}
	var check string
	if err := db.QueryRow("PRAGMA quick_check").Scan(&check); err != nil || check != "ok" {
		db.Close()
		if cerr := recoverFromCorrupt(path, err); cerr != nil {
			return nil, cerr
		}
		// 重建后重新打开
		db, err = sql.Open("sqlite3", path+"?_journal_mode=WAL&_busy_timeout=5000")
		if err != nil {
			return nil, fmt.Errorf("reopen knowledge db: %w", err)
		}
	}

	s := &SQLiteStore{db: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

// recoverFromCorrupt 把损坏的库改名并让调用方重建
func recoverFromCorrupt(path string, origErr error) error {
	newPath := path + ".corrupt-" + time.Now().Format("20060102-150405")
	if err := os.Rename(path, newPath); err != nil {
		// 改名失败（文件不存在等）不视为致命
		log.Warnf("knowledge db: corrupt recovery rename failed: %v (original error: %v)", err, origErr)
		return nil
	}
	log.Warnf("knowledge db corrupted, moved to %s, recreating empty db (original error: %v)", newPath, origErr)
	return nil
}

// migrate 建表（幂等）
func (s *SQLiteStore) migrate() error {
	stmts := []string{
		// Create sets table (new)
		`CREATE TABLE IF NOT EXISTS sets (
			game TEXT NOT NULL,
			set_key TEXT NOT NULL,
			set_number INTEGER NOT NULL,
			name TEXT NOT NULL,
			start_date INTEGER,
			PRIMARY KEY (game, set_key)
		)`,
		// Create patches table with Set/Mode
		`CREATE TABLE IF NOT EXISTS patches (
			patch TEXT NOT NULL,
			game TEXT NOT NULL,
			set_key TEXT NOT NULL,
			mode TEXT NOT NULL DEFAULT 'standard',
			downloaded_at TEXT NOT NULL,
			source TEXT NOT NULL,
			PRIMARY KEY (patch, game, mode)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_patches_set_key ON patches(game, set_key)`,
		// Create champions with Set/Mode
		`CREATE TABLE IF NOT EXISTS champions (
			patch TEXT NOT NULL,
			game TEXT NOT NULL,
			mode TEXT NOT NULL DEFAULT 'standard',
			id TEXT NOT NULL,
			name TEXT NOT NULL,
			set_key TEXT NOT NULL,
			data TEXT NOT NULL,
			PRIMARY KEY (patch, game, mode, id)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_champions_name ON champions(game, patch, mode, name)`,
		// Create items with Set/Mode
		`CREATE TABLE IF NOT EXISTS items (
			patch TEXT NOT NULL,
			game TEXT NOT NULL,
			mode TEXT NOT NULL DEFAULT 'standard',
			id TEXT NOT NULL,
			name TEXT NOT NULL,
			set_key TEXT NOT NULL,
			data TEXT NOT NULL,
			PRIMARY KEY (patch, game, mode, id)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_items_name ON items(game, patch, mode, name)`,
		// Create traits with Set/Mode
		`CREATE TABLE IF NOT EXISTS traits (
			patch TEXT NOT NULL,
			game TEXT NOT NULL,
			mode TEXT NOT NULL DEFAULT 'standard',
			id TEXT NOT NULL,
			name TEXT NOT NULL,
			set_key TEXT NOT NULL,
			data TEXT NOT NULL,
			PRIMARY KEY (patch, game, mode, id)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_traits_name ON traits(game, patch, mode, name)`,
		// Create compositions with Set/Mode
		`CREATE TABLE IF NOT EXISTS compositions (
			patch TEXT NOT NULL,
			game TEXT NOT NULL,
			mode TEXT NOT NULL DEFAULT 'standard',
			id TEXT NOT NULL,
			name TEXT NOT NULL,
			set_key TEXT NOT NULL,
			data TEXT NOT NULL,
			PRIMARY KEY (patch, game, mode, id)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_compositions_name ON compositions(game, patch, mode, name)`,
		// Create augments with Set/Mode
		`CREATE TABLE IF NOT EXISTS augments (
			patch TEXT NOT NULL,
			game TEXT NOT NULL,
			mode TEXT NOT NULL DEFAULT 'standard',
			id TEXT NOT NULL,
			name TEXT NOT NULL,
			description TEXT,
			set_key TEXT NOT NULL,
			data TEXT NOT NULL,
			PRIMARY KEY (patch, game, mode, id)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_augments_name ON augments(game, patch, mode, name)`,
	}
	for _, stmt := range stmts {
		if _, err := s.db.Exec(stmt); err != nil {
			return fmt.Errorf("migrate knowledge db: %w", err)
		}
	}
	return nil
}

// Close 关闭数据库
func (s *SQLiteStore) Close() error { return s.db.Close() }

// ImportNormalized 原子导入：单事务内只替换该 (game, patch) 的数据，其他版本不动（版本隔离）
func (s *SQLiteStore) ImportNormalized(ctx context.Context, d knowledge.NormalizedPatchData) (knowledge.ImportSummary, errors.AppError) {
	if d.Game == "" || d.Patch == "" {
		return knowledge.ImportSummary{}, errors.New(errors.CodeInvalidConfig, "import requires game and patch")
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return knowledge.ImportSummary{}, wrapStorage("begin import tx", err)
	}
	defer tx.Rollback() // 已提交时为 no-op

	// Defaults
	if d.Mode == "" {
		d.Mode = "standard"
	}
	if d.SetKey == "" {
		d.SetKey = "unknown"
	}

	summary := knowledge.ImportSummary{Patch: d.Patch}

	// 只清空当前 (game, patch, mode)，绝不触碰其他版本/模式
	for _, table := range []string{"champions", "items", "traits", "compositions", "augments"} {
		if _, err := tx.ExecContext(ctx,
			fmt.Sprintf("DELETE FROM %s WHERE game=? AND patch=? AND mode=?", table), d.Game, d.Patch, d.Mode); err != nil {
			return summary, wrapStorage("clear "+table, err)
		}
	}

	summary.Champions, err = insertRecords(ctx, tx, "champions", d)
	if err != nil {
		return summary, wrapStorage("insert champions", err)
	}
	summary.Items, err = insertRecords(ctx, tx, "items", d)
	if err != nil {
		return summary, wrapStorage("insert items", err)
	}
	summary.Traits, err = insertRecords(ctx, tx, "traits", d)
	if err != nil {
		return summary, wrapStorage("insert traits", err)
	}
	summary.Compositions, err = insertRecords(ctx, tx, "compositions", d)
	if err != nil {
		return summary, wrapStorage("insert compositions", err)
	}
	summary.Augments, err = insertRecords(ctx, tx, "augments", d)
	if err != nil {
		return summary, wrapStorage("insert augments", err)
	}

	source := d.Source
	if source == "" {
		source = "unknown"
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO patches (patch, game, set_key, mode, downloaded_at, source) VALUES (?, ?, ?, ?, ?, ?)
		 ON CONFLICT(patch, game, mode) DO UPDATE SET set_key=excluded.set_key, downloaded_at=excluded.downloaded_at, source=excluded.source`,
		d.Patch, d.Game, d.SetKey, d.Mode, time.Now().Format(time.RFC3339), source); err != nil {
		return summary, wrapStorage("upsert patches", err)
	}

	if err := tx.Commit(); err != nil {
		return summary, wrapStorage("commit import", err)
	}
	return summary, nil
}

// insertRecords 插入一类记录并返回数量
func insertRecords(ctx context.Context, tx *sql.Tx, table string, d knowledge.NormalizedPatchData) (int, error) {
	var records []knowledge.Record
	switch table {
	case "champions":
		records = d.Champions
	case "items":
		records = d.Items
	case "traits":
		records = d.Traits
	case "compositions":
		records = d.Compositions
	case "augments":
		records = d.Augments
	}
	if len(records) == 0 {
		return 0, nil
	}
	stmt, err := tx.PrepareContext(ctx,
		fmt.Sprintf("INSERT OR REPLACE INTO %s (patch, game, mode, id, name, set_key, data) VALUES (?, ?, ?, ?, ?, ?, ?)", table))
	if err != nil {
		return 0, err
	}
	defer stmt.Close()
	for _, r := range records {
		data := r.Data
		if len(data) == 0 {
			data = json.RawMessage("{}")
		}
		if _, err := stmt.ExecContext(ctx, d.Patch, d.Game, d.Mode, r.ID, r.Name, d.SetKey, string(data)); err != nil {
			return 0, err
		}
	}
	return len(records), nil
}

// buildWhere 构造版本域查询条件：强制 WHERE game=? AND patch=? AND mode=?，再加 ID 精确 / Name 模糊
func buildWhere(q knowledge.Query, mode string) (string, []interface{}) {
	conds := []string{"game=?", "patch=?", "mode=?"}
	args := []interface{}{mode}
	// NOTE: game and patch will be prepended by prependArgs, so we only add mode/q here
	if q.ID != "" {
		conds = append(conds, "id=?")
		args = append(args, q.ID)
	}
	if q.Name != "" {
		conds = append(conds, "lower(name) LIKE '%'||lower(?)||'%'")
		args = append(args, q.Name)
	}
	if len(args) == 0 {
		// 无条件 → 限量返回全部
		return strings.Join(conds, " AND ") + " LIMIT " + fmt.Sprint(queryLimit), args
	}
	return strings.Join(conds, " AND ") + " LIMIT " + fmt.Sprint(queryLimit), args
}

func wrapStorage(op string, err error) errors.AppError {
	return errors.Wrap(errors.CodeStorageError, "knowledge store: "+op+": "+err.Error(), err)
}

// QueryChampion 查询英雄（版本域）
func (s *SQLiteStore) QueryChampion(ctx context.Context, game, patch string, q knowledge.Query) ([]knowledge.Champion, errors.AppError) {
	where, args := buildWhere(q, "standard")
	rows, err := s.db.QueryContext(ctx,
		"SELECT patch, id, name, data FROM champions WHERE "+where, prependArgs(args, game, patch)...)
	if err != nil {
		return nil, wrapStorage("query champions", err)
	}
	defer rows.Close()
	var out []knowledge.Champion
	for rows.Next() {
		var c knowledge.Champion
		var data string
		if err := rows.Scan(&c.Patch, &c.ID, &c.Name, &data); err != nil {
			return nil, wrapStorage("scan champion", err)
		}
		c.Data = json.RawMessage(data)
		out = append(out, c)
	}
	return out, wrapIfErr(rows.Err())
}

// QueryItem 查询装备（版本域）
func (s *SQLiteStore) QueryItem(ctx context.Context, game, patch string, q knowledge.Query) ([]knowledge.Item, errors.AppError) {
	where, args := buildWhere(q, "standard")
	rows, err := s.db.QueryContext(ctx,
		"SELECT patch, id, name, data FROM items WHERE "+where, prependArgs(args, game, patch)...)
	if err != nil {
		return nil, wrapStorage("query items", err)
	}
	defer rows.Close()
	var out []knowledge.Item
	for rows.Next() {
		var it knowledge.Item
		var data string
		if err := rows.Scan(&it.Patch, &it.ID, &it.Name, &data); err != nil {
			return nil, wrapStorage("scan item", err)
		}
		it.Data = json.RawMessage(data)
		out = append(out, it)
	}
	return out, wrapIfErr(rows.Err())
}

// QueryTrait 查询羁绊（版本域）
func (s *SQLiteStore) QueryTrait(ctx context.Context, game, patch string, q knowledge.Query) ([]knowledge.Trait, errors.AppError) {
	where, args := buildWhere(q, "standard")
	rows, err := s.db.QueryContext(ctx,
		"SELECT patch, id, name, data FROM traits WHERE "+where, prependArgs(args, game, patch)...)
	if err != nil {
		return nil, wrapStorage("query traits", err)
	}
	defer rows.Close()
	var out []knowledge.Trait
	for rows.Next() {
		var t knowledge.Trait
		var data string
		if err := rows.Scan(&t.Patch, &t.ID, &t.Name, &data); err != nil {
			return nil, wrapStorage("scan trait", err)
		}
		t.Data = json.RawMessage(data)
		out = append(out, t)
	}
	return out, wrapIfErr(rows.Err())
}

// QueryAugment 查询强化符文（版本域）
func (s *SQLiteStore) QueryAugment(ctx context.Context, game, patch string, q knowledge.Query) ([]knowledge.Augment, errors.AppError) {
	where, args := buildWhere(q, "standard")
	rows, err := s.db.QueryContext(ctx,
		"SELECT patch, id, name, COALESCE(description, ''), data FROM augments WHERE "+where, prependArgs(args, game, patch)...)
	if err != nil {
		return nil, wrapStorage("query augments", err)
	}
	defer rows.Close()
	var out []knowledge.Augment
	for rows.Next() {
		var a knowledge.Augment
		var data string
		if err := rows.Scan(&a.Patch, &a.ID, &a.Name, &a.Description, &data); err != nil {
			return nil, wrapStorage("scan augment", err)
		}
		a.Data = json.RawMessage(data)
		out = append(out, a)
	}
	return out, wrapIfErr(rows.Err())
}

// QueryComposition 查询阵容（版本域）
func (s *SQLiteStore) QueryComposition(ctx context.Context, game, patch string, q knowledge.Query) ([]knowledge.Composition, errors.AppError) {
	where, args := buildWhere(q, "standard")
	rows, err := s.db.QueryContext(ctx,
		"SELECT patch, id, name, data FROM compositions WHERE "+where, prependArgs(args, game, patch)...)
	if err != nil {
		return nil, wrapStorage("query compositions", err)
	}
	defer rows.Close()
	var out []knowledge.Composition
	for rows.Next() {
		var c knowledge.Composition
		var data string
		if err := rows.Scan(&c.Patch, &c.ID, &c.Name, &data); err != nil {
			return nil, wrapStorage("scan composition", err)
		}
		c.Data = json.RawMessage(data)
		out = append(out, c)
	}
	return out, wrapIfErr(rows.Err())
}

func prependArgs(args []interface{}, game, patch string) []interface{} {
	// buildWhere already includes mode in the args, so we just prepend game and patch
	return append([]interface{}{game, patch}, args...)
}

// LatestPatch 最新已下载版本（按版本号字符串倒序）
func (s *SQLiteStore) LatestPatch(ctx context.Context) (string, bool, errors.AppError) {
	var patch string
	err := s.db.QueryRowContext(ctx,
		"SELECT patch FROM patches ORDER BY patch DESC LIMIT 1").Scan(&patch)
	if err == sql.ErrNoRows {
		return "", false, nil
	}
	if err != nil {
		return "", false, wrapStorage("latest patch", err)
	}
	return patch, true, nil
}

// PatchExists 指定版本是否已下载
func (s *SQLiteStore) PatchExists(ctx context.Context, game, patch string) (bool, errors.AppError) {
	var one int
	err := s.db.QueryRowContext(ctx,
		"SELECT 1 FROM patches WHERE game=? AND patch=?", game, patch).Scan(&one)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, wrapStorage("patch exists", err)
	}
	return true, nil
}

// Status 汇总状态
func (s *SQLiteStore) Status(ctx context.Context) (*knowledge.Status, errors.AppError) {
	st := &knowledge.Status{Counts: map[string]int{}}

	rows, err := s.db.QueryContext(ctx,
		"SELECT patch, downloaded_at, source FROM patches ORDER BY patch DESC")
	if err != nil {
		return nil, wrapStorage("status patches", err)
	}
	defer rows.Close()
	for rows.Next() {
		var p knowledge.PatchInfo
		if err := rows.Scan(&p.Patch, &p.DownloadedAt, &p.Source); err != nil {
			return nil, wrapStorage("scan patch info", err)
		}
		st.Patches = append(st.Patches, p)
	}
	if err := rows.Err(); err != nil {
		return nil, wrapStorage("status patches rows", err)
	}

	for table, key := range map[string]string{
		"champions": "champions", "items": "items", "traits": "traits", "compositions": "compositions", "augments": "augments",
	} {
		_ = table
		var n int
		if err := s.db.QueryRowContext(ctx,
			fmt.Sprintf("SELECT COUNT(*) FROM %s WHERE game='tft'", key)).Scan(&n); err != nil {
			return nil, wrapStorage("count "+key, err)
		}
		st.Counts[key] = n
	}
	return st, nil
}

// wrapIfErr rows.Err() 为 nil 时返回 nil 结果
func wrapIfErr(err error) errors.AppError {
	if err == nil {
		return nil
	}
	return errors.Wrap(errors.CodeStorageError, "knowledge store: rows: "+err.Error(), err)
}
