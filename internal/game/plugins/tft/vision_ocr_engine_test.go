// Phase 14-D: Vision OCR 引擎测试
//
// 测试范围：
//   - VisionOCREngine 基本调用（Recognize / RecognizeField）
//   - FieldAwareOCREngine 接口实现正确性
//   - LLM caller 失败时优雅降级
//   - 字段感知：不同 field 传不同 prompt
//   - Empty image / Nil caller / LLM error → graceful failure
//   - LLM stats 计数正确
//
// 测试策略：
//   - 用 stubLLMCaller（实现 visionLLMCaller 接口）注入测试响应
//   - 不依赖真实 LLM API
//   - 验证 engine 行为（call dispatch、prompt 构造、error 路径）
package tft

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
)

// stubLLMCaller 测试用 LLM caller
//
// 可预设响应（按调用顺序）或自定义函数
type stubLLMCaller struct {
	mu        sync.Mutex
	responses []string // 按调用顺序消费
	errors    []error  // 按调用顺序消费（与 responses 配对）
	calls     []stubCall
	customFn  func(ctx context.Context, systemPrompt, userPrompt, imageDataURI string) (string, error)
}

type stubCall struct {
	systemPrompt string
	userPrompt   string
	imageDataURI string
}

func (s *stubLLMCaller) Name() string { return "stub" }

func (s *stubLLMCaller) Call(ctx context.Context, systemPrompt, userPrompt, imageDataURI string) (string, error) {
	s.mu.Lock()
	s.calls = append(s.calls, stubCall{
		systemPrompt: systemPrompt,
		userPrompt:   userPrompt,
		imageDataURI: imageDataURI,
	})
	if s.customFn != nil {
		s.mu.Unlock()
		return s.customFn(ctx, systemPrompt, userPrompt, imageDataURI)
	}
	var (
		text string
		err  error
	)
	if len(s.responses) > 0 {
		text = s.responses[0]
		s.responses = s.responses[1:]
	}
	if len(s.errors) > 0 {
		err = s.errors[0]
		s.errors = s.errors[1:]
	}
	s.mu.Unlock()
	return text, err
}

func (s *stubLLMCaller) callCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.calls)
}

// --- VisionOCREngine 基本测试 ---

func TestVisionOCREngine_Name(t *testing.T) {
	e := NewVisionOCREngine(&stubLLMCaller{}, 256, 0, "zh")
	if e.Name() != "vision-llm" {
		t.Errorf("Name = %q, want %q", e.Name(), "vision-llm")
	}
}

func TestVisionOCREngine_Recognize_BasicCall(t *testing.T) {
	stub := &stubLLMCaller{
		responses: []string{"42"},
	}
	e := NewVisionOCREngine(stub, 256, 0, "zh")

	// 构造一个最小图像
	imgBytes := []byte{0x89, 0x50, 0x4E, 0x47} // PNG magic
	result, err := e.Recognize(context.Background(), imgBytes, "image/png")
	if err != nil {
		t.Fatalf("Recognize error: %v", err)
	}
	if result.Text != "42" {
		t.Errorf("Text = %q, want %q", result.Text, "42")
	}
	if result.Confidence != 0.7 {
		t.Errorf("Confidence = %f, want 0.7", result.Confidence)
	}
	if stub.callCount() != 1 {
		t.Errorf("LLM call count = %d, want 1", stub.callCount())
	}
}

func TestVisionOCREngine_Recognize_NilCaller(t *testing.T) {
	e := NewVisionOCREngine(nil, 256, 0, "zh")
	_, err := e.Recognize(context.Background(), []byte{1, 2, 3}, "image/png")
	if err == nil {
		t.Fatal("expected error for nil caller, got nil")
	}
}

func TestVisionOCREngine_Recognize_EmptyImage(t *testing.T) {
	e := NewVisionOCREngine(&stubLLMCaller{}, 256, 0, "zh")
	_, err := e.Recognize(context.Background(), nil, "image/png")
	if err == nil {
		t.Fatal("expected error for empty image, got nil")
	}
}

func TestVisionOCREngine_Recognize_LLMError(t *testing.T) {
	stub := &stubLLMCaller{
		errors: []error{errors.New("llm unavailable")},
	}
	e := NewVisionOCREngine(stub, 256, 0, "zh")
	_, err := e.Recognize(context.Background(), []byte{1, 2, 3}, "image/png")
	if err == nil {
		t.Fatal("expected error from LLM, got nil")
	}
	if !strings.Contains(err.Error(), "llm unavailable") {
		t.Errorf("error message = %q, want contains %q", err.Error(), "llm unavailable")
	}
}

func TestVisionOCREngine_Recognize_EmptyResponse(t *testing.T) {
	// LLM 主动返回空 → engine 透传空文本
	// 调用方在 ocrRawText 中通过 confidence < 0.3 阈值判断为识别失败
	// （通用 Recognize 用固定 0.7 confidence，与 Tesseract 一致）
	stub := &stubLLMCaller{
		responses: []string{""},
	}
	e := NewVisionOCREngine(stub, 256, 0, "zh")
	result, err := e.Recognize(context.Background(), []byte{1, 2, 3}, "image/png")
	if err != nil {
		t.Fatalf("Recognize error: %v", err)
	}
	if result.Text != "" {
		t.Errorf("Text = %q, want empty", result.Text)
	}
}

