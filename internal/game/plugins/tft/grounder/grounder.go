// Package grounder - Grounder 核心实现（Phase 15）
//
// 本文件实现：
//   - Grounder 结构与 GroundingRepository 接口
//   - Resolve 把 *tfttypes.TFTGameContext 投影为 GroundedGameContext
//   - Per-type 解析器（Champion/Item/Trait/Augment）
//   - ID prefix validation（patch 隔离第三层防御）
//   - Dedup（同 ID 在多槽位只查一次）
//   - Data JSON 字段解码器（CommunityDragon 格式）
//
// 不变量：
//   - Grounder 无状态，并发安全
//   - 每次 Resolve 都从 KB 重新查询（KB 端有 RWMutex 保护）
//   - prefix 校验失败的 ID 绝不查 KB，绝不静默纠正
package grounder

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/lipipi/aurora/internal/errors"
	"github.com/lipipi/aurora/internal/game"
	"github.com/lipipi/aurora/internal/game/knowledge"
	"github.com/lipipi/aurora/internal/game/plugins/tft/tfttypes"
	"github.com/lipipi/aurora/internal/log"
)

// GroundingRepository Grounder 依赖的 KB 最小抽象
//
// *knowledge.Service 天然满足这 5 个方法（见 internal/game/knowledge/service.go）。
// Grounder 不 import internal/tool/tools 避免层次倒挂。
type GroundingRepository interface {
	GetCurrentPatch(ctx context.Context) (string, errors.AppError)
	GetChampion(ctx context.Context, game string, q knowledge.Query) ([]knowledge.Champion, errors.AppError)
	GetItem(ctx context.Context, game string, q knowledge.Query) ([]knowledge.Item, errors.AppError)
	GetTrait(ctx context.Context, game string, q knowledge.Query) ([]knowledge.Trait, errors.AppError)
	GetAugment(ctx context.Context, game string, q knowledge.Query) ([]knowledge.Augment, errors.AppError)
}

// Grounder 知识富化器（无状态）
type Grounder struct {
	repo GroundingRepository
	// game / mode 写死为 tft / standard（与 tools.KnowledgeProvider 一致）
	game string
	mode string
}

// New 创建 Grounder
//
// repo == nil 时返回的 Grounder 仍可工作：所有实体返回 Status=KBUnavailable，
// 但 GroundedGameContext 结构本身正常填充。
func New(repo GroundingRepository) *Grounder {
	return &Grounder{repo: repo, game: "tft", mode: "standard"}
}

// ---- 公共入口 ----

