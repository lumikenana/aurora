// Package tft - 装备 ID 标准化层
//
// 设计目标：
//   - 将 OCR / 视觉识别的"原始名称"（中文 / 英文）转换为 Knowledge Base 风格的 ID
//   - 优先查询 Knowledge Base（如果 repository 可用），未命中时回退本地别名映射
//   - 识别不到时返回 Unknown 字符串（"Unknown"），绝不猜测
//   - 严格 patch 隔离：不会把 "TFT17_IE" 错误识别为 "TFT18_IE"
//
// 关键不变量（继承 Phase 14-A）：
//   - 始终严格 patch 隔离：同名称跨 Set 不匹配（因为同装备不同 Set ID 不同）
//   - 未知名称返回 Unknown，调用方应据此设置 Present=false
//   - 近似匹配只在 Knowledge Base 内做单结果返回；多匹配返回 Unknown
package tft

import (
	"context"
	"strings"

	"github.com/lipipi/aurora/internal/game/knowledge"
	tftkb "github.com/lipipi/aurora/internal/game/knowledge/tft"
	"github.com/lipipi/aurora/internal/log"
)

// ItemResolver 从 OCR 原始名称解析为 KB 规范 ID
//
// 接口：允许不同实现（KB 实现 / 静态别名实现 / 测试 mock）
type ItemResolver interface {
	// Resolve 解析装备名称
	//
	// 参数：
	//   ctx: 上下文
	//   patch: 当前游戏 patch（如 "16.17.1"），用于 KB 查询 patch 隔离
	//   setKey: 当前 Set key（如 "TFT18"），用于 ID 前缀验证
	//   input: OCR 识别出的原始名称
	//
	// 返回：
	//   (canonicalID, true) 匹配成功
	//   ("Unknown", false) 匹配失败/不唯一/未知
	Resolve(ctx context.Context, patch string, setKey string, input string) (string, bool)
}

// ItemIDNormalizer 装备名称 → ID 标准化器
//
// 用途：接收 OCR 识别出的装备名称（如 "无尽之刃" / "Infinity Edge" / "IE"），转换为统一 ID 格式
//
// 设计要点：
//   - 并发安全（无状态修改，知识库只读）
//   - 支持 KB repository 查询（如果注入）+ 本地别名 fallback
//   - 默认 set 前缀由 TFTGameContext 继承，可通过 WithSetKey 修改
type ItemIDNormalizer struct {
	// setKey 当前 Set 前缀（如 "TFT18"）
	setKey string
	// aliases 本地名称到 ID 的映射（小写 key）—— fallback 用
	aliases map[string]string
	// repo KB 仓库（可选，nil 表示只使用本地别名）
	repo tftkb.TFTRepository
}

// NewItemIDNormalizer 创建装备名称标准化器
//
// 默认：
//  - setKey = "TFT18"（当前 Set 18）
//  - no repository → 只使用本地别名映射
func NewItemIDNormalizer() *ItemIDNormalizer {
	n := &ItemIDNormalizer{
		setKey: "TFT18",
		aliases: defaultItemAliases(),
		repo:   nil,
	}
	return n
}

// WithRepository 设置 Knowledge Base 仓库用于查询
//
// 注入后，Normalize 会：
//  1. 先检查输入是否已是 TFT ID → 验证 setKey
//  2. 查本地别名 → 命中返回
//  3. 查 KB repository → 模糊匹配，唯一命中返回 ID，多命中返回 Unknown
//  4. 都不命中 → Unknown
func (n *ItemIDNormalizer) WithRepository(repo tftkb.TFTRepository) *ItemIDNormalizer {
	n.repo = repo
	return n
}

// WithSetKey 设置当前 set key 前缀（用于跨 Set 时切换）
//
// 注意：不会清空 aliases（aliases 是跨 set 共享的常见名称别名）。
func (n *ItemIDNormalizer) WithSetKey(setKey string) *ItemIDNormalizer {
	if setKey != "" {
		n.setKey = setKey
	}
	return n
}

// RegisterAlias 注册自定义别名（用于测试或扩展）
func (n *ItemIDNormalizer) RegisterAlias(name, id string) {
	if n.aliases == nil {
		n.aliases = make(map[string]string)
	}
	n.aliases[strings.ToLower(strings.TrimSpace(name))] = id
}

