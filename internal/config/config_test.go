package config

import (
	"encoding/json"
	"strings"
	"testing"
)

// 旧 Continuous 配置（Phase 3 移除 ModeContinuous、Phase 6 移除互斥 voice_mode 前的格式）。
const legacyContinuousConfigJSON = `{
  "voice_mode": "continuous",
  "continuous_timeout_seconds": 30,
  "continuous_exit_commands": ["退出", "停止对话"],
  "log_level": "info"
}`

// TestLegacyContinuousConfig_ReadIgnored 旧 continuous / voice_mode JSON 键读取时安全忽略：
// 不报错（Config struct 无对应字段）；wakeword.enabled 未被 JSON 设置时保持默认。
func TestLegacyContinuousConfig_ReadIgnored(t *testing.T) {
	cfg := DefaultConfig()
	// 显式关闭，验证迁移会打开
	cfg.WakeWord.Enabled = false
	if err := json.Unmarshal([]byte(legacyContinuousConfigJSON), cfg); err != nil {
		t.Fatalf("legacy config should unmarshal without error: %v", err)
	}
	if cfg.WakeWord.Enabled {
		t.Fatal("wakeword.enabled should stay false before migration")
	}

	// 旧 continuous 用户当时在使用唤醒 → 迁移为启用 WakeWord Trigger
	migrateLegacyVoiceMode([]byte(legacyContinuousConfigJSON), cfg)
	if !cfg.WakeWord.Enabled {
		t.Fatal("legacy voice_mode=continuous should migrate to wakeword trigger enabled")
	}
}

// TestLegacyVoiceModeMigration 旧互斥 voice_mode → Trigger 开关迁移规则（Phase 6）：
//   - "wakeword" / "continuous" → wakeword.enabled = true
//   - "ptt" / 缺失 / 非法值 → 保持 wakeword.enabled 原值
func TestLegacyVoiceModeMigration(t *testing.T) {
	cases := []struct {
		name    string
		json    string
		initial bool
		want    bool
	}{
		{"wakeword enables trigger", `{"voice_mode":"wakeword"}`, false, true},
		{"continuous enables trigger", `{"voice_mode":"continuous"}`, false, true},
		{"ptt keeps trigger off", `{"voice_mode":"ptt"}`, false, false},
		{"ptt keeps explicit on", `{"voice_mode":"ptt"}`, true, true},
		{"missing key keeps value", `{"log_level":"info"}`, false, false},
		{"garbage keeps value", `{"voice_mode":"garbage"}`, true, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			cfg := DefaultConfig()
			cfg.WakeWord.Enabled = c.initial
			migrateLegacyVoiceMode([]byte(c.json), cfg)
			if cfg.WakeWord.Enabled != c.want {
				t.Errorf("wakeword.enabled = %v, want %v", cfg.WakeWord.Enabled, c.want)
			}
		})
	}
}

// TestLegacyConfig_SaveEliminatesModeKeys 迁移后保存：
// 旧 voice_mode / continuous_* 键全部消失（不再生成互斥 Mode），
// 重新读取后 wakeword.enabled 保持，且迁移幂等（无 voice_mode 键 → 不改配置）。
func TestLegacyConfig_SaveEliminatesModeKeys(t *testing.T) {
	cfg := DefaultConfig()
	cfg.WakeWord.Enabled = false
	if err := json.Unmarshal([]byte(legacyContinuousConfigJSON), cfg); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	migrateLegacyVoiceMode([]byte(legacyContinuousConfigJSON), cfg)
	if !cfg.WakeWord.Enabled {
		t.Fatal("migration should enable wakeword trigger")
	}

	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	s := string(data)
	for _, key := range []string{"voice_mode", "continuous_timeout_seconds", "continuous_exit_commands", "continuous"} {
		if strings.Contains(s, key) {
			t.Fatalf("saved config should not contain %q, got: %s", key, s)
		}
	}

	// 重新读取：wakeword.enabled 保留；无 voice_mode 键 → 迁移幂等
	reloaded := DefaultConfig()
	reloaded.WakeWord.Enabled = false
	if err := json.Unmarshal(data, reloaded); err != nil {
		t.Fatalf("re-unmarshal: %v", err)
	}
	migrateLegacyVoiceMode(data, reloaded)
	if !reloaded.WakeWord.Enabled {
		t.Fatal("reloaded wakeword.enabled should stay true")
	}
}
