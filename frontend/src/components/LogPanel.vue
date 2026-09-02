<script setup lang="ts">
// LogPanel.vue - 日志面板（Phase 6 重构）
//
// 职责：
//   - 渲染日志条目
//   - 搜索（大小写不敏感，即时过滤）
//   - 级别过滤（仅暴露真实存在的级别）
//   - 清空（仅清 UI 缓冲区，不删除磁盘文件 — 任务 B4）
//   - 智能 auto-scroll：用户在底部时跟随，向上滚时不强制跳
//
// 数据：
//   - 父组件 App.vue 持有 logs（最多 500 条 in-memory）
//   - 本组件接收 props.logs，并做本地 filter；不修改父状态（除通过 emit('clear')）
//
// 不负责：
//   - 后端日志收集（已由 App.vue 的 'log' event 处理）
//   - 日志文件持久化（任务 B4 明确不动）

import { ref, computed, nextTick, watch, onMounted } from 'vue'

export interface LogEntry {
  level: string  // 'debug' | 'info' | 'warn' | 'error' | 其他
  message: string
  time: string    // 来自 toLocaleTimeString()
}

const props = defineProps<{
  logs: LogEntry[]
}>()

const emit = defineEmits<{
  (e: 'clear'): void
}>()

// ---- 搜索 / 过滤 ----

const searchQuery = ref('')
const activeLevel = ref<'all' | 'debug' | 'info' | 'warn' | 'error'>('all')

// 实际出现过的级别（用于决定 level filter 显示哪些 chip）
const availableLevels = computed(() => {
  const set = new Set<string>()
  for (const e of props.logs) {
    const lvl = (e.level || '').toLowerCase()
    if (lvl) set.add(lvl)
  }
  // 按常见顺序
  return ['debug', 'info', 'warn', 'error'].filter((l) => set.has(l))
})

const filteredLogs = computed(() => {
  const q = searchQuery.value.trim().toLowerCase()
  return props.logs.filter((entry) => {
    if (activeLevel.value !== 'all' && entry.level !== activeLevel.value) {
      return false
    }
    if (!q) return true
    return (
      entry.message.toLowerCase().includes(q) ||
      entry.level.toLowerCase().includes(q)
    )
  })
})

// ---- 智能 auto-scroll ----

const containerRef = ref<HTMLElement | null>(null)
const NEAR_BOTTOM_PX = 80
const userNearBottom = ref(true)
const showJumpToLatest = ref(false)

function updateScrollState() {
  const el = containerRef.value
  if (!el) return
  const distance = el.scrollHeight - el.scrollTop - el.clientHeight
  userNearBottom.value = distance <= NEAR_BOTTOM_PX
  showJumpToLatest.value = !userNearBottom.value
}

function scrollToLatest(smooth = false) {
  const el = containerRef.value
  if (!el) return
  el.scrollTo({ top: el.scrollHeight, behavior: smooth ? 'smooth' : 'auto' })
}

// 过滤后列表 / 新日志进入时 — 仅当用户在底部时跟随
watch(
  [filteredLogs, () => props.logs.length],
  async () => {
    await nextTick()
    if (userNearBottom.value) {
      scrollToLatest(false)
    } else {
      showJumpToLatest.value = true
    }
  },
  { flush: 'post' },
)

onMounted(async () => {
  await nextTick()
  updateScrollState()
})

function onContainerScroll() {
  updateScrollState()
}

// ---- 级别 token 映射 ----
//
// 只在确实存在该级别时使用对应 status token；未知级别降级到 default。
const SEVERITY_CLASS: Record<string, string> = {
  debug: 'lvl-debug',
  info: 'lvl-info',
  warn: 'lvl-warn',
  error: 'lvl-error',
}

function severityClass(level: string): string {
  return SEVERITY_CLASS[level] || 'lvl-info'
}

// ---- 清空 ----

function handleClear() {
  emit('clear')
}
</script>

<template>
  <div class="log-panel">
    <div class="log-toolbar">
      <input
        v-model="searchQuery"
        type="search"
        class="log-search"
        placeholder="Search logs..."
        aria-label="Search logs"
      />
      <div v-if="availableLevels.length > 0" class="log-levels" role="tablist" aria-label="Filter by level">
        <button
          :class="['log-level', { active: activeLevel === 'all' }]"
          type="button"
          @click="activeLevel = 'all'"
        >All</button>
        <button
          v-for="lvl in availableLevels"
          :key="lvl"
          :class="['log-level', `level-${lvl}`, { active: activeLevel === lvl }]"
          type="button"
          @click="activeLevel = lvl"
        >{{ lvl.charAt(0).toUpperCase() + lvl.slice(1) }}</button>
      </div>
      <button
        type="button"
        class="log-clear"
        :disabled="logs.length === 0"
        aria-label="Clear log buffer"
        @click="handleClear"
      >Clear</button>
    </div>

    <div
      class="log-container"
      ref="containerRef"
      @scroll="onContainerScroll"
    >
      <template v-if="filteredLogs.length === 0">
        <div class="log-empty">
          <div class="log-empty-icon" aria-hidden="true">📋</div>
          <div class="log-empty-title">
            {{ logs.length === 0 ? 'No logs yet' : 'No matching logs' }}
          </div>
          <div class="log-empty-hint">
            {{
              logs.length === 0
                ? 'Aurora will report runtime events here.'
                : 'Try a different search or level filter.'
            }}
          </div>
        </div>
      </template>
      <template v-else>
        <div
          v-for="(entry, i) in filteredLogs"
          :key="i"
          class="log-entry"
          :class="severityClass(entry.level)"
        >
          <span class="log-time">{{ entry.time }}</span>
          <span class="log-level-tag" :class="severityClass(entry.level)">{{ (entry.level || 'info').toUpperCase() }}</span>
          <span class="log-msg">{{ entry.message }}</span>
        </div>
      </template>

      <button
        v-if="showJumpToLatest && filteredLogs.length > 0"
        type="button"
        class="log-jump-btn"
        aria-label="Jump to latest log"
        @click="scrollToLatest(true)"
      >↓ Latest</button>
    </div>
  </div>
