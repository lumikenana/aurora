// Package game - GameContext 通用数据结构、生命周期状态、Provider 扩展接口
//
// Phase 10-6 范围：
//   - 定义 ContextPhase / ContextSource / CommonData / WindowInfo
//   - 定义 ContextProvider / ContextInput / ContextDelta（骨架）
//   - 不实现任何具体的 Provider（OCR / Vision / GameAPI 留待后续阶段）
package game

import (
	"context"
)

// ContextPhase 对局生命周期阶段
//
// 旧实现的 GameContext 阶段信息完全靠字段零值推断（HP=0 可能是满血未识别也可能是阵亡），
// 现改为显式状态机：先确定 Phase，再讨论字段。
type ContextPhase string

const (
	// PhaseUnknown 插件未运行 / 不可识别
	PhaseUnknown ContextPhase = "unknown"
	// PhaseNotInGame 检测到客户端但不在对局（如大厅、商店、客户端首页）
	PhaseNotInGame ContextPhase = "not_in_game"
	// PhaseCarousel 选秀 / 抢装备阶段
	PhaseCarousel ContextPhase = "carousel"
	// PhaseDraft 选强化符文 / 选秀（Augment 决策）
	PhaseDraft ContextPhase = "draft"
	// PhaseInGame 正常对局中
	PhaseInGame ContextPhase = "in_game"
	// PhaseGameOver 玩家已被淘汰
	PhaseGameOver ContextPhase = "game_over"
	// PhaseMatchEnded 对局结束（胜利/失败/退出）— 终态，下一次 Detect 应触发 Clear
	PhaseMatchEnded ContextPhase = "match_ended"
)

// ContextSource 数据来源标识
type ContextSource string

const (
	// SourceWindow 来自窗口检测（基础设施，可信度最高）
	SourceWindow ContextSource = "window"
	// SourceVision 来自视觉 LLM 识别
	SourceVision ContextSource = "vision"
	// SourceOCR 来自图像 OCR
	SourceOCR ContextSource = "ocr"
	// SourceGameAPI 来自游戏内 API（未来）
	SourceGameAPI ContextSource = "game_api"
	// SourceManual 来自用户手动指定
	SourceManual ContextSource = "manual"
)

// CommonData 通用游戏数据（所有游戏可共享的字段）
//
// 字段全部使用 Value[T] 表达"未识别"和"零值"的区别。
type CommonData struct {
	// Version 游戏客户端版本（如 "16.17.1"）
	Version Value[string]
	// PlayerID 玩家 ID（防止多账号混淆；仅在窗口检测确定唯一玩家时填充）
	PlayerID Value[string]
	// MatchID 对局 ID（用于关联日志；不参与 LLM 决策）
	MatchID Value[string]
}

// ContextInput 各 Provider 的输入
//
// Phase 10-6 仅预留字段，不挂任何具体 Provider。
type ContextInput struct {
	// Screenshot 最近一次截图（字节）
	Screenshot []byte
	// ScreenshotMIME 截图 MIME（如 "image/jpeg"）
	ScreenshotMIME string
	// Extra Provider 特定的附加数据（由调用方与 Provider 自行约定类型）
	Extra any
}

// ContextDelta 一次 Provider 的产出
//
// 只包含本轮"建议写入"的字段，未列出的字段不被修改。
// 字段使用指针，nil = 不修改。
type ContextDelta struct {
	// Phase 建议写入的阶段（nil = 不修改）
	Phase *ContextPhase
	// Common 建议写入的通用数据（nil = 不修改）
	Common *CommonData
	// GameData 建议写入的游戏特定数据（如 *tft.TFTGameContext）
	// 调用方应将 GameData 整体替换为新对象，不做字段级合并
	GameData any
}

// ContextProvider 上下文数据提供者（未来 OCR / Vision / GameAPI 各实现一个）
//
// Phase 10-6 仅定义接口，不实现任何具体 Provider。
// 设计要点：
//   - Provider 是"无状态"的纯函数，输入 ContextInput 输出 ContextDelta
//   - 所有 state 在 *GameContext 中（Manager 持有 lastKnown）
//   - 多个 Provider 串行执行，Order 决定先后
//   - WindowProvider 总是最先执行（填 Phase/Window）
//   - Vision/OCR Provider 后续执行（填具体字段）
type ContextProvider interface {
	// Source 数据来源标识
	Source() ContextSource
	// Order 优先级（数字小 = 先跑；WindowProvider = 0）
	Order() int
	// Contribute 计算本轮应当合并的字段（纯函数）
	Contribute(ctx context.Context, in ContextInput) (*ContextDelta, error)
}

// GameDataMerger 游戏特定数据合并接口（Phase 10-7）
//
// GameContext.GameData 字段是 any 类型。
// 当多个 Provider 增量更新同一 GameData 时，需要支持字段级合并：
//   - 若 current.GameData 和 newGameData 都实现本接口
//     → 调用 current.MergeFrom(newGameData)，仅覆盖 newGameData 中 Present=true 的字段
//   - 否则 → 整体替换（Phase 10-6 行为）
//
// 本接口的存在避免了 game 包对 tft 等具体游戏包的依赖（避免循环导入）。
// TFTGameContext 在 tft 包内实现本接口。
type GameDataMerger interface {
	// MergeFrom 从 other 合并字段到自身
	// 实现约定：仅覆盖 other 中 Present=true 的 Value 字段
	MergeFrom(other any)
}
