//go:build windows
// +build windows

package audio

import (
	"sync"
	"syscall"
	"unsafe"

	"github.com/lipipi/game-ai/internal/errors"
	"github.com/lipipi/game-ai/internal/log"
)

var (
	winmm                  = syscall.NewLazyDLL("winmm.dll")
	procWaveInGetNumDevs   = winmm.NewProc("waveInGetNumDevs")
	procWaveInOpen         = winmm.NewProc("waveInOpen")
	procWaveInClose        = winmm.NewProc("waveInClose")
	procWaveInPrepareHeader  = winmm.NewProc("waveInPrepareHeader")
	procWaveInUnprepareHeader = winmm.NewProc("waveInUnprepareHeader")
	procWaveInAddBuffer    = winmm.NewProc("waveInAddBuffer")
	procWaveInStart        = winmm.NewProc("waveInStart")
	procWaveInStop         = winmm.NewProc("waveInStop")
	procWaveInReset        = winmm.NewProc("waveInReset")
)

// Winmm constants
const (
	waveInOpenW          = 0x3BE // not used directly, see below
	mmsyserrNoError      = 0
	waveFormatPCM        = 1
	callBackFunction     = 0x00030000
	whdrDone             = 0x00000001
	whdrPrepared         = 0x00000002
	waveInOpenDeviceID   = 0xFFFFFFFF // WAVE_MAPPER
)

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

// windowsAudioCapture Windows waveIn 音频采集实现
type windowsAudioCapture struct {
	cfg       Config
	mu        sync.Mutex
	recording bool
	callback  AudioCallback
	hWaveIn   uintptr
	buffers   []*waveHdr
	dataPtrs  [][]byte // keep references to prevent GC
	done      chan struct{}
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
		log.Warn("Audio capture already recording")
		return nil
	}

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

	// 创建 WAVEFORMATEX
	wfx := waveFormatEx{
		WFormatTag:      waveFormatPCM,
		NChannels:       uint16(format.Channels),
		NSamplesPerSec:  uint32(format.SampleRate),
		WBitsPerSample:  uint16(format.BitsPerSample),
		NBlockAlign:     uint16(format.Channels * format.BitsPerSample / 8),
		NAvgBytesPerSec: uint32(format.SampleRate * format.Channels * format.BitsPerSample / 8),
	}

	// 创建回调函数
	w.callback = cb
	w.done = make(chan struct{})

	// waveIn 回调
	callbackPtr := syscall.NewCallback(waveInCallback)

	// 打开设备
	var hWaveIn uintptr
	ret, _, err := procWaveInOpen.Call(
		uintptr(unsafe.Pointer(&hWaveIn)),
		waveInOpenDeviceID, // WAVE_MAPPER
		uintptr(unsafe.Pointer(&wfx)),
		callbackPtr,
		0, // dwInstance - not used with CALLBACK_FUNCTION
		callBackFunction,
	)
	if ret != mmsyserrNoError {
		return errors.New(errors.CodeAudioFailed, "waveInOpen failed: "+err.Error())
	}
	w.hWaveIn = hWaveIn

	// 创建缓冲区 (4 buffers × 200ms each)
	// 200ms @ 16kHz/16bit/mono = 6400 bytes
	bufSize := format.SampleRate * format.Channels * (format.BitsPerSample / 8) / 5 // 200ms
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

		// Prepare header
		ret, _, err := procWaveInPrepareHeader.Call(
			hWaveIn,
			uintptr(unsafe.Pointer(hdr)),
			unsafe.Sizeof(*hdr),
		)
		if ret != mmsyserrNoError {
			w.cleanupBuffers()
			procWaveInClose.Call(hWaveIn)
			w.hWaveIn = 0
			return errors.New(errors.CodeAudioFailed, "waveInPrepareHeader failed: "+err.Error())
		}

		// Add buffer
		ret, _, err = procWaveInAddBuffer.Call(
			hWaveIn,
			uintptr(unsafe.Pointer(hdr)),
			unsafe.Sizeof(*hdr),
		)
		if ret != mmsyserrNoError {
			w.cleanupBuffers()
			procWaveInClose.Call(hWaveIn)
			w.hWaveIn = 0
			return errors.New(errors.CodeAudioFailed, "waveInAddBuffer failed: "+err.Error())
		}
	}

	w.recording = true
	registerCapture(hWaveIn, w) // 必须在 waveInStart 之前注册，否则回调找不到实例

	// 开始录音
	ret, _, err = procWaveInStart.Call(hWaveIn)
	if ret != mmsyserrNoError {
		unregisterCapture(hWaveIn)
		w.recording = false
		w.cleanupBuffers()
		procWaveInClose.Call(hWaveIn)
		w.hWaveIn = 0
		return errors.New(errors.CodeAudioFailed, "waveInStart failed: "+err.Error())
	}

	log.Info("Audio capture started (waveIn)")
	return nil
}

