package tft

import "testing"

func TestParseGold(t *testing.T) {
	tests := []struct {
		input   string
		want    int
		success bool
	}{
		{"42", 42, true},
		{"0", 0, true},
		{" 42 ", 42, true},
		{"Gold: 42", 42, true},
		{"O42", 42, true}, // 字母 O 误识别
		{"999", 999, true},
		{"abc", 0, false},
		{"", 0, false},
		{"1000", 0, false}, // 超过 GoldMax
		{"-1", 0, false},   // 负数
	}
	for _, tt := range tests {
		got, ok := parseGold(tt.input)
		if ok != tt.success {
			t.Errorf("parseGold(%q) ok=%v, want %v", tt.input, ok, tt.success)
		}
		if ok && got != tt.want {
			t.Errorf("parseGold(%q) = %d, want %d", tt.input, got, tt.want)
		}
	}
}

func TestParseHP(t *testing.T) {
	tests := []struct {
		input   string
		want    int
		success bool
	}{
		{"67", 67, true},
		{"0", 0, true},   // 0 = 阵亡，合法
		{"100", 100, true},
		{"HP: 67", 67, true},
		{"abc", 0, false},
		{"101", 0, false}, // 超过 HPMax
		{"-5", 0, false},
		{"", 0, false},
	}
	for _, tt := range tests {
		got, ok := parseHP(tt.input)
		if ok != tt.success {
			t.Errorf("parseHP(%q) ok=%v, want %v", tt.input, ok, tt.success)
		}
		if ok && got != tt.want {
			t.Errorf("parseHP(%q) = %d, want %d", tt.input, got, tt.want)
		}
	}
}

func TestParseLevel(t *testing.T) {
	tests := []struct {
		input   string
		want    int
		success bool
	}{
		{"7", 7, true},
		{"1", 1, true},
		{"11", 11, true},
		{"Level 7", 7, true},
		{"Lv 8", 8, true},
		{"0", 0, false},  // 0 < LevelMin
		{"12", 0, false}, // 12 > LevelMax
		{"abc", 0, false},
		{"", 0, false},
	}
	for _, tt := range tests {
		got, ok := parseLevel(tt.input)
		if ok != tt.success {
			t.Errorf("parseLevel(%q) ok=%v, want %v", tt.input, ok, tt.success)
		}
		if ok && got != tt.want {
			t.Errorf("parseLevel(%q) = %d, want %d", tt.input, got, tt.want)
		}
	}
}

