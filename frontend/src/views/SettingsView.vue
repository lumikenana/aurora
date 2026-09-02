<script setup lang="ts">
// SettingsView - Settings 容器页面
//
// 现有 Settings.vue 组件承担所有真实配置项；
// 本阶段保持原有结构（不重新拆分类），但 Overlay 相关配置已移至 OverlayView。
//
// 未来 Settings Polish Phase 可在此处引入分类侧栏。
</script>

<template>
  <div class="settings-view">
    <header class="view-header">
      <h1>Settings</h1>
      <p class="view-subtitle">
        Configure Aurora's AI, voice, and shortcuts
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