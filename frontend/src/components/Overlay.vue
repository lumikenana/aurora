<script setup lang="ts">
// Overlay - Native HUD companion（独立 frameless webview）。
//
// 视觉目标：
//   - HUD Companion 风格，非 SaaS Dashboard
//   - Dark-first, minimal, low distraction
//   - 主内容区可滚动，header 固定
//
// 两种交互态（Native Window is the only Geometry Source of Truth）：
//   1. Locked / HUD 模式（默认）：
//        - header 不参与拖拽（--wails-draggable: no-drag）
//        - 窗口 SetResizable(false) + SetIgnoreMouseEvents(true) → 全 click-through
//        - 不显示交互 chrome（按钮、resize handle 都不在 DOM）
//   2. Edit Mode（!locked）：
//        - header 设为 native drag region（--wails-draggable: drag）
//        - 窗口 SetResizable(true)（Wails 5px 边缘 / 10px 角落 native resize）
//        - 显示 edit-bar（hint + Close + Lock）
//        - SetIgnoreMouseEvents(false) → 交互可触达
//
// 几何 / 持久化（不在前端）：
//   - OS 直接处理 drag / resize（CSS draggable region + Wails native 边缘检测）
//   - Go 侧监听 events.Common.WindowDidMove / WindowDidResize → 300ms debounce
//     → 写 cfg.Overlay.X/Y/Width/Height + Save + 广播 overlayConfigChanged
//   - WindowClosing / WindowHide 前 flush 待处理 geometry
//   - 前端 Edit Mode 不发起任何 IPC（无 SetPosition / SetSize / commit）

import { ref, watch, nextTick, computed, onMounted } from 'vue'
import type { ChatMessage, OverlayConfig } from '../api/backend'

const props = withDefaults(
  defineProps<{
    messages: ChatMessage[]
    config: OverlayConfig
    // locked：HUD 锁定态。native webview 由 overlay-main.ts 从后端状态提供。
    locked?: boolean
  }>(),
  { locked: true },
)

const emit = defineEmits<{
  (e: 'close'): void
  (e: 'lock'): void
}>()

const bodyRef = ref<HTMLElement | null>(null)
const streaming = ref(false)

// ---- streaming 检测 ----
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

const editMode = computed(() => !props.locked)

