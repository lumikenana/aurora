// overlay-main.ts — Overlay native window 独立入口（Phase 8F）。
//
// Main（src/main.ts → index.html）与 Overlay（本文件 → overlay.html）是两个
// 物理独立的 webview。本入口只负责：
//   1. 组装对话 messages（监听 chatUserMessage / llmChunk / llmFinish）
//   2. 订阅 overlayConfigChanged refetch 配置（opacity/字体/过滤实时生效）
//   3. 挂载复用的 Overlay.vue（native prop → 填满窗口；业务逻辑零复制）
//   4. close → setOverlayVisible(false)（与托盘/Settings 同一状态口）
//
// 可见性由后端驱动（overlayVisibilityChanged → main.go Show/Hide 窗口），
// 本窗口只在可见时出现在屏幕上，前端不需要 visible prop。
//
// 与 App.vue 的组装差异（有意为之，非复制）：
//   - user 消息来自 chatUserMessage 事件（App.vue 是 handleSubmit 本地 push /
//     voiceTranscript push —— 两条路径都会触发后端 chatUserMessage，overlay
//     只听这一个事件即可覆盖所有输入源，不会双份）
//   - 不监听 voiceTranscript / gameDetect / log（Overlay.vue 不需要）

import { createApp, h, ref } from 'vue'
import Overlay from './components/Overlay.vue'
import {
  getOverlayConfig,
  setOverlayVisible,
  type ChatMessage,
  type OverlayConfig,
} from './api/backend'
import { EventsOn } from './api/events'
import './style.css'

// ---- 对话组装（与 App.vue 语义一致的最小子集） ----
const messages = ref<ChatMessage[]>([])

function appendToLastAssistant(content: string) {
  const last = messages.value[messages.value.length - 1]
  if (last && last.role === 'assistant') {
    last.content += content
  } else {
    messages.value.push({ role: 'assistant', content })
  }
}

EventsOn('chatUserMessage', (text: unknown) => {
  if (typeof text !== 'string' || !text) return
  messages.value.push({ role: 'user', content: text })
  messages.value.push({ role: 'assistant', content: '' })
})

EventsOn('llmChunk', (content: unknown) => {
  if (typeof content === 'string' && content) {
    appendToLastAssistant(content)
  }
})

EventsOn('llmFinish', () => {
  // 流式结束；messages 已聚合，Overlay.vue 自行检测 streaming 淡出。
})

// ---- 配置 ----
const overlayConfig = ref<OverlayConfig>({
  enabled: false,
  opacity: 0.9,
  font_size: 'medium',
  auto_scroll: true,
  show_user_messages: true,
  show_assistant_messages: true,
  anchor: 'bottom-right',
})

async function reloadConfig() {
  try {
    overlayConfig.value = await getOverlayConfig()
  } catch (err) {
    console.error('[overlay] failed to load overlay config:', err)
  }
}

EventsOn('overlayConfigChanged', reloadConfig)
reloadConfig()

// ---- 挂载 ----
const root = document.getElementById('overlay') as HTMLElement
root.style.width = '100vw'
root.style.height = '100vh'

createApp({
  render() {
    return h(Overlay, {
      native: true,
      messages: messages.value,
      config: overlayConfig.value,
      onClose: () => {
        // 与托盘 / OverlayView Show 按钮同一状态口：
        // SetOverlayVisible(false) → overlayVisibilityChanged → main.go Hide 窗口
        setOverlayVisible(false).catch(() => { /* ignore */ })
      },
    })
  },
}).mount(root)