func TestParseStage(t *testing.T) {
	tests := []struct {
		input   string
		want    string
		success bool
	}{
		{"4-2", "4-2", true},
		{"4 - 2", "4-2", true},
		{"Stage 4-2", "4-2", true},
		{"1-1", "1-1", true},
		{"10-9", "10-9", true},
		{"abc", "", false},
		{"4", "", false},      // 缺子阶段
		{"4-", "", false},     // 缺数字
		{"0-1", "", false},    // 主阶段 < 1
		{"11-1", "", false},   // 主阶段 > 10
		{"1-10", "", false},   // 子阶段 > 9
		{"1-0", "", false},    // 子阶段 < 1
		{"", "", false},
	}
	for _, tt := range tests {
		got, ok := parseStage(tt.input)
		if ok != tt.success {
			t.Errorf("parseStage(%q) ok=%v, want %v", tt.input, ok, tt.success)
		}
		if ok && got != tt.want {
			t.Errorf("parseStage(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

// TestParseZeroIsValid 验证 0 是合法值（HP=0 阵亡 / Gold=0 破产）
func TestParseZeroIsValid(t *testing.T) {
	if _, ok := parseGold("0"); !ok {
		t.Error("parseGold(\"0\") should be valid (0 gold is legal)")
	}
	if _, ok := parseHP("0"); !ok {
		t.Error("parseHP(\"0\") should be valid (0 HP = eliminated is legal)")
	}
}

func TestExtractDigits(t *testing.T) {
	tests := []struct {
		input, want string
	}{
		{"42", "42"},
		{"Gold: 42", "42"},
		{"O42", "42"},
		{"4 2", "42"},
		{"abc", ""},
		{"", ""},
		{"  42  ", "42"},
	}
	for _, tt := range tests {
		if got := extractDigits(tt.input); got != tt.want {
			t.Errorf("extractDigits(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

// --- Phase 10-8 新增解析器 ---

func TestParseXP(t *testing.T) {
	tests := []struct {
		input   string
		want    int
		success bool
	}{
		{"0", 0, true},
		{"42", 42, true},
		{" 100 ", 100, true},
		{"200", 200, true},
		{"201", 0, false}, // 超过 XPMax
		{"-1", 0, false},
		{"abc", 0, false},
		{"", 0, false},
	}
	for _, tt := range tests {
		got, ok := parseXP(tt.input)
		if ok != tt.success {
			t.Errorf("parseXP(%q) ok=%v, want %v", tt.input, ok, tt.success)
		}
		if ok && got != tt.want {
			t.Errorf("parseXP(%q) = %d, want %d", tt.input, got, tt.want)
		}
	}
}

func TestParseStar(t *testing.T) {
	tests := []struct {
		input   string
		want    int
		success bool
	}{
		{"1", 1, true},
		{"2", 2, true},
		{"3", 3, true},
		{"0", 0, false}, // < StarMin
		{"4", 0, false}, // > StarMax
		{"abc", 0, false},
		{"", 0, false},
	}
	for _, tt := range tests {
		got, ok := parseStar(tt.input)
		if ok != tt.success {
			t.Errorf("parseStar(%q) ok=%v, want %v", tt.input, ok, tt.success)
		}
		if ok && got != tt.want {
			t.Errorf("parseStar(%q) = %d, want %d", tt.input, got, tt.want)
		}
	}
}

func TestParseShopCost(t *testing.T) {
	tests := []struct {
		input   string
		want    int
		success bool
	}{
		{"1", 1, true},
		{"5", 5, true},
		{"0", 0, false},  // < 1
		{"6", 0, false},  // > ShopCostMax
		{"abc", 0, false},
		{"", 0, false},
	}
	for _, tt := range tests {
		got, ok := parseShopCost(tt.input)
		if ok != tt.success {
			t.Errorf("parseShopCost(%q) ok=%v, want %v", tt.input, ok, tt.success)
		}
		if ok && got != tt.want {
			t.Errorf("parseShopCost(%q) = %d, want %d", tt.input, got, tt.want)
		}
	}
}

func TestParseChampionName(t *testing.T) {
	tests := []struct {
		input   string
		want    string
		success bool
	}{
		{"Vayne", "Vayne", true},
		{"  Ahri  ", "Ahri", true},
		{"薇恩", "薇恩", true},
		{"", "", false},
		{"   ", "", false},
		{"123", "", false},  // 纯数字
		{"42", "", false},   // 纯数字
	}
	for _, tt := range tests {
		got, ok := parseChampionName(tt.input)
		if ok != tt.success {
			t.Errorf("parseChampionName(%q) ok=%v, want %v", tt.input, ok, tt.success)
		}
		if ok && got != tt.want {
			t.Errorf("parseChampionName(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestParseAugmentTier(t *testing.T) {
	tests := []struct {
		input   string
		want    string
		success bool
	}{
		{"silver", "silver", true},
		{"Silver", "silver", true},
		{"SILVER", "silver", true},
		{"gold", "gold", true},
		{"Gold", "gold", true},
		{"prismatic", "prismatic", true},
		{"prism", "prismatic", true},
		{"Prismatic", "prismatic", true},
		{"bronze", "", false},
		{"", "", false},
		{"abc", "", false},
	}
	for _, tt := range tests {
		got, ok := parseAugmentTier(tt.input)
		if ok != tt.success {
			t.Errorf("parseAugmentTier(%q) ok=%v, want %v", tt.input, ok, tt.success)
		}
		if ok && got != tt.want {
			t.Errorf("parseAugmentTier(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestParseTraitLine(t *testing.T) {
	tests := []struct {
		input      string
		wantActive int
		wantStyle  int
		success    bool
	}{
		{"Arcanist 2/4", 2, 4, true},
		{"Arcanist 2/4/6", 2, 6, true}, // 阶数取最后
		{"Arcanist 4 / 6", 4, 6, true},
		{"Frontline 1/2/3/4", 1, 4, true},
		{"NoNumbers", 0, 0, false},
		{"", 0, 0, false},
		{"abc 2 def", 0, 0, false}, // 阶数格式错误
	}
	for _, tt := range tests {
		a, s, ok := parseTraitLine(tt.input)
		if ok != tt.success {
			t.Errorf("parseTraitLine(%q) ok=%v, want %v", tt.input, ok, tt.success)
		}
		if ok {
			if a != tt.wantActive {
				t.Errorf("parseTraitLine(%q) active=%d, want %d", tt.input, a, tt.wantActive)
			}
			if s != tt.wantStyle {
				t.Errorf("parseTraitLine(%q) style=%d, want %d", tt.input, s, tt.wantStyle)
			}
		}
	}
}

// TestParseCellText 测试 Board/Bench 格子文本解析
func TestParseCellText(t *testing.T) {
	tests := []struct {
		input     string
		wantName  string
		wantStar  int
		success   bool
	}{
		{"Vayne", "Vayne", 1, true},
		{"Vayne | 2", "Vayne", 2, true},
		{"Vayne | 3", "Vayne", 3, true},
		{"Vayne ★2", "Vayne", 2, true},
		{"  Vayne  ", "Vayne", 1, true},
		// Phase 14-C 新增 "Name 2" 格式
		{"Vayne 2", "Vayne", 2, true},
		{"Vayne 3", "Vayne", 3, true},
		{"薇恩 2", "薇恩", 2, true},
		// 多词英雄名（Phase 14-C 关键场景：LastIndex 空格 + 末尾数字）
		{"Miss Fortune 2", "Miss Fortune", 2, true},
		{"Aurelion Sol 3", "Aurelion Sol", 3, true},
		// 失败情况
		{"", "", 0, false},
		{"   ", "", 0, false},
		// 末尾数字越界（>3 不是合法星级）→ 整体当 name, star=1
		{"Vayne 4", "Vayne 4", 1, true},
		// 末尾不是纯数字
		{"Vayne abc", "Vayne abc", 1, true},
	}
	for _, tt := range tests {
		name, star, ok := parseCellText(tt.input)
		if ok != tt.success {
			t.Errorf("parseCellText(%q) ok=%v, want %v", tt.input, ok, tt.success)
		}
		if ok {
			if name != tt.wantName {
				t.Errorf("parseCellText(%q) name=%q, want %q", tt.input, name, tt.wantName)
			}
			if star != tt.wantStar {
				t.Errorf("parseCellText(%q) star=%d, want %d", tt.input, star, tt.wantStar)
			}
		}
	}
}

// TestParseShopCellText 测试 Shop 格子文本解析
func TestParseShopCellText(t *testing.T) {
	tests := []struct {
		input    string
		wantName string
		wantCost int
		success  bool
	}{
		{"Vayne", "Vayne", 1, true},
		{"Vayne | 3", "Vayne", 3, true},
		{"Vayne | 5", "Vayne", 5, true},
		// Phase 14-C 新增 "Name Cost" 格式
		{"Vayne 3", "Vayne", 3, true},
		{"Vayne 5", "Vayne", 5, true},
		{"Vayne 1", "Vayne", 1, true},
		{"薇恩 5", "薇恩", 5, true},
		// 多词英雄名
		{"Miss Fortune 3", "Miss Fortune", 3, true},
		{"Aurelion Sol 5", "Aurelion Sol", 5, true},
		// 失败情况
		{"", "", 0, false},
		{"   ", "", 0, false},
		// 末尾数字越界（>5 不是合法费用）→ 整体当 name, cost=1
		{"Vayne 6", "Vayne 6", 1, true},
		// 末尾不是纯数字
		{"Vayne abc", "Vayne abc", 1, true},
	}
	for _, tt := range tests {
		name, cost, ok := parseShopCellText(tt.input)
		if ok != tt.success {
			t.Errorf("parseShopCellText(%q) ok=%v, want %v", tt.input, ok, tt.success)
		}
		if ok {
			if name != tt.wantName {
				t.Errorf("parseShopCellText(%q) name=%q, want %q", tt.input, name, tt.wantName)
			}
			if cost != tt.wantCost {
				t.Errorf("parseShopCellText(%q) cost=%d, want %d", tt.input, cost, tt.wantCost)
			}
		}
	}
}

// TestParseAugmentCellText 测试 Augment 格子文本解析
func TestParseAugmentCellText(t *testing.T) {
	tests := []struct {
		input     string
		wantName  string
		wantTier  string
		success   bool
	}{
		{"Combat Medic", "Combat Medic", "", true},
		{"Combat Medic | silver", "Combat Medic", "silver", true},
		{"Combat Medic | gold", "Combat Medic", "gold", true},
		{"Combat Medic | prismatic", "Combat Medic", "prismatic", true},
		{"", "", "", false},
	}
	for _, tt := range tests {
		name, tier, ok := parseAugmentCellText(tt.input)
		if ok != tt.success {
			t.Errorf("parseAugmentCellText(%q) ok=%v, want %v", tt.input, ok, tt.success)
		}
		if ok {
			if name != tt.wantName {
				t.Errorf("parseAugmentCellText(%q) name=%q, want %q", tt.input, name, tt.wantName)
			}
			if tier != tt.wantTier {
				t.Errorf("parseAugmentCellText(%q) tier=%q, want %q", tt.input, tier, tt.wantTier)
			}
		}
	}
}

// TestParseBoardCellText 测试棋盘格子文本解析（英雄+星级+装备）
func TestParseBoardCellText(t *testing.T) {
	tests := []struct {
		input      string
		wantName   string
		wantStar   int
		wantItems  []string
		wantOK     bool
	}{
		// 最简单情况：只有英雄
		{"Vayne", "Vayne", 1, []string{}, true},
		// 英雄+星级
		{"Vayne | 2", "Vayne", 2, []string{}, true},
		// 英雄+星级+单个装备 '/'
		{"Vayne | 2 | IE", "Vayne", 2, []string{"IE"}, true},
		// 英雄+星级+多个装备 '/'
		{"Vayne | 2 | IE/Rageblade", "Vayne", 2, []string{"IE", "Rageblade"}, true},
		// 英雄+星级+多个装备空格分隔
		{"Vayne | 2 | IE Rageblade", "Vayne", 2, []string{"IE", "Rageblade"}, true},
		// 英雄+星级+多个装备 ' / ' 带空格
		{"Vayne | 2 | IE / Rageblade / GS", "Vayne", 2, []string{"IE", "Rageblade", "GS"}, true},
		// 英雄+装备（没有星级，默认1）
		{"Vayne | IE/Rageblade", "Vayne", 1, []string{"IE", "Rageblade"}, true},
		// 英雄+★星级+装备
		{"Vayne ★2 | IE/Rageblade", "Vayne", 2, []string{"IE", "Rageblade"}, true},
		// 多个 |，后面都是装备
		{"Vayne | 2 | IE | Rageblade", "Vayne", 2, []string{"IE", "Rageblade"}, true},
		// 中文名称+装备
		{"薇恩 | 2 | 无尽/羊刀", "薇恩", 2, []string{"无尽", "羊刀"}, true},
		// 空输入
		{"", "", 0, nil, false},
		// 只有空格
		{"   ", "", 0, nil, false},
		// 纯数字不是合法英雄名
		{"123 | 2", "", 0, nil, false},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			name, star, items, ok := parseBoardCellText(tt.input)
			if ok != tt.wantOK {
				t.Errorf("parseBoardCellText(%q) ok=%v, want %v", tt.input, ok, tt.wantOK)
				return
			}
			if !tt.wantOK {
				return
			}
			if name != tt.wantName {
				t.Errorf("parseBoardCellText(%q) name=%q, want %q", tt.input, name, tt.wantName)
			}
			if star != tt.wantStar {
				t.Errorf("parseBoardCellText(%q) star=%d, want %d", tt.input, star, tt.wantStar)
			}
			// 检查 items 长度和内容
			if len(items) != len(tt.wantItems) {
				t.Errorf("parseBoardCellText(%q) items=%v, want %v (len mismatch)", tt.input, items, tt.wantItems)
				return
			}
			for i := range items {
				if items[i] != tt.wantItems[i] {
					t.Errorf("parseBoardCellText(%q) items[%d]=%q, want %q", tt.input, i, items[i], tt.wantItems[i])
				}
			}
		})
	}
}

func TestSplitItems(t *testing.T) {
	tests := []struct {
		input string
		want  []string
	}{
		{"IE/Rageblade", []string{"IE", "Rageblade"}},
		{" IE / Rageblade / GS ", []string{"IE", "Rageblade", "GS"}},
		{"IE Rageblade", []string{"IE", "Rageblade"}},
		{"  IE   Rageblade  ", []string{"IE", "Rageblade"}},
		{"IE\nRageblade", []string{"IE", "Rageblade"}},
		{"", []string{}},
		{"   ", []string{}},
		{"///", []string{}},
		{"IE//Rageblade", []string{"IE", "Rageblade"}},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := splitItems(tt.input)
			if len(got) != len(tt.want) {
				t.Errorf("splitItems(%q) = %v, want %v (len mismatch)", tt.input, got, tt.want)
				return
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("splitItems(%q)[%d] = %q, want %q", tt.input, i, got[i], tt.want[i])
				}
			}
		})
	}
}