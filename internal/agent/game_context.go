// Package agent - GameContext → LLM prompt 摘要的薄包装（Phase 17-A）
//
// 历史：Phase 12-16 期间本文件承担了 TFT 全部 prompt 渲染工作（~600 行）：
//   - FormatTFTGameContext
//   - queryRelevantKB（patch 隔离）
//   - needsGameContext / pickContextDetail（关键词路由）
//   - detectAnalysisOptions（按用户问题切片）
//   - composition / equipment / economy / preparation 五个 analyzer 的串联
//
// Phase 17-A：以上所有逻辑整体迁移到 internal/game/plugins/tft/prompt.go，
// 本文件退化为"通用调度壳"：
//   - 持有一个 *game.Manager 引用
//   - Prepare(ctx, userInput) 读 Manager.LastKnown() → 迭代 Plugins() →
//     对每个实现 game.GameContextRenderer 的插件调用 RenderGameContext
//   - 第一个返回 used=true 的插件决定结果
//
// 不变量：
//   - Agent Core 不知道 tft / 任何具体游戏
//   - 0 LLM 调用（所有分析都是本地 deterministic；Phase 12-16 已落地）
//   - Unknown 字段不输出，Known(0) 正确输出（在 tft plugin 内部保证）
package agent

import (
	"context"

	"github.com/lipipi/aurora/internal/game"
	"github.com/lipipi/aurora/internal/log"
)

// ContextDetail 控制 GameContext Summary 的细节级别（保留类型，Compatibility）
//
// Phase 17-A 后由插件自己决定 detail；Agent Core 不再 pick。
// 保留是为了不破坏外部 API（之前 Prepare 返回 3 元组）。
type ContextDetail string

const (
	// ContextMinimal 最小（仅基础资源）
	ContextMinimal ContextDetail = "minimal"
	// ContextFull 完整（包含装备/经济/备战/Composition 等）
	ContextFull ContextDetail = "full"
)

// GameContextPreparer 通用 GameContext → prompt 摘要调度器
//
// 通过 type assertion 委托给实现 game.GameContextRenderer 的插件。
// Agent Core 不知道任何具体游戏；插件可零或多个。
type GameContextPreparer struct {
	manager *game.Manager
}

// NewGameContextPreparer 构造通用 Preparer
//
// manager == nil 时 Prepare 永远返回 (used=false)。
func NewGameContextPreparer(mgr *game.Manager) *GameContextPreparer {
	return &GameContextPreparer{manager: mgr}
}

// Prepare 决定是否注入 GameContext 摘要
//
// 返回 (summary, detail, used)：
//   - summary == "" && !used → 不注入（无活跃游戏 / 插件判定不相关）
//   - summary != ""          → 可注入的 prompt 片段
//
// detail 当前固定为 ContextFull（Phase 17-A 由插件内部决定；未来可让插件
// 决定 detail 并通过 renderer 返回更细的 signal）。
//
// 调用方负责把 summary 拼到 ExtraSystemPrompt 尾部，不影响对话历史。
func (p *GameContextPreparer) Prepare(ctx context.Context, userInput string) (string, ContextDetail, bool) {
	if p == nil || p.manager == nil {
		return "", "", false
	}
	gctx := p.manager.LastKnown()
	if gctx == nil {
		log.Debug("game_context: skipped reason=snapshot_nil")
		return "", "", false
	}

	// 迭代所有已注册插件；第一个返回 used=true 的决定结果
	for _, pl := range p.manager.Plugins() {
		r, ok := pl.(game.GameContextRenderer)
		if !ok {
			continue
		}
		summary, used := r.RenderGameContext(gctx, userInput)
		if used {
			log.Debugf("game_context: prepared via plugin=%s summary_len=%d", pl.ID(), len(summary))
			return summary, ContextFull, true
		}
	}
	log.Debug("game_context: skipped reason=no_renderer_matched")
	return "", "", false
}
