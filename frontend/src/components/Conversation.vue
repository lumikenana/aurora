<script setup lang="ts">
// Conversation.vue - 对话消息列表（Phase 6 重构）
//
// 职责：
//   - 渲染消息列表（user / assistant / system error）
//   - assistant 消息：Markdown 渲染 + 整段 Copy + 代码块 Copy
//   - user 消息：纯文本（保留空白）
//   - **不** 自带滚动容器（由 ChatView 的 chat-body 负责）
//
// 架构：
//   - markdown 渲染通过 src/utils/markdown.ts，零依赖、已转义安全
//   - 代码块 Copy 按钮在 onUpdated 时注入到 <pre> 内部（v-html 后处理）
//   - 滚动 / 自动滚动策略完全在 ChatView，Conversation 不持有 ref

import { ref, onMounted, onUpdated, nextTick, watch } from 'vue'
import type { ChatMessage } from '../api/backend'
import { renderMarkdown } from '../utils/markdown'
import { copyToClipboard } from '../utils/clipboard'

const props = defineProps<{
  messages: ChatMessage[]
}>()

const messageRefs = ref<HTMLElement[]>([])

function setMessageRef(el: Element | null | undefined, index: number) {
  if (el instanceof HTMLElement) {
    messageRefs.value[index] = el
  }
}

// 复制整条 assistant 消息（原始 markdown 源）
const messageCopyState = ref<Record<number, 'idle' | 'copied' | 'failed'>>({})

async function copyMessage(index: number) {
  const msg = props.messages[index]
  if (!msg) return
  const ok = await copyToClipboard(msg.content)
  messageCopyState.value = {
    ...messageCopyState.value,
    [index]: ok ? 'copied' : 'failed',
  }
  if (ok) {
    setTimeout(() => {
      messageCopyState.value = {
        ...messageCopyState.value,
        [index]: 'idle',
      }
    }, 1500)
  } else {
    setTimeout(() => {
      messageCopyState.value = {
        ...messageCopyState.value,
        [index]: 'idle',
      }
    }, 3000)
  }
}

// ---- 代码块 Copy 按钮注入 ----
//
// 渲染策略：
//   1. v-html 注入 markdown 产物（含 <pre class="md-code-block">）
//   2. onMounted / onUpdated 时对每个 <pre> 增加一个 header（语言 + Copy 按钮）
//   3. 按钮的 onclick 走 addEventListener，事件对象不通过 Vue 树

function attachCodeBlockCopyButtons(root: HTMLElement) {
  const pres = root.querySelectorAll<HTMLPreElement>('pre.md-code-block')
  pres.forEach((pre) => {
    if (pre.dataset.copyAttached === '1') return
    pre.dataset.copyAttached = '1'

    const lang = pre.className.replace('md-code-block', '').trim()
    const langLabel = lang.replace(/^md-code-lang-/, '') || ''

    // 包装 pre 为可定位容器
    const wrapper = document.createElement('div')
    wrapper.className = 'md-code-wrap'

    // 头部
    const header = document.createElement('div')
    header.className = 'md-code-header'

    const langEl = document.createElement('span')
    langEl.className = 'md-code-lang'
    langEl.textContent = langLabel || 'code'
    header.appendChild(langEl)

    const btn = document.createElement('button')
    btn.className = 'md-code-copy-btn'
    btn.type = 'button'
    btn.textContent = 'Copy'
    btn.setAttribute('aria-label', 'Copy code block to clipboard')
    btn.addEventListener('click', async (ev) => {
      ev.stopPropagation()
      const code = pre.querySelector('code')?.textContent ?? ''
      const ok = await copyToClipboard(code)
      if (ok) {
        btn.textContent = 'Copied'
        btn.classList.add('copied')
        setTimeout(() => {
          btn.textContent = 'Copy'
          btn.classList.remove('copied')
        }, 1500)
      } else {
        btn.textContent = 'Copy failed'
        btn.classList.add('failed')
        setTimeout(() => {
          btn.textContent = 'Copy'
          btn.classList.remove('failed')
        }, 3000)
      }
    })
    header.appendChild(btn)

    // 替换 pre 为 wrapper > header + pre
    const parent = pre.parentNode
    if (parent) {
      parent.insertBefore(wrapper, pre)
      wrapper.appendChild(header)
      wrapper.appendChild(pre)
    }
  })
}