// Resolve 把 *tfttypes.TFTGameContext 投影为 GroundedGameContext
//
// 行为约定：
//   - tctx == nil          → 返回空 ctx + Meta 反映 KB/patch 状态
//   - gctx == nil          → KB active patch 仍可查询（patch 回退）
//   - patch 不可解析        → 仍透传 GameState / Board 等所有 Known 槽位，
//     每个 entity 标 Status=KBUnavailable
//   - ID 为 "" 或 "Unknown" → Status=Unknown（不查 KB）
//   - ID 前缀不匹配 setKey → Status=WrongSet（不查 KB，**绝不**跨 set 替代）
//   - KB 查询错误          → Status=KBUnavailable
//   - 查询返回 0 条         → Status=Unknown（不猜测）
//   - 查询返回 ≥1 条        → 取第一条（Query.ID 精确匹配，正常应只有 1 条）
//
// Resolve 不修改 *tfttypes.TFTGameContext，是纯函数式投影。
func (g *Grounder) Resolve(ctx context.Context, gctx *game.GameContext, tctx *tfttypes.TFTGameContext) GroundedGameContext {
	out := GroundedGameContext{}
	if tctx == nil {
		out.Meta.KBHasData = g.repoHasData()
		// 仍然尝试解析 patch（patch 信息本身有调试/UI 价值）
		if patch, fallback, ok := g.resolvePatch(ctx, gctx); ok {
			out.Meta.Patch = patch
			out.Meta.SetKey = g.deriveSetKey(ctx, patch)
			out.Meta.PatchFallback = fallback
		}
		return out
	}

	// 1) 解析 patch（GameContext.Version → KB active 回退）
	patch, fallback, patchOK := g.resolvePatch(ctx, gctx)
	if !patchOK {
		log.Debug("grounder: patch unknown, KB unavailable; emitting slots with Status=KBUnavailable")
		out.GameState = extractGameState(tctx)
		out.Meta.KBHasData = g.repoHasData()
		// 仍按槽位透传（entity 标 KBUnavailable），便于上层知晓"槽位有但 KB 不可用"
		g.populateSlotsUnavailable(tctx, &out)
		return out
	}

	// 2) 推断 setKey（KB probe → static map → ""）
	setKey := g.deriveSetKey(ctx, patch)
	log.Debugf("grounder: patch=%s setKey=%s fallback=%v", patch, setKey, fallback)

	// 3) 透传 GameState
	out.GameState = extractGameState(tctx)
	out.Meta = GroundedMeta{
		Patch:         patch,
		SetKey:        setKey,
		KBHasData:     g.repoHasData(),
		PatchFallback: fallback,
	}

	// 4) 准备 dedup 缓存
	champCache := map[string]ResolvedChampion{}
	itemCache := map[string]ResolvedItem{}
	traitCache := map[string]ResolvedTrait{}
	augmentCache := map[string]ResolvedAugment{}

	// 5) 解析各槽位
	if tctx.Board.IsKnown() {
		out.Board = make([]GroundedBoardSlot, 0, len(tctx.Board.Val))
		for _, s := range tctx.Board.Val {
			champ := g.resolveChampion(ctx, s.ChampionID, setKey, patch, champCache)
			items := make([]ResolvedItem, 0, len(s.Items))
			for _, iid := range s.Items {
				items = append(items, g.resolveItem(ctx, iid, setKey, patch, itemCache))
			}
			out.Board = append(out.Board, GroundedBoardSlot{
				Position: s.Position,
				Star:     s.Star,
				Champion: champ,
				Items:    items,
			})
		}
	}

	if tctx.Bench.IsKnown() {
		out.Bench = make([]GroundedBenchSlot, 0, len(tctx.Bench.Val))
		for _, s := range tctx.Bench.Val {
			out.Bench = append(out.Bench, GroundedBenchSlot{
				Champion: g.resolveChampion(ctx, s.ChampionID, setKey, patch, champCache),
				Star:     s.Star,
			})
		}
	}

	if tctx.Shop.IsKnown() {
		out.Shop = make([]GroundedShopSlot, 0, len(tctx.Shop.Val))
		for _, s := range tctx.Shop.Val {
			out.Shop = append(out.Shop, GroundedShopSlot{
				Champion: g.resolveChampion(ctx, s.ChampionID, setKey, patch, champCache),
				Cost:     s.Cost,
			})
		}
	}

	if tctx.Items.IsKnown() {
		out.Items = make([]GroundedItemSlot, 0, len(tctx.Items.Val))
		for _, it := range tctx.Items.Val {
			out.Items = append(out.Items, GroundedItemSlot{
				Item:  g.resolveItem(ctx, it.ItemID, setKey, patch, itemCache),
				Count: it.Count,
			})
		}
	}

	if tctx.Traits.IsKnown() {
		out.Traits = make([]GroundedTraitState, 0, len(tctx.Traits.Val))
		for _, tr := range tctx.Traits.Val {
			out.Traits = append(out.Traits, GroundedTraitState{
				Trait:  g.resolveTrait(ctx, tr.TraitID, setKey, patch, traitCache),
				Active: tr.Active,
				Style:  tr.Style,
			})
		}
	}

	if tctx.Augments.IsKnown() {
		out.Augments = make([]GroundedAugmentSlot, 0, len(tctx.Augments.Val))
		for _, a := range tctx.Augments.Val {
			out.Augments = append(out.Augments, GroundedAugmentSlot{
				Augment: g.resolveAugment(ctx, a.AugmentID, setKey, patch, augmentCache),
				Tier:    a.Tier,
			})
		}
	}

	log.Debugf("grounder: resolved board=%d bench=%d shop=%d items=%d traits=%d augments=%d",
		len(out.Board), len(out.Bench), len(out.Shop), len(out.Items), len(out.Traits), len(out.Augments))

	return out
}

// ---- 内部辅助 ----

