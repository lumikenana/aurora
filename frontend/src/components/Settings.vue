<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { getLLMConfig, saveLLMConfig, getSpeechConfig, saveSpeechConfig } from '../api/backend'

const provider = ref('openai')
const model = ref('doubao-seed-2.0-lite')
const baseURL = ref('https://ark.cn-beijing.volces.com/api/plan/v3')
const apiKey = ref('')
const hasApiKey = ref(false)
const saving = ref(false)
const saved = ref(false)

// Speech / ASR config
const speechProvider = ref('volcengine')
const speechEndpoint = ref('wss://openspeech.bytedance.com/api/v3/sauc/bigmodel')
const speechResourceID = ref('volc.bigasr.sauc.duration')
const speechApiKey = ref('')
const speechHasApiKey = ref(false)
const speechSaving = ref(false)
const speechSaved = ref(false)

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
    speechEndpoint.value = config.endpoint || 'wss://openspeech.bytedance.com/api/v3/sauc/bigmodel'
    speechResourceID.value = config.resource_id || 'volc.bigasr.sauc.duration'
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

onMounted(() => {
  loadConfig()
  loadSpeechConfig()
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
      <input v-model="speechEndpoint" placeholder="wss://openspeech.bytedance.com/api/v3/sauc/bigmodel" />
    </div>
    <div class="setting-item">
      <label>Resource ID</label>
      <input v-model="speechResourceID" placeholder="volc.bigasr.sauc.duration" />
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
</style>
