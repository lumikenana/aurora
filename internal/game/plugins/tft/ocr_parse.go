// Package tft - OCR 文本解析器
//
// Phase 10-7 范围：
//   - 解析 OCR 输出为强类型值（int / string）
//   - 容错处理 OCR 常见错误（前/后空格、多余字符、字符误识别等）
//   - 范围校验（不合理的值直接拒绝，避免写入 GameContext）
//
// Phase 10-8 范围：
//   - 增加 XP 解析（数字）
//   - 增加 棋盘/备战/商店 格子文本解析（英雄名 + 星级 / 费用）
//   - 增加 装备 文本解析（item name + count）
//   - 增加 强化符文 文本解析（augment name + tier）
//   - 增加 羁绊 文本解析（trait name + count + style）
//   - 不直接做 ID 归一化（归一化在 Provider 层调用 ChampionIDNormalizer）
//
// 关键不变量：
//   - 0 是合法值（HP=0 表示阵亡，Gold=0 表示破产，ShopCost=0 表示无费用英雄）
//   - 返回 (val, true) 才表示成功解析
//   - 返回 (零值, false) 表示解析失败，调用方应写 Unknown
package tft

import (
	"regexp"
	"strconv"
	"strings"
)

// 范围常量
const (
	// GoldMax 合理金币上限（TFT 实际不会超过此值）
	GoldMax = 999
	// HPMax 合理血量上限
	HPMax = 100
	// LevelMin / LevelMax 等级范围
	LevelMin = 1
	LevelMax = 11
	// XPMax 合理 XP 上限（升级所需 XP 不会超过此值）
	XPMax = 200
	// ShopCostMax 英雄费用上限
	ShopCostMax = 5
	// StarMin / StarMax 星级范围
	StarMin = 1
	StarMax = 3
)

// --- 通用工具 ---

