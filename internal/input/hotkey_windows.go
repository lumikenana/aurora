//go:build windows
// +build windows

package input

import (
	"fmt"
	"syscall"
	"unsafe"

	"github.com/lipipi/game-ai/internal/errors"
	"github.com/lipipi/game-ai/internal/log"
)

const (
	// WM_HOTKEY 热键消息ID
	wmHotkey = 0x0312
	// CW_USEDEFAULT 默认窗口位置
	cwUseDefault = 0x80000000
	// HWND_MESSAGE 消息窗口句柄
	hWNDMessage = uintptr(0xffff)
)

// windows API lazy loading
var (
	user32               = syscall.NewLazyDLL("user32.dll")
	procRegisterClassEx  = user32.NewProc("RegisterClassExW")
	procCreateWindowEx   = user32.NewProc("CreateWindowExW")
	procDefWindowProc   = user32.NewProc("DefWindowProcW")
	procDestroyWindow   = user32.NewProc("DestroyWindow")
	procRegisterHotKey   = user32.NewProc("RegisterHotKey")
	procUnregisterHotKey = user32.NewProc("UnregisterHotKey")
	procGetMessage      = user32.NewProc("GetMessageW")
	procTranslateMessage = user32.NewProc("TranslateMessage")
	procDispatchMessage = user32.NewProc("DispatchMessageW")
)

// WNDCLASSEX window class structure
type wndClassEx struct {
	cbSize        uint32
	style         uint32
	lpfnWndProc   uintptr
	cbClsExtra    int32
	cbWndExtra    int32
	hInstance     syscall.Handle
	hIcon         uintptr
	hCursor       uintptr
	hbrBackground uintptr
	lpszMenuName  uintptr
	lpszClassName uintptr
	hIconSm       uintptr
}

// MSG message structure
type msg struct {
	Hwnd    uintptr
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Pt      struct {
		X int32
		Y int32
	}
}

// windowsHotkeyListener Windows平台热键监听器实现
type windowsHotkeyListener struct {
	hotkeys        map[HotkeyID]Hotkey
	registeredIDs  map[HotkeyID]int // 保存注册后的ID
	hwnd           uintptr
	running        bool
	done           chan struct{}
}

func createHotkeyListener() HotkeyListener {
	return &windowsHotkeyListener{
		hotkeys:        make(map[HotkeyID]Hotkey),
		registeredIDs:  make(map[HotkeyID]int),
		done:           make(chan struct{}),
	}
}

func (h *windowsHotkeyListener) Init() errors.AppError {
	log.Debug("Initializing Windows hotkey listener...")

	// 创建一个隐藏窗口用于接收热键消息
	className, err := syscall.UTF16PtrFromString("GameAIHotkeyWindow")
	if err != nil {
		return errors.Wrap(errors.CodeInputError, "Failed to encode class name", err)
	}

	windowName, err := syscall.UTF16PtrFromString("GameAI Hotkey Listener")
	if err != nil {
		return errors.Wrap(errors.CodeInputError, "Failed to encode window name", err)
	}

	// 注册窗口类
	wc := wndClassEx{
		cbSize:        uint32(unsafe.Sizeof(wndClassEx{})),
		lpfnWndProc:   syscall.NewCallback(h.wndProc),
		lpszClassName: uintptr(unsafe.Pointer(className)),
	}

	ret, _, err := procRegisterClassEx.Call(uintptr(unsafe.Pointer(&wc)))
	if ret == 0 {
		return errors.Wrap(errors.CodeInputError, "Failed to register window class", err)
	}

	// 创建隐藏窗口
	hwnd, _, err := procCreateWindowEx.Call(
		0,
		uintptr(unsafe.Pointer(className)),
		uintptr(unsafe.Pointer(windowName)),
		0,
		uintptr(cwUseDefault),
		uintptr(cwUseDefault),
		uintptr(cwUseDefault),
		uintptr(cwUseDefault),
		hWNDMessage,
		0,
		0,
		0,
	)
	if hwnd == 0 {
		return errors.Wrap(errors.CodeInputError, "Failed to create message window", err)
	}

	h.hwnd = hwnd
	h.running = true

	// 启动消息循环
	go h.StartListening()

	log.Debugf("Hotkey listener window created: HWND=%d", hwnd)
	return nil
}

