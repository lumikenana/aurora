// Package grounder - Format(GroundedGameContext) → string（prompt 渲染器）
//
// 设计要点：
//   - 输出结构化 Markdown 段落，可直接拼入 system prompt
//   - 顶部 patch / setKey / KB 状态元信息
//   - WrongSet 实体顶部显式警告（但仍渲染 ID，不静默替换）
//   - Unknown / KBUnavailable 各有明确标注
//   - 总长度 ≤ 3000 tokens；超限按"商店 → 强化符文"优先级截断
//   - 永不丢弃棋盘 / 备战 / 羁绊（核心决策输入）
package grounder

import (
	"fmt"
	"strings"
)

// FormatMaxTokens 单次 GroundedGameContext 渲染硬上限
const FormatMaxTokens = 3000

// 估算：1 token ≈ 4 个英文字符 / 1.5 个中文字符。混合内容按 2 字符/token 近似。
const charsPerToken = 2

// Format 把 GroundedGameContext 渲染为可注入 system prompt 的 string
//
// 返回值约定：
//   - 输入完全为空（GameState 透传 0 + 全部槽位未 Known）→ ""
//   - 任何已知内容存在 → 至少 1 行（"## 知识库对照 …"）
func Format(g GroundedGameContext) string {
	if g.IsEmpty() {
		return ""
	}

	var sb strings.Builder
	writeHeader(&sb, g)

	// 顶部 WrongSet 警告
	if g.HasWrongSet() {
		writeWrongSetWarnings(&sb, g)
	}

	// GameState
	if g.GameState.HasAny() {
		sb.WriteString("\n### 对局资源\n")
		writeGameState(&sb, g.GameState)
	}

	// 核心：棋盘 / 备战 / 羁绊（永不截断）
	if len(g.Board) > 0 {
		sb.WriteString("\n### 棋盘\n")
		writeBoard(&sb, g.Board)
	}
	if len(g.Bench) > 0 {
		sb.WriteString("\n### 备战席\n")
		writeBench(&sb, g.Bench)
	}
	if len(g.Traits) > 0 {
		sb.WriteString("\n### 羁绊\n")
		writeTraits(&sb, g.Traits)
	}

	// 次要：装备 / 商店 / 强化（按需截断）
	if len(g.Items) > 0 {
		sb.WriteString("\n### 装备/散件\n")
		writeItems(&sb, g.Items)
	}
	if len(g.Shop) > 0 {
		sb.WriteString("\n### 商店\n")
		writeShop(&sb, g.Shop)
	}
	if len(g.Augments) > 0 {
		sb.WriteString("\n### 强化符文\n")
		writeAugments(&sb, g.Augments)
	}

	// 截断检查
	return truncateToBudget(sb.String())
}

// IsEmpty 报告 GroundedGameContext 是否完全无内容
func (g GroundedGameContext) IsEmpty() bool {
	_, bench, shop, items, traits, augs := g.Counts()
	return g.GameState.IsEmpty() &&
		len(g.Board) == 0 && bench == 0 && shop == 0 &&
		items == 0 && traits == 0 && augs == 0 &&
		g.Meta.Patch == "" && g.Meta.SetKey == ""
}

// ---- 区块渲染 ----

func writeHeader(sb *strings.Builder, g GroundedGameContext) {
	kbStatus := "ok"
	if !g.Meta.KBHasData {
		kbStatus = "no_data"
	}
	fallbackTag := ""
	if g.Meta.PatchFallback {
		fallbackTag = " (fallback)"
	}
	patch := g.Meta.Patch
	if patch == "" {
		patch = "?"
	}
	setKey := g.Meta.SetKey
	if setKey == "" {
		setKey = "?"
	}
	fmt.Fprintf(sb, "## 知识库对照（patch=%s, set=%s, KB=%s%s）",
		patch, setKey, kbStatus, fallbackTag)
}

