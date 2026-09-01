// Package knowledge Service 编排：active patch 解析 + 下载 + 导入
package knowledge

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/lipipi/aurora/internal/errors"
	"github.com/lipipi/aurora/internal/game"
	"github.com/lipipi/aurora/internal/log"
)

// Service 知识库服务，实现 GameKnowledgeProvider
type Service struct {
	store  Store
	source DataSource
	cfg    game.KnowledgeConfig
	mu     sync.Mutex // 下载单飞
	// rawDir 原始 JSON 缓存目录（appData/knowledge/raw）
	rawDir string
}

// NewService 创建知识库服务
func NewService(store Store, source DataSource, cfg game.KnowledgeConfig, rawDir string) *Service {
	return &Service{store: store, source: source, cfg: cfg, rawDir: rawDir}
}

// SetConfig 热更新配置（设置界面保存后调用）
func (s *Service) SetConfig(cfg game.KnowledgeConfig) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cfg = cfg
}

// resolveActivePatch 解析当前生效版本：配置覆盖（存在才生效）→ 最新已下载 → 空
func (s *Service) resolveActivePatch(ctx context.Context) (string, bool) {
	if s.cfg.Patch != "" {
		exists, err := s.store.PatchExists(ctx, "tft", s.cfg.Patch)
		if err == nil && exists {
			return s.cfg.Patch, true
		}
		if err != nil {
			log.Warnf("knowledge: patch exists check failed: %v", err)
		} else {
			log.Warnf("knowledge: patch override %q not downloaded, falling back to latest", s.cfg.Patch)
		}
	}
	patch, ok, err := s.store.LatestPatch(ctx)
	if err != nil {
		log.Warnf("knowledge: latest patch query failed: %v", err)
		return "", false
	}
	return patch, ok
}

// GetCurrentPatch 当前生效版本
func (s *Service) GetCurrentPatch(ctx context.Context) (string, errors.AppError) {
	patch, ok := s.resolveActivePatch(ctx)
	if !ok {
		return "", errors.New(errors.CodeStorageError, "knowledge base is empty")
	}
	return patch, nil
}

// GetChampion 查询英雄（限定 active patch）
func (s *Service) GetChampion(ctx context.Context, gameID string, q Query) ([]Champion, errors.AppError) {
	return queryScoped(ctx, s, gameID, q, s.store.QueryChampion)
}

// GetItem 查询装备（限定 active patch）
func (s *Service) GetItem(ctx context.Context, gameID string, q Query) ([]Item, errors.AppError) {
	return queryScoped(ctx, s, gameID, q, s.store.QueryItem)
}

// GetTrait 查询羁绊（限定 active patch）
func (s *Service) GetTrait(ctx context.Context, gameID string, q Query) ([]Trait, errors.AppError) {
	return queryScoped(ctx, s, gameID, q, s.store.QueryTrait)
}

// GetAugment 查询强化符文（限定 active patch）
func (s *Service) GetAugment(ctx context.Context, gameID string, q Query) ([]Augment, errors.AppError) {
	return queryScoped(ctx, s, gameID, q, s.store.QueryAugment)
}

// GetComposition 查询阵容（限定 active patch）
func (s *Service) GetComposition(ctx context.Context, gameID string, q Query) ([]Composition, errors.AppError) {
	return queryScoped(ctx, s, gameID, q, s.store.QueryComposition)
}

// queryScoped 统一查询入口：先解析 active patch，再委托 store（泛型函数，go1.26 方法不支持泛型）
func queryScoped[T any](ctx context.Context, s *Service, gameID string, q Query,
	fn func(ctx context.Context, game, patch string, q Query) ([]T, errors.AppError)) ([]T, errors.AppError) {
	patch, ok := s.resolveActivePatch(ctx)
	if !ok {
		return nil, errors.New(errors.CodeStorageError, "knowledge base is empty, please download data first")
	}
	return fn(ctx, gameID, patch, q)
}

