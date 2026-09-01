// Package tft 云顶之弈（Teamfight Tactics）游戏插件
//
// Phase 1 仅做窗口检测（LOL 客户端候选）+ 附加系统提示词，
// 不读取游戏内部数据；截图分析由 Agent Core + 视觉 LLM 完成
//
// Phase 17-A 升级：Plugin 现在实现 3 个可选接口：
//   - game.GameContextRenderer（把 *TFTGameContext 渲染为 LLM prompt 摘要）
//   - game.ToolContributor（把 KB / state 注入到 tft 自己的工具）
//   - game.SetupReceiver（接收 Agent 注入的 GameState / Knowledge 依赖）
//
// Agent Core 不知道这 3 个接口存在；调用方做 type assertion。
package tft

import (
	"context"
	"time"

	"github.com/lipipi/aurora/internal/errors"
	"github.com/lipipi/aurora/internal/game"
	"github.com/lipipi/aurora/internal/game/plugins/tft/composition"
	"github.com/lipipi/aurora/internal/game/plugins/tft/gamestate"
	"github.com/lipipi/aurora/internal/game/plugins/tft/grounder"
	"github.com/lipipi/aurora/internal/game/plugins/tft/tools"
	"github.com/lipipi/aurora/internal/log"
	"github.com/lipipi/aurora/internal/llm"
	"github.com/lipipi/aurora/internal/tool"
)

// Plugin 云顶之弈插件
type Plugin struct {
	ocrProvider *TFTOCRProvider

	// Phase 17-A：Plugin 持有的运行时依赖（由 SetupReceiver.AttachDependencies 注入）
	// stateProvider 类型为 game.GameStateProvider（单方法 Snapshot），与
	// tools.GameStateProvider 方法集相同；通过 interface 转换在 RegisterTools
	// 内部传给工具。
	stateProvider game.GameStateProvider
	kbProvider    KnowledgeProvider

	// Phase 17-A：Plugin 持有的 prompt renderer（AttachDependencies 后构造）
	renderer *PromptRenderer
}

// New 创建 TFT 插件（工厂，供注册）
//
// llmProvider: 可为 nil（未配置 LLM 时 OCR 退化到 Unavailable / Mock）
func New(cfg game.Config) (game.GamePlugin, errors.AppError) {
	return NewWithLLM(cfg, nil)
}

// NewWithLLM 创建 TFT 插件并注入 LLM Provider（供 VisionOCREngine 使用）
//
// 主要路径：Agent.Init 阶段调用此函数把已配 LLM 注入；
// 直接调用 New 时 LLM 为 nil，OCR 自动降级。
func NewWithLLM(cfg game.Config, llmProvider llm.LLMProvider) (game.GamePlugin, errors.AppError) {
	p := &Plugin{}

	engine := createOCREngine(cfg.OCR, llmProvider)
	if engine != nil {
		p.ocrProvider = NewTFTOCRProvider(engine)
	}

	return p, nil
}

// createOCREngine 根据配置创建 OCR 引擎
//
// Phase 14-D 策略：
//   - engine="auto" → 优先 Vision LLM → 否则 Gosseract → 否则 Unavailable
//   - engine="vision" → 仅 Vision LLM（LLM 未配时降级 Unavailable）
//   - engine="tesseract" → 仅 Gosseract（需 libtesseract 运行时）
//   - engine="mock" → 仅 Mock
//
// 旧"auto"默认走 Gosseract 的行为已被替换为 Vision LLM 优先，
// 因为 Tesseract CGO 在 Windows 单 EXE 部署中需要额外运行时
// （libtesseract + 语言包），违反"零额外依赖"约束。
func createOCREngine(cfg game.OCRConfig, llmProvider llm.LLMProvider) OCREngine {
	lang := cfg.Language
	if lang == "" {
		lang = "chi_sim+eng"
	}

	switch cfg.Engine {
	case "mock":
		return NewMockOCREngine()

	case "vision":
		// 强制使用 Vision LLM
		if llmProvider == nil || !llmProvider.IsConfigured() {
			log.Warnf("TFT OCR: engine=vision but LLM not configured → unavailable")
			return NewUnavailableOCREngine("vision LLM not configured")
		}
		engine := NewVisionOCREngine(newLLMAdapter(llmProvider), 256, 0, "zh")
		log.Infof("TFT OCR: using Vision LLM (provider=%s)", llmProvider.Name())
		return engine

	case "tesseract":
		return createGosseractEngine(lang)

	case "auto", "":
		// Phase 14-D：优先 Vision LLM（已配 LLM）
		if llmProvider != nil && llmProvider.IsConfigured() {
			engine := NewVisionOCREngine(newLLMAdapter(llmProvider), 256, 0, "zh")
			log.Infof("TFT OCR: auto-selected Vision LLM (provider=%s)", llmProvider.Name())
			return engine
		}
		// Fallback: Tesseract（如有运行时）
		log.Info("TFT OCR: LLM not available, trying Tesseract fallback")
		return createGosseractEngine(lang)

	default:
		log.Warnf("TFT OCR: unknown engine '%s' → engine unavailable", cfg.Engine)
		return NewUnavailableOCREngine("unknown engine: " + cfg.Engine)
	}
}

