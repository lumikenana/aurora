// Package tools - get_tft_game_state 工具
//
// Phase 11-1：只读工具。
//   - 读取 game.Manager.Snapshot()（不触发 DetectActive / 不截图 / 不 OCR）
//   - 返回当前已识别的 GameContext 字段
//   - 未识别字段以 known=false 显式表达，绝不猜为 0
//   - 不得自动查询知识库 / 不得展开 Champion 详情
//
// Phase 17-A：本文件 import 改为 tfttypes（不再 import tft 父包）。
// 原因：tft 父包要 import 本 tools 包做 RegisterTools，父子互引 → cycle。
// tfttypes 是 tft 父包的子包，类型上"等价"于 tfttypes.TFTGameContext（type alias），
// 但 import 方向单向（tools → tfttypes）→ 无 cycle。
package tools

import (
	"context"
	"encoding/json"

	"github.com/lipipi/aurora/internal/errors"
	"github.com/lipipi/aurora/internal/game"
	"github.com/lipipi/aurora/internal/game/plugins/tft/tfttypes"
	"github.com/lipipi/aurora/internal/tool"
)

// GameStateProvider 提供当前 GameContext 快照（Phase 11-1 工具边界）
//
// Tool 只调用 Snapshot()，不触发检测 / 截图 / OCR。
// 这保证多次连续工具调用看到的状态一致（避免在一次回答里状态前后跳变）。
type GameStateProvider interface {
	Snapshot() *game.GameContext
}

// TFTGameStateTool 返回当前 TFT 对局状态（只读）
type TFTGameStateTool struct {
	state GameStateProvider
}

// NewTFTGameStateTool 工厂
func NewTFTGameStateTool(cfg tool.Config) (tool.Tool, errors.AppError) {
	return &TFTGameStateTool{}, nil
}

// SetGameStateProvider 注入 GameStateProvider（由 Agent 在初始化时调用）
func (t *TFTGameStateTool) SetGameStateProvider(p GameStateProvider) {
	t.state = p
}

// Name 工具名（LLM 调用标识）
func (t *TFTGameStateTool) Name() string { return "get_tft_game_state" }

// Description 工具描述（LLM 阅读）
func (t *TFTGameStateTool) Description() string {
	return "获取当前 TFT 对局的实时游戏状态。返回当前已识别的数据，未识别的字段 known=false（不代表 0 / 空）。`patch` 字段为当前对局识别到的游戏版本，未识别时为空字符串。仅返回原始状态，不自动查询英雄/装备/羁绊知识库——若需要查询知识，请使用 get_tft_champion / get_tft_item / get_tft_trait / get_tft_augment。"
}

// InputSchema 输入参数 schema
func (t *TFTGameStateTool) InputSchema() string {
	return `{
  "type": "object",
  "properties": {},
  "required": []
}`
}

// IsAvailable 是否可用
//
// 始终进入 registry：缺依赖时由 Execute 内部返回 error。
func (t *TFTGameStateTool) IsAvailable() bool {
	return true
}

// gameStateOutput Tool 输出的统一结构
type gameStateOutput struct {
	Available   bool                   `json:"available"`
	Game        string                 `json:"game,omitempty"`
	Name        string                 `json:"name,omitempty"`
	Phase       string                 `json:"phase,omitempty"`
	Patch       string                 `json:"patch,omitempty"`     // 来源: CommonData.Version（Present 时填充）
	PatchKnown  bool                   `json:"patch_known"`         // Version.Present
	WindowTitle string                 `json:"window_title,omitempty"`
	NotTFT      bool                   `json:"not_tft,omitempty"`   // 检到非 TFT 游戏时为 true
	TFT         *tftStateBlock         `json:"tft,omitempty"`       // 仅当 GameData 是 *TFTGameContext 时填充
}

// tftStateBlock TFT 特有字段
type tftStateBlock struct {
	Stage     string             `json:"stage,omitempty"`
	Level     valueInt           `json:"level"`
	XP        valueInt           `json:"xp"`
	XPTolevel valueInt           `json:"xp_to_level"`
	Gold      valueInt           `json:"gold"`
	Interest  valueInt           `json:"interest"`
	HP        valueInt           `json:"hp"`
	WinStreak valueInt           `json:"win_streak"`
	LoseStreak valueInt          `json:"lose_streak"`
	Board     valueBoard         `json:"board"`
	Bench     valueBench         `json:"bench"`
	Shop      valueShop          `json:"shop"`
	Items     valueItems         `json:"items"`
	Traits    valueTraits        `json:"traits"`
	Augments  valueAugments      `json:"augments"`
	Carousel  valueCarousel      `json:"carousel"`
	Opponents valueOpponents     `json:"opponents"`
}

