package tft

import (
	"image"
	"testing"
)

func TestScaleRegion_Identity(t *testing.T) {
	// 1920x1080 → 1920x1080：缩放因子 = 1
	r := image.Rect(100, 200, 300, 400)
	scaled := ScaleRegion(r, image.Pt(1920, 1080), image.Pt(1920, 1080))
	if scaled != r {
		t.Fatalf("Identity scale mismatch: got %+v, want %+v", scaled, r)
	}
}

func TestScaleRegion_Downscale(t *testing.T) {
	// 1920x1080 → 1280x720：缩放因子 2/3
	r := image.Rect(60, 30, 180, 60) // 在 1920x1080 下的某个区域
	scaled := ScaleRegion(r, image.Pt(1920, 1080), image.Pt(1280, 720))
	// 期望：每个坐标 × 2/3
	want := image.Rect(40, 20, 120, 40)
	if scaled != want {
		t.Fatalf("Downscale mismatch: got %+v, want %+v", scaled, want)
	}
}

func TestScaleRegion_Upscale(t *testing.T) {
	// 1920x1080 → 2560x1440：缩放因子 4/3
	r := image.Rect(60, 30, 180, 60)
	scaled := ScaleRegion(r, image.Pt(1920, 1080), image.Pt(2560, 1440))
	// 期望：每个坐标 × 4/3
	want := image.Rect(80, 40, 240, 80)
	if scaled != want {
		t.Fatalf("Upscale mismatch: got %+v, want %+v", scaled, want)
	}
}

func TestScaleRegion_NonSquareAspect(t *testing.T) {
	// 1920x1080 → 1920x540（非等比，X 不变，Y × 0.5）
	r := image.Rect(100, 200, 300, 400)
	scaled := ScaleRegion(r, image.Pt(1920, 1080), image.Pt(1920, 540))
	want := image.Rect(100, 100, 300, 200)
	if scaled != want {
		t.Fatalf("Non-square scale mismatch: got %+v, want %+v", scaled, want)
	}
}

func TestScaleRegion_ZeroDims(t *testing.T) {
	r := image.Rect(100, 200, 300, 400)
	// 零维度 → 返回原区域（不缩放）
	if got := ScaleRegion(r, image.Pt(0, 1080), image.Pt(1920, 1080)); got != r {
		t.Fatalf("Zero ref width should return original")
	}
	if got := ScaleRegion(r, image.Pt(1920, 1080), image.Pt(0, 720)); got != r {
		t.Fatalf("Zero actual width should return original")
	}
}

func TestScaleRegions_AllFields(t *testing.T) {
	regions := DefaultTFTScreenRegions
	actual := image.Pt(1280, 720)
	scaled := ScaleRegions(regions, ReferenceResolution, actual)
	// 验证 Phase 10-7 区域
	if scaled.Gold.Empty() {
		t.Fatal("Scaled Gold region is empty")
	}
	if scaled.HP.Empty() {
		t.Fatal("Scaled HP region is empty")
	}
	if scaled.Level.Empty() {
		t.Fatal("Scaled Level region is empty")
	}
	if scaled.Stage.Empty() {
		t.Fatal("Scaled Stage region is empty")
	}
	// Phase 10-8 新增区域
	if scaled.XP.Empty() {
		t.Fatal("Scaled XP region is empty")
	}
	if scaled.Items.Empty() {
		t.Fatal("Scaled Items region is empty")
	}
	if scaled.Traits.Empty() {
		t.Fatal("Scaled Traits region is empty")
	}
	// 缩放后字段长度的合理性
	if len(scaled.Board) != BoardSize {
		t.Fatalf("Scaled Board length = %d, want %d", len(scaled.Board), BoardSize)
	}
	if len(scaled.Bench) != BenchSize {
		t.Fatalf("Scaled Bench length = %d, want %d", len(scaled.Bench), BenchSize)
	}
	if len(scaled.Shop) != ShopSize {
		t.Fatalf("Scaled Shop length = %d, want %d", len(scaled.Shop), ShopSize)
	}
	if len(scaled.Augments) != AugmentSlotCount {
		t.Fatalf("Scaled Augments length = %d, want %d", len(scaled.Augments), AugmentSlotCount)
	}
	// Carousel 长度保持默认（≥ 1）
	if len(scaled.Carousel) == 0 {
		t.Fatal("Scaled Carousel is empty")
	}
}

