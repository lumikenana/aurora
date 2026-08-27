<script setup lang="ts">
import { ref, onMounted, onUnmounted } from 'vue'
import { getLLMConfig, saveLLMConfig, getSpeechConfig, saveSpeechConfig, getWakeWordConfig, saveWakeWordConfig, getTTSConfig, saveTTSConfig, testTTS, getVoiceMode, setVoiceMode } from '../api/backend'
import { EventsOn, EventsOff } from '../api/events'

const provider = ref('openai')
const model = ref('doubao-seed-2.0-lite')
const baseURL = ref('https://ark.cn-beijing.volces.com/api/plan/v3')
const apiKey = ref('')
const hasApiKey = ref(false)
const saving = ref(false)
const saved = ref(false)

// Speech / ASR config
const speechProvider = ref('volcengine')
const speechEndpoint = ref('wss://openspeech.bytedance.com/api/v3/sauc/bigmodel_async')
const speechResourceID = ref('volc.seedasr.sauc.duration')
const speechApiKey = ref('')
const speechHasApiKey = ref(false)
const speechSaving = ref(false)
const speechSaved = ref(false)

// Wake Word config
const wakeWordEnabled = ref(false)
const wakeWordModelPath = ref('')
const wakeWordThreshold = ref(0.5)
const wakeWordCooldownMs = ref(1000)
const wakeWordVADEnabled = ref(true)
const wakeWordOnnxRuntimePath = ref('')
const wakeWordSaving = ref(false)
const wakeWordSaved = ref(false)

// Download progress state
const wakeWordDownloading = ref(false)
const wakeWordDownloadPercent = ref(0)
const wakeWordDownloadMessage = ref('')
const wakeWordDownloadError = ref('')

function handleDownloadProgress(data: { percent: number; message: string }) {
  wakeWordDownloading.value = true
  wakeWordDownloadPercent.value = data.percent
  wakeWordDownloadMessage.value = data.message
  wakeWordDownloadError.value = ''
}

function handleDownloadError(data: { error: string }) {
  wakeWordDownloadError.value = data.error
  wakeWordDownloading.value = false
}

function handleDownloadComplete() {
  wakeWordDownloading.value = false
  wakeWordDownloadPercent.value = 100
  wakeWordDownloadMessage.value = '下载完成'
  // Reload after download completes
  setTimeout(() => {
    location.reload()
  }, 1500)
}

async function loadConfig() {
  try {
    const config = await getLLMConfig()
    provider.value = config.provider || 'openai'
    model.value = config.model || 'doubao-seed-2.0-lite'
    baseURL.value = config.base_url || 'https://ark.cn-beijing.volces.com/api/plan/v3'
    hasApiKey.value = config.has_api_key
    if (hasApiKey.value) {
      apiKey.value = '********' // 不显示真实 API Key
    }
  } catch (err) {
    console.error('Failed to load config:', err)
  }
}

async function saveConfig() {
  saving.value = true
  saved.value = false
  try {
    // If apiKey is still '********', send empty string to backend
    // so backend won't overwrite the existing API Key
    let sendApiKey = apiKey.value
    if (sendApiKey === '********') {
      sendApiKey = ''
    }
    await saveLLMConfig(provider.value, model.value, baseURL.value, sendApiKey)
    saved.value = true
    hasApiKey.value = sendApiKey !== '' || hasApiKey.value
    if (sendApiKey) {
      apiKey.value = '********'
    }
    setTimeout(() => {
      saved.value = false
    }, 3000)
  } catch (err) {
    console.error('Failed to save config:', err)
    alert(`保存失败: ${(err as Error).message || String(err)}`)
  } finally {
    saving.value = false
  }
}

function handleApiKeyInput(e: Event) {
  const target = e.target as HTMLInputElement
  if (target.value === '********') {
    target.value = ''
    apiKey.value = ''
  }
}

