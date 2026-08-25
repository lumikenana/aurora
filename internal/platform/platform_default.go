//go:build !windows
// +build !windows

package platform

import (
	"runtime"

	"github.com/lipipi/game-ai/internal/errors"
	"github.com/lipipi/game-ai/internal/log"
)

// defaultPlatform 非Windows平台默认实现
type defaultPlatform struct {
}

func createPlatform() Platform {
	return &defaultPlatform{}
}

func (p *defaultPlatform) Init() error {
	log.Infof("Running on non-Windows platform: %s", p.GetPlatformName())
	return nil
}

func (p *defaultPlatform) Shutdown() error {
	return nil
}

func (p *defaultPlatform) IsWindows() bool {
	return false
}

func (p *defaultPlatform) GetPlatformName() string {
	return runtime.GOOS
}

func (p *defaultPlatform) SetAutoStart(enable bool) error {
	log.Debugf("SetAutoStart called (not implemented on %s)", p.GetPlatformName())
	return errors.New(errors.CodePlatformError, "SetAutoStart not implemented on this platform")
}

func (p *defaultPlatform) IsAutoStart() (bool, error) {
	return false, nil
}

func (p *defaultPlatform) OpenURL(url string) error {
	// 可以用open/xdg-open处理，但这里简化处理
	log.Debugf("OpenURL called: %s", url)
	return nil
}

func (p *defaultPlatform) MinimizeToTray() error {
	log.Debug("MinimizeToTray called")
	return nil
}

func (p *defaultPlatform) RestoreFromTray() error {
	log.Debug("RestoreFromTray called")
	return nil
}
