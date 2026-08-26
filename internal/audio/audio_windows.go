//go:build windows
// +build windows

package audio

import (
	"runtime"
	"sync"
	"syscall"
	"unsafe"

	"github.com/lipipi/game-ai/internal/errors"
	"github.com/lipipi/game-ai/internal/log"
)

var (
	winmm                     = syscall.NewLazyDLL("winmm.dll")
	procWaveInGetNumDevs      = winmm.NewProc("waveInGetNumDevs")
	procWaveInOpen            = winmm.NewProc("waveInOpen")
	procWaveInClose           = winmm.NewProc("waveInClose")
	procWaveInPrepareHeader   = winmm.NewProc("waveInPrepareHeader")
	procWaveInUnprepareHeader = winmm.NewProc("waveInUnprepareHeader")
	procWaveInAddBuffer       = winmm.NewProc("waveInAddBuffer")
	procWaveInStart           = winmm.NewProc("waveInStart")
	procWaveInStop            = winmm.NewProc("waveInStop")
	procWaveInReset           = winmm.NewProc("waveInReset")

	user32                = syscall.NewLazyDLL("user32.dll")
	procRegisterClassExW  = user32.NewProc("RegisterClassExW")
	procCreateWindowExW   = user32.NewProc("CreateWindowExW")
	procDestroyWindow     = user32.NewProc("DestroyWindow")
	procDefWindowProcW    = user32.NewProc("DefWindowProcW")
	procGetMessageW       = user32.NewProc("GetMessageW")
	procTranslateMessage  = user32.NewProc("TranslateMessage")
	procDispatchMessageW  = user32.NewProc("DispatchMessageW")
	procPostMessageW      = user32.NewProc("PostMessageW")
)

// Windows constants
const (
	mmsyserrNoError    = 0
	waveFormatPCM      = 1
	callbackWindow     = 0x00010000 // CALLBACK_WINDOW
	waveMapperDeviceID = 0xFFFFFFFF // WAVE_MAPPER

	// Window messages (waveIn)
	// MM_WIM_OPEN = 0x3BE, MM_WIM_CLOSE = 0x3BF, MM_WIM_DATA = 0x3C0

	// Window class
	cwUseDefault = ^0x7FFFFFFF // CW_USEDEFAULT as int32
)

// Correct WIM_DATA value from Windows SDK (MM_WIM_DATA)
// MM_WIM_OPEN = 0x3BE, MM_WIM_CLOSE = 0x3BF, MM_WIM_DATA = 0x3C0
const wimDataMsg = 0x3C0

// WAVEFORMATEX 结构
type waveFormatEx struct {
	WFormatTag      uint16
	NChannels       uint16
	NSamplesPerSec  uint32
	NAvgBytesPerSec uint32
	NBlockAlign     uint16
	WBitsPerSample  uint16
	CbSize          uint16
}

// WAVEHDR 结构
type waveHdr struct {
	LpData          uintptr
	DwBufferLength  uint32
	DwBytesRecorded uint32
	DwUser          uintptr
	DwFlags         uint32
	DwLoops         uint32
	LpNext          uintptr
	Reserved        uintptr
}

// MSG 结构
type msg struct {
	HWnd    uintptr
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	PtX     int32
	PtY     int32
}

// WNDCLASSEXW 结构 (简化版)
type wndClassExW struct {
	CbSize        uint32
	Style         uint32
	LpfnWndProc   uintptr
	CbClsExtra    int32
	CbWndExtra    int32
	HInstance     uintptr
	HIcon         uintptr
	HCursor       uintptr
	HbrBackground uintptr
	LpszMenuName  *uint16
	LpszClassName *uint16
	HIconSm       uintptr
}

