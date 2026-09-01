package tft

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/lipipi/aurora/internal/errors"
	"github.com/lipipi/aurora/internal/game/knowledge"
)

// GetChampion 查询英雄（优先匹配 key，其次模糊匹配 name）
func (r *sqliteTFTRepository) GetChampion(ctx context.Context, patch, query string) (*knowledge.Champion, errors.AppError) {
	// First try exact match on key
	var c knowledge.Champion
	var dataStr string

	err := r.db.DB().QueryRowContext(ctx, `
		SELECT patches.version, champions.key, champions.name, champions.data
		FROM champions
		JOIN patches ON champions.patch_id = patches.id
		WHERE patches.version = ? AND champions.key = ?
	`, patch, query).Scan(&c.Patch, &c.ID, &c.Name, &dataStr)

	if err == sql.ErrNoRows {
		// Try fuzzy match on name
		err = r.db.DB().QueryRowContext(ctx, `
			SELECT patches.version, champions.key, champions.name, champions.data
			FROM champions
			JOIN patches ON champions.patch_id = patches.id
			WHERE patches.version = ? AND lower(champions.name) LIKE lower(?)
			LIMIT 1
		`, patch, "%"+query+"%").Scan(&c.Patch, &c.ID, &c.Name, &dataStr)
	}

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, errors.Wrap(errors.CodeStorageError, "query champion failed", err)
	}

	c.Data = json.RawMessage(dataStr)
	return &c, nil
}

// GetItem 查询装备（优先匹配 key，其次模糊匹配 name）
func (r *sqliteTFTRepository) GetItem(ctx context.Context, patch, query string) (*knowledge.Item, errors.AppError) {
	var it knowledge.Item
	var dataStr string

	err := r.db.DB().QueryRowContext(ctx, `
		SELECT patches.version, items.key, items.name, items.data
		FROM items
		JOIN patches ON items.patch_id = patches.id
		WHERE patches.version = ? AND items.key = ?
	`, patch, query).Scan(&it.Patch, &it.ID, &it.Name, &dataStr)

	if err == sql.ErrNoRows {
		err = r.db.DB().QueryRowContext(ctx, `
			SELECT patches.version, items.key, items.name, items.data
			FROM items
			JOIN patches ON items.patch_id = patches.id
			WHERE patches.version = ? AND lower(items.name) LIKE lower(?)
			LIMIT 1
		`, patch, "%"+query+"%").Scan(&it.Patch, &it.ID, &it.Name, &dataStr)
	}

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, errors.Wrap(errors.CodeStorageError, "query item failed", err)
	}

	it.Data = json.RawMessage(dataStr)
	return &it, nil
}

// GetTrait 查询羁绊（优先匹配 key，其次模糊匹配 name）
func (r *sqliteTFTRepository) GetTrait(ctx context.Context, patch, query string) (*knowledge.Trait, errors.AppError) {
	var t knowledge.Trait
	var dataStr string

	err := r.db.DB().QueryRowContext(ctx, `
		SELECT patches.version, traits.key, traits.name, traits.data
		FROM traits
		JOIN patches ON traits.patch_id = patches.id
		WHERE patches.version = ? AND traits.key = ?
	`, patch, query).Scan(&t.Patch, &t.ID, &t.Name, &dataStr)

	if err == sql.ErrNoRows {
		err = r.db.DB().QueryRowContext(ctx, `
			SELECT patches.version, traits.key, traits.name, traits.data
			FROM traits
			JOIN patches ON traits.patch_id = patches.id
			WHERE patches.version = ? AND lower(traits.name) LIKE lower(?)
			LIMIT 1
		`, patch, "%"+query+"%").Scan(&t.Patch, &t.ID, &t.Name, &dataStr)
	}

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, errors.Wrap(errors.CodeStorageError, "query trait failed", err)
	}

	t.Data = json.RawMessage(dataStr)
	return &t, nil
}

// GetPatch 获取补丁信息
func (r *sqliteTFTRepository) GetPatch(ctx context.Context, patch string) (*PatchInfo, errors.AppError) {
	var pi PatchInfo
	err := r.db.DB().QueryRowContext(ctx, `
		SELECT version, release_date, notes FROM patches WHERE version = ?
	`, patch).Scan(&pi.Version, &pi.ReleaseDate, &pi.Notes)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, errors.Wrap(errors.CodeStorageError, "query patch failed", err)
	}

	return &pi, nil
}

