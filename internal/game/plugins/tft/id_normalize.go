// Package tft - 轻量级 ID 标准化层
//
// 设计目标：
//   - 将 OCR / 视觉识别的"原始名称"（中文 / 英文）转换为 Knowledge Base 风格的 ID
//   //     如 "TFT18_Vayne"）
//   - 优先查询 Knowledge Base（如果 repository 可用），未命中时回退本地别名映射
//   - 识别不到时返回 Unknown 字符串（"Unknown"），绝不猜测
//   - 严格 patch 隔离：不会把 "TFT17_Vayne" 错误识别为 "TFT18_Vayne"
//
// 关键不变量（Phase 14-A）：
//   - 始终严格 patch 隔离：同名称跨 Set 不匹配（因为同英雄不同 Set ID 不同）
//   - 未知名称返回 Unknown，调用方应写为 Value[T]{Present: false}
//   - 近似匹配只在 Knowledge Base 内做单结果返回；多匹配返回 Unknown
package tft

import (
	"context"
	"strings"

	"github.com/lipipi/aurora/internal/game/knowledge"
	tftkb "github.com/lipipi/aurora/internal/game/knowledge/tft"
	"github.com/lipipi/aurora/internal/log"
)

// ChampionResolver 从 OCR 原始名称解析为 KB 规范 ID
//
// 接口：允许不同实现（KB 实现 / 静态别名实现 / 测试 mock）
type ChampionResolver interface {
	// Resolve 解析英雄名称
	//
	// 参数：
	//   ctx: 上下文
	//   patch: 当前游戏 patch（如 "16.17.1"），用于 KB 查询 patch 隔离
	//   input: OCR 识别出的原始名称
	//
	// 返回：
	//   (canonicalID, true) 匹配成功
	//   ("Unknown", false) 匹配失败/不唯一/未知
	Resolve(ctx context.Context, patch string, input string) (string, bool)
}

// ChampionIDNormalizer 英雄名称 → ID 标准化器
//
// 用途：接收 OCR 识别出的英雄名称（如 "Vayne" / "薇恩"），转换为统一 ID 格式
//
// 设计要点：
//   - 并发安全（无状态修改，知识库只读）
//   - 支持 KB repository 查询（如果注入）+ 本地别名 fallback
//   - 默认 patch 前缀由 TFTGameContext 继承，可通过 WithPatch 修改
type ChampionIDNormalizer struct {
	// patch 当前 Set/Patch 前缀（如 "TFT18"）
	patch string
	// aliases 本地名称到 ID 的映射（小写 key）—— fallback 用
	aliases map[string]string
	// repo KB 仓库（可选，nil 表示只使用本地别名）
	repo tftkb.TFTRepository
}

// NewChampionIDNormalizer 创建英雄名称标准化器
//
// 默认：
//  - patch = "TFT18"（当前 Set 18）
//  - no repository → 只使用本地别名映射
func NewChampionIDNormalizer() *ChampionIDNormalizer {
	n := &ChampionIDNormalizer{
		patch:   "TFT18",
		aliases: defaultChampionAliases(),
		repo:    nil,
	}
	return n
}

// WithRepository 设置 Knowledge Base 仓库用于查询
//
// 注入后，Normalize 会：
//  1. 先检查输入是否已是 TFT ID → 验证 patch
//  2. 查本地别名 → 命中返回
//  3. 查 KB repository → 模糊匹配，唯一命中返回 ID，多命中返回 Unknown
//  4. 都不命中 → Unknown
func (n *ChampionIDNormalizer) WithRepository(repo tftkb.TFTRepository) *ChampionIDNormalizer {
	n.repo = repo
	return n
}

// WithPatch 设置当前 patch 前缀（用于跨 Set 时切换）
//
// 注意：不会清空 aliases（aliases 是跨 patch 共享的常见名称）。
func (n *ChampionIDNormalizer) WithPatch(patch string) *ChampionIDNormalizer {
	if patch != "" {
		n.patch = patch
	}
	return n
}

// RegisterAlias 注册自定义别名（用于测试或扩展）
func (n *ChampionIDNormalizer) RegisterAlias(name, id string) {
	if n.aliases == nil {
		n.aliases = make(map[string]string)
	}
	n.aliases[strings.ToLower(strings.TrimSpace(name))] = id
}

// Normalize 标准化英雄名称为 ID
//
// 输入：原始名称（OCR 识别结果或视觉模型输出）
//
// 返回：
//   - 已识别：返回 TFT 风格 ID（如 "TFT18_Vayne"）
//   - 未识别：返回 "Unknown"（调用方据此设置 Present=false）
//
// 匹配策略（有 repository 时）：
//  1. 已经是 TFT 风格 ID → 验证 patch 前缀匹配 → 返回 / Unknown
//  2. 本地别名表精确小写匹配 → 返回
//  3. 都失败 → Unknown
//
// 注：完整 KB 匹配需要 full patch 版本，请使用 ResolveWithPatch。
func (n *ChampionIDNormalizer) Normalize(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return UnknownID
	}
	// 已经是 TFT 风格 ID，原样返回（大小写不敏感）
	if isTFTID(name) {
		return canonicalizeTFTID(name, n.patch)
	}
	// 查别名表（小写匹配）
	if id, ok := n.aliases[strings.ToLower(name)]; ok {
		return id
	}
	return UnknownID
}

