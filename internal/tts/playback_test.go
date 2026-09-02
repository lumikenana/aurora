package tts

import "testing"

// TestPlayerInterfacePreferredFormat 验证 Player 接口包含 PreferredFormat 方法。
// 使用编译期检查确保所有 Player 实现都满足接口。
func TestPlayerInterfacePreferredFormat(t *testing.T) {
	// 编译期断言：NewPlatformPlayer 返回的 Player 必须实现 PreferredFormat。
	// 如果某个平台的实现遗漏了 PreferredFormat，这里会编译失败。
	var p Player = NewPlatformPlayer()
	format := p.PreferredFormat()
	if format == "" {
		t.Error("Player.PreferredFormat() returned empty string, want non-empty (e.g. 'mp3' or 'wav')")
	}
}
