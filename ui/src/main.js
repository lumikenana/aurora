// GameAI 前端入口
// 使用 Wails v2 运行时事件机制（EventsOn）监听后端事件
// 注意：原生 JS module 不支持 import CSS（style.css 已在 index.html 中通过 <link> 引入）

import { EventsOn } from './wailsjs/runtime/runtime.js'

const quickInputOverlay = document.getElementById('quick-input-overlay')
const quickInputTextarea = document.getElementById('quick-input-textarea')
const conversationContainer = document.getElementById('conversation')

// Wails 绑定的后端方法（运行时注册在 window.go 下，context 参数由运行时自动处理）
const AgentBindings = () => window.go.agent.Bindings
const InputBindings = () => window.go.input.Bindings

// 缓存早期日志，等 DOM 加载后再显示
let logContainer = null
let earlyLogs = []

// 监听来自Go后端的快速输入显示事件
EventsOn('showQuickInput', () => {
  // 回执：确认前端收到了事件（后端日志可见）
  try { InputBindings().FrontendEcho('showQuickInput') } catch (e) { /* ignore */ }
  showQuickInput()
})

// 显示快速输入框
function showQuickInput() {
  quickInputOverlay.classList.remove('hidden')
  quickInputTextarea.value = ''
  quickInputTextarea.focus()
  window.focus()
}

// 隐藏快速输入框
function hideQuickInput() {
  quickInputOverlay.classList.add('hidden')
  quickInputTextarea.value = ''
}

// 添加消息到对话
function addMessage(role, content) {
  const messageDiv = document.createElement('div')
  messageDiv.className = `message ${role}`

  const headerDiv = document.createElement('div')
  headerDiv.className = 'message-header'
  headerDiv.textContent = role === 'user' ? '你' : 'AI'

  const contentDiv = document.createElement('div')
  contentDiv.className = 'message-content'
  contentDiv.textContent = content

  messageDiv.appendChild(headerDiv)
  messageDiv.appendChild(contentDiv)
  conversationContainer.appendChild(messageDiv)

  // 滚动到底部
  conversationContainer.scrollTop = conversationContainer.scrollHeight

  return contentDiv
}

// 追加内容到最后一条助手消息（流式响应）
function appendToLastAssistantMessage(content) {
  const messages = conversationContainer.querySelectorAll('.message.assistant')
  let contentDiv

  if (messages.length === 0) {
    // 没有助手消息，创建新的
    const messageDiv = document.createElement('div')
    messageDiv.className = 'message assistant'

    const headerDiv = document.createElement('div')
    headerDiv.className = 'message-header'
    headerDiv.textContent = 'AI'

    contentDiv = document.createElement('div')
    contentDiv.className = 'message-content'
    contentDiv.textContent = ''

    messageDiv.appendChild(headerDiv)
    messageDiv.appendChild(contentDiv)
    conversationContainer.appendChild(messageDiv)
  } else {
    const lastMessage = messages[messages.length - 1]
    contentDiv = lastMessage.querySelector('.message-content')
  }

  contentDiv.textContent += content
  conversationContainer.scrollTop = conversationContainer.scrollHeight
}

// 处理快速输入提交
async function handleSubmit(text) {
  if (!text) return

  // 添加用户消息
  addMessage('user', text)

  // 检查 Agent 是否配置
  // 注意：agent.Bindings 的 Go 方法带 context.Context 参数，
  // 此版本的 Wails 要求前端传占位参数（null），运行时会自动注入真实 ctx
  let status
  try {
    status = await AgentBindings().GetConfigStatus(null)
  } catch (err) {
    addMessage('assistant', `获取配置状态失败: ${err.message || String(err)}`)
    return
  }
  if (!status.configured) {
    addMessage('assistant', 'LLM 未配置，请配置 API Key 后重试。')
    return
  }

  try {
    // 创建空的助手消息用于流式填充
    addMessage('assistant', '')
    // 开始流式响应
    await AgentBindings().ChatStream(null, text)
  } catch (err) {
    addMessage('assistant', `错误: ${err.message || String(err)}`)
  }
}

// 处理键盘事件
quickInputTextarea.addEventListener('keydown', async (e) => {
  // Esc 关闭窗口
  if (e.key === 'Escape') {
    e.preventDefault()
    try { await InputBindings().CancelQuickInput() } catch (err) { /* ignore */ }
    hideQuickInput()
    return
  }

  // Enter 提交（如果没有按住 Shift）
  if (e.key === 'Enter' && !e.shiftKey) {
    e.preventDefault()
    const text = quickInputTextarea.value.trim()
    if (text) {
      try {
        await handleSubmit(text)
        await InputBindings().SubmitQuickInput(text)
      } catch (err) {
        addMessage('assistant', `错误: ${err.message || String(err)}`)
      } finally {
        // 无论成功失败都关闭快速输入窗口
        hideQuickInput()
      }
    } else {
      try { await InputBindings().CancelQuickInput() } catch (err) { /* ignore */ }
      hideQuickInput()
    }
    return
  }
})

