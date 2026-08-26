//go:build windows
// +build windows

package input

import (
	"fmt"
	"runtime"
	"sync"
	"syscall"
	"unsafe"

	"github.com/lipipi/game-ai/internal/errors"
	"github.com/lipipi/game-ai/internal/log"
)

const (
	// WM_HOTKEY 热键消息ID
	wmHotkey = 0x0312
	// WM_APP 自定义消息，用于唤醒窗口线程处理操作
	wmAppExecuteOp = 0x8000 + 1
	// CW_USEDEFAULT 默认窗口位置
	cwUseDefault = 0x80000000
	// HWND_MESSAGE 消息窗口父句柄，WinUser.h 中定义为 (HWND)-3
	hWNDMessage = ^uintptr(2)
)

// windows API lazy loading
var (
	kernel32            = syscall.NewLazyDLL("kernel32.dll")
	procGetModuleHandle = kernel32.NewProc("GetModuleHandleW")

	user32               = syscall.NewLazyDLL("user32.dll")
	procRegisterClassEx  = user32.NewProc("RegisterClassExW")
	procCreateWindowEx   = user32.NewProc("CreateWindowExW")
	procDefWindowProc    = user32.NewProc("DefWindowProcW")
	procDestroyWindow    = user32.NewProc("DestroyWindow")
	procRegisterHotKey   = user32.NewProc("RegisterHotKey")
	procUnregisterHotKey = user32.NewProc("UnregisterHotKey")
	procGetMessage       = user32.NewProc("GetMessageW")
	procTranslateMessage = user32.NewProc("TranslateMessage")
	procDispatchMessage  = user32.NewProc("DispatchMessageW")
	procPostMessage      = user32.NewProc("PostMessageW")
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
	mu            sync.Mutex // 保护 hotkeys/registeredIDs 的跨线程访问
	hotkeys       map[HotkeyID]Hotkey
	registeredIDs map[HotkeyID]int // 保存注册后的ID
	hwnd          uintptr
	running       bool
	done          chan struct{}
	ops           chan *windowOp // 待窗口线程执行的操作
}

// windowOp 在窗口线程上执行的操作请求
type windowOp struct {
	fn     func() errors.AppError
	result chan errors.AppError
}

func createHotkeyListener() HotkeyListener {
	return &windowsHotkeyListener{
		hotkeys:       make(map[HotkeyID]Hotkey),
		registeredIDs: make(map[HotkeyID]int),
		done:          make(chan struct{}),
		ops:           make(chan *windowOp, 16),
	}
}

func (h *windowsHotkeyListener) Init() errors.AppError {
	log.Debug("Initializing Windows hotkey listener...")

	// 窗口创建和消息循环必须在同一个 OS 线程上运行：
	// Windows 的窗口消息队列是按线程的，GetMessage 只能取到本线程窗口的消息。
	// Go 的 goroutine 会在 syscall 之间迁移线程，因此这里锁定一个专用线程。
	ready := make(chan struct{})
	errCh := make(chan errors.AppError, 1)

	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()

		hwnd, err := h.createMessageWindow()
		if err != nil {
			errCh <- err
			return
		}

		h.hwnd = hwnd
		h.running = true
		close(ready)

		h.messageLoop()
	}()

	select {
	case err := <-errCh:
		return err
	case <-ready:
		log.Debugf("Hotkey listener window created: HWND=%d", h.hwnd)
		return nil
	}
}

// createMessageWindow 创建接收热键消息的隐藏窗口（必须在锁定的线程中调用）
func (h *windowsHotkeyListener) createMessageWindow() (uintptr, errors.AppError) {
	className, err := syscall.UTF16PtrFromString("GameAIHotkeyWindow")
	if err != nil {
		return 0, errors.Wrap(errors.CodeInputError, "Failed to encode class name", err)
	}

	windowName, err := syscall.UTF16PtrFromString("GameAI Hotkey Listener")
	if err != nil {
		return 0, errors.Wrap(errors.CodeInputError, "Failed to encode window name", err)
	}

	// 获取当前模块句柄，注册窗口类和创建窗口时必须使用
	hInst, _, _ := procGetModuleHandle.Call(0)

	// 注册窗口类
	wc := wndClassEx{
		cbSize:        uint32(unsafe.Sizeof(wndClassEx{})),
		lpfnWndProc:   syscall.NewCallback(h.wndProc),
		hInstance:     syscall.Handle(hInst),
		lpszClassName: uintptr(unsafe.Pointer(className)),
	}

	ret, _, err := procRegisterClassEx.Call(uintptr(unsafe.Pointer(&wc)))
	if ret == 0 {
		// 类可能已注册过（重复初始化），直接继续
		log.Debug("RegisterClassEx returned 0, class may already exist")
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
		hInst,
		0,
	)
	if hwnd == 0 {
		return 0, errors.Wrap(errors.CodeInputError, "Failed to create message window", err)
	}

	return hwnd, nil
}

func (h *windowsHotkeyListener) Shutdown() {
	if !h.running {
		return
	}

	log.Debug("Shutting down Windows hotkey listener...")
	h.UnregisterAll()

	h.running = false
	if h.hwnd != 0 {
		// 销毁窗口会使窗口线程的 GetMessage 退出循环
		procDestroyWindow.Call(h.hwnd)
	}

	// 等待消息循环退出
	<-h.done
	log.Debug("Windows hotkey listener shutdown complete")
}

