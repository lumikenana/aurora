package config

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/lipipi/aurora/internal/input"
)

// 旧 Continuous 配置（Phase 3 移除 ModeContinuous 前的格式）。
const legacyContinuousConfigJSON = `{
  "voice_mode": "continuous",
  "continuous_timeout_seconds": 30,
  "continuous_exit_commands": ["退出", "停止对话"],
  "log_level": "info"
}`

// TestLegacyContinuousConfig_ReadIgnored 旧 continuous JSON 键读取时安全忽略：
// 不报错、不参与运行逻辑（Config struct 无对应字段），voice_mode 原始值保留
// 直到 normalizeVoiceMode 归一化。
func TestLegacyContinuousConfig_ReadIgnored(t *testing.T) {
	cfg := DefaultConfig()
	if err := json.Unmarshal([]byte(legacyContinuousConfigJSON), cfg); err != nil {
		t.Fatalf("legacy config should unmarshal without error: %v", err)
	}
	if cfg.VoiceMode != "continuous" {
		t.Fatalf("raw voice_mode should be preserved on read, got %q", cfg.VoiceMode)
	}
	// 运行逻辑入口：ParseVoiceMode 兼容映射 continuous → wakeword
	if m := input.ParseVoiceMode(cfg.VoiceMode); m != input.ModeWakeWord {
		t.Fatalf("continuous should map to ModeWakeWord, got %v", m)
	}
}

// TestLegacyContinuousConfig_SaveEliminatesOldKeys 归一化后保存：
// 旧 continuous_* 键消失，voice_mode 只写 "ptt" / "wakeword"；重新读取正确。
func TestLegacyContinuousConfig_SaveEliminatesOldKeys(t *testing.T) {
	cfg := DefaultConfig()
	if err := json.Unmarshal([]byte(legacyContinuousConfigJSON), cfg); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	// 模拟 Init 加载路径的归一化
	normalizeVoiceMode(cfg)
	if cfg.VoiceMode != "wakeword" {
		t.Fatalf("voice_mode should be normalized to wakeword, got %q", cfg.VoiceMode)
	}

	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	s := string(data)
	for _, key := range []string{"continuous_timeout_seconds", "continuous_exit_commands", "continuous"} {
		if strings.Contains(s, key) {
			t.Fatalf("saved config should not contain %q, got: %s", key, s)
		}
	}

	// 重新读取仍然正确
	reloaded := DefaultConfig()
	if err := json.Unmarshal(data, reloaded); err != nil {
		t.Fatalf("re-unmarshal: %v", err)
	}
	normalizeVoiceMode(reloaded)
	if reloaded.VoiceMode != "wakeword" {
		t.Fatalf("reloaded voice_mode should be wakeword, got %q", reloaded.VoiceMode)
	}
	if m := input.ParseVoiceMode(reloaded.VoiceMode); m != input.ModeWakeWord {
		t.Fatalf("reloaded mode should be ModeWakeWord, got %v", m)
	}
}

// TestNormalizeVoiceMode 归一化只产生 "ptt" / "wakeword" 两个合法值。
func TestNormalizeVoiceMode(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"ptt", "ptt"},
		{"wakeword", "wakeword"},
		{"continuous", "wakeword"},
		{"", "wakeword"},
		{"garbage", "wakeword"},
	}
	for _, c := range cases {
		cfg := DefaultConfig()
		cfg.VoiceMode = c.in
		normalizeVoiceMode(cfg)
		if cfg.VoiceMode != c.want {
			t.Errorf("normalizeVoiceMode(%q) = %q, want %q", c.in, cfg.VoiceMode, c.want)
		}
	}
}