async function loadSpeechConfig() {
  try {
    const config = await getSpeechConfig()
    speechProvider.value = config.provider || 'volcengine'
    speechEndpoint.value = config.endpoint || 'wss://openspeech.bytedance.com/api/v3/sauc/bigmodel_async'
    speechResourceID.value = config.resource_id || 'volc.seedasr.sauc.duration'
    speechHasApiKey.value = config.has_api_key
    if (speechHasApiKey.value) {
      speechApiKey.value = '********'
    }
  } catch (err) {
    console.error('Failed to load speech config:', err)
  }
}

async function saveSpeech() {
  speechSaving.value = true
  speechSaved.value = false
  try {
    let sendKey = speechApiKey.value
    if (sendKey === '********') {
      sendKey = ''
    }
    await saveSpeechConfig(speechProvider.value, speechEndpoint.value, speechResourceID.value, sendKey)
    speechSaved.value = true
    speechHasApiKey.value = sendKey !== '' || speechHasApiKey.value
    if (sendKey) {
      speechApiKey.value = '********'
    }
    setTimeout(() => { speechSaved.value = false }, 3000)
  } catch (err) {
    console.error('Failed to save speech config:', err)
    alert(`保存失败: ${(err as Error).message || String(err)}`)
  } finally {
    speechSaving.value = false
  }
}

function handleSpeechApiKeyInput(e: Event) {
  const target = e.target as HTMLInputElement
  if (target.value === '********') {
    target.value = ''
    speechApiKey.value = ''
  }
}

// Wake Word config loading
async function loadWakeWordConfig() {
  try {
    const config = await getWakeWordConfig()
    wakeWordEnabled.value = config.enabled ?? false
    wakeWordModelPath.value = config.model_path ?? ''
    wakeWordThreshold.value = config.threshold ?? 0.5
    wakeWordCooldownMs.value = config.cooldown_ms ?? 1000
    wakeWordVADEnabled.value = config.vad_enabled ?? true
    wakeWordOnnxRuntimePath.value = config.onnx_runtime_path ?? ''
  } catch (err) {
    console.error('Failed to load wake word config:', err)
  }
}

async function saveWakeWord() {
  wakeWordSaving.value = true
  wakeWordSaved.value = false
  try {
    await saveWakeWordConfig(
      wakeWordEnabled.value,
      wakeWordModelPath.value,
      wakeWordThreshold.value,
      wakeWordCooldownMs.value,
      wakeWordVADEnabled.value,
      wakeWordOnnxRuntimePath.value
    )
    wakeWordSaved.value = true
    setTimeout(() => { wakeWordSaved.value = false }, 3000)
  } catch (err) {
    console.error('Failed to save wake word config:', err)
    alert(`保存失败: ${(err as Error).message || String(err)}`)
  } finally {
    wakeWordSaving.value = false
  }
}

onMounted(() => {
  loadConfig()
  loadSpeechConfig()
  loadWakeWordConfig()
  loadTTSConfig()
  loadVoiceMode()

  // Register download progress event listeners
  EventsOn('wakewordDownloadProgress', handleDownloadProgress)
  EventsOn('wakewordDownloadError', handleDownloadError)
  EventsOn('wakewordDownloadComplete', handleDownloadComplete)
})

onUnmounted(() => {
  // Clean up event listeners
  EventsOff('wakewordDownloadProgress')
  EventsOff('wakewordDownloadError')
  EventsOff('wakewordDownloadComplete')
})

// TTS config
const ttsProvider = ref('volcengine')
const ttsResourceId = ref('seed-tts-2.0')
const ttsSpeaker = ref('zh_female_xiaohe_uranus_bigtts')
const ttsFormat = ref('mp3')
const ttsSampleRate = ref(24000)
const ttsSpeechRate = ref(0)
const ttsLoudnessRate = ref(0)
const ttsPitch = ref(0)
const ttsApiKey = ref('')
const ttsHasApiKey = ref(false)
const ttsSaving = ref(false)
const ttsSaved = ref(false)
const ttsTesting = ref(false)
const ttsTestText = ref('你好，我是 GameAI 语音助手。')