const overlayStyle = computed(() => {
  const base: Record<string, string> = {
    '--overlay-opacity': String(Math.max(0.3, Math.min(1, props.config.opacity))),
    // Edit Mode：header 设为 native drag region，OS 接管 window 移动
    // Locked：no-drag，header 上点不到 / 也不该拖（HUD click-through）
    '--wails-draggable': editMode.value ? 'drag' : 'no-drag',
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

function close() {
  emit('close')
}

function lockFromOverlay() {
  emit('lock')
}
</script>

<template>
  <div
    class="overlay-root"
    :class="{ edit: editMode }"
    :style="overlayStyle"
    role="region"
    aria-label="Aurora Game Overlay"
  >
    <!-- Edit Mode 提示条（编辑态专用 chrome） -->
    <div v-if="editMode" class="edit-bar">
      <span class="edit-hint">Drag header to move · resize from corner</span>
      <button class="edit-bar-btn" title="Hide overlay" @click="close">Close</button>
      <button class="edit-bar-btn edit-bar-btn-primary" title="Lock overlay (HUD mode)" @click="lockFromOverlay">
        Lock
      </button>
    </div>

    <header class="overlay-header">
      <div class="overlay-brand">
        <span class="brand-dot" :class="{ streaming: isStreaming }"></span>
        <span class="brand-name">Aurora</span>
        <span v-if="isStreaming" class="brand-status">generating…</span>
      </div>
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
/* Native window 模式：填满独立 frameless 窗口（四周留 8px 给阴影/圆角 + OS resize edge）。 */
.overlay-root {
  position: absolute;
  top: 8px;
  right: 8px;
  bottom: 8px;
  left: 8px;
  display: flex;
  flex-direction: column;
  /* 面板背景 alpha = cfg.Overlay.Opacity（--overlay-opacity 由 overlay-main.ts 注入）。
   * 窗口级透明由 main.go BackgroundTypeTransparent + overlay.css（html/body 透明）
   * 共同保证——任何一层不透明都会把 alpha 压平，Opacity 滑块失效。 */
  background: rgba(28, 32, 44, var(--overlay-opacity, 0.85));
  border: 1px solid rgba(255, 255, 255, 0.10);
  border-radius: 10px;
  /* 不用 box-shadow：Overlay 窗口是真正透明的（BackgroundTypeTransparent +
   * html/body transparent + DWM 去边），阴影投在窗口内透明像素上 → 在桌面/游戏上
   * 会看到一圈深色模糊边，而非浮起感。浮起层次靠边框 + 面板自身 alpha 体现即可。
   * （Win11 真实窗口服从 DWM drop shadow 由 OS 管，这里画了也没用。） */
  color: #e6e8ee;
  font-family: inherit;
  overflow: hidden;
  /* 不用 backdrop-filter：WebView 看不到身后的桌面/游戏画面（只能采样页内内容，
   * 此处页内身后是透明 body → 恒无效），且 WebView2 透明窗口 + backdrop-filter
   * 有渲染成不透明/黑块的已知问题。文字可读性由下方 text-shadow 保证。 */
  font-size: var(--overlay-font-size, 13px);
  line-height: 1.5;
  /* 低透明度（α≈0.3）时面板底几乎全透，深/浅游戏画面都会吃掉浅色文字的对比度。
   * text-shadow 会被子元素继承，给全部文字垫一层暗描边保持可读。 */
  text-shadow: 0 1px 3px rgba(0, 0, 0, 0.85), 0 0 6px rgba(0, 0, 0, 0.5);
}

/* ---- Edit Mode chrome（仅编辑态出现） ---- */
.overlay-root.edit {
  border-color: var(--accent-color, #007acc);
  /* 保留 focus ring（outline 风格），不带普通阴影（原因见 .overlay-root 注释）。 */
  box-shadow: var(--focus-ring, 0 0 0 2px rgba(0, 122, 204, 0.45));
}

.edit-bar {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  gap: 8px;
  padding: 4px 10px;
  background: rgba(0, 122, 204, 0.18);
  border-bottom: 1px solid rgba(0, 122, 204, 0.35);
  user-select: none;
  -webkit-user-select: none;
}

.edit-hint {
  flex: 1;
  font-size: 10px;
  letter-spacing: 0.03em;
  color: #bcd8f2;
  margin-right: 8px;
}

.edit-bar-btn {
  background: rgba(255, 255, 255, 0.08);
  color: #e6e8ee;
  border: 0;
  border-radius: 4px;
  font-size: 10px;
  font-weight: 600;
  padding: 2px 8px;
  cursor: pointer;
  transition: background 0.15s ease, filter 0.15s ease;
}
.edit-bar-btn:hover {
  background: rgba(255, 255, 255, 0.14);
}
.edit-bar-btn-primary {
  background: var(--accent-color, #007acc);
  color: #fff;
}
.edit-bar-btn-primary:hover {
  background: var(--accent-color, #007acc);
  filter: brightness(1.15);
}

.overlay-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 8px 12px;
  border-bottom: 1px solid rgba(255, 255, 255, 0.06);
  background: rgba(255, 255, 255, 0.02);
  user-select: none;
  -webkit-user-select: none;
  /* native drag 由 Wails v3 runtime 读取 --wails-draggable CSS 实现
   * （drag.js:99 — isDraggableEvent 检查 event target 的 style），
   * 无需前端 pointer 事件。 */
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
  animation: overlay-pulse 1.4s ease-in-out infinite;
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

@keyframes overlay-pulse {
  0%, 100% {
    box-shadow: 0 0 0 0 rgba(78, 201, 176, 0.4);
  }
  50% {
    box-shadow: 0 0 0 6px rgba(78, 201, 176, 0);
  }
}
</style>