func (w *windowsAudioCapture) Stop() {
	w.mu.Lock()
	defer w.mu.Unlock()

	if !w.recording {
		return
	}

	log.Debug("Stopping audio capture...")

	// Stop recording
	procWaveInStop.Call(w.hWaveIn)
	procWaveInReset.Call(w.hWaveIn)

	// Unprepare and free buffers
	w.cleanupBuffers()

	// Close device
	unregisterCapture(w.hWaveIn)
	procWaveInClose.Call(w.hWaveIn)
	w.hWaveIn = 0

	w.recording = false
	if w.done != nil {
		close(w.done)
	}
	log.Info("Audio capture stopped")
}

// cleanupBuffers 释放所有缓冲区（调用者必须持有锁，且 hWaveIn 有效）
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

// waveInCallback waveIn 回调函数
// 当缓冲区填满数据时由 Windows 调用
func waveInCallback(hWaveIn uintptr, msg uint32, instance uintptr, param1 uintptr, param2 uintptr) uintptr {
	const wParamData = 0x3BD // WIM_DATA

	if msg != wParamData {
		return 0
	}

	// 获取全局 windowsAudioCapture 实例
	capture := getCaptureInstance(hWaveIn)
	if capture == nil {
		log.Debug("waveIn callback: capture instance not found")
		return 0
	}
	if capture.callback == nil {
		log.Debug("waveIn callback: no callback set")
		return 0
	}

	hdr := (*waveHdr)(unsafe.Pointer(param1))
	if hdr == nil || hdr.DwBytesRecorded == 0 {
		// 空缓冲区，重新入队
		procWaveInAddBuffer.Call(hWaveIn, param1, unsafe.Sizeof(*hdr))
		return 0
	}

	log.Debugf("waveIn callback: %d bytes recorded", hdr.DwBytesRecorded)

	// 拷贝数据（回调中不能阻塞）
	data := make([]byte, hdr.DwBytesRecorded)
	copy(data, unsafe.Slice((*byte)(unsafe.Pointer(hdr.LpData)), hdr.DwBytesRecorded))

	// 重新入队缓冲区
	procWaveInAddBuffer.Call(hWaveIn, param1, unsafe.Sizeof(*hdr))

	// 调用用户回调（异步，不阻塞 waveIn 回调线程）
	go capture.callback(data)

	return 0
}

// 全局实例注册（用于 waveIn 回调查找实例）
var (
	captureRegistry   = make(map[uintptr]*windowsAudioCapture)
	captureRegistryMu sync.Mutex
)

func registerCapture(hWaveIn uintptr, c *windowsAudioCapture) {
	captureRegistryMu.Lock()
	defer captureRegistryMu.Unlock()
	captureRegistry[hWaveIn] = c
}

func unregisterCapture(hWaveIn uintptr) {
	captureRegistryMu.Lock()
	defer captureRegistryMu.Unlock()
	delete(captureRegistry, hWaveIn)
}

func getCaptureInstance(hWaveIn uintptr) *windowsAudioCapture {
	captureRegistryMu.Lock()
	defer captureRegistryMu.Unlock()
	return captureRegistry[hWaveIn]
}