onMounted(async () => {
  await nextTick()
  messageRefs.value.forEach((el) => el && attachCodeBlockCopyButtons(el))
})

onUpdated(async () => {
  await nextTick()
  messageRefs.value.forEach((el) => el && attachCodeBlockCopyButtons(el))
})

// 消息数组变化时（包括流式 chunks 拼接）也走 onUpdated；不再需要独立 watch。
// 但为了 messageRefs 数组与 messages 同步，做个空数组清理。
watch(
  () => props.messages.length,
  (len) => {
    if (messageRefs.value.length > len) {
      messageRefs.value = messageRefs.value.slice(0, len)
    }
  },
)
</script>

<template>
  <div class="conversation">
    <article
      v-for="(msg, i) in messages"
      :key="i"
      :ref="(el) => setMessageRef(el as Element, i)"
      class="msg"
      :class="['msg-' + msg.role, { 'msg-error': msg.role === 'error' }]"
    >
      <header class="msg-head">
        <span class="msg-author">{{ msg.role === 'user' ? 'You' : (msg.role === 'error' ? 'System' : 'Aurora') }}</span>
        <button
          v-if="msg.role === 'assistant'"
          type="button"
          class="msg-copy-btn"
          :class="{
            copied: messageCopyState[i] === 'copied',
            failed: messageCopyState[i] === 'failed',
          }"
          :aria-label="messageCopyState[i] === 'copied' ? 'Copied' : 'Copy assistant response'"
          @click="copyMessage(i)"
        >
          {{
            messageCopyState[i] === 'copied'
              ? 'Copied'
              : messageCopyState[i] === 'failed'
              ? 'Copy failed'
              : 'Copy'
          }}
        </button>
      </header>

      <div v-if="msg.role === 'user'" class="msg-body msg-body-plain">{{ msg.content }}</div>
      <div
        v-else-if="msg.role === 'assistant'"
        class="msg-body msg-body-md"
        v-html="renderMarkdown(msg.content)"
      ></div>
      <div v-else class="msg-body msg-body-error">{{ msg.content }}</div>
    </article>
  </div>
</template>

<style scoped>
.conversation {
  display: flex;
  flex-direction: column;
  gap: var(--space-5);
  max-width: 780px;
  margin: 0 auto;
  padding: var(--space-2) 0 var(--space-6);
}

/* ---- Message card ---- */
.msg {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
}

.msg-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0 var(--space-1);
}

.msg-author {
  font-size: 11px;
  font-weight: 600;
  letter-spacing: 0.08em;
  text-transform: uppercase;
  color: var(--text-muted);
}

.msg-user .msg-author {
  color: var(--accent-color);
}

.msg-error .msg-author {
  color: var(--status-danger);
}

/* ---- Message body ---- */
.msg-body {
  font-size: 14px;
  line-height: 1.65;
  color: var(--text-default);
}

.msg-body-plain {
  white-space: pre-wrap;
  word-break: break-word;
  padding: var(--space-3) var(--space-4);
  background: var(--bg-secondary);
  border-radius: var(--radius-md);
  border-left: 2px solid var(--accent-color);
}

.msg-body-md {
  /* 无 background card；强调为"正文段落"而非"对话气泡" */
  word-break: break-word;
}

.msg-body-md :deep(p) {
  margin: 0 0 var(--space-3);
}
.msg-body-md :deep(p:last-child) {
  margin-bottom: 0;
}

.msg-body-md :deep(h1),
.msg-body-md :deep(h2),
.msg-body-md :deep(h3),
.msg-body-md :deep(h4),
.msg-body-md :deep(h5),
.msg-body-md :deep(h6) {
  margin: var(--space-4) 0 var(--space-2);
  font-weight: 600;
  line-height: 1.3;
  color: var(--text-default);
}
.msg-body-md :deep(h1) { font-size: 1.4rem; }
.msg-body-md :deep(h2) { font-size: 1.2rem; }
.msg-body-md :deep(h3) { font-size: 1.05rem; }
.msg-body-md :deep(h4),
.msg-body-md :deep(h5),
.msg-body-md :deep(h6) { font-size: 0.95rem; }

