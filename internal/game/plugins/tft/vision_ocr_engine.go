// Package tft - Vision LLM OCR 引擎（Phase 14-D）
//
// 背景：
//   - TFT 客户端文字大量使用艺术字体（英雄名/装备名/强化符文名）
//   - Tesseract 等传统 OCR 在艺术字 + 短词上准确率很低
//   - 已有 LLM Provider 支持多模态（ChatMessage.Images），可以"看图识字"
//   - 因此 Phase 14-D 把 LLM 包成 OCREngine，零新依赖地接入真实识别
//
// 设计要点：
//   - 实现现有 OCREngine 接口 → 保留所有调用方不变
//   - 额外实现 FieldAwareOCREngine（新增可选接口）→ 让 LLM 知道要识别什么字段
//   - Tesseract / Mock / Unavailable 引擎不动（继续可用）
//   - 默认作为 auto 模式首选；Tesseract 留作 fallback
//
// 性能 / 成本：
//   - 每个 region 一次 LLM 调用（~28 board + 9 bench + 5 shop + 5 player + ...）
//   - LLM 通常 < 1s 响应，结构化 prompt 让 token 消耗可控
//   - 未来 Phase 可优化为单次全图调用 + 整张图分析（更高效）
//
// 单 EXE 保留：
//   - 不引入 CGO / 第三方 OCR 库
//   - 复用现有 LLM Provider（已配 Doubao Vision 等）
//   - 任意时刻 OCR 不可用 → UnavailableOCREngine 兜底
package tft

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/lipipi/aurora/internal/errors"
	"github.com/lipipi/aurora/internal/llm"
)

// FieldAwareOCREngine 可选扩展接口：OCR 引擎支持"按字段提示"识别
//
// 实现此接口的引擎可在识别时获得字段语义信息（如 "gold" / "champion_name"），
// 让 LLM 等智能引擎能针对不同字段生成更精准的 prompt。
//
// 现有引擎（Tesseract / Mock / Unavailable）无需实现此接口，
// 调用方在 type-assert 失败时回退到通用 OCREngine.Recognize。
type FieldAwareOCREngine interface {
	OCREngine
	// RecognizeField 按字段语义识别图像
	// field: 字段名（如 "gold"/"hp"/"level"/"stage"/"xp"/"champion_name"/"item_name" 等）
	RecognizeField(ctx context.Context, image []byte, mimeType string, field string) (OCRResult, error)
}

// visionLLMCaller 抽象 LLM 调用，便于测试时 mock
//
// llm.LLMProvider 本身是接口，但为保持本包对其他包的低耦合，
// 在 VisionOCREngine 内部通过此小接口调用，避免直接依赖
// llm.ChatMessage 等具体类型到处扩散。
type visionLLMCaller interface {
	Call(ctx context.Context, systemPrompt, userPrompt string, imageDataURI string) (string, error)
	Name() string
}

// --- VisionOCREngine ---

// VisionOCREngine 基于 Vision LLM 的 OCR 引擎
//
// 工作流程：
//  1. 接收图像字节 + 字段提示
//  2. 编码为 data URI
//  3. 构造针对该字段优化的 prompt
//  4. 调用 LLM（ChatCompletion 非流式）
//  5. 解析 LLM 输出（结构化 JSON 或纯文本）→ OCRResult
//
// 并发安全：内部 mutex 保护共享 LLM client（虽然 LLM client 本身是线程安全，
// 但我们仍需保护自己的统计 counter）。
type VisionOCREngine struct {
	caller     visionLLMCaller
	maxTokens  int
	timeout    time.Duration
	lang       string // 期望输出语言 ("zh"/"en")

	mu        sync.Mutex
	callCount int
}

// NewVisionOCREngine 创建 Vision LLM OCR 引擎
//
// caller: LLM 调用器（生产环境用 llmAdapter 包装 llm.LLMProvider）
// maxTokens: 每次调用最大 token（默认 256）
// timeout: 单次调用超时（默认 15s）
// lang: 期望输出语言（"zh" 中文 / "en" 英文，默认 "zh"）
func NewVisionOCREngine(caller visionLLMCaller, maxTokens int, timeout time.Duration, lang string) *VisionOCREngine {
	if maxTokens <= 0 {
		maxTokens = 256
	}
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	if lang == "" {
		lang = "zh"
	}
	return &VisionOCREngine{
		caller:    caller,
		maxTokens: maxTokens,
		timeout:   timeout,
		lang:      lang,
	}
}