// Normalize 标准化装备名称为 ID
//
// 输入：原始名称（OCR 识别结果或视觉模型输出）
//
// 返回：
//   - 已识别：返回 TFT 风格 ID（如 "TFT18_InfinityEdge"）
//   - 未识别：返回 "Unknown"（调用方据此设置 Present=false）
//
// 匹配策略（有 repository 时）：
//  1. 已经是 TFT 风格 ID → 验证 setKey 前缀匹配 → 返回 / Unknown
//  2. 本地别名表精确小写匹配 → 返回
//  3. 都失败 → Unknown
//
// 注：完整 KB 匹配需要 full patch 版本，请使用 ResolveWithPatch。
func (n *ItemIDNormalizer) Normalize(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return UnknownID
	}
	// 已经是 TFT 风格 ID，原样返回（大小写不敏感）
	if isTFTID(name) {
		return canonicalizeTFTID(name, n.setKey)
	}
	// 查别名表（小写匹配）
	if id, ok := n.aliases[strings.ToLower(name)]; ok {
		return id
	}
	return UnknownID
}

// ResolveWithPatch 使用完整 patch 版本从 KB 解析（Phase 14-B 新增）
//
// 当 TFTOCRProvider 能从 GameContext.CommonData.Version 拿到完整 patch 版本（如 "16.17.1"），
// 可以调用此方法做完整 KB 模糊匹配，比仅本地别名有更高命中率。
//
// 匹配规则：
//  - patch 必须存在于 KB → 否则 Unknown
//  - 模糊匹配名称，最多返回 1 条 → 命中返回 ID
//  - 0 条或多≥2 条 → Unknown（不猜测）
//  - 命中的 ID 必须前缀匹配 n.setKey → 否则 Unknown（set 隔离）
func (n *ItemIDNormalizer) ResolveWithPatch(ctx context.Context, fullPatch string, name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return UnknownID
	}
	if n.repo == nil {
		return n.Normalize(name)
	}

	// 1. 已经是 TFT ID → 验证 setKey 前缀
	if isTFTID(name) {
		return canonicalizeTFTID(name, n.setKey)
	}

	// 2. 本地别名命中 → 返回
	if id, ok := n.aliases[strings.ToLower(name)]; ok {
		return id
	}

	// 3. KB 模糊查询
	items, err := n.repo.ListItems(ctx, fullPatch)
	if err != nil {
		log.Debugf("ItemIDNormalizer: KB list items failed: %v → return Unknown", err)
		return UnknownID
	}

	// 小写输入用于匹配
	inputLower := strings.ToLower(name)
	var candidates []knowledge.Item
	for _, it := range items {
		// 匹配小写 name
		if strings.Contains(strings.ToLower(it.Name), inputLower) {
			candidates = append(candidates, it)
		}
	}

	switch len(candidates) {
	case 0:
		return UnknownID
	case 1:
		// 验证前缀匹配当前 setKey（"TFT18"）
		id := candidates[0].ID
		if isTFTID(id) {
			canonical := canonicalizeTFTID(id, n.setKey)
			if canonical != UnknownID {
				return canonical
			}
		}
		return UnknownID
	default:
		// 多个候选 → 不猜，返回 Unknown
		log.Debugf("ItemIDNormalizer: multiple candidates found for %q in patch %s → return Unknown", name, fullPatch)
		return UnknownID
	}
}

