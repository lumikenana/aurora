<script setup lang="ts">
// ChatView.vue - Aurora Main Window 的默认页面（Phase 6 重构）
//
// 职责：
//   - 单一滚动容器（chat-body）：避免 Conversation 嵌套滚动
//   - 流式响应提示 + 智能 auto-scroll（用户向上滚时不强制 jump）
//   - Empty State
//   - 持久化输入区（textarea + Send 按钮，Enter / Shift+Enter 行为）
//
// 不负责：
//   - 后端 events（llmChunk 等）— 由 App.vue 监听并通过 props 传入 messages
//   - 截图、Settings、Logs 等其它功能
//   - 滚动策略的"小细节"（已经在本文件内就近处理）

import { ref, computed, nextTick, watch, onMounted, onUnmounted } from 'vue'
import Conversation from '../components/Conversation.vue'
import type { ChatMessage } from '../api/backend'

const props = defineProps<{
  messages: ChatMessage[]
  busy: boolean
}>()

const emit = defineEmits<{
  (e: 'submit', text: string): void
}>()

const draft = ref('')
const textareaRef = ref<HTMLTextAreaElement | null>(null)
const chatBodyRef = ref<HTMLElement | null>(null)

// 推断 streaming：最后一条是 assistant 且有内容
const isStreaming = computed(() => {
  const last = props.messages[props.messages.length - 1]
  return props.busy && last?.role === 'assistant'
})

function autoResize() {
  const el = textareaRef.value
  if (!el) return
  el.style.height = 'auto'
  el.style.height = Math.min(el.scrollHeight, 180) + 'px'
}

watch(draft, () => {
  autoResize()
})

// ---- 智能 auto-scroll ----
//
// 规则：
//   1. 用户在底部（容差 80px）→ 流式 chunks 来时跟随到最新
//   2. 用户向上滚 → 不强制 jump；显示"跳到最新"按钮
//   3. 用户再次滚到底部 → 自动隐藏按钮

const NEAR_BOTTOM_PX = 80
const userNearBottom = ref(true)
const showJumpToLatest = ref(false)

function updateScrollState() {
  const el = chatBodyRef.value
  if (!el) return
  const distance = el.scrollHeight - el.scrollTop - el.clientHeight
  userNearBottom.value = distance <= NEAR_BOTTOM_PX
  showJumpToLatest.value = !userNearBottom.value
}

function scrollToLatest(smooth = true) {
  const el = chatBodyRef.value
  if (!el) return
  el.scrollTo({ top: el.scrollHeight, behavior: smooth ? 'smooth' : 'auto' })
}

// 流式 / 新消息 → 仅当用户在底部时跟随；不在底部则显示 jump 按钮
watch(
  () => props.messages.map((m) => m.content).join(''),
  async () => {
    await nextTick()
    if (userNearBottom.value) {
      scrollToLatest(false)
    } else {
      showJumpToLatest.value = true
    }
  },
)

// 监听容器 scroll 事件以更新状态
function onChatBodyScroll() {
  updateScrollState()
}

onMounted(async () => {
  await nextTick()
  updateScrollState()
})

onUnmounted(() => {
  // 无需清理：watch 自动 stop
})

// ---- 输入 / 发送 ----

async function send() {
  const text = draft.value.trim()
  if (!text || props.busy) return
  draft.value = ''
  await nextTick()
  autoResize()
  emit('submit', text)
  // 发送后立即滚到底部（即使之前用户向上滚了）
  await nextTick()
  userNearBottom.value = true
  scrollToLatest(true)
}

async function onKeydown(e: KeyboardEvent) {
  // Enter 发送；Shift+Enter 换行
  if (e.key === 'Enter' && !e.shiftKey) {
    e.preventDefault()
    await send()
  }
}

function onTextareaFocus() {
  // 焦点时不强制滚到底部；保持用户当前位置
}

function clearDraft() {
  draft.value = ''
  nextTick(autoResize)
  textareaRef.value?.focus()
}
</script>

<template>
  <div class="chat-view">
    <header class="view-header">
      <h1>Chat</h1>
      <p class="view-subtitle">
        Talk with Aurora while you're gaming
      </p>
    </header>

    <div
      class="chat-body"
      ref="chatBodyRef"
      @scroll="onChatBodyScroll"
    >
      <template v-if="messages.length === 0">
        <div class="empty-state">
          <div class="empty-icon">💬</div>
          <h2 class="empty-title">No conversation yet</h2>
          <p class="empty-hint">
            Ask Aurora for help with the game you're playing.
          </p>
          <ul class="empty-suggestions">
            <li>What's the current meta composition?</li>
            <li>Help me decide whether to level or roll</li>
            <li>Explain this item combination</li>
          </ul>
          <p class="empty-tip">
            Press <kbd>Ctrl + Alt + R</kbd> for Quick Input
          </p>
        </div>
      </template>
      <template v-else>
        <Conversation :messages="messages" />
        <div v-if="isStreaming" class="streaming-hint">
          <span class="streaming-dot" aria-hidden="true"></span>
          <span>Aurora is generating…</span>
        </div>
      </template>

      <button
        v-if="showJumpToLatest && messages.length > 0"
        type="button"
        class="jump-latest-btn"
        aria-label="Jump to latest"
        @click="scrollToLatest(true)"
      >
        ↓ Latest
      </button>
    </div>

    <div class="chat-input-area">
      <textarea
        ref="textareaRef"
        v-model="draft"
        rows="1"
        placeholder="Ask Aurora..."
        :disabled="busy"
        :aria-label="busy ? 'Waiting for Aurora to respond' : 'Message Aurora'"
        @keydown="onKeydown"
        @focus="onTextareaFocus"
      ></textarea>
      <button
        v-if="draft && !busy"
        type="button"
        class="chat-clear-btn"
        aria-label="Clear input"
        @click="clearDraft"
      >×</button>
      <button
        type="button"
        class="chat-send-btn"
        :disabled="!draft.trim() || busy"
        :aria-label="busy ? 'Sending' : 'Send message'"
        @click="send"
      >
        {{ busy ? 'Sending…' : 'Send ↵' }}
      </button>
    </div>
  </div>
