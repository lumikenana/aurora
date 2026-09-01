// Package preparation - TFT 备战理解层（Phase 16-B）
//
// 本包实现 "GroundedGameContext → PreparationAnalysis" 的确定性本地分析：
//   - 备战席（Bench）英雄统计
//   - 商店（Shop）英雄统计
//   - 棋盘+备战+商店 重复英雄计数（用于判断升星距离）
//
// 严格遵守不变量：
//   - 0 LLM 调用：纯本地计数，不做决策
//   - 不实现买/卖/刷决策（该不该拿牌 → Phase 17）
//   - 不修改 GroundedGameContext（只读消费）
//
// 不实现：
//   - "这个英雄该拿吗"
//   - "该卖哪个备战英雄"
//   - "该刷新商店吗"
package preparation

import (
	"sort"
	"sync"
	"time"

	"github.com/lipipi/aurora/internal/game/plugins/tft/grounder"
	"github.com/lipipi/aurora/internal/log"
)

// PreparationFingerprint 备战缓存 key
//
// 仅包含"影响备战分析结论"的字段：
//   - Bench 所有英雄 (ChampionID + Star)
//   - Shop 所有英雄 (ChampionID)
//   - Patch / SetKey（KB 版本变化 → 重算）
//
// 不包含：
//   - Board 装备 / 经济数据（不影响备战计数）
//   - Gold / Interest / XP（不影响备战计数）
type PreparationFingerprint struct {
	Patch      string                `json:"patch"`
	SetKey     string                `json:"set_key"`
	BenchChampions []BenchChampionRef `json:"bench_champions"`
	ShopChampions  []ShopChampionRef  `json:"shop_champions"`
}

// BenchChampionRef 备战席英雄引用
type BenchChampionRef struct {
	ChampionID string `json:"champion_id"`
	Star       int    `json:"star"`
}

// ShopChampionRef 商店英雄引用
type ShopChampionRef struct {
	ChampionID string `json:"champion_id"`
	Cost       int    `json:"cost"`
}

// ChampionDuplicateCount 英雄全局重复计数
type ChampionDuplicateCount struct {
	// ChampionID
	ChampionID string `json:"champion_id"`
	// Name 来自 KB
	Name string `json:"name,omitempty"`
	// Cost 来自 KB
	Cost int `json:"cost,omitempty"`
	// BoardCount 棋盘上数量（含已有星级分解：2星 = 2个，3星 = 3个）
	BoardCount int `json:"board_count"`
	// BenchCount 备战席数量（分解后）
	BenchCount int `json:"bench_count"`
	// ShopCount 商店中数量
	ShopCount int `json:"shop_count"`
	// TotalCount 全局总数量
	TotalCount int `json:"total_count"`
	// NextStar 下一个星级需要多少数量（0 = 已3星）
	NextStar int `json:"next_star"`
	// NeededForNextStar 距离下一个星级还需要多少个
	NeededForNextStar int `json:"needed_for_next_star"`
	// CanUpgradeTo2 当前可以升2星吗
	CanUpgradeTo2 bool `json:"can_upgrade_to_2"`
	// CanUpgradeTo3 当前可以升3星吗
	CanUpgradeTo3 bool `json:"can_upgrade_to_3"`
}

// PreparationAnalysis PreparationAnalyzer 输出
type PreparationAnalysis struct {
	// Fingerprint 缓存 key
	Fingerprint PreparationFingerprint `json:"fingerprint"`
	// GeneratedAt
	GeneratedAt time.Time `json:"generated_at"`
	// Patch / SetKey
	Patch  string `json:"patch"`
	SetKey string `json:"set_key"`

	// Bench 备战席英雄列表（按 cost 排序？不，保持原始顺序）
	Bench []BenchChampion `json:"bench"`
	// Shop 商店英雄列表（保持原始顺序）
	Shop []ShopChampion `json:"shop"`

	// DuplicateCounts 所有重复英雄计数
	DuplicateCounts []ChampionDuplicateCount `json:"duplicate_counts"`

	// Summary 汇总统计
	Summary PreparationSummary `json:"summary"`
}

// BenchChampion 备战席完整信息
type BenchChampion struct {
	ChampionID string `json:"champion_id"`
	Name       string `json:"name,omitempty"`
	Star       int    `json:"star"`
	Cost       int    `json:"cost,omitempty"`
}

