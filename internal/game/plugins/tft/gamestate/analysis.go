// Package gamestate - Unified TFT Game State Analysis (Phase 16-B)
//
// 本包统一整合三个独立分析模块：
//   1. EquipmentAnalysis → 装备理解（Champion→Items / 全局统计）
//   2. EconomyAnalysis   → 经济理解（Gold/Interest/Level/XP/Streak）
//   3. PreparationAnalysis → 备战理解（Bench/Shop 重复计数）
//
// 架构原则：
//   - 每个分析模块完全独立，各自拥有独立缓存（Fingerprint 缓存）
//   - 只有当对应模块的依赖字段变化时，才会重算对应分析
//   - 装备变化 → 仅重算装备；经济变化 → 仅重算经济；备战变化 → 仅重算备战
//   - 严格零 LLM：所有分析都是本地确定性计算
//
// 不包含：
//   - 决策推荐（装备换装/升级/刷牌 → Phase 17）
//   - LLM 自然语言解释 → 在上游 Agent Context Scope 阶段完成
package gamestate

import (
	"github.com/lipipi/aurora/internal/game/plugins/tft/composition"
	"github.com/lipipi/aurora/internal/game/plugins/tft/equipment"
	"github.com/lipipi/aurora/internal/game/plugins/tft/economy"
	"github.com/lipipi/aurora/internal/game/plugins/tft/grounder"
	"github.com/lipipi/aurora/internal/game/plugins/tft/preparation"
)

// GameStateAnalysis 整合所有三个分析模块的输出
//
// 每个字段都是 optional：根据 Context Scope 请求，可能只包含部分分析
type GameStateAnalysis struct {
	// Equipment 装备分析（nil = 未请求/未计算）
	Equipment *equipment.EquipmentAnalysis `json:"equipment,omitempty"`
	// Economy 经济分析（nil = 未请求/未计算）
	Economy *economy.EconomyAnalysis `json:"economy,omitempty"`
	// Preparation 备战分析（nil = 未请求/未计算）
	Preparation *preparation.PreparationAnalysis `json:"preparation,omitempty"`
	// Composition 阵容分析（上游已存在，仅持有引用）
	Composition *composition.CompositionAnalysis `json:"composition,omitempty"`
}

// Analyzer 统一 GameState 分析器（持有三个独立分析器）
type Analyzer struct {
	equipment   *equipment.Analyzer
	economy     *economy.Analyzer
	preparation *preparation.Analyzer
}

// NewAnalyzer 构造统一 GameState 分析器
func NewAnalyzer() *Analyzer {
	return &Analyzer{
		equipment:   equipment.NewAnalyzer(),
		economy:     economy.NewAnalyzer(),
		preparation: preparation.NewAnalyzer(),
	}
}

// AnalysisOptions 指定需要分析哪些模块
//
// 用于 Context Scope：只分析用户问题相关的模块 → 最小化 Token 消耗
type AnalysisOptions struct {
	NeedEquipment   bool `json:"need_equipment"`
	NeedEconomy     bool `json:"need_economy"`
	NeedPreparation bool `json:"need_preparation"`
	NeedComposition bool `json:"need_composition"`
}

// Analyze 执行分析，只分析 options 指定需要的模块，复用缓存
//
// 参数：
//   - g: GroundedGameContext（只读）
//   - comp: CompositionAnalysis 来自 Phase 16-A（可为 nil）
//   - opts: AnalysisOptions 指定需要哪些分析
//
// 返回：GameStateAnalysis，仅包含请求的模块
//
// 不变量：0 LLM 调用，纯本地计算
func (a *Analyzer) Analyze(
	g grounder.GroundedGameContext,
	comp *composition.CompositionAnalysis,
	opts AnalysisOptions,
) GameStateAnalysis {
	result := GameStateAnalysis{
		Composition: comp,
	}

	if opts.NeedEquipment {
		ea := a.equipment.Analyze(g, comp)
		result.Equipment = &ea
	}

	if opts.NeedEconomy {
		ec := a.economy.Analyze(g)
		result.Economy = &ec
	}

	if opts.NeedPreparation {
		pa := a.preparation.Analyze(g)
		result.Preparation = &pa
	}

	return result
}

// InvalidateAll 清空所有缓存（强制全量重算，一般用于调试）
func (a *Analyzer) InvalidateAll() {
	// 由于每个 analyzer 用 mu RWMutex，我们通过替换实现清空
	*a.equipment = *equipment.NewAnalyzer()
	*a.economy = *economy.NewAnalyzer()
	*a.preparation = *preparation.NewAnalyzer()
}
