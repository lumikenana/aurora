<script setup lang="ts">
// ToastHost - 全局 Toast 渲染容器（配合 composables/useToast）
//
// 挂载点：App.vue / overlay-main.ts / quickinput-main.ts（每个 webview 入口一个，
// Toast 是纯前端视觉层，不跨窗口）。

import { useToast } from '../composables/useToast'

const toast = useToast()
</script>

<template>
  <div class="toast-host" aria-live="polite">
    <div
      v-for="item in toast.items"
      :key="item.id"
      :class="['toast-item', `tone-${item.tone}`, { leaving: item.leaving }]"
      @click="toast.dismiss(item.id)"
    >
      <span class="toast-dot" aria-hidden="true"></span>
      <span>{{ item.text }}</span>
    </div>
  </div>
</template>
