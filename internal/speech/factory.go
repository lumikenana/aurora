package speech

import (
	"github.com/lipipi/game-ai/internal/errors"
	"github.com/lipipi/game-ai/internal/log"
)

// ProviderFactory Provider 工厂函数类型
type ProviderFactory func(cfg any) (SpeechRecognizer, errors.AppError)

var (
	providerFactories = make(map[string]ProviderFactory)
)

// RegisterProvider 注册 Provider 工厂
func RegisterProvider(providerType string, factory ProviderFactory) {
	providerFactories[providerType] = factory
}

// CreateProvider 根据配置创建 SpeechRecognizer Provider
func CreateProvider(cfg Config) (SpeechRecognizer, errors.AppError) {
	providerType := cfg.Provider
	if providerType == "" {
		providerType = "volcengine"
	}

	factory, exists := providerFactories[providerType]
	if !exists {
		return nil, errors.New(errors.CodeInvalidConfig, "unsupported speech provider: "+providerType)
	}

	// 传递 provider 特定的配置
	var providerCfg any
	switch providerType {
	case "volcengine":
		providerCfg = cfg.Volcengine
	default:
		providerCfg = cfg
	}

	p, err := factory(providerCfg)
	if err != nil {
		return nil, err
	}

	log.Debugf("Created %s speech provider", p.Name())
	return p, nil
}
