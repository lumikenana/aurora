// Package tft - TFT 对局状态 → LLM prompt 摘要（Phase 17-A 重定位）
//
// 历史：原本在 internal/agent/game_context.go（~600 行），属于 Agent Core。
// Phase 17-A 把这段逻辑下放到 tft 插件自己，因为：
//   - 包含 TFT 关键词、装备名、英雄名等游戏特定知识
//   - 依赖 composition / grounder / economy / equipment / preparation / gamestate
//     等 6 个 tft 专属子包
//   - Agent Core 不应知道 "tft / 装备 / 阵容" 等任何游戏语义（CLAUDE.md §6）
//
// 现在的调用路径（generic）：
//   Agent.onUserInput
//     → GameContextPreparer.Prepare
//         → game.Manager.Plugins() 迭代
//             → 对每个实现 game.GameContextRenderer 的插件调用 RenderGameContext
//                 → TFT 插件的 RenderGameContext → 内部 tftPromptRenderer.Render
//                     → 0 LLM 本地分析（grounder + composition + gamestate）
//     → 拼到 ExtraSystemPrompt 尾部
//
// 严格 0 LLM 调用（所有分析都是本地 deterministic）。
package tft

import (
	"context"
	"fmt"
	"strings"

	"github.com/lipipi/aurora/internal/game"
	"github.com/lipipi/aurora/internal/game/knowledge"
	"github.com/lipipi/aurora/internal/game/plugins/tft/composition"
	"github.com/lipipi/aurora/internal/game/plugins/tft/economy"
	"github.com/lipipi/aurora/internal/game/plugins/tft/equipment"
	"github.com/lipipi/aurora/internal/game/plugins/tft/gamestate"
	"github.com/lipipi/aurora/internal/game/plugins/tft/grounder"
	"github.com/lipipi/aurora/internal/game/plugins/tft/preparation"
	"github.com/lipipi/aurora/internal/log"
)

// ContextDetail 控制 GameContext Summary 的细节级别
//
// v1 实际只用 Minimal / Full；Combat / Economy 保留给未来扩展。
type ContextDetail string

const (
	ContextMinimal ContextDetail = "minimal" // 基础资源（阶段/金币/血量/等级/经济流）
	ContextCombat  ContextDetail = "combat"  // Combat（含 Board/Bench）
	ContextEconomy ContextDetail = "economy" // Economy（含 Gold/Interest/WinStreak）
	ContextFull    ContextDetail = "full"    // 全部
)

// PromptRenderer 把 TFTGameContext 转换为 LLM 可消费的 prompt 摘要
//
// 设计要点：
//   - 只读消费 gctx（不修改游戏状态）
//   - knowledge 可为 nil（不可用时跳过 KB 查询，FormatTFTGameContext 仍可用）
//   - grounder 可为 nil（回退到 queryRelevantKB 旧路径）
//   - 严格遵守 patch 隔离：GameContext.Version 优先 → KB active patch 回退
//   - 不在内部缓存 GameContext（每次调用都基于最新快照）
type PromptRenderer struct {
	knowledge          KnowledgeProvider
	grounder           *grounder.Grounder
	compositionAnalyzer *composition.Analyzer
	gameStateAnalyzer  *gamestate.Analyzer
}

// NewPromptRenderer 构造 Renderer
//
// 参数均为可空（nil-safe）：
//   - kb = nil         → 不查 KB
//   - grnd = nil       → 不走 grounder，回退到 queryRelevantKB
//   - comp = nil       → 不附加 Composition 段
//   - gsa = nil        → 不附加 GameState 结构化分析
func NewPromptRenderer(kb KnowledgeProvider, grnd *grounder.Grounder, comp *composition.Analyzer, gsa *gamestate.Analyzer) *PromptRenderer {
	return &PromptRenderer{
		knowledge:          kb,
		grounder:           grnd,
		compositionAnalyzer: comp,
		gameStateAnalyzer:  gsa,
	}
}

