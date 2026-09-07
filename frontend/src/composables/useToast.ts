// useToast - 统一轻提示（UI 重构：取代 alert() / 各组件零散的错误提示）
//
// 用法：
//   const toast = useToast()
//   toast.success('已保存')
//   toast.error(`保存失败：${msg}`)
//
// 全局唯一实例（模块级单例），由挂载在 App.vue / overlay-main.ts /
// quickinput-main.ts 的 <ToastHost /> 渲染。
//
// 设计原则（CLAUDE.md 风格）：
//   - 不引入第三方库；进程内简单 map/state
//   - 自动消失（默认 3s）；error 稍长（4.5s）
//   - 同一时刻最多 5 条，超出丢弃最旧

import { reactive } from 'vue'

export type ToastTone = 'success' | 'error' | 'info' | 'warn'

export interface ToastItem {
  id: number
  tone: ToastTone
  text: string
  leaving: boolean
}

const state = reactive<{ items: ToastItem[] }>({ items: [] })

let nextId = 1
const MAX_TOASTS = 5

function push(tone: ToastTone, text: string, durationMs: number) {
  const id = nextId++
  state.items.push({ id, tone, text, leaving: false })
  if (state.items.length > MAX_TOASTS) {
    state.items.splice(0, state.items.length - MAX_TOASTS)
  }
  window.setTimeout(() => dismiss(id), durationMs)
}

function dismiss(id: number) {
  const item = state.items.find((t) => t.id === id)
  if (!item || item.leaving) return
  item.leaving = true
  // 与 toast-out 动画时长一致，之后移除
  window.setTimeout(() => {
    const idx = state.items.findIndex((t) => t.id === id)
    if (idx >= 0) state.items.splice(idx, 1)
  }, 200)
}

export function useToast() {
  return {
    items: state.items,
    dismiss,
    success: (text: string) => push('success', text, 3000),
    error: (text: string) => push('error', text, 4500),
    info: (text: string) => push('info', text, 3000),
    warn: (text: string) => push('warn', text, 4000),
  }
}
