package cdparser

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestParseStats(t *testing.T) {
	p := New(Config{BaseDir: "../../../../data/communitydragon"})
	stats, err := p.CountStats()
	if err != nil {
		t.Fatalf("CountStats failed: %v", err)
	}
	t.Logf("Stats: %+v", stats)

	// Verify we got data
	if stats["active_sets"] == 0 {
		t.Errorf("expected at least 1 active set, got 0")
	}
	if stats["champions_total"] == 0 {
		t.Errorf("expected some champions, got 0")
	}
	if stats["traits_total"] == 0 {
		t.Errorf("expected some traits, got 0")
	}
	if stats["items_total"] == 0 {
		t.Errorf("expected some items, got 0")
	}

	t.Logf("Default set: %s", func() string { s, _ := p.GetDefaultSet(); return s }())
}

func TestSetKeyConversion(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"TFTSet18", "set18"},
		{"TFTSet17", "set17"},
		{"TFTSet1", "set1"},
		{"TFTSetTutorial", "settutorial"},
	}
	for _, tt := range tests {
		result := SetKeyFromCDName(tt.input)
		if result != tt.expected {
			t.Errorf("SetKeyFromCDName(%q) = %q, expected %q", tt.input, result, tt.expected)
		}
	}
}

func TestExtractSetFromNameID(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"TFT18_Jinx", "TFTSet18"},
		{"TFT17_Briar", "TFTSet17"},
		{"TFTTutorial_Zed", "TFTTutorial"},
		{"TFT1_TFTKatarina", "TFTSet1"},
	}
	for _, tt := range tests {
		result := ExtractSetFromNameID(tt.input)
		expectedFull := tt.expected
		if result != expectedFull {
			t.Errorf("ExtractSetFromNameID(%q) = %q, expected %q", tt.input, result, expectedFull)
		}
	}
}

