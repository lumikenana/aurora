// Package ddragon Riot Data Dragon TFT 数据解析器
//
// 解析 Riot Data Dragon 提供的四个独立 JSON 文件：
//  - tft-champion.json
//  - tft-trait.json
//  - tft-item.json
//  - tft-augments.json
package ddragon

import (
	"encoding/json"
	"fmt"

	"github.com/lipipi/aurora/internal/errors"
	"github.com/lipipi/aurora/internal/game/knowledge"
)

// ddChampion Data Dragon 英雄结构
type ddChampion struct {
	ID    string      `json:"id"`    // API ID: "TFTTutorial_Zed"
	Name  string      `json:"name"`  // 中文名: "劫"
	Tier  int         `json:"tier"`  // 品质等级
	Cost  int         `json:"cost"`  // 费用
	Image ddImageInfo `json:"image"` // 图片信息
}

// ddTrait Data Dragon 羁绊结构
type ddTrait struct {
	ID    string      `json:"id"`
	Name  string      `json:"name"`
	Image ddImageInfo `json:"image"`
}

// ddItem Data Dragon 装备结构
type ddItem struct {
	ID    string      `json:"id"`
	Name  string      `json:"name"`
	Image ddImageInfo `json:"image"`
}

// ddAugment Data Dragon 强化符文结构
type ddAugment struct {
	ID          string      `json:"id"`
	Name        string      `json:"name"`
	Description string      `json:"description"`
	Image       ddImageInfo `json:"image"`
}

// ddImageInfo 图片信息
type ddImageInfo struct {
	Full string `json:"full"` // 文件名: "TFTTutorial_Zed.png"
}

// ddPayload 顶级结构
type ddPayload struct {
	Data map[string]json.RawMessage `json:"data"`
}

// ParseDDChampions 解析 tft-champion.json 原始数据
func ParseDDChampions(raw []byte) ([]knowledge.Record, error) {
	var payload ddPayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, fmt.Errorf("parse tft-champion.json: %w", err)
	}

	var records []knowledge.Record
	for _, rawEntry := range payload.Data {
		var c ddChampion
		if err := json.Unmarshal(rawEntry, &c); err != nil {
			continue // skip malformed entry
		}
		if c.Name == "" || c.ID == "" {
			continue
		}
		// 保留完整原始数据到 Data 字段
		records = append(records, knowledge.Record{
			ID:   c.ID,
			Name: c.Name,
			Data: rawEntry,
		})
	}
	return records, nil
}

// ParseDDTraits 解析 tft-trait.json 原始数据
func ParseDDTraits(raw []byte) ([]knowledge.Record, error) {
	var payload ddPayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, fmt.Errorf("parse tft-trait.json: %w", err)
	}

	var records []knowledge.Record
	for _, rawEntry := range payload.Data {
		var t ddTrait
		if err := json.Unmarshal(rawEntry, &t); err != nil {
			continue
		}
		if t.Name == "" || t.ID == "" {
			continue
		}
		records = append(records, knowledge.Record{
			ID:   t.ID,
			Name: t.Name,
			Data: rawEntry,
		})
	}
	return records, nil
}

// ParseDDItems 解析 tft-item.json 原始数据
func ParseDDItems(raw []byte) ([]knowledge.Record, error) {
	var payload ddPayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, fmt.Errorf("parse tft-item.json: %w", err)
	}

	var records []knowledge.Record
	for _, rawEntry := range payload.Data {
		var it ddItem
		if err := json.Unmarshal(rawEntry, &it); err != nil {
			continue
		}
		if it.Name == "" || it.ID == "" {
			continue
		}
		records = append(records, knowledge.Record{
			ID:   it.ID,
			Name: it.Name,
			Data: rawEntry,
		})
	}
	return records, nil
}

// ParseDDAugments 解析 tft-augments.json 原始数据
func ParseDDAugments(raw []byte) ([]knowledge.Record, error) {
	var payload ddPayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, fmt.Errorf("parse tft-augments.json: %w", err)
	}

	var records []knowledge.Record
	for _, rawEntry := range payload.Data {
		var a ddAugment
		if err := json.Unmarshal(rawEntry, &a); err != nil {
			continue
		}
		if a.Name == "" || a.ID == "" {
			continue
		}
		records = append(records, knowledge.Record{
			ID:   a.ID,
			Name: a.Name,
			Data: rawEntry,
		})
	}
	return records, nil
}

// ParseAll 从四个原始 JSON 字节创建归一化数据
func ParseAll(
	championsRaw []byte,
	traitsRaw []byte,
	itemsRaw []byte,
	augmentsRaw []byte,
	patch string,
	setKey string,
	mode string,
) (*knowledge.NormalizedPatchData, error) {

	if setKey == "" {
		setKey = "unknown"
	}
	if mode == "" {
		mode = "standard"
	}

	d := &knowledge.NormalizedPatchData{
		Game:    "tft",
		SetKey:  setKey,
		Patch:   patch,
		Mode:    mode,
		Source:  "datadragon",
	}

	var err error

	if len(championsRaw) > 0 {
		d.Champions, err = ParseDDChampions(championsRaw)
		if err != nil {
			return nil, errors.Wrap(errors.CodeInvalidConfig, "parse champions", err)
		}
	}

	if len(traitsRaw) > 0 {
		d.Traits, err = ParseDDTraits(traitsRaw)
		if err != nil {
			return nil, errors.Wrap(errors.CodeInvalidConfig, "parse traits", err)
		}
	}

	if len(itemsRaw) > 0 {
		d.Items, err = ParseDDItems(itemsRaw)
		if err != nil {
			return nil, errors.Wrap(errors.CodeInvalidConfig, "parse items", err)
		}
	}

	if len(augmentsRaw) > 0 {
		// Augments need special handling because they were added after the original NormalizedPatchData
		// We'll add them to the normalized data via a new field or append to existing records
		// For now, we need to add Augments to NormalizedPatchData
		// Let's check if the type already has an Augments field by looking at the go:
		// Actually, since we control the code, let's add it properly

		// NOTE: The current knowledge.NormalizedPatchData only has:
		// Champions, Items, Traits, Compositions
		// We need to add Augments field

		// Since this file is in the same module, we can add the field there
		// Let me check... actually I already added Augments to ImportSummary, but not to NormalizedPatchData
		// Wait, looking back: In knowledge.go, I only added Augments to ImportSummary, not to NormalizedPatchData
		// Let me fix that now...

		// Actually, let's just add it here - since we need to pass augments through to store,
		// the insertRecords already handles "augments" table specially, but NormalizedPatchData
		// needs the field to store them. So let me add it to knowledge.go.

		d.Augments, err = ParseDDAugments(augmentsRaw)
		if err != nil {
			return nil, errors.Wrap(errors.CodeInvalidConfig, "parse augments", err)
		}
	}

	return d, nil
}