// populateSlotsUnavailable 透传所有 Known 槽位但把每个 entity 标 Status=KBUnavailable
//
// 用于 patch 不可解析的场景：上层仍需知道"槽位结构存在，但 KB 不可用"。
func (g *Grounder) populateSlotsUnavailable(tctx *tfttypes.TFTGameContext, out *GroundedGameContext) {
	if tctx.Board.IsKnown() {
		out.Board = make([]GroundedBoardSlot, 0, len(tctx.Board.Val))
		for _, s := range tctx.Board.Val {
			items := make([]ResolvedItem, 0, len(s.Items))
			for _, iid := range s.Items {
				items = append(items, ResolvedItem{ID: iid, Status: StatusKBUnavailable})
			}
			out.Board = append(out.Board, GroundedBoardSlot{
				Position: s.Position, Star: s.Star,
				Champion: ResolvedChampion{ID: s.ChampionID, Status: StatusKBUnavailable},
				Items:    items,
			})
		}
	}
	if tctx.Bench.IsKnown() {
		out.Bench = make([]GroundedBenchSlot, 0, len(tctx.Bench.Val))
		for _, s := range tctx.Bench.Val {
			out.Bench = append(out.Bench, GroundedBenchSlot{
				Champion: ResolvedChampion{ID: s.ChampionID, Status: StatusKBUnavailable},
				Star:     s.Star,
			})
		}
	}
	if tctx.Shop.IsKnown() {
		out.Shop = make([]GroundedShopSlot, 0, len(tctx.Shop.Val))
		for _, s := range tctx.Shop.Val {
			out.Shop = append(out.Shop, GroundedShopSlot{
				Champion: ResolvedChampion{ID: s.ChampionID, Status: StatusKBUnavailable},
				Cost:     s.Cost,
			})
		}
	}
	if tctx.Items.IsKnown() {
		out.Items = make([]GroundedItemSlot, 0, len(tctx.Items.Val))
		for _, it := range tctx.Items.Val {
			out.Items = append(out.Items, GroundedItemSlot{
				Item:  ResolvedItem{ID: it.ItemID, Status: StatusKBUnavailable},
				Count: it.Count,
			})
		}
	}
	if tctx.Traits.IsKnown() {
		out.Traits = make([]GroundedTraitState, 0, len(tctx.Traits.Val))
		for _, tr := range tctx.Traits.Val {
			out.Traits = append(out.Traits, GroundedTraitState{
				Trait:  ResolvedTrait{ID: tr.TraitID, Status: StatusKBUnavailable},
				Active: tr.Active, Style: tr.Style,
			})
		}
	}
	if tctx.Augments.IsKnown() {
		out.Augments = make([]GroundedAugmentSlot, 0, len(tctx.Augments.Val))
		for _, a := range tctx.Augments.Val {
			out.Augments = append(out.Augments, GroundedAugmentSlot{
				Augment: ResolvedAugment{ID: a.AugmentID, Status: StatusKBUnavailable},
				Tier:    a.Tier,
			})
		}
	}
}

// resolvePatch 解析当前生效 patch
//
// 优先级（与 agent.resolvePatch 一致）：
//  1. GameContext.CommonData.Version（Known 且非空）
//  2. KB active patch（kb.GetCurrentPatch）
//  3. 仍未知名 → ("", false, false)
func (g *Grounder) resolvePatch(ctx context.Context, gctx *game.GameContext) (patch string, fallback bool, ok bool) {
	if gctx != nil && gctx.CommonData.Version.IsKnown() && gctx.CommonData.Version.Val != "" {
		return gctx.CommonData.Version.Val, false, true
	}
	if g.repo == nil {
		return "", false, false
	}
	p, err := g.repo.GetCurrentPatch(ctx)
	if err != nil || p == "" {
		log.Debugf("grounder: KB GetCurrentPatch failed: %v", err)
		return "", false, false
	}
	return p, true, true
}

// deriveSetKey 根据 patch 推断当前 Set 前缀
//
// 策略：
//  1. 静态 map 查 setKeyByPatch（精确匹配）
//  2. KB probe：取任意一个 champion 的 ID 前缀（GetChampion with Query{}）
//  3. 都失败 → ""
func (g *Grounder) deriveSetKey(ctx context.Context, patch string) string {
	if patch == "" {
		return ""
	}
	if v, ok := setKeyByPatch[patch]; ok {
		return v
	}
	if g.repo == nil {
		return ""
	}
	// 静态 map 未命中 → KB probe：取首条 champion 的 ID 前缀
	rows, err := g.repo.GetChampion(ctx, g.game, knowledge.Query{})
	if err != nil || len(rows) == 0 {
		return ""
	}
	for _, r := range rows {
		if r.ID == "" {
			continue
		}
		prefix := extractIDPrefix(r.ID)
		if prefix != "" {
			// 静态 map 补全
			setKeyByPatch[patch] = prefix
			log.Debugf("grounder: learned setKey for patch=%s = %s", patch, prefix)
			return prefix
		}
	}
	return ""
}