// TestParseTraits_BreakpointFields 验证 CDTrait 包含 conditional_trait_sets / innate_trait_sets
// 这是 Phase 16-B 前置：composition.decodeBreakpoints 依赖这两个字段。
//
// 不依赖真实 data/communitydragon/ 目录，使用临时 fixture。
func TestParseTraits_BreakpointFields(t *testing.T) {
	// 构造临时 CD 数据：含一个 conditional trait (3/6/9) + 一个 innate trait
	tmpDir := t.TempDir()
	traitsJSON := `[{
		"trait_id": "TestBrawler",
		"display_name": "TestBrawler",
		"set": "TFTSet18",
		"tooltip_text": "Brawlers gain @BonusHealth@ HP",
		"icon_path": "/fake/path.png",
		"innate_trait_sets": [],
		"conditional_trait_sets": [
			{"constants": [{"name": "BonusHealth", "value": 100}], "min_units": 2, "max_units": 3, "style_idx": 2, "style_name": "kBronze"},
			{"constants": [{"name": "BonusHealth", "value": 200}], "min_units": 4, "max_units": 5, "style_idx": 3, "style_name": "kSilver"},
			{"constants": [{"name": "BonusHealth", "value": 300}], "min_units": 6, "max_units": 25000, "style_idx": 5, "style_name": "kGold"}
		]
	}, {
		"trait_id": "TestInnate",
		"display_name": "TestInnate",
		"set": "TFTSet18",
		"tooltip_text": "Innate passive",
		"icon_path": "/fake/innate.png",
		"innate_trait_sets": [
			{"constants": [], "min_units": 1, "max_units": 25000, "style_idx": 1, "style_name": "kBronze"}
		],
		"conditional_trait_sets": []
	}]`
	if err := os.WriteFile(filepath.Join(tmpDir, "tfttraits.json"), []byte(traitsJSON), 0644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	// augment-lists.json 也需要存在（避免 404 HTML warning）
	if err := os.WriteFile(filepath.Join(tmpDir, "augment-lists.json"), []byte(`[]`), 0644); err != nil {
		t.Fatalf("write augment fixture: %v", err)
	}

	p := New(Config{BaseDir: tmpDir})
	traits, err := p.ParseTraits()
	if err != nil {
		t.Fatalf("ParseTraits: %v", err)
	}
	if len(traits) != 2 {
		t.Fatalf("expected 2 traits, got %d", len(traits))
	}

	// 找 conditional trait
	var brawler *CDTrait
	for i := range traits {
		if traits[i].TraitID == "TestBrawler" {
			brawler = &traits[i]
		}
	}
	if brawler == nil {
		t.Fatalf("TestBrawler not found")
	}
	if len(brawler.ConditionalTraitSets) != 3 {
		t.Errorf("expected 3 conditional breakpoints, got %d", len(brawler.ConditionalTraitSets))
	}
	if brawler.ConditionalTraitSets[0].MinUnits != 2 || brawler.ConditionalTraitSets[0].MaxUnits != 3 {
		t.Errorf("first breakpoint wrong: min=%d max=%d", brawler.ConditionalTraitSets[0].MinUnits, brawler.ConditionalTraitSets[0].MaxUnits)
	}
	if brawler.ConditionalTraitSets[0].StyleName != "kBronze" {
		t.Errorf("first breakpoint style_name = %q, expected kBronze", brawler.ConditionalTraitSets[0].StyleName)
	}
	if len(brawler.ConditionalTraitSets[0].Constants) != 1 || brawler.ConditionalTraitSets[0].Constants[0].Name != "BonusHealth" {
		t.Errorf("first breakpoint constants wrong: %+v", brawler.ConditionalTraitSets[0].Constants)
	}
	if brawler.ConditionalTraitSets[2].MaxUnits != 25000 {
		t.Errorf("last breakpoint max_units = %d, expected 25000 (no cap)", brawler.ConditionalTraitSets[2].MaxUnits)
	}

	// 找 innate trait
	var innate *CDTrait
	for i := range traits {
		if traits[i].TraitID == "TestInnate" {
			innate = &traits[i]
		}
	}
	if innate == nil {
		t.Fatalf("TestInnate not found")
	}
	if len(innate.InnateTraitSets) != 1 {
		t.Errorf("expected 1 innate breakpoint, got %d", len(innate.InnateTraitSets))
	}
	if len(innate.ConditionalTraitSets) != 0 {
		t.Errorf("expected 0 conditional breakpoints, got %d", len(innate.ConditionalTraitSets))
	}
}

// TestParseTraits_RealFile_ContainsBreakpoints 验证真实 tfttraits.json 解析后含 conditional_trait_sets
//
// 跳过条件：data/communitydragon/tfttraits.json 不存在（CI 沙箱）。
func TestParseTraits_RealFile_ContainsBreakpoints(t *testing.T) {
	// 找 data/communitydragon 相对位置（从当前 package 向上 4 级）
	candidates := []string{
		"../../../../data/communitydragon/tfttraits.json",
	}
	var found string
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			found = c
			break
		}
	}
	if found == "" {
		t.Skip("tfttraits.json not found; skipping real-file test")
	}

	p := New(Config{BaseDir: filepath.Dir(found)})
	traits, err := p.ParseTraits()
	if err != nil {
		t.Fatalf("ParseTraits: %v", err)
	}
	// 至少有一个 trait 应该有 breakpoint
	withBreakpoints := 0
	for _, tr := range traits {
		if len(tr.ConditionalTraitSets) > 0 {
			withBreakpoints++
		}
	}
	if withBreakpoints == 0 {
		t.Errorf("expected at least some traits to have conditional_trait_sets; got 0 / %d", len(traits))
	}
	t.Logf("Real tfttraits.json: %d total, %d with breakpoints", len(traits), withBreakpoints)

	// 验证 raw JSON round-trip：CDTraitBreakpoint 字段映射正确
	for _, tr := range traits {
		if len(tr.ConditionalTraitSets) > 0 {
			bp := tr.ConditionalTraitSets[0]
			// 序列化再反序列化，确认字段未丢
			b, err := json.Marshal(bp)
			if err != nil {
				t.Errorf("marshal bp: %v", err)
				continue
			}
			var back CDTraitBreakpoint
			if err := json.Unmarshal(b, &back); err != nil {
				t.Errorf("unmarshal bp: %v", err)
				continue
			}
			if back.MinUnits != bp.MinUnits || back.MaxUnits != bp.MaxUnits || back.StyleName != bp.StyleName {
				t.Errorf("round-trip mismatch: %+v → %+v", bp, back)
			}
			break
		}
	}
}