// runOnWindowThread 将操作调度到窗口线程执行并等待结果
// （RegisterHotKey/UnregisterHotKey 必须在窗口所属线程上调用）
func (h *windowsHotkeyListener) runOnWindowThread(fn func() errors.AppError) errors.AppError {
	if !h.running || h.hwnd == 0 {
		return errors.New(errors.CodeInputError, "hotkey listener not running")
	}

	op := &windowOp{fn: fn, result: make(chan errors.AppError, 1)}
	h.ops <- op
	// 唤醒窗口线程的消息循环，wndProc 会处理队列中的操作
	procPostMessage.Call(h.hwnd, wmAppExecuteOp, 0, 0)
	return <-op.result
}

func (h *windowsHotkeyListener) RegisterHotkey(hotkey Hotkey) errors.AppError {
	if h.IsRegistered(hotkey.ID) {
		// 已经注册过，先注销
		h.UnregisterHotkey(hotkey.ID)
	}

	err := h.runOnWindowThread(func() errors.AppError {
		// 使用ID从1开始递增
		localID := int(hotkey.ID)
		log.Debugf("Calling RegisterHotKey: hwnd=%p, id=%d, mod=%d, vk=%d", h.hwnd, localID, hotkey.Modifier, hotkey.Key)
		ret, _, err := procRegisterHotKey.Call(h.hwnd, uintptr(localID), uintptr(hotkey.Modifier), uintptr(hotkey.Key))
		if ret == 0 {
			log.Errorf("❌ Failed to register hotkey: mod=%d, key=%d (already taken by another program?)", hotkey.Modifier, hotkey.Key)
			return errors.Wrap(errors.CodeInputError, fmt.Sprintf("Failed to register hotkey %d", hotkey.ID), err)
		}

		h.mu.Lock()
		h.hotkeys[hotkey.ID] = hotkey
		h.registeredIDs[hotkey.ID] = localID
		h.mu.Unlock()

		log.Debugf("✅ Hotkey registered: ID=%d, mod=%d, key=%d", hotkey.ID, hotkey.Modifier, hotkey.Key)
		return nil
	})
	if err != nil {
		return err
	}

	return nil
}

func (h *windowsHotkeyListener) UnregisterHotkey(id HotkeyID) errors.AppError {
	if !h.IsRegistered(id) {
		return nil
	}

	return h.runOnWindowThread(func() errors.AppError {
		h.mu.Lock()
		localID, ok := h.registeredIDs[id]
		h.mu.Unlock()
		if !ok {
			return nil
		}
		ret, _, err := procUnregisterHotKey.Call(h.hwnd, uintptr(localID))
		if ret == 0 {
			return errors.Wrap(errors.CodeInputError, fmt.Sprintf("Failed to unregister hotkey %d", id), err)
		}

		h.mu.Lock()
		delete(h.hotkeys, id)
		delete(h.registeredIDs, id)
		h.mu.Unlock()
		log.Debugf("Hotkey unregistered: ID=%d", id)
		return nil
	})
}

func (h *windowsHotkeyListener) UnregisterAll() {
	log.Debug("Unregistering all hotkeys...")
	h.mu.Lock()
	ids := make([]HotkeyID, 0, len(h.hotkeys))
	for id := range h.hotkeys {
		ids = append(ids, id)
	}
	h.mu.Unlock()

	for _, id := range ids {
		_ = h.UnregisterHotkey(id)
	}
}

// messageLoop 消息循环（必须与 createMessageWindow 在同一线程）
func (h *windowsHotkeyListener) messageLoop() {
	var m msg
	for h.running {
		ret, _, _ := procGetMessage.Call(uintptr(unsafe.Pointer(&m)), h.hwnd, 0, 0)
		if !h.running {
			break
		}
		if int32(ret) == -1 {
			log.Errorf("GetMessage error")
			break
		}
		if ret == 0 {
			// WM_QUIT
			break
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
		procDispatchMessage.Call(uintptr(unsafe.Pointer(&m)))
	}
	close(h.done)
}

func (h *windowsHotkeyListener) IsRegistered(id HotkeyID) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	_, exists := h.registeredIDs[id]
	return exists
}

// wndProc 窗口过程处理热键消息
func (h *windowsHotkeyListener) wndProc(hwnd uintptr, msg, wParam, lParam uintptr) uintptr {
	switch msg {
	case wmHotkey:
		// wParam 是热键ID
		id := HotkeyID(wParam)
		// log.Debugf("Hotkey triggered: ID=%d", id)
		h.mu.Lock()
		hotkey, ok := h.hotkeys[id]
		h.mu.Unlock()
		if ok && hotkey.Callback != nil {
			hotkey.Callback()
		}
		return 0
	case wmAppExecuteOp:
		// 处理其他线程请求的操作（热键注册/注销）
		for {
			select {
			case op := <-h.ops:
				op.result <- op.fn()
			default:
				return 0
			}
		}
	default:
		ret, _, _ := procDefWindowProc.Call(hwnd, msg, wParam, lParam)
		return ret
	}
}