// Render 是 GameContextRenderer 接口的实现入口（也作为 Plugin.RenderGameContext 内部调用）
//
// 返回 (summary, used)：
//   - used=false  → 跳过注入（不在游戏中 / 不相关问题 / GameContext 全空）
//   - used=true   → summary 是紧凑 prompt 片段
//
// 内部 grounder / KB 查询使用 context.Background()（同步短路径；
// game.GameContextRenderer 接口本身不接受 ctx，本地 KB 查询也无需取消）。
// 若未来 KB 变远程或需取消，需扩展接口签名。
func (r *PromptRenderer) Render(gctx *game.GameContext, userInput string) (string, bool) {
	ctx := context.Background()
	if r == nil || gctx == nil {
		return "", false
	}
	// 非 TFT 游戏（LOL/CS2 未来）跳过
	if gctx.Game != "tft" {
		log.Debugf("tft_prompt: skipped reason=not_tft_game game=%s", gctx.Game)
		return "", false
	}
	tctx, ok := gctx.GameData.(*TFTGameContext)
	if !ok || tctx == nil {
		log.Debug("tft_prompt: skipped reason=tft_context_missing")
		return "", false
	}

	// Phase 必须是 in_game / carousel / draft 之一
	if !isInjectablePhase(gctx.Phase) {
		log.Debugf("tft_prompt: skipped reason=phase=%s", gctx.Phase)
		return "", false
	}

	if !needsGameContext(userInput) {
		log.Debug("tft_prompt: skipped reason=not_tft_question")
		return "", false
	}

	detail := pickContextDetail(userInput)
	summary := FormatTFTGameContext(tctx, detail)
	if summary == "" {
		log.Debug("tft_prompt: skipped reason=empty_summary")
		return "", false
	}

	log.Debugf("tft_prompt: prepared detail=%s stage=%v gold=%v hp=%v board=%d bench=%d",
		detail,
		tctx.Stage.IsKnown(), tctx.Gold.IsKnown(), tctx.HP.IsKnown(),
		boardCount(tctx), benchCount(tctx))

	// 按需查询 KB / Grounder / Composition / GameState
	if r.grounder != nil && detail == ContextFull {
		grounded := r.grounder.Resolve(ctx, gctx, tctx)
		if section := grounder.Format(grounded); section != "" {
			summary = summary + "\n\n" + section
			log.Debugf("tft_prompt: grounder applied meta=(patch=%s, set=%s, fallback=%v) board=%d bench=%d items=%d",
				grounded.Meta.Patch, grounded.Meta.SetKey, grounded.Meta.PatchFallback,
				len(grounded.Board), len(grounded.Bench), len(grounded.Items))
		}

		var comp *composition.CompositionAnalysis
		if r.compositionAnalyzer != nil {
			compResult := r.compositionAnalyzer.Analyze(grounded)
			comp = &compResult
			if compSection := composition.Format(compResult); compSection != "" {
				summary = summary + "\n\n" + compSection
				log.Debugf("tft_prompt: composition applied champions=%d traits=%d cores=%d fp=%s",
					compResult.ChampionCount, len(compResult.Traits), len(compResult.CoreCandidates),
					compResult.Fingerprint.Hash()[:8])
			}
		}

		if r.gameStateAnalyzer != nil {
			opts := r.detectAnalysisOptions(userInput)
			gsa := r.gameStateAnalyzer.Analyze(grounded, comp, opts)

			if gsa.Equipment != nil {
				if eqSection := equipment.Format(*gsa.Equipment); eqSection != "" {
					summary = summary + "\n\n" + eqSection
					log.Debugf("tft_prompt: equipment applied items=%d unique=%d fp=%s",
						gsa.Equipment.Summary.TotalItems, gsa.Equipment.Summary.UniqueItems,
						gsa.Equipment.Fingerprint.Hash()[:8])
				}
			}

			if gsa.Economy != nil {
				if ecSection := economy.Format(*gsa.Economy); ecSection != "" {
					summary = summary + "\n\n" + ecSection
					log.Debugf("tft_prompt: economy applied gold=%d level=%d fp=%s",
						gsa.Economy.Gold, gsa.Economy.Level,
						gsa.Economy.Fingerprint.Hash()[:8])
				}
			}

			if gsa.Preparation != nil {
				if prSection := preparation.Format(*gsa.Preparation); prSection != "" {
					summary = summary + "\n\n" + prSection
					log.Debugf("tft_prompt: preparation applied bench=%d shop=%d upgradable=%d fp=%s",
						gsa.Preparation.Summary.BenchSlotsUsed, gsa.Preparation.Summary.ShopSlotsUsed,
						gsa.Preparation.Summary.Upgradable,
						gsa.Preparation.Fingerprint.Hash()[:8])
				}
			}
		}
	} else if kbSection := r.queryRelevantKB(ctx, gctx, tctx, detail); kbSection != "" {
		summary = summary + "\n\n" + kbSection
	}

	return summary, true
}