// valueInt 整型 Value 序列化
type valueInt struct {
	Known      bool    `json:"known"`
	Value      int     `json:"value,omitempty"`
	Confidence float64 `json:"confidence,omitempty"`
	Source     string  `json:"source,omitempty"`
}

func newValueInt(v game.Value[int]) valueInt {
	if !v.Present {
		return valueInt{Known: false}
	}
	return valueInt{
		Known:      true,
		Value:      v.Val,
		Confidence: v.Confidence,
		Source:     string(v.Source),
	}
}

// valueString 字符串 Value 序列化
type valueString struct {
	Known      bool    `json:"known"`
	Value      string  `json:"value,omitempty"`
	Confidence float64 `json:"confidence,omitempty"`
	Source     string  `json:"source,omitempty"`
}

func newValueString(v game.Value[string]) valueString {
	if !v.Present {
		return valueString{Known: false}
	}
	return valueString{
		Known:      true,
		Value:      v.Val,
		Confidence: v.Confidence,
		Source:     string(v.Source),
	}
}

// valueBoard / valueBench / valueShop / valueItems / valueTraits / valueAugments
// 统一表达 Present + 内层 []T（id-only，不展开 name 等大字段）
type valueBoard struct {
	Known  bool        `json:"known"`
	Items  []boardLite `json:"items,omitempty"`
}

type boardLite struct {
	ChampionID string   `json:"champion_id"`
	Star       int      `json:"star"`
	Items      []string `json:"items,omitempty"`
	Position   int      `json:"position"`
}

type valueBench struct {
	Known bool       `json:"known"`
	Items []benchLite `json:"items,omitempty"`
}

type benchLite struct {
	ChampionID string `json:"champion_id"`
	Star       int    `json:"star"`
}

type valueShop struct {
	Known bool      `json:"known"`
	Items []shopLite `json:"items,omitempty"`
}

type shopLite struct {
	ChampionID string `json:"champion_id"`
	Cost       int    `json:"cost"`
}

type valueItems struct {
	Known bool       `json:"known"`
	Items []itemLite `json:"items,omitempty"`
}

type itemLite struct {
	ItemID string `json:"item_id"`
	Count  int    `json:"count"`
}

type valueTraits struct {
	Known bool         `json:"known"`
	Items []traitState `json:"items,omitempty"`
}

type traitState struct {
	TraitID string `json:"trait_id"`
	Active  int    `json:"active"`
	Style   int    `json:"style"`
}

type valueAugments struct {
	Known bool          `json:"known"`
	Items []augmentLite `json:"items,omitempty"`
}

type augmentLite struct {
	AugmentID string `json:"augment_id"`
	Tier      string `json:"tier,omitempty"`
}

type valueCarousel struct {
	Known  bool           `json:"known"`
	Round  string         `json:"round,omitempty"`
	Offerings []string    `json:"offerings,omitempty"`
	PickedIndex int        `json:"picked_index,omitempty"`
}

type valueOpponents struct {
	Known    bool          `json:"known"`
	Items    []opponentLite `json:"items,omitempty"`
}

type opponentLite struct {
	PlayerName string   `json:"player_name,omitempty"`
	HP         int      `json:"hp,omitempty"`
	Stage      string   `json:"stage,omitempty"`
	Board      []boardLite `json:"board,omitempty"`
}

// Execute 执行工具
func (t *TFTGameStateTool) Execute(ctx context.Context, _ json.RawMessage) (*tool.ToolResult, errors.AppError) {
	if t.state == nil {
		return &tool.ToolResult{
			Success: false,
			Error:   "game state provider not configured",
		}, errors.New(errors.CodeInvalidConfig, "game state provider not configured")
	}

	gctx := t.state.Snapshot()
	if gctx == nil {
		out := gameStateOutput{Available: false}
		return jsonResult(out), nil
	}

	out := gameStateOutput{
		Available:   true,
		Game:        gctx.Game,
		Name:        gctx.Name,
		Phase:       string(gctx.Phase),
		WindowTitle: gctx.Window.Title,
	}

	// Patch 来自 CommonData.Version（Present 时填充）
	if gctx.CommonData.Version.IsKnown() {
		out.Patch = gctx.CommonData.Version.Val
		out.PatchKnown = true
	}

	// GameData 仅当 *TFTGameContext 时展开；其他游戏类型标记 not_tft
	if gctx.GameData != nil {
		if tctx, ok := gctx.GameData.(*tfttypes.TFTGameContext); ok {
			out.TFT = tftStateToBlock(tctx)
		} else {
			out.NotTFT = true
		}
	}

	return jsonResult(out), nil
}

