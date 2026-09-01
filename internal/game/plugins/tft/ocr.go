// Package tft - TFT OCR ContextProvider
//
// Phase 10-7 范围：
//   - 实现 game.ContextProvider
//   - 链路上：ContextInput.Screenshot → 区域裁剪 → OCREngine → 解析 → ContextDelta
//   - 不实际调用截图（截图由调用方负责，通过 ContextInput 传入）
//   - 不修改 game / agent / llm 任何链路
//
// Phase 10-8 范围：
//   - 在 Phase 10-7 基础上识别 XP / Board / Bench / Shop / Items /
//     Traits / Augments / Carousel 等字段
//   - 棋盘 / 备战 / 商店 / 强化符文按"多格子"模型识别（每个格子一次 OCR 调用）
//   - 引入 ChampionIDNormalizer 做"名称 → ID"归一化（不查 Knowledge Base）
//   - 识别不到时返回 Unknown，**绝不猜测**
//
// 增量更新关键不变量：
//   - OCR 只填本轮识别出的字段；未识别的字段保持 Unknown
//   - Manager.applyDelta 负责字段级合并（保留 lastKnown 中已识别的字段）
//   - 空集合（empty slice）表示"已识别为空"；Present=false 表示"未识别"
//     → Value[[]T] 的 IsKnown() == true && len(Val) == 0 会覆盖旧值
package tft

import (
	"bytes"
	"context"
	"image"
	"image/jpeg"
	"image/png"
	"strings"

	"github.com/lipipi/aurora/internal/game"
	tftkb "github.com/lipipi/aurora/internal/game/knowledge/tft"
)

// TFTOCRProvider TFT 屏幕 OCR Provider
//
// 工作流程：
//  1. 从 ContextInput.Screenshot 解码为 image.Image
//  2. 按当前窗口尺寸缩放参考区域
//  3. 对每个区域裁剪子图
//  4. 调用 OCREngine 识别
//  5. 调用 parseGold/HP/Level/Stage/XP 等解析为强类型
//  6. 棋盘 / 备战 / 商店按格子逐个 OCR
//  7. 名称通过 ChampionIDNormalizer 归一化（Unknown 时跳过该字段）
//  8. 装备通过 ItemIDNormalizer 归一化（Unknown 时跳过该装备）
//  9. 构造 *game.ContextDelta（仅填本轮识别的字段）
//
// 关键不变量：
//   - 引擎复用（避免每次创建）
//   - 并发安全（通过引擎自身的并发安全保证）
//   - 失败不 panic；返回 error 或带部分字段的 delta
type TFTOCRProvider struct {
	engine         OCREngine
	regions        TFTScreenRegions
	normalizer     *ChampionIDNormalizer
	itemNormalizer *ItemIDNormalizer
}

// NewTFTOCRProvider 创建 TFT OCR Provider
func NewTFTOCRProvider(engine OCREngine) *TFTOCRProvider {
	return &TFTOCRProvider{
		engine:         engine,
		regions:        DefaultTFTScreenRegions,
		normalizer:     NewChampionIDNormalizer(),
		itemNormalizer: NewItemIDNormalizer(),
	}
}

// SetRegions 自定义 UI 区域（用于测试或调优）
func (p *TFTOCRProvider) SetRegions(r TFTScreenRegions) {
	p.regions = r
}

// SetPatch 设置当前 patch 前缀（用于跨 Set 时切换 ID 归一化行为）
func (p *TFTOCRProvider) SetPatch(patch string) {
	if patch != "" {
		p.normalizer = p.normalizer.WithPatch(patch)
		// setKey 就是 patch 前缀（如 "TFT18"）
		p.itemNormalizer = p.itemNormalizer.WithSetKey(patch)
	}
}

// SetRepository 设置 KB repository 用于 champion ID 解析（Phase 14-A）
//
// 注入后，OCR 识别出的原始名称会通过 KB 模糊匹配获取 canonical ID，
// 提高命中率。不注入仍可使用本地别名 fallback。
func (p *TFTOCRProvider) SetRepository(repo tftkb.TFTRepository) {
	p.normalizer = p.normalizer.WithRepository(repo)
	p.itemNormalizer = p.itemNormalizer.WithRepository(repo)
}

