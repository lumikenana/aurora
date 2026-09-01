package tft

import "testing"

// TestChampionIDNormalizer_Empty 验证空字符串返回 Unknown
func TestChampionIDNormalizer_Empty(t *testing.T) {
	n := NewChampionIDNormalizer()
	if got := n.Normalize(""); got != UnknownID {
		t.Errorf("Normalize(\"\") = %q, want %q", got, UnknownID)
	}
	if got := n.Normalize("   "); got != UnknownID {
		t.Errorf("Normalize(\"   \") = %q, want %q", got, UnknownID)
	}
}

// TestChampionIDNormalizer_AlreadyTFTID 验证已经是 TFT 风格 ID 的字符串原样返回
func TestChampionIDNormalizer_AlreadyTFTID(t *testing.T) {
	n := NewChampionIDNormalizer()
	tests := []struct {
		input string
		want  string
	}{
		{"TFT18_Vayne", "TFT18_Vayne"},
		{"TFT18_Ahri", "TFT18_Ahri"},
		{"TFT18_Xayah", "TFT18_Xayah"},
	}
	for _, tt := range tests {
		if got := n.Normalize(tt.input); got != tt.want {
			t.Errorf("Normalize(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

// TestChampionIDNormalizer_RejectsCrossPatch 验证跨 patch 的 ID 不会被接受
//
// 关键不变量：当前 patch = "TFT18"，输入 "TFT17_Vayne"（同英雄跨 Set）
// 必须返回 Unknown（不能因名称相同而误识别为 TFT18 的英雄）
func TestChampionIDNormalizer_RejectsCrossPatch(t *testing.T) {
	n := NewChampionIDNormalizer() // patch = "TFT18"
	tests := []string{
		"TFT17_Vayne",     // 跨 Set
		"TFT19_Vayne",     // 未来 Set
		"TFTSet17_Vayne",  // 另一种格式
		"TFT16_Ahri",      // 旧 Set
	}
	for _, input := range tests {
		if got := n.Normalize(input); got != UnknownID {
			t.Errorf("Normalize(%q) = %q, want %q (cross-patch should be rejected)", input, got, UnknownID)
		}
	}
}

// TestChampionIDNormalizer_Unknown 验证未知名称返回 Unknown
func TestChampionIDNormalizer_Unknown(t *testing.T) {
	n := NewChampionIDNormalizer()
	// 没有任何注册的别名
	tests := []string{
		"RandomChampionName",
		"不存在的英雄",
		"SomeName",
	}
	for _, input := range tests {
		if got := n.Normalize(input); got != UnknownID {
			t.Errorf("Normalize(%q) = %q, want %q (unknown name)", input, got, UnknownID)
		}
	}
}

// TestChampionIDNormalizer_RegisterAlias 验证自定义别名注册后能被识别
func TestChampionIDNormalizer_RegisterAlias(t *testing.T) {
	n := NewChampionIDNormalizer()
	// 注册 "Vayne" → "TFT18_Vayne"
	n.RegisterAlias("Vayne", "TFT18_Vayne")
	n.RegisterAlias("VAYNE", "TFT18_Vayne")           // 大写
	n.RegisterAlias("  vayne  ", "TFT18_Vayne")       // 带空白

	tests := []struct {
		input string
		want  string
	}{
		{"Vayne", "TFT18_Vayne"},
		{"VAYNE", "TFT18_Vayne"},
		{"vayne", "TFT18_Vayne"},
		{"  vayne  ", "TFT18_Vayne"},
	}
	for _, tt := range tests {
		if got := n.Normalize(tt.input); got != tt.want {
			t.Errorf("Normalize(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

// TestChampionIDNormalizer_NoGuess 验证不做任何"近似匹配"
func TestChampionIDNormalizer_NoGuess(t *testing.T) {
	n := NewChampionIDNormalizer()
	n.RegisterAlias("Vayne", "TFT18_Vayne")

	// 仅注册了 Vayne，下列输入都不应被猜测
	tests := []string{
		"Vay",      // 短一字符
		"Vaynee",   // 长一字符
		"Vaynex",   // 近似但不同
		"Xayne",    // 近似但不同
	}
	for _, input := range tests {
		if got := n.Normalize(input); got != UnknownID {
			t.Errorf("Normalize(%q) = %q, want %q (must not guess)", input, got, UnknownID)
		}
	}
}

// TestChampionIDNormalizer_WithPatch 验证切换 patch 前缀
func TestChampionIDNormalizer_WithPatch(t *testing.T) {
	n := NewChampionIDNormalizer().WithPatch("TFT19")

	// 当前 patch = "TFT19"，TFT18_Vayne 应该被拒绝
	if got := n.Normalize("TFT18_Vayne"); got != UnknownID {
		t.Errorf("Normalize(TFT18_Vayne) with patch=TFT19 = %q, want %q", got, UnknownID)
	}
	// TFT19_Vayne 接受
	if got := n.Normalize("TFT19_Vayne"); got != "TFT19_Vayne" {
		t.Errorf("Normalize(TFT19_Vayne) with patch=TFT19 = %q, want TFT19_Vayne", got)
	}
}

// TestIsTFTID 验证 TFT ID 格式检测
//
// TFT ID 必须以 "TFT" 开头（大小写不敏感）后接下划线 + 名称
// 非 "TFT" 前缀即使有下划线也不视为 TFT ID
func TestIsTFTID(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"TFT18_Vayne", true},
		{"TFTSet18_Vayne", true},
		{"tft18_vayne", true},   // 大小写不敏感
		{"TFT_18", true},
		{"TFT18_", false},       // 缺少 name
		{"_Vayne", false},       // 缺少 prefix
		{"Vayne", false},        // 没有下划线
		{"ABC18_Vayne", false},  // 非 TFT 前缀拒绝
		{"LOL_Ahri", false},     // 其他游戏 ID 不接受
		{"", false},
		{"a_b", false},          // 不是 TFT 开头
	}
	for _, tt := range tests {
		if got := isTFTID(tt.input); got != tt.want {
			t.Errorf("isTFTID(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}