// ShopChampion 商店完整信息
type ShopChampion struct {
	ChampionID string `json:"champion_id"`
	Name       string `json:"name,omitempty"`
	Cost       int    `json:"cost"`
}

// PreparationSummary 备战汇总
type PreparationSummary struct {
	// BenchSlotsUsed 备战席已占用槽位
	BenchSlotsUsed int `json:"bench_slots_used"`
	// BenchSlotsAvailable 备战席可用槽位（默认 9）
	BenchSlotsAvailable int `json:"bench_slots_available"`
	// ShopSlotsUsed 商店已占用槽位
	ShopSlotsUsed int `json:"shop_slots_used"`
	// TotalDuplicates 全局重复英雄总数（有多个相同的）
	TotalDuplicates int `json:"total_duplicates"`
	// Upgradable 可升级英雄数量
	Upgradable int `json:"upgradable"`
}

// Analyzer 备战分析器（带 Fingerprint 缓存）
type Analyzer struct {
	mu    sync.RWMutex
	cache *PreparationAnalysis
}

// NewAnalyzer 构造 Analyzer
func NewAnalyzer() *Analyzer {
	return &Analyzer{}
}

// Analyze 输入 GroundedGameContext，输出 PreparationAnalysis
//
// 行为约定：
//   - g == nil → 返回 zero-value PreparationAnalysis
//   - 缓存命中（Fingerprint.Equal）→ 直接返回缓存
//   - 0 LLM 调用（纯本地计数）
//   - 不做决策，只做结构化汇总
func (a *Analyzer) Analyze(g grounder.GroundedGameContext) PreparationAnalysis {
	fp := ComputeFingerprint(g)

	// Fast path: cache hit
	a.mu.RLock()
	if a.cache != nil && a.cache.Fingerprint.Equal(fp) {
		out := *a.cache
		a.mu.RUnlock()
		log.Debugf("preparation: cache hit fp=%s", fp.Hash()[:8])
		return out
	}
	a.mu.RUnlock()

	// Slow path: rebuild
	out := a.analyzeOnce(g, fp)

	a.mu.Lock()
	a.cache = &out
	a.mu.Unlock()
	log.Debugf("preparation: cache miss fp=%s bench=%d shop=%d",
		fp.Hash()[:8], len(out.Bench), len(out.Shop))
	return out
}

// analyzeOnce 真正做分析（pure function，不加锁）
func (a *Analyzer) analyzeOnce(g grounder.GroundedGameContext, fp PreparationFingerprint) PreparationAnalysis {
	out := PreparationAnalysis{
		Fingerprint: fp,
		GeneratedAt: time.Now(),
		Patch:       g.Meta.Patch,
		SetKey:      g.Meta.SetKey,
		Bench:       make([]BenchChampion, 0, len(g.Bench)),
		Shop:        make([]ShopChampion, 0, len(g.Shop)),
	}

	// 1) Process bench
	countByID := map[string]int{}
	for _, b := range g.Bench {
		// 计数分解：星级 → 个数（已有 2星算 2个）
		countByID[b.Champion.ID] += b.Star
		out.Bench = append(out.Bench, BenchChampion{
			ChampionID: b.Champion.ID,
			Name:       b.Champion.Name,
			Star:       b.Star,
			Cost:       b.Champion.Cost,
		})
	}

	// 2) Process board（用于重复计数：棋盘上的也要算进去）
	for _, bd := range g.Board {
		countByID[bd.Champion.ID] += bd.Star
	}

	// 3) Process shop
	for _, s := range g.Shop {
		countByID[s.Champion.ID] += 1 // shop 每个都是 1星
		out.Shop = append(out.Shop, ShopChampion{
			ChampionID: s.Champion.ID,
			Name:       s.Champion.Name,
			Cost:       s.Cost,
		})
	}

	// 4) Build duplicate counts
	out.DuplicateCounts = make([]ChampionDuplicateCount, 0, len(countByID))

	// 需要从 board/bench/shop 提取完整信息（name/cost）
	// 先构建完整信息索引
	allChampions := buildChampionIndex(g.Board, g.Bench, g.Shop)

	for id, total := range countByID {
		info := allChampions[id]
		bc := calculateBreakdown(id, total, g.Board, g.Bench, g.Shop)
		dc := ChampionDuplicateCount{
			ChampionID: id,
			Name:       info.Name,
			Cost:       info.Cost,
			BoardCount: bc.board,
			BenchCount: bc.bench,
			ShopCount:  bc.shop,
			TotalCount: bc.total,
		}

		// 计算升星需求
		switch {
		case dc.TotalCount >= 9:
			// already 3-star (3*3)
			dc.NextStar = 0
			dc.NeededForNextStar = 0
		case dc.TotalCount >= 3:
			// can upgrade to 3-star: need 9 - total
			dc.NextStar = 3
			dc.NeededForNextStar = 9 - dc.TotalCount
			dc.CanUpgradeTo2 = true
			dc.CanUpgradeTo3 = dc.TotalCount >= 9
		default:
			// can upgrade to 2-star: need 3 - total
			dc.NextStar = 2
			dc.NeededForNextStar = 3 - dc.TotalCount
			dc.CanUpgradeTo2 = dc.TotalCount >= 3
			dc.CanUpgradeTo3 = false
		}
		out.DuplicateCounts = append(out.DuplicateCounts, dc)
	}

	// 排序：按 total desc 让重复多的在前
	sort.SliceStable(out.DuplicateCounts, func(i, j int) bool {
		return out.DuplicateCounts[i].TotalCount > out.DuplicateCounts[j].TotalCount
	})

	// 5) Build summary
	out.Summary = PreparationSummary{
		BenchSlotsUsed:     len(g.Bench),
		BenchSlotsAvailable: 9 - len(g.Bench), // TFT 标准备战席 9 个槽位
		ShopSlotsUsed:      len(g.Shop),
		TotalDuplicates:    len(out.DuplicateCounts),
	}
	for _, dc := range out.DuplicateCounts {
		if dc.CanUpgradeTo2 || dc.CanUpgradeTo3 {
			out.Summary.Upgradable++
		}
	}

	return out
}

