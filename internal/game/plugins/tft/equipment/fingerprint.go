// Package equipment - EquipmentFingerprint 实现（Phase 16-B）
//
// Fingerprint 设计原则：
//   - 仅包含"影响装备分析"的字段
//   - 排除 Gold/Interest/XP/HP/Stage（这些变化不应触发重算）
//   - 排除 Bench/Shop（Phase 16-B 单独处理）
//   - 棋盘装备按 ChampionID + Position 排序（位置无关）
package equipment

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"

	"github.com/lipipi/aurora/internal/game/plugins/tft/grounder"
)

// ComputeFingerprint 从 GroundedGameContext 计算 EquipmentFingerprint
//
// 输入约定：
//   - g == nil          → 返回 zero-value fingerprint
//   - Board 槽位为空    → 仍返回带 patch/setKey 的 fingerprint
//   - 排序：BoardItemSlots 按 Position 升序；ItemIDs 升序；PocketItems 按 ItemID 升序
func ComputeFingerprint(g grounder.GroundedGameContext) EquipmentFingerprint {
	fp := EquipmentFingerprint{
		Patch: g.Meta.Patch,
		SetKey: g.Meta.SetKey,
	}

	// Board 槽位 → BoardItemSlots
	for _, s := range g.Board {
		if s.Champion.ID == "" || s.Champion.ID == "Unknown" {
			continue
		}
		ids := make([]string, 0, len(s.Items))
		for _, it := range s.Items {
			if it.ID == "" {
				continue
			}
			ids = append(ids, it.ID)
		}
		sort.Strings(ids)
		fp.BoardItemSlots = append(fp.BoardItemSlots, BoardItemSlot{
			ChampionID: s.Champion.ID,
			Position:   s.Position,
			Star:       s.Star,
			ItemIDs:    ids,
		})
	}
	// BoardItemSlots 按 Position 排序（保证位置无关 Equal）
	sort.SliceStable(fp.BoardItemSlots, func(i, j int) bool {
		return fp.BoardItemSlots[i].Position < fp.BoardItemSlots[j].Position
	})

	// tctx.Items 散件 → PocketItems
	for _, it := range g.Items {
		if it.Item.ID == "" {
			continue
		}
		fp.PocketItems = append(fp.PocketItems, PocketItem{
			ItemID: it.Item.ID,
			Count:  it.Count,
		})
	}
	sort.SliceStable(fp.PocketItems, func(i, j int) bool {
		return fp.PocketItems[i].ItemID < fp.PocketItems[j].ItemID
	})

	return fp
}

// Hash 返回 Fingerprint 的 SHA-256 hex（用于日志/debug；不用于 Equal 判定）
func (f EquipmentFingerprint) Hash() string {
	h := sha256.New()
	h.Write([]byte(f.Patch))
	h.Write([]byte{0})
	h.Write([]byte(f.SetKey))
	for _, s := range f.BoardItemSlots {
		h.Write([]byte(s.ChampionID))
		h.Write([]byte{0})
		h.Write([]byte(itoa(s.Position)))
		h.Write([]byte{0})
		h.Write([]byte(itoa(s.Star)))
		h.Write([]byte{0})
		for _, id := range s.ItemIDs {
			h.Write([]byte(id))
			h.Write([]byte{0})
		}
		h.Write([]byte{0xff})
	}
	for _, p := range f.PocketItems {
		h.Write([]byte(p.ItemID))
		h.Write([]byte{0})
		h.Write([]byte(itoa(p.Count)))
		h.Write([]byte{0xff})
	}
	return hex.EncodeToString(h.Sum(nil))
}

// IsZero 报告 fingerprint 是否为零值
func (f EquipmentFingerprint) IsZero() bool {
	return f.Patch == "" && f.SetKey == "" && len(f.BoardItemSlots) == 0 && len(f.PocketItems) == 0
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
