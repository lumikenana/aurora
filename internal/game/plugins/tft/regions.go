// Package tft - TFT UI 区域定义
//
// Phase 10-7 范围：
//   - 定义参考分辨率（reference 1920x1080）下 Gold / HP / Level / Stage 的位置
//   - 提供按实际窗口尺寸缩放的能力
//   - 区域坐标不直接散落在 OCR 代码里
//
// Phase 10-8 范围：
//   - 在保留 Phase 10-7 Player 区域扁平字段的基础上，
//     增加 XP / Board / Bench / Shop / Items / Traits / Augments / Carousel 区域
//   - 棋盘 / 备战 / 商店 / 强化符文 / 选秀按"多格子"模型定义
//   - 继续使用 ScaleRegions 缩放（独立 X/Y 缩放因子）
//
// 重要说明：
//   本阶段的区域坐标是基于 TFT 客户端常见布局的"占位值"。
//   由于 TFT 客户端 UI 可能随时变化，实际 E2E 可能需要微调。
//   坐标基于"参考分辨率"1920x1080，缩放通过 ScaleRegion / ScaleBoard 等完成。
package tft

import "image"

// BoardSize / BenchSize / ShopSize / AugmentSlotCount TFT UI 固定数量
const (
	// BoardRows 棋盘行数
	BoardRows = 4
	// BoardCols 棋盘列数
	BoardCols = 7
	// BoardSize 棋盘格子数（4*7 = 28，Position 0~27）
	BoardSize = BoardRows * BoardCols
	// BenchSize 备战席数量
	BenchSize = 9
	// ShopSize 商店数量
	ShopSize = 5
	// AugmentSlotCount 强化符文候选数量
	AugmentSlotCount = 3
)

// CellRegion 通用格子区域（棋盘/备战/商店/强化符文等使用）
//
// Position / Slot 由调用方根据上下文解释。
type CellRegion struct {
	// Position 格子编号（棋盘 0~27 / 备战 0~8 / 商店 0~4 / 强化符文 0~2）
	Position int
	// Rect 区域矩形
	Rect image.Rectangle
}

// TFTScreenRegions TFT UI 区域集合（参考分辨率下的坐标）
//
// 坐标系：左上角 (0, 0)，右下方为正方向
// 区域定义的是"包含目标内容的最小矩形"
//
// 设计：
//   - Player 区域（Gold/HP/Level/Stage/XP）保持扁平字段（Phase 10-7 兼容）
//   - Board / Bench / Shop / Augments / Carousel 是多格子列表（Phase 10-8）
//   - Items / Traits 是单一矩形（Phase 10-8）
type TFTScreenRegions struct {
	// ---- Player (Phase 10-7) ----

	// Gold 金币数字（屏幕左下区域）
	Gold image.Rectangle
	// HP 玩家血量（屏幕左下区域，金币上方）
	HP image.Rectangle
	// Level 玩家等级（屏幕左下区域，HP 旁）
	Level image.Rectangle
	// Stage 阶段（屏幕顶部中央）
	Stage image.Rectangle

	// ---- Player (Phase 10-8) ----

	// XP 当前 XP（Level 附近）
	XP image.Rectangle

	// ---- 棋盘 / 备战 / 商店 (Phase 10-8) ----

	// Board 棋盘格子（28 个，按 4 行 × 7 列排列；index = row*BoardCols+col）
	// 长度必须为 BoardSize，否则 ScaleBoard 会返回原 slice
	Board []CellRegion
	// Bench 备战席（9 个）
	// 长度必须为 BenchSize，否则 ScaleBench 会返回原 slice
	Bench []CellRegion
	// Shop 商店（5 个）
	// 长度必须为 ShopSize，否则 ScaleShop 会返回原 slice
	Shop []CellRegion

	// ---- 状态 (Phase 10-8) ----

	// Items 玩家装备 / 散件库存区域（屏幕中部下方或左侧）
	Items image.Rectangle
	// Traits 羁绊面板区域（屏幕左侧）
	Traits image.Rectangle
	// Augments 强化符文候选（3 个，draft 阶段可见）
	// 长度必须为 AugmentSlotCount
	Augments []CellRegion
	// Carousel 选秀候选（数量不固定，通常 5~8 个；本阶段用 8 占位）
	Carousel []CellRegion
}

