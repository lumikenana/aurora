<script setup lang="ts">
// OverlayPreview - 主窗口配置页内的纯视觉 HUD 模拟
//
// 职责：
//   - 渲染 HUD 样式预览（opacity / font_size / anchor 切换 / 消息流）
//   - 不挂 edit-mode chrome（edit-bar / resize-handle / Lock 按钮）
//   - 不调任何 IPC（moveOverlayWindow / commitOverlayGeometry 等）
//   - 由 OverlayView 内的 .preview-frame (position: absolute; inset: 0) 承载
//
// 拆分原因（UI 重构清扫）：
//   - 原 Overlay.vue 同时承担 native + preview 两种形态，CSS 互覆盖混乱
//   - Preview 逃出 preview-frame 容器（之前 position: fixed + anchor-*）

import { ref, watch, nextTick, computed, onMounted } from 'vue'
import type { ChatMessage, OverlayConfig } from '../api/backend'

const props = defineProps<{
  messages: ChatMessage[]
  config: OverlayConfig
}>()

const bodyRef = ref<HTMLElement | null>(null)
const streaming = ref(false)

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

let lastAssistantLen = 0
watch(
  () => props.messages,
  (msgs) => {
    const lastAssistant = [...msgs].reverse().find((m) => m.role === 'assistant')
    const curLen = lastAssistant ? lastAssistant.content.length : 0
    streaming.value = curLen > lastAssistantLen
    lastAssistantLen = curLen
    if (streaming.value) {
      setTimeout(() => {
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
    class="preview-root"
    :class="[`anchor-${config.anchor}`]"
    :style="overlayStyle"
    role="region"
    aria-label="Overlay Preview"
  >
    <header class="preview-header">
      <div class="preview-brand">
        <span class="brand-dot" :class="{ streaming: isStreaming }"></span>
        <span class="brand-name">Aurora</span>
        <span v-if="isStreaming" class="brand-status">generating…</span>
      </div>
    </header>

    <div ref="bodyRef" class="preview-body">
      <template v-if="messages.length === 0">
        <div class="preview-empty">
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
          class="preview-msg"
          :class="msg.role"
        >
          <div class="preview-msg-role">{{ msg.role === 'user' ? 'You' : 'Aurora' }}</div>
          <div class="preview-msg-content">{{ msg.content }}</div>
        </div>
      </template>
    </div>
  </div>
</template>

<style scoped>
/* Preview root：position: absolute 限制在 .preview-frame 内（OverlayView 持有），
   不影响其他页面。width/height 由 anchor-* 类决定四角定位。 */
.preview-root {
  position: absolute;
  width: 280px;
  max-width: calc(100% - 32px);
  max-height: calc(100% - 32px);
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

.preview-root.anchor-top-left {
  top: 12px;
  left: 12px;
}
.preview-root.anchor-top-right {
  top: 12px;
  right: 12px;
}
.preview-root.anchor-bottom-left {
  bottom: 12px;
  left: 12px;
}
.preview-root.anchor-bottom-right {
  bottom: 12px;
  right: 12px;
}

.preview-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 8px 12px;
  border-bottom: 1px solid rgba(255, 255, 255, 0.06);
  background: rgba(255, 255, 255, 0.02);
  user-select: none;
  flex-shrink: 0;
}

.preview-brand {
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
  animation: preview-pulse 1.4s ease-in-out infinite;
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

.preview-body {
  flex: 1;
  overflow-y: auto;
  padding: 10px 12px;
  scroll-behavior: smooth;
  min-height: 0;
}

.preview-empty {
  color: #8a93a3;
  font-size: 0.92em;
  text-align: center;
  padding: 18px 8px;
  line-height: 1.7;
}
.preview-empty kbd {
  background: rgba(255, 255, 255, 0.06);
  border: 1px solid rgba(255, 255, 255, 0.1);
  border-radius: 3px;
  padding: 1px 5px;
  font-size: 0.92em;
  font-family: ui-monospace, monospace;
}

.preview-msg {
  margin-bottom: 8px;
  padding: 6px 8px;
  border-radius: 6px;
  background: rgba(255, 255, 255, 0.03);
  border-left: 2px solid transparent;
  word-wrap: break-word;
  white-space: pre-wrap;
}
.preview-msg.user {
  border-left-color: #4c8bf5;
  background: rgba(76, 139, 245, 0.06);
}
.preview-msg.assistant {
  border-left-color: #4ec9b0;
  background: rgba(78, 201, 176, 0.05);
}

.preview-msg-role {
  font-size: 0.78em;
  color: #8a93a3;
  margin-bottom: 2px;
  letter-spacing: 0.04em;
  text-transform: uppercase;
}

.preview-msg-content {
  font-size: 1em;
  color: #e6e8ee;
}

@keyframes preview-pulse {
  0%, 100% {
    box-shadow: 0 0 0 0 rgba(78, 201, 176, 0.4);
  }
  50% {
    box-shadow: 0 0 0 6px rgba(78, 201, 176, 0);
  }
}
</style>
