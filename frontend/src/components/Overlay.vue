<script setup lang="ts">
// Overlay - HUD companion（UI 重构：Locked(HUD) vs Edit Mode）
//
// 视觉目标：
//   - HUD Companion 风格，非 SaaS Dashboard
//   - Dark-first, minimal, low distraction
//   - 主内容区可滚动，header 固定
//
// 两种运行形态（UI 重构）：
//   1. Locked（HUD，默认）：纯展示，无交互 chrome，不响应拖拽/缩放
//   2. Edit Mode（native && !locked）：
//        - header 变为拖拽手柄（pointer events → MoveOverlayWindow 实时移动）
//        - 右下角 resize 手柄（→ ResizeOverlayWindow）
//        - 结束时 CommitOverlayGeometry 持久化（Go 侧 clamp + 广播）
//        - 显示编辑提示 + Lock 按钮
//
// Preview 模式（native 未传）：渲染在 Main Window 配置页预览，走 anchor-* CSS。
// 物理像素换算：CSS px × devicePixelRatio = 物理 px（DPI 适配）。

import { ref, watch, nextTick, computed, onMounted } from 'vue'
import type { ChatMessage, OverlayConfig } from '../api/backend'
import {
  getOverlayGeometry,
  moveOverlayWindow,
  resizeOverlayWindow,
  commitOverlayGeometry,
  type OverlayGeometry,
} from '../api/backend'

const props = withDefaults(
  defineProps<{
    messages: ChatMessage[]
    config: OverlayConfig
    // native：overlay 渲染在独立 frameless 透明 webview（overlay.html）
    native?: boolean
    // locked：HUD 锁定态。native 模式下由 overlay-main.ts 从后端状态提供。
    locked?: boolean
  }>(),
  { native: false, locked: true },
)

const emit = defineEmits<{
  (e: 'close'): void
  (e: 'lock'): void
}>()

const bodyRef = ref<HTMLElement | null>(null)
const streaming = ref(false)
const geom = ref<OverlayGeometry | null>(null)

// ---- streaming 检测（逻辑保持 Phase 3 行为） ----
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
  // Edit Mode 需要窗口当前几何 + clamp 边界
  if (props.native) {
    refreshGeom()
  }
})

function refreshGeom() {
  getOverlayGeometry()
    .then((g) => {
      geom.value = g
    })
    .catch(() => { /* ignore */ })
}

function close() {
  emit('close')
}

function shouldShow(msg: ChatMessage): boolean {
  if (msg.role === 'user') return props.config.show_user_messages
  if (msg.role === 'assistant') return props.config.show_assistant_messages
  return true
}

const editMode = computed(() => !!props.native && !props.locked)

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

// ---- Edit Mode：拖拽 / 缩放（pointer events + 物理 px 换算） ----

interface DragState {
  mode: 'move' | 'resize'
  pointerId: number
  startClientX: number
  startClientY: number
  startWinX: number
  startWinY: number
  startWinW: number
  startWinH: number
}

let drag: DragState | null = null

function dpr(): number {
  return window.devicePixelRatio || 1
}

async function beginDrag(e: PointerEvent, mode: 'move' | 'resize') {
  if (!editMode.value || drag) return
  let g = geom.value
  if (!g) {
    try {
      g = await getOverlayGeometry()
      geom.value = g
    } catch {
      return
    }
  }
  drag = {
    mode,
    pointerId: e.pointerId,
    startClientX: e.clientX,
    startClientY: e.clientY,
    startWinX: g.x,
    startWinY: g.y,
    startWinW: g.width,
    startWinH: g.height,
  }
  window.addEventListener('pointermove', onDragMove)
  window.addEventListener('pointerup', onDragEnd, { once: true })
}

function onDragMove(e: PointerEvent) {
  if (!drag || e.pointerId !== drag.pointerId) return
  const scale = dpr()
  const dx = (e.clientX - drag.startClientX) * scale
  const dy = (e.clientY - drag.startClientY) * scale
  if (drag.mode === 'move') {
    moveOverlayWindow(drag.startWinX + dx, drag.startWinY + dy)
  } else {
    const minW = geom.value?.min_width ?? 240
    const minH = geom.value?.min_height ?? 200
    const maxW = geom.value?.max_width ?? 960
    const maxH = geom.value?.max_height ?? 900
    const w = Math.min(maxW, Math.max(minW, drag.startWinW + dx))
    const h = Math.min(maxH, Math.max(minH, drag.startWinH + dy))
    resizeOverlayWindow(w, h)
  }
}

