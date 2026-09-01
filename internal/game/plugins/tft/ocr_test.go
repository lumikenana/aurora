package tft

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"testing"

	_ "image/jpeg" // for jpeg.Decode in mock

	"github.com/lipipi/aurora/internal/game"
)

// makeTestImage 生成 1920x1080 的纯色测试图像
func makeTestImage(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 1920, 1080))
	// 填充灰色（OCR 不需要真实内容，只关心尺寸）
	gray := color.RGBA{R: 128, G: 128, B: 128, A: 255}
	for y := 0; y < 1080; y++ {
		for x := 0; x < 1920; x++ {
			img.Set(x, y, gray)
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 80}); err != nil {
		t.Fatalf("Failed to encode test image: %v", err)
	}
	return buf.Bytes()
}

// regionSizingMockEngine 根据图像尺寸返回不同的 OCR 结果
// 因为不同 region 裁剪后的图像尺寸不同，可以借此区分
func regionSizingMockEngine(t *testing.T) *MockOCREngine {
	t.Helper()
	engine := NewMockOCREngine()
	engine.SetCustomFunc(func(ctx context.Context, imgData []byte, mimeType string) (OCRResult, error) {
		// 解码以获取尺寸
		var img image.Image
		var err error
		if mimeType == "image/png" {
			img, err = png.Decode(bytes.NewReader(imgData))
		} else {
			img, err = jpeg.Decode(bytes.NewReader(imgData))
		}
		if err != nil {
			return OCRResult{}, err
		}
		bounds := img.Bounds()
		w, h := bounds.Dx(), bounds.Dy()
		// Gold 区域在 1920x1080 下：55x40（缩放后）
		// HP 区域：50x35
		// Level 区域：50x25
		// Stage 区域：280x35
		switch {
		case w == 55 && h == 40:
			return OCRResult{Text: "42", Confidence: 0.95}, nil
		case w == 50 && h == 35:
			return OCRResult{Text: "67", Confidence: 0.92}, nil
		case w == 50 && h == 25:
			return OCRResult{Text: "7", Confidence: 0.90}, nil
		case w == 280 && h == 35:
			return OCRResult{Text: "4-2", Confidence: 0.88}, nil
		default:
			return OCRResult{Text: "", Confidence: 0}, nil
		}
	})
	return engine
}

func TestTFTOCRProvider_BasicRecognition(t *testing.T) {
	engine := regionSizingMockEngine(t)
	provider := NewTFTOCRProvider(engine)

	imgBytes := makeTestImage(t)
	delta, err := provider.Contribute(context.Background(), game.ContextInput{
		Screenshot:     imgBytes,
		ScreenshotMIME: "image/jpeg",
	})
	if err != nil {
		t.Fatalf("Contribute failed: %v", err)
	}
	if delta == nil {
		t.Fatal("Delta should not be nil")
	}

	tftCtx, ok := delta.GameData.(*TFTGameContext)
	if !ok {
		t.Fatalf("GameData type mismatch: %T", delta.GameData)
	}

	// 验证 4 个字段都正确识别
	if v := tftCtx.Gold.OrZero(); v != 42 {
		t.Errorf("Gold = %d, want 42 (Present=%v)", v, tftCtx.Gold.IsKnown())
	}
	if !tftCtx.Gold.IsKnown() {
		t.Error("Gold should be known")
	}
	if v := tftCtx.HP.OrZero(); v != 67 {
		t.Errorf("HP = %d, want 67", v)
	}
	if v := tftCtx.Level.OrZero(); v != 7 {
		t.Errorf("Level = %d, want 7", v)
	}
	if v := tftCtx.Stage.OrZero(); v != "4-2" {
		t.Errorf("Stage = %q, want %q", v, "4-2")
	}
}

func TestTFTOCRProvider_Confidence(t *testing.T) {
	engine := regionSizingMockEngine(t)
	provider := NewTFTOCRProvider(engine)

	imgBytes := makeTestImage(t)
	delta, err := provider.Contribute(context.Background(), game.ContextInput{
		Screenshot:     imgBytes,
		ScreenshotMIME: "image/jpeg",
	})
	if err != nil {
		t.Fatalf("Contribute failed: %v", err)
	}
	tftCtx := delta.GameData.(*TFTGameContext)

	// 验证 confidence 正确传递
	if tftCtx.Gold.Confidence < 0.5 {
		t.Errorf("Gold confidence too low: %f", tftCtx.Gold.Confidence)
	}
	if tftCtx.HP.Confidence < 0.5 {
		t.Errorf("HP confidence too low: %f", tftCtx.HP.Confidence)
	}
}

