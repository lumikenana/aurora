// Package screenshot 定义屏幕截图 Provider 接口
package screenshot

import (
	"context"
	"image"

	"github.com/lipipi/game-ai/internal/errors"
)

// CaptureMode 截图模式
type CaptureMode string

const (
	// CaptureModeFullScreen 全屏截图
	CaptureModeFullScreen CaptureMode = "fullscreen"
	// CaptureModePrimaryDisplay 主显示器截图
	CaptureModePrimaryDisplay CaptureMode = "primary"
	// CaptureModeSpecifiedDisplay 指定显示器截图
	CaptureModeSpecifiedDisplay CaptureMode = "specified"
	// CaptureModeActiveWindow 当前活动窗口截图
	CaptureModeActiveWindow CaptureMode = "active_window"
	// CaptureModeSpecifiedWindow 指定窗口截图
	CaptureModeSpecifiedWindow CaptureMode = "specified_window"
)

// ScreenshotRequest 截图请求
type ScreenshotRequest struct {
	Mode         CaptureMode  `json:"mode"`
	DisplayIndex int         `json:"display_index,omitempty"` // 指定显示器索引，从 0 开始
	WindowHWND   uintptr     `json:"window_hwnd,omitempty"`   // 指定窗口句柄
	MaxWidth     int         `json:"max_width,omitempty"`     // 最大宽度，0 表示不限制
	MaxHeight    int         `json:"max_height,omitempty"`    // 最大高度，0 表示不限制
	Quality      int         `json:"quality,omitempty"`       // JPEG 质量 (1-100), 0 表示不压缩
}

// Screenshot 截图结果
type Screenshot struct {
	OriginalImage image.Image `json:"-"`         // 原始 image 对象，内存中
	Width         int         `json:"width"`     // 实际宽度
	Height        int         `json:"height"`    // 实际高度
	Bytes         []byte      `json:"bytes"`     // 压缩后的字节数据（JPEG 格式）
	Format        string      `json:"format"`    // "jpeg", "png"
	Mode          CaptureMode `json:"mode"`      // 使用的截图模式
}

// ScreenshotProvider 屏幕截图 Provider 接口
// Agent Core 依赖此接口，不依赖具体实现
type ScreenshotProvider interface {
	// Capture 执行截图
	Capture(ctx context.Context, request ScreenshotRequest) (*Screenshot, errors.AppError)
	// GetDisplayCount 获取显示器数量
	GetDisplayCount() int
	// Name 返回 Provider 名称
	Name() string
	// IsAvailable 检查 Provider 是否可用
	IsAvailable() bool
}

// DefaultConfig 返回默认截图配置
func DefaultConfig() Config {
	return Config{
		DefaultQuality:   80,
		DefaultMaxWidth:  1920,
		DefaultMaxHeight: 1080,
	}
}

// Config 截图模块配置
type Config struct {
	DefaultQuality   int `json:"default_quality"`
	DefaultMaxWidth  int `json:"default_max_width"`
	DefaultMaxHeight int `json:"default_max_height"`
}
