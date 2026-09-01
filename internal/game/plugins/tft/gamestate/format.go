// Package gamestate - Format（Phase 16-B）
//
// GameStateAnalysis → 紧凑 prompt 片段（包含所有子模块）。
// 0 LLM 调用；按"请求过哪些模块"动态拼接 sections。
//
// 设计原则：
//   - 每个子模块独立判空（Equipment/Economy/Preparation 可独立 nil）
//   - nil 字段不输出对应 section（保持 Minimal Context 原则）
//   - 直接复用各子模块的 Format()，不做二次加工
package gamestate

import (
	"strings"

	"github.com/lipipi/aurora/internal/game/plugins/tft/economy"
	"github.com/lipipi/aurora/internal/game/plugins/tft/equipment"
	"github.com/lipipi/aurora/internal/game/plugins/tft/preparation"
)

// Format 将 GameStateAnalysis 格式化为紧凑 prompt 片段
//
// 输出约定：
//   - 所有模块都 nil → 返回 ""
//   - 只输出 non-nil 模块的 section
//   - 顺序：Equipment → Economy → Preparation（与 Context Scope 优先级一致）
//
// Composition 字段不在本函数输出 — Composition 有自己的 Format（Phase 16-A）。
// 调用方需要组合时，自己拼接 composition.Format(comp) + gamestate.Format(gsa)。
func Format(gsa GameStateAnalysis) string {
	var sb strings.Builder

	if gsa.Equipment != nil {
		if s := equipment.Format(*gsa.Equipment); s != "" {
			sb.WriteString(s)
		}
	}
	if gsa.Economy != nil {
		if s := economy.Format(*gsa.Economy); s != "" {
			sb.WriteString(s)
		}
	}
	if gsa.Preparation != nil {
		if s := preparation.Format(*gsa.Preparation); s != "" {
			sb.WriteString(s)
		}
	}

	return sb.String()
}
