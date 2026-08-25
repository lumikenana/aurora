<script setup lang="ts">
import { ref, watch, nextTick } from 'vue'
import { cancelQuickInput } from '../api/backend'

const props = defineProps<{
  visible: boolean
}>()

const emit = defineEmits<{
  (e: 'submit', text: string): void
  (e: 'update:visible', value: boolean): void
}>()

function close() {
  emit('update:visible', false)
}

const text = ref('')
const textarea = ref<HTMLTextAreaElement | null>(null)

// 显示时自动聚焦输入框
watch(
  () => props.visible,
  async (visible) => {
    if (visible) {
      text.value = ''
      await nextTick()
      textarea.value?.focus()
      window.focus()
    }
  },
)

async function onKeydown(e: KeyboardEvent) {
  // Esc 关闭
  if (e.key === 'Escape') {
    e.preventDefault()
    try { await cancelQuickInput() } catch { /* ignore */ }
    text.value = ''
    close()
    return
  }

  // Enter 提交（Shift + Enter 换行）
  if (e.key === 'Enter' && !e.shiftKey) {
    e.preventDefault()
    const value = text.value.trim()
    if (value) {
      emit('submit', value)
    } else {
      try { await cancelQuickInput() } catch { /* ignore */ }
      close()
    }
    text.value = ''
  }
}

function onOverlayClick(e: MouseEvent) {
  // 点击遮罩层关闭（点击输入框本身除外）
  if (e.target === e.currentTarget) {
    text.value = ''
    close()
  }
}
</script>

<template>
  <div v-if="visible" class="quick-input-overlay" @click="onOverlayClick">
    <div class="quick-input-container">
      <textarea
        ref="textarea"
        v-model="text"
        placeholder="输入你的问题，按 Enter 发送，Shift + Enter 换行，Esc 关闭"
        rows="4"
        autocomplete="off"
        @keydown="onKeydown"
      ></textarea>
      <div class="hint-bar">
        <span class="hint">Enter 发送 · Esc 关闭</span>
      </div>
    </div>
  </div>
</template>

<style scoped>
.quick-input-overlay {
  position: fixed;
  top: 0;
  left: 0;
  right: 0;
  bottom: 0;
  background-color: rgba(0, 0, 0, 0.5);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 9999;
}

.quick-input-container {
  width: 560px;
  max-width: 90vw;
  background-color: var(--bg-primary);
  border-radius: 8px;
  border: 2px solid var(--border-color);
  box-shadow: 0 8px 32px rgba(0, 0, 0, 0.6);
  overflow: hidden;
}

textarea {
  width: 100%;
  background-color: transparent;
  color: var(--text-primary);
  border: none;
  outline: none;
  padding: 16px;
  font-size: 16px;
  line-height: 1.5;
  resize: none;
  font-family: inherit;
}

textarea::placeholder {
  color: var(--text-secondary);
}

textarea:focus {
  outline: none;
}

.hint-bar {
  border-top: 1px solid rgba(255, 255, 255, 0.1);
  padding: 6px 16px;
  background-color: var(--bg-secondary);
}

.hint {
  color: var(--text-secondary);
  font-size: 12px;
}
</style>
