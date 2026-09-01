// Package economy - TFT 经济分析层（Phase 16-B）
//
// 本包实现 "GroundedGameContext → EconomyAnalysis" 的确定性本地分析：
//   - Gold / Interest 当前值汇总
//   - Level / XP 进度汇总
//   - 连胜 / 连败 统计
//   - 当前 Stage / Phase
//
// 严格遵守不变量：
//   - 0 LLM 调用：纯本地计算（直接读取 GroundedGameState）
//   - 不实现经济决策（该不该升级 / 该不该刷牌 → Phase 17）
//   - 不修改 GroundedGameContext（只读消费）
//
// 不实现：
//   - "现在应该升级吗"
//   - "现在应该刷牌吗"
//   - "利息最大化策略"
package economy

import (
	"sync"
	"time"

	"github.com/lipipi/aurora/internal/game/plugins/tft/grounder"
	"github.com/lipipi/aurora/internal/log"
)

// EconomyFingerprint 经济缓存 key
//
// 仅包含"影响经济分析结论"的字段：
//   - 所有 GroundedGameState 核心经济字段
//   - Patch / SetKey（KB 版本变化 → 重算）
//
// 不包含：
//   - Board / Items / Bench / Shop（这些变化不影响经济分析结论）
type EconomyFingerprint struct {
	Patch   string `json:"patch"`
	SetKey  string `json:"set_key"`
	Stage   string `json:"stage"`
	Phase   string `json:"phase"`
	Level   int    `json:"level"`
	XP      int    `json:"xp"`
	XPToLevel int  `json:"xp_to_level"`
	Gold    int    `json:"gold"`
	Interest int   `json:"interest"`
	HP      int    `json:"hp"`
	WinStreak  int `json:"win_streak"`
	LoseStreak int `json:"lose_streak"`
}

// EconomyAnalysis EconomyAnalyzer 输出
type EconomyAnalysis struct {
	// Fingerprint 缓存 key
	Fingerprint EconomyFingerprint `json:"fingerprint"`
	// GeneratedAt
	GeneratedAt time.Time `json:"generated_at"`
	// Patch / SetKey
	Patch  string `json:"patch"`
	SetKey string `json:"set_key"`

	// Stage 阶段（如 "3-2"）
	Stage string `json:"stage"`
	// Phase 阶段类型（PvE / PvP）
	Phase string `json:"phase"`

	// Level 当前等级
	Level int `json:"level"`
	// XP 当前经验
	XP int `json:"xp"`
	// XPToLevel 升级还需要多少经验
	XPToLevel int `json:"xp_to_level"`
	// XPPercent 升级进度百分比（0-100）
	XPPercent float64 `json:"xp_percent"`

	// Gold 当前金币
	Gold int `json:"gold"`
	// Interest 当前可获得利息
	Interest int `json:"interest"`
	// InterestMax 最大利息（TFT 常规规则是 5）
	InterestMax int `json:"interest_max"`
	// GoldToInterest 距离下一点利息还差多少金币
	GoldToInterest int `json:"gold_to_interest"`

	// HP 当前生命值
	HP int `json:"hp"`

	// WinStreak 当前连胜
	WinStreak int `json:"win_streak"`
	// LoseStreak 当前连败
	LoseStreak int `json:"lose_streak"`
	// ActiveStreak 当前激活连胜/连败（0 无，正连胜，负连败）
	ActiveStreak int `json:"active_streak"`

	// Summary 文本总结
	Summary string `json:"summary"`
}

// Analyzer 经济分析器（带 Fingerprint 缓存）
type Analyzer struct {
	mu    sync.RWMutex
	cache *EconomyAnalysis
}

// NewAnalyzer 构造 Analyzer
func NewAnalyzer() *Analyzer {
	return &Analyzer{}
}

// Analyze 输入 GroundedGameContext，输出 EconomyAnalysis
//
// 行为约定：
//   - g == nil → 返回 zero-value EconomyAnalysis
//   - 缓存命中（Fingerprint.Equal）→ 直接返回缓存
//   - 0 LLM 调用（纯本地）
//   - 不做决策，只做结构化汇总
func (a *Analyzer) Analyze(g grounder.GroundedGameContext) EconomyAnalysis {
	fp := ComputeFingerprint(g)

	// Fast path: cache hit
	a.mu.RLock()
	if a.cache != nil && a.cache.Fingerprint.Equal(fp) {
		out := *a.cache
		a.mu.RUnlock()
		log.Debugf("economy: cache hit fp=%s", fp.Hash()[:8])
		return out
	}
	a.mu.RUnlock()

	// Slow path: rebuild
	out := a.analyzeOnce(g, fp)

	a.mu.Lock()
	a.cache = &out
	a.mu.Unlock()
	log.Debugf("economy: cache miss fp=%s gold=%d level=%d",
		fp.Hash()[:8], out.Gold, out.Level)
	return out
}

// analyzeOnce 真正做分析（pure function，不加锁）
func (a *Analyzer) analyzeOnce(g grounder.GroundedGameContext, fp EconomyFingerprint) EconomyAnalysis {
	gs := g.GameState
	out := EconomyAnalysis{
		Fingerprint: fp,
		GeneratedAt: time.Now(),
		Patch:       g.Meta.Patch,
		SetKey:      g.Meta.SetKey,
		Stage:       gs.Stage,
		Phase:       gs.Phase,
		Level:       gs.Level,
		XP:          gs.XP,
		XPToLevel:   gs.XPTolevel,
		Gold:        gs.Gold,
		Interest:    gs.Interest,
		HP:          gs.HP,
		WinStreak:   gs.WinStreak,
		LoseStreak:  gs.LoseStreak,
		InterestMax: 5, // TFT 标准规则：最大利息 5
	}

	// 计算 XP 百分比
	if out.XPToLevel > 0 {
		totalXP := out.XP + out.XPToLevel
		out.XPPercent = float64(out.XP) / float64(totalXP) * 100
	}

	// 计算距离下一点利息还差多少
	// 标准 TFT 规则：每 10 金币 1 利息，最大 5
	nextInterest := out.Interest + 1
	if nextInterest <= out.InterestMax {
		requiredGold := nextInterest * 10
		out.GoldToInterest = requiredGold - out.Gold
		if out.GoldToInterest < 0 {
			out.GoldToInterest = 0
		}
	} else {
		out.GoldToInterest = 0 // 已经达到最大利息
	}

	// 计算当前激活连胜/连败
	switch {
	case out.WinStreak > 0:
		out.ActiveStreak = out.WinStreak
	case out.LoseStreak > 0:
		out.ActiveStreak = -out.LoseStreak
	default:
		out.ActiveStreak = 0
	}

	return out
}

// Equal 判断两个 EconomyFingerprint 是否相等
func (f EconomyFingerprint) Equal(other EconomyFingerprint) bool {
	return f.Patch == other.Patch &&
		f.SetKey == other.SetKey &&
		f.Stage == other.Stage &&
		f.Phase == other.Phase &&
		f.Level == other.Level &&
		f.XP == other.XP &&
		f.XPToLevel == other.XPToLevel &&
		f.Gold == other.Gold &&
		f.Interest == other.Interest &&
		f.HP == other.HP &&
		f.WinStreak == other.WinStreak &&
		f.LoseStreak == other.LoseStreak
}