async function loadTTSConfig() {
  try {
    const c = await getTTSConfig()
    ttsProvider.value = c.provider || 'volcengine'
    ttsResourceId.value = c.resource_id || 'seed-tts-2.0'
    ttsSpeaker.value = c.voice_type || 'zh_female_xiaohe_uranus_bigtts'
    ttsFormat.value = c.format || 'mp3'
    ttsSampleRate.value = c.sample_rate || 24000
    ttsSpeechRate.value = c.speech_rate ?? 0
    ttsLoudnessRate.value = c.loudness_rate ?? 0
    ttsPitch.value = c.pitch ?? 0
    ttsHasApiKey.value = c.has_api_key
    if (ttsHasApiKey.value) {
      ttsApiKey.value = '********'
    }
  } catch (err) {
    console.error('Failed to load TTS config:', err)
  }
}

async function saveTTS() {
  ttsSaving.value = true
  ttsSaved.value = false
  try {
    const apiKeyToSend = ttsApiKey.value === '********' ? '' : ttsApiKey.value
    await saveTTSConfig(
      ttsProvider.value,
      ttsResourceId.value,
      ttsSpeaker.value,
      ttsFormat.value,
      ttsSampleRate.value,
      ttsSpeechRate.value,
      ttsLoudnessRate.value,
      ttsPitch.value,
      apiKeyToSend
    )
    ttsSaved.value = true
    setTimeout(() => { ttsSaved.value = false }, 2000)
    await loadTTSConfig()
  } catch (err) {
    console.error('Failed to save TTS config:', err)
    alert('保存失败：' + err)
  } finally {
    ttsSaving.value = false
  }
}

async function handleTestTTS() {
  ttsTesting.value = true
  try {
    await testTTS(ttsTestText.value)
  } catch (err) {
    console.error('Test TTS failed:', err)
    alert('测试失败：' + err)
  } finally {
    ttsTesting.value = false
  }
}

// Voice mode config
const voiceMode = ref<'ptt' | 'wakeword' | 'continuous'>('wakeword')
const voiceModeSaving = ref(false)
const voiceModeSaved = ref(false)
const continuousTimeoutSec = ref(30)
const continuousExitCmds = ref('结束对话,停止,退出对话,exit,stop')

async function loadVoiceMode() {
  try {
    voiceMode.value = await getVoiceMode()
  } catch (err) {
    console.error('Failed to load voice mode:', err)
  }
}

async function changeVoiceMode(mode: 'ptt' | 'wakeword' | 'continuous') {
  voiceMode.value = mode
  voiceModeSaving.value = true
  voiceModeSaved.value = false
  try {
    await setVoiceMode(mode)
    voiceModeSaved.value = true
    setTimeout(() => { voiceModeSaved.value = false }, 1500)
  } catch (err) {
    console.error('Failed to set voice mode:', err)
    alert('切换模式失败：' + err)
  } finally {
    voiceModeSaving.value = false
  }
}
</script>

