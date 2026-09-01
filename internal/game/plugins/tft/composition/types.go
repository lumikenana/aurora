// Package composition - TFT 阵容分析层（Phase 16-A）
//
// 本包实现 "GroundedGameContext → CompositionAnalysis" 的确定性本地分析：
//   - 场上英雄（Active Champions）
//   - 羁绊聚合（Trait Aggregates）+ 当前/下一 Breakpoint
//   - 核心候选（Core Candidates）：基于 Cost / Star / Items / 唯一羁绊的 Evidence 评分
//   - CompositionSummary：结构化描述（不强行命名阵容）
//
// 严格遵守不变量：
//   - 0 LLM 调用：所有计算纯本地（KB 查询发生在上游 Grounder.Resolve；本包只读 GroundedGameContext）
//   - 0 硬编码羁绊规则：breakpoint 全部来自 KB（Trait.Data 的 effects 字段）
//   - patch 隔离：复用 GroundedMeta.Patch / SetKey；breakpoint 来自当前 patch 的 KB
//   - Unknown 显式标注：KB 缺失 / 解析失败 → TraitAggregate.Status = Unknown，不猜测
//   - 幂等 + 缓存：Fingerprint 不变 → 直接返回缓存，不重新分析
//
// 不实现：
//   - D 牌 / 升级 / 经济决策
//   - 装备推荐 / 装备合成
//   - Meta 阵容命名
//   - 战斗预测 / 自动操作
//   - LLM 解释（交给 Agent LLM 做）
package composition

import (
	"time"

	"github.com/lipipi/aurora/internal/game/plugins/tft/grounder"
)

// TraitAggregateStatus 羁绊聚合状态
//
// 与 grounder.MatchStatus 区分：
//   - MatchStatus 表达单个 ID 在 KB 里的解析状态
//   - TraitAggregateStatus 表达"聚合结果"的状态（KB 完整 vs. breakpoint 缺失 vs. ID 缺失等）
type TraitAggregateStatus string

const (
	// TraitStatusActive 羁绊在当前单位数下至少达到 1 阶（最低 breakpoint 已激活）
	TraitStatusActive TraitAggregateStatus = "active"
	// TraitStatusInactive 单位数不足，未达到任何 breakpoint
	TraitStatusInactive TraitAggregateStatus = "inactive"
	// TraitStatusUnknown KB 中找不到该羁绊的 breakpoint 数据（patch 不支持 / 实体未识别 / KB 不可用）
	TraitStatusUnknown TraitAggregateStatus = "unknown"
)

// CoreCandidateStatus 核心候选状态
//
// 不使用 Confirmed / Rejected 这类二元结论（KB 没有"meta tier"数据），
// 只区分 Candidate / Likely / Unknown 三档：
//   - Unknown：没有足够证据判断（如 1 星白板 1 费）
//   - Candidate：有部分证据
//   - Likely：多个独立证据汇聚
type CoreCandidateStatus string

const (
	// CoreStatusUnknown 证据不足，无法判断
	CoreStatusUnknown CoreCandidateStatus = "unknown"
	// CoreStatusCandidate 有部分证据（单维度）
	CoreStatusCandidate CoreCandidateStatus = "candidate"
	// CoreStatusLikely 多个独立证据汇聚（多维度）
	CoreStatusLikely CoreCandidateStatus = "likely"
)

// EvidenceType 证据类型
type EvidenceType string

const (
	EvidenceHighCost   EvidenceType = "high_cost"    // 高费（5 费）
	EvidenceHighStar   EvidenceType = "high_star"    // 高星（2★ / 3★）
	EvidenceHasItems   EvidenceType = "has_items"    // 已装备
	EvidenceManyItems  EvidenceType = "many_items"   // 装备 ≥ 2
	EvidenceUniqueTribe EvidenceType = "unique_trait" // 持有其他 Champion 不具备的羁绊
)