// createGosseractEngine 包装 Gosseract（带 panic recover）
//
// 编译时若 CGO 链接失败会 panic；运行时若 libtesseract 缺失会直接返回 error。
// 任一情况都安全降级为 UnavailableOCREngine。
func createGosseractEngine(lang string) OCREngine {
	defer func() {
		if r := recover(); r != nil {
			log.Warnf("TFT OCR: gosseract link failed: %v → engine unavailable", r)
		}
	}()
	engine := NewGosseractOCREngine(lang)
	log.Info("TFT OCR: using Gosseract/Tesseract")
	return engine
}

// 自动注册（ADR-012 注册惯用法；需 main.go blank import 触发）
func init() {
	game.RegisterPlugin("tft", New)
}

// ID 插件唯一标识
func (p *Plugin) ID() string { return "tft" }

// Name 插件显示名
func (p *Plugin) Name() string { return "云顶之弈" }

// Detect 检测 LOL/TFT 窗口（TFT 候选；由视觉 LLM 二次确认）
//
// Phase 10-6 改造：
//   - 窗口信息收敛到 Window 字段
//   - 新增 Phase=PhaseUnknown（窗口检测本身不识别阶段，Phase 由未来 Provider 推断）
//   - GameData 槽位挂载 *TFTGameContext（所有字段保持 Unknown）
//   - 不读取游戏内部数据（ADR-021）
func (p *Plugin) Detect(ctx context.Context) (*game.GameContext, error) {
	win, ok := detectTFTWindow()
	if !ok {
		return nil, nil
	}
	return &game.GameContext{
		Game:       p.ID(),
		Name:       p.Name(),
		Phase:      game.PhaseUnknown, // 窗口检测无法识别阶段，由未来 ContextProvider 填
		DetectedAt: time.Now(),
		Window:     win,
		GameData:   &TFTGameContext{}, // 所有字段保持 Unknown，等待未来 Provider
	}, nil
}

// SystemPrompt TFT 附加系统提示词
func (p *Plugin) SystemPrompt() string { return systemPrompt }

// GetOCROvider 返回 plugin 持有的 OCR ContextProvider（nil 表示不可用）
// 由 Agent 在初始化后调用注册到 game.Manager
func (p *Plugin) GetOCROvider() game.ContextProvider {
	return p.ocrProvider
}

// SetLLMProvider 注入 LLM Provider（Phase 14-D）
//
// 解决 chicken-and-egg：tft.Plugin 通过工厂 New 创建时 LLM 可能还没就绪。
// agent.Init 在创建 LLM provider 之后调用本方法，触发 OCR 引擎升级：
//   - 当前为 Unavailable + LLM 已配 → 升级为 VisionOCREngine
//   - 当前为 Mock → 保持 Mock（测试场景）
//   - 当前已是 VisionOCREngine → 仅更新 LLM 引用（hot-reload 场景）
//
// 本方法幂等，重复调用安全。
func (p *Plugin) SetLLMProvider(provider llm.LLMProvider) {
	if provider == nil {
		return
	}
	// 若已有 Vision OCR engine，只更新 LLM 引用
	if existing, ok := p.ocrProvider.engine.(*VisionOCREngine); ok {
		existing.caller = newLLMAdapter(provider)
		log.Debugf("TFT: updated Vision OCR engine LLM reference to %s", provider.Name())
		return
	}
	// 已有 mock → 保持（避免测试时被覆盖）
	if _, ok := p.ocrProvider.engine.(*MockOCREngine); ok {
		log.Debug("TFT: keeping Mock OCR engine (test mode)")
		return
	}
	// Unavailable 或其他 → 升级为 Vision OCR
	if provider.IsConfigured() {
		engine := NewVisionOCREngine(newLLMAdapter(provider), 256, 0, "zh")
		p.ocrProvider.engine = engine
		log.Infof("TFT: upgraded OCR engine to Vision LLM (provider=%s)", provider.Name())
	}
}