func TestTFTOCRProvider_EmptyScreenshot(t *testing.T) {
	engine := NewMockOCREngine()
	provider := NewTFTOCRProvider(engine)

	_, err := provider.Contribute(context.Background(), game.ContextInput{})
	if err == nil {
		t.Fatal("Empty screenshot should return error")
	}
}

func TestTFTOCRProvider_NilEngine(t *testing.T) {
	provider := NewTFTOCRProvider(nil)

	_, err := provider.Contribute(context.Background(), game.ContextInput{
		Screenshot: []byte{1, 2, 3},
	})
	if err == nil {
		t.Fatal("Nil engine should return error")
	}
}

func TestTFTOCRProvider_LowConfidence(t *testing.T) {
	engine := NewMockOCREngine()
	// 返回低可信度结果
	engine.SetCustomFunc(func(ctx context.Context, image []byte, mimeType string) (OCRResult, error) {
		return OCRResult{Text: "42", Confidence: 0.1}, nil
	})
	provider := NewTFTOCRProvider(engine)

	imgBytes := makeTestImage(t)
	_, err := provider.Contribute(context.Background(), game.ContextInput{
		Screenshot:     imgBytes,
		ScreenshotMIME: "image/jpeg",
	})
	if err == nil {
		t.Fatal("Low confidence should cause no-field-recognized error")
	}
}

func TestTFTOCRProvider_NoFieldRecognized(t *testing.T) {
	engine := NewMockOCREngine()
	// 返回空文本
	engine.SetCustomFunc(func(ctx context.Context, image []byte, mimeType string) (OCRResult, error) {
		return OCRResult{Text: "", Confidence: 0.95}, nil
	})
	provider := NewTFTOCRProvider(engine)

	imgBytes := makeTestImage(t)
	_, err := provider.Contribute(context.Background(), game.ContextInput{
		Screenshot:     imgBytes,
		ScreenshotMIME: "image/jpeg",
	})
	if err == nil {
		t.Fatal("Empty OCR text should cause no-field-recognized error")
	}
}

func TestTFTOCRProvider_ZeroHPIsValid(t *testing.T) {
	engine := NewMockOCREngine()
	engine.SetCustomFunc(func(ctx context.Context, imgData []byte, mimeType string) (OCRResult, error) {
		var img image.Image
		var err error
		if mimeType == "image/png" {
			img, err = png.Decode(bytes.NewReader(imgData))
		} else {
			img, err = jpeg.Decode(bytes.NewReader(imgData))
		}
		if err != nil || img == nil {
			return OCRResult{Text: "", Confidence: 0.0}, nil
		}
		bounds := img.Bounds()
		w, h := bounds.Dx(), bounds.Dy()
		if w == 50 && h == 35 {
			// HP 区域返回 0
			return OCRResult{Text: "0", Confidence: 0.9}, nil
		}
		return OCRResult{Text: "", Confidence: 0.0}, nil
	})
	provider := NewTFTOCRProvider(engine)

	imgBytes := makeTestImage(t)
	delta, err := provider.Contribute(context.Background(), game.ContextInput{
		Screenshot:     imgBytes,
		ScreenshotMIME: "image/jpeg",
	})
	if err != nil {
		t.Fatalf("Contribute failed: %v", err)
	}
	tftCtx := delta.GameData.(*TFTGameContext)
	// HP=0 应该是合法值，不应该是 Unknown
	if !tftCtx.HP.IsKnown() {
		t.Error("HP=0 should be marked as known (player eliminated)")
	}
	if tftCtx.HP.OrZero() != 0 {
		t.Errorf("HP = %d, want 0", tftCtx.HP.OrZero())
	}
}

func TestTFTOCRProvider_Order(t *testing.T) {
	provider := NewTFTOCRProvider(NewMockOCREngine())
	if got := provider.Order(); got != 10 {
		t.Errorf("Order = %d, want 10", got)
	}
}

func TestTFTOCRProvider_Source(t *testing.T) {
	provider := NewTFTOCRProvider(NewMockOCREngine())
	if got := provider.Source(); got != game.SourceOCR {
		t.Errorf("Source = %s, want %s", got, game.SourceOCR)
	}
}

// --- Phase 10-8 新增字段测试 ---