// Source 实现 game.ContextProvider
func (p *TFTOCRProvider) Source() game.ContextSource {
	return game.SourceOCR
}

// Order 实现 game.ContextProvider
// OCR Provider 在 WindowProvider 之后执行（Order=10）
func (p *TFTOCRProvider) Order() int {
	return 10
}

// Contribute 实现 game.ContextProvider
//
// 输入：ContextInput（必须包含 Screenshot 字节；可选 Extra）
// 输出：*game.ContextDelta（GameData 应是 *tft.TFTGameContext）
//
// 错误：截图为空 / 解码失败 / 引擎错误 → 返回 error（不写 lastKnown）
// 至少一个字段被识别时才返回 delta（避免无意义更新）
func (p *TFTOCRProvider) Contribute(ctx context.Context, in game.ContextInput) (*game.ContextDelta, error) {
	if len(in.Screenshot) == 0 {
		return nil, errEmptyScreenshot
	}
	if p.engine == nil {
		return nil, errNilEngine
	}

	// 解码图像
	img, err := decodeImage(in.Screenshot, in.ScreenshotMIME)
	if err != nil {
		return nil, err
	}
	bounds := img.Bounds()
	actualSize := image.Point{X: bounds.Dx(), Y: bounds.Dy()}

	// 缩放区域到当前实际尺寸
	scaled := ScaleRegions(p.regions, ReferenceResolution, actualSize)

	// OCR + 解析 Player 字段
	gold, goldOK := ocrParseField(ctx, p.engine, img, scaled.Gold, parseGold, "gold")
	hp, hpOK := ocrParseField(ctx, p.engine, img, scaled.HP, parseHP, "hp")
	level, levelOK := ocrParseField(ctx, p.engine, img, scaled.Level, parseLevel, "level")
	stage, stageOK := ocrParseField(ctx, p.engine, img, scaled.Stage, parseStage, "stage")
	xp, xpOK := ocrParseField(ctx, p.engine, img, scaled.XP, parseXP, "xp")

	// OCR + 解析 棋盘 / 备战 / 商店（多格子）
	boardSlots, boardOK := p.recognizeBoard(ctx, img, scaled.Board)
	benchSlots, benchOK := p.recognizeBench(ctx, img, scaled.Bench)
	shopSlots, shopOK := p.recognizeShop(ctx, img, scaled.Shop)

	// OCR + 解析 装备 / 羁绊 / 强化符文 / 选秀
	itemSlots, itemsOK := p.recognizeItems(ctx, img, scaled.Items)
	traitSlots, traitsOK := p.recognizeTraits(ctx, img, scaled.Traits)
	augmentSlots, augmentsOK := p.recognizeAugments(ctx, img, scaled.Augments)
	carouselInfo, carouselOK := p.recognizeCarousel(ctx, img, scaled.Carousel)

	// 构造 TFTGameContext（仅填本轮识别的字段；未识别保持 Unknown）
	tftCtx := &TFTGameContext{}
	anyRecognized := false

	if goldOK {
		tftCtx.Gold = game.Detected(gold, game.SourceOCR, 0.9)
		anyRecognized = true
	}
	if hpOK {
		tftCtx.HP = game.Detected(hp, game.SourceOCR, 0.9)
		anyRecognized = true
	}
	if levelOK {
		tftCtx.Level = game.Detected(level, game.SourceOCR, 0.9)
		anyRecognized = true
	}
	if stageOK {
		tftCtx.Stage = game.Detected(stage, game.SourceOCR, 0.9)
		anyRecognized = true
	}
	if xpOK {
		tftCtx.XP = game.Detected(xp, game.SourceOCR, 0.9)
		anyRecognized = true
	}
	if boardOK {
		tftCtx.Board = game.Detected(boardSlots, game.SourceOCR, 0.8)
		anyRecognized = true
	}
	if benchOK {
		tftCtx.Bench = game.Detected(benchSlots, game.SourceOCR, 0.8)
		anyRecognized = true
	}
	if shopOK {
		tftCtx.Shop = game.Detected(shopSlots, game.SourceOCR, 0.8)
		anyRecognized = true
	}
	if itemsOK {
		tftCtx.Items = game.Detected(itemSlots, game.SourceOCR, 0.7)
		anyRecognized = true
	}
	if traitsOK {
		tftCtx.Traits = game.Detected(traitSlots, game.SourceOCR, 0.7)
		anyRecognized = true
	}
	if augmentsOK {
		tftCtx.Augments = game.Detected(augmentSlots, game.SourceOCR, 0.8)
		anyRecognized = true
	}
	if carouselOK {
		tftCtx.Carousel = game.Detected(carouselInfo, game.SourceOCR, 0.7)
		anyRecognized = true
	}

	// 至少识别出一个字段才返回 delta（避免无意义更新）
	if !anyRecognized {
		return nil, errNoFieldRecognized
	}

	return &game.ContextDelta{
		GameData: tftCtx,
	}, nil
}

