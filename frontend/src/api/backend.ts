// 后端 bindings 统一封装
//
// 注意：所有 Go 绑定方法都不带 context.Context 参数
// （此版本 Wails 对 ctx 参数处理有缺陷，传 null 会导致
//   reflect: Call using zero Value argument，Promise 永远 pending）

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
  // Go 的 []byte 经 Wails 序列化后直接就是 base64 字符串
  data: string
}

// ---- agent ----

export async function isReady(): Promise<boolean> {
  return agentBindings().IsReady()
}

export async function getConfigStatus(): Promise<{ initialized: boolean; configured: boolean; provider?: string }> {
  return agentBindings().GetConfigStatus()
}

export async function chatStream(userMessage: string): Promise<void> {
  return agentBindings().ChatStream(userMessage)
}

export async function chat(userMessage: string): Promise<string> {
  return agentBindings().Chat(userMessage)
}

export async function clearHistory(): Promise<void> {
  return agentBindings().ClearHistory()
}

export async function captureFullScreenshot(): Promise<ScreenshotResult> {
  return agentBindings().CaptureFullScreenshot()
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

// ---- settings ----

export async function getLLMConfig(): Promise<{
  provider: string
  model: string
  base_url: string
  has_api_key: boolean
}> {
  return agentBindings().GetLLMConfig()
}

export async function saveLLMConfig(
  provider: string,
  model: string,
  baseURL: string,
  apiKey: string
): Promise<void> {
  return agentBindings().SaveLLMConfig(provider, model, baseURL, apiKey)
}

// ---- speech / PTT ----

export async function isPTTAvailable(): Promise<boolean> {
  return agentBindings().IsPTTAvailable()
}

export async function isPTTRecording(): Promise<boolean> {
  return agentBindings().IsPTTRecording()
}

export async function getPTTState(): Promise<number> {
  return agentBindings().GetPTTState()
}

export async function getSpeechConfig(): Promise<{
  provider: string
  endpoint: string
  resource_id: string
  has_api_key: boolean
}> {
  return agentBindings().GetSpeechConfig()
}

export async function saveSpeechConfig(
  provider: string,
  endpoint: string,
  resourceID: string,
  apiKey: string
): Promise<void> {
  return agentBindings().SaveSpeechConfig(provider, endpoint, resourceID, apiKey)
}

// ---- wake word ----

export interface WakeWordConfig {
  enabled: boolean
  model_path: string
  threshold: number
  cooldown_ms: number
  vad_enabled: boolean
  onnx_runtime_path: string
}

export async function getWakeWordConfig(): Promise<WakeWordConfig> {
  return agentBindings().GetWakeWordConfig()
}

export async function saveWakeWordConfig(
  enabled: boolean,
  modelPath: string,
  threshold: number,
  cooldownMs: number,
  vadEnabled: boolean,
  onnxRuntimePath: string
): Promise<void> {
  return agentBindings().SaveWakeWordConfig(
    enabled,
    modelPath,
    threshold,
    cooldownMs,
    vadEnabled,
    onnxRuntimePath
  )
}
