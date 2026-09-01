// Package tft TFT 特定知识库提供者
//
// 遵循分层设计：
//
//	GameKnowledgeProvider
//	        ↓
//	TFTKnowledgeProvider (本包)
//	        ↓
//	TFTRepository (本包)
//	        ↓
//	SQLite (storage 层)
//
// 所有查询必须明确指定 patch/version，不同版本数据不混用。
package tft

import (
	"context"

	"github.com/lipipi/aurora/internal/errors"
	"github.com/lipipi/aurora/internal/game/knowledge"
	"github.com/lipipi/aurora/internal/storage"
)

// TFTKnowledgeProvider TFT 知识库提供者接口
// 定义 TFT 特有的结构化知识查询方法，所有查询强制指定 patch
type TFTKnowledgeProvider interface {
	// GetChampion 根据名称或key获取英雄信息（必须指定 patch）
	GetChampion(ctx context.Context, patch, name string) (*knowledge.Champion, errors.AppError)
	// GetItem 根据名称或key获取装备信息（必须指定 patch）
	GetItem(ctx context.Context, patch, name string) (*knowledge.Item, errors.AppError)
	// GetTrait 根据名称或key获取羁绊信息（必须指定 patch）
	GetTrait(ctx context.Context, patch, name string) (*knowledge.Trait, errors.AppError)
	// GetPatch 获取补丁版本信息
	GetPatch(ctx context.Context, patch string) (*PatchInfo, errors.AppError)
	// ListChampions 列出指定 patch 的所有英雄
	ListChampions(ctx context.Context, patch string) ([]knowledge.Champion, errors.AppError)
	// ListItems 列出指定 patch 的所有装备
	ListItems(ctx context.Context, patch string) ([]knowledge.Item, errors.AppError)
	// ListTraits 列出指定 patch 的所有羁绊
	ListTraits(ctx context.Context, patch string) ([]knowledge.Trait, errors.AppError)
	// HasPatch 检查指定 patch 是否已导入
	HasPatch(ctx context.Context, patch string) (bool, errors.AppError)
}

// PatchInfo 补丁版本信息
type PatchInfo struct {
	Version     string `json:"version"`
	ReleaseDate int64  `json:"release_date"`
	Notes       string `json:"notes,omitempty"`
}

// TFTRepository TFT 知识数据访问层接口
// 直接操作 SQLite 表，实现结构化数据查询
type TFTRepository interface {
	// GetChampion 根据 patch 和 key/name 查询英雄
	GetChampion(ctx context.Context, patch, query string) (*knowledge.Champion, errors.AppError)
	// GetItem 根据 patch 和 key/name 查询装备
	GetItem(ctx context.Context, patch, query string) (*knowledge.Item, errors.AppError)
	// GetTrait 根据 patch 和 key/name 查询羁绊
	GetTrait(ctx context.Context, patch, query string) (*knowledge.Trait, errors.AppError)
	// GetPatch 获取补丁信息
	GetPatch(ctx context.Context, patch string) (*PatchInfo, errors.AppError)
	// ListChampions 列出指定 patch 所有英雄
	ListChampions(ctx context.Context, patch string) ([]knowledge.Champion, errors.AppError)
	// ListItems 列出指定 patch 所有装备
	ListItems(ctx context.Context, patch string) ([]knowledge.Item, errors.AppError)
	// ListTraits 列出指定 patch 所有羁绊
	ListTraits(ctx context.Context, patch string) ([]knowledge.Trait, errors.AppError)
	// HasPatch 检查补丁是否存在
	HasPatch(ctx context.Context, patch string) (bool, errors.AppError)
}

// tftKnowledgeProvider 实现 TFTKnowledgeProvider 接口
type tftKnowledgeProvider struct {
	repo TFTRepository
}

// New 创建 TFTKnowledgeProvider
func New(repo TFTRepository) TFTKnowledgeProvider {
	return &tftKnowledgeProvider{repo: repo}
}

// GetChampion 实现 TFTKnowledgeProvider
func (p *tftKnowledgeProvider) GetChampion(ctx context.Context, patch, name string) (*knowledge.Champion, errors.AppError) {
	return p.repo.GetChampion(ctx, patch, name)
}

// GetItem 实现 TFTKnowledgeProvider
func (p *tftKnowledgeProvider) GetItem(ctx context.Context, patch, name string) (*knowledge.Item, errors.AppError) {
	return p.repo.GetItem(ctx, patch, name)
}

// GetTrait 实现 TFTKnowledgeProvider
func (p *tftKnowledgeProvider) GetTrait(ctx context.Context, patch, name string) (*knowledge.Trait, errors.AppError) {
	return p.repo.GetTrait(ctx, patch, name)
}

// GetPatch 实现 TFTKnowledgeProvider
func (p *tftKnowledgeProvider) GetPatch(ctx context.Context, patch string) (*PatchInfo, errors.AppError) {
	return p.repo.GetPatch(ctx, patch)
}

// ListChampions 实现 TFTKnowledgeProvider
func (p *tftKnowledgeProvider) ListChampions(ctx context.Context, patch string) ([]knowledge.Champion, errors.AppError) {
	return p.repo.ListChampions(ctx, patch)
}

// ListItems 实现 TFTKnowledgeProvider
func (p *tftKnowledgeProvider) ListItems(ctx context.Context, patch string) ([]knowledge.Item, errors.AppError) {
	return p.repo.ListItems(ctx, patch)
}

// ListTraits 实现 TFTKnowledgeProvider
func (p *tftKnowledgeProvider) ListTraits(ctx context.Context, patch string) ([]knowledge.Trait, errors.AppError) {
	return p.repo.ListTraits(ctx, patch)
}

// HasPatch 实现 TFTKnowledgeProvider
func (p *tftKnowledgeProvider) HasPatch(ctx context.Context, patch string) (bool, errors.AppError) {
	return p.repo.HasPatch(ctx, patch)
}

// sqliteTFTRepository 实现 TFTRepository 接口，基于 SQLite 存储
type sqliteTFTRepository struct {
	db *storage.DB
}

// NewSQLiteRepository 创建 SQLite 底的 TFTRepository
func NewSQLiteRepository(db *storage.DB) TFTRepository {
	return &sqliteTFTRepository{db: db}
}
