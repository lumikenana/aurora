//go:build !windows
// +build !windows

package input

// pollKeyRelease 非 Windows 平台的 PTT 按键释放检测（空实现）
func (m *PTTManager) pollKeyRelease() {
	<-m.done // 等待 done 信号
}
