<script setup lang="ts">
// Overlay - Phase 3 MVP
//
// 视觉目标：
//   - HUD Companion 风格，非 SaaS Dashboard
//   - Dark-first, minimal, low distraction
//   - 主内容区可滚动，header 固定
//
// 已知限制（任务 §6/§8）：
//   Overlay 渲染在 Main Window webview 内，因此：
//   - Main Window 隐藏/最小化/关闭时 Overlay 一并不可见
//   - 无法做到 OS 级 AlwaysOnTop（webview 组件没有此能力）
//   完整解决方案是把 Overlay 提升为独立 Wails Window（需 Window Lifecycle Phase）。
//
// 数据流：
//   App.vue 持有 messages 数组与 overlayConfig/overlayVisible 状态
//   Overlay 通过 props 接收 messages 与 overlayConfig
//   llmChunk / llmFinish 事件已在 App.vue 监听，App.vue 更新 messages 后通过
//   Vue 响应式传入 Overlay，无需 Overlay 重复监听。

import { ref, watch, nextTick, computed, onMounted } from 'vue'
import type { ChatMessage } from '../api/backend'
import type { OverlayConfig } from '../api/backend'

const props = defineProps<{
  messages: ChatMessage[]
  config: OverlayConfig
}>()

const emit = defineEmits<{
  (e: 'close'): void
}>()

const bodyRef = ref<HTMLElement | null>(null)
const streaming = ref(false)

// 最近一条消息为 assistant 且 content 末尾仍有更新 → 视为 streaming
//（App.vue 的 llmChunk 监听会把增量 append 到最后一条 assistant）
const lastMessage = computed(() => {
  const arr = props.messages
  return arr.length > 0 ? arr[arr.length - 1] : null
})

const isStreaming = computed(() => {
  const last = lastMessage.value
  if (!last) return false
  return last.role === 'assistant' && streaming.value
})

watch(
  () => lastMessage.value?.content,
  async () => {
    if (!props.config.auto_scroll) return
    await nextTick()
    if (bodyRef.value) {
      bodyRef.value.scrollTop = bodyRef.value.scrollHeight
    }
  },
)

// 通过对比 messages 数组最后一条 assistant 长度变化来粗略检测 streaming
//（更精确的方式是监听 llmChunk/llmFinish 事件，但 messages 已聚合）
let lastAssistantLen = 0
watch(
  () => props.messages,
  (msgs) => {
    const lastAssistant = [...msgs].reverse().find((m) => m.role === 'assistant')
    const curLen = lastAssistant ? lastAssistant.content.length : 0
    streaming.value = curLen > lastAssistantLen
    lastAssistantLen = curLen
    // 收到新消息后短暂保持 streaming 标识，然后淡出
    if (streaming.value) {
      setTimeout(() => {
        // 若此期间没有继续增长，则视为结束
        const nowAssistant = [...props.messages].reverse().find((m) => m.role === 'assistant')
        const nowLen = nowAssistant ? nowAssistant.content.length : 0
        if (nowLen === curLen) {
          streaming.value = false
        }
      }, 1500)
    }
  },
  { deep: true },
)

onMounted(async () => {
  await nextTick()
  if (bodyRef.value && props.config.auto_scroll) {
    bodyRef.value.scrollTop = bodyRef.value.scrollHeight
  }
})

function close() {
  emit('close')
}

function shouldShow(msg: ChatMessage): boolean {
  if (msg.role === 'user') return props.config.show_user_messages
  if (msg.role === 'assistant') return props.config.show_assistant_messages
  return true
}

const overlayStyle = computed(() => {
  const base: Record<string, string> = {
    '--overlay-opacity': String(Math.max(0.3, Math.min(1, props.config.opacity))),
  }
  switch (props.config.font_size) {
    case 'small':
      base['--overlay-font-size'] = '12px'
      break
    case 'large':
      base['--overlay-font-size'] = '15px'
      break
    case 'medium':
    default:
      base['--overlay-font-size'] = '13px'
      break
  }
  return base
})
</script>

