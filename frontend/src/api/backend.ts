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
  /**
   * - 'user'      : user input
   * - 'assistant' : Aurora reply (streamed via llmChunk)
   * - 'error'     : system error card (Phase 6 A7 — distinct from assistant text
   *                 so we can render a proper error state instead of `Error: ...`)
   */
  role: 'user' | 'assistant' | 'error'
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

export interface ActiveGame {
  active: boolean
  id?: string
  name?: string
  window?: string
}

export async function getActiveGame(): Promise<ActiveGame> {
  return agentBindings().GetActiveGame()
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

// ---- voice mode ----

export type VoiceMode = 'ptt' | 'wakeword' | 'continuous'

export async function getVoiceMode(): Promise<VoiceMode> {
  return agentBindings().GetVoiceMode()
}

export async function setVoiceMode(mode: VoiceMode): Promise<void> {
  return agentBindings().SetVoiceMode(mode)
}

export async function cycleVoiceMode(): Promise<VoiceMode> {
  return agentBindings().CycleVoiceMode()
}

export async function getVoiceState(): Promise<string> {
  return agentBindings().GetVoiceState()
}

export async function isAgentBusy(): Promise<boolean> {
  return agentBindings().IsAgentBusy()
}

export async function isTTSPlaying(): Promise<boolean> {
  return agentBindings().IsTTSPlaying()
}

// ---- TTS ----

export interface TTSConfig {
  provider: string
  resource_id: string
  voice_type: string
  format: string
  sample_rate: number
  speech_rate: number
  loudness_rate: number
  pitch: number
  has_api_key: boolean
}

export async function getTTSConfig(): Promise<TTSConfig> {
  return agentBindings().GetTTSConfig()
}

export async function saveTTSConfig(
  provider: string,
  resourceId: string,
  speaker: string,
  format: string,
  sampleRate: number,
  speechRate: number,
  loudnessRate: number,
  pitch: number,
  apiKey: string
): Promise<void> {
  return agentBindings().SaveTTSConfig(
    provider, resourceId, speaker, format,
    sampleRate, speechRate, loudnessRate, pitch, apiKey
  )
}

export async function testTTS(text: string): Promise<void> {
  return agentBindings().TestTTS(text)
}

// ---- Overlay (Phase 3) ----

export interface OverlayConfig {
  enabled: boolean
  opacity: number
  font_size: 'small' | 'medium' | 'large'
  auto_scroll: boolean
  show_user_messages: boolean
  show_assistant_messages: boolean
  anchor: 'top-left' | 'top-right' | 'bottom-left' | 'bottom-right'
}

export async function getOverlayConfig(): Promise<OverlayConfig> {
  return agentBindings().GetOverlayConfig()
}

export async function saveOverlayConfig(cfg: OverlayConfig): Promise<void> {
  return agentBindings().SaveOverlayConfig(
    cfg.enabled,
    cfg.opacity,
    cfg.font_size,
    cfg.auto_scroll,
    cfg.show_user_messages,
    cfg.show_assistant_messages,
    cfg.anchor,
  )
}

export async function setOverlayVisible(visible: boolean): Promise<void> {
  return agentBindings().SetOverlayVisible(visible)
}

export async function getOverlayVisible(): Promise<boolean> {
  return agentBindings().GetOverlayVisible()
}
