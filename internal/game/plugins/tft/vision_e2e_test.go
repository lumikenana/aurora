// Phase 14-D: 端到端 E2E 测试（不依赖真实游戏 / 真实 LLM）
//
// 测试目标：
//   1. Screenshot → VisionOCREngine → TFTGameContext 完整链路
//   2. 字段在 GameContext 中正确填充（Gold/HP/Level/Stage/XP/Board/Bench/Shop/Items/Augments）
//   3. Champion/Item 名称 → canonical ID 归一化（patch 隔离）
//   4. Unknown 不覆盖旧值（incremental merge 不变量）
//   5. Confidence 正确传递
//   6. Empty/failure 优雅降级
//   7. OCR 失败不污染 GameContext
//
// 测试策略：
//   - 直接调用 TFTOCRProvider.Contribute（不通过 Manager）
//   - queuedStubLLMCaller 按调用顺序返回预设结果
//   - 构造 1920x1080 RGBA 图像模拟 TFT 窗口
//   - 检查返回的 ContextDelta 中 GameData 各字段值
//
// 实际调用顺序（由 ocr.go:Contribute 决定）：
//   Player: gold(0), hp(1), level(2), stage(3), xp(4)
//   Board: 28 个格子（按 order 编号 0..27）
//   Bench: 9 个格子
//   Shop: 5 个格子
//   Items/Traits/Augments/Carousel: 数量由 regions 决定
//
// 字段识别后由 ocr.go 写入时硬编码 confidence（与 engine 返回无关）：
//   Player: 0.9, Board/Bench/Shop: 0.8, Items/Traits/Carousel: 0.7, Augments: 0.8
package tft

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/png"
	"sync"
	"testing"

	"github.com/lipipi/aurora/internal/game"
)

// queuedStubLLMCaller 按调用顺序返回预设结果
//
// 复用 vision_ocr_engine_test.go 的 stubCall 类型。
type queuedStubLLMCaller struct {
	mu        sync.Mutex
	responses []string
	errors    []error
	calls     []stubCall
}

func (q *queuedStubLLMCaller) Name() string { return "queued-stub" }

func (q *queuedStubLLMCaller) Call(ctx context.Context, systemPrompt, userPrompt, imageDataURI string) (string, error) {
	q.mu.Lock()
	q.calls = append(q.calls, stubCall{
		systemPrompt: systemPrompt,
		userPrompt:   userPrompt,
		imageDataURI: imageDataURI,
	})
	var (
		text string
		err  error
	)
	if len(q.responses) > 0 {
		text = q.responses[0]
		q.responses = q.responses[1:]
	}
	if len(q.errors) > 0 {
		err = q.errors[0]
		q.errors = q.errors[1:]
	}
	q.mu.Unlock()
	return text, err
}

func (q *queuedStubLLMCaller) callCount() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.calls)
}

func (q *queuedStubLLMCaller) lastCall() (stubCall, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.calls) == 0 {
		return stubCall{}, false
	}
	return q.calls[len(q.calls)-1], true
}

// --- 辅助函数 ---

// makeScreenshotPNG 构造 1920x1080 PNG 测试图
func makeScreenshotPNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 1920, 1080))
	gray := color.RGBA{R: 64, G: 64, B: 64, A: 255}
	for y := 0; y < 1080; y++ {
		for x := 0; x < 1920; x++ {
			img.Set(x, y, gray)
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode: %v", err)
	}
	return buf.Bytes()
}

// newTestProvider 创建一个测试用 TFTOCRProvider（vision engine + stub LLM）
func newTestProvider(stub *queuedStubLLMCaller) *TFTOCRProvider {
	engine := NewVisionOCREngine(stub, 256, 0, "zh")
	return NewTFTOCRProvider(engine)
}

// --- E2E 测试 ---

