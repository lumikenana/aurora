<script setup lang="ts">
// LogsView - 日志页面
//
// Phase 6：作为 LogPanel 的容器，把 LogPanel 的 clear 事件冒泡到 App.vue
// （logs 数组的实际所有权在 App.vue）。

import LogPanel from '../components/LogPanel.vue'
import type { LogEntry } from '../components/LogPanel.vue'

defineProps<{
  logs: LogEntry[]
}>()

defineEmits<{
  (e: 'clear'): void
}>()
</script>

<template>
  <div class="logs-view">
    <header class="view-header">
      <h1>Logs</h1>
      <p class="view-subtitle">
        Aurora runtime events and AI request traces
      </p>
    </header>

    <div class="logs-content">
      <LogPanel :logs="logs" @clear="$emit('clear')" />
    </div>
  </div>
</template>

<style scoped>
.logs-view {
  display: flex;
  flex-direction: column;
  width: 100%;
  height: 100%;
  min-height: 0;
}

.view-header {
  padding: var(--space-5) var(--space-7) var(--space-3);
  border-bottom: 1px solid var(--border-color);
  flex-shrink: 0;
}

.view-header h1 {
  font-size: 20px;
  font-weight: 600;
  color: var(--text-primary);
}

.view-subtitle {
  font-size: 12px;
  color: var(--text-secondary);
  margin-top: 2px;
}

.logs-content {
  flex: 1;
  overflow: hidden;
  padding: var(--space-4) var(--space-7) var(--space-6);
  min-height: 0;
  display: flex;
  flex-direction: column;
}
</style>