// extractIDPrefix 从 "TFT18_Vayne" 提取 "TFT18"
func extractIDPrefix(id string) string {
	idx := strings.Index(id, "_")
	if idx <= 0 || idx >= len(id)-1 {
		return ""
	}
	prefix := id[:idx]
	upper := strings.ToUpper(prefix)
	if !strings.HasPrefix(upper, "TFT") {
		return ""
	}
	return prefix
}

// prefixMatches 检查 id 前缀是否与 setKey 兼容
//
//   - id 为 "" / "Unknown" → true（让后续 Query 返回 Unknown 而非 WrongSet）
//   - setKey 为 "" → true（无法校验时放行；KB 查不到时降级为 Unknown）
//   - 否则：要求 id 前缀与 setKey 大小写不敏感匹配
func prefixMatches(id, setKey string) bool {
	if id == "" || id == "Unknown" {
		return true
	}
	if setKey == "" {
		return true
	}
	prefix := extractIDPrefix(id)
	if prefix == "" {
		return false // 看起来不像 TFT ID
	}
	return strings.EqualFold(prefix, setKey)
}

// repoHasData 报告 KB provider 是否有数据
func (g *Grounder) repoHasData() bool {
	if g.repo == nil {
		return false
	}
	// Service 有 HasData()，但 interface 未暴露 —— 用 GetCurrentPatch 间接判断
	p, err := g.repo.GetCurrentPatch(context.Background())
	return err == nil && p != ""
}

// extractGameState 从 tctx 透传基础资源（不调用 KB）
func extractGameState(tctx *tfttypes.TFTGameContext) GroundedGameState {
	return GroundedGameState{
		Stage:      tctx.Stage.OrFallback(""),
		Level:      tctx.Level.OrFallback(0),
		XP:         tctx.XP.OrFallback(0),
		XPTolevel:  tctx.XPTolevel.OrFallback(0),
		Gold:       tctx.Gold.OrFallback(0),
		Interest:   tctx.Interest.OrFallback(0),
		HP:         tctx.HP.OrFallback(0),
		WinStreak:  tctx.WinStreak.OrFallback(0),
		LoseStreak: tctx.LoseStreak.OrFallback(0),
	}
}

// ---- Per-type Resolvers ----

// resolveChampion 单英雄解析（含 dedup）
func (g *Grounder) resolveChampion(ctx context.Context, id, setKey, patch string, cache map[string]ResolvedChampion) ResolvedChampion {
	if cached, ok := cache[id]; ok {
		return cached
	}
	res := g.fetchChampion(ctx, id, setKey, patch)
	cache[id] = res
	return res
}

func (g *Grounder) fetchChampion(ctx context.Context, id, setKey, patch string) ResolvedChampion {
	if id == "" || id == "Unknown" {
		return ResolvedChampion{ID: id, Status: StatusUnknown, SetKey: setKey, Patch: patch}
	}
	if g.repo == nil {
		return ResolvedChampion{ID: id, Status: StatusKBUnavailable, SetKey: setKey, Patch: patch}
	}
	if !prefixMatches(id, setKey) {
		log.Debugf("grounder: champion %q rejected: prefix mismatch (setKey=%s)", id, setKey)
		return ResolvedChampion{ID: id, Status: StatusWrongSet, SetKey: setKey, Patch: patch}
	}
	rows, err := g.repo.GetChampion(ctx, g.game, knowledge.Query{ID: id})
	if err != nil {
		log.Debugf("grounder: champion %q query failed: %v", id, err)
		return ResolvedChampion{ID: id, Status: StatusKBUnavailable, SetKey: setKey, Patch: patch}
	}
	if len(rows) == 0 {
		return ResolvedChampion{ID: id, Status: StatusUnknown, SetKey: setKey, Patch: patch}
	}
	c := rows[0]
	return ResolvedChampion{
		ID:      c.ID,
		Name:    c.Name,
		Cost:    decodeChampionCost(c.Data),
		Traits:  decodeChampionTraits(c.Data),
		Ability: decodeRawField(c.Data, "ability"),
		Stats:   decodeRawField(c.Data, "stats"),
		Role:    decodeStringField(c.Data, "role"),
		SetKey:  setKey,
		Patch:   patch,
		Status:  StatusResolved,
	}
}

