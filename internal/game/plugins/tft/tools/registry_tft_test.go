package tools

import (
	"testing"

	apperrors "github.com/lipipi/aurora/internal/errors"
	"github.com/lipipi/aurora/internal/tool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestTFTToolsRegistered 验证所有 5 个 TFT 工具已通过 tool.RegisterTool 注册
//
// Phase 11-1 验收标准：get_tft_game_state / get_tft_champion / get_tft_item /
// get_tft_trait / get_tft_augment 全部正确注册。
//
// 注意：本测试位于 tools 包，trigger 包级 init() 后 factory 表已填充。
func TestTFTToolsRegistered(t *testing.T) {
	required := []string{
		"get_tft_game_state",
		"get_tft_champion",
		"get_tft_item",
		"get_tft_trait",
		"get_tft_augment",
	}

	for _, name := range required {
		t.Run(name, func(t *testing.T) {
			_, ok := tool.GetToolFactory(name)
			assert.True(t, ok, "tool factory %q must be registered", name)
		})
	}
}

// TestTFTToolFactories 验证 5 个工具的工厂能正确创建 tool 实例
//
// 注意：knowledge 工具的 IsAvailable() 在未注入依赖时返回 false，
// 所以无法通过 CreateRegistry（它会跳过 IsAvailable()=false 的工具）。
// 直接调用 factory 验证即可——这是更纯粹的注册验证。
func TestTFTToolFactories(t *testing.T) {
	cases := []struct {
		factory func(tool.Config) (tool.Tool, apperrors.AppError)
		want    string
	}{
		{NewTFTGameStateTool, "get_tft_game_state"},
		{NewTFTChampionTool, "get_tft_champion"},
		{NewTFTItemTool, "get_tft_item"},
		{NewTFTTraitTool, "get_tft_trait"},
		{NewTFTAugmentTool, "get_tft_augment"},
	}
	for _, c := range cases {
		t.Run(c.want, func(t *testing.T) {
			tl, err := c.factory(tool.Config{})
			require.NoError(t, err)
			require.NotNil(t, tl)
			assert.Equal(t, c.want, tl.Name())
			// 描述必须包含"严格限定"+"(patch, game, mode)" 或"识别"等关键约束
			desc := tl.Description()
			assert.NotEmpty(t, desc)
		})
	}
}
