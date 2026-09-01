//go:build windows

// Package tft 云顶之弈插件 - Windows 窗口检测
package tft

import "github.com/lipipi/aurora/internal/game"

// detectTFTWindow 检测 LOL 客户端 / 游戏内窗口
//
// 匹配规则（类名/标题均不区分大小写）：
//   - "RiotWindowClass"（LeagueClientUx 客户端，即大厅/TFT 界面）：
//     需标题包含 "League of Legends"。该类名被 Riot Client 登录器
//     （标题 "Riot Client"）和 Valorant 共用，必须靠标题排除误报
//   - "Riot Window"（游戏内客户端，标题 "League of Legends (TM) Client"）：
//     类名命中即可，该类名为 LOL 游戏内专属
//
// 注意：TFT 运行在 LOL 客户端内，此处检测到的是"LOL/TFT 候选"，
// 截图是否真的是 TFT 对局由视觉 LLM 通过 system prompt 指令二次确认
func detectTFTWindow() (game.WindowInfo, bool) {
	if w, ok := game.FindWindowByClassAndTitle("League of Legends", "RiotWindowClass"); ok {
		return w, true
	}
	if w, ok := game.FindWindowByClass("Riot Window"); ok {
		return w, true
	}
	return game.WindowInfo{}, false
}