// ListChampions 列出指定 patch 的所有英雄
func (r *sqliteTFTRepository) ListChampions(ctx context.Context, patch string) ([]knowledge.Champion, errors.AppError) {
	rows, err := r.db.DB().QueryContext(ctx, `
		SELECT patches.version, champions.key, champions.name, champions.data
		FROM champions
		JOIN patches ON champions.patch_id = patches.id
		WHERE patches.version = ?
		ORDER BY champions.cost DESC, champions.name ASC
	`, patch)
	if err != nil {
		return nil, errors.Wrap(errors.CodeStorageError, "list champions failed", err)
	}
	defer rows.Close()

	var result []knowledge.Champion
	for rows.Next() {
		var c knowledge.Champion
		var dataStr string
		if err := rows.Scan(&c.Patch, &c.ID, &c.Name, &dataStr); err != nil {
			return nil, errors.Wrap(errors.CodeStorageError, "scan champion failed", err)
		}
		c.Data = json.RawMessage(dataStr)
		result = append(result, c)
	}

	if err := rows.Err(); err != nil {
		return nil, errors.Wrap(errors.CodeStorageError, "iterate champions failed", err)
	}

	return result, nil
}

// ListItems 列出指定 patch 的所有装备
func (r *sqliteTFTRepository) ListItems(ctx context.Context, patch string) ([]knowledge.Item, errors.AppError) {
	rows, err := r.db.DB().QueryContext(ctx, `
		SELECT patches.version, items.key, items.name, items.data
		FROM items
		JOIN patches ON items.patch_id = patches.id
		WHERE patches.version = ?
		ORDER BY items.name ASC
	`, patch)
	if err != nil {
		return nil, errors.Wrap(errors.CodeStorageError, "list items failed", err)
	}
	defer rows.Close()

	var result []knowledge.Item
	for rows.Next() {
		var it knowledge.Item
		var dataStr string
		if err := rows.Scan(&it.Patch, &it.ID, &it.Name, &dataStr); err != nil {
			return nil, errors.Wrap(errors.CodeStorageError, "scan item failed", err)
		}
		it.Data = json.RawMessage(dataStr)
		result = append(result, it)
	}

	if err := rows.Err(); err != nil {
		return nil, errors.Wrap(errors.CodeStorageError, "iterate items failed", err)
	}

	return result, nil
}

// ListTraits 列出指定 patch 的所有羁绊
func (r *sqliteTFTRepository) ListTraits(ctx context.Context, patch string) ([]knowledge.Trait, errors.AppError) {
	rows, err := r.db.DB().QueryContext(ctx, `
		SELECT patches.version, traits.key, traits.name, traits.data
		FROM traits
		JOIN patches ON traits.patch_id = patches.id
		WHERE patches.version = ?
		ORDER BY traits.name ASC
	`, patch)
	if err != nil {
		return nil, errors.Wrap(errors.CodeStorageError, "list traits failed", err)
	}
	defer rows.Close()

	var result []knowledge.Trait
	for rows.Next() {
		var t knowledge.Trait
		var dataStr string
		if err := rows.Scan(&t.Patch, &t.ID, &t.Name, &dataStr); err != nil {
			return nil, errors.Wrap(errors.CodeStorageError, "scan trait failed", err)
		}
		t.Data = json.RawMessage(dataStr)
		result = append(result, t)
	}

	if err := rows.Err(); err != nil {
		return nil, errors.Wrap(errors.CodeStorageError, "iterate traits failed", err)
	}

	return result, nil
}

// HasPatch 检查补丁是否存在
func (r *sqliteTFTRepository) HasPatch(ctx context.Context, patch string) (bool, errors.AppError) {
	var exists int
	err := r.db.DB().QueryRowContext(ctx, `
		SELECT 1 FROM patches WHERE version = ?
	`, patch).Scan(&exists)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, errors.Wrap(errors.CodeStorageError, "check patch exists failed", err)
	}
	return true, nil
}