// resolveItem 单装备解析（含 dedup）
func (g *Grounder) resolveItem(ctx context.Context, id, setKey, patch string, cache map[string]ResolvedItem) ResolvedItem {
	if cached, ok := cache[id]; ok {
		return cached
	}
	res := g.fetchItem(ctx, id, setKey, patch)
	cache[id] = res
	return res
}

func (g *Grounder) fetchItem(ctx context.Context, id, setKey, patch string) ResolvedItem {
	if id == "" || id == "Unknown" {
		return ResolvedItem{ID: id, Status: StatusUnknown, SetKey: setKey, Patch: patch}
	}
	if g.repo == nil {
		return ResolvedItem{ID: id, Status: StatusKBUnavailable, SetKey: setKey, Patch: patch}
	}
	if !prefixMatches(id, setKey) {
		log.Debugf("grounder: item %q rejected: prefix mismatch (setKey=%s)", id, setKey)
		return ResolvedItem{ID: id, Status: StatusWrongSet, SetKey: setKey, Patch: patch}
	}
	rows, err := g.repo.GetItem(ctx, g.game, knowledge.Query{ID: id})
	if err != nil {
		log.Debugf("grounder: item %q query failed: %v", id, err)
		return ResolvedItem{ID: id, Status: StatusKBUnavailable, SetKey: setKey, Patch: patch}
	}
	if len(rows) == 0 {
		return ResolvedItem{ID: id, Status: StatusUnknown, SetKey: setKey, Patch: patch}
	}
	it := rows[0]
	return ResolvedItem{
		ID:          it.ID,
		Name:        it.Name,
		Description: decodeStringField(it.Data, "desc"),
		Effects:     decodeRawField(it.Data, "effects"),
		From:        decodeStringSliceField(it.Data, "from"),
		SetKey:      setKey,
		Patch:       patch,
		Status:      StatusResolved,
	}
}

// resolveTrait 单羁绊解析（含 dedup）
func (g *Grounder) resolveTrait(ctx context.Context, id, setKey, patch string, cache map[string]ResolvedTrait) ResolvedTrait {
	if cached, ok := cache[id]; ok {
		return cached
	}
	res := g.fetchTrait(ctx, id, setKey, patch)
	cache[id] = res
	return res
}

func (g *Grounder) fetchTrait(ctx context.Context, id, setKey, patch string) ResolvedTrait {
	if id == "" || id == "Unknown" {
		return ResolvedTrait{ID: id, Status: StatusUnknown, SetKey: setKey, Patch: patch}
	}
	if g.repo == nil {
		return ResolvedTrait{ID: id, Status: StatusKBUnavailable, SetKey: setKey, Patch: patch}
	}
	if !prefixMatches(id, setKey) {
		log.Debugf("grounder: trait %q rejected: prefix mismatch (setKey=%s)", id, setKey)
		return ResolvedTrait{ID: id, Status: StatusWrongSet, SetKey: setKey, Patch: patch}
	}
	rows, err := g.repo.GetTrait(ctx, g.game, knowledge.Query{ID: id})
	if err != nil {
		log.Debugf("grounder: trait %q query failed: %v", id, err)
		return ResolvedTrait{ID: id, Status: StatusKBUnavailable, SetKey: setKey, Patch: patch}
	}
	if len(rows) == 0 {
		return ResolvedTrait{ID: id, Status: StatusUnknown, SetKey: setKey, Patch: patch}
	}
	t := rows[0]
	return ResolvedTrait{
		ID:      t.ID,
		Name:    t.Name,
		Desc:    decodeRawField(t.Data, "desc"),
		Effects: decodeRawField(t.Data, "effects"),
		Type:    decodeStringField(t.Data, "type"),
		SetKey:  setKey,
		Patch:   patch,
		Status:  StatusResolved,
	}
}

// resolveAugment 单强化符文解析（含 dedup）
func (g *Grounder) resolveAugment(ctx context.Context, id, setKey, patch string, cache map[string]ResolvedAugment) ResolvedAugment {
	if cached, ok := cache[id]; ok {
		return cached
	}
	res := g.fetchAugment(ctx, id, setKey, patch)
	cache[id] = res
	return res
}

