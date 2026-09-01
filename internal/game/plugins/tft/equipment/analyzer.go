// Package equipment - TFT 装备分析层（Phase 16-B）
//
// 本包实现 "GroundedGameContext → EquipmentAnalysis" 的确定性本地分析：
//   - Champion → Items（每个棋盘英雄当前装备）
//   - Item → Champion（每个装备被谁持有）
//   - Item 数量统计（同名装备聚合）
//   - Item 类型识别（Completed/Component/Unknown）—— 通过 KB ResolvedItem.From 字段
//   - 与 Core Candidate 关联（复用 Phase 16-A）
//
// 严格遵守不变量：
//   - 0 LLM 调用：纯本地计算（KB 查询发生在上游 Grounder.Resolve）
//   - 0 硬编码 TFT 规则：item 类型（completed vs component）由 KB 决定（From 字段）
//   - 不实现装备推荐 / 装备合成 / 装备换装（Phase 17）
//   - 不修改 GroundedGameContext（只读消费）
//
// 不实现：
//   - "装备应该给谁"
//   - "装备应该换下来"
//   - "装备合成建议"
//   - "最佳装备组合"
package equipment

import (
	"sort"
	"sync"
	"time"

	"github.com/lipipi/aurora/internal/game/plugins/tft/composition"
	"github.com/lipipi/aurora/internal/game/plugins/tft/grounder"
	"github.com/lipipi/aurora/internal/log"
)

// ItemType 装备类型（基于 KB From 字段判定；不可识别时为 Unknown）
type ItemType string

const (
	// ItemTypeCompleted 完整装备（From 字段有 ≥1 个组件）
	ItemTypeCompleted ItemType = "completed"
	// ItemTypeComponent 基础散件（From 字段为空或 nil）
	ItemTypeComponent ItemType = "component"
	// ItemTypeUnknown KB 中找不到（Unknown / KBUnavailable / WrongSet）
	ItemTypeUnknown ItemType = "unknown"
)

// ItemOwnership 单个装备的全局归属
type ItemOwnership struct {
	// ItemID canonical ItemID（如 "TFT18_InfinityEdge"）
	ItemID string `json:"item_id"`
	// Name 来自 KB（可能为空：Unknown / KBUnavailable）
	Name string `json:"name,omitempty"`
	// Type 装备类型
	Type ItemType `json:"type"`
	// From 组件 ID 列表（仅 Completed 时有意义；KB ResolvedItem.From 字段）
	From []string `json:"from,omitempty"`
	// TotalCount 全局数量（棋盘装备 + Items 列表的同名聚合）
	TotalCount int `json:"total_count"`
	// BoardCount 棋盘上数量（BoardSlots[].Items 聚合）
	BoardCount int `json:"board_count"`
	// PocketCount 物品栏数量（来自 tctx.Items 散件）
	PocketCount int `json:"pocket_count"`
	// Holders 持有者列表（ChampionID + 位置 + 星级）
	Holders []ItemHolder `json:"holders,omitempty"`
}

// ItemHolder 装备的持有者
type ItemHolder struct {
	// ChampionID 持有英雄的 ID
	ChampionID string `json:"champion_id"`
	// Name 持有英雄的名称
	Name string `json:"name,omitempty"`
	// Position 棋盘位置
	Position int `json:"position"`
	// Star 星级
	Star int `json:"star"`
}

// ChampionEquipment 棋盘上每个英雄的装备
type ChampionEquipment struct {
	// ChampionID
	ChampionID string `json:"champion_id"`
	// Name 来自 KB
	Name string `json:"name,omitempty"`
	// Star
	Star int `json:"star"`
	// Cost
	Cost int `json:"cost,omitempty"`
	// Position
	Position int `json:"position"`
	// Items 装备列表（ID + 名称 + 类型）
	Items []ItemRef `json:"items"`
	// ItemCount 装备数量
	ItemCount int `json:"item_count"`
	// IsCore 是否为 Core Candidate（Likely 或 Candidate）
	IsCore bool `json:"is_core"`
	// CoreScore Core Candidate 评分（0 = 不是 Core）
	CoreScore float64 `json:"core_score,omitempty"`
	// CoreStatus Core Candidate 状态
	CoreStatus composition.CoreCandidateStatus `json:"core_status,omitempty"`
}

// ItemRef 装备引用（轻量）
type ItemRef struct {
	// ItemID
	ItemID string `json:"item_id"`
	// Name 来自 KB
	Name string `json:"name,omitempty"`
	// Type
	Type ItemType `json:"type"`
	// Status 来自 grounder.ResolvedItem.Status
	Status grounder.MatchStatus `json:"status"`
}

