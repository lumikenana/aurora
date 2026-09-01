//go:build !windows

// Package tft 云顶之弈插件 - 非 Windows 平台空实现
package tft

import "github.com/lipipi/aurora/internal/game"

// detectTFTWindow 非 Windows 平台始终返回未检测到
func detectTFTWindow() (game.WindowInfo, bool) {
	return game.WindowInfo{}, false
}
