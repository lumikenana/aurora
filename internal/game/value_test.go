package game

import "testing"

// TestValue_KnownZeroVsUnknown 验证 Known(0) 与 Unknown[int]() 可以区分
// 这是 Phase 10-6 的核心不变量：unknown ≠ zero
func TestValue_KnownZeroVsUnknown(t *testing.T) {
	zero := Known(0)
	if !zero.IsKnown() {
		t.Fatal("Known(0) should be considered known")
	}
	if zero.Val != 0 {
		t.Fatalf("Known(0) Val should be 0, got %d", zero.Val)
	}

	unk := Unknown[int]()
	if unk.IsKnown() {
		t.Fatal("Unknown[int]() should NOT be considered known")
	}
	if unk.Val != 0 {
		t.Fatalf("Unknown[int]() Val should be zero value, got %d", unk.Val)
	}

	// Present 字段不同
	if zero.Present == unk.Present {
		t.Fatal("Known and Unknown must have different Present values")
	}
}

// TestValue_KnownEmptyStringVsUnknown 验证 Known("") 与 Unknown[string]() 可以区分
func TestValue_KnownEmptyStringVsUnknown(t *testing.T) {
	empty := Known("")
	if !empty.IsKnown() {
		t.Fatal("Known(\"\") should be considered known")
	}

	unk := Unknown[string]()
	if unk.IsKnown() {
		t.Fatal("Unknown[string]() should NOT be considered known")
	}
}

// TestValue_Detected 验证 Detected 构造带来源/可信度
func TestValue_Detected(t *testing.T) {
	v := Detected(42, SourceVision, 0.95)
	if !v.IsKnown() {
		t.Fatal("Detected should be known")
	}
	if v.Val != 42 {
		t.Fatalf("Val mismatch: %d", v.Val)
	}
	if v.Source != SourceVision {
		t.Fatalf("Source mismatch: %s", v.Source)
	}
	if v.Confidence != 0.95 {
		t.Fatalf("Confidence mismatch: %f", v.Confidence)
	}
}

// TestValue_OrZero 验证未识别时返回零值
func TestValue_OrZero(t *testing.T) {
	unk := Unknown[int]()
	if got := unk.OrZero(); got != 0 {
		t.Fatalf("Unknown.OrZero() should be 0, got %d", got)
	}

	known := Known(7)
	if got := known.OrZero(); got != 7 {
		t.Fatalf("Known.OrZero() should return Val, got %d", got)
	}
}

// TestValue_OrFallback 验证未识别时返回 fallback
func TestValue_OrFallback(t *testing.T) {
	unk := Unknown[int]()
	if got := unk.OrFallback(99); got != 99 {
		t.Fatalf("Unknown.OrFallback(99) should be 99, got %d", got)
	}

	known := Known(7)
	if got := known.OrFallback(99); got != 7 {
		t.Fatalf("Known(7).OrFallback(99) should be 7, got %d", got)
	}
}
