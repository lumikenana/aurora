<script setup lang="ts">
// App.vue - Aurora Main Window App Shell (Phase 4)
//
// 职责（Phase 4 重构后）：
//   1. 持有 application 全局状态
//   2. 监听后端 events（llmChunk / log / showQuickInput / overlayConfigChanged 等）
//   3. Sidebar 导航 + Content 区域
//   4. 全局挂载 QuickInput / VoiceIndicator / Overlay
//   5. Active view 切换
//
// 不再负责（已拆分到 views）：
//   - Chat / Settings / Logs / Overlay 业务 UI
//   - 表单状态
//   - 业务事件处理

import { ref, computed, onMounted, watch } from 'vue'
import { EventsOn } from './api/events'
import ChatView from './views/ChatView.vue'
import OverlayView from './views/OverlayView.vue'
import SettingsView from './views/SettingsView.vue'
import LogsView from './views/LogsView.vue'
import Settings from './components/Settings.vue'
import QuickInput from './components/QuickInput.vue'
import VoiceIndicator from './components/VoiceIndicator.vue'
import Overlay from './components/Overlay.vue'
import {
  getConfigStatus,
  chatStream,
  frontendEcho,
  getActiveGame,
  submitQuickInput,
  getOverlayConfig,
  saveOverlayConfig,
  setOverlayVisible,
  getOverlayVisible,
  type ActiveGame,
  type ChatMessage,
  type OverlayConfig,
} from './api/backend'
import type { LogEntry } from './components/LogPanel.vue'

// ---- 全局状态 ----
const messages = ref<ChatMessage[]>([])
const logs = ref<LogEntry[]>([])
const quickInputVisible = ref(false)
const activeGame = ref<ActiveGame>({ active: false })
const agentBusy = ref(false) // 后端真正忙：chatStream 调用期间

const activeView = ref<'chat' | 'overlay' | 'settings' | 'logs'>('chat')

// ---- Overlay 状态 ----
const overlayConfig = ref<OverlayConfig>({
  enabled: false,
  opacity: 0.9,
  font_size: 'medium',
  auto_scroll: true,
  show_user_messages: true,
  show_assistant_messages: true,
  anchor: 'bottom-right',
})
const overlayVisible = ref(false)

// ---- 日志 ----
const MAX_LOGS = 500

function addLog(level: string, message: string) {
  logs.value.push({
    level,
    message,
    time: new Date().toLocaleTimeString(),
  })
  if (logs.value.length > MAX_LOGS) {
    logs.value.splice(0, logs.value.length - MAX_LOGS)
  }
}

// Phase 6 B4: 清空 UI log buffer（不动磁盘上的应用日志文件）。
function clearLogs() {
  logs.value.splice(0, logs.value.length)
}

// ---- 对话 ----
function appendToLastAssistant(content: string) {
  const last = messages.value[messages.value.length - 1]
  if (last && last.role === 'assistant') {
    last.content += content
  } else {
    messages.value.push({ role: 'assistant', content })
  }
}

async function handleSubmit(text: string) {
  messages.value.push({ role: 'user', content: text })
  agentBusy.value = true
  try {
    let status
    try {
      status = await getConfigStatus()
    } catch (err) {
      // Phase 6 A7: 错误独立成 role，UI 渲染为系统错误卡。
      // 仍是"未配置"的归类时，给出可执行的下一步。
      console.error('[handleSubmit] getConfigStatus failed:', err)
      messages.value.push({
        role: 'error',
        content: 'Unable to reach Aurora. Check Settings or restart the app.',
      })
      return
    }
    if (!status.configured) {
      messages.value.push({
        role: 'error',
        content: 'AI provider is not configured. Open Settings → AI to add your API key.',
      })
      return
    }

    messages.value.push({ role: 'assistant', content: '' })
    await chatStream(text)
  } catch (err) {
    // Phase 6 A7: 不暴露原始 Go / LLM 错误细节。给一个可操作的通用错误。
    console.error('[handleSubmit] chat failed:', err)
    messages.value.push({
      role: 'error',
      content: 'Unable to get a response. Check your AI settings or connection and try again.',
    })
  } finally {
    try { await submitQuickInput(text) } catch { /* ignore */ }
    quickInputVisible.value = false
    agentBusy.value = false
  }
}