<template>
  <div class="settings-panel">
    <h2>设置</h2>
    <div class="setting-item">
      <label>LLM Provider</label>
      <input v-model="provider" placeholder="openai" />
    </div>
    <div class="setting-item">
      <label>模型名称</label>
      <input v-model="model" placeholder="doubao-seed-2.0-lite" />
    </div>
    <div class="setting-item">
      <label>API Base URL</label>
      <input v-model="baseURL" placeholder="https://ark.cn-beijing.volces.com/api/plan/v3" />
    </div>
    <div class="setting-item">
      <label>API Key</label>
      <input
        v-model="apiKey"
        type="password"
        placeholder="输入你的 API Key"
        @focus="handleApiKeyInput"
      />
    </div>
    <div class="setting-actions">
      <button :disabled="saving" @click="saveConfig">
        {{ saving ? '保存中...' : '保存配置' }}
      </button>
      <span v-if="saved" class="saved-message">✓ 已保存</span>
    </div>
    <div class="settings-help">
      <p><strong>说明</strong></p>
      <ul>
        <li>API Key 保存在本地配置文件，不会上传到任何服务器</li>
        <li>火山引擎 ARK 用户：API Key = 密钥 &lt;访问密钥&gt;</li>
        <li>OpenAI 用户：直接填入你的 OpenAI API Key</li>
      </ul>
    </div>

    <!-- 语音识别 (ASR) 配置 -->
    <h3 style="margin-top: 1.5rem; color: var(--accent-color);">语音识别 (ASR)</h3>
    <div class="setting-item">
      <label>ASR Provider</label>
      <input v-model="speechProvider" placeholder="volcengine" />
    </div>
    <div class="setting-item">
      <label>ASR Endpoint</label>
      <input v-model="speechEndpoint" placeholder="wss://openspeech.bytedance.com/api/v3/sauc/bigmodel_async" />
    </div>
    <div class="setting-item">
      <label>Resource ID</label>
      <input v-model="speechResourceID" placeholder="volc.seedasr.sauc.duration" />
    </div>
    <div class="setting-item">
      <label>ASR API Key</label>
      <input
        v-model="speechApiKey"
        type="password"
        placeholder="火山引擎语音识别 API Key"
        @focus="handleSpeechApiKeyInput"
      />
    </div>
    <div class="setting-actions">
      <button :disabled="speechSaving" @click="saveSpeech">
        {{ speechSaving ? '保存中...' : '保存语音配置' }}
      </button>
      <span v-if="speechSaved" class="saved-message">✓ 已保存</span>
    </div>
    <div class="settings-help">
      <p><strong>语音输入说明</strong></p>
      <ul>
        <li>按住 <kbd>Ctrl + Shift + R</kbd> 说话，松开结束</li>
        <li>需要火山引擎流式语音识别 2.0 的 API Key</li>
        <li>配置保存后需重启应用生效</li>
      </ul>
    </div>

    <!-- 唤醒词检测 (Wake Word) 配置 -->
    <h3 style="margin-top: 1.5rem; color: var(--accent-color);">唤醒词检测 (Wake Word)</h3>
    <div class="setting-item">
      <label>
        <input
          type="checkbox"
          v-model="wakeWordEnabled"
          style="width: auto; margin-right: 0.5rem;"
        />
        启用唤醒词检测
      </label>
      <p class="help-text">启用后，应用会持续在后台监听麦克风，检测到唤醒词后自动启动语音识别</p>
    </div>
    <div class="setting-item">
      <label>唤醒词模型路径 (.onnx)</label>
      <input v-model="wakeWordModelPath" placeholder="%APPDATA%\GameAI\wakeword-models\hey_mycroft_v0.1.onnx" />
    </div>
    <div class="setting-item">
      <label>检测阈值 ({{ wakeWordThreshold.toFixed(2) }})</label>
      <input
        type="range"
        v-model.number="wakeWordThreshold"
        min="0.1"
        max="1.0"
        step="0.05"
      />
      <p class="help-text">越高越不容易误触发，但也可能漏检测。推荐: 0.5</p>
    </div>
    <div class="setting-item">
      <label>重复触发冷却 (毫秒)</label>
      <input
        type="number"
        v-model.number="wakeWordCooldownMs"
        min="0"
        max="5000"
        step="100"
      />
      <p class="help-text">检测后冷却多久才能再次检测。默认: 1000 (1秒)</p>
    </div>
    <div class="setting-item">
      <label>
        <input
          type="checkbox"
          v-model="wakeWordVADEnabled"
          style="width: auto; margin-right: 0.5rem;"
        />
        启用 VAD 静音抑制
      </label>
      <p class="help-text">跳过静音片段，减少误检测。推荐启用</p>
    </div>
    <div class="setting-item">
      <label>自定义 ONNX Runtime 路径 (可选)</label>
      <input v-model="wakeWordOnnxRuntimePath" placeholder="留空自动使用默认" />
      <p class="help-text">如果自动下载失败，可以手动指定 onnxruntime.dll 路径</p>
    </div>
    <div class="setting-actions">
      <button :disabled="wakeWordSaving" @click="saveWakeWord">
        {{ wakeWordSaving ? '保存中...' : '保存唤醒词配置' }}
      </button>
      <span v-if="wakeWordSaved" class="saved-message">✓ 已保存</span>
    </div>

    <!-- Download progress bar -->
    <div v-if="wakeWordDownloading || wakeWordDownloadError" class="download-progress">
      <div v-if="wakeWordDownloading">
        <div class="progress-label">{{ wakeWordDownloadMessage }}</div>
        <div class="progress-bar-container">
          <div class="progress-bar-fill" :style="{ width: wakeWordDownloadPercent + '%' }"></div>
        </div>
        <div class="progress-percent">{{ wakeWordDownloadPercent }}%</div>
      </div>
      <div v-if="wakeWordDownloadError" class="download-error">
        ❌ 下载失败: {{ wakeWordDownloadError }}
      </div>
    </div>

    <div class="settings-help">
      <p><strong>唤醒词说明</strong></p>
      <ul>
        <li>首次启用会自动下载所需模型和 ONNX Runtime</li>
        <li>默认模型: "hey mycroft"，可以重新训练为 "hey gameai"</li>
        <li>持续监听麦克风需要持续占用 CPU，大约 1-5%</li>
        <li>配置保存后需重启应用生效</li>
      </ul>
    </div>

    <h3 style="margin-top: 1.5rem; color: var(--accent-color);">语音模式 (Voice Mode)</h3>
    <div class="setting-item">
      <label>当前模式</label>
      <div class="voice-mode-group">
        <button
          :class="['voice-mode-btn', { active: voiceMode === 'ptt' }]"
          :disabled="voiceModeSaving"
          @click="changeVoiceMode('ptt')"
        >🎤 按键说话</button>
        <button
          :class="['voice-mode-btn', { active: voiceMode === 'wakeword' }]"
          :disabled="voiceModeSaving"
          @click="changeVoiceMode('wakeword')"
        >🎙️ 唤醒模式</button>
        <button
          :class="['voice-mode-btn', { active: voiceMode === 'continuous' }]"
          :disabled="voiceModeSaving"
          @click="changeVoiceMode('continuous')"
        >🔁 连续对话</button>
      </div>
    </div>
    <div v-if="voiceModeSaved" class="save-status">✓ 模式已切换</div>
    <div class="settings-help">
      <p><strong>语音模式说明</strong></p>
      <ul>
        <li><strong>按键说话</strong>：按住 Ctrl+Shift+R 录音；不占用额外 CPU</li>
        <li><strong>唤醒模式</strong>：后台监听唤醒词，唤醒后识别一次；需开启唤醒词检测</li>
        <li><strong>连续对话</strong>：免唤醒循环识别；30s 无输入自动退出</li>
        <li>热键 Ctrl+Alt+V 可循环切换模式</li>
      </ul>
    </div>

    <h3 style="margin-top: 1.5rem; color: var(--accent-color);">语音合成 (TTS · 豆包 2.0)</h3>
    <div class="setting-item">
      <label>Provider</label>
      <input v-model="ttsProvider" placeholder="volcengine" />
    </div>
    <div class="setting-item">
      <label>Resource ID (模型版本)</label>
      <input v-model="ttsResourceId" placeholder="seed-tts-2.0" />
    </div>
    <div class="setting-item">
      <label>Speaker (音色 ID)</label>
      <input v-model="ttsSpeaker" placeholder="zh_female_xiaohe_uranus_bigtts" />
    </div>
    <div class="setting-item">
      <label>音频格式</label>
      <select v-model="ttsFormat">
        <option value="mp3">mp3</option>
        <option value="pcm">pcm</option>
        <option value="ogg_opus">ogg_opus</option>
        <option value="wav">wav</option>
      </select>
    </div>
    <div class="setting-item">
      <label>采样率 (Hz)</label>
      <select v-model.number="ttsSampleRate">
        <option :value="8000">8000</option>
        <option :value="16000">16000</option>
        <option :value="22050">22050</option>
        <option :value="24000">24000</option>
        <option :value="32000">32000</option>
        <option :value="44100">44100</option>
        <option :value="48000">48000</option>
      </select>
    </div>
    <div class="setting-item">
      <label>语速 (Speech Rate, -50~100)</label>
      <input v-model.number="ttsSpeechRate" type="number" min="-50" max="100" />
    </div>
    <div class="setting-item">
      <label>音量 (Loudness Rate, -50~100)</label>
      <input v-model.number="ttsLoudnessRate" type="number" min="-50" max="100" />
    </div>
    <div class="setting-item">
      <label>音调 (Pitch, -12~12)</label>
      <input v-model.number="ttsPitch" type="number" min="-12" max="12" />
    </div>
    <div class="setting-item">
      <label>API Key (X-Api-Key)</label>
      <input v-model="ttsApiKey" type="password" :placeholder="ttsHasApiKey ? '已配置' : '输入 API Key'" />
    </div>
    <div class="button-row">
      <button class="primary-btn" :disabled="ttsSaving" @click="saveTTS">
        {{ ttsSaving ? '保存中...' : '保存 TTS 配置' }}
      </button>
      <button class="secondary-btn" :disabled="ttsTesting" @click="handleTestTTS">
        {{ ttsTesting ? '播放中...' : '🔊 测试播放' }}
      </button>
    </div>
    <div v-if="ttsSaved" class="save-status">✓ TTS 配置已保存</div>
    <div class="setting-item">
      <label>测试文本</label>
      <input v-model="ttsTestText" placeholder="输入要朗读的文本" />
    </div>
    <div class="settings-help">
      <p><strong>TTS 说明（豆包语音合成 2.0）</strong></p>
      <ul>
        <li>使用火山引擎双向流式 WebSocket 协议（V3）</li>
        <li>API Key 来自火山控制台 → 语音技术 → API Key 管理</li>
        <li>默认音色 zh_female_xiaohe_uranus_bigtts（小何 · 普通话，需与 seed-tts-2.0 资源匹配）</li>
        <li>完整音色列表：<a href="https://www.volcengine.com/docs/6561/1257544" target="_blank" style="color: var(--accent-color);">豆包 2.0 音色列表</a></li>
        <li>TTS 播放时抑制 ASR，避免自激</li>
        <li>语音模式下，Agent 回复会自动朗读</li>
      </ul>
    </div>
  </div>
