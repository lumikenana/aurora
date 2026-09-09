//go:build !windows
// +build !windows

package audio

// ListInputDevices 非 Windows 平台暂不支持设备枚举
func ListInputDevices() []AudioDevice {
	return nil
}

// ListOutputDevices 非 Windows 平台暂不支持设备枚举
func ListOutputDevices() []AudioDevice {
	return nil
}

// ResolveInputDeviceID 非 Windows 平台始终返回 0（默认设备）
func ResolveInputDeviceID(_ string) uint32 {
	return 0
}

// ResolveOutputDeviceID 非 Windows 平台始终返回 0（默认设备）
func ResolveOutputDeviceID(_ string) uint32 {
	return 0
}