// extractFullPatch 从 current GameContext 提取完整 patch 版本
// 用于 KB 模糊查询
func extractFullPatch(current *game.GameContext) string {
	if current == nil {
		return ""
	}
	if current.CommonData.Version.IsKnown() {
		return current.CommonData.Version.Val
	}
	return ""
}

// recognizeBoard 识别棋盘格子
//
// 返回 BoardSlot 列表（去掉空格子 / Unknown ID 的格子）
// 全部格子都识别不到时，返回 (nil, false) — 不写入 Board 字段
//
// Phase 14-A: 现在使用 full patch 版本从 Knowledge Base 模糊匹配提高命中率
// Phase 14-B: 添加装备识别 - 每个格子识别英雄+星级+装备
// Phase 14-D: 若 OCR 引擎实现 FieldAwareOCREngine，传入 "champion_with_items" 字段提示
func (p *TFTOCRProvider) recognizeBoard(ctx context.Context, img image.Image, cells []CellRegion) ([]BoardSlot, bool) {
	if len(cells) == 0 {
		return nil, false
	}
	slots := make([]BoardSlot, 0, len(cells))
	anyOK := false
	for _, c := range cells {
		text, ok := ocrRawText(ctx, p.engine, img, c.Rect, "champion_with_items")
		if !ok || strings.TrimSpace(text) == "" {
			continue
		}
		// 解析 "ChampionName" 或 "ChampionName | Star" 或 "ChampionName | Star | Item1/Item2"
		name, star, items, ok := parseBoardCellText(text)
		if !ok || name == "" {
			continue
		}
		// Champion ID 归一化
		id := p.normalizer.Normalize(name)
		if id == UnknownID {
			continue
		}

		// 装备 ID 归一化（Phase 14-B）
		// 每个物品一行或用 '/' 分隔，逐个解析
		canonicalItems := make([]string, 0, len(items))
		for _, itemName := range items {
			itemName = strings.TrimSpace(itemName)
			if itemName == "" {
				continue
			}
			itemID := p.itemNormalizer.Normalize(itemName)
			if itemID != UnknownID {
				canonicalItems = append(canonicalItems, itemID)
			}
		}

		slots = append(slots, BoardSlot{
			ChampionID: id,
			Star:       star,
			Items:      canonicalItems,
			Position:   c.Position,
		})
		anyOK = true
	}
	if !anyOK {
		return nil, false
	}
	return slots, true
}

// recognizeBench 识别备战席格子
//
// 返回 BenchSlot 列表；规则同 Board
// Phase 14-D: 字段提示 "champion_with_star"
func (p *TFTOCRProvider) recognizeBench(ctx context.Context, img image.Image, cells []CellRegion) ([]BenchSlot, bool) {
	if len(cells) == 0 {
		return nil, false
	}
	slots := make([]BenchSlot, 0, len(cells))
	anyOK := false
	for _, c := range cells {
		text, ok := ocrRawText(ctx, p.engine, img, c.Rect, "champion_with_star")
		if !ok || strings.TrimSpace(text) == "" {
			continue
		}
		name, star, ok := parseCellText(text)
		if !ok {
			continue
		}
		id := p.normalizer.Normalize(name)
		if id == UnknownID {
			continue
		}
		slots = append(slots, BenchSlot{
			ChampionID: id,
			Star:       star,
		})
		anyOK = true
	}
	if !anyOK {
		return nil, false
	}
	return slots, true
}