// ----------------------------------------------------------------------------
// Phase 17-A：实现 game.GameContextRenderer / game.ToolContributor / game.SetupReceiver
// ----------------------------------------------------------------------------

// AttachDependencies 接收 Agent 注入的 GameState / Knowledge 依赖
//
// 时序：Agent.Init 阶段，先调本方法，再调 RegisterTools。
// 本方法：
//   1. 保存 GameState / KB 引用
//   2. 构造 PromptRenderer（含 grounder / composition / gamestate 三个 analyzer）
//
// 注意：本方法不读 GameContext 状态（不触发 Detect），只是把 provider
// 存起来；PromptRenderer 内部 Render 时再读。
func (p *Plugin) AttachDependencies(deps game.SetupDeps) error {
	p.stateProvider = deps.GameState
	// deps.Knowledge 是 game.KnowledgeProvider（只声明 GetCurrentPatch），
	// 但实际对象 *knowledge.Service 实现完整 5 方法。延迟断言。
	if kp, ok := deps.Knowledge.(KnowledgeProvider); ok {
		p.kbProvider = kp
	} else if deps.Knowledge != nil {
		log.Warnf("TFT: SetupDeps.Knowledge does not implement 5-method KnowledgeProvider (got %T); KB-based prompt injection will be skipped", deps.Knowledge)
	}

	// 构造 PromptRenderer（grounder / composition / gamestate 都是无状态 analyzer）
	grnd := grounder.New(p.kbProvider)
	comp := composition.NewAnalyzer()
	gsa := gamestate.NewAnalyzer()
	p.renderer = NewPromptRenderer(p.kbProvider, grnd, comp, gsa)

	log.Debug("TFT: Plugin.AttachDependencies completed; renderer + analyzers ready")
	return nil
}

// RegisterTools 把 GameStateProvider / KnowledgeProvider 注入到 tft 自己的工具
//
// 调用方（Agent）通过 type assertion 触发本方法（实现 game.ToolContributor）。
// 工具已由 tft/tools 包 init() 通过 tool.RegisterTool 注册；本方法从
// registry 取出实例并注入依赖。
//
// 不感知其它插件的工具名（仅注入 tft 自己注册的 5 个）。
func (p *Plugin) RegisterTools(reg *tool.ToolRegistry) error {
	if reg == nil {
		return nil
	}

	// game.GameStateProvider → tools.GameStateProvider interface 转换
	// （方法集相同：仅 Snapshot() *GameContext）
	var toolsState tools.GameStateProvider
	if p.stateProvider != nil {
		toolsState = tools.GameStateProvider(p.stateProvider)
	}
	// tft.KnowledgeProvider → tools.KnowledgeProvider（方法集相同：5 个 Get*）
	var toolsKB tools.KnowledgeProvider
	if p.kbProvider != nil {
		toolsKB = tools.KnowledgeProvider(p.kbProvider)
	}

	// 1) get_tft_game_state：仅需 state
	if t, ok := reg.GetTool("get_tft_game_state"); ok {
		if gst, ok := t.(*tools.TFTGameStateTool); ok && toolsState != nil {
			gst.SetGameStateProvider(toolsState)
			log.Debug("TFT: wired get_tft_game_state")
		}
	}

	// 2) 4 个 knowledge 工具：需 state + kb
	for _, name := range []string{
		"get_tft_champion",
		"get_tft_item",
		"get_tft_trait",
		"get_tft_augment",
	} {
		t, exists := reg.GetTool(name)
		if !exists {
			continue
		}
		kt, ok := t.(*tools.TFTKnowledgeTool)
		if !ok {
			continue
		}
		if toolsState != nil {
			kt.SetGameStateProvider(toolsState)
		}
		if toolsKB != nil {
			kt.SetKnowledgeProvider(toolsKB)
		}
		log.Debugf("TFT: wired %s", name)
	}

	return nil
}

// RenderGameContext 把 GameContext 渲染为 LLM prompt 摘要
//
// 实现 game.GameContextRenderer 接口。Agent 通过 type assertion 调用。
// 直接委托给内部 PromptRenderer（构造与状态已在 AttachDependencies 完成）。
func (p *Plugin) RenderGameContext(gctx *game.GameContext, userInput string) (string, bool) {
	if p == nil || p.renderer == nil {
		return "", false
	}
	return p.renderer.Render(gctx, userInput)
}