// extractDigits 提取字符串中的所有数字字符
//   "Gold: 42" -> "42"
//   " 67 " -> "67"
//   "O42" -> "42"
//   "-1"    -> "1"（负号被忽略，调用方需额外检查原文本是否包含 '-'）
//
// 注意：负数检测需调用方使用 hasLeadingMinus 辅助函数，
// 单独 extractDigits 会把 "-1" 提取为 "1"
func extractDigits(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// hasLeadingMinus 检查字符串是否包含前导负号
// 用于拒绝 OCR 误识别的负数（如 "-1"）
func hasLeadingMinus(s string) bool {
	s = strings.TrimSpace(s)
	return strings.HasPrefix(s, "-")
}

// normalizeWhitespace 合并连续空白为单空格并 trim
func normalizeWhitespace(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// parseIntInRange 通用整数解析 + 范围校验
//
// 拒绝负数；只接受纯数字文本（extractDigits 后结果）
// 范围 [minV, maxV] 闭区间
func parseIntInRange(text string, minV, maxV int) (int, bool) {
	if hasLeadingMinus(text) {
		return 0, false
	}
	digits := extractDigits(text)
	if digits == "" {
		return 0, false
	}
	v, err := strconv.Atoi(digits)
	if err != nil {
		return 0, false
	}
	if v < minV || v > maxV {
		return 0, false
	}
	return v, true
}

// --- Gold ---

// parseGold 解析金币文本
//
// 期望：纯数字或带 "Gold:" 前缀的文本
// 范围：0 ~ GoldMax
func parseGold(text string) (int, bool) {
	return parseIntInRange(text, 0, GoldMax)
}

// --- HP ---

// parseHP 解析血量文本
//
// 期望：纯数字
// 范围：0 ~ HPMax（0 = 阵亡，合法）
func parseHP(text string) (int, bool) {
	return parseIntInRange(text, 0, HPMax)
}

// --- Level ---

// parseLevel 解析等级文本
//
// 期望：纯数字或带 "Level" 前缀
// 范围：LevelMin ~ LevelMax
func parseLevel(text string) (int, bool) {
	return parseIntInRange(text, LevelMin, LevelMax)
}

// --- XP (Phase 10-8) ---

// parseXP 解析 XP 文本
//
// 期望：纯数字
// 范围：0 ~ XPMax
func parseXP(text string) (int, bool) {
	return parseIntInRange(text, 0, XPMax)
}

// --- Stage ---

// stagePattern 阶段格式：N-M（N 是阶段数，M 是回合内子阶段）
var stagePattern = regexp.MustCompile(`(\d+)\s*-\s*(\d+)`)

// parseStage 解析阶段文本
//
// 期望："4-2" / "4 - 2" / "Stage 4-2" 等
// 返回：归一化的 "N-M" 字符串
func parseStage(text string) (string, bool) {
	m := stagePattern.FindStringSubmatch(text)
	if m == nil || len(m) < 3 {
		return "", false
	}
	// 验证范围
	n, err1 := strconv.Atoi(m[1])
	sub, err2 := strconv.Atoi(m[2])
	if err1 != nil || err2 != nil {
		return "", false
	}
	if n < 1 || n > 10 || sub < 1 || sub > 9 {
		return "", false
	}
	return m[1] + "-" + m[2], true
}

// --- Star (Phase 10-8) ---

// parseStar 解析星级
//
// 期望：1/2/3（可带前缀 "★" / "Star"）
// 范围：StarMin ~ StarMax
func parseStar(text string) (int, bool) {
	return parseIntInRange(text, StarMin, StarMax)
}

// --- ShopCost (Phase 10-8) ---

// parseShopCost 解析英雄费用
//
// 期望：1~5
// 范围：1 ~ ShopCostMax
func parseShopCost(text string) (int, bool) {
	return parseIntInRange(text, 1, ShopCostMax)
}

// --- ChampionName (Phase 10-8) ---

// parseChampionName 解析英雄名称（OCR 文本）
//
// 本函数不做 ID 归一化（归一化由 Provider 层调用 ChampionIDNormalizer）。
// 只负责：
//   - trim 空白
//   - 拒绝空字符串
//   - 拒绝纯数字（不是英雄名）
//
// 返回：原始名称（供 Provider 层做归一化）
func parseChampionName(text string) (string, bool) {
	text = normalizeWhitespace(text)
	if text == "" {
		return "", false
	}
	// 拒绝纯数字（OCR 误识别为纯数字不是英雄名）
	allDigits := true
	for _, r := range text {
		if r < '0' || r > '9' {
			allDigits = false
			break
		}
	}
	if allDigits {
		return "", false
	}
	return text, true
}

// --- BoardCell with Items (Phase 14-B) ---

// parseBoardCellText 解析棋盘格子文本（包含英雄+星级+装备）
//
// 期望格式：
//   - "ChampionName" → (name, 1, [], true)
//   - "ChampionName | 2" → (name, 2, [], true)
//   - "ChampionName | 2 | IE/Rageblade" → (name, 2, ["IE", "Rageblade"], true)
//   - "ChampionName | 2 | IE / Rageblade" → (name, 2, ["IE", "Rageblade"], true)
//   - "ChampionName ★2 | IE Rageblade" → (name, 2, ["IE", "Rageblade"], true)
//
// 装备可以用 '/' 分隔或换行分隔
func parseBoardCellText(text string) (string, int, []string, bool) {
	text = normalizeWhitespace(text)
	if text == "" {
		return "", 0, nil, false
	}

	// 按 '|' 分割多段
	parts := strings.Split(text, "|")
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}

	// 过滤空部分
	nonEmpty := make([]string, 0, 3)
	for _, p := range parts {
		if p != "" {
			nonEmpty = append(nonEmpty, p)
		}
	}

	switch len(nonEmpty) {
	case 0:
		return "", 0, nil, false
	case 1:
		// 只有名称 → 星级 1，无装备
		name, ok := parseChampionName(nonEmpty[0])
		if !ok {
			return "", 0, nil, false
		}
		return name, 1, []string{}, true
	case 2:
		// 名称 + 星级 或者 名称 + 装备（一行装备）
		name, ok := parseChampionName(nonEmpty[0])
		if !ok {
			return "", 0, nil, false
		}
		// 尝试解析第二部分为星级
		if star, ok := parseStar(nonEmpty[1]); ok {
			// 名称 + 星级 → 无装备
			return name, star, []string{}, true
		}
		// 第二部分不是星级 → 当作装备列表（用 '/' 分割）
		items := splitItems(nonEmpty[1])
		return name, 1, items, true
	default:
		// 名称 + 星级 + 装备
		name, ok := parseChampionName(nonEmpty[0])
		if !ok {
			return "", 0, nil, false
		}
		star, ok := parseStar(nonEmpty[1])
		if !ok {
			// 第二部分不是星级 → 当作装备，星级默认为 1
			items := splitItems(strings.Join(nonEmpty[1:], " "))
			return name, 1, items, true
		}
		// 第三部分是装备
		items := splitItems(nonEmpty[2])
		return name, star, items, true
	}
}

// splitItems 分割装备列表
//
// 支持：
//   - '/' 分隔 ("IE/Rageblade" → ["IE", "Rageblade"])
//   - 空格分隔 ("IE Rageblade" → ["IE", "Rageblade"])
//   - 换行分隔 ("IE\nRageblade" → ["IE", "Rageblade"])
func splitItems(text string) []string {
	text = strings.TrimSpace(text)
	if text == "" {
		return []string{}
	}

	// 如果有 '/'，按 '/' 分割
	if strings.Contains(text, "/") {
		items := strings.Split(text, "/")
		result := make([]string, 0, len(items))
		for _, it := range items {
			it = strings.TrimSpace(it)
			if it != "" {
				result = append(result, it)
			}
		}
		return result
	}

	// 否则按空白分割（换行或多个空格）
	items := strings.Fields(text)
	result := make([]string, 0, len(items))
	for _, it := range items {
		it = strings.TrimSpace(it)
		if it != "" {
			result = append(result, it)
		}
	}
	return result
}

// --- AugmentTier (Phase 10-8) ---

// parseAugmentTier 解析强化符文等级
//
// 期望：silver / gold / prismatic（不区分大小写）
// 返回：归一化的小写字符串
func parseAugmentTier(text string) (string, bool) {
	text = strings.ToLower(normalizeWhitespace(text))
	switch text {
	case "silver":
		return "silver", true
	case "gold":
		return "gold", true
	case "prismatic", "prism":
		return "prismatic", true
	}
	return "", false
}

// --- TraitStyle (Phase 10-8) ---

// traitNumberPattern 匹配单个数字（用于解析 "2/4/6" 等样式）
var traitNumberPattern = regexp.MustCompile(`\d+`)

// parseTraitLine 解析一行羁绊文本
//
// 期望：包含 "2/4" / "2/4/6" / "1/2/3/4" 等样式（任意层数）
// 返回：(active, style, ok)
//   - active: 当前激活数（第一个数字）
//   - style: 阶数（链中最后一个数字）
//   - ok: 是否成功解析
func parseTraitLine(text string) (active, style int, ok bool) {
	matches := traitNumberPattern.FindAllString(text, -1)
	if len(matches) < 2 {
		return 0, 0, false
	}
	a, err1 := strconv.Atoi(matches[0])
	if err1 != nil {
		return 0, 0, false
	}
	// 阶数取链中最后一个（最高 tier）
	s, err2 := strconv.Atoi(matches[len(matches)-1])
	if err2 != nil {
		return 0, 0, false
	}
	if a < 0 || s < 0 || a > 99 || s > 99 {
		return 0, 0, false
	}
	// active 不能超过 style（激活数不可能 > 目标阶数）
	if a > s {
		return 0, 0, false
	}
	return a, s, true
}