// recognizeShop 识别商店格子
//
// 返回 ShopSlot 列表
// 文本格式："ChampionName" 或 "ChampionName | Cost"
// Phase 14-D: 字段提示 "champion_with_star"（cost 由 parseShopCellText 解析）
func (p *TFTOCRProvider) recognizeShop(ctx context.Context, img image.Image, cells []CellRegion) ([]ShopSlot, bool) {
	if len(cells) == 0 {
		return nil, false
	}
	slots := make([]ShopSlot, 0, len(cells))
	anyOK := false
	for _, c := range cells {
		text, ok := ocrRawText(ctx, p.engine, img, c.Rect, "champion_with_star")
		if !ok || strings.TrimSpace(text) == "" {
			continue
		}
		name, cost, ok := parseShopCellText(text)
		if !ok {
			continue
		}
		id := p.normalizer.Normalize(name)
		if id == UnknownID {
			continue
		}
		slots = append(slots, ShopSlot{
			ChampionID: id,
			Cost:       cost,
		})
		anyOK = true
	}
	if !anyOK {
		return nil, false
	}
	return slots, true
}

// recognizeItems 识别装备区
//
// 本阶段简化为：OCR 整个区域后按行分割，每行一个 Item
// 未来可通过专用识别器或图标匹配提升精度
// Phase 14-D: 字段提示 "item_name"
func (p *TFTOCRProvider) recognizeItems(ctx context.Context, img image.Image, region image.Rectangle) ([]ItemSlot, bool) {
	text, ok := ocrRawText(ctx, p.engine, img, region, "item_name")
	if !ok || strings.TrimSpace(text) == "" {
		return nil, false
	}
	items := make([]ItemSlot, 0)
	for _, line := range strings.Split(text, "\n") {
		line = normalizeWhitespace(line)
		if line == "" {
			continue
		}
		// 简化：当前阶段不解析 count，Count = 1
		items = append(items, ItemSlot{
			ItemID: line,
			Count:  1,
		})
	}
	if len(items) == 0 {
		return nil, false
	}
	return items, true
}

// recognizeTraits 识别羁绊面板
//
// 文本格式：每行 "TraitName 2/4/6"
// Phase 14-D: 字段提示 "trait_line"
func (p *TFTOCRProvider) recognizeTraits(ctx context.Context, img image.Image, region image.Rectangle) ([]TraitState, bool) {
	text, ok := ocrRawText(ctx, p.engine, img, region, "trait_line")
	if !ok || strings.TrimSpace(text) == "" {
		return nil, false
	}
	traits := make([]TraitState, 0)
	for _, line := range strings.Split(text, "\n") {
		line = normalizeWhitespace(line)
		if line == "" {
			continue
		}
		active, style, ok := parseTraitLine(line)
		if !ok {
			continue
		}
		// Trait 名称 = 去掉所有数字（"2/4/6" 样式中的数字）后剩余的文本
		name := strings.TrimSpace(traitNumberPattern.ReplaceAllString(line, ""))
		// 清理可能残留的 "/" 字符
		name = strings.TrimSpace(strings.ReplaceAll(name, "/", ""))
		if name == "" {
			continue
		}
		traits = append(traits, TraitState{
			TraitID: name,
			Active:  active,
			Style:   style,
		})
	}
	if len(traits) == 0 {
		return nil, false
	}
	return traits, true
}

// recognizeAugments 识别强化符文候选（3 格）
//
// 返回 AugmentSlot 列表
// 文本格式："AugmentName" 或 "AugmentName | tier"
// Phase 14-D: 字段提示 "augment_with_tier"
func (p *TFTOCRProvider) recognizeAugments(ctx context.Context, img image.Image, cells []CellRegion) ([]AugmentSlot, bool) {
	if len(cells) == 0 {
		return nil, false
	}
	slots := make([]AugmentSlot, 0, len(cells))
	anyOK := false
	for _, c := range cells {
		text, ok := ocrRawText(ctx, p.engine, img, c.Rect, "augment_with_tier")
		if !ok || strings.TrimSpace(text) == "" {
			continue
		}
		name, tier, ok := parseAugmentCellText(text)
		if !ok {
			continue
		}
		slots = append(slots, AugmentSlot{
			AugmentID: name,
			Tier:      tier,
		})
		anyOK = true
	}
	if !anyOK {
		return nil, false
	}
	return slots, true
}