</template>

<style scoped>
.log-panel {
  display: flex;
  flex-direction: column;
  background: var(--bg-secondary);
  border-radius: var(--radius-lg);
  border: 1px solid var(--border-default);
  min-height: 0;
  flex: 1;
  overflow: hidden;
}

.log-toolbar {
  display: flex;
  align-items: center;
  gap: var(--space-2);
  padding: var(--space-2) var(--space-3);
  border-bottom: 1px solid var(--border-default);
  background: var(--bg-primary);
  flex-wrap: wrap;
}

.log-search {
  flex: 1 1 200px;
  min-width: 160px;
  background: var(--bg-secondary);
  border: 1px solid var(--border-default);
  border-radius: var(--radius-sm);
  color: var(--text-default);
  font-size: 12px;
  padding: 5px 10px;
  outline: none;
  font-family: inherit;
  transition: border-color var(--transition-fast);
}
.log-search:focus {
  border-color: var(--accent-color);
}
.log-search::placeholder {
  color: var(--text-faint);
}

.log-levels {
  display: flex;
  gap: 4px;
  flex-wrap: wrap;
}

.log-level {
  background: transparent;
  border: 1px solid var(--border-default);
  color: var(--text-muted);
  padding: 3px 10px;
  font-size: 11px;
  font-weight: 500;
  border-radius: var(--radius-pill);
  cursor: pointer;
  font-family: inherit;
  letter-spacing: 0.02em;
  transition: color var(--transition-fast), background var(--transition-fast), border-color var(--transition-fast);
}
.log-level:hover {
  color: var(--text-default);
}
.log-level.active {
  background: var(--bg-secondary);
  color: var(--text-default);
  border-color: var(--border-strong);
}
.log-level.level-error.active {
  border-color: var(--status-danger);
  color: var(--status-danger);
}
.log-level.level-warn.active {
  border-color: var(--status-warn);
  color: var(--status-warn);
}

.log-clear {
  background: transparent;
  border: 1px solid var(--border-default);
  color: var(--text-muted);
  padding: 3px 12px;
  font-size: 11px;
  border-radius: var(--radius-sm);
  cursor: pointer;
  font-family: inherit;
  transition: color var(--transition-fast), border-color var(--transition-fast);
}
.log-clear:hover:not(:disabled) {
  color: var(--text-default);
  border-color: var(--border-strong);
}
.log-clear:disabled {
  opacity: 0.4;
  cursor: not-allowed;
}

.log-container {
  flex: 1;
  overflow-y: auto;
  font-family: ui-monospace, SFMono-Regular, "Cascadia Mono", Menlo, Consolas, monospace;
  font-size: 12px;
  line-height: 1.55;
  padding: var(--space-2) 0;
  position: relative;
  min-height: 0;
}

.log-entry {
  display: grid;
  grid-template-columns: 80px 60px 1fr;
  gap: var(--space-2);
  padding: 2px var(--space-3);
  border-bottom: 1px solid var(--border-subtle);
  align-items: baseline;
}

.log-entry:hover {
  background: var(--hover-overlay);
}

.log-time {
  color: var(--text-faint);
  font-size: 11px;
  white-space: nowrap;
}

.log-level-tag {
  font-size: 10px;
  font-weight: 600;
  letter-spacing: 0.06em;
  text-align: center;
  padding: 1px 0;
  border-radius: 2px;
  align-self: center;
}

.log-msg {
  word-break: break-word;
  white-space: pre-wrap;
  color: var(--text-default);
}

/* ---- Severity colors via Phase 5 status tokens ---- */
.lvl-debug .log-level-tag,
.lvl-debug .log-msg { color: var(--text-faint); }

.lvl-info .log-level-tag { color: var(--status-info); }
.lvl-info .log-msg { color: var(--text-default); }

.lvl-warn .log-level-tag { color: var(--status-warn); }
.lvl-warn .log-msg { color: var(--status-warn); }

.lvl-error .log-level-tag { color: var(--status-danger); }
.lvl-error .log-msg { color: var(--status-danger); }

/* ---- Empty state ---- */
.log-empty {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  height: 100%;
  text-align: center;
  color: var(--text-muted);
  padding: var(--space-7) var(--space-4);
  gap: var(--space-2);
}
.log-empty-icon {
  font-size: 28px;
  opacity: 0.4;
  margin-bottom: var(--space-1);
}
.log-empty-title {
  font-size: 14px;
  font-weight: 500;
  color: var(--text-default);
}
.log-empty-hint {
  font-size: 12px;
  color: var(--text-muted);
}

/* ---- Jump to latest ---- */
.log-jump-btn {
  position: sticky;
  bottom: var(--space-3);
  left: 50%;
  transform: translateX(-50%);
  display: block;
  margin: 0 auto;
  padding: 4px 12px;
  background: var(--bg-secondary);
  color: var(--text-default);
  border: 1px solid var(--border-default);
  border-radius: var(--radius-pill);
  font-size: 11px;
  font-weight: 500;
  cursor: pointer;
  font-family: inherit;
  box-shadow: var(--shadow-md);
  transition: background var(--transition-fast);
}
.log-jump-btn:hover {
  background: var(--surface-2, var(--bg-primary));
}
</style>