</template>

<style scoped>
.settings-panel {
  background-color: var(--bg-secondary);
  border-radius: 8px;
  padding: 1.5rem;
  border: 1px solid var(--border-color);
  margin-top: 1rem;
}

.settings-panel h2 {
  margin-top: 0;
  margin-bottom: 1rem;
  font-size: 1.25rem;
  color: var(--accent-color);
}

.setting-item {
  margin-bottom: 1rem;
}

.setting-item label {
  display: block;
  margin-bottom: 0.5rem;
  font-weight: 500;
  color: var(--text-secondary);
  font-size: 0.9rem;
}

.setting-item input {
  width: 100%;
  padding: 0.5rem 0.75rem;
  border: 1px solid var(--border-color);
  border-radius: 4px;
  background-color: var(--bg-primary);
  color: var(--text-primary);
  font-size: 0.9rem;
}

.setting-item input:focus {
  outline: none;
  border-color: var(--accent-color);
}

.setting-actions {
  display: flex;
  align-items: center;
  gap: 1rem;
  margin-bottom: 1rem;
}

.setting-actions button {
  padding: 0.5rem 1.5rem;
  background: var(--accent-color);
  color: white;
  border: none;
  border-radius: 4px;
  cursor: pointer;
  font-size: 0.9rem;
}

