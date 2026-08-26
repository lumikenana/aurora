package wakeword

import (
	"github.com/lipipi/game-ai/internal/errors"
)

// ProviderFactory is the function type for creating a WakeWordDetector provider.
type ProviderFactory func(cfg any) (WakeWordDetector, errors.AppError)

var providerFactories = make(map[string]ProviderFactory)

// RegisterProvider registers a provider factory. Called from provider's init().
func RegisterProvider(providerType string, factory ProviderFactory) {
	providerFactories[providerType] = factory
}

// CreateProvider creates a WakeWordDetector from the given config.
func CreateProvider(cfg Config) (WakeWordDetector, errors.AppError) {
	factory, ok := providerFactories[cfg.Provider]
	if !ok {
		return nil, errors.New(errors.CodeWakeWordFailed, "unsupported wakeword provider: "+cfg.Provider)
	}

	// Dispatch provider-specific config
	// Each provider needs to handle type assertion
	return factory(cfg)
}

// GetProviderFactories returns the map of registered provider factories.
// For testing/debugging only.
func GetProviderFactories() map[string]ProviderFactory {
	return providerFactories
}