// recognizeCarousel 识别选秀候选
//
// 返回 *CarouselInfo（包含所有候选）
// Phase 14-D: 字段提示 "item_name"（选秀掉落通常是装备）
func (p *TFTOCRProvider) recognizeCarousel(ctx context.Context, img image.Image, cells []CellRegion) (*CarouselInfo, bool) {
	if len(cells) == 0 {
		return nil, false
	}
	offerings := make([]string, 0, len(cells))
	for _, c := range cells {
		text, ok := ocrRawText(ctx, p.engine, img, c.Rect, "item_name")
		if !ok || strings.TrimSpace(text) == "" {
			continue
		}
		offerings = append(offerings, normalizeWhitespace(text))
	}
	if len(offerings) == 0 {
		return nil, false
	}
	return &CarouselInfo{
		Offerings: offerings,
	}, true
}

// ocrParseField 泛型：单区域 OCR + 解析
//
// parser: 解析器（如 parseGold），签名 func(string) (T, bool)
// field: 字段名提示（Phase 14-D）；如 engine 实现 FieldAwareOCREngine 则会传给它
//
// 失败（区域空 / OCR 错误 / 可信度低 / 解析失败）→ 返回 (零值, false)
// 成功 → 返回 (val, true)
func ocrParseField[T any](
	ctx context.Context,
	engine OCREngine,
	img image.Image,
	region image.Rectangle,
	parser func(string) (T, bool),
	field string,
) (T, bool) {
	var zero T
	text, ok := ocrRawText(ctx, engine, img, region, field)
	if !ok {
		return zero, false
	}
	return parser(text)
}

// ocrRawText 单区域 OCR，返回原始文本（不解析）
//
// 用于多格子和复杂区域识别：调用方自己解析
// 失败（区域空 / OCR 错误 / 可信度低）→ 返回 ("", false)
//
// field: 字段名提示（Phase 14-D）；如 engine 实现 FieldAwareOCREngine 则会传给它
func ocrRawText(ctx context.Context, engine OCREngine, img image.Image, region image.Rectangle, field string) (string, bool) {
	if engine == nil {
		return "", false
	}
	bounds := img.Bounds()
	region = region.Intersect(bounds)
	if region.Empty() {
		return "", false
	}
	type subImager interface {
		SubImage(r image.Rectangle) image.Image
	}
	si, ok := img.(subImager)
	if !ok {
		return "", false
	}
	cropped := si.SubImage(region)
	var buf bytes.Buffer
	if err := png.Encode(&buf, cropped); err != nil {
		return "", false
	}

	// Phase 14-D：若引擎实现 FieldAwareOCREngine，优先用 RecognizeField
	// 现有引擎（Tesseract/Mock/Unavailable）不实现此接口 → 回退到 Recognize
	var (
		result OCRResult
		err    error
	)
	if fae, ok := engine.(FieldAwareOCREngine); ok {
		result, err = fae.RecognizeField(ctx, buf.Bytes(), "image/png", field)
	} else {
		result, err = engine.Recognize(ctx, buf.Bytes(), "image/png")
	}
	if err != nil {
		return "", false
	}
	if result.Confidence < 0.3 {
		return "", false
	}
	return result.Text, true
}

