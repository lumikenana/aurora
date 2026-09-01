// Package knowledge 数据转换：CommunityDragon 载荷 / 手动导入 envelope → NormalizedPatchData
package knowledge

import (
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/lipipi/aurora/internal/errors"
)

// cdItem CommunityDragon items[] 条目（强化符文混在其中：isAugment=true 且 id=null）
type cdItem struct {
	APIName   string          `json:"apiName"`
	Name      string          `json:"name"`
	ID        *int            `json:"id"` // 装备有数字 id，强化符文为 null
	Desc      string          `json:"desc"`
	IsAugment bool            `json:"isAugment"`
	Effects   json.RawMessage `json:"effects"`
	From      []string        `json:"from"`
}

// cdChampion CommunityDragon sets.{n}.champions[] 条目
type cdChampion struct {
	APIName string          `json:"apiName"`
	Name    string          `json:"name"`
	Cost    int             `json:"cost"`
	Traits  []string        `json:"traits"`
	Ability json.RawMessage `json:"ability"`
	Stats   json.RawMessage `json:"stats"`
	Role    string          `json:"role"`
}

// cdTrait CommunityDragon sets.{n}.traits[] 条目
type cdTrait struct {
	APIName string          `json:"apiName"`
	Name    string          `json:"name"`
	Desc    json.RawMessage `json:"desc"`
	Effects json.RawMessage `json:"effects"`
	Type    string          `json:"type"`
}

// cdSet 单个赛季的数据
type cdSet struct {
	Champions []cdChampion `json:"champions"`
	Traits    []cdTrait    `json:"traits"`
}

// cdPayload CommunityDragon cdragon/tft/{locale}.json 结构（实测 2026-08-28）
type cdPayload struct {
	Items []cdItem           `json:"items"`
	Sets  map[string]cdSet   `json:"sets"`
}

// ParseCDPayload 解析 CommunityDragon TFT 载荷
// game/patch 由调用方注入（CD 文件本身不带版本标签）
func ParseCDPayload(raw []byte, game, patch string) (*NormalizedPatchData, error) {
	var p cdPayload
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, fmt.Errorf("invalid CommunityDragon payload: %w", err)
	}
	if len(p.Sets) == 0 && len(p.Items) == 0 {
		return nil, fmt.Errorf("payload has no sets/items data")
	}

	d := &NormalizedPatchData{Game: game, Patch: patch, Source: "communitydragon"}

	// 装备：过滤强化符文（isAugment 或 id 为 null）
	for _, it := range p.Items {
		if it.IsAugment || it.ID == nil || it.Name == "" {
			continue
		}
		d.Items = append(d.Items, Record{ID: it.APIName, Name: it.Name, Data: mustMarshal(it)})
	}

	// 取最大数字 key 的赛季为当前赛季
	maxSet := -1
	var current *cdSet
	for key, set := range p.Sets {
		n, err := strconv.Atoi(key)
		if err != nil {
			continue
		}
		if n > maxSet {
			maxSet = n
			s := set // 拷贝 map 值
			current = &s
		}
	}
	if current != nil {
		for _, c := range current.Champions {
			if c.Name == "" {
				continue
			}
			d.Champions = append(d.Champions, Record{ID: c.APIName, Name: c.Name, Data: mustMarshal(c)})
		}
		for _, t := range current.Traits {
			if t.Name == "" {
				continue
			}
			d.Traits = append(d.Traits, Record{ID: t.APIName, Name: t.Name, Data: mustMarshal(t)})
		}
	}

	return d, nil
}

// envelope 手动导入格式（patch 必填——版本隔离的硬性要求）
type envelope struct {
	Game  string            `json:"game"`
	Patch string            `json:"patch"`
	Kind  string            `json:"kind"` // champions|items|traits|compositions
	Data  []json.RawMessage `json:"data"`
}

// ParseEnvelope 解析手动导入的 envelope 格式
// 记录内 id/name 之外的字段（如阵容的 tier/champions/coreItems）原样保留在 Data 中
func ParseEnvelope(raw []byte) (*NormalizedPatchData, errors.AppError) {
	var e envelope
	if err := json.Unmarshal(raw, &e); err != nil {
		return nil, errors.New(errors.CodeInvalidConfig, "invalid knowledge import JSON: "+err.Error())
	}
	if e.Game == "" {
		return nil, errors.New(errors.CodeInvalidConfig, "knowledge import missing 'game' field")
	}
	if e.Patch == "" {
		return nil, errors.New(errors.CodeInvalidConfig, "knowledge import missing 'patch' field (所有数据必须带版本)")
	}
	if len(e.Data) == 0 {
		return nil, errors.New(errors.CodeInvalidConfig, "knowledge import 'data' is empty")
	}

	d := &NormalizedPatchData{Game: e.Game, Patch: e.Patch, Source: "manual"}
	for _, rec := range e.Data {
		var key struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		}
		if err := json.Unmarshal(rec, &key); err != nil {
			return nil, errors.New(errors.CodeInvalidConfig, "invalid record in 'data': "+err.Error())
		}
		if key.Name == "" {
			continue
		}
		r := Record{ID: key.ID, Name: key.Name, Data: rec}
		switch e.Kind {
		case "champions":
			d.Champions = append(d.Champions, r)
		case "items":
			d.Items = append(d.Items, r)
		case "traits":
			d.Traits = append(d.Traits, r)
		case "compositions":
			d.Compositions = append(d.Compositions, r)
		default:
			return nil, errors.New(errors.CodeInvalidConfig,
				"unknown kind: "+e.Kind+" (支持 champions|items|traits|compositions)")
		}
	}
	return d, nil
}

// DetectFormat 判断导入内容是手动 envelope 还是 CD 原始文件
func DetectFormat(raw []byte) string {
	var probe struct {
		Kind  string                     `json:"kind"`
		Sets  map[string]json.RawMessage `json:"sets"`
		Items []json.RawMessage          `json:"items"`
	}
	if err := json.Unmarshal(raw, &probe); err != nil {
		return "unknown"
	}
	if probe.Kind != "" {
		return "envelope"
	}
	if len(probe.Sets) > 0 || len(probe.Items) > 0 {
		return "communitydragon"
	}
	return "unknown"
}

func mustMarshal(v interface{}) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		return json.RawMessage("{}")
	}
	return b
}
