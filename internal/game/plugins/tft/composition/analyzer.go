// Package composition - CompositionAnalyzer 主实现（Phase 16-A）
//
// 公共入口：
//   - Analyzer.Analyze(grnd) → CompositionAnalysis（带 fingerprint 缓存）
//   - 0 LLM 调用、0 网络、0 截图、0 OCR
//
// 内部流程：
//  1. 计算 Fingerprint
//  2. 命中缓存 → 直接返回（fingerprint Equal 即命中）
//  3. 未命中 → 重建：
//     a. ActiveChampions ← GroundedGameContext.Board
//     b. TraitAggregate ← group(ActiveChampions.Traits) + 查 KB breakpoint
//     c. CoreCandidate ← 对每个 ActiveChampion 按 Evidence 累加
//     d. CompositionSummary ← 排序 + 选 Primary/Secondary
//  4. 写缓存
//
// 并发安全：Analyzer 持有 mutex；Analyze 持有时间短（KB 查询发生在上游 Grounder）。
package composition

import (
	"encoding/json"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/lipipi/aurora/internal/game/plugins/tft/grounder"
	"github.com/lipipi/aurora/internal/log"
)

// EffectLimit Breakpoint.Effect 字符上限（避免 prompt 爆炸；与 grounder.format 一致）
const EffectLimit = 120

// Evidence weights（核心候选评分权重）
//
// 数值经过人工调优，目标：
//   - 单个高强度证据（如 5 费）就能进入 Candidate
//   - 多个证据汇聚才能进入 Likely
//   - 完全无证据（1 费 1 星白板）→ Unknown
const (
	WeightHighCost     = 0.4 // 5 费英雄（潜在的 carry）
	WeightMidCost      = 0.2 // 4 费英雄
	WeightStar2        = 0.3 // 2 星
	WeightStar3        = 0.6 // 3 星（最高）
	WeightHasItem      = 0.2 // 至少有 1 件装备
	WeightManyItems    = 0.3 // 装备 ≥ 2 件
	WeightUniqueTrait  = 0.4 // 持有其他 Champion 不具备的羁绊
)

// Status 升档阈值
const (
	CoreScoreUnknown    = 0.0
	CoreScoreCandidate  = 0.3
	CoreScoreLikely     = 0.7
)

// Analyzer TFT 阵容分析器（带 Fingerprint 缓存）
//
// 无状态 + 缓存：
//   - analyzeOnce 不持锁（pure function）
//   - Analyze 加读锁（cache hit 快速返回）+ 写锁（miss 重建后写回）
//   - 缓存只存 1 个 slot（最近一次 fingerprint 的结果）；Aurora 当前是单 stream 决策
//
// 线程安全：sync.RWMutex 保护 cache。
type Analyzer struct {
	mu    sync.RWMutex
	cache *CompositionAnalysis
}

// NewAnalyzer 构造 Analyzer（可选）
func NewAnalyzer() *Analyzer {
	return &Analyzer{}
}

// Analyze 输入 GroundedGameContext，输出 CompositionAnalysis
//
// 行为约定：
//   - g == nil → 返回 zero-value CompositionAnalysis（Fingerprint.IsZero() == true）
//   - patch 未解析 / KB 不可用 → 仍输出结构，但所有 Trait.Status = Unknown
//   - Empty Board → ChampionCount=0，其他字段为空
//   - 缓存命中（Fingerprint.Equal）→ 直接返回缓存，不重建
func (a *Analyzer) Analyze(g grounder.GroundedGameContext) CompositionAnalysis {
	fp := ComputeFingerprint(g)

	// Fast path: cache hit
	a.mu.RLock()
	if a.cache != nil && a.cache.Fingerprint.Equal(fp) {
		out := *a.cache
		a.mu.RUnlock()
		log.Debugf("composition: cache hit fp=%s", fp.Hash()[:8])
		return out
	}
	a.mu.RUnlock()

	// Slow path: rebuild
	out := a.analyzeOnce(g, fp)

	a.mu.Lock()
	a.cache = &out
	a.mu.Unlock()
	log.Debugf("composition: cache miss fp=%s board=%d traits=%d cores=%d",
		fp.Hash()[:8], out.ChampionCount, len(out.Traits), len(out.CoreCandidates))
	return out
}