// detectAnalysisOptions 根据用户问题检测需要哪些分析模块
//
// 规则：
//   - 包含装备相关关键词 → NeedEquipment
//   - 包含经济/金币/利息/升级/等级相关关键词 → NeedEconomy
//   - 包含备战/商店/刷新/升星/拿牌相关关键词 → NeedPreparation
//   - ContextFull → 全要（用户问整体/怎么玩/策略）
//
// 目的：只注入用户问题真正需要的分析 → 最小化 Token 消耗
func (r *PromptRenderer) detectAnalysisOptions(userInput string) gamestate.AnalysisOptions {
	s := strings.ToLower(userInput)
	opts := gamestate.AnalysisOptions{}

	equipmentKeywords := []string{"装备", "散件", "成装", "给什么装备", "带什么", "无尽", "法爆", "巨杀", "正义", "蓝buff", "偷偷告诉你装备", "装备给谁"}
	for _, k := range equipmentKeywords {
		if strings.Contains(s, k) {
			opts.NeedEquipment = true
			break
		}
	}

	economyKeywords := []string{"金币", "利息", "经济", "连胜", "连败", "血量", "升级", "升人口", "经验", "xp", "现在多少金", "能吃多少利息"}
	for _, k := range economyKeywords {
		if strings.Contains(s, k) {
			opts.NeedEconomy = true
			break
		}
	}

	preparationKeywords := []string{"备战", "备战席", " bench ", "商店", "刷新", "d牌", "刷牌", "升星", "能升几星", "这个英雄够吗", "拿不拿"}
	for _, k := range preparationKeywords {
		if strings.Contains(s, k) {
			opts.NeedPreparation = true
			break
		}
	}

	fullKeywords := []string{"运营", "怎么玩", "怎么打", "怎么运营", "下一步", "思路", "策略", "建议", "打法", "这局", "这把"}
	for _, k := range fullKeywords {
		if strings.Contains(s, k) {
			opts.NeedEquipment = true
			opts.NeedEconomy = true
			opts.NeedPreparation = true
			opts.NeedComposition = true
			break
		}
	}

	if !opts.NeedEquipment && !opts.NeedEconomy && !opts.NeedPreparation {
		opts.NeedEquipment = true
		opts.NeedEconomy = true
		opts.NeedPreparation = true
		opts.NeedComposition = true
	}

	return opts
}

// isInjectablePhase 判断 Phase 是否适合注入 GameContext
func isInjectablePhase(phase game.ContextPhase) bool {
	switch phase {
	case game.PhaseInGame, game.PhaseCarousel, game.PhaseDraft:
		return true
	}
	return false
}

// needsGameContext 简单关键词判断（未来可升级为 NLP 分类）
//
// 命中策略：包含任一 TFT 游戏关键词即认为需要 GameContext。
// 故意宽松：宁可多注入（让 LLM 自行判断用不用），不要漏注入。
// 对完全无关的问题（如"今天天气"）也不会误命中（关键词不重叠）。
func needsGameContext(userInput string) bool {
	s := strings.ToLower(userInput)
	if s == "" {
		return false
	}
	keywords := []string{
		// 通用 TFT 词
		"tft", "云顶", "云顶之弈", "金铲铲", "铲铲",
		// 资源 / 经济
		"金币", "利息", "经济", "连胜", "连败", "血量", "生命", "hp",
		// 棋盘 / 备战 / 商店
		"棋盘", "阵容", "备战", "备战席", "商店", "刷新", "d牌", "升级", "升人口", "经验",
		// 装备
		"装备", "散件", "成装", "无尽", "电刀", "法爆", "巨杀", "正义", "蓝buff", "给什么",
		// 强化
		"强化", "符文", "海克斯", "棱彩", "银色的", "金色的", "彩色强化",
		// 羁绊
		"羁绊", "特质",
		// 决策
		"运营", "怎么玩", "怎么打", "怎么运营", "打法", "思路", "策略", "下一步", "建议",
		// 指示代词（需要上下文才能回答）
		"这把", "这局", "当前", "现在", "我该",
		// 常见英雄名（中英）
		"薇恩", "vayne", "vn", "瑟提", "亚索", "金克丝", "jinx", "卡莎", "kaisa", "厄运小姐",
	}
	for _, k := range keywords {
		if strings.Contains(s, k) {
			return true
		}
	}
	return false
}

