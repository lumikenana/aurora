// quickinput-main.ts — QuickInput native window 独立入口（Phase 8E）。
//
// Main（src/main.ts → index.html）与 QuickInput（本文件 → quickinput.html）
// 是两个物理独立的 webview。本入口只负责：
//   1. 全局样式 + 共享 CSS 变量
//   2. 挂载复用的 QuickInput.vue（不复制业务逻辑）
//   3. emit('submit') → chatStream(text)（LLM 提交链路与 Main 保持一致）
//      + cancelQuickInput()（后端 QuickInputManager.Hide → driver.Hide 关窗）
//   4. emit('update:visible', false) → cancelQuickInput()（Esc / 遮罩点击 → 隐藏窗口）
//
// 提交后窗口隐藏，不 Show Main Window（8E 约束：Main 由用户手动打开）。
// LLM 回复流式渲染在 Main Window 的 Chat（llmChunk 事件广播到所有 webview）。

import { createApp, h, nextTick } from 'vue'
import QuickInput from './components/QuickInput.vue'
import { chatStream, cancelQuickInput, getConfigStatus } from './api/backend'
import './style.css'

// QuickInput 窗口本身 frameless + transparent + always-on-top：
// 根容器撑满窗口，输入框样式由 QuickInput.vue 内部 scoped CSS 决定。
const root = document.getElementById('quickinput') as HTMLElement
root.style.width = '100vw'
root.style.height = '100vh'

createApp({
  render() {
    return h(QuickInput, {
      visible: true,
      onSubmit: async (text: string) => {
        // 与 App.vue handleSubmit 相同的提交链路：
        // chatStream(text) → 后端 Agent.ChatStream → llmChunk/llmFinish 广播
        try {
          const status = await getConfigStatus()
          if (!status.configured) {
            // 未配置 LLM：提示后关闭（错误直接显示在本窗口输入框 placeholder，
            // 不再 Show Main Window —— 8E 约束）
            console.error('[quickinput] AI provider is not configured. Open Settings to add your API key.')
            return
          }
          await chatStream(text)
        } catch (err) {
          console.error('[quickinput] chat failed:', err)
        } finally {
          try { await cancelQuickInput() } catch { /* ignore */ }
        }
      },
      'onUpdate:visible': (value: boolean) => {
        if (!value) {
          // Esc / 遮罩点击 / close → 后端 Hide（native window 关闭）
          cancelQuickInput().catch(() => { /* ignore */ })
        }
      },
    })
  },
}).mount(root)

// QuickInput.vue 的 autofocus 依赖 visible false→true 的 watch；
// 本窗口 visible 恒为 true，watch 不触发 → 挂载后手动聚焦。
// 窗口每次 Show 都重新加载 URL 吗？不会 —— v3 单例窗口 Show 不重载页面，
// 因此这里补充 window focus 事件时也重新聚焦输入框。
function focusTextarea() {
  const ta = root.querySelector('textarea')
  if (ta instanceof HTMLTextAreaElement) {
    ta.focus()
  }
}
nextTick(focusTextarea)
window.addEventListener('focus', focusTextarea)