// ---- 后端 events ----
EventsOn('gameDetect', (data: ActiveGame) => {
  activeGame.value = data
})

EventsOn('showQuickInput', (show: unknown) => {
  const isShow = !!show
  try { frontendEcho(`showQuickInput:${isShow}`) } catch { /* ignore */ }
  quickInputVisible.value = isShow
})

EventsOn('log', (data: { level: string; message: string }) => {
  if (data && data.level && data.message) {
    addLog(data.level, data.message)
  }
})

EventsOn('llmChunk', (content: string) => {
  if (content) {
    appendToLastAssistant(content)
  }
})

EventsOn('llmFinish', () => {
  // Streaming end; messages already populated.
})

EventsOn('voiceTranscript', (data: { text: string; is_final: boolean }) => {
  if (data.is_final && data.text) {
    messages.value.push({ role: 'user', content: data.text })
    messages.value.push({ role: 'assistant', content: '' })
  }
})

EventsOn('overlayConfigChanged', async () => {
  try {
    overlayConfig.value = await getOverlayConfig()
  } catch (err) {
    console.error('[overlayConfigChanged] failed to reload:', err)
  }
})

EventsOn('overlayVisibilityChanged', (visible: unknown) => {
  overlayVisible.value = !!visible
})

// ---- console 重定向到日志面板 ----
function redirectConsole(
  original: (...args: unknown[]) => void,
  level: string,
) {
  return (...args: unknown[]) => {
    original(...args)
    args.forEach((arg) => {
      addLog(level, typeof arg === 'string' ? arg : JSON.stringify(arg))
    })
  }
}

console.log = redirectConsole(console.log.bind(console), 'debug')
console.warn = redirectConsole(console.warn.bind(console), 'warn')
console.error = redirectConsole(console.error.bind(console), 'error')

// ---- 启动 ----
onMounted(() => {
  console.log('Aurora initialized (Vue)')
  try { frontendEcho('frontend-loaded') } catch { /* ignore */ }
  getActiveGame().then((g) => { activeGame.value = g }).catch(() => { /* ignore */ })
  getOverlayConfig().then((c) => {
    overlayConfig.value = c
  }).catch((err) => {
    console.error('[init] failed to load overlay config:', err)
  })
  getOverlayVisible().then((v) => {
    overlayVisible.value = v
  }).catch((err) => {
    console.error('[init] failed to load overlay visible:', err)
  })
})

// enabled 变化 → 自动同步可见性
watch(
  () => overlayConfig.value.enabled,
  (enabled) => {
    overlayVisible.value = enabled
  },
)

// ---- Overlay 控制 ----
async function onOverlaySave(cfg: OverlayConfig) {
  try {
    await saveOverlayConfig(cfg)
  } catch (err) {
    console.error('[onOverlaySave] failed:', err)
  }
}

async function onOverlayToggleVisible(visible: boolean) {
  overlayVisible.value = visible
  try {
    await setOverlayVisible(visible)
  } catch (err) {
    console.error('[onOverlayToggleVisible] failed:', err)
  }
}

async function onOverlayClose() {
  overlayVisible.value = false
  try {
    await setOverlayVisible(false)
  } catch (err) {
    console.error('[onOverlayClose] failed:', err)
  }
}

// ---- Sidebar 状态显示 ----
//
// 真实状态来自现有事件：
//   - agentBusy: handleSubmit 期间为 true
//   - overlayVisible: 反映当前 Overlay 真实可见状态
//   - activeGame: 来自后端 gameDetect
const statusLabel = computed(() => {
  if (agentBusy.value) return { text: 'Thinking', tone: 'busy' as const }
  if (overlayVisible.value) return { text: 'Overlay on', tone: 'streaming' as const }
  return { text: 'Ready', tone: 'idle' as const }
})
</script>