// ReferenceResolution 参考分辨率（TFT 客户端默认 1920x1080）
var ReferenceResolution = image.Point{X: 1920, Y: 1080}

// DefaultTFTScreenRegions 参考分辨率下的默认区域
//
// 注：坐标为 Phase 10-7/10-8 占位值，需要真实 TFT 客户端调优
//
//   - Gold:   左下角金币图标右侧数字（约 (155, 1020) - (210, 1060)）
//   - HP:     左下角 HP 数字（约 (60, 1040) - (110, 1075)）
//   - Level:  HP 旁等级文字（约 (60, 1010) - (110, 1035)）
//   - Stage:  顶部中央阶段文字（约 (820, 25) - (1100, 60)）
//   - XP:     Level 下方 XP 条上的数字（约 (60, 985) - (110, 1005)）
//   - Board:  棋盘 4×7 网格（约 (570, 480) 起）
//   - Bench:  备战席 9 格（屏幕中下方，约 (570, 880) 起）
//   - Shop:   商店 5 格（屏幕底部，约 (570, 1000) 起）
//   - Items:  玩家装备区（屏幕左中部，约 (240, 600) - (520, 800)）
//   - Traits: 羁绊面板（屏幕左侧，约 (20, 200) - (220, 800)）
//   - Augments: 3 个强化符文候选（屏幕中央偏上，约 (520, 200) 起）
//   - Carousel: 8 个选秀候选（屏幕中央，约 (220, 400) 起）
var DefaultTFTScreenRegions = TFTScreenRegions{
	Gold:  image.Rect(155, 1020, 210, 1060),
	HP:    image.Rect(60, 1040, 110, 1075),
	Level: image.Rect(60, 1010, 110, 1035),
	Stage: image.Rect(820, 25, 1100, 60),
	XP:    image.Rect(60, 985, 110, 1005),

	Board:     defaultBoardCells(),
	Bench:     defaultBenchCells(),
	Shop:      defaultShopCells(),
	Items:     image.Rect(240, 600, 520, 800),
	Traits:    image.Rect(20, 200, 220, 800),
	Augments:  defaultAugmentCells(),
	Carousel:  defaultCarouselCells(),
}

// defaultBoardCells 生成参考分辨率下的棋盘格子区域
//
// 4 行 × 7 列；cell 宽 ~64, 高 ~64；起点 (570, 480)
func defaultBoardCells() []CellRegion {
	cells := make([]CellRegion, 0, BoardSize)
	const cellW, cellH = 64, 64
	const originX, originY = 570, 480
	for r := 0; r < BoardRows; r++ {
		for c := 0; c < BoardCols; c++ {
			cells = append(cells, CellRegion{
				Position: r*BoardCols + c,
				Rect: image.Rect(
					originX+c*cellW,
					originY+r*cellH,
					originX+(c+1)*cellW,
					originY+(r+1)*cellH,
				),
			})
		}
	}
	return cells
}

// defaultBenchCells 生成参考分辨率下的备战席格子区域
//
// 9 格横向排列；cell 宽 ~64, 高 ~64；起点 (570, 880)
func defaultBenchCells() []CellRegion {
	cells := make([]CellRegion, 0, BenchSize)
	const cellW, cellH = 64, 64
	const originX, originY = 570, 880
	for i := 0; i < BenchSize; i++ {
		cells = append(cells, CellRegion{
			Position: i,
			Rect: image.Rect(
				originX+i*cellW,
				originY,
				originX+(i+1)*cellW,
				originY+cellH,
			),
		})
	}
	return cells
}

// defaultShopCells 生成参考分辨率下的商店格子区域
//
// 5 格横向排列；cell 宽 ~64, 高 ~64；起点 (570, 1000)
func defaultShopCells() []CellRegion {
	cells := make([]CellRegion, 0, ShopSize)
	const cellW, cellH = 64, 64
	const originX, originY = 570, 1000
	for i := 0; i < ShopSize; i++ {
		cells = append(cells, CellRegion{
			Position: i,
			Rect: image.Rect(
				originX+i*cellW,
				originY,
				originX+(i+1)*cellW,
				originY+cellH,
			),
		})
	}
	return cells
}