.setting-actions button:disabled {
  opacity: 0.6;
  cursor: wait;
}

.saved-message {
  color: #22c55e;
  font-size: 0.9rem;
}

.settings-help {
  margin-top: 1rem;
  padding-top: 1rem;
  border-top: 1px solid var(--border-color);
  font-size: 0.85rem;
  color: var(--text-secondary);
}

.settings-help p {
  margin-top: 0;
}

.settings-help ul {
  margin: 0.5rem 0 0 0;
  padding-left: 1.25rem;
}

.settings-help li {
  margin-bottom: 0.25rem;
}

.help-text {
  font-size: 0.8rem;
  color: var(--text-secondary);
  margin-top: 0.25rem;
  margin-bottom: 0;
}

.download-progress {
  margin-top: 1rem;
  padding: 1rem;
  background-color: var(--bg-primary);
  border-radius: 6px;
  border: 1px solid var(--border-color);
}

.progress-label {
  font-size: 0.9rem;
  color: var(--text-primary);
  margin-bottom: 0.5rem;
}

.progress-bar-container {
  width: 100%;
  height: 12px;
  background-color: var(--bg-secondary);
  border-radius: 6px;
  overflow: hidden;
  border: 1px solid var(--border-color);
}

.progress-bar-fill {
  height: 100%;
  background: linear-gradient(90deg, var(--accent-color), #60a5fa);
  border-radius: 6px;
  transition: width 0.2s ease-out;
}

.progress-percent {
  font-size: 0.8rem;
  color: var(--text-secondary);
  margin-top: 0.25rem;
  text-align: right;
}

.download-error {
  color: #ef4444;
  font-size: 0.9rem;
  padding: 0.5rem;
  background-color: rgba(239, 68, 68, 0.1);
  border-radius: 4px;
}

.voice-mode-group {
  display: flex;
  gap: 8px;
  flex-wrap: wrap;
}

.voice-mode-btn {
  flex: 1;
  min-width: 110px;
  padding: 8px 12px;
  background: rgba(60, 60, 70, 0.5);
  color: #ccc;
  border: 1px solid var(--border-color, #444);
  border-radius: 6px;
  cursor: pointer;
  font-size: 0.85rem;
  transition: all 0.15s ease;
}

.voice-mode-btn:hover:not(:disabled) {
  background: rgba(80, 80, 100, 0.7);
  border-color: #888;
}

.voice-mode-btn.active {
  background: rgba(96, 165, 250, 0.2);
  border-color: #60a5fa;
  color: #60a5fa;
  font-weight: 500;
}

.voice-mode-btn:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.save-status {
  color: #22c55e;
  font-size: 0.85rem;
  margin-top: 4px;
  margin-bottom: 8px;
}

.button-row {
  display: flex;
  gap: 8px;
  margin-top: 8px;
}

.primary-btn,
.secondary-btn {
  padding: 8px 16px;
  border-radius: 6px;
  cursor: pointer;
  font-size: 0.85rem;
  border: 1px solid transparent;
}

.primary-btn {
  background: var(--accent-color, #60a5fa);
  color: white;
  border-color: var(--accent-color, #60a5fa);
}

.primary-btn:hover:not(:disabled) {
  filter: brightness(1.1);
}

.secondary-btn {
  background: rgba(34, 211, 238, 0.15);
  color: #22d3ee;
  border-color: rgba(34, 211, 238, 0.4);
}

.secondary-btn:hover:not(:disabled) {
  background: rgba(34, 211, 238, 0.25);
}

.primary-btn:disabled,
.secondary-btn:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}
</style>