func TestVisionOCREngine_RecognizeField_EmptyResponse(t *testing.T) {
	// RecognizeField 在 LLM 返回空时主动降 confidence 到 0.3
	// 让 ocrRawText 的 < 0.3 阈值过滤掉
	stub := &stubLLMCaller{
		responses: []string{""},
	}
	e := NewVisionOCREngine(stub, 256, 0, "zh")
	result, err := e.RecognizeField(context.Background(), []byte{1, 2, 3}, "image/png", "gold")
	if err != nil {
		t.Fatalf("RecognizeField error: %v", err)
	}
	if result.Text != "" {
		t.Errorf("Text = %q, want empty", result.Text)
	}
	if result.Confidence != 0.3 {
		t.Errorf("Confidence = %f, want 0.3 (low for empty in field-aware)", result.Confidence)
	}
}

// --- FieldAwareOCREngine 测试 ---

func TestVisionOCREngine_RecognizeField_Gold(t *testing.T) {
	stub := &stubLLMCaller{
		responses: []string{"42"},
	}
	e := NewVisionOCREngine(stub, 256, 0, "zh")

	result, err := e.RecognizeField(context.Background(), []byte{1, 2, 3}, "image/png", "gold")
	if err != nil {
		t.Fatalf("RecognizeField error: %v", err)
	}
	if result.Text != "42" {
		t.Errorf("Text = %q, want %q", result.Text, "42")
	}
	if result.Confidence != 0.85 {
		t.Errorf("Confidence = %f, want 0.85 (field-aware)", result.Confidence)
	}
	// 验证 prompt 包含 gold 字段
	if len(stub.calls) != 1 {
		t.Fatalf("call count = %d, want 1", len(stub.calls))
	}
	if !strings.Contains(stub.calls[0].userPrompt, "金币") {
		t.Errorf("userPrompt should mention 金币 for field=gold, got %q", stub.calls[0].userPrompt)
	}
}

func TestVisionOCREngine_RecognizeField_ChampionWithStar(t *testing.T) {
	stub := &stubLLMCaller{
		responses: []string{"Vayne 2"},
	}
	e := NewVisionOCREngine(stub, 256, 0, "zh")

	result, err := e.RecognizeField(context.Background(), []byte{1, 2, 3}, "image/jpeg", "champion_with_star")
	if err != nil {
		t.Fatalf("RecognizeField error: %v", err)
	}
	if result.Text != "Vayne 2" {
		t.Errorf("Text = %q, want %q", result.Text, "Vayne 2")
	}
	// 验证 prompt 包含 hero/star hint
	if !strings.Contains(stub.calls[0].userPrompt, "英雄") {
		t.Errorf("userPrompt should mention 英雄 for field=champion_with_star, got %q", stub.calls[0].userPrompt)
	}
}

func TestVisionOCREngine_RecognizeField_UnknownFieldFallsBackToGeneric(t *testing.T) {
	stub := &stubLLMCaller{
		responses: []string{"text"},
	}
	e := NewVisionOCREngine(stub, 256, 0, "zh")

	_, err := e.RecognizeField(context.Background(), []byte{1, 2, 3}, "image/png", "totally_unknown_field")
	if err != nil {
		t.Fatalf("RecognizeField error: %v", err)
	}
	// 未知字段应回退到通用 prompt（包含"读取"）
	if !strings.Contains(stub.calls[0].userPrompt, "读取") {
		t.Errorf("userPrompt for unknown field should fall back to generic, got %q", stub.calls[0].userPrompt)
	}
}

func TestVisionOCREngine_ImplementsFieldAwareOCREngine(t *testing.T) {
	// 编译时类型断言（保证实现一致性）
	var engine OCREngine = NewVisionOCREngine(&stubLLMCaller{}, 256, 0, "zh")
	var fae FieldAwareOCREngine = engine.(FieldAwareOCREngine)
	if fae == nil {
		t.Fatal("VisionOCREngine should implement FieldAwareOCREngine")
	}
}

// --- 统计测试 ---

func TestVisionOCREngine_Stats(t *testing.T) {
	stub := &stubLLMCaller{
		responses: []string{"a", "b", "c"},
	}
	e := NewVisionOCREngine(stub, 256, 0, "zh")
	ctx := context.Background()
	img := []byte{1, 2, 3}

	for i := 0; i < 3; i++ {
		_, _ = e.Recognize(ctx, img, "image/png")
	}

	if got := e.Stats(); got != 3 {
		t.Errorf("Stats() = %d, want 3", got)
	}
}

// --- 与 OCREngine 接口的兼容性 ---

func TestVisionOCREngine_AsOCREngine(t *testing.T) {
	stub := &stubLLMCaller{
		responses: []string{"hello"},
	}
	var engine OCREngine = NewVisionOCREngine(stub, 256, 0, "zh")
	if engine.Name() != "vision-llm" {
		t.Errorf("Name via OCREngine = %q, want %q", engine.Name(), "vision-llm")
	}
	result, err := engine.Recognize(context.Background(), []byte{1, 2, 3}, "image/png")
	if err != nil {
		t.Fatalf("Recognize via OCREngine error: %v", err)
	}
	if result.Text != "hello" {
		t.Errorf("Text = %q, want %q", result.Text, "hello")
	}
}

// --- buildImageDataURI 测试 ---

func TestBuildImageDataURI(t *testing.T) {
	data := []byte{0x01, 0x02, 0x03}
	uri := buildImageDataURI(data, "image/jpeg")
	if !strings.HasPrefix(uri, "data:image/jpeg;base64,") {
		t.Errorf("URI prefix wrong: %s", uri[:50])
	}
}

func TestBuildImageDataURI_EmptyMIME(t *testing.T) {
	uri := buildImageDataURI([]byte{1}, "")
	if !strings.HasPrefix(uri, "data:image/jpeg;base64,") {
		t.Errorf("expected default image/jpeg, got: %s", uri[:50])
	}
}