// CoreEquipmentLink 核心英雄的装备聚焦（Format 时单独输出）
type CoreEquipmentLink struct {
	// ChampionID
	ChampionID string `json:"champion_id"`
	// Name
	Name string `json:"name,omitempty"`
	// Star / Cost
	Star int `json:"star"`
	Cost int `json:"cost,omitempty"`
	// Items
	Items []ItemRef `json:"items"`
	// CoreStatus
	CoreStatus composition.CoreCandidateStatus `json:"core_status"`
	// CoreReason
	CoreReason string `json:"core_reason,omitempty"`
}

// EquipmentSummary 装备全局统计
type EquipmentSummary struct {
	// TotalItems 棋盘装备总数
	TotalItems int `json:"total_items"`
	// UniqueItems 不同装备 ID 数
	UniqueItems int `json:"unique_items"`
	// CompletedCount 完整装备数
	CompletedCount int `json:"completed_count"`
	// ComponentCount 散件数
	ComponentCount int `json:"component_count"`
	// UnknownCount 未知装备数
	UnknownCount int `json:"unknown_count"`
	// CoreChampionsWithFullItems 装备齐全的 Core 数（≥3 件）
	CoreChampionsWithFullItems int `json:"core_champions_with_full_items"`
	// ItemKBStatus "ok" / "partial" / "unavailable"
	ItemKBStatus string `json:"item_kb_status"`
}

// EquipmentFingerprint 装备缓存 key
//
// 仅包含"影响装备分析结论"的字段：
//   - Board 槽位的 ItemIDs（含持有者位置 + 星级 + ChampionID）
//   - Items 列表的 ItemID + Count（散件口袋）
//
// 不包含：
//   - Gold / Interest / XP / Stage / HP（这些与装备分析无关）
//   - Bench / Shop（Phase 16-B 单独处理）
//   - Patch / SetKey 是 KB 版本键，patch 变化 → 重算
type EquipmentFingerprint struct {
	// Patch 当前生效 patch
	Patch string `json:"patch"`
	// SetKey 推断自 patch
	SetKey string `json:"set_key"`

	// BoardItemSlots 棋盘每个槽位的 (ChampionID, Position, Star) → 排序后的 itemIDs
	BoardItemSlots []BoardItemSlot `json:"board_item_slots"`
	// PocketItems tctx.Items 散件（ItemID, Count）
	PocketItems []PocketItem `json:"pocket_items"`
}

// BoardItemSlot 棋盘一个槽位 + 装备
type BoardItemSlot struct {
	ChampionID string   `json:"champion_id"`
	Position   int      `json:"position"`
	Star       int      `json:"star"`
	ItemIDs    []string `json:"item_ids"`
}

// PocketItem 散件口袋
type PocketItem struct {
	ItemID string `json:"item_id"`
	Count  int    `json:"count"`
}

// EquipmentAnalysis EquipmentAnalyzer 输出
type EquipmentAnalysis struct {
	// Fingerprint 缓存 key
	Fingerprint EquipmentFingerprint `json:"fingerprint"`
	// GeneratedAt
	GeneratedAt time.Time `json:"generated_at"`
	// Patch / SetKey
	Patch  string `json:"patch"`
	SetKey string `json:"set_key"`

	// Champions 棋盘英雄 + 装备（按 Position 排序）
	Champions []ChampionEquipment `json:"champions"`
	// ItemIndex 装备 ID → 归属（全局）
	ItemIndex map[string]ItemOwnership `json:"item_index"`
	// CoreEquipment 核心英雄的装备（仅 Likely + Candidate）
	CoreEquipment []CoreEquipmentLink `json:"core_equipment,omitempty"`
	// Summary
	Summary EquipmentSummary `json:"summary"`
}

// Analyzer 装备分析器（带 Fingerprint 缓存）
type Analyzer struct {
	mu    sync.RWMutex
	cache *EquipmentAnalysis
}

// NewAnalyzer 构造 Analyzer
func NewAnalyzer() *Analyzer {
	return &Analyzer{}
}

