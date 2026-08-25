<script setup lang="ts">
import { ref, watch, nextTick } from 'vue'

export interface LogEntry {
  level: string
  message: string
  time: string
}

const props = defineProps<{
  logs: LogEntry[]
}>()

const container = ref<HTMLElement | null>(null)

// 新日志时滚动到底部
watch(
  () => props.logs.length,
  async () => {
    await nextTick()
    if (container.value) {
      container.value.scrollTop = container.value.scrollHeight
    }
  },
)
</script>

<template>
  <div class="log-panel">
    <h3>系统日志</h3>
    <div class="log-container" ref="container">
      <div
        v-for="(entry, i) in logs"
        :key="i"
        class="log-entry"
        :class="entry.level"
      >[{{ entry.time }}] [{{ entry.level.toUpperCase() }}] {{ entry.message }}</div>
    </div>
  </div>
</template>

<style scoped>
.log-panel {
  margin-top: 2rem;
  background-color: var(--bg-secondary);
  border-radius: 8px;
  padding: 1rem;
  border: 1px solid var(--border-color);
  max-height: 300px;
  overflow: hidden;
  display: flex;
  flex-direction: column;
}

.log-panel h3 {
  font-size: 1rem;
  color: var(--accent-color);
  margin-bottom: 0.5rem;
}

.log-container {
  flex: 1;
  overflow-y: auto;
  font-family: monospace;
  font-size: 0.8rem;
  line-height: 1.4;
}

.log-entry {
  padding: 0.25rem 0;
  border-bottom: 1px solid rgba(255, 255, 255, 0.05);
}

.log-entry.debug {
  color: var(--text-secondary);
}

.log-entry.info {
  color: var(--text-primary);
}

.log-entry.warn {
  color: #ffa500;
}

.log-entry.error {
  color: #ff6b6b;
}
</style>
