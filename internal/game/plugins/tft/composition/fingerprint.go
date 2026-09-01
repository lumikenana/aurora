// Package composition - CompositionFingerprint 实现（Phase 16-A）
//
// Fingerprint 设计原则：
//   - 仅包含"影响 Board 阵容结构"的字段
//   - 完全排除 Shop / Gold / XP / HP / Stage / Bench（这些变化不应触发重算）
//   - Board 内部排序无关（同阵容不同排列 → 同一 fingerprint）
//   - Hashable：Fingerprint.Equal() 即可判断 cache hit

package composition

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"

	"github.com/lipipi/aurora/internal/game/plugins/tft/grounder"
)

// ComputeFingerprint 从 GroundedGameContext 计算 CompositionFingerprint
//
// 输入约定：
//   - g == nil          → 返回 zero-value fingerprint
//   - Board 槽位为空    → 仍返回带 patch/setKey 的 fingerprint（Champions 字段为空）
//   - ChampionID 重复   → 在 Star/Items map 中合并（同一英雄多位置）
//
// 不排序输出：BoardChampionIDs 内部 sort.Strings，但不影响 Cache Equal
// 行为。ChampionID "" 会被忽略（不计入 fingerprint）。
func ComputeFingerprint(g grounder.GroundedGameContext) CompositionFingerprint {
	fp := CompositionFingerprint{
		Patch:           g.Meta.Patch,
		SetKey:          g.Meta.SetKey,
		StarByChampion:  map[string]int{},
		ItemsByChampion: map[string][]string{},
	}

	// 收集 Board Champion IDs（去重 + 排序）
	idSeen := map[string]struct{}{}
	for _, s := range g.Board {
		id := s.Champion.ID
		if id == "" || id == "Unknown" {
			continue
		}
		if _, ok := idSeen[id]; ok {
			// 同 ID 多次出现 → 在 Star map 中累加（罕见，但应正确处理）
			fp.StarByChampion[id] += s.Star
			continue
		}
		idSeen[id] = struct{}{}
		fp.BoardChampionIDs = append(fp.BoardChampionIDs, id)
		fp.StarByChampion[id] = s.Star
	}
	sort.Strings(fp.BoardChampionIDs)

	// 收集 Board Items（按 ID 排序，确保 fingerprint 一致）
	for _, s := range g.Board {
		id := s.Champion.ID
		if id == "" || id == "Unknown" {
			continue
		}
		if len(s.Items) == 0 {
			continue
		}
		items := make([]string, 0, len(s.Items))
		for _, it := range s.Items {
			items = append(items, it.ID)
		}
		sort.Strings(items)
		fp.ItemsByChampion[id] = items
	}

	return fp
}

// Equal 比较两个 Fingerprint 是否等价
//
// 严格相等：
//   - Patch / SetKey 完全相同
//   - BoardChampionIDs 长度 + 顺序完全相同（已排序）→ 集合等价
//   - StarByChampion：每个 ID 的 Star 值相同
//   - ItemsByChampion：每个 ID 的 Items 序列相同（已排序）
func (f CompositionFingerprint) Equal(other CompositionFingerprint) bool {
	if f.Patch != other.Patch || f.SetKey != other.SetKey {
		return false
	}
	if len(f.BoardChampionIDs) != len(other.BoardChampionIDs) {
		return false
	}
	for i, id := range f.BoardChampionIDs {
		if other.BoardChampionIDs[i] != id {
			return false
		}
	}
	if len(f.StarByChampion) != len(other.StarByChampion) {
		return false
	}
	for id, star := range f.StarByChampion {
		if other.StarByChampion[id] != star {
			return false
		}
	}
	if len(f.ItemsByChampion) != len(other.ItemsByChampion) {
		return false
	}
	for id, items := range f.ItemsByChampion {
		otherItems := other.ItemsByChampion[id]
		if len(items) != len(otherItems) {
			return false
		}
		for i, it := range items {
			if otherItems[i] != it {
				return false
			}
		}
	}
	return true
}

// Hash 返回 Fingerprint 的 SHA-256 hex（用于日志/debug；不用于 Equal 判定）
//
// Equal 优先于 Hash：Equal 不需要序列化，速度更快。
func (f CompositionFingerprint) Hash() string {
	h := sha256.New()
	h.Write([]byte(f.Patch))
	h.Write([]byte{0})
	h.Write([]byte(f.SetKey))
	h.Write([]byte{0})
	for _, id := range f.BoardChampionIDs {
		h.Write([]byte(id))
		h.Write([]byte{0})
	}
	for id, star := range f.StarByChampion {
		h.Write([]byte(id))
		h.Write([]byte{0xFF})
		h.Write([]byte{byte(star)})
		h.Write([]byte{0})
	}
	for id, items := range f.ItemsByChampion {
		h.Write([]byte(id))
		h.Write([]byte{0xFE})
		for _, it := range items {
			h.Write([]byte(it))
			h.Write([]byte{0xFD})
		}
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}

// IsZero 判断 fingerprint 是否为零值（g == nil 时的产物）
func (f CompositionFingerprint) IsZero() bool {
	return f.Patch == "" && f.SetKey == "" && len(f.BoardChampionIDs) == 0
}