// windowsAudioCapture Windows waveIn 音频采集实现
// 使用 CALLBACK_WINDOW 模式：waveIn 发送消息到隐藏窗口，消息循环处理数据
type windowsAudioCapture struct {
	cfg       Config
	mu        sync.Mutex
	recording bool
	callback  AudioCallback
	hWaveIn   uintptr
	hwnd      uintptr
	buffers   []*waveHdr
	dataPtrs  [][]byte
	done      chan struct{}
	threadDone chan struct{}
}

func init() {
	RegisterProvider("windows", NewWindowsAudioCapture)
}

// NewWindowsAudioCapture 创建 Windows 音频采集 Provider
func NewWindowsAudioCapture(cfg Config) (AudioCapture, errors.AppError) {
	return &windowsAudioCapture{cfg: cfg}, nil
}

func (w *windowsAudioCapture) Name() string {
	return "windows-wavein"
}

func (w *windowsAudioCapture) IsAvailable() bool {
	ret, _, _ := procWaveInGetNumDevs.Call()
	log.Debugf("Audio devices found: %d", ret)
	return ret > 0
}

func (w *windowsAudioCapture) IsRecording() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.recording
}

func (w *windowsAudioCapture) Start(cb AudioCallback) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.recording {
		return nil
	}

	log.Info("Audio capture starting (waveIn CALLBACK_WINDOW mode)...")

	format := w.cfg.Format
	if format.SampleRate == 0 {
		format.SampleRate = 16000
	}
	if format.BitsPerSample == 0 {
		format.BitsPerSample = 16
	}
	if format.Channels == 0 {
		format.Channels = 1
	}

	w.callback = cb
	w.done = make(chan struct{})
	w.threadDone = make(chan struct{})

	// 启动专用线程创建消息窗口和消息循环
	ready := make(chan uintptr, 1)
	errCh := make(chan error, 1)

	go func() {
		runtime.LockOSThread()
		defer func() {
			close(w.threadDone)
		}()

		hwnd, err := w.createMessageWindow()
		if err != nil {
			errCh <- err
			return
		}
		w.hwnd = hwnd
		ready <- hwnd

		// 消息循环
		w.messageLoop()
	}()

	// 等待窗口创建
	select {
	case hwnd := <-ready:
		log.Debugf("Audio message window created: 0x%x", hwnd)
	case err := <-errCh:
		return errors.Wrap(errors.CodeAudioFailed, "create audio window", err)
	}

	// 创建 WAVEFORMATEX
	wfx := waveFormatEx{
		WFormatTag:      waveFormatPCM,
		NChannels:       uint16(format.Channels),
		NSamplesPerSec:  uint32(format.SampleRate),
		WBitsPerSample:  uint16(format.BitsPerSample),
		NBlockAlign:     uint16(format.Channels * format.BitsPerSample / 8),
		NAvgBytesPerSec: uint32(format.SampleRate * format.Channels * format.BitsPerSample / 8),
	}

	// 打开设备 (CALLBACK_WINDOW 模式：消息发送到 w.hwnd)
	var hWaveIn uintptr
	ret, _, msgText := procWaveInOpen.Call(
		uintptr(unsafe.Pointer(&hWaveIn)),
		waveMapperDeviceID,
		uintptr(unsafe.Pointer(&wfx)),
		w.hwnd,            // callback = window handle
		0,                 // dwInstance (unused with CALLBACK_WINDOW)
		callbackWindow,    // CALLBACK_WINDOW
	)
	if ret != mmsyserrNoError {
		w.destroyWindow()
		return errors.New(errors.CodeAudioFailed, "waveInOpen failed: "+msgText.Error())
	}
	w.hWaveIn = hWaveIn
	log.Debugf("waveIn device opened, handle=0x%x", hWaveIn)

	// 创建缓冲区 (4 buffers × 200ms each)
	// 200ms @ 16kHz/16bit/mono = 6400 bytes
	bufSize := format.SampleRate * format.Channels * (format.BitsPerSample / 8) / 5
	w.buffers = make([]*waveHdr, 4)
	w.dataPtrs = make([][]byte, 4)

	for i := 0; i < 4; i++ {
		data := make([]byte, bufSize)
		w.dataPtrs[i] = data

		hdr := &waveHdr{
			LpData:         uintptr(unsafe.Pointer(&data[0])),
			DwBufferLength: uint32(len(data)),
		}
		w.buffers[i] = hdr

		ret, _, err := procWaveInPrepareHeader.Call(
			hWaveIn,
			uintptr(unsafe.Pointer(hdr)),
			unsafe.Sizeof(*hdr),
		)
		if ret != mmsyserrNoError {
			w.cleanupBuffers()
			procWaveInClose.Call(hWaveIn)
			w.destroyWindow()
			return errors.New(errors.CodeAudioFailed, "waveInPrepareHeader failed: "+err.Error())
		}

		ret, _, err = procWaveInAddBuffer.Call(
			hWaveIn,
			uintptr(unsafe.Pointer(hdr)),
			unsafe.Sizeof(*hdr),
		)
		if ret != mmsyserrNoError {
			w.cleanupBuffers()
			procWaveInClose.Call(hWaveIn)
			w.destroyWindow()
			return errors.New(errors.CodeAudioFailed, "waveInAddBuffer failed: "+err.Error())
		}
	}

	// 开始录音
	ret, _, err := procWaveInStart.Call(hWaveIn)
	if ret != mmsyserrNoError {
		w.cleanupBuffers()
		procWaveInClose.Call(hWaveIn)
		w.destroyWindow()
		return errors.New(errors.CodeAudioFailed, "waveInStart failed: "+err.Error())
	}

	w.recording = true
	log.Infof("Audio capture started: %dHz/%dbit/%dch, bufSize=%d, 4 buffers",
		format.SampleRate, format.BitsPerSample, format.Channels, bufSize)
	return nil
}

