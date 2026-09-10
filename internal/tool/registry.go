package tool

import (
	"github.com/lipipi/aurora/internal/errors"
	"github.com/lipipi/aurora/internal/log"
)

// ToolFactory Tool 工厂函数类型
type ToolFactory func(cfg Config) (Tool, errors.AppError)

var (
	toolFactories = make(map[string]ToolFactory)
)

// RegisterTool 注册工具工厂
func RegisterTool(toolType string, factory ToolFactory) {
	toolFactories[toolType] = factory
}

// GetToolFactory 查询已注册的工厂（用于测试 / 自省）
func GetToolFactory(toolType string) (ToolFactory, bool) {
	f, ok := toolFactories[toolType]
	return f, ok
}

// CreateRegistry 创建工具注册表，根据配置实例化所有启用的工具
//
// Phase 18-B 增强：支持游戏插件自注册工具。
//   - cfg.Enabled 为空：启用所有已注册工具（包括游戏插件通过 init() 注册的）
//   - cfg.Enabled 非空：只启用列表中的工具（向后兼容）
//
// 这允许新增游戏插件时，不必修改 central config，插件在 init() 中通过
// tool.RegisterTool 注册的工具会自动启用。
//
// 注意：本方法**不做** IsAvailable 检查。某些工具（如 capture_screenshot）
// 需要在创建后由调用方注入运行时依赖（如 ScreenshotProvider），注入完成后
// 再调用 FilterUnavailable 过滤掉仍不可用的工具。
// 这样避免了"先过滤后注入"导致工具被永久跳过的顺序 bug。
func CreateRegistry(cfg Config) (*ToolRegistry, errors.AppError) {
	registry := &ToolRegistry{
		tools: make(map[string]Tool),
	}

	// 确定要实例化的工具列表
	toolsToEnable := cfg.Enabled
	if len(toolsToEnable) == 0 {
		// 空配置 = 启用所有已注册工具（支持插件自注册）
		toolsToEnable = make([]string, 0, len(toolFactories))
		for name := range toolFactories {
			toolsToEnable = append(toolsToEnable, name)
		}
		log.Debugf("tool: no explicit config, enabling all %d registered tools", len(toolsToEnable))
	}

	for _, toolName := range toolsToEnable {
		factory, exists := toolFactories[toolName]
		if !exists {
			return nil, errors.New(errors.CodeInvalidConfig, "unsupported tool: "+toolName)
		}

		tool, err := factory(cfg)
		if err != nil {
			return nil, err
		}

		registry.tools[tool.Name()] = tool
		log.Debugf("Registered tool: %s", tool.Name())
	}

	return registry, nil
}

// FilterUnavailable 移除当前 IsAvailable() == false 的工具。
//
// 应在所有运行时依赖注入（如 SetScreenshotProvider）完成之后调用。
// 返回被移除的工具数量，便于日志记录。
func (r *ToolRegistry) FilterUnavailable() int {
	if r == nil {
		return 0
	}
	removed := 0
	for name, t := range r.tools {
		if !t.IsAvailable() {
			log.Warnf("Tool %s is not available, removing", name)
			delete(r.tools, name)
			removed++
		}
	}
	return removed
}

// ToolRegistry 工具注册表，维护所有已注册可用的工具
type ToolRegistry struct {
	tools map[string]Tool
}

// Add 直接把一个已构造的工具实例加入 registry（同名覆盖）。
// 用于核心生命周期工具（如 exit_conversation_mode）：它们是系统能力而非
// 可选配置，不应被 cfg.Tool.Enabled 的持久化列表过滤 —— 既有用户的列表
// 在工具新增前已落盘，不会自动包含新工具。
func (r *ToolRegistry) Add(t Tool) {
	if r == nil || t == nil {
		return
	}
	r.tools[t.Name()] = t
}

// GetTool 根据名称获取工具
func (r *ToolRegistry) GetTool(name string) (Tool, bool) {
	tool, exists := r.tools[name]
	return tool, exists
}

// ListTools 列出所有可用工具
func (r *ToolRegistry) ListTools() []Tool {
	tools := make([]Tool, 0, len(r.tools))
	for _, t := range r.tools {
		tools = append(tools, t)
	}
	return tools
}

// Count 返回可用工具数量
func (r *ToolRegistry) Count() int {
	return len(r.tools)
}