// HasData 是否有可用数据
func (s *Service) HasData() bool {
	_, ok := s.resolveActivePatch(context.Background())
	return ok
}

// Status 知识库状态
func (s *Service) Status(ctx context.Context) (*Status, errors.AppError) {
	st, err := s.store.Status(ctx)
	if err != nil {
		return nil, err
	}
	st.PatchOverride = s.cfg.Patch
	patch, ok := s.resolveActivePatch(ctx)
	st.ActivePatch = patch
	st.HasData = ok
	return st, nil
}

// Import 导入归一化数据（手动导入 / 下载共用）
func (s *Service) Import(ctx context.Context, d *NormalizedPatchData) (ImportSummary, errors.AppError) {
	return s.store.ImportNormalized(ctx, *d)
}

// DownloadLatest 下载最新版本数据并导入（单飞）
func (s *Service) DownloadLatest(ctx context.Context, progress ProgressFunc) (ImportSummary, errors.AppError) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.source == nil {
		return ImportSummary{}, errors.New(errors.CodeInvalidConfig, "no knowledge data source configured")
	}

	report := func(p int, msg string) {
		if progress != nil {
			progress(p, msg)
		}
		log.Debugf("knowledge download: %d%% %s", p, msg)
	}

	report(0, "获取最新版本号")
	patch, err := s.source.LatestPatch(ctx)
	if err != nil {
		return ImportSummary{}, err
	}

	// raw 缓存命中则跳过下载
	rawPath := filepath.Join(s.rawDir, "tft-"+patch+"-"+s.cfg.Locale+".json")
	var raw []byte
	cached, readErr := os.ReadFile(rawPath)
	if readErr == nil && len(cached) > 0 {
		raw = cached
		report(50, "使用本地缓存 "+patch)
	} else {
		report(10, "下载 "+patch+" 数据")
		raw, err = s.source.FetchTFT(ctx, patch, s.cfg.Locale, func(p int, msg string) {
			// 下载进度映射到 10-90 区间
			if p < 0 {
				report(-1, msg)
			} else {
				report(10+p*80/100, msg)
			}
		})
		if err != nil {
			return ImportSummary{}, err
		}
		if s.rawDir != "" {
			_ = os.MkdirAll(s.rawDir, 0755)
			_ = os.WriteFile(rawPath, raw, 0644)
		}
	}

	report(90, "解析数据")
	d, perr := ParseCDPayload(raw, "tft", patch)
	if perr != nil {
		return ImportSummary{}, errors.New(errors.CodeStorageError, "parse failed: "+perr.Error())
	}

	report(95, "写入数据库")
	summary, err := s.store.ImportNormalized(ctx, *d)
	if err != nil {
		return summary, err
	}
	report(100, "完成 "+patch)
	log.Debugf("knowledge: imported patch %s: %d champions, %d items, %d traits",
		patch, summary.Champions, summary.Items, summary.Traits)
	return summary, nil
}

// AutoDownloadIfNeeded 启动时后台调用：无数据或最新版本未下载时自动下载
func (s *Service) AutoDownloadIfNeeded(ctx context.Context, progress ProgressFunc) (ImportSummary, bool, errors.AppError) {
	if !s.cfg.Enabled || !s.cfg.AutoDownloadOnStart || s.source == nil {
		return ImportSummary{}, false, nil
	}

	latest, err := s.source.LatestPatch(ctx)
	if err != nil {
		return ImportSummary{}, false, err
	}
	exists, err := s.store.PatchExists(ctx, "tft", latest)
	if err == nil && exists {
		return ImportSummary{}, false, nil // 已是最新
	}
	log.Infof("knowledge: auto downloading patch %s", latest)
	summary, err := s.DownloadLatest(ctx, progress)
	return summary, true, err
}

// Now 便于测试的时间函数（避免直接依赖 time.Now 难以注入）
var Now = time.Now
