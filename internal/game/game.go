// Package game 游戏插件抽象层
// Agent Core 依赖本包接口，不依赖具体插件实现（plugins/ 子包仅被 main.go blank import）
package game

import (
	"context"
	"time"

	"github.com/lipipi/aurora/internal/tool"
)

// WindowInfo 枚举到的窗口信息（窗口检测的通用数据结构）
type WindowInfo struct {
	HWND  uintptr
	Class string
	Title string
}

// GameContext 游戏上下文（通用层）
//
// Phase 10-6 重构后：
//   - 窗口信息收敛到 Window 字段（WindowInfo struct）
//   - 新增 Phase 状态机字段
//   - 新增 CommonData 通用游戏数据
//   - 新增 GameData 任意类型槽位（用于挂载 *tft.TFTGameContext 等游戏特定数据）
//   - 删除所有 TFT 特定字段（Version/Stage/Gold/Board 等），由 tft 包在 GameData 内维护
//
// 关键不变量：
//   - 不知道的字段用 Value[T].Present=false 表达，不再用 0/"" 表达 unknown
//   - GameData 任意类型，调用方负责类型断言
//   - WindowHWND 不向 Wails 序列化（uintptr 在前端无意义）
type GameContext struct {
	// Game 插件 ID，如 "tft"
	Game string
	// Name 显示名，如 "云顶之弈"
	Name string

	// Phase 当前对局阶段（生命周期状态机）
	Phase ContextPhase
	// DetectedAt 最近一次更新（任何字段）
	DetectedAt time.Time

	// Window 窗口信息（Phase 1 唯一数据来源）
	Window WindowInfo

	// CommonData 通用游戏数据（所有游戏可共享）
	CommonData CommonData

	// GameData 游戏特定数据（如 *tft.TFTGameContext）
	// 调用方负责类型断言；不持有时为 nil
	GameData any
}

// GamePlugin 游戏插件接口
// 新增游戏（LOL / CS2 等）实现本接口并在 init() 中调用 RegisterPlugin 注册，
// 不需要修改 Agent Core
type GamePlugin interface {
	// ID 唯一标识（小写，如 "tft"）
	ID() string
	// Name 中文名（如 "云顶之弈"）
	Name() string
	// Detect 检测游戏是否正在运行，返回游戏上下文
	// 未检测到返回 (nil, nil)；仅基础设施故障返回 error
	Detect(ctx context.Context) (*GameContext, error)
	// SystemPrompt 返回该游戏的附加系统提示词（可为空串）
	// Agent Core 只负责拼接，不感知游戏语义
	SystemPrompt() string
}

// GameContextRenderer 可选接口：插件可实现以把 GameContext 渲染为 LLM prompt 片段。
//
// 实现应自行判断 gctx.Game == p.ID()，再决定是否渲染。
// 返回 (summary, used)：
//   - used=false  → 跳过注入（不在游戏中 / 不相关问题 / GameContext 全空）
//   - used=true   → summary 是紧凑 prompt 片段，由 Agent 拼到 ExtraSystemPrompt 尾部
//
// Agent Core 不知道插件内部结构：实现可以是"纯 GameContext → 字符串"，
// 也可以是"GameContext → KB 富化 → 分析 → 字符串"，对调用方完全透明。
type GameContextRenderer interface {
	RenderGameContext(ctx *GameContext, userInput string) (summary string, used bool)
}

// ToolContributor 可选接口：插件可向 ToolRegistry 注册自己的工具。
//
// 调用方（Agent）迭代 Manager.Plugins()，对实现了本接口的插件调用
// RegisterTools。插件负责：
//   - 创建自己的 Tool 实例
//   - 注入依赖（KB provider / game state provider 等）
//   - 通过 tool.RegisterTool 或直接修改 registry 注册
//
// 这把"工具注册"从中央 tool/tools/init.go 解耦到插件自己——
// 新增 DoudizhuPlugin 时，工具的注册不会触碰 Agent Core 也不会触碰
// 通用 tool 包。
//
// RegisterTools 的参数类型是 *tool.ToolRegistry；tool 包不依赖 game，
// 所以 game → tool 是单向无环的。
type ToolContributor interface {
	RegisterTools(reg *tool.ToolRegistry) error
}

// SetupReceiver 可选接口：插件可接收由 Agent 注入的 GameStateProvider
// 与 KnowledgeProvider。
//
// 这取代了"Agent 通过工具名查找工具并逐一注入依赖"的反模式
// （见原 internal/agent/tft_tools_wiring.go）。插件持有自己工具的
// 引用，调用 AttachDependencies 即可一次完成所有工具的依赖注入。
//
// 两个依赖是可选的：state=nil 表示 manager 不可用；kb=nil 表示 KB 不可用。
// 插件内部应自行处理 nil（与原 GameStateProvider/KnowledgeProvider 接口行为一致）。
type SetupReceiver interface {
	AttachDependencies(deps SetupDeps) error
}

// SetupDeps 插件运行时依赖（state / KB / 未来的可扩展项）
type SetupDeps struct {
	// GameState 当前 GameContext 快照读取器（来自 game.Manager.Snapshot）
	GameState GameStateProvider
	// Knowledge 知识库 provider（来自 internal/game/knowledge.Service）
	// 类型为 any：Agent Core 不关心具体接口；插件内部自行 type-assert
	// 复用其 N 个 Get* 方法。*knowledge.Service 是常见实现。
	Knowledge any
}

// GameStateProvider 通用 GameContext 快照读取接口
type GameStateProvider interface {
	Snapshot() *GameContext
}

// KnowledgeProvider 通用知识库读取接口（插件自行 type-assert 到具体 repo）
type KnowledgeProvider interface {
	// 仅声明最常用的方法；具体实现可能还有更多方法，插件自行断言
	GetCurrentPatch(ctx context.Context) (string, error)
}
