//go:build tesseract
// +build tesseract

// Package tft - Gosseract/Tesseract OCR 引擎实现
//
// Build tag 说明：
//   - 必须使用 `-tags tesseract` 才会编译此文件
//   - 编译时需安装 tesseract-ocr + leptonica 开发头文件（libtesseract + libleptonica）
//   - 运行时需 tesseract-ocr + tesseract-ocr-chi-sim 语言包
//
// Phase 14-D：默认构建不带 tesseract tag → 此文件被排除，
// createGosseractEngine 在无 tag 时自动降级为 Unavailable。
package tft

import (
	"bytes"
	"context"
	"image"
	"image/png"

	"github.com/otiai10/gosseract/v2"
)

// GosseractOCREngine 基于 github.com/otiai10/gosseract/v2 的 OCR 引擎实现
//
// 依赖：系统需要安装 Tesseract 开发包（头文件 + 库）
// 语言包：中文需要 `tesseract-ocr-chi-sim` 数据文件
// 默认语言："chi_sim+eng"（中文+英文）
type GosseractOCREngine struct {
	client *gosseract.Client
	lang   string
}

// NewGosseractOCREngine 创建 Gosseract OCR 引擎
// lang: 语言字符串，如 "chi_sim+eng"（默认 chi_sim+eng）
func NewGosseractOCREngine(lang string) *GosseractOCREngine {
	if lang == "" {
		lang = "chi_sim+eng"
	}
	client := gosseract.NewClient()
	client.SetLanguage(lang)
	return &GosseractOCREngine{
		client: client,
		lang:   lang,
	}
}

// Name 实现 OCREngine
func (g *GosseractOCREngine) Name() string {
	return "gosseract-tesseract"
}

// Recognize 实现 OCREngine
//
// 输入是 PNG 编码的裁剪子图字节（TFTOCRProvider 保证 PNG）
// 输出是识别文本 + 固定 0.7 可信度（Tesseract 不暴露单区域 confidence）
func (g *GosseractOCREngine) Recognize(ctx context.Context, imageBytes []byte, mimeType string) (OCRResult, error) {
	if len(imageBytes) == 0 {
		return OCRResult{}, errEmptyImage
	}

	// 解码图像 → 获取像素数据（gosseract 需要 image.Image）
	img, _, err := image.Decode(bytes.NewReader(imageBytes))
	if err != nil {
		return OCRResult{}, err
	}

	// 重新编码为 PNG 写入内存（gosseract 接受 bytes 输入）
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return OCRResult{}, err
	}

	// 使用 gosseract 识别（v2 API: SetImageFromBytes + Text）
	if err := g.client.SetImageFromBytes(buf.Bytes()); err != nil {
		return OCRResult{}, err
	}
	text, err := g.client.Text()
	if err != nil {
		return OCRResult{}, err
	}

	// Tesseract 不返回单区域 confidence → 固定用 0.7（表示"正常识别"）
	// 实际工程中 confidence 由 TFTOCRProvider 低层过滤（结果为空 → 识别失败）
	return OCRResult{
		Text:       text,
		Confidence: 0.7,
	}, nil
}

// Close 关闭客户端释放资源（程序退出时调用）
func (g *GosseractOCREngine) Close() error {
	return g.client.Close()
}