func writeWrongSetWarnings(sb *strings.Builder, g GroundedGameContext) {
	sb.WriteString("\n\n⚠️ 以下 ID 与当前 set 前缀不匹配（仅作提示，不替换）：\n")
	for i, b := range g.Board {
		if b.Champion.Status == StatusWrongSet {
			fmt.Fprintf(sb, "  - 棋盘[%d] %s [wrong_set: 当前 set 不包含此 ID]\n", i, b.Champion.ID)
		}
		for _, it := range b.Items {
			if it.Status == StatusWrongSet {
				fmt.Fprintf(sb, "  - 棋盘[%d] 装备 %s [wrong_set]\n", i, it.ID)
			}
		}
	}
	for i, b := range g.Bench {
		if b.Champion.Status == StatusWrongSet {
			fmt.Fprintf(sb, "  - 备战[%d] %s [wrong_set]\n", i, b.Champion.ID)
		}
	}
	for i, s := range g.Shop {
		if s.Champion.Status == StatusWrongSet {
			fmt.Fprintf(sb, "  - 商店[%d] %s [wrong_set]\n", i, s.Champion.ID)
		}
	}
	for i, it := range g.Items {
		if it.Item.Status == StatusWrongSet {
			fmt.Fprintf(sb, "  - 装备槽[%d] %s [wrong_set]\n", i, it.Item.ID)
		}
	}
	for i, t := range g.Traits {
		if t.Trait.Status == StatusWrongSet {
			fmt.Fprintf(sb, "  - 羁绊[%d] %s [wrong_set]\n", i, t.Trait.ID)
		}
	}
	for i, a := range g.Augments {
		if a.Augment.Status == StatusWrongSet {
			fmt.Fprintf(sb, "  - 强化[%d] %s [wrong_set]\n", i, a.Augment.ID)
		}
	}
}

func writeGameState(sb *strings.Builder, s GroundedGameState) {
	if s.Stage != "" {
		fmt.Fprintf(sb, "- 阶段: %s\n", s.Stage)
	}
	if s.Level > 0 || s.XP > 0 {
		fmt.Fprintf(sb, "- 等级: Lv.%d  XP=%d/%d\n", s.Level, s.XP, s.XPTolevel)
	}
	if s.Gold > 0 || s.Interest > 0 {
		fmt.Fprintf(sb, "- 经济: %dg (利息 %d)\n", s.Gold, s.Interest)
	}
	if s.HP > 0 {
		fmt.Fprintf(sb, "- 血量: %d\n", s.HP)
	}
	if s.WinStreak > 0 || s.LoseStreak > 0 {
		if s.WinStreak > 0 {
			fmt.Fprintf(sb, "- 连胜: %d\n", s.WinStreak)
		}
		if s.LoseStreak > 0 {
			fmt.Fprintf(sb, "- 连败: %d\n", s.LoseStreak)
		}
	}
}

func writeBoard(sb *strings.Builder, board []GroundedBoardSlot) {
	for i, s := range board {
		star := ""
		if s.Star > 0 {
			star = fmt.Sprintf("★%d ", s.Star)
		}
		pos := ""
		if s.Position > 0 {
			pos = fmt.Sprintf(" (pos=%d)", s.Position)
		}
		fmt.Fprintf(sb, "- [%d] %s%s%s\n", i, star, renderChampion(s.Champion), pos)
		for _, it := range s.Items {
			fmt.Fprintf(sb, "    · %s\n", renderItem(it))
		}
	}
}

func writeBench(sb *strings.Builder, bench []GroundedBenchSlot) {
	for i, s := range bench {
		star := ""
		if s.Star > 0 {
			star = fmt.Sprintf("★%d ", s.Star)
		}
		fmt.Fprintf(sb, "- [%d] %s%s\n", i, star, renderChampion(s.Champion))
	}
}

func writeTraits(sb *strings.Builder, traits []GroundedTraitState) {
	for i, t := range traits {
		fmt.Fprintf(sb, "- [%d] %s (active=%d style=%d)\n", i, renderTrait(t.Trait), t.Active, t.Style)
	}
}

func writeItems(sb *strings.Builder, items []GroundedItemSlot) {
	for i, it := range items {
		count := ""
		if it.Count > 0 {
			count = fmt.Sprintf(" ×%d", it.Count)
		}
		fmt.Fprintf(sb, "- [%d] %s%s\n", i, renderItem(it.Item), count)
	}
}

func writeShop(sb *strings.Builder, shop []GroundedShopSlot) {
	for i, s := range shop {
		fmt.Fprintf(sb, "- [%d] %s (cost=%d)\n", i, renderChampion(s.Champion), s.Cost)
	}
}

func writeAugments(sb *strings.Builder, augs []GroundedAugmentSlot) {
	for i, a := range augs {
		tier := ""
		if a.Tier != "" {
			tier = fmt.Sprintf(" [%s]", a.Tier)
		}
		fmt.Fprintf(sb, "- [%d]%s %s\n", i, tier, renderAugment(a.Augment))
	}
}

// ---- 单实体渲染 ----

