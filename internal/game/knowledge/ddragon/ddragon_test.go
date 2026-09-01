package ddragon

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseDDChampion_Sample(t *testing.T) {
	// Sample from actual Data Dragon
	sample := []byte(`{
		"data": {
			"TFTTutorial_Zed": {
				"id": "TFTTutorial_Zed",
				"name": "劫",
				"tier": 2,
				"cost": 0,
				"image": {
					"full": "TFTTutorial_Zed.png",
					"sprite": "tft-champion0.png",
					"group": "tft-champion",
					"x": 0,
					"y": 0,
					"w": 48,
					"h": 48
				}
			}
		}
	}`)

	records, err := ParseDDChampions(sample)
	require.NoError(t, err)
	require.Len(t, records, 1)

	rec := records[0]
	assert.Equal(t, "TFTTutorial_Zed", rec.ID)
	assert.Equal(t, "劫", rec.Name)
	assert.NotEmpty(t, rec.Data)

	// Verify the original JSON is preserved
	var parsed ddChampion
	err = json.Unmarshal(rec.Data, &parsed)
	require.NoError(t, err)
	assert.Equal(t, "TFTTutorial_Zed", parsed.ID)
	assert.Equal(t, "劫", parsed.Name)
	assert.Equal(t, 0, parsed.Cost)
	assert.Equal(t, 2, parsed.Tier)
	assert.Equal(t, "TFTTutorial_Zed.png", parsed.Image.Full)
}

func TestParseDDChampion_EmptyNameSkipped(t *testing.T) {
	sample := []byte(`{
		"data": {
			"TFTTutorial_Zed": {
				"id": "TFTTutorial_Zed",
				"name": "",
				"cost": 1
			}
		}
	}`)

	records, err := ParseDDChampions(sample)
	require.NoError(t, err)
	assert.Len(t, records, 0)
}

func TestParseDDTrait_Sample(t *testing.T) {
	sample := []byte(`{
		"data": {
			"TFTTutorial_Assassin": {
				"id": "TFTTutorial_Assassin",
				"name": "刺客",
				"image": {
					"full": "Trait_Icon_Assassin.png",
					"sprite": "tft-trait0.png",
					"group": "tft-trait",
					"x": 0,
					"y": 0,
					"w": 48,
					"h": 48
				}
			}
		}
	}`)

	records, err := ParseDDTraits(sample)
	require.NoError(t, err)
	require.Len(t, records, 1)

	rec := records[0]
	assert.Equal(t, "TFTTutorial_Assassin", rec.ID)
	assert.Equal(t, "刺客", rec.Name)
	assert.NotEmpty(t, rec.Data)
}

func TestParseDDItem_Sample(t *testing.T) {
	sample := []byte(`{
		"data": {
			"TFTTutorial_Item_NeedlesslyLargeRod": {
				"id": "TFTTutorial_Item_NeedlesslyLargeRod",
				"name": "无用大棒",
				"image": {
					"full": "TFTTutorial_Item_NeedlesslyLargeRod.png",
					"sprite": "tft-item0.png",
					"group": "tft-item",
					"x": 0,
					"y": 0,
					"w": 48,
					"h": 48
				}
			}
		}
	}`)

	records, err := ParseDDItems(sample)
	require.NoError(t, err)
	require.Len(t, records, 1)

	rec := records[0]
	assert.Equal(t, "TFTTutorial_Item_NeedlesslyLargeRod", rec.ID)
	assert.Equal(t, "无用大棒", rec.Name)
	assert.NotEmpty(t, rec.Data)
}

func TestParseDDAugment_Sample(t *testing.T) {
	sample := []byte(`{
		"data": {
			"TFTTutorial_Augment_BackRowAS1": {
				"id": "TFTTutorial_Augment_BackRowAS1",
				"name": "迅捷之击",
				"description": "战斗开始时：你后2排的弈子们获得20攻击距离。",
				"image": {
					"full": "Tutorial-Swift-Strike-I.png",
					"sprite": "tft-augment0.png",
					"group": "tft-augment",
					"x": 48,
					"y": 0,
					"w": 48,
					"h": 48
				}
			}
		}
	}`)

	records, err := ParseDDAugments(sample)
	require.NoError(t, err)
	require.Len(t, records, 1)

	rec := records[0]
	assert.Equal(t, "TFTTutorial_Augment_BackRowAS1", rec.ID)
	assert.Equal(t, "迅捷之击", rec.Name)
	assert.NotEmpty(t, rec.Data)

	var parsed ddAugment
	err = json.Unmarshal(rec.Data, &parsed)
	require.NoError(t, err)
	assert.Equal(t, "战斗开始时：你后2排的弈子们获得20攻击距离。", parsed.Description)
}

func TestParseAll_AllTypes(t *testing.T) {
	championsRaw := []byte(`{"data": {"TFT16_Zed": {"id": "TFT16_Zed", "name": "劫", "cost": 4}}}`)
	traitsRaw := []byte(`{"data": {"TFT16_Assassin": {"id": "TFT16_Assassin", "name": "刺客"}}}`)
	itemsRaw := []byte(`{"data": {"TFT_Item_Bow": {"id": "TFT_Item_Bow", "name": "反曲之弓"}}}`)
	augmentsRaw := []byte(`{"data": {"Augment1": {"id": "Augment1", "name": "TestAugment", "description": "desc"}}}`)

	result, err := ParseAll(championsRaw, traitsRaw, itemsRaw, augmentsRaw, "16.17.1", "set13", "standard")
	require.NoError(t, err)

	assert.Equal(t, "tft", result.Game)
	assert.Equal(t, "set13", result.SetKey)
	assert.Equal(t, "16.17.1", result.Patch)
	assert.Equal(t, "standard", result.Mode)
	assert.Equal(t, "datadragon", result.Source)

	assert.Len(t, result.Champions, 1)
	assert.Len(t, result.Traits, 1)
	assert.Len(t, result.Items, 1)
	assert.Len(t, result.Augments, 1)
}

func TestParseAll_EmptyAllowed(t *testing.T) {
	// Only champions, others empty
	result, err := ParseAll(
		[]byte(`{"data": {"TFT16_Zed": {"id": "TFT16_Zed", "name": "劫"}}}`),
		[]byte(`{"data": {}}`),
		[]byte(`{"data": {}}`),
		[]byte(`{"data": {}}`),
		"16.17.1",
		"set13",
		"standard",
	)
	require.NoError(t, err)

	assert.Len(t, result.Champions, 1) // champion is valid (id and name present)
}