// parseCellText 解析 Board/Bench 格子文本
//
// 期望格式：
//   - "ChampionName" → (name, star=1, true)
//   - "ChampionName | 2" → (name, star=2, true)
//   - "ChampionName ★2" → (name, star=2, true)
//   - "ChampionName 2" → (name, star=2, true) // 空格分隔，末尾是数字（Phase 14-C）
//   - "Miss Fortune 2" → (name, star=2, true) // 多词英雄名（Phase 14-C）
//
// 解析失败返回 ("", 0, false)
func parseCellText(text string) (string, int, bool) {
	text = normalizeWhitespace(text)
	if text == "" {
		return "", 0, false
	}
	// 尝试以 "|" 分割
	if idx := strings.Index(text, "|"); idx > 0 {
		name := strings.TrimSpace(text[:idx])
		rest := strings.TrimSpace(text[idx+1:])
		if name == "" {
			return "", 0, false
		}
		star, ok := parseStar(rest)
		if !ok {
			// "|" 后不是数字，整体视为 name
			return name, 1, true
		}
		return name, star, true
	}
	// 尝试以 "★" 分割
	if idx := strings.Index(text, "★"); idx >= 0 {
		name := strings.TrimSpace(text[:idx])
		rest := strings.TrimSpace(text[idx+1:])
		if name == "" {
			return "", 0, false
		}
		star, ok := parseStar(rest)
		if !ok {
			return name, 1, true
		}
		return name, star, true
	}
	// 尝试 "Name 2" 格式：最后一个空格后是星级数字（Phase 14-C）
	// 例如 "Vayne 2" / "Miss Fortune 2" / "薇恩 2"
	if idx := strings.LastIndex(text, " "); idx > 0 {
		name := strings.TrimSpace(text[:idx])
		rest := strings.TrimSpace(text[idx+1:])
		if star, ok := parseStar(rest); ok {
			if _, ok := parseChampionName(name); ok {
				return name, star, true
			}
		}
	}
	// 整体作为 name，star=1
	if _, ok := parseChampionName(text); !ok {
		return "", 0, false
	}
	return text, 1, true
}

// parseShopCellText 解析 Shop 格子文本
//
// 期望格式：
//   - "ChampionName" → (name, cost=1, true)
//   - "ChampionName | 3" → (name, cost=3, true)
//   - "ChampionName 3" → (name, cost=3, true) // 空格分隔，末尾是费用（Phase 14-C）
//   - "Miss Fortune 3" → (name, cost=3, true) // 多词英雄名（Phase 14-C）
func parseShopCellText(text string) (string, int, bool) {
	text = normalizeWhitespace(text)
	if text == "" {
		return "", 0, false
	}
	if idx := strings.Index(text, "|"); idx > 0 {
		name := strings.TrimSpace(text[:idx])
		rest := strings.TrimSpace(text[idx+1:])
		if name == "" {
			return "", 0, false
		}
		cost, ok := parseShopCost(rest)
		if !ok {
			return name, 1, true
		}
		return name, cost, true
	}
	// 尝试 "Name 3" 格式：最后一个空格后是费用数字（Phase 14-C）
	// 例如 "Vayne 3" / "Miss Fortune 3" / "薇恩 5"
	if idx := strings.LastIndex(text, " "); idx > 0 {
		name := strings.TrimSpace(text[:idx])
		rest := strings.TrimSpace(text[idx+1:])
		if cost, ok := parseShopCost(rest); ok {
			if _, ok := parseChampionName(name); ok {
				return name, cost, true
			}
		}
	}
	if _, ok := parseChampionName(text); !ok {
		return "", 0, false
	}
	return text, 1, true
}

// parseAugmentCellText 解析 Augment 格子文本
//
// 期望格式：
//   - "AugmentName" → (name, "", true)（tier 留空）
//   - "AugmentName | silver" → (name, "silver", true)
func parseAugmentCellText(text string) (string, string, bool) {
	text = normalizeWhitespace(text)
	if text == "" {
		return "", "", false
	}
	if idx := strings.Index(text, "|"); idx > 0 {
		name := strings.TrimSpace(text[:idx])
		rest := strings.TrimSpace(text[idx+1:])
		if name == "" {
			return "", "", false
		}
		tier, ok := parseAugmentTier(rest)
		if !ok {
			return name, "", true
		}
		return name, tier, true
	}
	return text, "", true
}

// decodeImage 解码 JPEG 或 PNG
func decodeImage(data []byte, mime string) (image.Image, error) {
	if len(data) == 0 {
		return nil, errEmptyImage
	}
	// 优先按 MIME 选择解码器
	if mime == "image/png" {
		return png.Decode(bytes.NewReader(data))
	}
	// 默认按 JPEG 解码
	return jpeg.Decode(bytes.NewReader(data))
}

// --- 错误类型 ---

type providerError string

func (e providerError) Error() string { return string(e) }

const (
	errEmptyScreenshot   = providerError("empty screenshot in ContextInput")
	errNilEngine         = providerError("OCR engine is nil")
	errEmptyRegion       = providerError("empty region after intersection")
	errNoFieldRecognized = providerError("no field recognized from OCR")
)
