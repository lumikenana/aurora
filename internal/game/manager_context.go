// Package game - Manager 上下文状态管理
//
// Phase 10-6 范围：
//   - Manager 持有 lastKnown *GameContext 状态
//   - UpdateContext 串行调用所有 ContextProvider 合并 delta（Phase 10-6 暂无 Provider）
//   - Snapshot 返回 lastKnown 副本（防外部修改）
//   - Clear 清空 lastKnown
//
// 关键不变量：
//   - 状态与窗口检测缓存分离（ctxMu vs mu）
//   - 不增加后台 goroutine / 定时任务
//   - 不阻塞 DetectActive
package game

import (
	"context"
)

// RegisterContextProvider 注册 ContextProvider（沿用 LLM/Speech/Tool 注册模式）
//
// 暂时不导出注册入口到外部包，未来 Provider 接入时再开放
func (m *Manager) RegisterContextProvider(p ContextProvider) {
	m.ctxMu.Lock()
	defer m.ctxMu.Unlock()
	m.providers = append(m.providers, p)
}

// UpdateContext 增量更新 GameContext
//
// Phase 10-6 行为：
//   - 若没有 current，使用 DetectActive 拿当前窗口上下文
//   - 串行调用所有 Provider（按 Order 升序），合并 ContextDelta
//   - 合并规则：Phase 字段按 Provider 顺序覆盖；GameData 按 Source 优先级整体替换
//   - 写入 lastKnown（加锁）
//
// Phase 10-6 没有任何实际 Provider 注册时，行为等价于"用窗口检测初始化 lastKnown"
func (m *Manager) UpdateContext(ctx context.Context, in ContextInput) *GameContext {
	m.ctxMu.Lock()
	providers := make([]ContextProvider, len(m.providers))
	copy(providers, m.providers)
	m.ctxMu.Unlock()

	// 取当前上下文（先读 lastKnown，缺失时调用 DetectActive）
	m.ctxMu.RLock()
	current := m.lastKnown
	m.ctxMu.RUnlock()
	if current == nil {
		_, detected, ok := m.DetectActive(ctx)
		if ok {
			current = detected
		} else {
			// 未检测到任何游戏，返回 nil（不更新 lastKnown）
			return nil
		}
	}

	// 复制 current 防止 Provider 持有内部状态
	updated := cloneGameContext(current)

	// 串行调用所有 Provider（按 Order 排序在调用方做，这里假设已排序）
	for _, p := range providers {
		delta, err := p.Contribute(ctx, in)
		if err != nil {
			// Provider 错误不阻塞整体流程，跳过
			continue
		}
		if delta == nil {
			continue
		}
		applyDelta(updated, delta)
	}

	// 写入 lastKnown
	m.ctxMu.Lock()
	m.lastKnown = updated
	m.ctxMu.Unlock()

	return updated
}

// Snapshot 返回当前 lastKnown 快照副本
//
// 调用方对返回值的修改不会影响 Manager 内部状态。
// 若 lastKnown 为 nil，返回 nil。
func (m *Manager) Snapshot() *GameContext {
	m.ctxMu.RLock()
	defer m.ctxMu.RUnlock()
	if m.lastKnown == nil {
		return nil
	}
	return cloneGameContext(m.lastKnown)
}

// Clear 清空 lastKnown（对局结束、玩家切号等场景）
func (m *Manager) Clear() {
	m.ctxMu.Lock()
	m.lastKnown = nil
	m.ctxMu.Unlock()
}

// cloneGameContext 深拷贝 GameContext 及其子结构
//
// 通用层字段（Game/Name/Phase/Window/CommonData）按值拷贝；
// GameData（any）按浅拷贝——TFT 字段本身是 Value[T]，Value 是值类型安全。
func cloneGameContext(src *GameContext) *GameContext {
	if src == nil {
		return nil
	}
	dst := *src
	// CommonData 按值拷贝（Value 本身是值类型）
	dst.CommonData = src.CommonData
	// GameData 保持原引用（Phase 10-6 范围内 GameData 内容不会被修改，
	// 未来 Provider 替换时整体替换 GameData 指针）
	return &dst
}

// applyDelta 将 Provider 的 ContextDelta 合并到 GameContext
//
// GameData 合并规则（Phase 10-7）：
//   - 若 current.GameData 实现了 GameDataMerger 且 newGameData 类型相同
//     → 字段级合并（仅覆盖 newGameData 中 Present=true 的字段）
//   - 否则 → 整体替换（Phase 10-6 行为）
func applyDelta(ctx *GameContext, delta *ContextDelta) {
	if delta.Phase != nil {
		ctx.Phase = *delta.Phase
	}
	if delta.Common != nil {
		// CommonData 整体替换（Provider 已构造完整 CommonData）
		ctx.CommonData = *delta.Common
	}
	if delta.GameData != nil {
		// 尝试字段级合并
		if merger, ok := ctx.GameData.(GameDataMerger); ok {
			merger.MergeFrom(delta.GameData)
		} else {
			// 整体替换
			ctx.GameData = delta.GameData
		}
	}
}
