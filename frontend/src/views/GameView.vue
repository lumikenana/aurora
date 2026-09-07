<script setup lang="ts">
// GameView - 游戏状态页（UI 重构新增）
//
// 职责：
//   - 展示当前检测到的游戏（来自 gameDetect 事件 / getActiveGame）
//   - 展示 GameContext 概要（GetGameContext binding，仅 Snapshot，不触发检测）
//
// 不做：
//   - 不自己实现游戏检测逻辑（检测在后端插件内，不变更 CLAUDE.md §2）
//   - 不显示猜测性内容（Unknown 保持 Unknown，CLAUDE.md §5）

import { ref, computed, watch, onMounted } from 'vue'
import type { ActiveGame, GameContextResponse } from '../api/backend'
import { getGameContext } from '../api/backend'

const props = defineProps<{
  activeGame: ActiveGame
}>()

const ctx = ref<GameContextResponse | null>(null)
const loading = ref(false)

async function refresh() {
  loading.value = true
  try {
    ctx.value = await getGameContext()
  } catch (err) {
    console.error('[game] refresh failed:', err)
  } finally {
    loading.value = false
  }
}

watch(
  () => props.activeGame,
  () => {
    refresh()
  },
)

onMounted(refresh)

// ---- 展示字段（Unknown 保持 Unknown，不编造） ----
const ctxRows = computed(() => {
  const c = ctx.value
  if (!c || !c.available) return []
  return [
    { key: 'Game', value: c.name || 'Unknown' },
    { key: 'Window', value: c.window_title || 'Unknown' },
    { key: 'Phase', value: c.phase || 'Unknown' },
    { key: 'Detected at', value: c.detected_at || 'Unknown' },
  ]
})
</script>

<template>
  <div class="game-view">
    <header class="view-header">
      <h1>Game</h1>
      <p class="view-subtitle">
        What Aurora sees in the current game session
      </p>
    </header>

    <div class="game-content">
      <div class="game-card">
        <div v-if="activeGame.active" class="game-active-head">
          <span class="game-dot"></span>
          <span class="game-name">{{ activeGame.name }}</span>
          <span class="game-window" :title="activeGame.window">{{ activeGame.window }}</span>
        </div>
        <div v-else class="game-idle">
          <span class="game-idle-dot"></span>
          <span>No game detected</span>
        </div>

        <div v-if="ctx && ctx.available" class="ctx-rows">
          <div v-for="row in ctxRows" :key="row.key" class="ctx-row">
            <span class="ctx-key">{{ row.key }}</span>
            <span class="ctx-value">{{ row.value }}</span>
          </div>
        </div>
        <div v-else class="ctx-empty">
          <template v-if="activeGame.active">
            No game context snapshot available yet — it appears once the game exposes state.
          </template>
          <template v-else>
            Start a supported game to see context here.
          </template>
        </div>

        <div class="ctx-refresh">
          <button class="btn" :disabled="loading" @click="refresh">Refresh</button>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.game-view {
  display: flex;
  flex-direction: column;
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
  letter-spacing: 0.01em;
}

.view-subtitle {
  font-size: 12px;
  color: var(--text-secondary);
  margin-top: 2px;
}

.game-content {
  flex: 1;
  overflow-y: auto;
  padding: var(--space-5) var(--space-7);
  min-height: 0;
}

.game-card {
  background: var(--bg-secondary);
  border: 1px solid var(--border-color);
  border-radius: var(--radius-lg);
  max-width: 720px;
  overflow: hidden;
}

.game-active-head {
  display: flex;
  align-items: center;
  gap: var(--space-2);
  padding: var(--space-3) var(--space-4);
  border-bottom: 1px solid var(--border-subtle);
}

.game-dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: var(--status-success);
  flex-shrink: 0;
}

.game-name {
  font-size: 14px;
  font-weight: 600;
  color: var(--text-primary);
}

.game-window {
  font-size: 11px;
  color: var(--text-muted);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.game-idle {
  display: flex;
  align-items: center;
  gap: var(--space-2);
  padding: var(--space-3) var(--space-4);
  border-bottom: 1px solid var(--border-subtle);
  color: var(--text-muted);
  font-size: 13px;
}

.game-idle-dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: var(--text-faint);
  flex-shrink: 0;
}

.ctx-rows {
  display: flex;
  flex-direction: column;
}

.ctx-row {
  display: flex;
  gap: var(--space-4);
  padding: var(--space-2) var(--space-4);
  border-bottom: 1px solid var(--border-subtle);
  font-size: 12px;
}

.ctx-key {
  width: 90px;
  flex-shrink: 0;
  color: var(--text-muted);
}

.ctx-value {
  color: var(--text-default);
  word-break: break-all;
}

.ctx-empty {
  padding: var(--space-4);
  font-size: 12px;
  color: var(--text-muted);
  line-height: 1.6;
}

.ctx-refresh {
  padding: var(--space-3) var(--space-4);
  display: flex;
  justify-content: flex-end;
}
</style>