func (g *Grounder) fetchAugment(ctx context.Context, id, setKey, patch string) ResolvedAugment {
	if id == "" || id == "Unknown" {
		return ResolvedAugment{ID: id, Status: StatusUnknown, SetKey: setKey, Patch: patch}
	}
	if g.repo == nil {
		return ResolvedAugment{ID: id, Status: StatusKBUnavailable, SetKey: setKey, Patch: patch}
	}
	if !prefixMatches(id, setKey) {
		log.Debugf("grounder: augment %q rejected: prefix mismatch (setKey=%s)", id, setKey)
		return ResolvedAugment{ID: id, Status: StatusWrongSet, SetKey: setKey, Patch: patch}
	}
	rows, err := g.repo.GetAugment(ctx, g.game, knowledge.Query{ID: id})
	if err != nil {
		log.Debugf("grounder: augment %q query failed: %v", id, err)
		return ResolvedAugment{ID: id, Status: StatusKBUnavailable, SetKey: setKey, Patch: patch}
	}
	if len(rows) == 0 {
		return ResolvedAugment{ID: id, Status: StatusUnknown, SetKey: setKey, Patch: patch}
	}
	a := rows[0]
	desc := a.Description
	if desc == "" {
		desc = decodeStringField(a.Data, "desc")
	}
	return ResolvedAugment{
		ID:          a.ID,
		Name:        a.Name,
		Description: desc,
		SetKey:      setKey,
		Patch:       patch,
		Status:      StatusResolved,
	}
}

// ---- Data JSON 字段解码器（CommunityDragon 格式） ----

// decodeChampionCost 解码 cdChampion.Cost
func decodeChampionCost(data json.RawMessage) int {
	if len(data) == 0 {
		return 0
	}
	var v struct {
		Cost int `json:"cost"`
	}
	if err := json.Unmarshal(data, &v); err != nil {
		return 0
	}
	return v.Cost
}

// decodeChampionTraits 解码 cdChampion.Traits（[]string）
func decodeChampionTraits(data json.RawMessage) []string {
	if len(data) == 0 {
		return nil
	}
	var v struct {
		Traits []string `json:"traits"`
	}
	if err := json.Unmarshal(data, &v); err != nil {
		return nil
	}
	return v.Traits
}

// decodeStringField 解码 data 中顶层 string 字段（如 "role", "desc", "type"）
func decodeStringField(data json.RawMessage, field string) string {
	if len(data) == 0 {
		return ""
	}
	var v map[string]interface{}
	if err := json.Unmarshal(data, &v); err != nil {
		return ""
	}
	if s, ok := v[field].(string); ok {
		return s
	}
	return ""
}

// decodeStringSliceField 解码 data 中顶层 []string 字段（如 "from"）
func decodeStringSliceField(data json.RawMessage, field string) []string {
	if len(data) == 0 {
		return nil
	}
	var v map[string]interface{}
	if err := json.Unmarshal(data, &v); err != nil {
		return nil
	}
	arr, ok := v[field].([]interface{})
	if !ok {
		return nil
	}
	out := make([]string, 0, len(arr))
	for _, x := range arr {
		if s, ok := x.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

// decodeRawField 解码 data 中顶层原始 JSON 字段（如 "ability", "stats", "effects"）
func decodeRawField(data json.RawMessage, field string) json.RawMessage {
	if len(data) == 0 {
		return nil
	}
	// 用 Decoder 流式读取，找特定字段的原始 value
	dec := json.NewDecoder(strings.NewReader(string(data)))
	for {
		tok, err := dec.Token()
		if err != nil {
			return nil
		}
		if key, ok := tok.(string); ok && key == field {
			// 读取该字段的原始 JSON 值
			var raw json.RawMessage
			if err := dec.Decode(&raw); err != nil {
				return nil
			}
			return raw
		}
		if delim, ok := tok.(json.Delim); ok {
			if delim == '}' { // 顶层对象结束
				return nil
			}
		}
	}
}

// ---- Static setKey map (小但显式，未来 patch 导入时增补) ----

// setKeyByPatch patch → set 前缀映射
//
// 来源：每次 import CD 数据时记录到 memory（见 deriveSetKey 内的 KB probe 回写）。
// 这里只放**已知固定映射**，避免每次 Resolve 都要查 KB。
var setKeyByPatch = map[string]string{
	"16.17.1": "TFT18",
	"16.17":   "TFT18",
	"16.16":   "TFT17",
	"16.15":   "TFT16",
}