<template>
  <div
    class="overlay-root"
    :class="[`anchor-${config.anchor}`, `font-${config.font_size}`]"
    :style="overlayStyle"
    role="region"
    aria-label="Aurora Game Overlay"
  >
    <header class="overlay-header">
      <div class="overlay-brand">
        <span class="brand-dot" :class="{ streaming: isStreaming }"></span>
        <span class="brand-name">Aurora</span>
        <span v-if="isStreaming" class="brand-status">generating…</span>
      </div>
      <button class="overlay-close" title="Hide overlay" @click="close">×</button>
    </header>

    <div ref="bodyRef" class="overlay-body">
      <template v-if="messages.length === 0">
        <div class="overlay-empty">
          No conversation yet.
          <br />
          Press <kbd>Ctrl + Alt + R</kbd> to ask Aurora.
        </div>
      </template>
      <template v-else>
        <div
          v-for="(msg, i) in messages"
          v-show="shouldShow(msg)"
          :key="i"
          class="overlay-msg"
          :class="msg.role"
        >
          <div class="overlay-msg-role">{{ msg.role === 'user' ? 'You' : 'Aurora' }}</div>
          <div class="overlay-msg-content">{{ msg.content }}</div>
        </div>
      </template>
    </div>
  </div>
</template>

<style scoped>
.overlay-root {
  position: fixed;
  z-index: 9000;
  width: 360px;
  max-width: 90vw;
  max-height: 50vh;
  display: flex;
  flex-direction: column;
  background: rgba(15, 17, 22, var(--overlay-opacity, 0.9));
  border: 1px solid rgba(255, 255, 255, 0.08);
  border-radius: 10px;
  box-shadow: 0 12px 32px rgba(0, 0, 0, 0.5);
  color: #e6e8ee;
  font-family: inherit;
  overflow: hidden;
  backdrop-filter: blur(8px);
  -webkit-backdrop-filter: blur(8px);
  font-size: var(--overlay-font-size, 13px);
  line-height: 1.5;
}

.overlay-root.anchor-top-left {
  top: 16px;
  left: 16px;
}
.overlay-root.anchor-top-right {
  top: 16px;
  right: 16px;
}
.overlay-root.anchor-bottom-left {
  bottom: 16px;
  left: 16px;
}
.overlay-root.anchor-bottom-right {
  bottom: 16px;
  right: 16px;
}

.overlay-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 8px 12px;
  border-bottom: 1px solid rgba(255, 255, 255, 0.06);
  background: rgba(255, 255, 255, 0.02);
  user-select: none;
}

.overlay-brand {
  display: flex;
  align-items: center;
  gap: 8px;
  font-weight: 500;
  letter-spacing: 0.02em;
}

.brand-dot {
  width: 6px;
  height: 6px;
  background: #5a6470;
  border-radius: 50%;
  transition: background 0.2s ease;
}
.brand-dot.streaming {
  background: #4ec9b0;
  box-shadow: 0 0 0 0 rgba(78, 201, 176, 0.4);
  animation: pulse 1.4s ease-in-out infinite;
}

.brand-name {
  color: #e6e8ee;
}

.brand-status {
  color: #8a93a3;
  font-size: 0.85em;
  font-weight: 400;
  margin-left: 4px;
}

.overlay-close {
  background: transparent;
  border: 0;
  color: #8a93a3;
  font-size: 18px;
  line-height: 1;
  cursor: pointer;
  padding: 2px 6px;
  border-radius: 4px;
  transition: background 0.15s ease, color 0.15s ease;
}
.overlay-close:hover {
  background: rgba(255, 255, 255, 0.06);
  color: #e6e8ee;
}

.overlay-body {
  flex: 1;
  overflow-y: auto;
  padding: 10px 12px;
  scroll-behavior: smooth;
}

.overlay-empty {
  color: #8a93a3;
  font-size: 0.92em;
  text-align: center;
  padding: 18px 8px;
  line-height: 1.7;
}
.overlay-empty kbd {
  background: rgba(255, 255, 255, 0.06);
  border: 1px solid rgba(255, 255, 255, 0.1);
  border-radius: 3px;
  padding: 1px 5px;
  font-size: 0.92em;
  font-family: ui-monospace, monospace;
}

.overlay-msg {
  margin-bottom: 8px;
  padding: 6px 8px;
  border-radius: 6px;
  background: rgba(255, 255, 255, 0.03);
  border-left: 2px solid transparent;
  word-wrap: break-word;
  white-space: pre-wrap;
}
.overlay-msg.user {
  border-left-color: #4c8bf5;
  background: rgba(76, 139, 245, 0.06);
}
.overlay-msg.assistant {
  border-left-color: #4ec9b0;
  background: rgba(78, 201, 176, 0.05);
}

.overlay-msg-role {
  font-size: 0.78em;
  color: #8a93a3;
  margin-bottom: 2px;
  letter-spacing: 0.04em;
  text-transform: uppercase;
}
.overlay-msg-content {
  font-size: 1em;
  color: #e6e8ee;
}

@keyframes pulse {
  0%, 100% {
    box-shadow: 0 0 0 0 rgba(78, 201, 176, 0.4);
  }
  50% {
    box-shadow: 0 0 0 6px rgba(78, 201, 176, 0);
  }
}
</style>