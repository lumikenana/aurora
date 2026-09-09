//go:build windows
// +build windows

package audio

import (
	"unicode/utf16"
	"unsafe"

	"github.com/lipipi/aurora/internal/log"
)

var (
	procWaveInGetDevCapsW  = winmm.NewProc("waveInGetDevCapsW")
	procWaveOutGetNumDevs  = winmm.NewProc("waveOutGetNumDevs")
	procWaveOutGetDevCapsW = winmm.NewProc("waveOutGetDevCapsW")
)

// maxPNameLen WAVEINCAPSW/WAVEOUTCAPSW 中 szPname 的最大长度
const maxPNameLen = 32

// defaultDeviceID 配置中表示"跟随系统默认"的标识
const defaultDeviceID = "default"

// waveInCapsW 对应 WAVEINCAPSW，仅提取 szPname
type waveInCapsW struct {
	WMid          uint16
	WPid          uint16
	VDriverVer    uint32
	SzPname       [maxPNameLen]uint16
	DwFormats     uint32
	WChannels     uint16
	WReserved1    uint16
}

// waveOutCapsW 对应 WAVEOUTCAPSW
type waveOutCapsW struct {
	WMid          uint16
	WPid          uint16
	VDriverVer    uint32
	SzPname       [maxPNameLen]uint16
	DwFormats     uint32
	WChannels     uint16
	WReserved1    uint16
}

// ListInputDevices 枚举所有 Windows 输入设备（麦克风等）
func ListInputDevices() []AudioDevice {
	numDevs, _, _ := procWaveInGetNumDevs.Call()
	if numDevs == 0 {
		return nil
	}

	devices := make([]AudioDevice, 0, numDevs)
	for i := uint32(0); i < uint32(numDevs); i++ {
		var caps waveInCapsW
		ret, _, _ := procWaveInGetDevCapsW.Call(
			uintptr(i),
			uintptr(unsafe.Pointer(&caps)),
			uintptr(unsafe.Sizeof(caps)),
		)
		if ret != 0 {
			log.Debugf("waveInGetDevCapsW(%d) failed: ret=%d", i, ret)
			continue
		}
		name := utf16ToString(caps.SzPname[:])
		devices = append(devices, AudioDevice{
			ID:    name,
			Name:  name,
			Index: int(i),
		})
	}
	return devices
}

// ListOutputDevices 枚举所有 Windows 输出设备（扬声器/耳机等）
func ListOutputDevices() []AudioDevice {
	numDevs, _, _ := procWaveOutGetNumDevs.Call()
	if numDevs == 0 {
		return nil
	}

	devices := make([]AudioDevice, 0, numDevs)
	for i := uint32(0); i < uint32(numDevs); i++ {
		var caps waveOutCapsW
		ret, _, _ := procWaveOutGetDevCapsW.Call(
			uintptr(i),
			uintptr(unsafe.Pointer(&caps)),
			uintptr(unsafe.Sizeof(caps)),
		)
		if ret != 0 {
			log.Debugf("waveOutGetDevCapsW(%d) failed: ret=%d", i, ret)
			continue
		}
		name := utf16ToString(caps.SzPname[:])
		devices = append(devices, AudioDevice{
			ID:    name,
			Name:  name,
			Index: int(i),
		})
	}
	return devices
}

// ResolveInputDeviceID 将配置中的 InputDeviceID 解析为 waveIn 设备索引
//
// 返回 waveMapperDeviceID (0xFFFFFFFF) 表示使用系统默认设备。
// 如果指定设备不存在，返回 waveMapperDeviceID + 日志警告。
func ResolveInputDeviceID(deviceID string) uint32 {
	return resolveDeviceID(deviceID, ListInputDevices, "input")
}

// ResolveOutputDeviceID 将配置中的 OutputDeviceID 解析为 waveOut 设备索引
func ResolveOutputDeviceID(deviceID string) uint32 {
	return resolveDeviceID(deviceID, ListOutputDevices, "output")
}

// resolveDeviceID 通用设备 ID 解析逻辑
func resolveDeviceID(deviceID string, listFunc func() []AudioDevice, direction string) uint32 {
	// "default" / "" → WAVE_MAPPER
	if deviceID == "" || deviceID == defaultDeviceID {
		return waveMapperDeviceID
	}

	// 枚举设备，按 ID 精确匹配
	devices := listFunc()
	var matchIndex int = -1
	matchCount := 0

	for _, dev := range devices {
		if dev.ID == deviceID {
			if matchCount == 0 {
				matchIndex = dev.Index
			}
			matchCount++
		}
	}

	switch {
	case matchCount == 0:
		log.Warnf("Audio %s device %q not found, falling back to system default", direction, deviceID)
		return waveMapperDeviceID
	case matchCount > 1:
		log.Warnf("Audio %s device %q: %d devices with same name, using first match (index=%d)",
			direction, deviceID, matchCount, matchIndex)
		return uint32(matchIndex)
	default:
		log.Debugf("Audio %s device %q resolved to index %d", direction, deviceID, matchIndex)
		return uint32(matchIndex)
	}
}

// utf16ToString 将 uint16 切片（null-terminated）转为 Go string
func utf16ToString(arr []uint16) string {
	// 找到 null terminator
	n := 0
	for n < len(arr) && arr[n] != 0 {
		n++
	}
	return string(utf16.Decode(arr[:n]))
}
