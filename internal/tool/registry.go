package tool

import (
	"github.com/lipipi/game-ai/internal/errors"
	"github.com/lipipi/game-ai/internal/log"
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

// CreateRegistry 创建工具注册表，根据配置实例化所有启用的工具
func CreateRegistry(cfg Config) (*ToolRegistry, errors.AppError) {
	registry := &ToolRegistry{
		tools: make(map[string]Tool),
	}

	for _, toolName := range cfg.Enabled {
		factory, exists := toolFactories[toolName]
		if !exists {
			return nil, errors.New(errors.CodeInvalidConfig, "unsupported tool: "+toolName)
		}

		tool, err := factory(cfg)
		if err != nil {
			return nil, err
		}

		if !tool.IsAvailable() {
			log.Warnf("Tool %s is not available, skipping", tool.Name())
			continue
		}

		registry.tools[tool.Name()] = tool
		log.Debugf("Registered tool: %s", tool.Name())
	}

	return registry, nil
}

// ToolRegistry 工具注册表，维护所有已注册可用的工具
type ToolRegistry struct {
	tools map[string]Tool
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