// CoreEvidence 核心候选的证据
type CoreEvidence struct {
	// Type 证据类型
	Type EvidenceType `json:"type"`
	// Weight 证据权重（累加得到 Score）
	Weight float64 `json:"weight"`
	// Detail 人类可读描述（用于 Format 输出 / Debug）
	Detail string `json:"detail"`
}

// ActiveChampion 场上英雄（聚合所需最小信息）
//
// 来源：GroundedGameContext.Board（每个 GroundedBoardSlot 投影为 ActiveChampion）
// 不修改 GroundedBoardSlot；本包只读消费。
type ActiveChampion struct {
	// ID 原始 canonical ID（如 "TFT18_Vayne"）
	ID string `json:"id"`
	// Name 来自 KB（可能为空：Unknown / KBUnavailable）
	Name string `json:"name,omitempty"`
	// Cost 来自 KB（0 = Unknown / KBUnavailable）
	Cost int `json:"cost,omitempty"`
	// Star 来自 GameContext
	Star int `json:"star"`
	// Position 棋盘格子编号
	Position int `json:"position,omitempty"`
	// Items 装备 ID 列表
	Items []string `json:"items,omitempty"`
	// Traits 该英雄持有的羁绊 ID 列表（来自 KB）
	Traits []string `json:"traits,omitempty"`
	// Role 来自 KB
	Role string `json:"role,omitempty"`
	// Status 来自 GroundedGameContext.Board[i].Champion.Status
	Status grounder.MatchStatus `json:"status"`
}

// Breakpoint 羁绊的一个激活阶
type Breakpoint struct {
	// MinUnits 激活所需的最小单位数（如 2/4/6 的 2、4、6）
	MinUnits int `json:"min_units"`
	// MaxUnits 该阶生效的最大单位数（0 = 无上限；如 6/9 的 max_units=8, 9+）
	MaxUnits int `json:"max_units"`
	// StyleIdx 阶索引（CD 用 2/3/5/... 表示 Bronze/Silver/Gold/Prismatic）
	StyleIdx int `json:"style_idx,omitempty"`
	// StyleName 阶名称（kBronze / kSilver / kGold / kPrismatic）
	StyleName string `json:"style_name,omitempty"`
	// Effect 该阶效果的简要描述（KB 完整 desc 截断到 EffectLimit 字符）
	Effect string `json:"effect,omitempty"`
}

// TraitAggregate 单个羁绊的聚合结果
type TraitAggregate struct {
	// TraitID 原始 ID（如 "TFT18_Bulwark"）
	TraitID string `json:"trait_id"`
	// Name 来自 KB（可能为空：Unknown）
	Name string `json:"name,omitempty"`
	// Type 来自 KB（Origin / Class 等）
	Type string `json:"type,omitempty"`
	// ActiveUnits 当前场上持有该羁绊的英雄数（去重后）
	ActiveUnits int `json:"active_units"`
	// ContributingIDs 贡献单位的 ID 列表
	ContributingIDs []string `json:"contributing_ids,omitempty"`
	// CurrentBreakpoint 当前激活的最高 breakpoint；nil 表示未达到任何阶
	CurrentBreakpoint *Breakpoint `json:"current_breakpoint,omitempty"`
	// NextBreakpoint 下一个未达成的 breakpoint；nil 表示已达满阶
	NextBreakpoint *Breakpoint `json:"next_breakpoint,omitempty"`
	// IsActive 是否激活（CurrentBreakpoint != nil）
	IsActive bool `json:"is_active"`
	// Status 聚合状态
	Status TraitAggregateStatus `json:"status"`
	// BreakpointCount KB 中已知的 breakpoint 数量（0 = 没有 breakpoint 数据）
	BreakpointCount int `json:"breakpoint_count"`
}

// CoreCandidate 核心候选
type CoreCandidate struct {
	// ChampionID 关联的 ActiveChampion.ID
	ChampionID string `json:"champion_id"`
	// Name 来自 KB
	Name string `json:"name,omitempty"`
	// Star
	Star int `json:"star"`
	// Cost
	Cost int `json:"cost,omitempty"`
	// Score 总分（累加所有 Evidence.Weight）
	Score float64 `json:"score"`
	// Status 候选状态
	Status CoreCandidateStatus `json:"status"`
	// Evidence 全部证据
	Evidence []CoreEvidence `json:"evidence,omitempty"`
	// Reason 简短说明（由 Format 生成）
	Reason string `json:"reason,omitempty"`
}