// TestDefaultTFTScreenRegions_AllFields 验证默认区域覆盖所有 Phase 10-8 字段
func TestDefaultTFTScreenRegions_AllFields(t *testing.T) {
	r := DefaultTFTScreenRegions
	// Phase 10-7
	if r.Gold.Empty() {
		t.Fatal("Default Gold is empty")
	}
	if r.HP.Empty() {
		t.Fatal("Default HP is empty")
	}
	if r.Level.Empty() {
		t.Fatal("Default Level is empty")
	}
	if r.Stage.Empty() {
		t.Fatal("Default Stage is empty")
	}
	// Phase 10-8
	if r.XP.Empty() {
		t.Fatal("Default XP is empty")
	}
	if len(r.Board) != BoardSize {
		t.Fatalf("Default Board length = %d, want %d", len(r.Board), BoardSize)
	}
	if len(r.Bench) != BenchSize {
		t.Fatalf("Default Bench length = %d, want %d", len(r.Bench), BenchSize)
	}
	if len(r.Shop) != ShopSize {
		t.Fatalf("Default Shop length = %d, want %d", len(r.Shop), ShopSize)
	}
	if r.Items.Empty() {
		t.Fatal("Default Items is empty")
	}
	if r.Traits.Empty() {
		t.Fatal("Default Traits is empty")
	}
	if len(r.Augments) != AugmentSlotCount {
		t.Fatalf("Default Augments length = %d, want %d", len(r.Augments), AugmentSlotCount)
	}
	if len(r.Carousel) == 0 {
		t.Fatal("Default Carousel is empty")
	}
	// Board Position 0~27 单调递增
	for i, c := range r.Board {
		if c.Position != i {
			t.Errorf("Board[%d].Position = %d, want %d", i, c.Position, i)
		}
	}
	// Bench Position 0~8 单调递增
	for i, c := range r.Bench {
		if c.Position != i {
			t.Errorf("Bench[%d].Position = %d, want %d", i, c.Position, i)
		}
	}
	// Shop Position 0~4 单调递增
	for i, c := range r.Shop {
		if c.Position != i {
			t.Errorf("Shop[%d].Position = %d, want %d", i, c.Position, i)
		}
	}
	// Augments Position 0~2 单调递增
	for i, c := range r.Augments {
		if c.Position != i {
			t.Errorf("Augments[%d].Position = %d, want %d", i, c.Position, i)
		}
	}
}

// TestScaleCells_Basic 测试 ScaleCells 按比例缩放并保留 Position
func TestScaleCells_Basic(t *testing.T) {
	cells := []CellRegion{
		{Position: 0, Rect: image.Rect(100, 100, 200, 200)},
		{Position: 1, Rect: image.Rect(200, 100, 300, 200)},
	}
	// 1920x1080 → 1280x720 (2/3 缩放)
	scaled := ScaleCells(cells, image.Pt(1920, 1080), image.Pt(1280, 720))
	if len(scaled) != 2 {
		t.Fatalf("Scaled length = %d, want 2", len(scaled))
	}
	if scaled[0].Position != 0 {
		t.Errorf("scaled[0].Position = %d, want 0", scaled[0].Position)
	}
	want0 := image.Rect(66, 66, 133, 133) // × 2/3
	if scaled[0].Rect != want0 {
		t.Errorf("scaled[0].Rect = %+v, want %+v", scaled[0].Rect, want0)
	}
	want1 := image.Rect(133, 66, 200, 133)
	if scaled[1].Rect != want1 {
		t.Errorf("scaled[1].Rect = %+v, want %+v", scaled[1].Rect, want1)
	}
}

// TestScaleCells_Empty 测试空 slice 和零维度安全降级
func TestScaleCells_Empty(t *testing.T) {
	// 空 slice 直接返回
	got := ScaleCells(nil, image.Pt(1920, 1080), image.Pt(1280, 720))
	if got != nil {
		t.Errorf("nil input should return nil, got %v", got)
	}

	cells := []CellRegion{{Position: 0, Rect: image.Rect(0, 0, 100, 100)}}
	// 零维度 → 返回原 slice
	if got := ScaleCells(cells, image.Pt(0, 1080), image.Pt(1920, 1080)); len(got) != 1 || got[0].Rect != cells[0].Rect {
		t.Errorf("Zero ref dim should return original, got %+v", got)
	}
}

// TestScaleRegions_NonSquareAspect 验证非等比缩放下所有字段都正确处理
func TestScaleRegions_NonSquareAspect(t *testing.T) {
	regions := DefaultTFTScreenRegions
	// 非等比缩放：X 1920 不变，Y 540（Y × 0.5）
	actual := image.Pt(1920, 540)
	scaled := ScaleRegions(regions, ReferenceResolution, actual)
	// Gold 区域在原 1920x1080 是 (155,1020,210,1060)
	// 缩放后 Y × 0.5：(155, 510, 210, 530)
	wantGold := image.Rect(155, 510, 210, 530)
	if scaled.Gold != wantGold {
		t.Errorf("Non-square Gold scale mismatch: got %+v, want %+v", scaled.Gold, wantGold)
	}
	// Board 第一个格子也按 Y × 0.5 缩放
	if len(scaled.Board) == BoardSize {
		first := scaled.Board[0]
		if first.Rect.Min.Y != 240 { // 480 × 0.5
			t.Errorf("Board[0].Min.Y = %d, want 240", first.Rect.Min.Y)
		}
	}
}
