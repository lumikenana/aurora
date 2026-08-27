//go:build !windows
// +build !windows

package audio

import (
	"github.com/lipipi/aurora/internal/errors"
	"github.com/lipipi/aurora/internal/log"
)

type defaultAudioCapture struct {
	cfg Config
}

func init() {
	RegisterProvider("default", NewDefaultAudioCapture)
}

// NewDefaultAudioCapture 创建默认（空）音频采集 Provider
func NewDefaultAudioCapture(cfg Config) (AudioCapture, errors.AppError) {
	return &defaultAudioCapture{cfg: cfg}, nil
}

func (d *defaultAudioCapture) Start(callback AudioCallback) error {
	return errors.New(errors.CodeAudioFailed, "audio capture not available on this platform")
}

func (d *defaultAudioCapture) Stop() {}

func (d *defaultAudioCapture) IsRecording() bool {
	return false
}

func (d *defaultAudioCapture) Name() string {
	return "default"
}

func (d *defaultAudioCapture) IsAvailable() bool {
	log.Debug("Default audio provider is not available (non-Windows platform)")
	return false
}