<template>
  <div class="app-shell">
    <!-- Sidebar -->
    <aside class="sidebar">
      <div class="sidebar-brand">
        <div class="brand-mark">A</div>
        <div class="brand-text">
          <div class="brand-name">Aurora</div>
          <div class="brand-tag">AI Game Assistant</div>
        </div>
      </div>

      <nav class="sidebar-nav">
        <button
          v-for="item in navItems"
          :key="item.key"
          :class="['nav-item', { active: activeView === item.key }]"
          @click="activeView = item.key"
        >
          <span class="nav-icon" v-html="item.icon"></span>
          <span class="nav-label">{{ item.label }}</span>
        </button>
      </nav>

      <div class="sidebar-footer">
        <div v-if="activeGame.active" class="game-chip" :title="activeGame.window">
          <span class="game-dot"></span>
          <span class="game-name">{{ activeGame.name }}</span>
        </div>
        <div class="status-pill" :class="`tone-${statusLabel.tone}`">
          <span class="status-dot"></span>
          <span>{{ statusLabel.text }}</span>
        </div>
      </div>
    </aside>

    <!-- Content -->
    <main class="app-main">
      <ChatView
        v-if="activeView === 'chat'"
        :messages="messages"
        :busy="agentBusy"
        @submit="handleSubmit"
      />
      <OverlayView
        v-else-if="activeView === 'overlay'"
        :config="overlayConfig"
        :visible="overlayVisible"
        :messages="messages"
        @save="onOverlaySave"
        @toggle-visible="onOverlayToggleVisible"
      />
      <SettingsView v-else-if="activeView === 'settings'">
        <Settings />
      </SettingsView>
      <LogsView
        v-else-if="activeView === 'logs'"
        :logs="logs"
        @clear="clearLogs"
      />
    </main>
  </div>

  <!-- Global layers (not part of view content) -->
  <QuickInput v-model:visible="quickInputVisible" @submit="handleSubmit" />
  <VoiceIndicator />
  <Overlay
    v-if="overlayVisible"
    :messages="messages"
    :config="overlayConfig"
    @close="onOverlayClose"
  />
</template>

<script lang="ts">
const navItems = [
  {
    key: 'chat' as const,
    label: 'Chat',
    icon: '<svg width="16" height="16" viewBox="0 0 16 16" fill="none"><path d="M2 4.5C2 3.67 2.67 3 3.5 3h9c.83 0 1.5.67 1.5 1.5v6c0 .83-.67 1.5-1.5 1.5H6l-2.5 2v-2H3.5C2.67 12 2 11.33 2 10.5v-6z" stroke="currentColor" stroke-width="1.4" stroke-linejoin="round"/></svg>',
  },
  {
    key: 'overlay' as const,
    label: 'Overlay',
    icon: '<svg width="16" height="16" viewBox="0 0 16 16" fill="none"><rect x="2" y="2.5" width="12" height="9" rx="1.5" stroke="currentColor" stroke-width="1.4"/><path d="M5 14h6" stroke="currentColor" stroke-width="1.4" stroke-linecap="round"/></svg>',
  },
  {
    key: 'settings' as const,
    label: 'Settings',
    icon: '<svg width="16" height="16" viewBox="0 0 16 16" fill="none"><circle cx="8" cy="8" r="2" stroke="currentColor" stroke-width="1.4"/><path d="M8 1.5v2M8 12.5v2M14.5 8h-2M3.5 8h-2M12.6 3.4l-1.4 1.4M4.8 11.2l-1.4 1.4M12.6 12.6l-1.4-1.4M4.8 4.8L3.4 3.4" stroke="currentColor" stroke-width="1.4" stroke-linecap="round"/></svg>',
  },
  {
    key: 'logs' as const,
    label: 'Logs',
    icon: '<svg width="16" height="16" viewBox="0 0 16 16" fill="none"><rect x="2.5" y="2" width="11" height="12" rx="1.5" stroke="currentColor" stroke-width="1.4"/><path d="M5 5.5h6M5 8.5h6M5 11.5h3.5" stroke="currentColor" stroke-width="1.4" stroke-linecap="round"/></svg>',
  },
]
</script>

<style scoped>
.app-shell {
  display: flex;
  height: 100vh;
  width: 100vw;
  background: var(--bg-primary);
  overflow: hidden;
}

