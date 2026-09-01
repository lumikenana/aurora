// Package game 插件管理器
package game

import (
	"context"
	"sync"
	"time"

	"github.com/lipipi/aurora/internal/errors"
	"github.com/lipipi/aurora/internal/log"
)

// PluginFactory 插件工厂
type PluginFactory func(cfg Config) (GamePlugin, errors.AppError)

// pluginFactories 插件注册表（各插件包 init() 中调用 RegisterPlugin 自注册）
var pluginFactories = make(map[string]PluginFactory)

// RegisterPlugin 注册插件工厂（沿用 LLM/Speech/Tool 的注册模式，ADR-012）
func RegisterPlugin(id string, factory PluginFactory) {
	pluginFactories[id] = factory
}

// UnregisterPlugin 注销插件工厂（主要用于测试清理，避免全局注册表污染其他测试）
func UnregisterPlugin(id string) {
	delete(pluginFactories, id)
}

// detection 一次检测的结果（含 TTL 缓存时间戳）
type detection struct {
	plugin GamePlugin
	ctx    *GameContext
	at     time.Time
}

// Manager 游戏插件管理器
type Manager struct {
	plugins []GamePlugin
	ttl     time.Duration
	mu      sync.Mutex
	cache   *detection // TTL 缓存，正负结果都缓存

	// 上下文状态（Phase 10-6 新增；与窗口检测缓存分离，避免阻塞 DetectActive）
	ctxMu     sync.RWMutex
	providers []ContextProvider
	lastKnown *GameContext
}

// CreateManager 创建插件管理器，按 cfg.Enabled 实例化已注册的插件
func CreateManager(cfg Config) (*Manager, errors.AppError) {
	m := &Manager{ttl: 5 * time.Second}
	for _, id := range cfg.Enabled {
		factory, ok := pluginFactories[id]
		if !ok {
			log.Warnf("Unknown game plugin: %s, skipping (需要 blank import 触发注册)", id)
			continue
		}
		p, err := factory(cfg)
		if err != nil {
			log.Warnf("Game plugin %s init failed: %v", id, err)
			continue
		}
		m.plugins = append(m.plugins, p)
		log.Debugf("Registered game plugin: %s (%s)", p.ID(), p.Name())
	}
	return m, nil
}

// DetectActive 返回当前检测到的第一个插件
// 结果带 TTL 缓存（默认 5s），缓存期内不重复执行窗口枚举
func (m *Manager) DetectActive(ctx context.Context) (GamePlugin, *GameContext, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.cache != nil && time.Since(m.cache.at) < m.ttl {
		if m.cache.plugin != nil {
			return m.cache.plugin, m.cache.ctx, true
		}
		return nil, nil, false
	}

	for _, p := range m.plugins {
		gctx, err := p.Detect(ctx)
		if err != nil {
			// 检测失败视为未检测到，不阻塞对话
			log.Warnf("Game plugin %s detect error: %v", p.ID(), err)
			continue
		}
		if gctx != nil {
			m.cache = &detection{plugin: p, ctx: gctx, at: time.Now()}
			return p, gctx, true
		}
	}

	m.cache = &detection{plugin: nil, at: time.Now()}
	return nil, nil, false
}

// Count 已启用的插件数量
func (m *Manager) Count() int {
	return len(m.plugins)
}

// Plugins 返回所有已实例化的插件（只读副本）
func (m *Manager) Plugins() []GamePlugin {
	// 返回副本防止外部修改内部切片
	result := make([]GamePlugin, len(m.plugins))
	copy(result, m.plugins)
	return result
}

// AddContextProvider 添加一个上下文提供者
// ContextProvider 用于从截图/API等数据源提取游戏状态，更新 GameContext
func (m *Manager) AddContextProvider(provider ContextProvider) {
	m.ctxMu.Lock()
	defer m.ctxMu.Unlock()
	m.providers = append(m.providers, provider)
	log.Debugf("Added game context provider: %s (source=%s, order=%d)", provider, provider.Source(), provider.Order())
}

// GetContextProviders 获取所有已注册的上下文提供者（按 Order 排序）
func (m *Manager) GetContextProviders() []ContextProvider {
	m.ctxMu.RLock()
	defer m.ctxMu.RUnlock()
	// 返回副本防止外部修改
	result := make([]ContextProvider, len(m.providers))
	copy(result, m.providers)
	return result
}

// LastKnown 返回最近一次 ContextProvider 更新得到的 GameContext（Phase 16-B）
//
// 不触发检测：仅返回 ContextProvider 写入的快照。
// 调试面板、debug hook 走这个；真实检测走 DetectActive。
//
// 返回 nil 表示从未有 ContextProvider 写入过。
func (m *Manager) LastKnown() *GameContext {
	m.ctxMu.RLock()
	defer m.ctxMu.RUnlock()
	return m.lastKnown
}