// Name 实现 OCREngine
func (v *VisionOCREngine) Name() string { return "vision-llm" }

// Stats 返回调用统计（用于测试和监控）
func (v *VisionOCREngine) Stats() (calls int) {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.callCount
}

// Recognize 实现 OCREngine（无字段提示，通用识别）
//
// LLM 收到图像后被要求直接读出图中所有文字。这种模式准确率低，
// 建议优先使用 RecognizeField。
func (v *VisionOCREngine) Recognize(ctx context.Context, image []byte, mimeType string) (OCRResult, error) {
	if v.caller == nil {
		return OCRResult{}, ocrError("vision OCR: nil LLM caller")
	}
	if len(image) == 0 {
		return OCRResult{}, errEmptyImage
	}

	dataURI := buildImageDataURI(image, mimeType)
	prompt := v.buildGenericPrompt()
	text, err := v.call(ctx, prompt, dataURI)
	if err != nil {
		return OCRResult{}, err
	}
	return OCRResult{
		Text:       strings.TrimSpace(text),
		Confidence: 0.7, // LLM 不返回 confidence；用合理默认
	}, nil
}

// RecognizeField 实现 FieldAwareOCREngine
//
// LLM 收到图像 + 字段语义后，针对该字段输出"标准答案"。
// 优点：准确率显著高于通用识别（LLM 知道要找什么）
func (v *VisionOCREngine) RecognizeField(ctx context.Context, image []byte, mimeType string, field string) (OCRResult, error) {
	if v.caller == nil {
		return OCRResult{}, ocrError("vision OCR: nil LLM caller")
	}
	if len(image) == 0 {
		return OCRResult{}, errEmptyImage
	}

	dataURI := buildImageDataURI(image, mimeType)
	prompt := v.buildFieldPrompt(field)
	text, err := v.call(ctx, prompt, dataURI)
	if err != nil {
		return OCRResult{}, err
	}
	text = strings.TrimSpace(text)
	if text == "" {
		// LLM 主动返回空 → 识别为"无文字"（confidence 低）
		return OCRResult{Text: "", Confidence: 0.3}, nil
	}
	return OCRResult{
		Text:       text,
		Confidence: 0.85, // 字段感知识别 → 较高 confidence
	}, nil
}

// call 真正调用 LLM（带 timeout + 计数）
func (v *VisionOCREngine) call(ctx context.Context, userPrompt, dataURI string) (string, error) {
	v.mu.Lock()
	v.callCount++
	v.mu.Unlock()

	cctx, cancel := context.WithTimeout(ctx, v.timeout)
	defer cancel()

	systemPrompt := v.buildSystemPrompt()
	return v.caller.Call(cctx, systemPrompt, userPrompt, dataURI)
}

// buildSystemPrompt 构造系统提示词
func (v *VisionOCREngine) buildSystemPrompt() string {
	langHint := "中文"
	if v.lang == "en" {
		langHint = "English"
	}
	return fmt.Sprintf(
		"你是一个高精度的 OCR 助手，专门用于游戏画面文字识别。\n"+
			"请直接输出图片中读取到的文字内容，不要加任何解释、引号或额外符号。\n"+
			"如果图片中没有相应内容，输出空字符串。\n"+
			"输出语言：%s。",
		langHint,
	)
}

// buildGenericPrompt 通用识别 prompt（无字段信息）
func (v *VisionOCREngine) buildGenericPrompt() string {
	return "请读取图片中的所有文字内容，按原样输出。"
}

