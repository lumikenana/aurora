//go:build !windows

package openwakeword

import (
	"context"

	"github.com/lipipi/game-ai/internal/errors"
	"github.com/lipipi/game-ai/internal/wakeword"
)

// noopOpenWakeWordDetector no-op implementation for non-Windows platforms.
type noopOpenWakeWordDetector struct {
}

// New creates a no-op openWakeWordDetector for non-Windows.
func New(cfg any) (wakeword.WakeWordDetector, errors.AppError) {
	return &noopOpenWakeWordDetector{}, nil
}

func (d *noopOpenWakeWordDetector) Start(ctx context.Context, audioCh <-chan []byte) error {
	return nil
}

func (d *noopOpenWakeWordDetector) Stop() error {
	return nil
}

func (d *noopOpenWakeWordDetector) Events() <-chan wakeword.WakeEvent {
	// Return a closed channel that never sends
	ch := make(chan wakeword.WakeEvent)
	close(ch)
	return ch
}

func (d *noopOpenWakeWordDetector) IsConfigured() bool {
	return false
}

func (d *noopOpenWakeWordDetector) Name() string {
	return "openwakeword (unsupported platform)"
}

// initializeONNX no-op implementation for non-Windows.
func (d *noopOpenWakeWordDetector) initializeONNX(
	onnxRuntimePath, melspectrogramPath, embeddingModelPath, vadModelPath string,
) errors.AppError {
	return errors.New(errors.CodeWakeWordFailed, "openwakeword not supported on this platform")
}

func init() {
	// Register the provider
	wakeword.RegisterProvider("openwakeword", New)
}
