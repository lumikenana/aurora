// Package tools - get_tft_champion / get_tft_item / get_tft_trait / get_tft_augment
//
// Phase 11-1：只读知识库查询工具。
//   - 严格限定 (patch, game, mode) 三个维度，绝不跨版本返回数据
//   - patch 来自 GameContext.CommonData.Version（避免猜测）
//   - 未识别 patch → 工具返回错误，不得 fallback 到 latest
//   - 数据不存在 → 返回 not_found，不得使用其他 patch 数据替代
//   - Tool 描述中明确写明这些约束，避免 LLM 误用
//
// Phase 17-A 重定位：
//   - KnowledgeProvider 接口在本包内独立定义（不再 import tft 父包复用）
//   - 原因：tft 父包要 import 本 tools 包做 RegisterTools，
//     父子互引 → cycle。本包独立定义 KnowledgeProvider 后：
//     tools → tfttypes（叶→叶单向）
//     tft   → tools  +  → tfttypes（父→叶单向）
//   - 实际实现 *knowledge.Service 同时满足本接口与 tft.KnowledgeProvider
//     （方法集相同，type assertion 在 tft.RegisterTools 内完成）
package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/lipipi/aurora/internal/errors"
	"github.com/lipipi/aurora/internal/game/knowledge"
	"github.com/lipipi/aurora/internal/tool"
)

// KnowledgeProvider 知识库抽象的最小接口（工具包独立定义，避免 tft 父子互引）
//
// 5 个查询方法 + 1 个 patch 解析方法，与 tft.KnowledgeProvider 方法集相同。
// 实际 KB 实现（*knowledge.Service）天然同时满足两个接口。
type KnowledgeProvider interface {
	GetCurrentPatch(ctx context.Context) (string, errors.AppError)
	GetChampion(ctx context.Context, game string, q knowledge.Query) ([]knowledge.Champion, errors.AppError)
	GetItem(ctx context.Context, game string, q knowledge.Query) ([]knowledge.Item, errors.AppError)
	GetTrait(ctx context.Context, game string, q knowledge.Query) ([]knowledge.Trait, errors.AppError)
	GetAugment(ctx context.Context, game string, q knowledge.Query) ([]knowledge.Augment, errors.AppError)
}

// TFTKnowledgeTool 通用 TFT 知识库查询工具（用于 champion / item / trait / augment）
//
// 4 个工具名（get_tft_champion / get_tft_item / get_tft_trait / get_tft_augment）
// 共享同一执行逻辑，仅 kind 字段不同——避免复制粘贴。
type TFTKnowledgeTool struct {
	kind   string // "champion" | "item" | "trait" | "augment"
	desc   string
	schema string
	kb     KnowledgeProvider
	state  GameStateProvider
}

// NewTFTChampionTool / NewTFTItemTool / NewTFTTraitTool / NewTFTAugmentTool
//
// 4 个独立工厂函数 → 4 个独立 tool.Type 注册项。
func NewTFTChampionTool(cfg tool.Config) (tool.Tool, errors.AppError) {
	return newTFTKnowledgeTool("champion"), nil
}

func NewTFTItemTool(cfg tool.Config) (tool.Tool, errors.AppError) {
	return newTFTKnowledgeTool("item"), nil
}

func NewTFTTraitTool(cfg tool.Config) (tool.Tool, errors.AppError) {
	return newTFTKnowledgeTool("trait"), nil
}

func NewTFTAugmentTool(cfg tool.Config) (tool.Tool, errors.AppError) {
	return newTFTKnowledgeTool("augment"), nil
}

func newTFTKnowledgeTool(kind string) *TFTKnowledgeTool {
	t := &TFTKnowledgeTool{kind: kind}
	switch kind {
	case "champion":
		t.desc = "查询当前 TFT 补丁版本下的英雄（champion）数据。`id` 必须是知识库中的 apiName（KB key），例如 \"TFT18_Vayne\"。查询严格限定 (patch, game=tft, mode=standard)，绝不返回其他版本数据。如果 patch 未识别（未在游戏中），工具返回错误——不要用任何其他 patch 数据替代。数据不存在时返回 found=false。"
		t.schema = `{
  "type": "object",
  "properties": {
    "id": {"type": "string", "description": "英雄 apiName（KB key），例如 'TFT18_Vayne'"}
  },
  "required": ["id"]
}`
	case "item":
		t.desc = "查询当前 TFT 补丁版本下的装备（item）数据。`id` 必须是知识库中的 apiName，例如 \"TFT_IE\"（无尽之刃）。查询严格限定 (patch, game=tft, mode=standard)，绝不返回其他版本数据。如果 patch 未识别，工具返回错误。数据不存在时返回 found=false。"
		t.schema = `{
  "type": "object",
  "properties": {
    "id": {"type": "string", "description": "装备 apiName（KB key），例如 'TFT_IE'"}
  },
  "required": ["id"]
}`
	case "trait":
		t.desc = "查询当前 TFT 补丁版本下的羁绊（trait）数据。`id` 必须是知识库中的 apiName，例如 \"TFT18_Bulwark\"。查询严格限定 (patch, game=tft, mode=standard)，绝不返回其他版本数据。如果 patch 未识别，工具返回错误。数据不存在时返回 found=false。"
		t.schema = `{
  "type": "object",
  "properties": {
    "id": {"type": "string", "description": "羁绊 apiName（KB key），例如 'TFT18_Bulwark'"}
  },
  "required": ["id"]
}`
	case "augment":
		t.desc = "查询当前 TFT 补丁版本下的强化符文（augment）数据。`id` 必须是知识库中的 apiName，例如 \"TFT18_Augment_X\"。查询严格限定 (patch, game=tft, mode=standard)，绝不返回其他版本数据。如果 patch 未识别，工具返回错误。数据不存在时返回 found=false。"
		t.schema = `{
  "type": "object",
  "properties": {
    "id": {"type": "string", "description": "强化符文 apiName（KB key）"}
  },
  "required": ["id"]
}`
	}
	return t
}

