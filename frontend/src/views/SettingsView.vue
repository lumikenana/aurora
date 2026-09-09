<script setup lang="ts">
// SettingsView - Settings 容器页面
//
// Settings.vue 现在包含所有四大 Tab：AI / Voice / Overlay / Shortcuts。
// 本组件作为容器，传递 Overlay 相关 props 给 Settings.vue。

import type { OverlayConfig, ChatMessage } from '../api/backend'

defineProps<{
  overlayConfig?: OverlayConfig
  overlayVisible?: boolean
  overlayMessages?: ChatMessage[]
}>()

defineEmits<{
  (e: 'toggle-overlay-visible', visible: boolean): void
}>()
</script>

<template>
  <div class="settings-view">
    <header class="view-header">
      <h1>Settings</h1>
      <p class="view-subtitle">
        Configure Aurora's AI, voice, overlay, and shortcuts
      </p>
    </header>

    <div class="settings-content">
      <slot />
    </div>
  </div>
</template>

<style scoped>
.settings-view {
  display: flex;
  flex-direction: column;
  width: 100%;
  height: 100%;
  min-height: 0;
}

.view-header {
  padding: 20px 28px 12px;
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

.settings-content {
  flex: 1;
  overflow-y: auto;
  padding: 16px 28px 24px;
  min-height: 0;
}
</style>