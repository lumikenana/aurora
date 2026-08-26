<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { EventsOn } from './api/events'
import Conversation from './components/Conversation.vue'
import LogPanel from './components/LogPanel.vue'
import Settings from './components/Settings.vue'
import type { LogEntry } from './components/LogPanel.vue'
import QuickInput from './components/QuickInput.vue'
import VoiceIndicator from './components/VoiceIndicator.vue'
import {
  getConfigStatus,
  chatStream,
  captureFullScreenshot,
  frontendEcho,
  submitQuickInput,
  type ChatMessage,
  type ScreenshotResult,
} from './api/backend'

// ---- 状态 ----
const messages = ref<ChatMessage[]>([])
const logs = ref<LogEntry[]>([])
const quickInputVisible = ref(false)

// ---- 日志 ----
const MAX_LOGS = 500

function addLog(level: string, message: string) {
  logs.value.push({
    level,
    message,
    time: new Date().toLocaleTimeString(),
  })
  if (logs.value.length > MAX_LOGS) {
    logs.value.splice(0, logs.value.length - MAX_LOGS)
  }
}

// ---- 对话 ----
function appendToLastAssistant(content: string) {
  const last = messages.value[messages.value.length - 1]
  if (last && last.role === 'assistant') {
    last.content += content
  } else {
    messages.value.push({ role: 'assistant', content })
  }
}

async function handleSubmit(text: string) {
  console.log('[handleSubmit] start, text:', text)
  messages.value.push({ role: 'user', content: text })

  // 无论成败都关闭快速输入窗口（在 finally 中处理）
  try {
    let status
    try {
      status = await getConfigStatus()
      console.log('[handleSubmit] config status:', status)
    } catch (err) {
      messages.value.push({
        role: 'assistant',
        content: `获取配置状态失败: ${(err as Error).message || String(err)}`,
      })
      return
    }
    if (!status.configured) {
      messages.value.push({ role: 'assistant', content: 'LLM 未配置，请配置 API Key 后重试。' })
      return
    }

    messages.value.push({ role: 'assistant', content: '' })
    console.log('[handleSubmit] calling chatStream, messages.length:', messages.value.length)
    await chatStream(text)
    console.log('[handleSubmit] chatStream returned')
  } catch (err) {
    console.error('[handleSubmit] error:', err)
    messages.value.push({
      role: 'assistant',
      content: `错误: ${(err as Error).message || String(err)}`,
    })
  } finally {
    try { await submitQuickInput(text) } catch { /* ignore */ }
    quickInputVisible.value = false
  }
}

// ---- Wails 事件 ----
EventsOn('showQuickInput', () => {
  try { frontendEcho('showQuickInput') } catch { /* ignore */ }
  quickInputVisible.value = true
})

EventsOn('log', (data: { level: string; message: string }) => {
  if (data && data.level && data.message) {
    addLog(data.level, data.message)
  }
})

EventsOn('llmChunk', (content: string) => {
  console.log('[llmChunk] received:', JSON.stringify(content), 'messages.length:', messages.value.length)
  if (content) {
    appendToLastAssistant(content)
    console.log('[llmChunk] after append, last message:', messages.value[messages.value.length - 1])
  }
})

EventsOn('llmFinish', () => {
  console.log('[llmFinish] received, messages:', messages.value)
  // 流式结束，消息已填充
})

// 语音识别最终结果 → 只显示用户消息到对话（Go 后端已通过 onUserInput 处理 Agent 调用）
EventsOn('voiceTranscript', (data: { text: string; is_final: boolean }) => {
  if (data.is_final && data.text) {
    console.log('[voiceTranscript] final:', data.text)
    // 只添加用户消息到 UI，不调 handleSubmit（后端会 emit llmChunk 事件）
    messages.value.push({ role: 'user', content: data.text })
    messages.value.push({ role: 'assistant', content: '' })
  }
})

// ---- console 重定向到日志面板 ----
function redirectConsole(
  original: (...args: unknown[]) => void,
  level: string,
) {
  return (...args: unknown[]) => {
    original(...args)
    args.forEach((arg) => {
      addLog(level, typeof arg === 'string' ? arg : JSON.stringify(arg))
    })
  }
}

console.log = redirectConsole(console.log.bind(console), 'debug')
console.warn = redirectConsole(console.warn.bind(console), 'warn')
console.error = redirectConsole(console.error.bind(console), 'error')

// ---- 截图测试 ----
const screenshotting = ref(false)
const screenshotResult = ref('')
const screenshotImage = ref('')