func (w *windowsAudioCapture) Stop() {
	w.mu.Lock()
	defer w.mu.Unlock()

	if !w.recording {
		return
	}

	log.Debug("Stopping audio capture...")

	procWaveInStop.Call(w.hWaveIn)
	procWaveInReset.Call(w.hWaveIn)

	w.cleanupBuffers()
	procWaveInClose.Call(w.hWaveIn)
	w.hWaveIn = 0

	w.recording = false

	// 关闭消息窗口（会退出消息循环）
	w.destroyWindow()

	if w.done != nil {
		select {
		case <-w.done:
		default:
			close(w.done)
		}
	}

	// 等待消息线程退出
	<-w.threadDone

	log.Info("Audio capture stopped")
}

// destroyWindow 销毁消息窗口（必须在窗口线程调用，或通过 PostMessage 触发退出）
func (w *windowsAudioCapture) destroyWindow() {
	if w.hwnd != 0 {
		// PostMessage WM_QUIT to exit message loop
		const wmQuit = 0x0012
		procPostMessageW.Call(w.hwnd, wmQuit, 0, 0)
		w.hwnd = 0
	}
}

// createMessageWindow 创建隐藏消息窗口
func (w *windowsAudioCapture) createMessageWindow() (uintptr, error) {
	className, _ := syscall.UTF16PtrFromString("GameAIAudioWindow")
	windowName, _ := syscall.UTF16PtrFromString("GameAI Audio")

	// 窗口过程
	wndProcPtr := syscall.NewCallback(audioWndProc)

	// 获取 hInstance
	hInstance, _, _ := syscall.NewLazyDLL("kernel32.dll").NewProc("GetModuleHandleW").Call(0)

	wndClass := wndClassExW{
		CbSize:        uint32(unsafe.Sizeof(wndClassExW{})),
		LpfnWndProc:   wndProcPtr,
		HInstance:     hInstance,
		LpszClassName: className,
	}

	ret, _, err := procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wndClass)))
	if ret == 0 {
		log.Warnf("RegisterClassExW failed (may already be registered): %v", err)
	}

	// 创建消息窗口 (HWND_MESSAGE = -3)
	const hwndMessage = ^uintptr(2) // (HWND)-3
	hwnd, _, err := procCreateWindowExW.Call(
		0,             // dwExStyle
		uintptr(unsafe.Pointer(className)),
		uintptr(unsafe.Pointer(windowName)),
		0,             // dwStyle
		0, 0, 0, 0,   // x, y, width, height
		hwndMessage,   // parent = HWND_MESSAGE
		0,             // menu
		hInstance,
		0,             // lpParam
	)
	if hwnd == 0 {
		return 0, errors.New(errors.CodeAudioFailed, "CreateWindowExW failed: "+err.Error())
	}

	// 将此窗口与 capture 关联
	setAudioWindowCapture(hwnd, w)

	return hwnd, nil
}

