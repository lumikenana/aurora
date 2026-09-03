//go:build !windows
// +build !windows

// Phase 8D：非 Windows 平台 stub 实现。v3 SystemTray 在 darwin/linux 也有
// 实现，但 Aurora 实际目标仅 Windows（macOS 仅作 dev 验证）。dev 阶段保留
// 旧 default 行为：Init/Shutdown/SetOn*Handler/SetRecordingIcon 全 no-op。
package tray

import "github.com/wailsapp/wails/v3/pkg/application"

// BuildMenuAndAttach 在非 Windows 平台是 no-op。main.go 调用此函数是安全的
// （darwin dev 构建可编译通过），但实际不会构造 menu — 与 v2 default 行为一致。
func BuildMenuAndAttach(sysTray *application.SystemTray) error {
	return nil
}