// pickContextDetail 根据用户问题决定 detail 级别
func pickContextDetail(userInput string) ContextDetail {
	s := strings.ToLower(userInput)
	fullKeywords := []string{
		"运营", "怎么玩", "怎么打", "怎么运营", "下一步", "思路", "策略", "建议", "打法", "这局", "这把",
		"装备", "给什么", "强化", "羁绊", "阵容", "棋盘", "备战",
	}
	for _, k := range fullKeywords {
		if strings.Contains(s, k) {
			return ContextFull
		}
	}
	return ContextMinimal
}

// FormatTFTGameContext 把 *TFTGameContext 格式化为紧凑的 prompt 片段
//
// 严格遵守：
//   - Value[T].Present=false → 不输出（Unknown 不进入 prompt）
//   - 不输出 Confidence / Source（内部调试信息）
//   - Known(0) 正确输出（HP=0 阵亡合法，Gold=0 破产合法）
//   - 空 GameContext / nil → 返回 ""（不进入 prompt）
//   - 不输出 KB 数据（KB 注入由 queryRelevantKB 负责）
func FormatTFTGameContext(tctx *TFTGameContext, detail ContextDetail) string {
	if tctx == nil {
		return ""
	}

	// 1) 收集 base 资源
	var base strings.Builder
	if tctx.Stage.IsKnown() {
		fmt.Fprintf(&base, "- 阶段：%s\n", tctx.Stage.Val)
	}
	if tctx.Level.IsKnown() {
		fmt.Fprintf(&base, "- 等级：%d\n", tctx.Level.Val)
	}
	if tctx.XP.IsKnown() {
		fmt.Fprintf(&base, "- 经验：%d\n", tctx.XP.Val)
	}
	if tctx.Gold.IsKnown() {
		fmt.Fprintf(&base, "- 金币：%d\n", tctx.Gold.Val)
	}
	if tctx.Interest.IsKnown() {
		fmt.Fprintf(&base, "- 利息：%d\n", tctx.Interest.Val)
	}
	if tctx.HP.IsKnown() {
		fmt.Fprintf(&base, "- 生命：%d\n", tctx.HP.Val)
	}
	if tctx.WinStreak.IsKnown() {
		fmt.Fprintf(&base, "- 连胜：%d\n", tctx.WinStreak.Val)
	}
	if tctx.LoseStreak.IsKnown() {
		fmt.Fprintf(&base, "- 连败：%d\n", tctx.LoseStreak.Val)
	}
	hasBase := base.Len() > 0

	// 2) Full-only 段
	var full strings.Builder
	if detail == ContextFull {
		if tctx.Board.IsKnown() && len(tctx.Board.Val) > 0 {
			full.WriteString("\n### 棋盘\n")
			for _, s := range tctx.Board.Val {
				items := ""
				if len(s.Items) > 0 {
					items = "（装备：" + strings.Join(s.Items, ", ") + "）"
				}
				fmt.Fprintf(&full, "- %s ★%d (位置 %d) %s\n", s.ChampionID, s.Star, s.Position, items)
			}
		}
		if tctx.Bench.IsKnown() && len(tctx.Bench.Val) > 0 {
			full.WriteString("\n### 备战席\n")
			for _, s := range tctx.Bench.Val {
				fmt.Fprintf(&full, "- %s ★%d\n", s.ChampionID, s.Star)
			}
		}
		if tctx.Shop.IsKnown() && len(tctx.Shop.Val) > 0 {
			full.WriteString("\n### 商店\n")
			for _, s := range tctx.Shop.Val {
				fmt.Fprintf(&full, "- %s (%d费)\n", s.ChampionID, s.Cost)
			}
		}
		if tctx.Items.IsKnown() && len(tctx.Items.Val) > 0 {
			full.WriteString("\n### 装备\n")
			for _, it := range tctx.Items.Val {
				fmt.Fprintf(&full, "- %s × %d\n", it.ItemID, it.Count)
			}
		}
		if tctx.Traits.IsKnown() && len(tctx.Traits.Val) > 0 {
			full.WriteString("\n### 羁绊\n")
			for _, tr := range tctx.Traits.Val {
				fmt.Fprintf(&full, "- %s (%d/%d)\n", tr.TraitID, tr.Active, tr.Style)
			}
		}
		if tctx.Augments.IsKnown() && len(tctx.Augments.Val) > 0 {
			full.WriteString("\n### 强化符文\n")
			for _, a := range tctx.Augments.Val {
				fmt.Fprintf(&full, "- %s [%s]\n", a.AugmentID, a.Tier)
			}
		}
	}
	hasFull := full.Len() > 0

	// 3) 无任何信息
	if !hasBase && !hasFull {
		return ""
	}

	// 4) 拼装
	var sb strings.Builder
	sb.WriteString("## 当前 TFT 对局状态\n")
	if hasBase {
		sb.WriteString("\n### 基础资源\n")
		sb.WriteString(base.String())
	}
	sb.WriteString(full.String())
	sb.WriteString("\n注意：以上信息来自当前游戏画面识别，部分字段可能存在识别误差。")
	return sb.String()
}

