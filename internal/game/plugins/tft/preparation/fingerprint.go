// Package preparation - PreparationFingerprint 实现（Phase 16-B）
//
// Fingerprint 设计原则：
//   - 仅包含"影响备战分析"的字段
//   - 排除 Board装备 / 经济数据（这些变化不应触发重算）
//   - 只有 Bench / Shop 变化才会导致指纹变更 → 独立缓存失效
package preparation

import (
	"crypto/sha256"
	"encoding/hex"

	"github.com/lipipi/aurora/internal/game/plugins/tft/grounder"
)

// ComputeFingerprint 从 GroundedGameContext 计算 PreparationFingerprint
//
// 输入约定：
//   - g == nil → 返回 zero-value fingerprint
//   - 只包含 Bench 和 Shop 相关字段，不包含棋盘装备/经济
//   - Bench 保持原有顺序；PocketItems 排序保证稳定
func ComputeFingerprint(g grounder.GroundedGameContext) PreparationFingerprint {
	fp := PreparationFingerprint{
		Patch:  g.Meta.Patch,
		SetKey: g.Meta.SetKey,
	}

	// Bench → BenchChampions
	for _, b := range g.Bench {
		if b.Champion.ID == "" || b.Champion.ID == "Unknown" {
			continue
		}
		fp.BenchChampions = append(fp.BenchChampions, BenchChampionRef{
			ChampionID: b.Champion.ID,
			Star:       b.Star,
		})
	}

	// Shop → ShopChampions
	for _, s := range g.Shop {
		if s.Champion.ID == "" || s.Champion.ID == "Unknown" {
			continue
		}
		fp.ShopChampions = append(fp.ShopChampions, ShopChampionRef{
			ChampionID: s.Champion.ID,
			Cost:       s.Cost,
		})
	}

	return fp
}

// Hash 返回 Fingerprint 的 SHA-256 hex（用于日志/debug；不用于 Equal 判定）
func (f PreparationFingerprint) Hash() string {
	h := sha256.New()
	h.Write([]byte(f.Patch))
	h.Write([]byte{0})
	h.Write([]byte(f.SetKey))
	h.Write([]byte{0})
	for _, b := range f.BenchChampions {
		h.Write([]byte(b.ChampionID))
		h.Write([]byte{0})
		h.Write([]byte(itoa(b.Star)))
		h.Write([]byte{0})
	}
	h.Write([]byte{0xff})
	for _, s := range f.ShopChampions {
		h.Write([]byte(s.ChampionID))
		h.Write([]byte{0})
		h.Write([]byte(itoa(s.Cost)))
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}

// IsZero 报告 fingerprint 是否为零值
func (f PreparationFingerprint) IsZero() bool {
	return f.Patch == "" && f.SetKey == "" && len(f.BenchChampions) == 0 && len(f.ShopChampions) == 0
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
