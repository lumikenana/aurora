// Package tft - TFT 附加系统提示词
//
// 历史：systemPrompt 常量原本可能在另一个文件。Phase 17-A 重构时把所有
// 文件迁入此目录并补齐此常量，避免编译失败。
//
// 内容保留通用 / 不绑定具体 patch / 英雄名 / 装备名（CLAUDE.md §3.1 +
// §5.1：不要在 system prompt 里硬编码游戏语义细节）。
package tft

// systemPrompt TFT 插件附加到 Agent 通用系统提示词后的内容
//
// 设计：
//   - 只在 Agent 不知道用户当前玩什么游戏时由 Plugin.SystemPrompt() 返回
//   - 真正的"对局状态摘要"走 GameContextRenderer 接口（见 prompt.go）
//   - 本常量仅承载 TFT 通用操作原则
const systemPrompt = `你正在协助玩家玩《云顶之弈》（Teamfight Tactics）。
当玩家询问具体阵容、装备、经济、备战等决策时：
- 优先依据当前实际游戏状态（棋盘 / 备战席 / 商店 / 装备 / 金币 / 血量）回答
- 不要凭训练数据中泛化的 TFT 经验推断
- 若无法从当前状态判断，明确告知玩家当前信息不足，需要补充截图或数据
- 严格区分"你看到的对局状态"与"通用 TFT 知识"——前者来自系统注入的实时数据，后者来自工具调用的知识库查询
`
