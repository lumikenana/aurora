package tools

import (
	"context"
	"encoding/json"

	"github.com/lipipi/aurora/internal/errors"
	"github.com/lipipi/aurora/internal/tool"
)

// ConversationExitRequester 是 exit_conversation_mode 对会话生命周期的最小依赖
// （由 input.ConversationManager 实现）。定义在本包以避免 tools → input 的包依赖。
type ConversationExitRequester interface {
	// RequestEnd 请求结束当前 Conversation：当前 Turn 正常完成后进入 Idle。
	// 无活跃 Conversation 时安全 no-op，返回 false。
	RequestEnd() bool
}

// ExitConversationTool 结束连续语音对话（Conversation）的工具（Voice Refactoring Phase 4）。
//
// 薄适配层：只做「LLM 理解的退出意图 → ConversationManager.RequestEnd()」的转发，
// 不承担任何生命周期逻辑 —— Tool 调用不会立即打断当前 Turn，
// 当前 Turn 的回复正常说完后 Conversation 才结束（RequestEnd 语义）。
//
// 无活跃 Conversation（含 PTT / 文本单轮场景）时 Execute 安全 no-op。
type ExitConversationTool struct {
	requester ConversationExitRequester
}

// NewExitConversationTool 创建 ExitConversationTool
func NewExitConversationTool(cfg tool.Config) (tool.Tool, errors.AppError) {
	return &ExitConversationTool{}, nil
}

// SetConversationExitRequester 注入会话生命周期（由 Agent 在 VoiceController 就绪后注入）
func (t *ExitConversationTool) SetConversationExitRequester(r ConversationExitRequester) {
	t.requester = r
}

func (t *ExitConversationTool) Name() string {
	return "exit_conversation_mode"
}

func (t *ExitConversationTool) Description() string {
	return "结束当前的连续语音对话。仅当用户明确表达想要结束对话、停止交流或离开时调用" +
		"（例如「结束对话」「不用再聊了」「先这样吧，我要继续玩游戏了」「再见」）。" +
		"不要因为「谢谢」「好的」「明白了」等普通确认或礼貌表达而调用。" +
		"调用后本轮回复会正常说完，然后对话结束。"
}

func (t *ExitConversationTool) InputSchema() string {
	return `{
	"type": "object",
	"properties": {},
	"required": []
}`
}

func (t *ExitConversationTool) Execute(ctx context.Context, input json.RawMessage) (*tool.ToolResult, errors.AppError) {
	// 无生命周期依赖（VoiceController 未创建）或无活跃 Conversation：安全 no-op。
	// PTT / 文本单轮 Turn 中即使 LLM 误调用也不影响其既有生命周期。
	if t.requester == nil || !t.requester.RequestEnd() {
		return &tool.ToolResult{
			Type:    tool.ToolResultTypeText,
			Text:    "当前没有进行中的连续对话，无需结束。",
			Success: true,
		}, nil
	}

	return &tool.ToolResult{
		Type:    tool.ToolResultTypeText,
		Text:    "已请求结束连续对话：本轮回复完成后对话将结束。",
		Success: true,
	}, nil
}

func (t *ExitConversationTool) IsAvailable() bool {
	// 始终可用：requester 延迟注入（VoiceController 在 Agent 之后创建），
	// 若因依赖未注入被 FilterUnavailable 移除将无法恢复；
	// 未注入 / 无 Conversation 时 Execute 安全 no-op。
	return true
}
