// Package knowledge 游戏知识库抽象层（ADR-022）
//
// 本包只含接口、实体类型和编排逻辑，不 import sqlite / net ——
// SQLite 实现在 knowledge/store 子包（仅 main.go import），
// CommunityDragon 数据源在 knowledge/cdsource 子包。
// 依赖方（agent / tool）只依赖本包接口，保持可交叉编译。
package knowledge

import (
	"context"
	"encoding/json"

	"github.com/lipipi/aurora/internal/errors"
)

// Champion 英雄（所有实体都带 Patch，查询强制限定 active patch，版本数据不混用）
type Champion struct {
	Patch string          `json:"patch"`
	ID    string          `json:"id"` // apiName，如 "TFT16_Jinx"
	Name  string          `json:"name"`
	Data  json.RawMessage `json:"data,omitempty"` // 完整源记录
}

// Item 装备
type Item struct {
	Patch string          `json:"patch"`
	ID    string          `json:"id"`
	Name  string          `json:"name"`
	Data  json.RawMessage `json:"data,omitempty"`
}

// Trait 羁绊
type Trait struct {
	Patch string          `json:"patch"`
	ID    string          `json:"id"`
	Name  string          `json:"name"`
	Data  json.RawMessage `json:"data,omitempty"`
}

// Composition 阵容（手动策展导入，无官方免费数据源）
type Composition struct {
	Patch string          `json:"patch"`
	ID    string          `json:"id"`
	Name  string          `json:"name"`
	Data  json.RawMessage `json:"data,omitempty"` // {tier, champions[], coreItems[], traits[], description, source}
}

// Augment 强化符文
type Augment struct {
	Patch       string          `json:"patch"`
	ID          string          `json:"id"`
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Data        json.RawMessage `json:"data,omitempty"`
}

// Query 查询条件；Name 为模糊匹配（大小写不敏感子串），ID 为精确 apiName
// 两者都为空时返回全部（上限 50 条）
type Query struct {
	Name string `json:"name,omitempty"`
	ID   string `json:"id,omitempty"`
}

// PatchInfo 已下载的版本信息
type PatchInfo struct {
	Patch        string `json:"patch"`
	DownloadedAt string `json:"downloaded_at"`
	Source       string `json:"source"`
}

// Status 知识库状态（供设置 UI 展示）
type Status struct {
	ActivePatch   string         `json:"active_patch"`
	PatchOverride string         `json:"patch_override"` // 配置的版本覆盖，空 = 无
	Patches       []PatchInfo    `json:"patches"`
	Counts        map[string]int `json:"counts"` // champions/items/traits/compositions
	HasData       bool           `json:"has_data"`
}

// ImportSummary 一次导入的结果
type ImportSummary struct {
	Patch        string `json:"patch"`
	Champions    int    `json:"champions"`
	Items        int    `json:"items"`
	Traits       int    `json:"traits"`
	Augments     int    `json:"augments"`
	Compositions int    `json:"compositions"`
}

// GameKnowledgeProvider 游戏知识库对外接口
// 所有 Get* 查询强制限定 active patch（版本数据绝不混用）
type GameKnowledgeProvider interface {
	GetCurrentPatch(ctx context.Context) (string, errors.AppError)
	GetChampion(ctx context.Context, game string, q Query) ([]Champion, errors.AppError)
	GetItem(ctx context.Context, game string, q Query) ([]Item, errors.AppError)
	GetTrait(ctx context.Context, game string, q Query) ([]Trait, errors.AppError)
	GetAugment(ctx context.Context, game string, q Query) ([]Augment, errors.AppError)
	GetComposition(ctx context.Context, game string, q Query) ([]Composition, errors.AppError)
	HasData() bool
	Status(ctx context.Context) (*Status, errors.AppError)
}

// NormalizedPatchData 归一化的单版本数据（由 transform 从 CD 载荷或手动 envelope 生成）
type NormalizedPatchData struct {
	Game         string
	SetKey       string  // 赛季 key，e.g. "set13"
	Patch        string
	Mode         string  // 游戏模式，默认 "standard"
	Champions    []Record
	Items        []Record
	Traits       []Record
	Augments     []Record
	Compositions []Record
	Source       string
}

// Record 单条归一化记录
type Record struct {
	ID   string
	Name string
	Data json.RawMessage
}

// Store 持久化接口（由 knowledge/store 的 SQLiteStore 实现）
type Store interface {
	// ImportNormalized 原子导入：单事务内只替换该 (game, patch) 的数据，其他版本不动
	ImportNormalized(ctx context.Context, d NormalizedPatchData) (ImportSummary, errors.AppError)
	QueryChampion(ctx context.Context, game, patch string, q Query) ([]Champion, errors.AppError)
	QueryItem(ctx context.Context, game, patch string, q Query) ([]Item, errors.AppError)
	QueryTrait(ctx context.Context, game, patch string, q Query) ([]Trait, errors.AppError)
	QueryAugment(ctx context.Context, game, patch string, q Query) ([]Augment, errors.AppError)
	QueryComposition(ctx context.Context, game, patch string, q Query) ([]Composition, errors.AppError)
	LatestPatch(ctx context.Context) (string, bool, errors.AppError)
	PatchExists(ctx context.Context, game, patch string) (bool, errors.AppError)
	Status(ctx context.Context) (*Status, errors.AppError)
	Close() error
}

// DataSource 在线数据源接口（由 knowledge/cdsource 实现）
type DataSource interface {
	Name() string
	// LatestPatch 返回最新可用版本 pin（如 "16.17"）
	LatestPatch(ctx context.Context) (string, errors.AppError)
	// FetchTFT 下载指定版本的 TFT 数据 JSON
	FetchTFT(ctx context.Context, patch, locale string, progress ProgressFunc) ([]byte, errors.AppError)
}

// ProgressFunc 进度回调（percent < 0 表示不确定进度）
type ProgressFunc func(percent int, message string)
