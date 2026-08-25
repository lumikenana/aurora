package screenshot

import (
	"github.com/lipipi/game-ai/internal/errors"
	"github.com/lipipi/game-ai/internal/log"
)

// ProviderFactory Provider 工厂函数类型
type ProviderFactory func(cfg Config) (ScreenshotProvider, errors.AppError)

var (
	providerFactories = make(map[string]ProviderFactory)
)

// RegisterProvider 注册 Provider 工厂
func RegisterProvider(providerType string, factory ProviderFactory) {
	providerFactories[providerType] = factory
}

// CreateProvider 根据配置创建 Screenshot Provider
func CreateProvider(cfg Config) (ScreenshotProvider, errors.AppError) {
	// 默认使用 windows provider（但在非 Windows 上会自动回退）
	providerType := "windows"

	factory, exists := providerFactories[providerType]
	if !exists {
		// 尝试默认 provider
		factory, exists = providerFactories["default"]
		if !exists {
			return nil, errors.New(errors.CodeInvalidConfig, "unsupported screenshot provider: "+providerType)
		}
		providerType = "default"
	}

	p, err := factory(cfg)
	if err != nil {
		return nil, err
	}

	if !p.IsAvailable() {
		log.Warnf("Screenshot provider %s is not available", p.Name())
		// 如果默认 provider 不可用，返回错误
		return nil, errors.New(errors.CodeScreenshotFailed, "no available screenshot provider")
	}

	log.Debugf("Created %s screenshot provider", p.Name())
	return p, nil
}
