//go:build windows

// Package game Win32 窗口枚举助手（供各游戏插件复用）
package game

import (
	"strings"
	"syscall"
	"unsafe"
)

var (
	gUser32              = syscall.NewLazyDLL("user32.dll")
	gProcEnumWindows     = gUser32.NewProc("EnumWindows")
	gProcIsWindowVisible = gUser32.NewProc("IsWindowVisible")
	gProcGetClassNameW   = gUser32.NewProc("GetClassNameW")
	gProcGetWindowTextW  = gUser32.NewProc("GetWindowTextW")
)

// FindWindowByClass 按窗口类名查找可见顶层窗口（不区分大小写，返回第一个命中）
func FindWindowByClass(classes ...string) (WindowInfo, bool) {
	return enumWindows(func(w WindowInfo) bool {
		for _, c := range classes {
			if strings.EqualFold(w.Class, c) {
				return true
			}
		}
		return false
	})
}

// FindWindowByClassAndTitle 类名命中且标题包含 titleContains（均不区分大小写）
func FindWindowByClassAndTitle(titleContains string, classes ...string) (WindowInfo, bool) {
	return enumWindows(func(w WindowInfo) bool {
		for _, c := range classes {
			if strings.EqualFold(w.Class, c) &&
				strings.Contains(strings.ToLower(w.Title), strings.ToLower(titleContains)) {
				return true
			}
		}
		return false
	})
}

// enumContext 一次窗口枚举的上下文。
// 固定 callback 通过 lParam 拿到本指针；每次 enumWindows 调用都持有独立实例，
// 并发枚举之间互不串扰（无全局可变状态，callback 因此可安全复用）。
type enumContext struct {
	match func(w WindowInfo) bool
	found WindowInfo
}

// windowsEnumCallback 进程内只注册一次的 EnumWindows 回调。
// syscall.NewCallback 每次调用都会占用一个 runtime callback 槽位（上限 2000），
// 此前在 enumWindows 内注册闭包导致周期性游戏检测逐渐耗尽槽位，
// 触发 "fatal error: too many callback functions"。
var windowsEnumCallback = syscall.NewCallback(func(hwnd, lParam uintptr) uintptr {
	ectx := (*enumContext)(unsafe.Pointer(lParam))

	// 只看可见窗口（Riot 有大量不可见的后台窗口）
	visible, _, _ := gProcIsWindowVisible.Call(hwnd)
	if visible == 0 {
		return 1 // 继续枚举
	}

	w := WindowInfo{HWND: hwnd, Class: windowClassName(hwnd), Title: windowText(hwnd)}
	if ectx.match(w) {
		ectx.found = w
		return 0 // 停止枚举
	}
	return 1
})

// enumWindows 枚举可见顶层窗口，match 返回 true 时停止并返回该窗口。
// 任何 Win32 调用失败都视为未找到，不 panic
func enumWindows(match func(w WindowInfo) bool) (WindowInfo, bool) {
	if err := gProcEnumWindows.Find(); err != nil {
		return WindowInfo{}, false
	}

	// 本次枚举的独立上下文经 lParam 传入固定 callback
	ectx := &enumContext{match: match}
	ret, _, _ := gProcEnumWindows.Call(windowsEnumCallback, uintptr(unsafe.Pointer(ectx)))
	if ret == 0 && ectx.found.HWND != 0 {
		return ectx.found, true
	}
	return WindowInfo{}, false
}

// windowClassName 读取窗口类名
func windowClassName(hwnd uintptr) string {
	var buf [256]uint16
	n, _, _ := gProcGetClassNameW.Call(hwnd, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	if n == 0 {
		return ""
	}
	return syscall.UTF16ToString(buf[:n])
}

// windowText 读取窗口标题
func windowText(hwnd uintptr) string {
	var buf [256]uint16
	n, _, _ := gProcGetWindowTextW.Call(hwnd, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	if n == 0 {
		return ""
	}
	return syscall.UTF16ToString(buf[:n])
}
