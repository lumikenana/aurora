// Package tft - OCR 引擎抽象 + Mock + Unavailable 实现
//
// Phase 10-7 范围：
//   - 定义 OCREngine 接口（image → text + confidence）
//   - 实现 MockOCREngine（基于预定义映射的桩实现）
//   - 真实 OCR 引擎（Tesseract / PaddleOCR 等）留待 Phase 10-8+
//
// Phase 14-D：
//   - Gosseract/Tesseract 引擎已拆出到独立 build-tag 文件：
//     gosseract_engine_tesseract.go（-tags tesseract 编译）
//     gosseract_engine_stub.go（默认构建占位）
//   - VisionOCREngine 在 vision_ocr_engine.go 实现（Phase 14-D 首选）
//
// 设计目标：
//   - 接口极简（一个方法）
//   - 输入：图像字节（JPEG/PNG）+ MIME
//   - 输出：识别文本 + 可信度
//   - 失败返回 error，不 panic
package tft

import (
	"context"
)

// OCRResult 单次 OCR 识别的结果
type OCRResult struct {
	// Text 识别出的文本
	Text string
	// Confidence 可信度（0.0 ~ 1.0）
	Confidence float64
}

// OCREngine OCR 引擎抽象
//
// 真实引擎（Tesseract / PaddleOCR / 云端 API）实现此接口即可挂入 TFTOCRProvider。
// 引擎应是并发安全的，可被多个 Provider 共享。
type OCREngine interface {
	// Recognize 从图像字节中识别文本
	// image: JPEG/PNG 编码的图像字节
	// mimeType: 图像 MIME（如 "image/jpeg"）
	// 返回: 识别结果；错误（如引擎不可用、图像解析失败）返回 error
	Recognize(ctx context.Context, image []byte, mimeType string) (OCRResult, error)

	// Name 引擎名称（用于日志）
	Name() string
}

// --- MockOCREngine ---

// MockOCRMapping 单个图像的预期识别结果
type MockOCRMapping struct {
	// Width 图像宽度（用于匹配）
	Width int
	// Height 图像高度（用于匹配）
	Height int
	// RegionHash 区域字节的简单哈希（用于匹配特定区域）；空 = 任意
	RegionHash uint32
	// Result 期望的 OCR 结果
	Result OCRResult
}

// MockOCREngine 桩 OCR 引擎（Phase 10-7）
//
// 工作原理：
//   - 注册一组预期映射 (MockOCRMapping)
//   - Recognize 时按图像宽高 + 简单字节哈希查找匹配项
//   - 命中返回预定义结果；未命中返回 error
//
// 用于：
//   1. 单元测试（不依赖真实 OCR 即可验证整条链路）
//   2. E2E 调试（在真实环境先用 mock 验证 Provider 逻辑，再替换为真实引擎）
type MockOCREngine struct {
	mappings []MockOCRMapping
	// defaultResult 默认结果（无匹配时返回）
	defaultResult OCRResult
	// defaultError  默认错误（无匹配时返回；非 nil 时优先返回错误）
	defaultError error
	// customFunc 自定义识别函数（若非 nil，优先于 mappings 调用）
	// 签名与 Recognize 一致
	customFunc func(ctx context.Context, image []byte, mimeType string) (OCRResult, error)
}

// NewMockOCREngine 创建 Mock OCR 引擎
func NewMockOCREngine() *MockOCREngine {
	return &MockOCREngine{
		defaultResult: OCRResult{Text: "", Confidence: 0.0},
	}
}

// RegisterMapping 注册一个图像 → 识别结果映射
func (e *MockOCREngine) RegisterMapping(m MockOCRMapping) {
	e.mappings = append(e.mappings, m)
}

// SetDefault 设置默认结果（无匹配时返回）
func (e *MockOCREngine) SetDefault(r OCRResult) {
	e.defaultResult = r
}

// SetDefaultError 设置默认错误（无匹配时返回）
func (e *MockOCREngine) SetDefaultError(err error) {
	e.defaultError = err
}

// SetCustomFunc 设置自定义识别函数（用于测试灵活场景）
func (e *MockOCREngine) SetCustomFunc(f func(ctx context.Context, image []byte, mimeType string) (OCRResult, error)) {
	e.customFunc = f
}

// Name 实现 OCREngine
func (e *MockOCREngine) Name() string { return "mock" }

// Recognize 实现 OCREngine
func (e *MockOCREngine) Recognize(ctx context.Context, image []byte, mimeType string) (OCRResult, error) {
	if len(image) == 0 {
		return OCRResult{}, errEmptyImage
	}
	// 自定义函数优先
	if e.customFunc != nil {
		return e.customFunc(ctx, image, mimeType)
	}
	// 简单哈希（按字节求和 + 位数）
	h := simpleHash(image)
	for _, m := range e.mappings {
		if m.RegionHash != 0 && m.RegionHash == h {
			return m.Result, nil
		}
	}
	if e.defaultError != nil {
		return OCRResult{}, e.defaultError
	}
	return e.defaultResult, nil
}

// simpleHash 简单的非加密字节哈希（仅用于 mock 匹配）
func simpleHash(b []byte) uint32 {
	var h uint32 = 2166136261
	for _, c := range b {
		h ^= uint32(c)
		h *= 16777619
	}
	return h
}

// errEmptyImage 空图像错误
type ocrError string

func (e ocrError) Error() string { return string(e) }

const errEmptyImage = ocrError("empty image")

// --- UnavailableOCREngine (占位，当 OCR 依赖未安装时) ---

// UnavailableOCREngine 占位引擎，总是返回错误
// 用于当 Tesseract 未安装时，保持应用可启动，OCR 静默失败
type UnavailableOCREngine struct {
	reason string
}

// NewUnavailableOCREngine 创建占位引擎
func NewUnavailableOCREngine(reason string) *UnavailableOCREngine {
	return &UnavailableOCREngine{reason: reason}
}

// Name 实现 OCREngine
func (u *UnavailableOCREngine) Name() string {
	return "unavailable"
}

// Recognize 实现 OCREngine，总是返回错误
func (u *UnavailableOCREngine) Recognize(_ context.Context, _ []byte, _ string) (OCRResult, error) {
	return OCRResult{}, ocrError("OCR engine unavailable: " + u.reason)
}
