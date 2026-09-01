// Package tft - 装备 ID 标准化层测试
//
// 修复 Phase 14-B 后 test mock 与 TFTRepository 接口签名不匹配的问题：
//   - TFTRepository.GetXxx/ListXxx 现在返回 errors.AppError（不再返回 error）
//   - 之前用 error stub 的 mockTFTRepository 改成返回 nil AppError
//   - duplicate TestIsTFTID 移除一个，保留在 id_normalize_test.go

package tft

import (
	"context"
	"testing"

	"github.com/lipipi/aurora/internal/errors"
	"github.com/lipipi/aurora/internal/game/knowledge"
	tftkb "github.com/lipipi/aurora/internal/game/knowledge/tft"
	"github.com/stretchr/testify/assert"
)

// Mock repository 用于测试
//
// 必须实现 tftkb.TFTRepository 接口（所有方法返回 errors.AppError）
type mockTFTRepository struct {
	items  []knowledge.Item
	hasErr bool
}

// ListItems 模拟 ListItems（可选返回错误）
func (m *mockTFTRepository) ListItems(ctx context.Context, patch string) ([]knowledge.Item, errors.AppError) {
	if m.hasErr {
		return nil, nil
	}
	return m.items, nil
}

// 实现其他必需接口方法（stub）
func (m *mockTFTRepository) GetChampion(ctx context.Context, patch, query string) (*knowledge.Champion, errors.AppError) {
	return nil, nil
}
func (m *mockTFTRepository) GetItem(ctx context.Context, patch, query string) (*knowledge.Item, errors.AppError) {
	return nil, nil
}
func (m *mockTFTRepository) GetTrait(ctx context.Context, patch, query string) (*knowledge.Trait, errors.AppError) {
	return nil, nil
}
func (m *mockTFTRepository) GetPatch(ctx context.Context, patch string) (*tftkb.PatchInfo, errors.AppError) {
	return nil, nil
}
func (m *mockTFTRepository) ListChampions(ctx context.Context, patch string) ([]knowledge.Champion, errors.AppError) {
	return nil, nil
}
func (m *mockTFTRepository) ListTraits(ctx context.Context, patch string) ([]knowledge.Trait, errors.AppError) {
	return nil, nil
}
func (m *mockTFTRepository) HasPatch(ctx context.Context, patch string) (bool, errors.AppError) {
	return true, nil
}

func TestItemIDNormalizer_DefaultAliases(t *testing.T) {
	n := NewItemIDNormalizer()

	tests := []struct {
		name     string
		input    string
		wantID   string
		wantKnown bool
	}{
		{"IE exact", "IE", "TFT18_InfinityEdge", true},
		{"infinity edge lowercase", "infinity edge", "TFT18_InfinityEdge", true},
		{"无尽之刃 Chinese", "无尽之刃", "TFT18_InfinityEdge", true},
		{"无尽 short Chinese", "无尽", "TFT18_InfinityEdge", true},
		{"GS giant sword", "GS", "TFT18_GiantSword", true},
		{"大剑 Chinese", "大剑", "TFT18_GiantSword", true},
		{"Bloodthirster", "Bloodthirster", "TFT18_Bloodthirster", true},
		{"饮血 Chinese", "饮血", "TFT18_Bloodthirster", true},
		{"Rageblade", "Rageblade", "TFT18_Rageblade", true},
		{"羊刀 Chinese", "羊刀", "TFT18_Rageblade", true},
		{"JG jeweled gauntlet", "JG", "TFT18_JeweledGauntlet", true},
		{"珠光 Chinese", "珠光", "TFT18_JeweledGauntlet", true},
		{"unknown item", "NonexistentItem", UnknownID, false},
		{"empty string", "", UnknownID, false},
		{"whitespace only", "   ", UnknownID, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := n.Normalize(tt.input)
			if tt.wantKnown {
				assert.Equal(t, tt.wantID, result)
			} else {
				assert.Equal(t, UnknownID, result)
			}
		})
	}
}

func TestItemIDNormalizer_AlreadyTFTID(t *testing.T) {
	n := NewItemIDNormalizer() // default setKey = TFT18

	tests := []struct {
		name     string
		input    string
		want     string
	}{
		{"correct prefix", "TFT18_InfinityEdge", "TFT18_InfinityEdge"},
		{"wrong prefix (set 17)", "TFT17_InfinityEdge", UnknownID},
		{"invalid format", "InfinityEdge", UnknownID},
		{"no underscore", "TFT18InfinityEdge", UnknownID},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := n.Normalize(tt.input)
			assert.Equal(t, tt.want, result)
		})
	}
}