// analyzeOnce 真正做分析（pure function，不加锁）
func (a *Analyzer) analyzeOnce(g grounder.GroundedGameContext, fp CompositionFingerprint) CompositionAnalysis {
	out := CompositionAnalysis{
		Fingerprint:  fp,
		GeneratedAt:  time.Now(),
		Patch:        g.Meta.Patch,
		SetKey:       g.Meta.SetKey,
	}

	// 1) Board → ActiveChampions（按 Position 排序；过滤空 / Unknown 槽位）
	board := g.Board
	sort.SliceStable(board, func(i, j int) bool {
		return board[i].Position < board[j].Position
	})
	for _, s := range board {
		id := s.Champion.ID
		// 跳过空 ID / "Unknown"——它们没有 Board 含义上的"英雄"信息
		if id == "" || id == "Unknown" {
			continue
		}
		out.ActiveChampions = append(out.ActiveChampions, ActiveChampion{
			ID:       id,
			Name:     s.Champion.Name,
			Cost:     s.Champion.Cost,
			Star:     s.Star,
			Position: s.Position,
			Traits:   s.Champion.Traits,
			Role:     s.Champion.Role,
			Status:   s.Champion.Status,
			Items:    extractItemIDs(s.Items),
		})
	}
	out.ChampionCount = len(out.ActiveChampions)

	// 2) Trait aggregation
	out.Traits = aggregateTraits(out.ActiveChampions, g.Traits)

	// 3) Core candidates
	out.CoreCandidates = scoreCoreCandidates(out.ActiveChampions, out.Traits)

	// 4) Summary
	out.Summary = buildSummary(out.Traits, out.CoreCandidates)

	return out
}

// ---- Trait aggregation ----

// aggregateTraits 按 TraitID 分组计数 + 查 KB breakpoint
//
// 实现细节：
//   - 只统计 Resolved 英雄的 traits（避免 Unknown 英雄的 "" 假数据）
//   - KB 通过 g.Traits（已 Grounded）查 breakpoint；若 KB 无该 trait 的 breakpoint 数据 → Status=Unknown
//   - breakpoints 排序：MinUnits 升序
func aggregateTraits(active []ActiveChampion, resolvedTraits []grounder.GroundedTraitState) []TraitAggregate {
	// 1) group by traitID
	type traitAcc struct {
		count    int
		contrib  []string
		resolved *grounder.GroundedTraitState // first matched KB row
	}
	byID := map[string]*traitAcc{}
	for _, c := range active {
		// 只采纳 Resolved 英雄的 trait（未知英雄的 traits 不可信）
		if c.Status != grounder.StatusResolved {
			continue
		}
		for _, tid := range c.Traits {
			if tid == "" {
				continue
			}
			acc, ok := byID[tid]
			if !ok {
				acc = &traitAcc{}
				byID[tid] = acc
			}
			acc.count++
			acc.contrib = append(acc.contrib, c.ID)
		}
	}

	// 2) KB lookup
	resolvedByID := map[string]grounder.GroundedTraitState{}
	for _, t := range resolvedTraits {
		if t.Trait.ID == "" {
			continue
		}
		// first wins（dedup；Grounder 内部已 dedup）
		if _, ok := resolvedByID[t.Trait.ID]; !ok {
			resolvedByID[t.Trait.ID] = t
		}
	}

	// 3) build aggregates
	aggregates := make([]TraitAggregate, 0, len(byID))
	for tid, acc := range byID {
		agg := TraitAggregate{
			TraitID:         tid,
			ActiveUnits:     acc.count,
			ContributingIDs: acc.contrib,
		}
		// KB lookup
		row, hasRow := resolvedByID[tid]
		if hasRow && row.Trait.Status == grounder.StatusResolved {
			agg.Name = row.Trait.Name
			agg.Type = row.Trait.Type
			breakpoints := decodeBreakpoints(row.Trait.Effects)
			agg.BreakpointCount = len(breakpoints)
			if len(breakpoints) == 0 {
				// KB 中无 breakpoint 数据
				agg.Status = TraitStatusUnknown
			} else {
				// 找当前激活的 breakpoint
				sort.SliceStable(breakpoints, func(i, j int) bool {
					return breakpoints[i].MinUnits < breakpoints[j].MinUnits
				})
				var current, next *Breakpoint
				for i := range breakpoints {
					bp := breakpoints[i]
					if acc.count >= bp.MinUnits {
						current = &bp
					} else if next == nil {
						next = &bp
					}
				}
				agg.CurrentBreakpoint = current
				agg.NextBreakpoint = next
				if current != nil {
					agg.IsActive = true
					agg.Status = TraitStatusActive
				} else {
					agg.Status = TraitStatusInactive
				}
			}
		} else {
			// KB 没找到该 trait
			agg.Status = TraitStatusUnknown
		}
		aggregates = append(aggregates, agg)
	}

	// 4) sort: ActiveUnits desc, TraitID asc
	sort.SliceStable(aggregates, func(i, j int) bool {
		if aggregates[i].ActiveUnits != aggregates[j].ActiveUnits {
			return aggregates[i].ActiveUnits > aggregates[j].ActiveUnits
		}
		return aggregates[i].TraitID < aggregates[j].TraitID
	})
	return aggregates
}