type championInfo struct {
	Name string
	Cost int
}

func buildChampionIndex(
	board []grounder.GroundedBoardSlot,
	bench []grounder.GroundedBenchSlot,
	shop []grounder.GroundedShopSlot,
) map[string]championInfo {
	idx := map[string]championInfo{}
	for _, b := range board {
		idx[b.Champion.ID] = championInfo{
			Name: b.Champion.Name,
			Cost: b.Champion.Cost,
		}
	}
	for _, b := range bench {
		idx[b.Champion.ID] = championInfo{
			Name: b.Champion.Name,
			Cost: b.Champion.Cost,
		}
	}
	for _, s := range shop {
		idx[s.Champion.ID] = championInfo{
			Name: s.Champion.Name,
			Cost: s.Champion.Cost,
		}
	}
	return idx
}

type countBreakdown struct {
	board int
	bench int
	shop  int
	total int
}

func calculateBreakdown(
	id string,
	total int,
	board []grounder.GroundedBoardSlot,
	bench []grounder.GroundedBenchSlot,
	shop []grounder.GroundedShopSlot,
) countBreakdown {
	bc := countBreakdown{}
	// board 计数
	for _, b := range board {
		if b.Champion.ID == id {
			bc.board += b.Star
		}
	}
	// bench 计数
	for _, b := range bench {
		if b.Champion.ID == id {
			bc.bench += b.Star
		}
	}
	// shop 计数
	for _, s := range shop {
		if s.Champion.ID == id {
			bc.shop += 1
		}
	}
	bc.total = bc.board + bc.bench + bc.shop
	return bc
}

// Equal 判断两个 PreparationFingerprint 是否相等
func (f PreparationFingerprint) Equal(other PreparationFingerprint) bool {
	if f.Patch != other.Patch || f.SetKey != other.SetKey {
		return false
	}
	if len(f.BenchChampions) != len(other.BenchChampions) {
		return false
	}
	for i, b := range f.BenchChampions {
		if b.ChampionID != other.BenchChampions[i].ChampionID ||
			b.Star != other.BenchChampions[i].Star {
			return false
		}
	}
	if len(f.ShopChampions) != len(other.ShopChampions) {
		return false
	}
	for i, s := range f.ShopChampions {
		if s.ChampionID != other.ShopChampions[i].ChampionID ||
			s.Cost != other.ShopChampions[i].Cost {
			return false
		}
	}
	return true
}
