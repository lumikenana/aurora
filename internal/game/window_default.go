//go:build !windows

// Package game 非 Windows 平台的窗口枚举空实现
package game

// FindWindowByClass 非 Windows 平台始终返回未找到
func FindWindowByClass(classes ...string) (WindowInfo, bool) {
	return WindowInfo{}, false
}

// FindWindowByClassAndTitle 非 Windows 平台始终返回未找到
func FindWindowByClassAndTitle(titleContains string, classes ...string) (WindowInfo, bool) {
	return WindowInfo{}, false
}