// Analyze 输入 GroundedGameContext + CompositionAnalysis，输出 EquipmentAnalysis
//
// 行为约定：
//   - g == nil → 返回 zero-value EquipmentAnalysis
//   - 缓存命中（Fingerprint.Equal）→ 直接返回缓存
//   - 0 LLM 调用（纯本地）
//
// Composition 分析用于标记 IsCore / CoreScore / CoreStatus（不重算 Core 评分）。
func (a *Analyzer) Analyze(g grounder.GroundedGameContext, comp *composition.CompositionAnalysis) EquipmentAnalysis {
	fp := ComputeFingerprint(g)

	// Fast path: cache hit
	a.mu.RLock()
	if a.cache != nil && a.cache.Fingerprint.Equal(fp) {
		out := *a.cache
		a.mu.RUnlock()
		log.Debugf("equipment: cache hit fp=%s", fp.Hash()[:8])
		return out
	}
	a.mu.RUnlock()

	// Slow path: rebuild
	out := a.analyzeOnce(g, comp, fp)

	a.mu.Lock()
	a.cache = &out
	a.mu.Unlock()
	log.Debugf("equipment: cache miss fp=%s champions=%d items=%d",
		fp.Hash()[:8], len(out.Champions), len(out.ItemIndex))
	return out
}

// analyzeOnce 真正做分析（pure function，不加锁）
func (a *Analyzer) analyzeOnce(g grounder.GroundedGameContext, comp *composition.CompositionAnalysis, fp EquipmentFingerprint) EquipmentAnalysis {
	out := EquipmentAnalysis{
		Fingerprint: fp,
		GeneratedAt: time.Now(),
		Patch:       g.Meta.Patch,
		SetKey:      g.Meta.SetKey,
		ItemIndex:   map[string]ItemOwnership{},
	}

	// Core 索引：ChampionID → CoreCandidate（用于填充 ChampionEquipment.IsCore）
	coreByID := map[string]composition.CoreCandidate{}
	if comp != nil {
		for _, c := range comp.CoreCandidates {
			coreByID[c.ChampionID] = c
		}
	}

	// 1) 棋盘：每个 Champion → Items
	board := g.Board
	sort.SliceStable(board, func(i, j int) bool {
		return board[i].Position < board[j].Position
	})
	for _, slot := range board {
		ce := ChampionEquipment{
			ChampionID: slot.Champion.ID,
			Name:       slot.Champion.Name,
			Star:       slot.Star,
			Cost:       slot.Champion.Cost,
			Position:   slot.Position,
			Items:      make([]ItemRef, 0, len(slot.Items)),
		}
		// 填 Core
		if core, ok := coreByID[slot.Champion.ID]; ok {
			ce.IsCore = core.Status == composition.CoreStatusLikely || core.Status == composition.CoreStatusCandidate
			ce.CoreScore = core.Score
			ce.CoreStatus = core.Status
		}
		// 填 Items
		for _, it := range slot.Items {
			t := classifyItemType(it)
			ce.Items = append(ce.Items, ItemRef{
				ItemID: it.ID,
				Name:   it.Name,
				Type:   t,
				Status: it.Status,
			})
			// 更新 ItemIndex
			a.accumulateOwnership(out.ItemIndex, it, slot)
		}
		ce.ItemCount = len(ce.Items)
		out.Champions = append(out.Champions, ce)
	}

	// 2) tctx.Items 散件口袋：直接累计到 ItemIndex.PocketCount
	if g.GameState.Stage != "" || g.GameState.Level != 0 {
		// 用 GroundedGameState 占位判断 GameContext 是否已知（避免空 Gc 时的误判）
	}
	// 走 GameState 路径无法拿到 Items 列表（Items 不在 GameState 字段里）
	// 改：从 GroundedGameContext 读 Items 列表
	for _, it := range g.Items {
		t := classifyItemType(it.Item)
		ownership := out.ItemIndex[it.Item.ID]
		if ownership.ItemID == "" {
			ownership = ItemOwnership{
				ItemID: it.Item.ID,
				Name:   it.Item.Name,
				Type:   t,
				From:   it.Item.From,
			}
		}
		ownership.PocketCount += it.Count
		ownership.TotalCount = ownership.BoardCount + ownership.PocketCount
		out.ItemIndex[it.Item.ID] = ownership
	}

	// 3) Core Equipment Link
	for _, c := range out.Champions {
		if !c.IsCore || len(c.Items) == 0 {
			continue
		}
		// 取 Core Candidate 的 Reason
		var reason string
		if core, ok := coreByID[c.ChampionID]; ok {
			reason = core.Reason
		}
		out.CoreEquipment = append(out.CoreEquipment, CoreEquipmentLink{
			ChampionID: c.ChampionID,
			Name:       c.Name,
			Star:       c.Star,
			Cost:       c.Cost,
			Items:      c.Items,
			CoreStatus: c.CoreStatus,
			CoreReason: reason,
		})
	}

	// 4) Summary
	out.Summary = buildSummary(out.Champions, out.ItemIndex)

	return out
}