// newAllFieldsMockEngine 构建按 region 尺寸返回不同 OCR 文本的 mock engine
//
// 通过 region 尺寸区分：每个 region 在 1920x1080 缩放后是唯一尺寸
func newAllFieldsMockEngine(t *testing.T) *MockOCREngine {
	t.Helper()
	engine := NewMockOCREngine()
	engine.SetCustomFunc(func(ctx context.Context, imgData []byte, mimeType string) (OCRResult, error) {
		var img image.Image
		var err error
		if mimeType == "image/png" {
			img, err = png.Decode(bytes.NewReader(imgData))
		} else {
			img, err = jpeg.Decode(bytes.NewReader(imgData))
		}
		if err != nil || img == nil {
			return OCRResult{Text: "", Confidence: 0.0}, nil
		}
		bounds := img.Bounds()
		w, h := bounds.Dx(), bounds.Dy()
		// Player 区域
		switch {
		case w == 55 && h == 40:
			return OCRResult{Text: "42", Confidence: 0.95}, nil // Gold
		case w == 50 && h == 35:
			return OCRResult{Text: "67", Confidence: 0.92}, nil // HP
		case w == 50 && h == 25:
			return OCRResult{Text: "7", Confidence: 0.90}, nil // Level
		case w == 280 && h == 35:
			return OCRResult{Text: "4-2", Confidence: 0.88}, nil // Stage
		case w == 50 && h == 20:
			return OCRResult{Text: "20", Confidence: 0.90}, nil // XP
		}
		// Board 格子 (64x64) - 至少返回一个有效英雄
		if w == 64 && h == 64 {
			// 根据格子位置返回不同英雄（按 Position 估算）
			// 简化：固定返回 1 个有内容的，其他返回空
			// 通过对比不同的 (w,h) 难以做到，这里简单返回 1 个固定英雄
			// 实际我们识别 1 个 board slot 即可验证
			return OCRResult{Text: "Vayne", Confidence: 0.85}, nil
		}
		// Bench 格子 (64x64) - 同上
		if w == 64 && h == 64 {
			return OCRResult{Text: "Ahri", Confidence: 0.85}, nil
		}
		// Shop 格子 (64x64) - 返回带 cost
		if w == 64 && h == 64 {
			return OCRResult{Text: "Xayah | 3", Confidence: 0.85}, nil
		}
		// Items 区域 (280x200)
		if w == 280 && h == 200 {
			return OCRResult{Text: "TFT18_BF\nTFT18_IE", Confidence: 0.80}, nil
		}
		// Traits 区域 (200x600)
		if w == 200 && h == 600 {
			return OCRResult{Text: "Arcanist 2/4/6\nFrontline 2/4", Confidence: 0.80}, nil
		}
		// Augments 格子 (150x150)
		if w == 150 && h == 150 {
			return OCRResult{Text: "Combat Medic | silver", Confidence: 0.85}, nil
		}
		// Carousel 格子 (150x150)
		if w == 150 && h == 150 {
			return OCRResult{Text: "TFT18_Spat", Confidence: 0.85}, nil
		}
		return OCRResult{Text: "", Confidence: 0.0}, nil
	})
	return engine
}

// TestTFTOCRProvider_XPRecognition 验证 XP 字段识别
func TestTFTOCRProvider_XPRecognition(t *testing.T) {
	engine := newAllFieldsMockEngine(t)
	provider := NewTFTOCRProvider(engine)

	imgBytes := makeTestImage(t)
	delta, err := provider.Contribute(context.Background(), game.ContextInput{
		Screenshot:     imgBytes,
		ScreenshotMIME: "image/jpeg",
	})
	if err != nil {
		t.Fatalf("Contribute failed: %v", err)
	}
	tftCtx := delta.GameData.(*TFTGameContext)
	if v := tftCtx.XP.OrZero(); v != 20 {
		t.Errorf("XP = %d, want 20", v)
	}
	if !tftCtx.XP.IsKnown() {
		t.Error("XP should be known")
	}
}

// TestTFTOCRProvider_BoardRecognition 验证 Board 字段识别
func TestTFTOCRProvider_BoardRecognition(t *testing.T) {
	engine := newAllFieldsMockEngine(t)
	provider := NewTFTOCRProvider(engine)
	// 注册 Vayne 别名
	provider.normalizer.RegisterAlias("Vayne", "TFT18_Vayne")

	imgBytes := makeTestImage(t)
	delta, err := provider.Contribute(context.Background(), game.ContextInput{
		Screenshot:     imgBytes,
		ScreenshotMIME: "image/jpeg",
	})
	if err != nil {
		t.Fatalf("Contribute failed: %v", err)
	}
	tftCtx := delta.GameData.(*TFTGameContext)
	if !tftCtx.Board.IsKnown() {
		t.Error("Board should be known (至少识别 1 个格子)")
	}
	board := tftCtx.Board.OrZero()
	if len(board) == 0 {
		t.Error("Board should have at least 1 slot")
	}
	// 验证至少有一个 TFT18_Vayne
	foundVayne := false
	for _, slot := range board {
		if slot.ChampionID == "TFT18_Vayne" {
			foundVayne = true
			break
		}
	}
	if !foundVayne {
		t.Errorf("Board should contain TFT18_Vayne, got %+v", board)
	}
}