// messageLoop 消息循环 - 处理 WIM_DATA 消息
func (w *windowsAudioCapture) messageLoop() {
	var m msg
	for {
		ret, _, _ := procGetMessageW.Call(
			uintptr(unsafe.Pointer(&m)),
			0,   // hwnd (all windows for this thread)
			0,   // wMsgFilterMin
			0,   // wMsgFilterMax
		)
		if ret == 0 || ret == ^uintptr(0) {
			// WM_QUIT or error
			break
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
		procDispatchMessageW.Call(uintptr(unsafe.Pointer(&m)))
	}
}

// cleanupBuffers 释放所有缓冲区
func (w *windowsAudioCapture) cleanupBuffers() {
	for _, hdr := range w.buffers {
		if hdr != nil {
			procWaveInUnprepareHeader.Call(
				w.hWaveIn,
				uintptr(unsafe.Pointer(hdr)),
				unsafe.Sizeof(*hdr),
			)
		}
	}
	w.buffers = nil
	w.dataPtrs = nil
}

// --- 窗口过程 ---

// audioWndProc 处理音频窗口的消息
func audioWndProc(hwnd uintptr, msg uint32, wParam uintptr, lParam uintptr) uintptr {
	if msg == wimDataMsg {
		capture := getAudioWindowCapture(hwnd)
		if capture == nil || capture.callback == nil {
			return 0
		}

		hdr := (*waveHdr)(unsafe.Pointer(lParam))
		if hdr == nil {
			return 0
		}

		if hdr.DwBytesRecorded > 0 {
			// 拷贝数据
			data := make([]byte, hdr.DwBytesRecorded)
			copy(data, unsafe.Slice((*byte)(unsafe.Pointer(hdr.LpData)), hdr.DwBytesRecorded))

			// 重新入队缓冲区
			procWaveInAddBuffer.Call(capture.hWaveIn, lParam, unsafe.Sizeof(*hdr))

			// 异步回调（不阻塞消息循环）
			go capture.callback(data)
		} else {
			// 空缓冲区，直接重新入队
			procWaveInAddBuffer.Call(capture.hWaveIn, lParam, unsafe.Sizeof(*hdr))
		}
		return 0
	}

	ret, _, _ := procDefWindowProcW.Call(hwnd, uintptr(msg), wParam, lParam)
	return ret
}

// --- HWND ↔ Capture 关联 ---

var (
	windowCaptureMap   = make(map[uintptr]*windowsAudioCapture)
	windowCaptureMapMu sync.Mutex
)

func setAudioWindowCapture(hwnd uintptr, c *windowsAudioCapture) {
	windowCaptureMapMu.Lock()
	defer windowCaptureMapMu.Unlock()
	windowCaptureMap[hwnd] = c
}

func getAudioWindowCapture(hwnd uintptr) *windowsAudioCapture {
	windowCaptureMapMu.Lock()
	defer windowCaptureMapMu.Unlock()
	return windowCaptureMap[hwnd]
}

func removeAudioWindowCapture(hwnd uintptr) {
	windowCaptureMapMu.Lock()
	defer windowCaptureMapMu.Unlock()
	delete(windowCaptureMap, hwnd)
}