function onDragEnd(e: PointerEvent) {
  window.removeEventListener('pointermove', onDragMove)
  if (!drag || e.pointerId !== drag.pointerId) {
    drag = null
    return
  }
  const scale = dpr()
  const dx = (e.clientX - drag.startClientX) * scale
  const dy = (e.clientY - drag.startClientY) * scale
  const mode = drag.mode
  const start = drag
  drag = null
  if (mode === 'move') {
    commitOverlayGeometry(start.startWinX + dx, start.startWinY + dy, 0, 0)
  } else {
    const minW = geom.value?.min_width ?? 240
    const minH = geom.value?.min_height ?? 200
    const maxW = geom.value?.max_width ?? 960
    const maxH = geom.value?.max_height ?? 900
    const w = Math.min(maxW, Math.max(minW, start.startWinW + dx))
    const h = Math.min(maxH, Math.max(minH, start.startWinH + dy))
    commitOverlayGeometry(start.startWinX, start.startWinY, w, h)
  }
}

function lockFromOverlay() {
  emit('lock')
}
</script>

<template>
  <div
    class="overlay-root"
    :class="[`anchor-${config.anchor}`, `font-${config.font_size}`, { native: props.native, edit: editMode }]"
    :style="overlayStyle"
    role="region"
    aria-label="Aurora Game Overlay"
  >
    <!-- Edit Mode 提示条（编辑态专用 chrome） -->
    <div v-if="editMode" class="edit-bar">
      <span class="edit-hint">Drag header to move · resize from corner</span>
      <button class="edit-lock-btn" title="Lock overlay (HUD mode)" @click="lockFromOverlay">
        Lock
      </button>
    </div>

    <header
      class="overlay-header"
      :class="{ draggable: editMode }"
      @pointerdown="beginDrag($event, 'move')"
    >
      <div class="overlay-brand">
        <span class="brand-dot" :class="{ streaming: isStreaming }"></span>
        <span class="brand-name">Aurora</span>
        <span v-if="isStreaming" class="brand-status">generating…</span>
      </div>
      <button
        v-if="!editMode"
        class="overlay-close"
        title="Hide overlay"
        @click="close"
      >×</button>
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

    <!-- Edit Mode resize 手柄 -->
    <div
      v-if="editMode"
      class="resize-handle"
      title="Resize"
      @pointerdown.stop="beginDrag($event, 'resize')"
    ></div>
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

/* native window 模式：填满独立 frameless 窗口（四周留 8px 给阴影/圆角）。
   必须定义在 anchor-* 之后以覆盖其 top/right/bottom/left。 */
.overlay-root.native {
  position: absolute;
  top: 8px;
  right: 8px;
  bottom: 8px;
  left: 8px;
  width: auto;
  max-width: none;
  max-height: none;
}

/* ---- Edit Mode chrome（仅编辑态出现） ---- */
.overlay-root.edit {
  border-color: var(--accent-color, #007acc);
  box-shadow: 0 12px 32px rgba(0, 0, 0, 0.5), var(--focus-ring, 0 0 0 2px rgba(0, 122, 204, 0.45));
}

.edit-bar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  padding: 4px 10px;
  background: rgba(0, 122, 204, 0.18);
  border-bottom: 1px solid rgba(0, 122, 204, 0.35);
  user-select: none;
}

.edit-hint {
  font-size: 10px;
  letter-spacing: 0.03em;
  color: #bcd8f2;
}

.edit-lock-btn {
  background: var(--accent-color, #007acc);
  color: #fff;
  border: 0;
  border-radius: 4px;
  font-size: 10px;
  font-weight: 600;
  padding: 2px 8px;
  cursor: pointer;
}
.edit-lock-btn:hover {
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
  touch-action: none;
}

.overlay-header.draggable {
  cursor: move;
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

/* ---- resize 手柄（编辑态右下角） ---- */
.resize-handle {
  position: absolute;
  right: 0;
  bottom: 0;
  width: 18px;
  height: 18px;
  cursor: nwse-resize;
  touch-action: none;
  z-index: 10;
}
.resize-handle::after {
  content: '';
  position: absolute;
  right: 4px;
  bottom: 4px;
  width: 8px;
  height: 8px;
  border-right: 2px solid rgba(255, 255, 255, 0.5);
  border-bottom: 2px solid rgba(255, 255, 255, 0.5);
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
