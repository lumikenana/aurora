//go:build windows

package tts

import "testing"

// TestMCITypeForFormat 验证音频格式 → MCI 设备类型映射。
// 覆盖核心修复：mpegvideo 不可用时降级为 waveaudio。
func TestMCITypeForFormat(t *testing.T) {
	tests := []struct {
		format string
		want   string
	}{
		{"wav", "waveaudio"},
		{".wav", "waveaudio"}, // 带点前缀（filepath.Ext 输出）
		{"mp3", "mpegvideo"},
		{".mp3", "mpegvideo"},
		{"", "mpegvideo"},     // 空格式默认 mpegvideo（向后兼容）
		{"ogg", "mpegvideo"},  // 未知格式走 mpegvideo
		{"WAV", "waveaudio"},  // 大写：本函数内部 ToLower
		{".WAV", "waveaudio"}, // 大写 + 点前缀
	}
	for _, tc := range tests {
		t.Run(tc.format, func(t *testing.T) {
			got := mciTypeForFormat(tc.format)
			if got != tc.want {
				t.Errorf("mciTypeForFormat(%q) = %q, want %q", tc.format, got, tc.want)
			}
		})
	}
}

// TestWindowsPlayerPreferredFormat 验证 windowsPlayer 的 preferredFormat 字段行为。
func TestWindowsPlayerPreferredFormat(t *testing.T) {
	t.Run("default when empty", func(t *testing.T) {
		p := &windowsPlayer{} // preferredFormat 未设置
		if got := p.PreferredFormat(); got != "mp3" {
			t.Errorf("PreferredFormat() = %q, want %q (safe default)", got, "mp3")
		}
	})
	t.Run("explicit wav", func(t *testing.T) {
		p := &windowsPlayer{preferredFormat: "wav"}
		if got := p.PreferredFormat(); got != "wav" {
			t.Errorf("PreferredFormat() = %q, want %q", got, "wav")
		}
	})
	t.Run("explicit mp3", func(t *testing.T) {
		p := &windowsPlayer{preferredFormat: "mp3"}
		if got := p.PreferredFormat(); got != "mp3" {
			t.Errorf("PreferredFormat() = %q, want %q", got, "mp3")
		}
	})
}