// queryRelevantKB 按 userInput / GameContext 决定要查询哪些 KB 记录
//
// 严格遵守 patch 隔离：
//   - 优先使用 gctx.CommonData.Version（Known 时）
//   - 未识别时回退到 KB active patch
//   - 仍未知 → 跳过 KB 查询
func (r *PromptRenderer) queryRelevantKB(ctx context.Context, gctx *game.GameContext, tctx *TFTGameContext, detail ContextDetail) string {
	if r.knowledge == nil {
		return ""
	}
	if detail != ContextFull {
		return ""
	}

	patch, hasPatch := r.resolvePatch(ctx, gctx)
	if !hasPatch {
		log.Debug("tft_prompt: kb skipped reason=patch_unknown")
		return ""
	}

	championIDs := collectChampionIDs(tctx)
	if len(championIDs) == 0 {
		return ""
	}

	const gameID = "tft"
	const mode = "standard"

	limit := len(championIDs)
	if limit > 4 {
		limit = 4
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "## 知识库查询（patch=%s, game=%s, mode=%s）\n\n", patch, gameID, mode)
	sb.WriteString("### 当前场上/备战席英雄（按 id 匹配当前 patch）\n")

	for _, id := range championIDs[:limit] {
		log.Debugf("tft_prompt: champion lookup id=%s patch=%s", id, patch)
		rows, err := r.knowledge.GetChampion(ctx, gameID, knowledge.Query{ID: id})
		if err != nil {
			log.Debugf("tft_prompt: champion lookup failed id=%s err=%v", id, err)
			continue
		}
		if len(rows) == 0 {
			continue
		}
		c := rows[0]
		if c.Name != "" && c.Name != c.ID {
			fmt.Fprintf(&sb, "- %s: %s\n", c.ID, c.Name)
		} else {
			fmt.Fprintf(&sb, "- %s\n", c.ID)
		}
	}

	return sb.String()
}

// resolvePatch 解析当前 patch
//
// 优先级：
//  1. GameContext.CommonData.Version（Known 且非空）
//  2. KB active patch
//  3. 仍未知名 → 返回 "" / false
func (r *PromptRenderer) resolvePatch(ctx context.Context, gctx *game.GameContext) (string, bool) {
	if gctx != nil && gctx.CommonData.Version.IsKnown() && gctx.CommonData.Version.Val != "" {
		return gctx.CommonData.Version.Val, true
	}
	if r.knowledge == nil {
		return "", false
	}
	patch, err := r.knowledge.GetCurrentPatch(ctx)
	if err != nil || patch == "" {
		return "", false
	}
	return patch, true
}

// collectChampionIDs 收集 Board + Bench 上的英雄 ID（去重）
func collectChampionIDs(tctx *TFTGameContext) []string {
	if tctx == nil {
		return nil
	}
	seen := map[string]struct{}{}
	var ids []string
	if tctx.Board.IsKnown() {
		for _, s := range tctx.Board.Val {
			if s.ChampionID == "" {
				continue
			}
			if _, ok := seen[s.ChampionID]; ok {
				continue
			}
			seen[s.ChampionID] = struct{}{}
			ids = append(ids, s.ChampionID)
		}
	}
	if tctx.Bench.IsKnown() {
		for _, s := range tctx.Bench.Val {
			if s.ChampionID == "" {
				continue
			}
			if _, ok := seen[s.ChampionID]; ok {
				continue
			}
			seen[s.ChampionID] = struct{}{}
			ids = append(ids, s.ChampionID)
		}
	}
	return ids
}

func boardCount(tctx *TFTGameContext) int {
	if tctx == nil || !tctx.Board.IsKnown() {
		return 0
	}
	return len(tctx.Board.Val)
}

func benchCount(tctx *TFTGameContext) int {
	if tctx == nil || !tctx.Bench.IsKnown() {
		return 0
	}
	return len(tctx.Bench.Val)
}
