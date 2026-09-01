// Package tft - TFT 数据类型的对外别名（Phase 17-A）
//
// Phase 17-A 把 TFTGameContext 等数据类型从 package tft 移到独立子包
// internal/game/plugins/tft/tfttypes/，原因：
//   - tft 父包要 import composition / grounder / economy / equipment /
//     preparation / gamestate 等子包（用于 prompt 渲染）
//   - grounder 等子包需要访问 *TFTGameContext
//   - 父子互引 → import cycle
//   - 把数据类型下沉到 tfttypes 子包后，父子均依赖 tfttypes，
//     但互不依赖 → 无 cycle
//
// 为了不破坏 package tft 内部其它文件对这些类型的引用（tft.go / ocr.go /
// regions.go / prompt.go 等），本文件用 type alias 把 tfttypes 类型
// "再导出"成 tft 包内的同名类型。
//
// 对外契约不变：
//   - 调用方仍可写 tft.TFTGameContext（实际指向 tfttypes.TFTGameContext）
//   - Agent Core 也不需要 import tfttypes（保持 #2.2 边界）
//
// 注意：仅做类型别名，不引入额外方法或行为；类型身份（reflect / type
// switch）保持一致——`var x any = tft.TFTGameContext{}` 的
// reflect.Type 仍为 tfttypes.TFTGameContext。
package tft

import (
	"github.com/lipipi/aurora/internal/game/plugins/tft/tfttypes"
)

// TFTGameContext 别名 → tfttypes.TFTGameContext
type TFTGameContext = tfttypes.TFTGameContext

// BoardSlot 别名 → tfttypes.BoardSlot
type BoardSlot = tfttypes.BoardSlot

// BenchSlot 别名 → tfttypes.BenchSlot
type BenchSlot = tfttypes.BenchSlot

// ShopSlot 别名 → tfttypes.ShopSlot
type ShopSlot = tfttypes.ShopSlot

// ItemSlot 别名 → tfttypes.ItemSlot
type ItemSlot = tfttypes.ItemSlot

// AugmentSlot 别名 → tfttypes.AugmentSlot
type AugmentSlot = tfttypes.AugmentSlot

// TraitState 别名 → tfttypes.TraitState
type TraitState = tfttypes.TraitState

// CarouselInfo 别名 → tfttypes.CarouselInfo
type CarouselInfo = tfttypes.CarouselInfo

// Opponent 别名 → tfttypes.Opponent
type Opponent = tfttypes.Opponent
