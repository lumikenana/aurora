// Package grounder - TFT Knowledge Grounding Layer (Phase 15)
//
// 本包实现 GameContext → GroundedGameContext 的确定性富化：
//   - 输入：*tft.TFTGameContext（仅含 canonical ID）
//   - 输出：GroundedGameContext（含 KB 详细字段）+ GroundedMeta（patch / set / KB 状态）
//
// 严格遵守不变量：
//   - 无 LLM / 无网络 / 无 OCR / 无截图
//   - 严格 patch 隔离（3 concentric rings: SQL PK / Service.queryScoped / ID-prefix）
//   - 未知实体保留为 Status=Unknown（不猜测，不跨 set 替代）
//   - 不修改 GameContext（纯投影 / value object）
//
// 不实现：
//   - 阵容评分 / 装备推荐 / 经济分析
//   - 自动游戏操作
//   - RAG / Embedding / Vector DB
package grounder

import "encoding/json"

// MatchStatus 单个实体的 KB 解析状态
//
// 与 GameContext.Value[T].Confidence（OCR 可信度）是**正交**的两条独立维度：
//   - Confidence 来自视觉识别层（高 = LLM 看清了）
//   - MatchStatus 来自知识库层（高 = KB 命中当前 patch）
type MatchStatus string

const (
	// StatusResolved 命中当前 patch 的 KB
	StatusResolved MatchStatus = "resolved"
	// StatusUnknown 输入为 "" / "Unknown" / KB 无此 ID
	StatusUnknown MatchStatus = "unknown"
	// StatusWrongSet ID 前缀与当前 setKey 不一致（跨 Set，**绝不**静默纠正）
	StatusWrongSet MatchStatus = "wrong_set"
	// StatusKBUnavailable KB provider 为 nil 或查询出错
	StatusKBUnavailable MatchStatus = "kb_unavailable"
	// StatusSkipped 主动跳过（不查 KB）
	StatusSkipped MatchStatus = "skipped"
)

// ResolvedChampion 关联后的英雄实体
//
// 字段来源：
//   - ID: GameContext 原始输入
//   - Name / Cost / Traits / Ability / Stats / Role: 来自 KB Champion.Data（CommunityDragon 格式）
//   - SetKey: 推断自 ID 前缀
//   - Patch: 实际查询 KB 使用的 patch
type ResolvedChampion struct {
	ID      string `json:"id"`
	Name    string `json:"name,omitempty"`
	Cost    int    `json:"cost,omitempty"`
	Traits  []string `json:"traits,omitempty"`
	Ability json.RawMessage `json:"ability,omitempty"`
	Stats   json.RawMessage `json:"stats,omitempty"`
	Role    string `json:"role,omitempty"`
	SetKey  string `json:"set_key,omitempty"`
	Patch   string `json:"patch,omitempty"`
	Status  MatchStatus `json:"status"`
}

// ResolvedItem 关联后的装备实体
type ResolvedItem struct {
	ID          string `json:"id"`
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
	Effects     json.RawMessage `json:"effects,omitempty"`
	From        []string `json:"from,omitempty"`
	SetKey      string `json:"set_key,omitempty"`
	Patch       string `json:"patch,omitempty"`
	Status      MatchStatus `json:"status"`
}

// ResolvedTrait 关联后的羁绊实体
type ResolvedTrait struct {
	ID      string `json:"id"`
	Name    string `json:"name,omitempty"`
	Desc    json.RawMessage `json:"desc,omitempty"`
	Effects json.RawMessage `json:"effects,omitempty"`
	Type    string `json:"type,omitempty"`
	SetKey  string `json:"set_key,omitempty"`
	Patch   string `json:"patch,omitempty"`
	Status  MatchStatus `json:"status"`
}

// ResolvedAugment 关联后的强化符文实体
type ResolvedAugment struct {
	ID          string `json:"id"`
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
	SetKey      string `json:"set_key,omitempty"`
	Patch       string `json:"patch,omitempty"`
	Status      MatchStatus `json:"status"`
}

// ---- 槽位类型（与 tft.TFTGameContext 槽位字段一一对应）----

// GroundedBoardSlot 棋盘一个槽位的富化结果
type GroundedBoardSlot struct {
	Position int              `json:"position"`
	Star     int              `json:"star"`
	Champion ResolvedChampion `json:"champion"`
	Items    []ResolvedItem   `json:"items,omitempty"`
}