// decodeBreakpoints 从 Trait.Effects (json.RawMessage) 解码 breakpoint 列表
//
// 兼容 3 种 KB 格式：
//  1. CD cdragon 格式（`conditional_trait_sets` 数组，含 min_units/max_units/style_idx/style_name/constants）
//  2. 简化的 `{min_units, max_units, effect}` 数组
//  3. 任何上述都不匹配 → 返回 nil（caller 标 Status=Unknown）
//
// 完全本地解析，0 LLM 调用。
func decodeBreakpoints(raw json.RawMessage) []Breakpoint {
	// case 1: CD conditional_trait_sets
	type cdStyle struct {
		StyleIdx  int    `json:"style_idx"`
		StyleName string `json:"style_name"`
	}
	type cdBreakpoint struct {
		MinUnits  int             `json:"min_units"`
		MaxUnits  int             `json:"max_units"`
		Style     cdStyle         `json:"style"`
		Constants json.RawMessage `json:"constants"`
	}
	type cdEffects struct {
		ConditionalTraitSets []cdBreakpoint `json:"conditional_trait_sets"`
	}

	// 先尝试对象格式（CD 风格）
	var cd cdEffects
	if err := json.Unmarshal(raw, &cd); err == nil && len(cd.ConditionalTraitSets) > 0 {
		out := make([]Breakpoint, 0, len(cd.ConditionalTraitSets))
		for _, b := range cd.ConditionalTraitSets {
			if b.MinUnits <= 0 {
				continue
			}
			out = append(out, Breakpoint{
				MinUnits:  b.MinUnits,
				MaxUnits:  b.MaxUnits,
				StyleIdx:  b.Style.StyleIdx,
				StyleName: b.Style.StyleName,
				Effect:    effectFromConstants(b.Constants),
			})
		}
		return out
	}

	// case 2: 简化 array 格式
	var simple []Breakpoint
	if err := json.Unmarshal(raw, &simple); err == nil && len(simple) > 0 {
		out := make([]Breakpoint, 0, len(simple))
		for _, b := range simple {
			if b.MinUnits <= 0 {
				continue
			}
			b.Effect = truncate(b.Effect, EffectLimit)
			out = append(out, b)
		}
		return out
	}

	return nil
}