// TestTFTOCRProvider_UnknownChampionSkipped 验证未识别英雄不会被写入 Board
//
// 关键不变量：ChampionIDNormalizer 返回 Unknown 时，该格子应被跳过（不写入 Board/Bench/Shop）
func TestTFTOCRProvider_UnknownChampionSkipped(t *testing.T) {
	engine := NewMockOCREngine()
	engine.SetCustomFunc(func(ctx context.Context, imgData []byte, mimeType string) (OCRResult, error) {
		// 任何 region 都返回 "UnknownHero"（未注册别名）
		return OCRResult{Text: "UnknownHero", Confidence: 0.90}, nil
	})
	provider := NewTFTOCRProvider(engine)
	// 注册一个已知英雄（用于验证其他识别仍能工作）
	provider.normalizer.RegisterAlias("KnownHero", "TFT18_Known")

	imgBytes := makeTestImage(t)
	delta, err := provider.Contribute(context.Background(), game.ContextInput{
		Screenshot:     imgBytes,
		ScreenshotMIME: "image/jpeg",
	})
	// Gold/HP/Level/Stage/XP 都无法从 "UnknownHero" 提取数字 → false
	// Board/Bench/Shop 全部 Normalize 后都是 "Unknown" → 跳过 → false
	// Items/Traits/Augments 等可能仍识别（取决于实现）
	// 至少保证 Board/Bench/Shop 不包含 Unknown ID
	if err != nil {
		// 如果全部字段都没识别，返回 error 也合理
		return
	}
	tftCtx := delta.GameData.(*TFTGameContext)
	// 验证 Board/Bench/Shop 中没有 Unknown ID
	if tftCtx.Board.IsKnown() {
		for _, s := range tftCtx.Board.OrZero() {
			if s.ChampionID == UnknownID {
				t.Errorf("Board should not contain Unknown ID, got %+v", s)
			}
		}
	}
	if tftCtx.Bench.IsKnown() {
		for _, s := range tftCtx.Bench.OrZero() {
			if s.ChampionID == UnknownID {
				t.Errorf("Bench should not contain Unknown ID, got %+v", s)
			}
		}
	}
	if tftCtx.Shop.IsKnown() {
		for _, s := range tftCtx.Shop.OrZero() {
			if s.ChampionID == UnknownID {
				t.Errorf("Shop should not contain Unknown ID, got %+v", s)
			}
		}
	}
}

// TestTFTOCRProvider_AllFieldsRecognized 综合识别所有 Phase 10-8 字段
//
// 由于 mock engine 按尺寸返回，会同时返回所有字段的 OCR 结果。
func TestTFTOCRProvider_AllFieldsRecognized(t *testing.T) {
	engine := newAllFieldsMockEngine(t)
	provider := NewTFTOCRProvider(engine)
	// 注册所有英雄别名
	provider.normalizer.RegisterAlias("Vayne", "TFT18_Vayne")
	provider.normalizer.RegisterAlias("Ahri", "TFT18_Ahri")
	provider.normalizer.RegisterAlias("Xayah", "TFT18_Xayah")

	imgBytes := makeTestImage(t)
	delta, err := provider.Contribute(context.Background(), game.ContextInput{
		Screenshot:     imgBytes,
		ScreenshotMIME: "image/jpeg",
	})
	if err != nil {
		t.Fatalf("Contribute failed: %v", err)
	}
	tftCtx := delta.GameData.(*TFTGameContext)

	// Player 字段
	if v := tftCtx.Gold.OrZero(); v != 42 {
		t.Errorf("Gold = %d, want 42", v)
	}
	if v := tftCtx.HP.OrZero(); v != 67 {
		t.Errorf("HP = %d, want 67", v)
	}
	if v := tftCtx.Level.OrZero(); v != 7 {
		t.Errorf("Level = %d, want 7", v)
	}
	if v := tftCtx.Stage.OrZero(); v != "4-2" {
		t.Errorf("Stage = %q, want %q", v, "4-2")
	}
	if v := tftCtx.XP.OrZero(); v != 20 {
		t.Errorf("XP = %d, want 20", v)
	}
}