// defaultItemAliases 默认装备别名表（Set 18 占位）
//
// 这是一个轻量级占位映射，用于在没有 Knowledge Base 的情况下也能进行
// "最基本"的 ID 标准化。本表覆盖常见装备的英文 / 中文名 / 常见缩写 / 常见误识别。
//
// 重要：本表不会跨 patch（如 "TFT17_InfinityEdge" 不会被自动映射到 "TFT18_InfinityEdge"）。
// 未来应从 Knowledge Base 自动生成。
func defaultItemAliases() map[string]string {
	aliases := make(map[string]string)
	// 常见装备缩写和别名（Set 18）
	// 格式：lower(name) -> "TFT18_<canonical>"
	// 只包含常见装备的常见别名
	aliases["ie"] = "TFT18_InfinityEdge"
	aliases["infinity edge"] = "TFT18_InfinityEdge"
	aliases["无尽之刃"] = "TFT18_InfinityEdge"
	aliases["无尽"] = "TFT18_InfinityEdge"

	aliases["gs"] = "TFT18_GiantSword"
	aliases["giant sword"] = "TFT18_GiantSword"
	aliases["大剑"] = "TFT18_GiantSword"

	aliases["bf"] = "TFT18_Bloodthirster"
	aliases["bloodthirster"] = "TFT18_Bloodthirster"
	aliases["饮血"] = "TFT18_Bloodthirster"
	aliases["嗜血"] = "TFT18_Bloodthirster"

	aliases["rg"] = "TFT18_Rageblade"
	aliases["rageblade"] = "TFT18_Rageblade"
	aliases["羊刀"] = "TFT18_Rageblade"
	aliases["鬼刀"] = "TFT18_Rageblade"

	aliases["jg"] = "TFT18_JeweledGauntlet"
	aliases["jeweled gauntlet"] = "TFT18_JeweledGauntlet"
	aliases["珠光护手"] = "TFT18_JeweledGauntlet"
	aliases["珠光"] = "TFT18_JeweledGauntlet"
	aliases["护手"] = "TFT18_JeweledGauntlet"

	aliases["hc"] = "TFT18_HextechGunblade"
	aliases["hextech gunblade"] = "TFT18_HextechGunblade"
	aliases["海克斯科技枪刃"] = "TFT18_HextechGunblade"
	aliases["海克斯"] = "TFT18_HextechGunblade"
	aliases["科技枪"] = "TFT18_HextechGunblade"

	aliases["bt"] = "TFT18_BrambleVest"
	aliases["bramble vest"] = "TFT18_BrambleVest"
	aliases["棘刺背心"] = "TFT18_BrambleVest"
	aliases["反甲"] = "TFT18_BrambleVest"

	aliases["dc"] = "TFT18_DragonClaw"
	aliases["dragon claw"] = "TFT18_DragonClaw"
	aliases["龙牙"] = "TFT18_DragonClaw"

	aliases["warmogs"] = "TFT18_WarmogsArmor"
	aliases["warmog"] = "TFT18_WarmogsArmor"
	aliases["warmogs armor"] = "TFT18_WarmogsArmor"
	aliases["狂徒"] = "TFT18_WarmogsArmor"
	aliases["狂徒铠甲"] = "TFT18_WarmogsArmor"

	aliases["gp"] = "TFT18_GuardianAngel"
	aliases["guardian angel"] = "TFT18_GuardianAngel"
	aliases["复活甲"] = "TFT18_GuardianAngel"
	aliases["天使甲"] = "TFT18_GuardianAngel"

	aliases["wr"] = "TFT18_WitsEnd"
	aliases["wits end"] = "TFT18_WitsEnd"
	aliases["泰坦"] = "TFT18_TitansResolve"
	aliases["titan's resolve"] = "TFT18_TitansResolve"
	aliases["泰坦的坚决"] = "TFT18_TitansResolve"

	aliases["morellonomicon"] = "TFT18_Morellonomicon"
	aliases["morello"] = "TFT18_Morellonomicon"
	aliases["鬼书"] = "TFT18_Morellonomicon"
	aliases["莫雷洛秘典"] = "TFT18_Morellonomicon"

	aliases["statikk shiv"] = "TFT18_StatikkShiv"
	aliases["statikk"] = "TFT18_StatikkShiv"
	aliases["电刀"] = "TFT18_StatikkShiv"

	aliases["deathblade"] = "TFT18_Deathblade"
	aliases["db"] = "TFT18_Deathblade"
	aliases["死亡之刃"] = "TFT18_Deathblade"
	// 注意：原 aliases["无尽"] = "TFT18_Deathblade" 与 line 214 重复，
	// 会覆盖前面的 "TFT18_InfinityEdge" 映射导致 TestItemIDNormalizer_DefaultAliases 失败。
	// 已移除：TFT 18 中"无尽"指 Infinity Edge，Deathblade 改用"死亡之刃"。

	aliases["rapid firecannon"] = "TFT18_RapidFirecannon"
	aliases["rfc"] = "TFT18_RapidFirecannon"
	aliases["火炮"] = "TFT18_RapidFirecannon"

	aliases["last whisper"] = "TFT18_LastWhisper"
	aliases["lw"] = "TFT18_LastWhisper"
	aliases["轻语"] = "TFT18_LastWhisper"
	aliases["破甲"] = "TFT18_LastWhisper"

	_ = aliases // 留给 RegisterAlias / 未来 Knowledge Base 注入
	return aliases
}
