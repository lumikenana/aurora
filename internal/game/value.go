// Package game - 通用可选值容器
//
// 设计目标：让 "unknown" 和 "已识别为 0 / 空字符串 / nil" 是两个不同的状态。
// 旧实现用零值表达未知，导致 Gold=0、Level=0、HP=0 等合法值与"未识别"无法区分。
// Value[T] 用 Present 字段显式表达"是否已识别"。
package game

import "time"

// Value 可选值容器
//   - Present=false：未识别 / 数据缺失 / 阶段不允许
//   - Present=true：Val 是有效值
//
// 0 / "" / nil 本身可以是合法值，请使用 Known(...) 显式构造。
type Value[T any] struct {
	// Val 当前值（仅在 Present=true 时有效）
	Val T
	// Present 是否已识别（false = unknown）
	Present bool
	// Source 数据来源（窗口检测 / 视觉 / OCR / 游戏 API / 手动）
	Source ContextSource
	// Confidence 可信度（0.0 ~ 1.0）
	Confidence float64
	// UpdatedAt 最近一次更新时间
	UpdatedAt time.Time
}

// Known 构造一个"已识别且完全可信"的值（通常来自窗口检测等基础设施）
func Known[T any](v T) Value[T] {
	return Value[T]{
		Val:        v,
		Present:    true,
		Source:     SourceWindow,
		Confidence: 1.0,
		UpdatedAt:  time.Now(),
	}
}

// Detected 构造一个"已识别但需要标注来源和可信度"的值（来自 Vision / OCR / GameAPI）
func Detected[T any](v T, src ContextSource, confidence float64) Value[T] {
	return Value[T]{
		Val:        v,
		Present:    true,
		Source:     src,
		Confidence: confidence,
		UpdatedAt:  time.Now(),
	}
}

// Unknown 构造一个"未识别"的零值
func Unknown[T any]() Value[T] {
	var zero T
	return Value[T]{
		Val:     zero,
		Present: false,
	}
}

// IsKnown 是否已识别
func (v Value[T]) IsKnown() bool {
	return v.Present
}

// OrZero 未识别时返回类型零值（注意：与 Unknown 仍然不同——Present 字段不同）
func (v Value[T]) OrZero() T {
	if !v.Present {
		var zero T
		return zero
	}
	return v.Val
}

// OrFallback 未识别时返回 fallback
func (v Value[T]) OrFallback(def T) T {
	if !v.Present {
		return def
	}
	return v.Val
}