</template>

<style scoped>
.chat-view {
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
  letter-spacing: 0.01em;
  color: var(--text-primary);
}

.view-subtitle {
  font-size: 12px;
  color: var(--text-secondary);
  margin-top: 2px;
}

/* ---- Single scroll container ---- */
.chat-body {
  flex: 1;
  overflow-y: auto;
  padding: var(--space-4) var(--space-7);
  min-height: 0;
  position: relative;
  scroll-behavior: smooth;
}

.empty-state {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  height: 100%;
  text-align: center;
  color: var(--text-secondary);
  gap: var(--space-2);
  padding: 40px var(--space-5);
}

.empty-icon {
  font-size: 36px;
  opacity: 0.45;
  margin-bottom: var(--space-2);
}

.empty-title {
  font-size: 16px;
  font-weight: 500;
  color: var(--text-primary);
  margin: 0;
}

.empty-hint {
  font-size: 13px;
  margin: 0;
}

.empty-suggestions {
  list-style: none;
  padding: 0;
  margin: var(--space-3) 0 0;
  display: flex;
  flex-direction: column;
  gap: var(--space-1);
  font-size: 12px;
  color: var(--text-faint);
  max-width: 320px;
}

.empty-suggestions li::before {
  content: '·';
  margin-right: var(--space-2);
  color: var(--text-faint);
}

.empty-tip {
  font-size: 12px;
  margin-top: var(--space-3);
  opacity: 0.7;
}

/* ---- Streaming hint ---- */
.streaming-hint {
  display: flex;
  align-items: center;
  gap: var(--space-2);
  padding: var(--space-2) var(--space-3);
  font-size: 12px;
  color: var(--text-muted);
  font-style: italic;
  /* max-width: 780px + margin auto 已移除（2026-09-07，全拉满右边） */
}

.streaming-dot {
  width: 6px;
  height: 6px;
  background: var(--status-success);
  border-radius: 50%;
  animation: pulse 1.4s ease-in-out infinite;
}

@keyframes pulse {
  0%, 100% { opacity: 1; transform: scale(1); }
  50% { opacity: 0.5; transform: scale(0.8); }
}

/* ---- Jump to latest button ---- */
.jump-latest-btn {
  position: sticky;
  bottom: var(--space-4);
  left: 50%;
  transform: translateX(-50%);
  display: block;
  margin: 0 auto;
  padding: 6px 14px;
  background: var(--bg-secondary);
  color: var(--text-default);
  border: 1px solid var(--border-default);
  border-radius: var(--radius-pill);
  font-size: 12px;
  font-weight: 500;
  cursor: pointer;
  box-shadow: var(--shadow-md);
  transition: background var(--transition-fast), border-color var(--transition-fast);
}
.jump-latest-btn:hover {
  background: var(--surface-2, var(--bg-primary));
  border-color: var(--border-strong);
}

/* ---- Input area ---- */
.chat-input-area {
  display: flex;
  gap: var(--space-2);
  padding: var(--space-3) var(--space-5) var(--space-4);
  border-top: 1px solid var(--border-color);
  background: var(--bg-secondary);
  flex-shrink: 0;
  align-items: flex-end;
  position: relative;
}

.chat-input-area textarea {
  flex: 1;
  background: var(--bg-primary);
  border: 1px solid var(--border-default);
  border-radius: var(--radius-md);
  padding: 10px var(--space-3);
  color: var(--text-default);
  font-size: 13px;
  line-height: 1.5;
  resize: none;
  font-family: inherit;
  outline: none;
  transition: border-color var(--transition-fast), box-shadow var(--transition-fast);
  min-height: 40px;
  max-height: 180px;
}

.chat-input-area textarea:focus {
  border-color: var(--accent-color);
  box-shadow: var(--focus-ring);
}

.chat-input-area textarea:disabled {
  opacity: 0.6;
  cursor: not-allowed;
}

.chat-input-area textarea::placeholder {
  color: var(--text-faint);
}

.chat-clear-btn {
  position: absolute;
  right: calc(var(--space-5) + 80px);
  bottom: calc(var(--space-4) + 8px);
  width: 20px;
  height: 20px;
  background: var(--bg-secondary);
  border: 1px solid var(--border-default);
  color: var(--text-muted);
  border-radius: 50%;
  font-size: 14px;
  line-height: 1;
  cursor: pointer;
  padding: 0;
  display: flex;
  align-items: center;
  justify-content: center;
  transition: color var(--transition-fast), border-color var(--transition-fast);
}
.chat-clear-btn:hover {
  color: var(--text-default);
  border-color: var(--border-strong);
}

.chat-send-btn {
  background: var(--accent-color);
  color: var(--text-on-accent);
  border: 0;
  border-radius: var(--radius-md);
  padding: 10px var(--space-4);
  font-size: 13px;
  font-weight: 500;
  cursor: pointer;
  transition: background var(--transition-fast), opacity var(--transition-fast);
  flex-shrink: 0;
  height: 40px;
}

.chat-send-btn:hover:not(:disabled) {
  background: var(--accent-hover);
}

.chat-send-btn:disabled {
  opacity: 0.4;
  cursor: not-allowed;
}
</style>