// GroundedBenchSlot 备战席一个槽位的富化结果
type GroundedBenchSlot struct {
	Champion ResolvedChampion `json:"champion"`
	Star     int              `json:"star"`
}

// GroundedShopSlot 商店一个槽位的富化结果
type GroundedShopSlot struct {
	Champion ResolvedChampion `json:"champion"`
	Cost     int              `json:"cost"`
}

// GroundedItemSlot 装备/散件一个槽位的富化结果
type GroundedItemSlot struct {
	Item  ResolvedItem `json:"item"`
	Count int          `json:"count"`
}

// GroundedTraitState 羁绊状态一个条目的富化结果
type GroundedTraitState struct {
	Trait  ResolvedTrait `json:"trait"`
	Active int           `json:"active"`
	Style  int           `json:"style"`
}

// GroundedAugmentSlot 强化符文一个槽位的富化结果
type GroundedAugmentSlot struct {
	Augment ResolvedAugment `json:"augment"`
	Tier    string          `json:"tier"`
}

// GroundedGameState 当前对局基础资源（从 tctx 透传）
type GroundedGameState struct {
	Stage      string `json:"stage,omitempty"`
	Level      int    `json:"level,omitempty"`
	XP         int    `json:"xp,omitempty"`
	XPTolevel  int    `json:"xp_to_level,omitempty"`
	Gold       int    `json:"gold,omitempty"`
	Interest   int    `json:"interest,omitempty"`
	HP         int    `json:"hp,omitempty"`
	WinStreak  int    `json:"win_streak,omitempty"`
	LoseStreak int    `json:"lose_streak,omitempty"`
	Phase      string `json:"phase,omitempty"`
}

// GroundedMeta Grounding 元数据
type GroundedMeta struct {
	// Patch 实际生效的 patch（GameContext.Version 优先，KB active 回退）
	Patch string `json:"patch,omitempty"`
	// SetKey 推断自 patch 或首个 ID 前缀
	SetKey string `json:"set_key,omitempty"`
	// KBHasData KB provider 是否报告有数据
	KBHasData bool `json:"kb_has_data"`
	// PatchFallback true = 用了 KB active patch 而非 GameContext.Version
	PatchFallback bool `json:"patch_fallback"`
}

// GroundedGameContext Grounding 输出（value object，不可变）
type GroundedGameContext struct {
	GameState GroundedGameState    `json:"game_state"`
	Board     []GroundedBoardSlot  `json:"board,omitempty"`
	Bench     []GroundedBenchSlot  `json:"bench,omitempty"`
	Shop      []GroundedShopSlot   `json:"shop,omitempty"`
	Items     []GroundedItemSlot   `json:"items,omitempty"`
	Traits    []GroundedTraitState `json:"traits,omitempty"`
	Augments  []GroundedAugmentSlot `json:"augments,omitempty"`
	Meta      GroundedMeta         `json:"meta"`
}

// Counts 便于日志/debug 的长度统计
func (g GroundedGameContext) Counts() (board, bench, shop, items, traits, augs int) {
	return len(g.Board), len(g.Bench), len(g.Shop), len(g.Items), len(g.Traits), len(g.Augments)
}

// HasWrongSet 是否有任何实体处于 WrongSet 状态（用于 Format 顶部警告）
func (g GroundedGameContext) HasWrongSet() bool {
	for _, b := range g.Board {
		if b.Champion.Status == StatusWrongSet {
			return true
		}
		for _, it := range b.Items {
			if it.Status == StatusWrongSet {
				return true
			}
		}
	}
	for _, b := range g.Bench {
		if b.Champion.Status == StatusWrongSet {
			return true
		}
	}
	for _, s := range g.Shop {
		if s.Champion.Status == StatusWrongSet {
			return true
		}
	}
	for _, it := range g.Items {
		if it.Item.Status == StatusWrongSet {
			return true
		}
	}
	for _, t := range g.Traits {
		if t.Trait.Status == StatusWrongSet {
			return true
		}
	}
	for _, a := range g.Augments {
		if a.Augment.Status == StatusWrongSet {
			return true
		}
	}
	return false
}
