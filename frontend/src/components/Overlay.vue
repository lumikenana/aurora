<script setup lang="ts">
// Overlay - Native HUD companion（独立 frameless webview）
//
// 视觉目标：
//   - HUD Companion 风格，非 SaaS Dashboard
//   - Dark-first, minimal, low distraction
//   - 主内容区可滚动，header 固定
//
// 两种交互态：
//   1. Locked（HUD，默认）：纯展示，无交互 chrome
//   2. Edit Mode（!locked）：
//        - header 变为拖拽手柄
//        - 右下角 resize 手柄
//        - 显示编辑提示 + Lock 按钮
//
// 拖拽实现（修复闪烁 + 落点不准）：
//   - 拖拽期间不调任何 IPC，用 CSS transform 实时视觉反馈
//   - 窗口实际位置在 beginDrag 时锁定，pointerup 时单次 commit
//   - 移除每像素 IPC 调用导致的拖影 + 异步落点漂移
//
// 物理像素换算：CSS px × devicePixelRatio = 物理 px（DPI 适配）。

import { ref, watch, nextTick, computed, onMounted } from 'vue'
import type { ChatMessage, OverlayConfig } from '../api/backend'
import {
  getOverlayGeometry,
  commitOverlayGeometry,
  type OverlayGeometry,
} from '../api/backend'

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

const rootRef = ref<HTMLElement | null>(null)
const bodyRef = ref<HTMLElement | null>(null)
const streaming = ref(false)
const geom = ref<OverlayGeometry | null>(null)

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
  refreshGeom()
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

const editMode = computed(() => !props.locked)

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

// ---- Edit Mode：拖拽 / 缩放（CSS transform 实时反馈 + 单次 commit） ----

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

function beginDrag(e: PointerEvent, mode: 'move' | 'resize') {
  if (!editMode.value || drag) return
  let g = geom.value
  if (!g) {
    refreshGeom()
    g = geom.value
    if (!g) return
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
  // 拖拽期间不挪窗，window 监听 pointermove 确保光标移出也跟手
  window.addEventListener('pointermove', onDragMove)
  window.addEventListener('pointerup', onDragEnd, { once: true })
}

function onDragMove(e: PointerEvent) {
  if (!drag || e.pointerId !== drag.pointerId) return
  const scale = dpr()
  const dx = (e.clientX - drag.startClientX) * scale
  const dy = (e.clientY - drag.startClientY) * scale
  if (!rootRef.value) return
  if (drag.mode === 'move') {
    rootRef.value.style.transform = `translate(${dx}px, ${dy}px)`
  } else {
    const minW = geom.value?.min_width ?? 240
    const minH = geom.value?.min_height ?? 200
    const maxW = geom.value?.max_width ?? 960
    const maxH = geom.value?.max_height ?? 900
    const w = Math.min(maxW, Math.max(minW, drag.startWinW + dx))
    const h = Math.min(maxH, Math.max(minH, drag.startWinH + dy))
    // resize: width/height 视觉反馈（不缩放内容）；窗口实际尺寸在 commit 时落
    rootRef.value.style.width = `${w}px`
    rootRef.value.style.height = `${h}px`
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
  // 清零 transform/width/height —— commit 后窗口已落到正确位置
  if (rootRef.value) {
    rootRef.value.style.transform = ''
    rootRef.value.style.width = ''
    rootRef.value.style.height = ''
  }
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
  // 刷新本地 geom，避免下次 beginDrag 用旧 startWin
  refreshGeom()
}

function lockFromOverlay() {
  emit('lock')
}
</script>

<template>
  <div
    ref="rootRef"
    class="overlay-root"
    :class="{ edit: editMode }"
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
/* Native window 模式：填满独立 frameless 窗口（四周留 8px 给阴影/圆角）。 */
.overlay-root {
  position: absolute;
  top: 8px;
  right: 8px;
  bottom: 8px;
  left: 8px;
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
  /* 拖拽期间会临时设置 transform，提交时清零；保留 transition 让"落窗"有点视觉缓冲（很轻） */
  transition: transform 0.05s linear;
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
