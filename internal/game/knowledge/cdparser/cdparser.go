// Package cdparser CommunityDragon 本地数据解析器
//
// 从本地 data/communitydragon 目录解析 CommunityDragon 数据，
// 用于补充 Data Dragon 缺失的 Set/Mode/分类信息。
//
// 职责：
//   - 解析 tftsets.json → 获取当前默认 Set
//   - 解析 tftchampions-teamplanner.json → Champion → Set 映射
//   - 解析 tfttraits.json → Trait → Set 映射
//   - 解析 tftitems.json → Item → 分类/Set 信息
//   - 从 augments 数据中提取 Augment → Mode 映射
package cdparser

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/lipipi/aurora/internal/errors"
	"github.com/lipipi/aurora/internal/log"
)

// Config CommunityDragon 解析器配置
type Config struct {
	BaseDir string // 数据根目录，默认 "data/communitydragon"
	Locale  string // 语言，默认 "zh_cn"
}

// DefaultConfig 返回默认配置
func DefaultConfig() Config {
	return Config{
		BaseDir: "data/communitydragon",
		Locale:  "zh_cn",
	}
}

// Parser CommunityDragon 本地数据解析器
type Parser struct {
	config Config
}

// New 创建解析器
func New(cfg Config) *Parser {
	if cfg.BaseDir == "" {
		cfg.BaseDir = DefaultConfig().BaseDir
	}
	if cfg.Locale == "" {
		cfg.Locale = DefaultConfig().Locale
	}
	return &Parser{config: cfg}
}

// TFTSet 表示从 tftsets.json 解析的 Set 信息
type TFTSet struct {
	SetName         string `json:"SetName"`
	SetCoreName     string `json:"SetCoreName"`
	SetDisplayName  string `json:"SetDisplayName"`
	SetAugmentName  string `json:"SetAugmentName"`
}

// TFTSetData 整个 tftsets.json 结构
type TFTSetData struct {
	LCTFTModeData struct {
		MActiveSets          []TFTSet `json:"mActiveSets"`
		MDefaultSet          TFTSet   `json:"mDefaultSet"`
		MDefaultTeamPlannerSet TFTSet  `json:"mDefaultTeamPlannerSet"`
		MEventSet            TFTSet   `json:"mEventSet"`
	} `json:"LCTFTModeData"`
}