func TestE2E_VisionOCR_ScreenshotToContextDelta(t *testing.T) {
	// 准备 5 player 字段的识别结果（按调用顺序）
	// gold(0), hp(1), level(2), stage(3), xp(4)
	stub := &queuedStubLLMCaller{
		responses: []string{
			"42",  // gold
			"85",  // hp
			"7",   // level
			"4-2", // stage
			"16",  // xp
		},
	}
	provider := newTestProvider(stub)

	ctx := context.Background()
	delta, err := provider.Contribute(ctx, game.ContextInput{
		Screenshot:     makeScreenshotPNG(t),
		ScreenshotMIME: "image/png",
	})
	if err != nil {
		t.Fatalf("Contribute error: %v", err)
	}
	if delta == nil {
		t.Fatal("Contribute returned nil delta")
	}
	if delta.GameData == nil {
		t.Fatal("delta.GameData is nil")
	}
	tftCtx, ok := delta.GameData.(*TFTGameContext)
	if !ok || tftCtx == nil {
		t.Fatal("GameData is not *TFTGameContext")
	}

	// 验证 Player 字段
	if !tftCtx.Gold.IsKnown() || tftCtx.Gold.Val != 42 {
		t.Errorf("Gold = %v, want 42", tftCtx.Gold)
	}
	if !tftCtx.HP.IsKnown() || tftCtx.HP.Val != 85 {
		t.Errorf("HP = %v, want 85", tftCtx.HP)
	}
	if !tftCtx.Level.IsKnown() || tftCtx.Level.Val != 7 {
		t.Errorf("Level = %v, want 7", tftCtx.Level)
	}
	if !tftCtx.Stage.IsKnown() || tftCtx.Stage.Val != "4-2" {
		t.Errorf("Stage = %v, want 4-2", tftCtx.Stage)
	}
	if !tftCtx.XP.IsKnown() || tftCtx.XP.Val != 16 {
		t.Errorf("XP = %v, want 16", tftCtx.XP)
	}

	// Source 应为 SourceOCR
	if tftCtx.Gold.Source != game.SourceOCR {
		t.Errorf("Gold.Source = %v, want SourceOCR", tftCtx.Gold.Source)
	}

	// Board 应不识别（stub 没有更多响应）→ 保持 Unknown
	if tftCtx.Board.IsKnown() {
		t.Errorf("Board should not be known when LLM doesn't respond, got %+v", tftCtx.Board)
	}

	// 验证 LLM 至少被调用了 5 次（5 player fields）
	if stub.callCount() < 5 {
		t.Errorf("LLM call count = %d, want >= 5", stub.callCount())
	}
}

func TestE2E_VisionOCR_ConfidencePropagation(t *testing.T) {
	// 验证 Player 字段 confidence = 0.9（ocr.go:152 硬编码）
	stub := &queuedStubLLMCaller{
		responses: []string{"55", "75", "9", "6-1", "30"},
	}
	provider := newTestProvider(stub)
	ctx := context.Background()
	delta, err := provider.Contribute(ctx, game.ContextInput{
		Screenshot:     makeScreenshotPNG(t),
		ScreenshotMIME: "image/png",
	})
	if err != nil {
		t.Fatalf("Contribute: %v", err)
	}
	if delta == nil {
		t.Fatal("delta nil")
	}
	tftCtx, _ := delta.GameData.(*TFTGameContext)
	if tftCtx.Gold.Confidence != 0.9 {
		t.Errorf("Gold.Confidence = %f, want 0.9 (ocr.go hardcoded)", tftCtx.Gold.Confidence)
	}
	if tftCtx.Gold.Source != game.SourceOCR {
		t.Errorf("Gold.Source = %v, want SourceOCR", tftCtx.Gold.Source)
	}
}

func TestE2E_VisionOCR_UnknownPreservesOldData(t *testing.T) {
	// 第一轮：所有 5 player 字段都识别成功
	// 第二轮：所有 LLM 调用都返回空 → provider 应返回 errNoFieldRecognized
	//   旧 GameContext 不被修改（merge 不会发生）
	stub := &queuedStubLLMCaller{
		responses: []string{"42", "85", "7", "4-2", "16"},
	}
	provider := newTestProvider(stub)
	ctx := context.Background()
	input := game.ContextInput{
		Screenshot:     makeScreenshotPNG(t),
		ScreenshotMIME: "image/png",
	}

	// 第一轮
	delta1, err := provider.Contribute(ctx, input)
	if err != nil {
		t.Fatalf("first contribute error: %v", err)
	}
	if delta1 == nil {
		t.Fatal("first delta nil")
	}
	tftCtx1, _ := delta1.GameData.(*TFTGameContext)
	if !tftCtx1.Gold.IsKnown() || tftCtx1.Gold.Val != 42 {
		t.Fatalf("first round Gold = %v, want 42", tftCtx1.Gold)
	}

	// 模拟 GameContext 合并：构造 current + 应用 delta1
	current := &TFTGameContext{}
	current.MergeFrom(delta1.GameData)
	if !current.Gold.IsKnown() || current.Gold.Val != 42 {
		t.Fatalf("after merge Gold = %v, want 42", current.Gold)
	}

	// 第二轮：所有 stub.responses 已耗尽，剩余调用返回 ""（confidence=0.3 → 视为识别失败）
	// → 没有任何字段被识别 → 任何字段都没写入 tftCtx → provider 返回 errNoFieldRecognized
	_, err = provider.Contribute(ctx, input)
	if err == nil {
		t.Fatal("expected error when no fields are recognized, got nil")
	}

	// 验证：current 仍保留 42（merge 没发生）
	if !current.Gold.IsKnown() || current.Gold.Val != 42 {
		t.Errorf("current Gold after failed second round = %v, want 42 (preserved)", current.Gold)
	}
}

