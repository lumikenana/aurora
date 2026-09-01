// Package composition - Format(CompositionAnalysis) → string（prompt 渲染器）
//
// 设计要点：
//   - 输出结构化 Markdown 段落，可直接拼入 system prompt（追加在 grounder.Format 之后）
//   - 顶部 patch / setKey / KB 状态
//   - 棋盘 / 羁绊 / 核心候选三段
//   - 完全空（无 Board）→ 返回 ""
//   - 永不展开 Effect 全文（截断至 EffectLimit 字符）
//   - 不重复 grounder.Format 的内容（Board Champion 名称 / Cost / Traits 等）——
//     Composition 段只输出**分析结论**（aggregated 状态、breakpoint、core、summary）

package composition

import (
	"fmt"
	"strings"
)

// Format 把 CompositionAnalysis 渲染为可注入 system prompt 的 string
//
// 返回值约定：
//   - 完全空（Board 0 英雄）→ ""
//   - 否则至少 1 行（"## 阵容分析 …"）
//
// 模板：
//
//	## 阵容分析（patch=…，set=…，trait_kb=…）
//	- 棋盘人数: N
//	- 主羁绊: <id>（单位数/breakpoint）
//	- 次羁绊: <id>（…）
//	- 核心候选（K 个）: A, B, C
//
//	### 羁绊状态
//	- <id>: <name> <count>/<next_min>  <current_bp> → <next_bp>
//
//	### 核心候选详情
//	- A ★<star> cost=<c>: <reason>
func Format(a CompositionAnalysis) string {
	if a.ChampionCount == 0 {
		return ""
	}

	var sb strings.Builder
	writeHeader(&sb, a)
	writeSummary(&sb, a)
	writeTraits(&sb, a.Traits)
	writeCoreCandidates(&sb, a.CoreCandidates)
	return sb.String()
}

func writeHeader(sb *strings.Builder, a CompositionAnalysis) {
	patch := a.Patch
	if patch == "" {
		patch = "?"
	}
	setKey := a.SetKey
	if setKey == "" {
		setKey = "?"
	}
	fmt.Fprintf(sb, "## 阵容分析（patch=%s, set=%s, trait_kb=%s）",
		patch, setKey, a.Summary.TraitKBStatus)
}

func writeSummary(sb *strings.Builder, a CompositionAnalysis) {
	fmt.Fprintf(sb, "\n- 棋盘人数: %d", a.ChampionCount)
	if a.Summary.PrimaryTrait != "" {
		fmt.Fprintf(sb, "\n- 主羁绊: %s", a.Summary.PrimaryTrait)
	}
	if a.Summary.SecondaryTrait != "" {
		fmt.Fprintf(sb, "\n- 次羁绊: %s", a.Summary.SecondaryTrait)
	}
	if a.Summary.CoreChampionCount > 0 {
		// 列出 likely + candidate 的 id（不超过 5 个，避免 prompt 爆炸）
		ids := make([]string, 0, 5)
		for _, c := range a.CoreCandidates {
			if c.Status == CoreStatusLikely || c.Status == CoreStatusCandidate {
				id := c.ChampionID
				if c.Name != "" {
					id = c.Name
				}
				ids = append(ids, id)
				if len(ids) >= 5 {
					break
				}
			}
		}
		fmt.Fprintf(sb, "\n- 核心候选（%d 个）: %s", a.Summary.CoreChampionCount, strings.Join(ids, ", "))
	} else {
		sb.WriteString("\n- 核心候选: （无）")
	}
}

func writeTraits(sb *strings.Builder, traits []TraitAggregate) {
	if len(traits) == 0 {
		return
	}
	sb.WriteString("\n\n### 羁绊状态")
	for _, t := range traits {
		name := t.Name
		if name == "" {
			name = t.TraitID
		}
		var state string
		switch t.Status {
		case TraitStatusActive:
			cur := t.CurrentBreakpoint
			if cur != nil {
				state = fmt.Sprintf("激活 %d 阶", cur.MinUnits)
				if t.NextBreakpoint != nil {
					state += fmt.Sprintf("（下一阶 %d 需 %d 单位）", t.NextBreakpoint.MinUnits, t.NextBreakpoint.MinUnits)
				}
			}
		case TraitStatusInactive:
			if t.NextBreakpoint != nil {
				state = fmt.Sprintf("未激活（需 %d 单位）", t.NextBreakpoint.MinUnits)
			} else {
				state = "未激活"
			}
		case TraitStatusUnknown:
			state = "[breakpoint 数据缺失]"
		}
		fmt.Fprintf(sb, "\n- %s (%s): %d 单位 %s", t.TraitID, name, t.ActiveUnits, state)
	}
}

func writeCoreCandidates(sb *strings.Builder, cands []CoreCandidate) {
	if len(cands) == 0 {
		return
	}
	// 只输出 Likely + Candidate；Unknown 不进 prompt（避免噪声）
	var actionable []CoreCandidate
	for _, c := range cands {
		if c.Status == CoreStatusLikely || c.Status == CoreStatusCandidate {
			actionable = append(actionable, c)
		}
	}
	if len(actionable) == 0 {
		return
	}

	sb.WriteString("\n\n### 核心候选详情")
	for _, c := range actionable {
		name := c.Name
		if name == "" {
			name = c.ChampionID
		}
		fmt.Fprintf(sb, "\n- %s ★%d (cost=%d) [%.2f] %s",
			name, c.Star, c.Cost, c.Score, c.Reason)
	}
}