func (h *windowsHotkeyListener) Shutdown() {
	if !h.running {
		return
	}

	log.Debug("Shutting down Windows hotkey listener...")
	h.UnregisterAll()

	if h.hwnd != 0 {
		procDestroyWindow.Call(h.hwnd)
	}

	h.running = false
	close(h.done)
	log.Debug("Windows hotkey listener shutdown complete")
}

func (h *windowsHotkeyListener) RegisterHotkey(hotkey Hotkey) errors.AppError {
	if h.IsRegistered(hotkey.ID) {
		// 已经注册过，先注销
		h.UnregisterHotkey(hotkey.ID)
	}

	// 使用ID从1开始递增
	localID := int(hotkey.ID)
	// RegisterHotKey 原型:
	// BOOL RegisterHotKey(
	//   [in] HWND hWnd,
	//   [in] int id,
	//   [in] UINT fsModifiers,
	//   [in] UINT vk
	// );
	// 参数顺序正确
	log.Debugf("Calling RegisterHotKey: hwnd=%p, id=%d, mod=%d, vk=%d", h.hwnd, localID, hotkey.Modifier, hotkey.Key)
	ret, _, err := procRegisterHotKey.Call(h.hwnd, uintptr(localID), uintptr(hotkey.Modifier), uintptr(hotkey.Key))
	if ret == 0 {
		log.Errorf("❌ Failed to register hotkey: mod=%d, key=%d (already taken by another program?)", hotkey.Modifier, hotkey.Key)
		return errors.Wrap(errors.CodeInputError, fmt.Sprintf("Failed to register hotkey %d", hotkey.ID), err)
	}

	h.hotkeys[hotkey.ID] = hotkey
	h.registeredIDs[hotkey.ID] = localID

	log.Debugf("✅ Hotkey registered: ID=%d, mod=%d, key=%d", hotkey.ID, hotkey.Modifier, hotkey.Key)
	return nil
}

func (h *windowsHotkeyListener) UnregisterHotkey(id HotkeyID) errors.AppError {
	if !h.IsRegistered(id) {
		return nil
	}

	localID := h.registeredIDs[id]
	ret, _, err := procUnregisterHotKey.Call(h.hwnd, uintptr(localID))
	if ret == 0 {
		return errors.Wrap(errors.CodeInputError, fmt.Sprintf("Failed to unregister hotkey %d", id), err)
	}

	delete(h.hotkeys, id)
	delete(h.registeredIDs, id)
	log.Debugf("Hotkey unregistered: ID=%d", id)
	return nil
}

func (h *windowsHotkeyListener) UnregisterAll() {
	log.Debug("Unregistering all hotkeys...")
	for id := range h.hotkeys {
		_ = h.UnregisterHotkey(id)
	}
	h.hotkeys = make(map[HotkeyID]Hotkey)
	h.registeredIDs = make(map[HotkeyID]int)
}

func (h *windowsHotkeyListener) StartListening() {
	var msg msg
	for h.running {
		ret, _, _ := procGetMessage.Call(uintptr(unsafe.Pointer(&msg)), h.hwnd, 0, 0)
		if !h.running {
			break
		}
		if int32(ret) == -1 {
			log.Errorf("GetMessage error")
			break
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&msg)))
		procDispatchMessage.Call(uintptr(unsafe.Pointer(&msg)))
	}
	h.done <- struct{}{}
}

func (h *windowsHotkeyListener) IsRegistered(id HotkeyID) bool {
	_, exists := h.registeredIDs[id]
	return exists
}

// wndProc 窗口过程处理热键消息
func (h *windowsHotkeyListener) wndProc(hwnd uintptr, msg uint32, wParam, lParam uintptr) uintptr {
	switch msg {
	case wmHotkey:
		// wParam 是热键ID
		id := HotkeyID(wParam)
		log.Debugf("Hotkey triggered: ID=%d", id)
		if hotkey, ok := h.hotkeys[id]; ok && hotkey.Callback != nil {
			hotkey.Callback()
		}
		return 0
	default:
		ret, _, _ := procDefWindowProc.Call(hwnd, uintptr(msg), wParam, lParam)
		return ret
	}
}
