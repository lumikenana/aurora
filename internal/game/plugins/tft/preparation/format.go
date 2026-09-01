// Package preparation - Format（Phase 16-B）
//
// PreparationAnalysis → 紧凑 prompt 片段。
// 0 LLM 调用；纯本地结构化输出。
package preparation

import (
	"fmt"
	"strings"
)

// Format 将 PreparationAnalysis 格式化为紧凑 prompt 片段
//
// 空数据约定：所有三个 section 都没有数据时返回 ""。
// 部分有数据时只输出有数据的 section（不会出现"空 sections"）。
func Format(pa PreparationAnalysis) string {
	if len(pa.Bench) == 0 && len(pa.Shop) == 0 && len(pa.DuplicateCounts) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("## 备战分析（本地结构化）\n\n")

	// 汇总
	sb.WriteString("### 备战席状态\n")
	sb.WriteString(fmt.Sprintf("- 已占用：%d / 可用：%d\n", pa.Summary.BenchSlotsUsed, pa.Summary.BenchSlotsAvailable))
	sb.WriteString(fmt.Sprintf("- 商店已有：%d\n", pa.Summary.ShopSlotsUsed))
	if pa.Summary.Upgradable > 0 {
		sb.WriteString(fmt.Sprintf("- 可升级英雄：%d\n\n", pa.Summary.Upgradable))
	} else {
		sb.WriteString("\n")
	}

	// 可升级英雄（重复计数排序在前）
	if len(pa.DuplicateCounts) > 0 {
		sb.WriteString("### 英雄重复计数（可升级）\n")
		for _, dc := range pa.DuplicateCounts {
			if dc.TotalCount < 3 {
				continue // 不够 3 个 → 不能升 2，跳过
			}
			nextStar := dc.NextStar
			needed := dc.NeededForNextStar
			name := dc.Name
			if name == "" {
				name = dc.ChampionID
			}
			switch nextStar {
			case 2:
				sb.WriteString(fmt.Sprintf("- %s: 需要 %d 个可升2星（当前共 %d）\n", name, needed, dc.TotalCount))
			case 3:
				sb.WriteString(fmt.Sprintf("- %s: 需要 %d 个可升3星（当前共 %d）\n", name, needed, dc.TotalCount))
			default:
				// NextStar == 0 → 已 3 星
				sb.WriteString(fmt.Sprintf("- %s: 已满3星（当前共 %d）\n", name, dc.TotalCount))
			}
		}
		sb.WriteString("\n")
	}

	// 备战席列表
	if len(pa.Bench) > 0 {
		sb.WriteString("### 备战席英雄\n")
		for _, b := range pa.Bench {
			name := b.Name
			if name == "" {
				name = b.ChampionID
			}
			sb.WriteString(fmt.Sprintf("- %s ★%d (%d费)\n", name, b.Star, b.Cost))
		}
		sb.WriteString("\n")
	}

	// 商店列表
	if len(pa.Shop) > 0 {
		sb.WriteString("### 商店刷新\n")
		for _, s := range pa.Shop {
			name := s.Name
			if name == "" {
				name = s.ChampionID
			}
			sb.WriteString(fmt.Sprintf("- %s (%d费)\n", name, s.Cost))
		}
	}

	return sb.String()
}