// SetKnowledgeProvider 注入知识库 provider
func (t *TFTKnowledgeTool) SetKnowledgeProvider(p KnowledgeProvider) { t.kb = p }

// SetGameStateProvider 注入 game state provider（用于读取 patch）
func (t *TFTKnowledgeTool) SetGameStateProvider(p GameStateProvider) { t.state = p }

// Name 工具名
func (t *TFTKnowledgeTool) Name() string { return "get_tft_" + t.kind }

// Description 工具描述
func (t *TFTKnowledgeTool) Description() string { return t.desc }

// InputSchema
func (t *TFTKnowledgeTool) InputSchema() string { return t.schema }

// IsAvailable
//
// 知识工具始终进入 registry：IsAvailable 在 factory 调用时为 true，
// 缺依赖时由 Execute 内部返回 error（比"工具根本不在 registry"更便于 LLM 诊断）。
func (t *TFTKnowledgeTool) IsAvailable() bool {
	return true
}

// knowledgeInput 工具输入
type knowledgeInput struct {
	ID string `json:"id"`
}

// notFoundOutput 通用 not_found 返回
type notFoundOutput struct {
	Found   bool   `json:"found"`
	ID      string `json:"id"`
	Patch   string `json:"patch"`
	Game    string `json:"game"`
	Mode    string `json:"mode"`
	Message string `json:"message,omitempty"`
}

// Execute 执行
//
// 流程：
//  1. 读 GameContext Snapshot → 拿 patch（CommonData.Version）
//  2. patch 为空 → 返回错误
//  3. 校验 game=tft / mode=standard 锁死（tool name 已限定，但显式表达便于审计）
//  4. 调用 KnowledgeProvider.Get*(ctx, "tft", Query{ID: id})
//  5. 0 条 → found=false
//  6. >0 条 → 取第一条，输出最小化字段
func (t *TFTKnowledgeTool) Execute(ctx context.Context, raw json.RawMessage) (*tool.ToolResult, errors.AppError) {
	if t.kb == nil || t.state == nil {
		return &tool.ToolResult{
			Success: false,
			Error:   "knowledge or game state provider not configured",
		}, errors.New(errors.CodeInvalidConfig, "knowledge or game state provider not configured")
	}

	var in knowledgeInput
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &in); err != nil {
			return &tool.ToolResult{
				Success: false,
				Error:   "invalid input: " + err.Error(),
			}, errors.Wrap(errors.CodeInvalidConfig, "invalid input JSON", err)
		}
	}
	if in.ID == "" {
		return &tool.ToolResult{
			Success: false,
			Error:   "missing required field 'id'",
		}, errors.New(errors.CodeInvalidConfig, "missing required field 'id'")
	}

	// 1. 取 patch（严格来自 GameContext，不来自 service）
	gctx := t.state.Snapshot()
	if gctx == nil {
		return &tool.ToolResult{
			Success: false,
			Error:   "no active game detected; cannot determine patch",
		}, errors.New(errors.CodeStorageError, "no active game detected; cannot determine patch")
	}
	if !gctx.CommonData.Version.IsKnown() || gctx.CommonData.Version.Val == "" {
		return &tool.ToolResult{
			Success: false,
			Error:   "current game patch is unknown (尚未识别到游戏版本，无法限定知识库版本)",
		}, errors.New(errors.CodeStorageError, "current game patch is unknown")
	}
	patch := gctx.CommonData.Version.Val

	// 2. game/mode 锁死
	const gameID = "tft"
	const mode = "standard"

	q := knowledge.Query{ID: in.ID}

	switch t.kind {
	case "champion":
		return t.executeChampion(ctx, gameID, patch, mode, q)
	case "item":
		return t.executeItem(ctx, gameID, patch, mode, q)
	case "trait":
		return t.executeTrait(ctx, gameID, patch, mode, q)
	case "augment":
		return t.executeAugment(ctx, gameID, patch, mode, q)
	}

	// unreachable
	return &tool.ToolResult{
		Success: false,
		Error:   fmt.Sprintf("unknown knowledge tool kind: %s", t.kind),
	}, errors.New(errors.CodeInvalidConfig, "unknown knowledge tool kind: "+t.kind)
}