// defaultAugmentCells 生成参考分辨率下的强化符文候选区域
//
// 3 个候选横向排列；cell 宽 ~150, 高 ~150；起点 (520, 200)
func defaultAugmentCells() []CellRegion {
	cells := make([]CellRegion, 0, AugmentSlotCount)
	const cellW, cellH = 150, 150
	const originX, originY = 520, 200
	for i := 0; i < AugmentSlotCount; i++ {
		cells = append(cells, CellRegion{
			Position: i,
			Rect: image.Rect(
				originX+i*(cellW+20),
				originY,
				originX+i*(cellW+20)+cellW,
				originY+cellH,
			),
		})
	}
	return cells
}

// defaultCarouselCells 生成参考分辨率下的选秀候选区域
//
// 8 个候选横向排列；cell 宽 ~150, 高 ~150；起点 (220, 400)
func defaultCarouselCells() []CellRegion {
	const carouselCount = 8
	cells := make([]CellRegion, 0, carouselCount)
	const cellW, cellH = 150, 150
	const originX, originY = 220, 400
	for i := 0; i < carouselCount; i++ {
		cells = append(cells, CellRegion{
			Position: i,
			Rect: image.Rect(
				originX+i*(cellW+20),
				originY,
				originX+i*(cellW+20)+cellW,
				originY+cellH,
			),
		})
	}
	return cells
}

// ScaleRegion 按比例缩放单个矩形区域
//
// 从 reference 分辨率缩放到 actual 分辨率。
// 缩放采用"独立 X / Y 缩放因子"，允许非等比缩放。
// 若 reference 或 actual 任意维度为 0，返回原区域（不缩放）。
func ScaleRegion(r image.Rectangle, reference, actual image.Point) image.Rectangle {
	if reference.X <= 0 || reference.Y <= 0 || actual.X <= 0 || actual.Y <= 0 {
		return r
	}
	scaleX := float64(actual.X) / float64(reference.X)
	scaleY := float64(actual.Y) / float64(reference.Y)
	return image.Rect(
		int(float64(r.Min.X)*scaleX),
		int(float64(r.Min.Y)*scaleY),
		int(float64(r.Max.X)*scaleX),
		int(float64(r.Max.Y)*scaleY),
	)
}

// ScaleCells 按比例缩放格子列表
//
// 若 cells 为空 / reference / actual 任意维度为 0，返回原 slice（不缩放）。
// 不修改入参，返回新 slice。
func ScaleCells(cells []CellRegion, reference, actual image.Point) []CellRegion {
	if len(cells) == 0 || reference.X <= 0 || reference.Y <= 0 || actual.X <= 0 || actual.Y <= 0 {
		return cells
	}
	out := make([]CellRegion, len(cells))
	for i, c := range cells {
		out[i] = CellRegion{
			Position: c.Position,
			Rect:     ScaleRegion(c.Rect, reference, actual),
		}
	}
	return out
}

// ScaleRegions 缩放整个区域集合
//
// 包含 Phase 10-7 的 Player 字段 + Phase 10-8 的 XP / Board / Bench / Shop /
// Augments / Carousel 等所有字段。
func ScaleRegions(regions TFTScreenRegions, reference, actual image.Point) TFTScreenRegions {
	return TFTScreenRegions{
		Gold:     ScaleRegion(regions.Gold, reference, actual),
		HP:       ScaleRegion(regions.HP, reference, actual),
		Level:    ScaleRegion(regions.Level, reference, actual),
		Stage:    ScaleRegion(regions.Stage, reference, actual),
		XP:       ScaleRegion(regions.XP, reference, actual),
		Board:    ScaleCells(regions.Board, reference, actual),
		Bench:    ScaleCells(regions.Bench, reference, actual),
		Shop:     ScaleCells(regions.Shop, reference, actual),
		Items:    ScaleRegion(regions.Items, reference, actual),
		Traits:   ScaleRegion(regions.Traits, reference, actual),
		Augments: ScaleCells(regions.Augments, reference, actual),
		Carousel: ScaleCells(regions.Carousel, reference, actual),
	}
}
