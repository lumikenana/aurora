// Package equipment - Format（Phase 16-B）
//
// EquipmentAnalysis → 紧凑 prompt 片段。
// 0 LLM 调用；纯本地结构化输出。
//
// 设计要点：
//   - ChampionEquipment.IsCore 由 CompositionAnalysis 决定（上游已算）
//   - 不在此处"推荐装备"或"判断装备是否合适"（Phase 17+）
//   - 装备分类（Completed/Component/Unknown）由 ItemType 字段决定（KB-driven）
package equipment

import (
	"fmt"
	"strings"
)

// Format 将 EquipmentAnalysis 格式化为紧凑 prompt 片段
//
// 空数据约定：ItemIndex 为空时返回 ""（不输出）。
// 这避免对 LLM 注入"装备完全缺失"的假数据。
func Format(ea EquipmentAnalysis) string {
	if ea.ItemIndex == nil || len(ea.ItemIndex) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("## 装备分析（本地结构化）\n\n")

	// Summary
	sb.WriteString("### 全局统计\n")
	sb.WriteString(fmt.Sprintf("- 总装备：%d\n", ea.Summary.TotalItems))
	sb.WriteString(fmt.Sprintf("- 独特装备：%d\n", ea.Summary.UniqueItems))
	sb.WriteString(fmt.Sprintf("- 成装：%d / 散件：%d / 未知：%d\n",
		ea.Summary.CompletedCount, ea.Summary.ComponentCount, ea.Summary.UnknownCount))
	if ea.Summary.CoreChampionsWithFullItems > 0 {
		sb.WriteString(fmt.Sprintf("- 装备齐全核心：%d\n", ea.Summary.CoreChampionsWithFullItems))
	}
	if ea.Summary.ItemKBStatus != "" {
		sb.WriteString(fmt.Sprintf("- KB状态：%s\n", ea.Summary.ItemKBStatus))
	}
	sb.WriteString("\n")

	// 棋盘英雄装备
	if len(ea.Champions) > 0 {
		sb.WriteString("### 棋盘英雄装备\n")
		for _, champ := range ea.Champions {
			items := make([]string, 0, len(champ.Items))
			for _, it := range champ.Items {
				name := it.Name
				if name == "" {
					name = it.ItemID
				}
				items = append(items, name)
			}
			coreMark := ""
			if champ.IsCore {
				coreMark = " [核心]"
			}
			sb.WriteString(fmt.Sprintf("- %s ★%d%s: %s\n",
				champ.Name, champ.Star, coreMark, strings.Join(items, ", ")))
		}
		sb.WriteString("\n")
	}

	// 核心英雄聚焦（已在分析阶段筛过 Likely + Candidate）
	if len(ea.CoreEquipment) > 0 {
		sb.WriteString("### 核心英雄聚焦\n")
		for _, core := range ea.CoreEquipment {
			items := make([]string, 0, len(core.Items))
			for _, it := range core.Items {
				name := it.Name
				if name == "" {
					name = it.ItemID
				}
				items = append(items, name)
			}
			sb.WriteString(fmt.Sprintf("- %s ★%d: %s\n", core.Name, core.Star, strings.Join(items, ", ")))
		}
	}

	return sb.String()
}