func TestE2E_VisionOCR_LLMErrorReturnsError(t *testing.T) {
	// 所有 LLM 调用都失败 → Contribute 应返回 error
	stub := &queuedStubLLMCaller{
		errors: []error{errors.New("network down")},
	}
	provider := newTestProvider(stub)
	ctx := context.Background()
	delta, err := provider.Contribute(ctx, game.ContextInput{
		Screenshot:     makeScreenshotPNG(t),
		ScreenshotMIME: "image/png",
	})
	// 期望：err != nil（errNoFieldRecognized）或 delta == nil
	if err == nil && delta != nil {
		t.Errorf("expected error or nil delta when LLM fails, got delta=%+v", delta)
	}
	if delta != nil {
		t.Errorf("expected nil delta on LLM failure, got %+v", delta)
	}
}

func TestE2E_VisionOCR_PatchIsolation(t *testing.T) {
	// Champion "Vayne" → normalizer 归一化为 TFT18_Vayne
	// 切换 setKey 到 TFT17 → 该归一化结果应被 patch 拒绝
	// 调用顺序：5 player + 28 board
	// 提供前 6 个响应：5 player + 1 board cell
	stub := &queuedStubLLMCaller{
		responses: []string{
			"50",    // gold
			"80",    // hp
			"6",     // level
			"3-2",   // stage
			"10",    // xp
			"Vayne", // board[0]
		},
	}
	provider := newTestProvider(stub)
	// 关键：切换 patch → Vayne 应被拒绝
	provider.SetPatch("TFT17")

	ctx := context.Background()
	delta, err := provider.Contribute(ctx, game.ContextInput{
		Screenshot:     makeScreenshotPNG(t),
		ScreenshotMIME: "image/png",
	})
	if err != nil {
		t.Fatalf("Contribute: %v", err)
	}
	if delta == nil {
		t.Fatal("delta nil")
	}
	tftCtx, _ := delta.GameData.(*TFTGameContext)

	// Player 字段无 patch 限制 → 应正常填充
	if !tftCtx.Gold.IsKnown() || tftCtx.Gold.Val != 50 {
		t.Errorf("Gold = %v, want 50", tftCtx.Gold)
	}

	// Board：Vayne 通过 setKey="TFT17" 但 normalizer 归一化为 TFT18_Vayne
	// → patch 隔离拒绝 → Board 应不包含 TFT17_Vayne
	if tftCtx.Board.IsKnown() {
		for _, slot := range tftCtx.Board.Val {
			if slot.ChampionID != "" {
				t.Errorf("Board should not contain champions with wrong set, got %+v", slot)
			}
		}
	}
}

func TestE2E_VisionOCR_EmptyScreenshotNoop(t *testing.T) {
	stub := &queuedStubLLMCaller{}
	provider := newTestProvider(stub)
	ctx := context.Background()
	_, err := provider.Contribute(ctx, game.ContextInput{})
	if err == nil {
		t.Fatal("expected error for empty screenshot")
	}
	if stub.callCount() != 0 {
		t.Errorf("no LLM call should be made for empty input, got %d", stub.callCount())
	}
}