// CompositionSummary 阵容结构化总结
type CompositionSummary struct {
	// PrimaryTrait 当前单位数最多的羁绊（ID），可能为空
	PrimaryTrait string `json:"primary_trait,omitempty"`
	// SecondaryTrait 单位数次多的羁绊（不同于 PrimaryTrait）
	SecondaryTrait string `json:"secondary_trait,omitempty"`
	// CoreChampionCount 核心候选数（Likely + Candidate）
	CoreChampionCount int `json:"core_champion_count"`
	// ActiveTraitCount 至少 1 阶已激活的羁绊数
	ActiveTraitCount int `json:"active_trait_count"`
	// TraitKBStatus KB 羁绊数据状态：ok / partial / unavailable
	TraitKBStatus string `json:"trait_kb_status"`
}

// CompositionFingerprint Composition 缓存 key
//
// 不变量：
//   - 仅包含"影响阵容结构"的字段（Board Champion IDs / Star / Items）
//   - 不包含 Shop / Gold / XP / Stage / HP（这些变化不应触发重算）
//   - 不包含 Bench（默认不参与 Board Composition 分析；Bench 在 Phase 16-B 单独处理）
//   - Patch / SetKey 是 KB 版本键，patch 变化 → 重算
type CompositionFingerprint struct {
	// Patch 当前生效 patch
	Patch string `json:"patch"`
	// SetKey 推断自 patch（用于 ID prefix 校验一致性）
	SetKey string `json:"set_key"`

	// BoardChampionIDs 排序后的 Board 英雄 ID（用于排序无关比较）
	BoardChampionIDs []string `json:"board_champion_ids"`
	// StarByChampion championID → star
	StarByChampion map[string]int `json:"star_by_champion"`
	// ItemsByChampion championID → 排序后的 items
	ItemsByChampion map[string][]string `json:"items_by_champion"`
}

// CompositionAnalysis CompositionAnalyzer 输出
type CompositionAnalysis struct {
	// Fingerprint 缓存 key（同一 Fingerprint → 复用本对象）
	Fingerprint CompositionFingerprint `json:"fingerprint"`
	// GeneratedAt 分析生成时间
	GeneratedAt time.Time `json:"generated_at"`
	// Patch 实际生效 patch
	Patch string `json:"patch"`
	// SetKey
	SetKey string `json:"set_key"`
	// ChampionCount Board 英雄数
	ChampionCount int `json:"champion_count"`
	// ActiveChampions Board 英雄列表（按 Position 排序）
	ActiveChampions []ActiveChampion `json:"active_champions"`
	// Traits 羁绊聚合（按 ActiveUnits 降序，ID 升序）
	Traits []TraitAggregate `json:"traits"`
	// CoreCandidates 核心候选（按 Score 降序，ID 升序）
	CoreCandidates []CoreCandidate `json:"core_candidates"`
	// Summary
	Summary CompositionSummary `json:"summary"`
}

// KBStatusTrait 报告 Trait KB 的整体可用性
//
// 在每次分析中通过已知的 breakpoint 数量计算：
//   - len(Traits) == 0 → "ok"（没有需要查询的羁绊）
//   - 所有 TraitAggregate.Status == Active|Inactive → "ok"
//   - 部分 Unknown → "partial"
//   - 全部 Unknown → "unavailable"
func KBStatusTrait(traits []TraitAggregate) string {
	if len(traits) == 0 {
		return "ok"
	}
	unknown := 0
	for _, t := range traits {
		if t.Status == TraitStatusUnknown {
			unknown++
		}
	}
	switch {
	case unknown == 0:
		return "ok"
	case unknown == len(traits):
		return "unavailable"
	default:
		return "partial"
	}
}
