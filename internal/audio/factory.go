package audio

import (
	"github.com/lipipi/game-ai/internal/errors"
	"github.com/lipipi/game-ai/internal/log"
)

// ProviderFactory Provider 工厂函数类型
type ProviderFactory func(cfg Config) (AudioCapture, errors.AppError)

var (
	providerFactories = make(map[string]ProviderFactory)
)

// RegisterProvider 注册 Provider 工厂
func RegisterProvider(providerType string, factory ProviderFactory) {
	providerFactories[providerType] = factory
}

// CreateProvider 根据配置创建 Audio Provider
func CreateProvider(cfg Config) (AudioCapture, errors.AppError) {
	// 默认使用 windows provider（但在非 Windows 上会自动回退）
	providerType := "windows"

	factory, exists := providerFactories[providerType]
	if !exists {
		// 尝试默认 provider
		factory, exists = providerFactories["default"]
		if !exists {
			return nil, errors.New(errors.CodeInvalidConfig, "unsupported audio provider: "+providerType)
		}
		providerType = "default"
	}

	p, err := factory(cfg)
	if err != nil {
		return nil, err
	}

	if !p.IsAvailable() {
		log.Warnf("Audio provider %s is not available", p.Name())
		return nil, errors.New(errors.CodeAudioFailed, "no available audio provider")
	}

	log.Debugf("Created %s audio provider", p.Name())
	return p, nil
}
