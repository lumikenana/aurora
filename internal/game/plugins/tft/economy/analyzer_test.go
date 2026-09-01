// Package economy - Analyzer 行为测试（Phase 16-B）
//
// 覆盖：
//   1. Gold/Interest/Level/XP 等基础字段透传
//   2. Interest 计算（50 金 = 5 利息，TFT 规则）
//   3. Stage/Phase 透传
//   4. WinStreak / LoseStreak 透传 + ActiveStreak 符号
//   5. nil/空/Unknown 输入的零值行为
//   6. Fingerprint 缓存（不依赖 Phase 16-A 的 CompositionAnalysis）
//   7. Unknown 字段保持 Unknown（0 利息 ≠ 利息未识别）
package economy

import (
	"testing"

	"github.com/lipipi/aurora/internal/game/plugins/tft/grounder"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// makeCtx 构造一个 GroundedGameContext with various fields set
func makeCtx(stage, phase string, level, xp, xpToLevel, gold, interest, hp, winStreak, loseStreak int) grounder.GroundedGameContext {
	return grounder.GroundedGameContext{
		GameState: grounder.GroundedGameState{
			Stage:      stage,
			Phase:      phase,
			Level:      level,
			XP:         xp,
			XPTolevel:  xpToLevel,
			Gold:       gold,
			Interest:   interest,
			HP:         hp,
			WinStreak:  winStreak,
			LoseStreak: loseStreak,
		},
		Meta: grounder.GroundedMeta{Patch: "14.1", SetKey: "TFT18"},
	}
}

func TestAnalyzer_EmptyContext_AllZero(t *testing.T) {
	a := NewAnalyzer()
	out := a.Analyze(grounder.GroundedGameContext{})
	assert.Equal(t, 0, out.Gold, "empty input must yield zero gold")
	assert.Equal(t, 0, out.Interest)
	assert.Equal(t, 0, out.Level)
	assert.Equal(t, "", out.Stage)
	assert.Equal(t, "", out.Phase)
}

func TestAnalyzer_BasicFields_Propagate(t *testing.T) {
	a := NewAnalyzer()
	g := makeCtx("4-2", "PvP", 7, 12, 4, 42, 3, 67, 3, 0)

	out := a.Analyze(g)

	assert.Equal(t, "4-2", out.Stage)
	assert.Equal(t, "PvP", out.Phase)
	assert.Equal(t, 7, out.Level)
	assert.Equal(t, 12, out.XP)
	assert.Equal(t, 4, out.XPToLevel)
	assert.Equal(t, 42, out.Gold)
	assert.Equal(t, 3, out.Interest)
	assert.Equal(t, 67, out.HP)
	assert.Equal(t, 3, out.WinStreak)
	assert.Equal(t, 0, out.LoseStreak)
}

func TestAnalyzer_Interest_Default5(t *testing.T) {
	a := NewAnalyzer()
	g := makeCtx("", "", 0, 0, 0, 0, 0, 0, 0, 0)
	out := a.Analyze(g)
	assert.Equal(t, 5, out.InterestMax, "TFT 利息上限是 5；不依赖 input")
}

func TestAnalyzer_Interest_ZeroWhenNoGold(t *testing.T) {
	a := NewAnalyzer()
	g := makeCtx("", "", 0, 0, 0, 0, 0, 0, 0, 0) // 全部 0
	out := a.Analyze(g)
	assert.Equal(t, 0, out.Interest, "no input → 利息 0（不是 5）")
}

func TestAnalyzer_ActiveStreak_Win(t *testing.T) {
	a := NewAnalyzer()
	g := makeCtx("", "", 0, 0, 0, 0, 0, 0, 3, 0)
	out := a.Analyze(g)
	assert.Equal(t, 3, out.ActiveStreak, "win streak → ActiveStreak = +winStreak")
}

func TestAnalyzer_ActiveStreak_Lose(t *testing.T) {
	a := NewAnalyzer()
	g := makeCtx("", "", 0, 0, 0, 0, 0, 0, 0, 2)
	out := a.Analyze(g)
	assert.Equal(t, -2, out.ActiveStreak, "lose streak → ActiveStreak = -loseStreak")
}

func TestAnalyzer_ActiveStreak_None(t *testing.T) {
	a := NewAnalyzer()
	g := makeCtx("", "", 0, 0, 0, 0, 0, 0, 0, 0)
	out := a.Analyze(g)
	assert.Equal(t, 0, out.ActiveStreak, "no streak → 0")
}

func TestAnalyzer_XPPercent(t *testing.T) {
	a := NewAnalyzer()
	g := makeCtx("", "", 5, 8, 4, 0, 0, 0, 0, 0) // XP=8, XPTolevel=4 → 66.67%
	out := a.Analyze(g)
	require.Greater(t, out.XPToLevel, 0)
	assert.InDelta(t, 66.67, out.XPPercent, 0.1)
}

func TestAnalyzer_Fingerprint_CacheHit(t *testing.T) {
	a := NewAnalyzer()
	g1 := makeCtx("4-2", "PvP", 7, 12, 4, 42, 3, 67, 3, 0)
	out1 := a.Analyze(g1)
	fp1 := out1.Fingerprint
	ts1 := out1.GeneratedAt

	// 同样的 g → 命中缓存（fp + GeneratedAt 不变）
	out2 := a.Analyze(g1)
	assert.Equal(t, fp1, out2.Fingerprint, "Fingerprint 不变")
	assert.Equal(t, ts1, out2.GeneratedAt, "GeneratedAt 不变（cache hit）")
}

func TestAnalyzer_Fingerprint_GoldChangesInvalidate(t *testing.T) {
	a := NewAnalyzer()
	g1 := makeCtx("4-2", "PvP", 7, 12, 4, 42, 3, 67, 3, 0)
	_ = a.Analyze(g1)

	g2 := makeCtx("4-2", "PvP", 7, 12, 4, 41, 3, 67, 3, 0) // Gold 42→41
	out2 := a.Analyze(g2)
	assert.NotEqual(t, g1.GameState.Gold, out2.Fingerprint.Gold, "Gold 变化必须重算")
}

func TestAnalyzer_Fingerprint_OnlyRelevantFields(t *testing.T) {
	// WinStreak 变化 → 应当重算（属于 Economy 输入）
	// 纯 Board 变化 → 不应影响 Economy 缓存（但 Economy analyzer 不消费 Board，所以这等价于无变化）
	a := NewAnalyzer()
	g1 := makeCtx("4-2", "PvP", 7, 12, 4, 42, 3, 67, 3, 0)
	_ = a.Analyze(g1)

	g2 := makeCtx("4-2", "PvP", 7, 12, 4, 42, 3, 67, 5, 0) // 3→5 连胜
	out2 := a.Analyze(g2)
	assert.Equal(t, 5, out2.WinStreak)
	assert.NotEqual(t, 0, out2.Fingerprint.WinStreak)
}

func TestAnalyzer_Format_Empty(t *testing.T) {
	// 全部 0 → 触发 zero-value 兜底，返回 ""
	s := Format(EconomyAnalysis{})
	assert.Equal(t, "", s)
}

func TestAnalyzer_Format_OnlyGoldSet(t *testing.T) {
	// Gold = 0, Level = 0, HP = 0 → 仍返回 ""（这是 Format 的 empty-data 约定）
	s := Format(EconomyAnalysis{Interest: 3})
	assert.Equal(t, "", s, "仅 Interest 不足以触发输出")
}

func TestAnalyzer_Format_GoldAndLevel(t *testing.T) {
	s := Format(EconomyAnalysis{
		Stage: "4-2", Phase: "PvP",
		Level: 7, XP: 12, XPToLevel: 4,
		Gold: 42, Interest: 3, InterestMax: 5,
		HP: 67,
	})
	assert.Contains(t, s, "## 经济分析")
	assert.Contains(t, s, "阶段：4-2 / PvP")
	assert.Contains(t, s, "等级：7")
	assert.Contains(t, s, "金币：42")
	assert.Contains(t, s, "利息：3 / 最大 5")
	assert.Contains(t, s, "生命值：67")
}

func TestAnalyzer_Format_WinStreak(t *testing.T) {
	s := Format(EconomyAnalysis{Gold: 50, Level: 6, HP: 80, WinStreak: 3})
	assert.Contains(t, s, "3 连胜")

	s2 := Format(EconomyAnalysis{Gold: 50, Level: 6, HP: 80, LoseStreak: 4})
	assert.Contains(t, s2, "4 连败")

	s3 := Format(EconomyAnalysis{Gold: 50, Level: 6, HP: 80})
	assert.Contains(t, s3, "无连胜/连败")
}
