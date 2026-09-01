// Package economy - EconomyFingerprint 实现（Phase 16-B）
//
// Fingerprint 设计原则：
//   - 仅包含"影响经济分析"的字段
//   - 排除 Board/Items/Bench/Shop（这些变化不应触发重算）
//   - 只有经济字段变化才会导致指纹变更 → 独立缓存失效
package economy

import (
	"crypto/sha256"
	"encoding/hex"

	"github.com/lipipi/aurora/internal/game/plugins/tft/grounder"
)

// ComputeFingerprint 从 GroundedGameContext 计算 EconomyFingerprint
//
// 输入约定：
//   - g == nil → 返回 zero-value fingerprint
//   - 只包含经济相关字段，不包含棋盘/装备/备战席/商店
func ComputeFingerprint(g grounder.GroundedGameContext) EconomyFingerprint {
	gs := g.GameState
	return EconomyFingerprint{
		Patch:       g.Meta.Patch,
		SetKey:      g.Meta.SetKey,
		Stage:       gs.Stage,
		Phase:       gs.Phase,
		Level:       gs.Level,
		XP:          gs.XP,
		XPToLevel:   gs.XPTolevel,
		Gold:        gs.Gold,
		Interest:    gs.Interest,
		HP:          gs.HP,
		WinStreak:   gs.WinStreak,
		LoseStreak:  gs.LoseStreak,
	}
}

// Hash 返回 Fingerprint 的 SHA-256 hex（用于日志/debug；不用于 Equal 判定）
func (f EconomyFingerprint) Hash() string {
	h := sha256.New()
	h.Write([]byte(f.Patch))
	h.Write([]byte{0})
	h.Write([]byte(f.SetKey))
	h.Write([]byte{0})
	h.Write([]byte(f.Stage))
	h.Write([]byte{0})
	h.Write([]byte(f.Phase))
	h.Write([]byte{0})
	h.Write([]byte(itoa(f.Level)))
	h.Write([]byte{0})
	h.Write([]byte(itoa(f.XP)))
	h.Write([]byte{0})
	h.Write([]byte(itoa(f.XPToLevel)))
	h.Write([]byte{0})
	h.Write([]byte(itoa(f.Gold)))
	h.Write([]byte{0})
	h.Write([]byte(itoa(f.Interest)))
	h.Write([]byte{0})
	h.Write([]byte(itoa(f.HP)))
	h.Write([]byte{0})
	h.Write([]byte(itoa(f.WinStreak)))
	h.Write([]byte{0})
	h.Write([]byte(itoa(f.LoseStreak)))
	return hex.EncodeToString(h.Sum(nil))
}

// IsZero 报告 fingerprint 是否为零值
func (f EconomyFingerprint) IsZero() bool {
	return f.Patch == "" && f.SetKey == "" && f.Level == 0 && f.Gold == 0 &&
		f.Stage == "" && f.WinStreak == 0 && f.LoseStreak == 0
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	neg := false
	if i < 0 {
		neg = true
		i = -i
	}
	var buf [20]byte
	pos := len(buf)
	for i > 0 {
		pos--
		buf[pos] = byte('0' + i%10)
		i /= 10
	}
	if neg {
		pos--
		buf[pos] = '-'
	}
	return string(buf[pos:])
}
