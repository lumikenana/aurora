//go:build !tesseract
// +build !tesseract

// Package tft - Gosseract 引擎占位（默认构建）
//
// 默认构建（无 tesseract tag）不编译 gosseract_engine_tesseract.go。
// 本文件提供 NewGosseractOCREngine 的占位实现，返回 Unavailable。
//
// 这样保证：
//   - 单 EXE 默认构建不依赖 libtesseract / libleptonica
//   - 想要 Tesseract 的用户可加 `-tags tesseract` 编译
package tft

// GosseractOCREngine 占位（无 tesseract tag 时）
//
// 真实实现在 gosseract_engine_tesseract.go（需要 `-tags tesseract` 编译）。
type GosseractOCREngine struct {
	lang string
}

// NewGosseractOCREngine 占位实现：返回 Unavailable 引擎
//
// 真实 Tesseract 引擎需要 `-tags tesseract` 编译 + 系统安装 libtesseract。
// 默认构建使用 VisionOCREngine 路径，无需任何系统依赖。
func NewGosseractOCREngine(lang string) OCREngine {
	_ = lang
	return NewUnavailableOCREngine("gosseract not compiled (build with -tags tesseract to enable)")
}
