// 后端 bindings 统一封装
//
// 注意：agent.Bindings 的 Go 方法第一个参数是 context.Context，
// 此版本的 Wails 要求前端按参数个数传占位符（null），运行时会自动注入真实 ctx；
// input.Bindings 的方法没有 ctx 参数，直接正常调用。

/* eslint-disable @typescript-eslint/no-explicit-any */

function agentBindings(): any {
  return (window as any).go.agent.Bindings
}

function inputBindings(): any {
  return (window as any).go.input.Bindings
}

export interface ChatMessage {
  role: 'user' | 'assistant'
  content: string
}

export interface ScreenshotResult {
  width: number
  height: number
  format: string
  data: Uint8Array | number[]
}

// ---- agent ----

export async function isReady(): Promise<boolean> {
  return agentBindings().IsReady(null)
}

export async function getConfigStatus(): Promise<{ initialized: boolean; configured: boolean; provider?: string }> {
  return agentBindings().GetConfigStatus(null)
}

export async function chatStream(userMessage: string): Promise<void> {
  return agentBindings().ChatStream(null, userMessage)
}

export async function chat(userMessage: string): Promise<string> {
  return agentBindings().Chat(null, userMessage)
}

export async function clearHistory(): Promise<void> {
  return agentBindings().ClearHistory(null)
}

export async function captureFullScreenshot(): Promise<ScreenshotResult> {
  return agentBindings().CaptureFullScreenshot(null)
}

// ---- input ----

export async function frontendEcho(event: string): Promise<void> {
  return inputBindings().FrontendEcho(event)
}

export async function cancelQuickInput(): Promise<void> {
  return inputBindings().CancelQuickInput()
}

export async function submitQuickInput(text: string): Promise<void> {
  return inputBindings().SubmitQuickInput(text)
}