func (t *TFTKnowledgeTool) executeChampion(ctx context.Context, gameID, patch, mode string, q knowledge.Query) (*tool.ToolResult, errors.AppError) {
	rows, err := t.kb.GetChampion(ctx, gameID, q)
	if err != nil {
		return &tool.ToolResult{Success: false, Error: err.Message()}, err
	}
	if len(rows) == 0 {
		return jsonResult(notFoundOutput{
			Found:   false,
			ID:      q.ID,
			Patch:   patch,
			Game:    gameID,
			Mode:    mode,
			Message: "not_found in current patch",
		}), nil
	}
	c := rows[0]
	out := championOutput{
		Found: true,
		ID:    c.ID,
		Name:  c.Name,
		Patch: c.Patch,
		Game:  gameID,
		Mode:  mode,
		Data:  c.Data, // 原始 JSON（受 schema 限制）
	}
	return jsonResult(out), nil
}

func (t *TFTKnowledgeTool) executeItem(ctx context.Context, gameID, patch, mode string, q knowledge.Query) (*tool.ToolResult, errors.AppError) {
	rows, err := t.kb.GetItem(ctx, gameID, q)
	if err != nil {
		return &tool.ToolResult{Success: false, Error: err.Message()}, err
	}
	if len(rows) == 0 {
		return jsonResult(notFoundOutput{
			Found:   false,
			ID:      q.ID,
			Patch:   patch,
			Game:    gameID,
			Mode:    mode,
			Message: "not_found in current patch",
		}), nil
	}
	it := rows[0]
	out := itemOutput{
		Found: true,
		ID:    it.ID,
		Name:  it.Name,
		Patch: it.Patch,
		Game:  gameID,
		Mode:  mode,
		Data:  it.Data,
	}
	return jsonResult(out), nil
}

func (t *TFTKnowledgeTool) executeTrait(ctx context.Context, gameID, patch, mode string, q knowledge.Query) (*tool.ToolResult, errors.AppError) {
	rows, err := t.kb.GetTrait(ctx, gameID, q)
	if err != nil {
		return &tool.ToolResult{Success: false, Error: err.Message()}, err
	}
	if len(rows) == 0 {
		return jsonResult(notFoundOutput{
			Found:   false,
			ID:      q.ID,
			Patch:   patch,
			Game:    gameID,
			Mode:    mode,
			Message: "not_found in current patch",
		}), nil
	}
	tr := rows[0]
	out := traitOutput{
		Found: true,
		ID:    tr.ID,
		Name:  tr.Name,
		Patch: tr.Patch,
		Game:  gameID,
		Mode:  mode,
		Data:  tr.Data,
	}
	return jsonResult(out), nil
}

func (t *TFTKnowledgeTool) executeAugment(ctx context.Context, gameID, patch, mode string, q knowledge.Query) (*tool.ToolResult, errors.AppError) {
	rows, err := t.kb.GetAugment(ctx, gameID, q)
	if err != nil {
		return &tool.ToolResult{Success: false, Error: err.Message()}, err
	}
	if len(rows) == 0 {
		return jsonResult(notFoundOutput{
			Found:   false,
			ID:      q.ID,
			Patch:   patch,
			Game:    gameID,
			Mode:    mode,
			Message: "not_found in current patch",
		}), nil
	}
	a := rows[0]
	out := augmentOutput{
		Found:       true,
		ID:          a.ID,
		Name:        a.Name,
		Patch:       a.Patch,
		Game:        gameID,
		Mode:        mode,
		Description: a.Description,
		Data:        a.Data,
	}
	return jsonResult(out), nil
}

// ---- 输出结构 ----

// championOutput 英雄（最小化：id/name/patch + Data 原始 JSON）
type championOutput struct {
	Found bool            `json:"found"`
	ID    string          `json:"id"`
	Name  string          `json:"name"`
	Patch string          `json:"patch"`
	Game  string          `json:"game"`
	Mode  string          `json:"mode"`
	Data  json.RawMessage `json:"data,omitempty"`
}

// itemOutput 装备
type itemOutput struct {
	Found bool            `json:"found"`
	ID    string          `json:"id"`
	Name  string          `json:"name"`
	Patch string          `json:"patch"`
	Game  string          `json:"game"`
	Mode  string          `json:"mode"`
	Data  json.RawMessage `json:"data,omitempty"`
}

// traitOutput 羁绊
type traitOutput struct {
	Found bool            `json:"found"`
	ID    string          `json:"id"`
	Name  string          `json:"name"`
	Patch string          `json:"patch"`
	Game  string          `json:"game"`
	Mode  string          `json:"mode"`
	Data  json.RawMessage `json:"data,omitempty"`
}

// augmentOutput 强化符文
type augmentOutput struct {
	Found       bool            `json:"found"`
	ID          string          `json:"id"`
	Name        string          `json:"name"`
	Patch       string          `json:"patch"`
	Game        string          `json:"game"`
	Mode        string          `json:"mode"`
	Description string          `json:"description,omitempty"`
	Data        json.RawMessage `json:"data,omitempty"`
}
