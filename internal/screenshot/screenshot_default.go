//go:build !windows
// +build !windows

package screenshot

import (
	"context"

	"github.com/lipipi/aurora/internal/errors"
)

type defaultScreenshotProvider struct {
	cfg Config
}

// NewDefaultScreenshotProvider 创建默认（非Windows）截图 Provider
// 在非Windows平台上，这个 Provider 总是不可用，只是占位让代码编译通过
func NewDefaultScreenshotProvider(cfg Config) (ScreenshotProvider, errors.AppError) {
	return &defaultScreenshotProvider{
		cfg: cfg,
	}, nil
}

func (p *defaultScreenshotProvider) Name() string {
	return "default-null"
}

func (p *defaultScreenshotProvider) IsAvailable() bool {
	// 非 Windows 平台不支持原生截图
	return false
}

func (p *defaultScreenshotProvider) GetDisplayCount() int {
	return 0
}

func (p *defaultScreenshotProvider) Capture(ctx context.Context, request ScreenshotRequest) (*Screenshot, errors.AppError) {
	return nil, errors.New(errors.CodeScreenshotFailed, "screenshot is not supported on this platform")
}

func init() {
	RegisterProvider("default", NewDefaultScreenshotProvider)
}