// effectFromConstants 从 CD `constants` 字段构造 Effect 文本
//
// CD 的 constants 是 key→value 映射（如 {"BonusHealth": "150"}），
// 这里只把 keys 列出来作为提示（真实 effect 文本需要替换 tooltip 中的占位符，
// 但 tooltip 不在 KB 里——所以我们不强行渲染）。
func effectFromConstants(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var m map[string]interface{}
	if err := json.Unmarshal(raw, &m); err != nil {
		return ""
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return "constants: " + strings.Join(keys, ", ")
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	if n <= 1 {
		return s[:n]
	}
	return s[:n-1] + "…"
}

// ---- Core candidate scoring ----

// scoreCoreCandidates 对每个 ActiveChampion 评分
//
// 评分规则（Evidence 累加）：
//   - HighCost (5):    +0.4
//   - MidCost (4):     +0.2
//   - Star3:           +0.6
//   - Star2:           +0.3
//   - HasItem (>=1):   +0.2
//   - ManyItems (>=2): +0.3
//   - UniqueTrait:     +0.4（持有 ≥1 个其他 Board 英雄没有的 Trait）
//
// 升档阈值：
//   - Score >= 0.7 → Likely
//   - Score >= 0.3 → Candidate
//   - Score <  0.3 → Unknown（KB 信息不完整时仍可输出）
//
// 不实现的"判断"（避免硬编码 Meta）：
//   - 不基于"是否是 X 阵容"判断
//   - 不基于"augment 组合"判断
//   - 不基于"对手是 X"判断
func scoreCoreCandidates(active []ActiveChampion, traits []TraitAggregate) []CoreCandidate {
	if len(active) == 0 {
		return nil
	}

	// 1) 计算 trait 出现频次（用于 UniqueTrait 判定）
	traitCount := map[string]int{}
	for _, c := range active {
		if c.Status != grounder.StatusResolved {
			continue
		}
		for _, t := range c.Traits {
			traitCount[t]++
		}
	}

	// 2) 评分每个 ActiveChampion
	cands := make([]CoreCandidate, 0, len(active))
	for _, c := range active {
		// KB 未识别的英雄不进入候选（无 Cost/Role/Traits 等评分依据）
		if c.Status != grounder.StatusResolved {
			cands = append(cands, CoreCandidate{
				ChampionID: c.ID,
				Name:       c.Name,
				Star:       c.Star,
				Cost:       c.Cost,
				Status:     CoreStatusUnknown,
				Reason:     "KB 未识别，无评分依据",
			})
			continue
		}

		var evidences []CoreEvidence
		var score float64

		// Cost
		switch {
		case c.Cost >= 5:
			evidences = append(evidences, CoreEvidence{
				Type:   EvidenceHighCost,
				Weight: WeightHighCost,
				Detail: "5 费高费英雄（潜在 carry）",
			})
			score += WeightHighCost
		case c.Cost == 4:
			evidences = append(evidences, CoreEvidence{
				Type:   EvidenceHighCost,
				Weight: WeightMidCost,
				Detail: "4 费中高费英雄",
			})
			score += WeightMidCost
		}

		// Star
		switch {
		case c.Star >= 3:
			evidences = append(evidences, CoreEvidence{
				Type:   EvidenceHighStar,
				Weight: WeightStar3,
				Detail: "3 星（最高星）",
			})
			score += WeightStar3
		case c.Star == 2:
			evidences = append(evidences, CoreEvidence{
				Type:   EvidenceHighStar,
				Weight: WeightStar2,
				Detail: "2 星",
			})
			score += WeightStar2
		}

		// Items
		switch n := len(c.Items); {
		case n >= 2:
			evidences = append(evidences, CoreEvidence{
				Type:   EvidenceManyItems,
				Weight: WeightManyItems,
				Detail: "装备齐全（≥2）",
			})
			score += WeightManyItems
		case n == 1:
			evidences = append(evidences, CoreEvidence{
				Type:   EvidenceHasItems,
				Weight: WeightHasItem,
				Detail: "已装备",
			})
			score += WeightHasItem
		}

		// Unique trait
		// 至少 2 个 Board 英雄时才有意义（否则"唯一持有"是平凡成立的）
		if len(active) >= 2 {
			for _, t := range c.Traits {
				if traitCount[t] == 1 {
					// 当前英雄是该 trait 在 Board 上唯一的持有者
					evidences = append(evidences, CoreEvidence{
						Type:   EvidenceUniqueTribe,
						Weight: WeightUniqueTrait,
						Detail: "持有其他 Board 英雄不具备的羁绊：" + t,
					})
					score += WeightUniqueTrait
					break // 只计一次（避免同一英雄多 unique trait 累加过高）
				}
			}
		}

		// 升档
		var status CoreCandidateStatus
		var reason string
		switch {
		case score >= CoreScoreLikely:
			status = CoreStatusLikely
			reason = buildCandidateReason(c, evidences)
		case score >= CoreScoreCandidate:
			status = CoreStatusCandidate
			reason = buildCandidateReason(c, evidences)
		default:
			status = CoreStatusUnknown
			reason = "证据不足"
		}

		cands = append(cands, CoreCandidate{
			ChampionID: c.ID,
			Name:       c.Name,
			Star:       c.Star,
			Cost:       c.Cost,
			Score:      score,
			Status:     status,
			Evidence:   evidences,
			Reason:     reason,
		})
	}

	// 3) sort: Status (Likely > Candidate > Unknown) → Score desc → ID asc
	statusRank := map[CoreCandidateStatus]int{
		CoreStatusLikely:    2,
		CoreStatusCandidate: 1,
		CoreStatusUnknown:   0,
	}
	sort.SliceStable(cands, func(i, j int) bool {
		if statusRank[cands[i].Status] != statusRank[cands[j].Status] {
			return statusRank[cands[i].Status] > statusRank[cands[j].Status]
		}
		if cands[i].Score != cands[j].Score {
			return cands[i].Score > cands[j].Score
		}
		return cands[i].ChampionID < cands[j].ChampionID
	})

	return cands
}

// buildCandidateReason 把 Evidence 转成简短中文 Reason
func buildCandidateReason(c ActiveChampion, evs []CoreEvidence) string {
	parts := make([]string, 0, len(evs))
	for _, e := range evs {
		parts = append(parts, e.Detail)
	}
	return strings.Join(parts, "；")
}

// ---- Summary ----

// buildSummary 选 Primary/Secondary Trait + 统计
//
// Primary = ActiveUnits 最多的 active trait
// Secondary = 第二多（不同 ID）
//
// 当 0 个 active trait → Primary/Secondary 都为空（不强行命名阵容）
func buildSummary(traits []TraitAggregate, cands []CoreCandidate) CompositionSummary {
	summary := CompositionSummary{
		TraitKBStatus: KBStatusTrait(traits),
	}
	for _, c := range cands {
		if c.Status == CoreStatusLikely || c.Status == CoreStatusCandidate {
			summary.CoreChampionCount++
		}
	}
	// 选 primary/secondary（从已排序的 traits 中选第一个 active）
	for _, t := range traits {
		if t.Status != TraitStatusActive {
			continue
		}
		if summary.PrimaryTrait == "" {
			summary.PrimaryTrait = t.TraitID
			summary.ActiveTraitCount++
			continue
		}
		if summary.SecondaryTrait == "" && t.TraitID != summary.PrimaryTrait {
			summary.SecondaryTrait = t.TraitID
			summary.ActiveTraitCount++
		}
	}
	return summary
}

// ---- Item ID extraction ----

// extractItemIDs 从 GroundedBoardSlot.Items 提取 ID 列表
func extractItemIDs(items []grounder.ResolvedItem) []string {
	if len(items) == 0 {
		return nil
	}
	out := make([]string, 0, len(items))
	for _, it := range items {
		if it.ID == "" {
			continue
		}
		out = append(out, it.ID)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
