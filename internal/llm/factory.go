package llm

import (
	"github.com/lipipi/game-ai/internal/errors"
	"github.com/lipipi/game-ai/internal/log"
)

// ProviderFactory  Provider 工厂函数类型
type ProviderFactory func(cfg any) (LLMProvider, errors.AppError)

var (
	providerFactories = make(map[string]ProviderFactory)
)

// RegisterProvider 注册 Provider 工厂
func RegisterProvider(providerType string, factory ProviderFactory) {
	providerFactories[providerType] = factory
}

// Config LLM 模块整体配置
type Config struct {
	Provider string         `json:"provider"`
	OpenAI   ProviderConfig `json:"openai"`
}

// DefaultConfig 返回默认 LLM 配置
func DefaultConfig() Config {
	return Config{
		Provider: "openai",
		OpenAI: ProviderConfig{
			TimeoutSec: 60,
			Model:       "gpt-4o-mini",
		},
	}
}

// CreateProvider 根据配置创建 LLM Provider
func CreateProvider(cfg Config) (LLMProvider, errors.AppError) {
	factory, exists := providerFactories[cfg.Provider]
	if !exists {
		return nil, errors.New(errors.CodeInvalidConfig, "unsupported LLM provider: "+cfg.Provider)
	}

	var provider any
	switch cfg.Provider {
	case "openai":
		provider = cfg.OpenAI
	default:
		provider = cfg
	}

	p, err := factory(provider)
	if err != nil {
		return nil, err
	}

	if !p.IsConfigured() {
		return nil, errors.New(errors.CodeInvalidConfig, p.Name()+" provider is not properly configured")
	}

	log.Debugf("Created %s LLM provider, model=%s", p.Name(), cfg.OpenAI.Model)
	return p, nil
}