/* ---- Sidebar ---- */
.sidebar {
  width: var(--sidebar-width);
  flex-shrink: 0;
  background: var(--bg-secondary);
  border-right: 1px solid var(--border-color);
  display: flex;
  flex-direction: column;
  padding: var(--space-4) var(--space-3) var(--space-3);
}

.sidebar-brand {
  display: flex;
  align-items: center;
  gap: var(--space-2);
  padding: var(--space-1) var(--space-2) var(--space-4);
  border-bottom: 1px solid var(--border-color);
  margin-bottom: var(--space-3);
}

.brand-mark {
  width: 32px;
  height: 32px;
  border-radius: var(--radius-lg);
  background: linear-gradient(135deg, var(--accent-color), var(--status-success));
  color: var(--text-on-accent);
  display: flex;
  align-items: center;
  justify-content: center;
  font-weight: 700;
  font-size: 16px;
  letter-spacing: -0.02em;
}

.brand-text {
  display: flex;
  flex-direction: column;
  gap: 1px;
}

.brand-name {
  font-size: 15px;
  font-weight: 600;
  letter-spacing: -0.01em;
  color: var(--text-primary);
}

.brand-tag {
  font-size: 10px;
  letter-spacing: 0.06em;
  text-transform: uppercase;
  color: var(--text-secondary);
}

.sidebar-nav {
  flex: 1;
  display: flex;
  flex-direction: column;
  gap: var(--space-1);
  margin-top: var(--space-1);
}

.nav-item {
  display: flex;
  align-items: center;
  gap: var(--space-2);
  padding: var(--space-2);
  background: transparent;
  border: 0;
  border-radius: var(--radius-md);
  color: var(--text-secondary);
  font-size: 13px;
  text-align: left;
  cursor: pointer;
  transition: background var(--transition-fast), color var(--transition-fast);
}

.nav-item:hover {
  background: var(--hover-overlay);
  color: var(--text-primary);
}

.nav-item.active {
  background: var(--bg-primary);
  color: var(--text-primary);
  box-shadow: inset 0 0 0 1px var(--border-color);
}

.nav-item.active .nav-icon {
  color: var(--accent-color);
}

.nav-icon {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 16px;
  height: 16px;
  color: inherit;
  flex-shrink: 0;
}

.nav-label {
  flex: 1;
  font-weight: 500;
}

.sidebar-footer {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  margin-top: auto;
  padding-top: var(--space-3);
  border-top: 1px solid var(--border-color);
}

.game-chip {
  display: flex;
  align-items: center;
  gap: var(--space-2);
  padding: var(--space-1) var(--space-2);
  background: var(--status-info-bg);
  border: 1px solid var(--status-info-bd);
  border-radius: var(--radius-pill);
  font-size: 11px;
  color: var(--status-info);
}

.game-dot {
  width: 6px;
  height: 6px;
  background: var(--status-info);
  border-radius: 50%;
}

.game-name {
  font-weight: 500;
}

.status-pill {
  display: flex;
  align-items: center;
  gap: var(--space-2);
  padding: var(--space-2);
  font-size: 11px;
  font-weight: 500;
  border-radius: var(--radius-md);
  border: 1px solid transparent;
}

.status-pill .status-dot {
  width: 6px;
  height: 6px;
  border-radius: 50%;
}

.status-pill.tone-idle {
  background: var(--status-success-bg);
  border-color: var(--status-success-bd);
  color: var(--status-success);
}
.status-pill.tone-idle .status-dot {
  background: var(--status-success);
}

.status-pill.tone-busy {
  background: var(--status-busy-bg);
  border-color: var(--status-busy-bd);
  color: var(--status-busy);
}
.status-pill.tone-busy .status-dot {
  background: var(--status-busy);
  animation: pulse 1s ease-in-out infinite;
}

.status-pill.tone-streaming {
  background: var(--status-streaming-bg);
  border-color: var(--status-streaming-bd);
  color: var(--status-streaming);
}
.status-pill.tone-streaming .status-dot {
  background: var(--status-streaming);
}

@keyframes pulse {
  0%, 100% { opacity: 1; }
  50% { opacity: 0.4; }
}

/* ---- Content ---- */
.app-main {
  flex: 1;
  display: flex;
  min-width: 0;
  background: var(--bg-primary);
  overflow: hidden;
}
</style>