// accumulateOwnership 把单条 ResolvedItem 累加到 ItemIndex
func (a *Analyzer) accumulateOwnership(idx map[string]ItemOwnership, it grounder.ResolvedItem, slot grounder.GroundedBoardSlot) {
	ownership := idx[it.ID]
	if ownership.ItemID == "" {
		ownership = ItemOwnership{
			ItemID: it.ID,
			Name:   it.Name,
			Type:   classifyItemType(it),
			From:   it.From,
		}
	}
	ownership.BoardCount++
	ownership.TotalCount = ownership.BoardCount + ownership.PocketCount
	ownership.Holders = append(ownership.Holders, ItemHolder{
		ChampionID: slot.Champion.ID,
		Name:       slot.Champion.Name,
		Position:   slot.Position,
		Star:       slot.Star,
	})
	idx[it.ID] = ownership
}

// classifyItemType 从 ResolvedItem 判定 ItemType
//
// 规则：
//   - Status != Resolved → Unknown（KB 不可用 / 找不到 / 跨 set）
//   - From 非空 → Completed（有合成路径）
//   - From 空（nil 或 []）→ Component（散件，本身不能再合成；或 KB 缺 From 字段）
//
// 注意：实际 TFT 中"无 From 的装备"通常是散件；少数 Completed 装备的 KB 缺 From 字段 → 误判为 Component。
// 这是 KB 数据限制，可接受；Format 阶段保留 ItemType 字段供 LLM 二次判断。
func classifyItemType(it grounder.ResolvedItem) ItemType {
	if it.Status != grounder.StatusResolved {
		return ItemTypeUnknown
	}
	if len(it.From) > 0 {
		return ItemTypeCompleted
	}
	return ItemTypeComponent
}

// buildSummary 汇总装备统计
func buildSummary(champs []ChampionEquipment, idx map[string]ItemOwnership) EquipmentSummary {
	s := EquipmentSummary{
		UniqueItems: len(idx),
	}
	// KB 状态：基于 ItemIndex 中 Unknown 的比例
	unknownItems := 0
	resolvedItems := 0
	for _, own := range idx {
		s.TotalItems += own.TotalCount
		switch own.Type {
		case ItemTypeCompleted:
			s.CompletedCount += own.TotalCount
			resolvedItems++
		case ItemTypeComponent:
			s.ComponentCount += own.TotalCount
			resolvedItems++
		case ItemTypeUnknown:
			s.UnknownCount += own.TotalCount
			unknownItems++
		}
	}
	// 至少有一个 item resolved → ok；只有 unknown → unavailable；混合 → partial
	switch {
	case resolvedItems > 0 && unknownItems == 0:
		s.ItemKBStatus = "ok"
	case resolvedItems == 0 && unknownItems > 0:
		s.ItemKBStatus = "unavailable"
	case resolvedItems > 0 && unknownItems > 0:
		s.ItemKBStatus = "partial"
	default:
		s.ItemKBStatus = "ok" // 没有 item 时也视为 ok
	}
	// Core 装备齐全数
	for _, c := range champs {
		if c.IsCore && c.ItemCount >= 3 {
			s.CoreChampionsWithFullItems++
		}
	}
	return s
}

// Equal 判断两个 Fingerprint 是否相等
func (f EquipmentFingerprint) Equal(other EquipmentFingerprint) bool {
	if f.Patch != other.Patch || f.SetKey != other.SetKey {
		return false
	}
	if len(f.BoardItemSlots) != len(other.BoardItemSlots) {
		return false
	}
	for i, s := range f.BoardItemSlots {
		if s.ChampionID != other.BoardItemSlots[i].ChampionID ||
			s.Position != other.BoardItemSlots[i].Position ||
			s.Star != other.BoardItemSlots[i].Star ||
			!stringSlicesEqual(s.ItemIDs, other.BoardItemSlots[i].ItemIDs) {
			return false
		}
	}
	if len(f.PocketItems) != len(other.PocketItems) {
		return false
	}
	for i, p := range f.PocketItems {
		if p.ItemID != other.PocketItems[i].ItemID || p.Count != other.PocketItems[i].Count {
			return false
		}
	}
	return true
}

// stringSlicesEqual 比较两个字符串 slice 是否相等
func stringSlicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