// ResolveWithPatch 使用完整 patch 版本从 KB 解析（Phase 14-A 新增）
//
// 当 TFTOCRProvider 能从 GameContext.CommonData.Version 拿到完整 patch 版本（如 "16.17.1"），
// 可以调用此方法做完整 KB 模糊匹配，比仅本地别名有更高命中率。
//
// 匹配规则：
//  - patch 必须存在于 KB → 否则 Unknown
//  - 模糊匹配名称，最多返回 1 条 → 命中返回 ID
//  - 0 条或多≥2 条 → Unknown（不猜测）
//  - 命中的 ID 必须前缀匹配 n.patch → 否则 Unknown（patch 隔离）
func (n *ChampionIDNormalizer) ResolveWithPatch(ctx context.Context, fullPatch string, name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return UnknownID
	}
	if n.repo == nil {
		return n.Normalize(name)
	}

	// 1. 已经是 TFT ID → 验证 patch 前缀
	if isTFTID(name) {
		return canonicalizeTFTID(name, n.patch)
	}

	// 2. 本地别名命中 → 返回
	if id, ok := n.aliases[strings.ToLower(name)]; ok {
		return id
	}

	// 3. KB 模糊查询
	champions, err := n.repo.ListChampions(ctx, fullPatch)
	if err != nil {
		log.Debugf("ChampionIDNormalizer: KB list champions failed: %v → return Unknown", err)
		return UnknownID
	}

	// 小写输入用于匹配
	inputLower := strings.ToLower(name)
	var candidates []knowledge.Champion
	for _, c := range champions {
		// 匹配小写 name
		if strings.Contains(strings.ToLower(c.Name), inputLower) {
			candidates = append(candidates, c)
		}
	}

	switch len(candidates) {
	case 0:
		return UnknownID
	case 1:
		// 验证前缀匹配当前 patch（"TFT18"）
		id := candidates[0].ID
		if isTFTID(id) {
			canonical := canonicalizeTFTID(id, n.patch)
			if canonical != UnknownID {
				return canonical
			}
		}
		return UnknownID
	default:
		// 多个候选 → 不猜，返回 Unknown
		log.Debugf("ChampionIDNormalizer: multiple candidates found for %q in patch %s → return Unknown", name, fullPatch)
		return UnknownID
	}
}

// UnknownID 未识别时返回的字符串
const UnknownID = "Unknown"

// isTFTID 检查字符串是否符合 TFT ID 格式
//
// 形如 "TFT18_Vayne" / "TFT17_Ahri" / "TFTSet18_Vayne"
func isTFTID(s string) bool {
	// 必须包含下划线
	idx := strings.Index(s, "_")
	if idx <= 0 || idx >= len(s)-1 {
		return false
	}
	// 前缀必须以 "TFT" 开头（允许 "TFT18" / "TFTSet18" / "TFT18b"）
	prefix := s[:idx]
	upper := strings.ToUpper(prefix)
	return strings.HasPrefix(upper, "TFT")
}

// canonicalizeTFTID 规范化已识别的 TFT ID
//
// 如果 patch 前缀与当前 patch 不一致，返回 Unknown（防止跨 patch 误识别）
// 例如：当前 patch = "TFT18"，输入 "TFT17_Vayne" → "Unknown"
// （同英雄跨 Set 表现可能完全不同，不能复用 ID）
func canonicalizeTFTID(id, currentPatch string) string {
	idx := strings.Index(id, "_")
	if idx <= 0 {
		return UnknownID
	}
	prefix := id[:idx]
	// 比较前缀（大小写不敏感）
	if !strings.EqualFold(prefix, currentPatch) {
		return UnknownID
	}
	return id
}

// defaultChampionAliases 默认英雄别名表（Set 18 占位）
//
// 这是一个轻量级占位映射，用于在没有 Knowledge Base 的情况下也能进行
// "最基本"的 ID 标准化。本表覆盖常见英雄的英文 / 中文名 / 常见误识别。
//
// 重要：本表不会跨 patch（如 "TFT17_Vayne" 不会被自动映射到 "TFT18_Vayne"）。
// 未来应从 Knowledge Base 自动生成。
func defaultChampionAliases() map[string]string {
	aliases := make(map[string]string)
	// 常见英雄（Set 18 占位）—— 实际填充依赖 Knowledge Base
	// 注：本阶段不强行填充大量英雄名（避免在缺失时假装识别成功）
	// 只包含"明显已知"的英雄，且 patch 前缀必须为当前 Set
	// 格式：lower(name) -> "TFT18_<canonical>"
	_ = aliases // 留给 RegisterAlias / 未来 Knowledge Base 注入
	return aliases
}
