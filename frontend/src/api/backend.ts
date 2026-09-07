// 后端 bindings 统一封装
//
// Phase 8: Wails v2 → v3 迁移。
// v2 时代通过 window.go.<pkg>.Bindings 全局对象调用；v3 改为
// `wails3 generate bindings` 生成的 ESM 模块（@wailsio/runtime 的 Call.ByID）。
// 方法名与 Go 导出方法一一对应，所有调用点（agentBindings().X / inputBindings().X）
// 契约保持不变，Vue 组件无需感知迁移。
//
// 注意：所有 Go 绑定方法都不带 context.Context 参数（ADR-010）

/* eslint-disable @typescript-eslint/no-explicit-any */

import * as agentModule from '../../bindings/github.com/lipipi/aurora/internal/agent/bindings.js'
import * as inputModule from '../../bindings/github.com/lipipi/aurora/internal/input/bindings.js'

function agentBindings(): any {
  return agentModule
}

function inputBindings(): any {
  return inputModule
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

export interface GameContextResponse {
  available: boolean
  game: string
  name: string
  phase: string
  detected_at?: string
  window_title?: string
  window_class?: string
  game_data_type?: string
}

export async function getGameContext(): Promise<GameContextResponse> {
  return agentBindings().GetGameContext()
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

// ---- Overlay 锁定态 / 窗口几何（UI 重构：Edit Mode vs HUD） ----

export async function getOverlayLocked(): Promise<boolean> {
  return agentBindings().GetOverlayLocked()
}

export async function setOverlayLocked(locked: boolean): Promise<void> {
  return agentBindings().SetOverlayLocked(locked)
}

export interface OverlayGeometry {
  x: number
  y: number
  width: number
  height: number
  config_x: number
  config_y: number
  config_w: number
  config_h: number
  anchor: string
  locked: boolean
  min_width: number
  min_height: number
  max_width: number
  max_height: number
  default_w: number
  default_h: number
  custom_geom: boolean
}

export async function getOverlayGeometry(): Promise<OverlayGeometry> {
  return agentBindings().GetOverlayGeometry()
}

// Edit Mode 拖拽 / 缩放中的实时反馈（不持久化；物理像素）
export async function moveOverlayWindow(x: number, y: number): Promise<void> {
  return agentBindings().MoveOverlayWindow(x, y)
}

export async function resizeOverlayWindow(w: number, h: number): Promise<void> {
  return agentBindings().ResizeOverlayWindow(w, h)
}

// 拖拽 / 缩放结束提交（clamp + 持久化 + 广播 overlayConfigChanged）
export async function commitOverlayGeometry(
  x: number,
  y: number,
  w: number,
  h: number
): Promise<void> {
  return agentBindings().CommitOverlayGeometry(x, y, w, h)
}

export async function resetOverlayGeometry(): Promise<void> {
  return agentBindings().ResetOverlayGeometry()
}

// ---- 语音（stop speaking 快捷键 / 按钮） ----

export async function stopSpeaking(): Promise<void> {
  return agentBindings().StopSpeaking()
}

// ---- 快捷键（UI 重构：统一 Registry + 可配置 + 冲突检测） ----

export type ShortcutAction =
  | 'quick_input'
  | 'ptt'
  | 'cycle_voice_mode'
  | 'toggle_main_window'
  | 'toggle_overlay'
  | 'toggle_overlay_lock'
  | 'emergency_hide'
  | 'analyze_screen'
  | 'stop_speaking'

export async function getShortcuts(): Promise<Record<string, string>> {
  return inputBindings().GetShortcuts()
}

// 冲突时 reject（错误信息为后端 ShortcutConflictError 格式，前端展示原文）
export async function setShortcut(
  action: ShortcutAction,
  combo: string
): Promise<Record<string, string>> {
  return inputBindings().SetShortcut(action, combo)
}

export async function resetShortcuts(): Promise<Record<string, string>> {
  return inputBindings().ResetShortcuts()
}
