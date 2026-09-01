// Package tfttypes - TFT 特定游戏上下文数据结构
//
// Phase 10-6 范围：
//   - 定义 TFTGameContext 与所有子结构（BoardSlot / BenchSlot / ShopSlot 等）
//   - 所有可识别字段全部使用 game.Value[T] 表达 unknown/known
//   - 不实现任何识别逻辑（窗口检测除外）
//   - 字段 ID 均为 Knowledge Base key 引用，不复制业务数据
//
// Phase 17-A 调整：从 package tft 移至独立 tfttypes 子包。
// 原因：tft 包需 import composition / grounder 等子包做 prompt 渲染，
// 而 grounder 需要访问 *TFTGameContext。两者直接互引会形成 import cycle
// （tft → composition → grounder → tft）。把数据类型下沉到独立的 tfttypes
// 子包后：
//   - tft 父包 import tfttypes 定义 Plugin
//   - grounder / composition 也 import tfttypes 访问数据
//   - 父与子均依赖 tfttypes，但互不依赖 → 无 cycle
package tfttypes

import "github.com/lipipi/aurora/internal/game"

// TFTGameContext TFT 特有游戏上下文
//
// 字段 ID 设计为 Knowledge Base key 引用（如 "TFT18_Vayne"），
// 不在 GameContext 内联业务数据（Name / Cost / Description 等）。
// 关联时机：未来 LLM / 工具按需查询 Knowledge Base。
type TFTGameContext struct {
	// ---- 基础资源 ----

	// Stage 当前阶段（如 "4-2" / "5-1"）
	Stage game.Value[string]
	// Level 玩家等级
	Level game.Value[int]
	// XP 当前 XP
	XP game.Value[int]
	// XPTolevel 升下一级所需 XP
	XPTolevel game.Value[int]
	// Gold 当前金币
	Gold game.Value[int]
	// Interest 当前利息（最多 5）
	Interest game.Value[int]
	// HP 玩家血量（0 = 阵亡）
	HP game.Value[int]

	// ---- 经济流 ----

	// WinStreak 胜场连胜
	WinStreak game.Value[int]
	// LoseStreak 败场连败
	LoseStreak game.Value[int]

	// ---- 棋盘 / 备战 / 商店 ----

	// Board 场上阵容
	Board game.Value[[]BoardSlot]
	// Bench 备战席
	Bench game.Value[[]BenchSlot]
	// Shop 商店刷新
	Shop game.Value[[]ShopSlot]
	// Items 已合成的装备 / 散件
	Items game.Value[[]ItemSlot]

	// ---- 状态 ----

	// Augments 已选强化符文
	Augments game.Value[[]AugmentSlot]
	// Traits 羁绊激活状态
	Traits game.Value[[]TraitState]
	// Carousel 选秀信息（选秀时非 nil；其他阶段为 Unknown）
	Carousel game.Value[*CarouselInfo]

	// ---- 对手 ----

	// Opponents 对手信息
	Opponents game.Value[[]Opponent]
}

// BoardSlot 棋盘上的一个英雄
type BoardSlot struct {
	// ChampionID Knowledge Base key（如 "TFT18_Vayne"）
	ChampionID string
	// Star 星级（1/2/3）
	Star int
	// Items 装备 key 列表
	Items []string
	// Position 棋盘格子编号（0~27）
	Position int
}

// BenchSlot 备战席上的一个英雄
type BenchSlot struct {
	ChampionID string
	Star       int
}

// ShopSlot 商店里的一个英雄
type ShopSlot struct {
	ChampionID string
	Cost       int
}

// ItemSlot 装备 / 散件槽
type ItemSlot struct {
	// ItemID Knowledge Base key（如 "TFT18_IE"）
	ItemID string
	// Count 同名散件数量（合成后为 1）
	Count int
}

// AugmentSlot 强化符文槽
type AugmentSlot struct {
	// AugmentID Knowledge Base key
	AugmentID string
	// Tier 等级（silver / gold / prismatic）
	Tier string
}

// TraitState 羁绊激活状态
type TraitState struct {
	// TraitID Knowledge Base key
	TraitID string
	// Active 已激活数
	Active int
	// Style 阶数（如 2/4/6/8）
	Style int
}

// CarouselInfo 选秀信息
type CarouselInfo struct {
	// Round 当前轮次
	Round string
	// Offerings 候选装备
	Offerings []string
	// PickedIndex 已选索引（>0；未选为 0）
	PickedIndex int
}

// Opponent 对手信息
type Opponent struct {
	PlayerName string
	HP         int
	Stage      string
	Board      []BoardSlot
}

// MergeFrom 实现 game.GameDataMerger 接口（Phase 10-7 增量更新）
//
// 合并规则：仅覆盖 other 中 Present=true 的 Value 字段
// 若 other 不是 *TFTGameContext 类型，直接整体替换指针
func (t *TFTGameContext) MergeFrom(other any) {
	if other == nil {
		return
	}
	src, ok := other.(*TFTGameContext)
	if !ok {
		return // 类型不匹配时不做任何修改（安全降级）
	}
	// 基础资源
	if src.Stage.IsKnown() {
		t.Stage = src.Stage
	}
	if src.Level.IsKnown() {
		t.Level = src.Level
	}
	if src.XP.IsKnown() {
		t.XP = src.XP
	}
	if src.XPTolevel.IsKnown() {
		t.XPTolevel = src.XPTolevel
	}
	if src.Gold.IsKnown() {
		t.Gold = src.Gold
	}
	if src.Interest.IsKnown() {
		t.Interest = src.Interest
	}
	if src.HP.IsKnown() {
		t.HP = src.HP
	}
	// 经济流
	if src.WinStreak.IsKnown() {
		t.WinStreak = src.WinStreak
	}
	if src.LoseStreak.IsKnown() {
		t.LoseStreak = src.LoseStreak
	}
	// 棋盘 / 备战 / 商店
	if src.Board.IsKnown() {
		t.Board = src.Board
	}
	if src.Bench.IsKnown() {
		t.Bench = src.Bench
	}
	if src.Shop.IsKnown() {
		t.Shop = src.Shop
	}
	if src.Items.IsKnown() {
		t.Items = src.Items
	}
	// 状态
	if src.Augments.IsKnown() {
		t.Augments = src.Augments
	}
	if src.Traits.IsKnown() {
		t.Traits = src.Traits
	}
	if src.Carousel.IsKnown() {
		t.Carousel = src.Carousel
	}
	// 对手
	if src.Opponents.IsKnown() {
		t.Opponents = src.Opponents
	}
}