// buildFieldPrompt 按字段生成专门的 prompt
//
// 每种字段有不同的输出格式要求（数字/中文名/英文名/结构等），
// 针对性 prompt 让 LLM 准确率更高。
func (v *VisionOCREngine) buildFieldPrompt(field string) string {
	switch field {
	case "gold":
		return "请读取图片中的金币数字（纯数字，不含'金币'等字样）。只输出数字。"
	case "hp":
		return "请读取图片中的玩家血量数字（纯数字）。只输出数字。"
	case "level":
		return "请读取图片中的玩家等级数字（纯数字，1-11）。只输出数字。"
	case "stage":
		return "请读取图片中的当前阶段文字（如 '4-2'、'5-1' 等 N-M 格式）。只输出阶段文字。"
	case "xp":
		return "请读取图片中的当前经验值数字（纯数字）。只输出数字。"
	case "champion_name":
		return "请读取图片中的英雄名称（可能是中文或英文）。只输出英雄名，不要加其他内容。"
	case "champion_with_star":
		return "请读取图片中的英雄名称和星级。格式：'英雄名 星级数字'，例如 'Vayne 2' 或 '薇恩 2'。"
	case "champion_with_items":
		return "请读取图片中的英雄名称、星级和装备。\n" +
			"格式：'英雄名 | 星级 | 装备1/装备2'。\n" +
			"如果无法识别某部分，对应位置留空，例如 'Vayne | 2 | IE'。"
	case "item_name":
		return "请读取图片中的装备名称（中文或英文）。只输出装备名。"
	case "shop_cost":
		return "请读取图片中的英雄费用数字（1-5 的纯数字）。只输出数字。"
	case "augment_name":
		return "请读取图片中的强化符文名称（中文或英文）。只输出名称。"
	case "augment_with_tier":
		return "请读取图片中的强化符文名称和等级。格式：'名称 | silver/gold/prismatic'。"
	case "trait_line":
		return "请读取图片中的一行羁绊文字（含名称和 2/4/6 之类样式数字）。按原样输出。"

	default:
		return "请读取图片中的文字内容，按原样输出。"
	}
}

// buildImageDataURI 把图像字节编码为 data URI
func buildImageDataURI(data []byte, mimeType string) string {
	if mimeType == "" {
		mimeType = "image/jpeg"
	}
	return "data:" + mimeType + ";base64," + base64.StdEncoding.EncodeToString(data)
}

// --- llmAdapter: 把 llm.LLMProvider 适配到 visionLLMCaller ---

// llmAdapter 把 llm.LLMProvider 适配成 visionLLMCaller
type llmAdapter struct {
	provider llm.LLMProvider
}

// newLLMAdapter 构造适配器
func newLLMAdapter(p llm.LLMProvider) *llmAdapter {
	return &llmAdapter{provider: p}
}

// Name 实现 visionLLMCaller
func (a *llmAdapter) Name() string {
	if a.provider == nil {
		return "none"
	}
	return a.provider.Name()
}

// Call 实现 visionLLMCaller
//
// 通过 llm.ChatCompletionRequest 单次调用 LLM，
// 复用现有 ChatMessage.Images 多模态通路。
func (a *llmAdapter) Call(ctx context.Context, systemPrompt, userPrompt, imageDataURI string) (string, error) {
	if a.provider == nil {
		return "", fmt.Errorf("vision OCR: LLM provider is nil")
	}
	if !a.provider.IsConfigured() {
		return "", fmt.Errorf("vision OCR: LLM provider not configured")
	}

	// 提取 MIME（data URI 格式: "data:<mime>;base64,<data>"）
	mimeType := "image/jpeg"
	if idx := strings.Index(imageDataURI, ":"); idx > 0 {
		header := imageDataURI[idx+1:]
		if semi := strings.Index(header, ";"); semi > 0 {
			mimeType = header[:semi]
		}
	}

	req := llm.ChatCompletionRequest{
		Messages: []llm.ChatMessage{
			{Role: llm.RoleSystem, Content: systemPrompt},
			{
				Role:    llm.RoleUser,
				Content: userPrompt,
				Images: []llm.ImageContent{
					{DataURI: imageDataURI, MIMEType: mimeType},
				},
			},
		},
		Temperature: 0.0, // 识别任务要确定性
		MaxTokens:   256,
		Stream:      false,
	}

	resp, err := a.provider.ChatCompletion(ctx, req)
	if err != nil {
		return "", err
	}
	if resp == nil {
		return "", fmt.Errorf("vision OCR: LLM returned nil response")
	}
	return resp.Content, nil
}

// --- 错误处理 ---

// visionEngineError 包装 vision OCR 错误
type visionEngineError struct {
	reason string
}

func (e *visionEngineError) Error() string { return "vision OCR: " + e.reason }

// JSONMarshalIndent 用于测试时输出格式化的 JSON（保持导入）
var _ = json.MarshalIndent

// ensure errors package used for future expansion
var _ = errors.CodeLLMRequest