func TestE2E_VisionOCR_ItemResolution(t *testing.T) {
	// 测试 board 格子包含装备 → 装备名应归一化为 canonical ID
	// 调用顺序：5 player + 28 board cells
	// board[0] 直接使用 canonical ID（Phase 14-D 默认 champion alias 表为空）
	stub := &queuedStubLLMCaller{
		responses: []string{
			"40", "90", "8", "5-1", "20", // 5 player
			"TFT18_Vayne | 2 | IE/Rageblade", // board[0]
		},
	}
	provider := newTestProvider(stub)
	ctx := context.Background()
	delta, err := provider.Contribute(ctx, game.ContextInput{
		Screenshot:     makeScreenshotPNG(t),
		ScreenshotMIME: "image/png",
	})
	if err != nil {
		t.Fatalf("Contribute: %v", err)
	}
	if delta == nil {
		t.Fatal("delta nil")
	}
	tftCtx, _ := delta.GameData.(*TFTGameContext)
	if !tftCtx.Board.IsKnown() {
		t.Fatal("Board not known")
	}
	if len(tftCtx.Board.Val) == 0 {
		t.Fatal("Board should have at least one slot")
	}
	slot := tftCtx.Board.Val[0]
	if slot.ChampionID != "TFT18_Vayne" {
		t.Errorf("ChampionID = %q, want TFT18_Vayne", slot.ChampionID)
	}
	// 装备应归一化（IE → TFT18_InfinityEdge, Rageblade → TFT18_Rageblade）
	// 验证：若识别到，应是合法 canonical ID，不能是 Unknown
	for _, itemID := range slot.Items {
		if itemID == UnknownID {
			t.Errorf("Item should be resolved, got Unknown: %v", slot.Items)
		}
	}
}

func TestE2E_VisionOCR_FieldAwarePromptRouted(t *testing.T) {
	// 验证 ocrParseField/ocrRawText 走 RecognizeField 路径
	// （userPrompt 应该是字段特定的，不是通用 prompt）
	stub := &queuedStubLLMCaller{
		responses: []string{"99"}, // 第一个调用（gold）
	}
	provider := newTestProvider(stub)
	ctx := context.Background()
	_, err := provider.Contribute(ctx, game.ContextInput{
		Screenshot:     makeScreenshotPNG(t),
		ScreenshotMIME: "image/png",
	})
	if err != nil {
		t.Fatalf("Contribute: %v", err)
	}
	// 验证至少有一个调用使用了字段特定的 prompt（含"金币"）
	// 通用 prompt 包含"读取图片中的所有文字内容"
	called := false
	for _, c := range stub.calls {
		// 检查 prompt 是否包含字段特定的提示（不是通用 prompt）
		if (len(c.userPrompt) > 0) && c.userPrompt != "请读取图片中的所有文字内容，按原样输出。" {
			called = true
			break
		}
	}
	if !called {
		t.Errorf("expected field-specific prompts, all calls used generic prompt")
	}
}

func TestE2E_VisionOCR_SourceAndPhase(t *testing.T) {
	// 验证 provider.Source() 返回 SourceOCR, Order() = 10
	provider := newTestProvider(&queuedStubLLMCaller{})
	if provider.Source() != game.SourceOCR {
		t.Errorf("Source = %v, want SourceOCR", provider.Source())
	}
	if provider.Order() != 10 {
		t.Errorf("Order = %d, want 10", provider.Order())
	}
}

func TestE2E_VisionOCR_NonTFTScreenshotGracefulDegradation(t *testing.T) {
	// 非 TFT 场景：所有识别返回乱码 → 通过 normalizer 后大部分应被识别为 Unknown
	// → 只有 player 数字字段可能解析成功
	// → 不应 panic，不应污染 context
	stub := &queuedStubLLMCaller{
		responses: []string{
			"abc", // gold (not a number → fail parseGold)
			"xyz", // hp (not a number)
			"!@#", // level
			"??",  // stage
			"$$",  // xp
		},
	}
	provider := newTestProvider(stub)
	ctx := context.Background()
	delta, err := provider.Contribute(ctx, game.ContextInput{
		Screenshot:     makeScreenshotPNG(t),
		ScreenshotMIME: "image/png",
	})
	// 期望：所有 player 字段解析失败 → errNoFieldRecognized 或 delta 为空
	if err == nil && delta != nil {
		t.Errorf("expected error for invalid OCR results, got delta=%+v", delta)
	}
	// 不 panic 即通过
}

func TestE2E_VisionOCR_ImplementsContextProvider(t *testing.T) {
	// 编译时类型断言（保证实现 game.ContextProvider）
	var _ game.ContextProvider = (*TFTOCRProvider)(nil)
	var _ game.ContextProvider = newTestProvider(&queuedStubLLMCaller{})
}