async function testScreenshot() {
  screenshotting.value = true
  screenshotResult.value = ''
  screenshotImage.value = ''
  try {
    const result: ScreenshotResult = await captureFullScreenshot()
    const kb = Math.round(result.data.length * 0.75 / 1024) // base64 长度转字节数
    screenshotResult.value = `✓ 截图成功: ${result.width} × ${result.height}, ${kb} KB`
    // data 已是 base64，直接用于 img src
    screenshotImage.value = `data:image/${result.format};base64,` + result.data
  } catch (err) {
    screenshotResult.value = `✗ 截图失败: ${(err as Error).message || String(err)}`
  } finally {
    screenshotting.value = false
  }
}

// ---- 启动 ----
onMounted(() => {
  console.log('GameAI initialized (Vue)')
  // 启动回执：确认前端脚本加载成功（后端日志可见）
  try { frontendEcho('frontend-loaded') } catch { /* ignore */ }
})
</script>

<template>
  <header class="app-header">
    <h1>GameAI</h1>
    <p>Windows 桌面 AI 游戏助手</p>
  </header>

  <main class="app-content">
    <div class="welcome-card">
      <h2>欢迎使用 GameAI</h2>
      <p>
        按下 <kbd>Ctrl + Alt + R</kbd> 全局快捷键呼出快速提问。
        按住 <kbd>Ctrl + Shift + R</kbd> 语音输入（Push-To-Talk）。
        也可以右键系统托盘图标，点击"快速提问"。
      </p>
      <ul class="feature-list">
        <li>✅ Go 核心逻辑</li>
        <li>✅ Vue 3 + Vite 前端</li>
        <li>✅ Wails 桌面框架</li>
        <li>✅ 系统托盘</li>
        <li>✅ 全局快捷键</li>
        <li>✅ 快速输入框</li>
        <li>✅ Push-To-Talk 语音输入</li>
      </ul>
    </div>

    <Conversation v-if="messages.length > 0" :messages="messages" />

    <!-- 截图测试 -->
    <div class="screenshot-test">
      <button :disabled="screenshotting" @click="testScreenshot">
        {{ screenshotting ? '截图中...' : '测试全屏截图' }}
      </button>
      <p v-if="screenshotResult">{{ screenshotResult }}</p>
      <img v-if="screenshotImage" :src="screenshotImage" alt="截图预览" />
    </div>

    <Settings />

    <LogPanel :logs="logs" />
  </main>

  <footer class="app-footer">
    <p>GameAI © 2025</p>
  </footer>

  <QuickInput v-model:visible="quickInputVisible" @submit="handleSubmit" />
  <VoiceIndicator />
</template>

<style scoped>
.app-header {
  background-color: var(--bg-secondary);
  padding: 1rem 2rem;
  border-bottom: 1px solid var(--border-color);
}

.app-header h1 {
  font-size: 1.5rem;
  font-weight: 600;
  margin-bottom: 0.25rem;
}

.app-header p {
  color: var(--text-secondary);
  font-size: 0.9rem;
}

.app-content {
  flex: 1;
  padding: 2rem;
  max-width: 800px;
  margin: 0 auto;
  width: 100%;
}

.welcome-card {
  background-color: var(--bg-secondary);
  border-radius: 8px;
  padding: 2rem;
  border: 1px solid var(--border-color);
}

.welcome-card h2 {
  font-size: 1.5rem;
  margin-bottom: 1rem;
  color: var(--accent-color);
}

.welcome-card p {
  color: var(--text-secondary);
  margin-bottom: 1.5rem;
}

.feature-list {
  list-style: none;
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(160px, 1fr));
  gap: 0.75rem;
}

.feature-list li {
  background-color: var(--bg-primary);
  padding: 0.75rem 1rem;
  border-radius: 4px;
  border: 1px solid var(--border-color);
  font-size: 0.9rem;
}

.screenshot-test {
  margin-top: 20px;
}

.screenshot-test button {
  padding: 10px 20px;
  background: var(--accent-color);
  color: white;
  border: none;
  border-radius: 4px;
  cursor: pointer;
}

.screenshot-test button:disabled {
  opacity: 0.6;
  cursor: wait;
}

.screenshot-test p {
  margin-top: 10px;
}

.screenshot-test img {
  max-width: 100%;
  border: 2px solid var(--border-color);
  border-radius: 4px;
  margin-top: 10px;
}

.app-footer {
  background-color: var(--bg-secondary);
  padding: 1rem 2rem;
  border-top: 1px solid var(--border-color);
  text-align: center;
  color: var(--text-secondary);
  font-size: 0.85rem;
}
</style>