// 点击遮罩层关闭
quickInputOverlay.addEventListener('click', (e) => {
  if (e.target === quickInputOverlay) {
    hideQuickInput()
  }
})

// 流式响应事件：接收 chunk
EventsOn('llmChunk', (content) => {
  if (content) {
    appendToLastAssistantMessage(content)
  }
})

// 流式响应结束事件
EventsOn('llmFinish', () => {
  // 流式结束，无需额外处理（消息已填充）
})

// 测试全屏截图按钮
document.addEventListener('DOMContentLoaded', () => {
  const testDiv = document.createElement('div')
  testDiv.style.marginTop = '20px'
  testDiv.innerHTML = `
    <button id="testScreenshotBtn" style="
      padding: 10px 20px;
      background: #007acc;
      color: white;
      border: none;
      border-radius: 4px;
      cursor: pointer;
    ">测试全屏截图</button>
    <div id="screenshotResult" style="margin-top: 10px;"></div>
  `
  document.querySelector('.app-content').appendChild(testDiv)

  document.getElementById('testScreenshotBtn').addEventListener('click', async function() {
    const btn = this
    btn.disabled = true
    btn.textContent = '截图中...'

    try {
      const result = await AgentBindings().CaptureFullScreenshot(null)
      console.log('截图结果:', result)

      // 显示结果
      const container = document.getElementById('screenshotResult')
      container.innerHTML = `<p>✓ 截图成功: ${result.width} × ${result.height}, ${(result.data.length / 1024).toFixed(1)} KB</p>`

      // 显示图片
      const img = document.createElement('img')
      img.style.maxWidth = '100%'
      img.style.border = '2px solid #ccc'
      img.style.borderRadius = '4px'
      img.style.marginTop = '10px'

      // 将 Uint8Array 转 base64
      const binary = result.data.reduce((acc, byte) => acc + String.fromCharCode(byte), '')
      img.src = 'data:image/jpeg;base64,' + btoa(binary)
      container.appendChild(img)
    } catch (err) {
      console.error('截图失败:', err)
      const container = document.getElementById('screenshotResult')
      container.innerHTML = `<p style="color: red;">✗ 截图失败: ${err.message || String(err)}</p>`
    } finally {
      btn.disabled = false
      btn.textContent = '测试全屏截图'
    }
  })
})

document.addEventListener('DOMContentLoaded', () => {
  console.log('GameAI initialized')
  // 获取日志容器并显示缓存的早期日志
  logContainer = document.getElementById('log-container')
  // 播放缓存的早期日志
  if (earlyLogs.length > 0) {
    earlyLogs.forEach(({ level, message }) => addLogEntry(level, message))
    earlyLogs = []
  }
})

// 添加日志到日志面板
function addLogEntry(level, message) {
  // 如果 DOM 还没准备好，缓存起来
  if (!logContainer) {
    earlyLogs.push({ level, message })
    return
  }

  const entryDiv = document.createElement('div')
  entryDiv.className = `log-entry ${level}`
  entryDiv.textContent = `[${new Date().toLocaleTimeString()}] [${level.toUpperCase()}] ${message}`
  logContainer.appendChild(entryDiv)

  // 滚动到底部
  logContainer.scrollTop = logContainer.scrollHeight

  // 限制最大行数，避免过多DOM
  const entries = logContainer.querySelectorAll('.log-entry')
  if (entries.length > 500) {
    logContainer.removeChild(entries[0])
  }
}

// 监听后端日志事件（Wails 事件，data 为 {level, message}）
EventsOn('log', (data) => {
  if (data && data.level && data.message) {
    addLogEntry(data.level, data.message)
  }
})

// 将浏览器console输出重定向到日志面板
const originalConsoleLog = console.log
const originalConsoleWarn = console.warn
const originalConsoleError = console.error

console.log = function(...args) {
  originalConsoleLog.apply(console, args)
  args.forEach(arg => {
    addLogEntry('debug', typeof arg === 'string' ? arg : JSON.stringify(arg))
  })
}

console.warn = function(...args) {
  originalConsoleWarn.apply(console, args)
  args.forEach(arg => {
    addLogEntry('warn', typeof arg === 'string' ? arg : JSON.stringify(arg))
  })
}

console.error = function(...args) {
  originalConsoleError.apply(console, args)
  args.forEach(arg => {
    addLogEntry('error', typeof arg === 'string' ? arg : JSON.stringify(arg))
  })
}

// 启动回执：确认前端脚本加载成功（后端日志可见）
try { InputBindings().FrontendEcho('frontend-loaded') } catch (e) { /* ignore */ }
