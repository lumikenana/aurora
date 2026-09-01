package tft

import (
	"context"
	"testing"

	"github.com/lipipi/aurora/internal/errors"
	"github.com/lipipi/aurora/internal/game/knowledge"
	tftkb "github.com/lipipi/aurora/internal/game/knowledge/tft"
	"github.com/stretchr/testify/assert"
)

// MockTFTRepository 用于测试的 mock repository
type MockTFTRepository struct {
	champions []knowledge.Champion
}

func (m *MockTFTRepository) GetChampion(ctx context.Context, patch, query string) (*knowledge.Champion, errors.AppError) {
	return nil, nil
}

func (m *MockTFTRepository) GetItem(ctx context.Context, patch, query string) (*knowledge.Item, errors.AppError) {
	return nil, nil
}

func (m *MockTFTRepository) GetTrait(ctx context.Context, patch, query string) (*knowledge.Trait, errors.AppError) {
	return nil, nil
}

func (m *MockTFTRepository) GetAugment(ctx context.Context, patch, query string) (*knowledge.Augment, errors.AppError) {
	return nil, nil
}

func (m *MockTFTRepository) GetPatch(ctx context.Context, patch string) (*tftkb.PatchInfo, errors.AppError) {
	return nil, nil
}

func (m *MockTFTRepository) ListChampions(ctx context.Context, patch string) ([]knowledge.Champion, errors.AppError) {
	return m.champions, nil
}

func (m *MockTFTRepository) ListItems(ctx context.Context, patch string) ([]knowledge.Item, errors.AppError) {
	return nil, nil
}

func (m *MockTFTRepository) ListTraits(ctx context.Context, patch string) ([]knowledge.Trait, errors.AppError) {
	return nil, nil
}

func (m *MockTFTRepository) HasPatch(ctx context.Context, patch string) (bool, errors.AppError) {
	return true, nil
}

// Tests for existing cases are already in id_normalize_test.go
// This file adds KB repository integration tests only

func TestChampionIDNormalizer_WithRepository(t *testing.T) {
	// Setup mock with one champion
	mock := &MockTFTRepository{
		champions: []knowledge.Champion{
			{
				Patch: "16.17.1",
				ID:   "TFT18_Vayne",
				Name: "Vayne",
			},
		},
	}

	n := NewChampionIDNormalizer()
	n.WithRepository(mock)

	// With patch "TFT18" and fullPatch "16.17.1"
	ctx := context.Background()
	id := n.ResolveWithPatch(ctx, "16.17.1", "Vayne")
	assert.Equal(t, "TFT18_Vayne", id)
}

func TestChampionIDNormalizer_WithRepository_Chinese(t *testing.T) {
	mock := &MockTFTRepository{
		champions: []knowledge.Champion{
			{
				Patch: "16.17.1",
				ID:   "TFT18_Vayne",
				Name: "薇恩",
			},
		},
	}

	n := NewChampionIDNormalizer()
	n.WithRepository(mock)

	ctx := context.Background()
	id := n.ResolveWithPatch(ctx, "16.17.1", "薇恩")
	assert.Equal(t, "TFT18_Vayne", id)
}

func TestChampionIDNormalizer_WithRepository_WrongSet(t *testing.T) {
	mock := &MockTFTRepository{
		champions: []knowledge.Champion{
			{
				Patch: "16.17.1",
				ID:   "TFT17_Vayne",
				Name: "Vayne",
			},
		},
	}

	n := NewChampionIDNormalizer() // default patch = TFT18
	n.WithRepository(mock)

	ctx := context.Background()
	id := n.ResolveWithPatch(ctx, "16.17.1", "Vayne")
	// Even though KB found it, prefix doesn't match current patch → Unknown
	assert.Equal(t, UnknownID, id)
}

func TestChampionIDNormalizer_WithRepository_MultipleCandidates(t *testing.T) {
	mock := &MockTFTRepository{
		champions: []knowledge.Champion{
			{
				Patch: "16.17.1",
				ID:   "TFT18_Ahri",
				Name: "Ahri",
			},
			{
				Patch: "16.17.1",
				ID:   "TFT18_Aatrox",
				Name: "Aatrox",
			},
		},
	}

	n := NewChampionIDNormalizer()
	n.WithRepository(mock)

	ctx := context.Background()
	id := n.ResolveWithPatch(ctx, "16.17.1", "a")
	// "a" matches both Ahri and Aatrox → multiple candidates → Unknown
	assert.Equal(t, UnknownID, id)
}

func TestChampionIDNormalizer_WithRepository_NoMatch(t *testing.T) {
	mock := &MockTFTRepository{
		champions: []knowledge.Champion{
			{
				Patch: "16.17.1",
				ID:   "TFT18_Vayne",
				Name: "Vayne",
			},
		},
	}

	n := NewChampionIDNormalizer()
	n.WithRepository(mock)

	ctx := context.Background()
	id := n.ResolveWithPatch(ctx, "16.17.1", "NonExistent")
	assert.Equal(t, UnknownID, id)
}

func TestChampionIDNormalizer_CaseInsensitive(t *testing.T) {
	n := NewChampionIDNormalizer()
	n.RegisterAlias("vayne", "TFT18_Vayne")

	id := n.Normalize("VAYNE")
	assert.Equal(t, "TFT18_Vayne", id)

	id = n.Normalize("vayne")
	assert.Equal(t, "TFT18_Vayne", id)

	id = n.Normalize("Vayne")
	assert.Equal(t, "TFT18_Vayne", id)
}

func TestChampionIDNormalizer_Whitespace(t *testing.T) {
	n := NewChampionIDNormalizer()
	n.RegisterAlias("vayne", "TFT18_Vayne")

	id := n.Normalize("  Vayne  ")
	assert.Equal(t, "TFT18_Vayne", id)
}

func TestBoardSlot_HasChampionID(t *testing.T) {
	slot := BoardSlot{
		ChampionID: "TFT18_Vayne",
		Star:       1,
		Position:   0,
	}
	// Verify structure matches expected
	assert.Equal(t, "TFT18_Vayne", slot.ChampionID)
	assert.Equal(t, 1, slot.Star)
	assert.Equal(t, 0, slot.Position)
	// Items field optional, default nil
	assert.Nil(t, slot.Items)
}
