// Package economy - Format（Phase 16-B）
//
// EconomyAnalysis → 紧凑 prompt 片段。
// 0 LLM 调用；纯本地结构化输出。
package economy

import (
	"fmt"
	"strings"
)

// Format 将 EconomyAnalysis 格式化为紧凑 prompt 片段
//
// 空数据约定：所有关键字段都是 0 时返回 ""（不输出）
// 这避免对 LLM 注入"经济状态未知"的假数据。
func Format(ea EconomyAnalysis) string {
	if ea.Gold == 0 && ea.Level == 0 && ea.HP == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("## 经济分析（本地结构化）\n\n")

	sb.WriteString("### 当前状态\n")
	if ea.Stage != "" {
		sb.WriteString(fmt.Sprintf("- 阶段：%s / %s\n", ea.Stage, ea.Phase))
	}
	sb.WriteString(fmt.Sprintf("- 等级：%d\n", ea.Level))
	if ea.XPToLevel > 0 {
		sb.WriteString(fmt.Sprintf("- 经验：%d/%d（%.1f%%）\n", ea.XP, ea.XP+ea.XPToLevel, ea.XPPercent))
	}
	sb.WriteString(fmt.Sprintf("- 金币：%d\n", ea.Gold))
	sb.WriteString(fmt.Sprintf("- 利息：%d / 最大 %d\n", ea.Interest, ea.InterestMax))
	if ea.GoldToInterest > 0 {
		sb.WriteString(fmt.Sprintf("- 距离下一点利息还需要：%d 金币\n", ea.GoldToInterest))
	}
	sb.WriteString(fmt.Sprintf("- 生命值：%d\n", ea.HP))
	switch {
	case ea.WinStreak > 0:
		sb.WriteString(fmt.Sprintf("- %d 连胜\n", ea.WinStreak))
	case ea.LoseStreak > 0:
		sb.WriteString(fmt.Sprintf("- %d 连败\n", ea.LoseStreak))
	default:
		sb.WriteString("- 无连胜/连败\n")
	}

	return sb.String()
}