func renderChampion(c ResolvedChampion) string {
	base := c.ID
	if c.Name != "" {
		base = c.Name
	}
	tail := statusTag(c.Status)

	var extras []string
	if c.Status == StatusResolved {
		if c.Cost > 0 {
			extras = append(extras, fmt.Sprintf("cost=%d", c.Cost))
		}
		if len(c.Traits) > 0 {
			extras = append(extras, "traits="+strings.Join(c.Traits, ","))
		}
		if c.Role != "" {
			extras = append(extras, "role="+c.Role)
		}
	}
	if len(extras) > 0 {
		return fmt.Sprintf("%s [%s] %s", base, strings.Join(extras, ", "), tail)
	}
	return fmt.Sprintf("%s %s", base, tail)
}

func renderItem(it ResolvedItem) string {
	base := it.ID
	if it.Name != "" {
		base = it.Name
	}
	tail := statusTag(it.Status)

	var extras []string
	if it.Status == StatusResolved {
		if it.Description != "" {
			extras = append(extras, truncateStr(it.Description, 80))
		}
		if len(it.From) > 0 {
			extras = append(extras, "from="+strings.Join(it.From, "+"))
		}
	}
	if len(extras) > 0 {
		return fmt.Sprintf("%s — %s %s", base, strings.Join(extras, "; "), tail)
	}
	return fmt.Sprintf("%s %s", base, tail)
}

func renderTrait(t ResolvedTrait) string {
	base := t.ID
	if t.Name != "" {
		base = t.Name
	}
	tail := statusTag(t.Status)

	if t.Status == StatusResolved && t.Type != "" {
		return fmt.Sprintf("%s (%s) %s", base, t.Type, tail)
	}
	return fmt.Sprintf("%s %s", base, tail)
}

func renderAugment(a ResolvedAugment) string {
	base := a.ID
	if a.Name != "" {
		base = a.Name
	}
	tail := statusTag(a.Status)

	if a.Status == StatusResolved && a.Description != "" {
		return fmt.Sprintf("%s — %s %s", base, truncateStr(a.Description, 80), tail)
	}
	return fmt.Sprintf("%s %s", base, tail)
}

func statusTag(s MatchStatus) string {
	switch s {
	case StatusResolved:
		return ""
	case StatusUnknown:
		return "[unknown: 名称未识别]"
	case StatusWrongSet:
		return "[wrong_set: 当前 set 不包含此 ID]"
	case StatusKBUnavailable:
		return "[kb_unavailable]"
	case StatusSkipped:
		return "[skipped]"
	default:
		return fmt.Sprintf("[%s]", s)
	}
}

// ---- 工具 ----

// HasAny 报告 GameState 是否有任一字段被透传
func (s GroundedGameState) HasAny() bool {
	return s.Stage != "" || s.Level > 0 || s.XP > 0 || s.Gold > 0 ||
		s.HP > 0 || s.WinStreak > 0 || s.LoseStreak > 0 || s.Interest > 0
}

// IsEmpty 报告 GameState 是否完全空
func (s GroundedGameState) IsEmpty() bool { return !s.HasAny() }

func truncateStr(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-1] + "…"
}

// truncateToBudget 按 3000 token 上限截断，按"商店 → 强化"优先级砍段
func truncateToBudget(s string) string {
	const maxChars = FormatMaxTokens * charsPerToken
	if len(s) <= maxChars {
		return s
	}

	// 找出"### 商店" / "### 强化符文" 段落
	// 简化：直接按段落丢弃（先强化后商店）
	candidates := []string{"\n### 强化符文\n", "\n### 商店\n", "\n### 装备/散件\n"}
	for _, marker := range candidates {
		idx := strings.Index(s, marker)
		if idx < 0 {
			continue
		}
		// 找该段结束（下一个 ### 或末尾）
		rest := s[idx+1:]
		nextSection := -1
		for _, m := range []string{"\n### ", "\n## "} {
			if i := strings.Index(rest[1:], m); i >= 0 {
				if nextSection < 0 || i < nextSection {
					nextSection = i
				}
			}
		}
		end := len(s)
		if nextSection >= 0 {
			end = idx + 1 + nextSection
		}
		// 截掉该段
		s = s[:idx] + s[end:]
		if len(s) <= maxChars {
			s += "\n[truncated for length: 移除非核心段以满足 token 预算]"
			return s
		}
	}
	// 还超 → 强制末尾截断
	if len(s) > maxChars {
		s = s[:maxChars-50] + "\n[truncated for length]\n"
	}
	return s
}