func TestItemIDNormalizer_WithSetKey(t *testing.T) {
	n := NewItemIDNormalizer().WithSetKey("TFT17")

	result := n.Normalize("TFT17_InfinityEdge")
	assert.Equal(t, "TFT17_InfinityEdge", result)

	// TFT18 should now fail
	result2 := n.Normalize("TFT18_InfinityEdge")
	assert.Equal(t, UnknownID, result2)
}

func TestItemIDNormalizer_RegisterAlias(t *testing.T) {
	n := NewItemIDNormalizer()
	n.RegisterAlias("myitem", "TFT18_MyCustomItem")

	result := n.Normalize("myitem")
	assert.Equal(t, "TFT18_MyCustomItem", result)

	// lowercase should also work
	result = n.Normalize("MYITEM")
	assert.Equal(t, "TFT18_MyCustomItem", result)
}

func TestItemIDNormalizer_ResolveWithPatch(t *testing.T) {
	mockRepo := &mockTFTRepository{
		items: []knowledge.Item{
			{Patch: "16.17.1", ID: "TFT18_InfinityEdge", Name: "Infinity Edge"},
			{Patch: "16.17.1", ID: "TFT18_GiantSword", Name: "Giant Sword"},
			{Patch: "16.17.1", ID: "TFT18_Bloodthirster", Name: "Bloodthirster"},
		},
	}

	n := NewItemIDNormalizer().WithRepository(mockRepo)

	tests := []struct {
		name      string
		fullPatch string
		input     string
		want      string
	}{
		{"exact match by name", "16.17.1", "Infinity Edge", "TFT18_InfinityEdge"},
		{"partial match inf", "16.17.1", "inf", "TFT18_InfinityEdge"},
		{"partial match gia", "16.17.1", "gia", "TFT18_GiantSword"},
		{"local alias still works", "16.17.1", "无尽", "TFT18_InfinityEdge"},
		{"already TFT ID correct set", "16.17.1", "TFT18_InfinityEdge", "TFT18_InfinityEdge"},
		{"already TFT ID wrong set", "16.17.1", "TFT17_InfinityEdge", UnknownID},
		{"unknown item", "16.17.1", "Nonexistent", UnknownID},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := n.ResolveWithPatch(context.Background(), tt.fullPatch, tt.input)
			assert.Equal(t, tt.want, result)
		})
	}
}

func TestItemIDNormalizer_ResolveWithPatch_MultipleCandidates(t *testing.T) {
	mockRepo := &mockTFTRepository{
		items: []knowledge.Item{
			{Patch: "16.17.1", ID: "TFT18_ItemOne", Name: "Something Sword"},
			{Patch: "16.17.1", ID: "TFT18_ItemTwo", Name: "Another Sword"},
		},
	}

	n := NewItemIDNormalizer().WithRepository(mockRepo)
	result := n.ResolveWithPatch(context.Background(), "16.17.1", "Sword")

	// Multiple candidates → return Unknown (don't guess)
	assert.Equal(t, UnknownID, result)
}

func TestItemIDNormalizer_ResolveWithPatch_WrongSet(t *testing.T) {
	mockRepo := &mockTFTRepository{
		items: []knowledge.Item{
			{Patch: "16.17.1", ID: "TFT17_InfinityEdge", Name: "Infinity Edge"},
		},
	}

	n := NewItemIDNormalizer() // default setKey = TFT18
	n = n.WithRepository(mockRepo)

	result := n.ResolveWithPatch(context.Background(), "16.17.1", "Infinity Edge")
	// Found but wrong set prefix → Unknown
	assert.Equal(t, UnknownID, result)
}

func TestItemIDNormalizer_ResolveWithPatch_NilRepo(t *testing.T) {
	n := NewItemIDNormalizer() // no repo

	// Falls back to Normalize → still works for aliases
	result := n.ResolveWithPatch(context.Background(), "16.17.1", "无尽")
	assert.Equal(t, "TFT18_InfinityEdge", result)

	// Unknown still unknown
	result2 := n.ResolveWithPatch(context.Background(), "16.17.1", "UnknownThing")
	assert.Equal(t, UnknownID, result2)
}

func TestCanonicalizeTFTID(t *testing.T) {
	tests := []struct {
		id         string
		currentSet string
		want       string
	}{
		{"TFT18_InfinityEdge", "TFT18", "TFT18_InfinityEdge"},
		{"tft18_infinityedge", "TFT18", "TFT18_Infinityedge"}, // case-insensitive prefix match
		{"TFT17_InfinityEdge", "TFT18", UnknownID}, // wrong set
		{"TFT_Infinity", "TFT18", UnknownID},        // no underscore after prefix
	}

	for _, tt := range tests {
		t.Run(tt.id, func(t *testing.T) {
			result := canonicalizeTFTID(tt.id, tt.currentSet)
			assert.Equal(t, tt.want, result)
		})
	}
}