func tftStateToBlock(tctx *tfttypes.TFTGameContext) *tftStateBlock {
	if tctx == nil {
		return nil
	}
	b := &tftStateBlock{
		Stage:      tctx.Stage.Val, // 直接用值，Known 通过 Known bool 表达
		Level:      newValueInt(tctx.Level),
		XP:         newValueInt(tctx.XP),
		XPTolevel:  newValueInt(tctx.XPTolevel),
		Gold:       newValueInt(tctx.Gold),
		Interest:   newValueInt(tctx.Interest),
		HP:         newValueInt(tctx.HP),
		WinStreak:  newValueInt(tctx.WinStreak),
		LoseStreak: newValueInt(tctx.LoseStreak),
	}
	// Stage: 用 valueString 形式
	b.Stage = tctx.Stage.Val
	if tctx.Board.IsKnown() {
		b.Board = valueBoard{Known: true, Items: toBoardLite(tctx.Board.Val)}
	}
	if tctx.Bench.IsKnown() {
		b.Bench = valueBench{Known: true, Items: toBenchLite(tctx.Bench.Val)}
	}
	if tctx.Shop.IsKnown() {
		b.Shop = valueShop{Known: true, Items: toShopLite(tctx.Shop.Val)}
	}
	if tctx.Items.IsKnown() {
		b.Items = valueItems{Known: true, Items: toItemLite(tctx.Items.Val)}
	}
	if tctx.Traits.IsKnown() {
		b.Traits = valueTraits{Known: true, Items: toTraitState(tctx.Traits.Val)}
	}
	if tctx.Augments.IsKnown() {
		b.Augments = valueAugments{Known: true, Items: toAugmentLite(tctx.Augments.Val)}
	}
	if tctx.Carousel.IsKnown() && tctx.Carousel.Val != nil {
		b.Carousel = valueCarousel{
			Known:       true,
			Round:       tctx.Carousel.Val.Round,
			Offerings:   tctx.Carousel.Val.Offerings,
			PickedIndex: tctx.Carousel.Val.PickedIndex,
		}
	}
	if tctx.Opponents.IsKnown() {
		b.Opponents = valueOpponents{Known: true, Items: toOpponentLite(tctx.Opponents.Val)}
	}
	return b
}

func toBoardLite(in []tfttypes.BoardSlot) []boardLite {
	out := make([]boardLite, 0, len(in))
	for _, s := range in {
		out = append(out, boardLite{
			ChampionID: s.ChampionID,
			Star:       s.Star,
			Items:      s.Items,
			Position:   s.Position,
		})
	}
	return out
}

func toBenchLite(in []tfttypes.BenchSlot) []benchLite {
	out := make([]benchLite, 0, len(in))
	for _, s := range in {
		out = append(out, benchLite{ChampionID: s.ChampionID, Star: s.Star})
	}
	return out
}

func toShopLite(in []tfttypes.ShopSlot) []shopLite {
	out := make([]shopLite, 0, len(in))
	for _, s := range in {
		out = append(out, shopLite{ChampionID: s.ChampionID, Cost: s.Cost})
	}
	return out
}

func toItemLite(in []tfttypes.ItemSlot) []itemLite {
	out := make([]itemLite, 0, len(in))
	for _, s := range in {
		out = append(out, itemLite{ItemID: s.ItemID, Count: s.Count})
	}
	return out
}

func toTraitState(in []tfttypes.TraitState) []traitState {
	out := make([]traitState, 0, len(in))
	for _, s := range in {
		out = append(out, traitState{TraitID: s.TraitID, Active: s.Active, Style: s.Style})
	}
	return out
}

func toAugmentLite(in []tfttypes.AugmentSlot) []augmentLite {
	out := make([]augmentLite, 0, len(in))
	for _, s := range in {
		out = append(out, augmentLite{AugmentID: s.AugmentID, Tier: s.Tier})
	}
	return out
}

func toOpponentLite(in []tfttypes.Opponent) []opponentLite {
	out := make([]opponentLite, 0, len(in))
	for _, o := range in {
		out = append(out, opponentLite{
			PlayerName: o.PlayerName,
			HP:         o.HP,
			Stage:      o.Stage,
			Board:      toBoardLite(o.Board),
		})
	}
	return out
}

// jsonResult 工具函数：把任意对象包装成 JSON 类型的 ToolResult
func jsonResult(v interface{}) *tool.ToolResult {
	data, _ := json.Marshal(v)
	return &tool.ToolResult{
		Type:    tool.ToolResultTypeJSON,
		Data:    data,
		Success: true,
	}
}
