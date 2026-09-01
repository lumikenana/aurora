// Package agent - 架构回归门禁（Phase 17-A）
//
// 目的：防止 internal/agent 重新引入对 internal/game/plugins/* 的依赖。
//
// 历史：
//   - Phase 12-16：agent.go / game_context.go / bindings.go / tft_tools_wiring.go
//     都直接 import internal/game/plugins/tft，并通过硬编码的
//     `*tft.TFTGameContext` 类型 switch / 工具名查找逻辑耦合
//   - 这违反 CLAUDE.md §2.2 / §2.4 / §15
//
// 修复（Phase 17-A）：
//   - Agent Core 改为通过 game.GameContextRenderer / game.ToolContributor /
//     game.SetupReceiver 等可选接口与插件协作
//   - tft_tools_wiring.go 删除；工具依赖注入由 tft.Plugin.RegisterTools 负责
//   - game_context.go 退化为 ~30 行薄包装，委托给插件的 Renderer
//
// 本测试断言 Agent Core 永远不能再 import 任何 internal/game/plugins/* 子包。
// 任何 PR / 重新引入此耦合的尝试都会让本测试 fail。
package agent

import (
	"os/exec"
	"strings"
	"testing"
)

// TestAgent_NoGamePluginDependency 断言 internal/agent 不 import 任何 internal/game/plugins/*
//
// 通过 `go list -f '{{.Imports}}' .` 拿 production 代码的直接 imports 列表，
// 断言其中不含 "internal/game/plugins/"。
//
// 注意：测试只检查直接 import（不递归）。`go list` 默认不递归 imports；
// 若 agent 通过 game 包间接使用 tft symbol，会被 game 包的依赖方向阻断
// （game 包不 import tft，见 internal/game/game.go 验证）。
func TestAgent_NoGamePluginDependency(t *testing.T) {
	out, err := exec.Command("go", "list", "-f", "{{.Imports}}", ".").Output()
	if err != nil {
		t.Fatalf("go list failed: %v", err)
	}
	if strings.Contains(string(out), "internal/game/plugins/") {
		t.Fatalf("internal/agent must not import any internal/game/plugins/* package.\nImports: %s", string(out))
	}
}

// TestTool_NoGamePluginDependency 断言 internal/tool* 不 import 任何 internal/game/plugins/*
//
// 防止 internal/tool/tools/init.go 重新出现 game-specific 工具注册。
// 通用 tool 包（capture_screenshot / get_current_time）必须保持游戏无关。
func TestTool_NoGamePluginDependency(t *testing.T) {
	out, err := exec.Command("go", "list", "-f", "{{.Imports}}",
		"github.com/lipipi/aurora/internal/tool",
		"github.com/lipipi/aurora/internal/tool/tools",
	).Output()
	if err != nil {
		t.Fatalf("go list failed: %v", err)
	}
	if strings.Contains(string(out), "internal/game/plugins/") {
		t.Fatalf("internal/tool* must not import any internal/game/plugins/* package.\nImports: %s", string(out))
	}
}

// TestGenericGame_NoGamePluginDependency 断言 internal/game 不 import 任何 internal/game/plugins/*
//
// internal/game 是通用抽象层；不能依赖具体插件。
func TestGenericGame_NoGamePluginDependency(t *testing.T) {
	out, err := exec.Command("go", "list", "-f", "{{.Imports}}", "github.com/lipipi/aurora/internal/game").Output()
	if err != nil {
		t.Fatalf("go list failed: %v", err)
	}
	if strings.Contains(string(out), "internal/game/plugins/") {
		t.Fatalf("internal/game must not import any internal/game/plugins/* package.\nImports: %s", string(out))
	}
}