.msg-body-md :deep(ul.md-list),
.msg-body-md :deep(ol.md-list) {
  margin: 0 0 var(--space-3);
  padding-left: var(--space-5);
}
.msg-body-md :deep(li) {
  margin: var(--space-1) 0;
}

.msg-body-md :deep(.md-inline-code) {
  font-family: ui-monospace, SFMono-Regular, "Cascadia Mono", Menlo, Consolas, monospace;
  font-size: 0.88em;
  padding: 1px 5px;
  border-radius: var(--radius-sm);
  background: var(--bg-secondary);
  color: var(--text-default);
  border: 1px solid var(--border-subtle);
}

.msg-body-md :deep(hr.md-hr) {
  border: 0;
  border-top: 1px solid var(--border-default);
  margin: var(--space-4) 0;
}

.msg-body-md :deep(a.md-link) {
  color: var(--accent-color);
  text-decoration: underline;
  text-underline-offset: 2px;
}
.msg-body-md :deep(a.md-link:hover) {
  color: var(--accent-hover);
}

.msg-body-md :deep(strong) { font-weight: 600; }
.msg-body-md :deep(em) { font-style: italic; }

/* ---- Code block (post-processed) ---- */
.msg-body-md :deep(.md-code-wrap) {
  position: relative;
  margin: var(--space-3) 0;
  background: var(--bg-primary);
  border: 1px solid var(--border-default);
  border-radius: var(--radius-md);
  overflow: hidden;
}
.msg-body-md :deep(.md-code-header) {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 4px var(--space-3);
  background: var(--bg-secondary);
  border-bottom: 1px solid var(--border-default);
  font-size: 11px;
  color: var(--text-muted);
}
.msg-body-md :deep(.md-code-lang) {
  font-family: ui-monospace, monospace;
  letter-spacing: 0.04em;
  text-transform: lowercase;
}
.msg-body-md :deep(.md-code-copy-btn) {
  background: transparent;
  border: 1px solid transparent;
  color: var(--text-muted);
  font-size: 11px;
  padding: 2px 8px;
  border-radius: var(--radius-sm);
  cursor: pointer;
  font-family: inherit;
  transition: color var(--transition-fast), background var(--transition-fast);
}
.msg-body-md :deep(.md-code-copy-btn:hover) {
  background: var(--hover-overlay);
  color: var(--text-default);
}
.msg-body-md :deep(.md-code-copy-btn.copied) {
  color: var(--status-success);
}
.msg-body-md :deep(.md-code-copy-btn.failed) {
  color: var(--status-danger);
}
.msg-body-md :deep(pre.md-code-block) {
  margin: 0;
  padding: var(--space-3) var(--space-4);
  overflow-x: auto;
  font-family: ui-monospace, SFMono-Regular, "Cascadia Mono", Menlo, Consolas, monospace;
  font-size: 12.5px;
  line-height: 1.55;
  color: var(--text-default);
  background: var(--bg-primary);
  white-space: pre;
  word-break: normal;
}
.msg-body-md :deep(pre.md-code-block code) {
  font-family: inherit;
  font-size: inherit;
  background: transparent;
  padding: 0;
  border: 0;
  white-space: pre;
}

/* ---- Error message ---- */
.msg-body-error {
  padding: var(--space-3) var(--space-4);
  background: var(--status-danger-bg);
  border: 1px solid var(--status-danger-bd);
  border-radius: var(--radius-md);
  color: var(--status-danger);
  font-size: 13px;
}

/* ---- Message copy button (whole message) ---- */
.msg-copy-btn {
  background: transparent;
  border: 1px solid transparent;
  color: var(--text-muted);
  font-size: 11px;
  padding: 2px 8px;
  border-radius: var(--radius-sm);
  cursor: pointer;
  font-family: inherit;
  transition: color var(--transition-fast), background var(--transition-fast);
}
.msg-copy-btn:hover {
  background: var(--hover-overlay);
  color: var(--text-default);
}
.msg-copy-btn.copied { color: var(--status-success); }
.msg-copy-btn.failed { color: var(--status-danger); }
</style>
