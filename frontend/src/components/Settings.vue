<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { getLLMConfig, saveLLMConfig, getSpeechConfig, saveSpeechConfig, getWakeWordConfig, saveWakeWordConfig } from '../api/backend'

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
})
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
    <div class="settings-help">
      <p><strong>唤醒词说明</strong></p>
      <ul>
        <li>首次启用会自动下载所需模型和 ONNX Runtime</li>
        <li>默认模型: "hey mycroft"，可以重新训练为 "hey gameai"</li>
        <li>持续监听麦克风需要持续占用 CPU，大约 1-5%</li>
        <li>配置保存后需重启应用生效</li>
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
</style>