// CDChampion Champion 数据（来自 tftchampions-teamplanner.json）
type CDChampion struct {
	CharacterID   string `json:"character_id"`
	DisplayName   string `json:"display_name"`
	Tier          int    `json:"tier"`
	Traits        []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"traits"`
}

// CDChampionsBySet 是 tftchampions-teamplanner.json 顶层结构：SetName → []CDChampion
type CDChampionsBySet map[string][]CDChampion

// CDTraitBreakpoint CD Trait 的一个 breakpoint（来自 conditional_trait_sets / innate_trait_sets）
type CDTraitBreakpoint struct {
	// MinUnits 激活所需最少单位数
	MinUnits int `json:"min_units"`
	// MaxUnits 该阶生效的最大单位数（0 / 25000 = 无上限）
	MaxUnits int `json:"max_units"`
	// StyleIdx CD 内部 style 索引
	StyleIdx int `json:"style_idx"`
	// StyleName 阶名（kBronze / kSilver / kGold / kPrismatic）
	StyleName string `json:"style_name"`
	// Constants 数值占位符映射（如 {"BonusHealth": "150"}）
	Constants []CDTraitConstant `json:"constants"`
}

// CDTraitConstant CD Trait 常量占位符
//
// 形如 {"name": "BonusHealth", "value": 150}。
// tooltip_text 中的 "@BonusHealth@" 会被这些 value 替换。
type CDTraitConstant struct {
	Name  string `json:"name"`
	Value float64 `json:"value"`
}

// CDTrait Trait 数据（来自 tfttraits.json）
//
// effects 来自 conditional_trait_sets（普通阶）+ innate_trait_sets（固有效果），
// 完整 raw JSON 保留在 EffectsRaw 字段供 composition.Breakpoint 解码。
type CDTrait struct {
	TraitID      string             `json:"trait_id"`
	DisplayName  string             `json:"display_name"`
	Set          string             `json:"set"`
	TooltipText  string             `json:"tooltip_text"`
	IconPath     string             `json:"icon_path"`
	// InnateTraitSets 固有效果（无 breakpoint，永久生效）
	InnateTraitSets []CDTraitBreakpoint `json:"innate_trait_sets"`
	// ConditionalTraitSets 普通阶（按 min_units 激活）
	ConditionalTraitSets []CDTraitBreakpoint `json:"conditional_trait_sets"`
}

// CDItem Item 数据（来自 tftitems.json）
type CDItem struct {
	GUID         string `json:"guid"`
	Name         string `json:"name"`
	NameID       string `json:"nameId"`
	ID           int    `json:"id"`
	SquareIconPath string `json:"squareIconPath"`
}

// ItemCategory 表示识别出的物品分类
type ItemCategory string

const (
	CategoryBasic          ItemCategory = "basic"           // 基础装备
	CategoryComponent      ItemCategory = "components"      // 合成部件（基础装备也叫 components）
	CategoryCompleted      ItemCategory = "standard"        // 普通合成装备
	CategoryRadiant        ItemCategory = "radiant"         // 光明装备
	CategoryOrnn           ItemCategory = "ornn"            // Ornn 神器
	CategoryConsumable     ItemCategory = "consumables"     // 消耗品
	CategoryAugment        ItemCategory = "augments"        // 强化符文
	CategoryAssistReward   ItemCategory = "assist-rewards" // 协助奖励
	CategorySetSpecific    ItemCategory = "set-specific"    // Set 特定阵营装备
	CategoryDebug          ItemCategory = "debug"           // 调试物品
	CategoryUnknown        ItemCategory = "unknown"         // 未知分类
)

// CDAugment Augment 数据（从 tftitems.json 提取，补充 Mode 信息）
type CDAugment struct {
	NameID string
	Mode   string // 如 "CHERRY", "KIWI", "KIWI_JADE" 等
	Set    string // Set 名称
}

// ParseSets 解析 tftsets.json
func (p *Parser) ParseSets() (*TFTSetData, error) {
	path := p.filePath("tftsets.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, errors.Wrap(errors.CodeIOError, "read tftsets.json", err)
	}

	var sets TFTSetData
	if err := json.Unmarshal(data, &sets); err != nil {
		return nil, fmt.Errorf("parse tftsets.json: %w", err)
	}

	return &sets, nil
}

// GetDefaultSet 获取默认 Set（从 mDefaultSet）
func (p *Parser) GetDefaultSet() (string, error) {
	setsData, err := p.ParseSets()
	if err != nil {
		return "", err
	}
	defaultSet := setsData.LCTFTModeData.MDefaultSet
	if defaultSet.SetName == "" {
		return "", fmt.Errorf("mDefaultSet is empty in tftsets.json")
	}
	return defaultSet.SetName, nil
}

// ParseChampionsBySet 解析 tftchampions-teamplanner.json
func (p *Parser) ParseChampionsBySet() (CDChampionsBySet, error) {
	path := p.filePath("tftchampions-teamplanner.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, errors.Wrap(errors.CodeIOError, "read tftchampions-teamplanner.json", err)
	}

	var championsBySet CDChampionsBySet
	if err := json.Unmarshal(data, &championsBySet); err != nil {
		return nil, fmt.Errorf("parse tftchampions-teamplanner.json: %w", err)
	}

	return championsBySet, nil
}

// ParseTraits 解析 tfttraits.json
func (p *Parser) ParseTraits() ([]CDTrait, error) {
	path := p.filePath("tfttraits.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, errors.Wrap(errors.CodeIOError, "read tfttraits.json", err)
	}

	var traits []CDTrait
	if err := json.Unmarshal(data, &traits); err != nil {
		return nil, fmt.Errorf("parse tfttraits.json: %w", err)
	}

	return traits, nil
}

// ParseItems 解析 tftitems.json
func (p *Parser) ParseItems() ([]CDItem, error) {
	path := p.filePath("tftitems.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, errors.Wrap(errors.CodeIOError, "read tftitems.json", err)
	}

	var items []CDItem
	if err := json.Unmarshal(data, &items); err != nil {
		return nil, fmt.Errorf("parse tftitems.json: %w", err)
	}

	return items, nil
}

// CDAugmentModeList 单个 Mode 的 Augment 列表
type CDAugmentModeList struct {
	ModeName    string   `json:"modeName"`
	AugmentList []string `json:"augmentList"`
}

// ParseAugmentLists 解析 augment-lists.json 返回 Augment 名称 → Mode 名称映射
// 输入路径格式 "Maps/ModeSpecificData/Augments/XXX" → 提取最后部分 XXX 作为 key
func (p *Parser) ParseAugmentLists() (map[string]string, error) {
	path := p.filePath("augment-lists.json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]string{}, nil
		}
		return nil, errors.Wrap(errors.CodeIOError, "read augment-lists.json", err)
	}

	// 检查是否是 HTML 404
	if len(data) > 10 && strings.Contains(string(data), "<html>") && strings.Contains(string(data), "404 Not Found") {
		log.Warnf("augment-lists.json is 404 HTML, skipping")
		return map[string]string{}, nil
	}

	var modeLists []CDAugmentModeList
	if err := json.Unmarshal(data, &modeLists); err != nil {
		return nil, fmt.Errorf("parse augment-lists.json: %w", err)
	}

	result := make(map[string]string)
	for _, ml := range modeLists {
		for _, augmentPath := range ml.AugmentList {
			// extract last component from path "Maps/ModeSpecificData/Augments/XXX" → "XXX"
			parts := strings.Split(augmentPath, "/")
			augmentName := parts[len(parts)-1]
			result[augmentName] = ml.ModeName
		}
	}

	log.Infof("augment-lists.json: parsed %d augments in %d modes", len(result), len(modeLists))
	return result, nil
}

// ClassifyItem 根据名称和 NameID 判断物品分类
// 基于 CommunityDragon 导出内容进行启发式分类（因为原始 JSON 不含分类字段）
func ClassifyItem(item CDItem) ItemCategory {
	nameLower := strings.ToLower(item.Name)
	nameID := item.NameID

	// Augments 容易识别：nameId 含 "Augment"
	if strings.Contains(strings.ToLower(nameID), "augment") {
		return CategoryAugment
	}

	// 启发式分类
	switch {
	case strings.Contains(nameLower, "radiant"):
		return CategoryRadiant
	case strings.Contains(nameLower, "ornn") || strings.Contains(nameLower, "artifact"):
		return CategoryOrnn
	case strings.Contains(nameLower, "consumable") || strings.Contains(nameLower, "potion"):
		return CategoryConsumable
	case strings.Contains(nameLower, "assist") || strings.Contains(nameLower, "reward"):
		return CategoryAssistReward
	case strings.Contains(nameLower, "debug"):
		return CategoryDebug
	// 对于 Set 特定物品，NameID 通常包含 Set 编号如 "TFT18_..."
	// 但不好直接判断，留给调用方处理
	default:
		// 通过长度和常见模式推断
		if isBasicComponent(item.Name) {
			return CategoryComponent
		}
		return CategoryUnknown
	}
}

// isBasicComponent 判断是否是基础合成部件
func isBasicComponent(name string) bool {
	// 常见基础装备名称
	basics := map[string]bool{
		"needlessly large rod": true,
		"recurve bow": true,
		"sword": true,
		"giant's belt": true,
		"chain vest": true,
		"negatron cloak": true,
		"tear of the goddess": true,
		"b. f. sword": true,
		"sparring gloves": true,
		"giant force": false,
	}
	return basics[strings.ToLower(name)]
}

// SetKeyFromCDName 将 CommunityDragon Set 名称转换为项目 set_key 格式
// Example: "TFTSet18" → "set18"
func SetKeyFromCDName(cdSetName string) string {
	// 去掉 "TFTSet" 前缀
	if strings.HasPrefix(cdSetName, "TFTSet") {
		suffix := strings.TrimPrefix(cdSetName, "TFTSet")
		// 处理特殊如 "TFTSet7_Stage2" → 保留数字部分
		// "TFTSet18" → "18" → "set18"
		// "TFTSetTutorial" → "Tutorial" → "settutorial"
		var digits strings.Builder
		var nonDigits strings.Builder
		for _, r := range suffix {
			if r >= '0' && r <= '9' {
				digits.WriteRune(r)
			} else {
				nonDigits.WriteRune(r)
			}
		}
		if digits.Len() > 0 {
			return "set" + digits.String()
		}
		// 如果全是非数字（如 Tutorial），保留全部
		return "set" + strings.ToLower(strings.Trim(suffix, "_"))
	}
	// 如果不含前缀，直接返回小写
	return strings.ToLower(cdSetName)
}

// ExtractSetFromNameID 从 ID 中提取 Set 信息
// 格式支持:
//   - "TFT18_Jinx" → "TFTSet18"
//   - "DA_18_XXX" → "TFTSet18" (Data Dragon augment format)
//   - "TFTTutorial_Zed" → "TFTSetTutorial"
func ExtractSetFromNameID(id string) string {
	// Check for DA_XX_... (Data Dragon modern augment format)
	if strings.HasPrefix(id, "DA_") {
		parts := strings.SplitN(id[3:], "_", 2)
		if len(parts) > 0 && parts[0] != "" {
			// parts[0] is the set number like "18"
			isAllDigits := true
			for _, c := range parts[0] {
				if c < '0' || c > '9' {
					isAllDigits = false
					break
				}
			}
			if isAllDigits {
				return "TFTSet" + parts[0]
			}
		}
	}
	// Check for original TFTXX_... format
	if strings.HasPrefix(id, "TFT") {
		// after "TFT" until next '_'
		parts := strings.SplitN(id[3:], "_", 2)
		if len(parts) > 0 && parts[0] != "" {
			// parts[0] is like "18", "17", "Tutorial"
			// 检测是否是纯数字
			isAllDigits := true
			for _, c := range parts[0] {
				if c < '0' || c > '9' {
					isAllDigits = false
					break
				}
			}
			if isAllDigits {
				return "TFTSet" + parts[0]
			}
			// 非数字如 "Tutorial"
			return "TFT" + parts[0]
		}
	}
	// Check for any occurrence of _XX_ where XX is digits
	// Handles formats like "XXX_18_XXXX"
	parts := strings.Split(id, "_")
	for i, part := range parts {
		if i > 0 && i < len(parts)-1 {
			isAllDigits := true
			for _, c := range part {
				if c < '0' || c > '9' {
					isAllDigits = false
					break
				}
			}
			if isAllDigits && len(part) <= 2 && part != "" {
				return "TFTSet" + part
			}
		}
	}
	return ""
}

// IsInSet 判断 ID 是否属于目标 Set
func IsInSet(id string, targetSet string) bool {
	extracted := ExtractSetFromNameID(id)
	if extracted == "" {
		return false
	}
	// 比较规范化的 setKey
	targetKey := SetKeyFromCDName(targetSet)
	extractedKey := SetKeyFromCDName(extracted)
	return targetKey == extractedKey
}

func (p *Parser) filePath(filename string) string {
	return fmt.Sprintf("%s/%s", p.config.BaseDir, filename)
}

// BuildChampionSetMap 建立 Champion ID → Set 名称 的映射
func (p *Parser) BuildChampionSetMap() (map[string]string, error) {
	championsBySet, err := p.ParseChampionsBySet()
	if err != nil {
		return nil, err
	}

	result := make(map[string]string)
	for setName, champions := range championsBySet {
		for _, champ := range champions {
			result[champ.CharacterID] = setName
		}
	}

	return result, nil
}

// BuildTraitSetMap 建立 Trait ID → Set 名称 的映射
func (p *Parser) BuildTraitSetMap() (map[string]string, error) {
	traits, err := p.ParseTraits()
	if err != nil {
		return nil, err
	}

	result := make(map[string]string)
	for _, trait := range traits {
		if trait.Set != "" {
			// trait.TraitID 是短名如 "Anima"，Data Dragon 使用完整如 "TFT17_AnimaSquad"
			// 所以我们需要保留原格式，匹配由调用方处理
			result[trait.TraitID] = trait.Set
		}
	}

	return result, nil
}

// BuildItemInfo 建立 Item NameID → (category, set) 信息
// 从 NameID 推断 Set
func (p *Parser) BuildItemInfo() (map[string]struct {
	Category ItemCategory
	Set      string
}, error) {
	items, err := p.ParseItems()
	if err != nil {
		return nil, err
	}

	result := make(map[string]struct {
		Category ItemCategory
		Set      string
	})

	for _, item := range items {
		cat := ClassifyItem(item)
		setName := ExtractSetFromNameID(item.NameID)
		result[item.NameID] = struct {
			Category ItemCategory
			Set      string
		}{cat, setName}
	}

	return result, nil
}

// CountStats 输出解析统计
func (p *Parser) CountStats() (map[string]int, error) {
	stats := make(map[string]int)

	setsData, err := p.ParseSets()
	if err != nil {
		return nil, err
	}
	stats["active_sets"] = len(setsData.LCTFTModeData.MActiveSets)

	championsBySet, err := p.ParseChampionsBySet()
	if err != nil {
		return nil, err
	}
	totalChamps := 0
	for set, champs := range championsBySet {
		stats["champions_"+strings.ToLower(set)] = len(champs)
		totalChamps += len(champs)
	}
	stats["champions_total"] = totalChamps

	traits, err := p.ParseTraits()
	if err != nil {
		return nil, err
	}
	stats["traits_total"] = len(traits)

	items, err := p.ParseItems()
	if err != nil {
		return nil, err
	}
	stats["items_total"] = len(items)

	// Count categories
	catCount := make(map[ItemCategory]int)
	for _, item := range items {
		cat := ClassifyItem(item)
		catCount[cat]++
	}
	for cat, cnt := range catCount {
		stats["items_"+string(cat)] = cnt
	}

	augmentModeMap, err := p.ParseAugmentLists()
	if err != nil {
		return nil, err
	}
	stats["augment_modes"] = len(augmentModeMap)

	return stats, nil
}
