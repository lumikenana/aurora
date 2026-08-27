<script setup lang="ts">
import { ref, computed } from 'vue'
import { EventsOn } from '../api/events'
import type { VoiceMode } from '../api/backend'

const recording = ref(false)
const partialText = ref('')
const finalText = ref('')
const errorMsg = ref('')

const currentMode = ref<VoiceMode>('wakeword')
const currentState = ref<string>('idle')
const ttsPlaying = ref(false)
const agentBusy = ref(false)
const timeoutMsg = ref('')

const modeLabel = computed(() => {
  switch (currentMode.value) {
    case 'ptt': return '按键说话'
    case 'wakeword': return '唤醒模式'
    case 'continuous': return '连续对话'
    default: return currentMode.value
  }
})

const stateLabel = computed(() => {
  switch (currentState.value) {
    case 'idle': return '待命'
    case 'wake_detected': return '唤醒'
    case 'speech_listening': return '识别中'
    case 'processing': return 'Agent 处理中'
    case 'speaking': return '播放中'
    default: return currentState.value
  }
})

const modeClass = computed(() => `mode-${currentMode.value}`)
const stateClass = computed(() => `state-${currentState.value}`)

EventsOn('pttStateChange', (data: { recording: boolean }) => {
  recording.value = data.recording
  if (!data.recording) {
    partialText.value = ''
  }
})

EventsOn('voiceTranscript', (data: { text: string; is_final: boolean }) => {
  if (data.is_final) {
    finalText.value = data.text
    partialText.value = ''
    setTimeout(() => { finalText.value = '' }, 3000)
  } else {
    partialText.value = data.text
  }
})

EventsOn('pttError', (data: { message: string }) => {
  errorMsg.value = data.message
  recording.value = false
  setTimeout(() => { errorMsg.value = '' }, 5000)
})

EventsOn('voiceModeChange', (data: { mode: VoiceMode; mode_name: string }) => {
  currentMode.value = data.mode
})

EventsOn('voiceStateChange', (data: { state: string; code: number }) => {
  currentState.value = data.state
  if (data.state === 'idle') {
    agentBusy.value = false
  } else if (data.state === 'processing') {
    agentBusy.value = true
  } else if (data.state === 'speaking') {
    ttsPlaying.value = true
  }
})

EventsOn('wakeWordDetected', () => {
  // 唤醒触发时不需要改变状态机（由 state change 事件驱动）
})

EventsOn('ttsStart', () => {
  ttsPlaying.value = true
})

EventsOn('ttsEnd', () => {
  ttsPlaying.value = false
})

EventsOn('voiceContinuousTimeout', (data: { timeout_seconds: number }) => {
  timeoutMsg.value = `连续对话已自动退出（${data.timeout_seconds}s 无输入）`
  setTimeout(() => { timeoutMsg.value = '' }, 4000)
})
</script>

<template>
  <div v-if="recording || finalText || errorMsg || ttsPlaying || agentBusy || timeoutMsg || currentMode"
       class="voice-indicator">
    <!-- 模式徽章 -->
    <div class="mode-badge" :class="modeClass">
      <span class="mode-icon">
        <template v-if="currentMode === 'wakeword'">🎙️</template>
        <template v-else-if="currentMode === 'continuous'">🔁</template>
        <template v-else>🎤</template>
      </span>
      <span>{{ modeLabel }}</span>
    </div>

    <!-- 状态徽章 -->
    <div class="state-badge" :class="stateClass">
      <span class="state-dot"></span>
      <span>{{ stateLabel }}</span>
    </div>

    <div v-if="errorMsg" class="voice-error">
      ⚠️ {{ errorMsg }}
    </div>

    <div v-if="timeoutMsg" class="voice-timeout">
      ⏰ {{ timeoutMsg }}
    </div>

    <div v-if="recording" class="recording-state">
      <span class="pulse-dot"></span>
      <span class="recording-text">录音中... 松开 Ctrl+Shift+R 结束</span>
    </div>

    <div v-if="ttsPlaying && !recording" class="tts-state">
      <span class="tts-wave"></span>
      <span>🔊 正在播报回复...</span>
    </div>

    <div v-if="agentBusy && !recording && !ttsPlaying" class="busy-state">
      <span class="busy-spinner"></span>
      <span>Agent 思考中...</span>
    </div>

    <div v-if="partialText" class="partial-text">
      {{ partialText }}
    </div>

    <div v-if="finalText" class="final-text">
      ✓ {{ finalText }}
    </div>
  </div>
</template>

<style scoped>
.voice-indicator {
  position: fixed;
  bottom: 20px;
  left: 50%;
  transform: translateX(-50%);
  z-index: 1000;
  background: rgba(30, 30, 30, 0.95);
  border: 1px solid var(--border-color, #444);
  border-radius: 12px;
  padding: 12px 20px;
  min-width: 260px;
  max-width: 560px;
  text-align: center;
  backdrop-filter: blur(8px);
  box-shadow: 0 4px 16px rgba(0, 0, 0, 0.3);
  display: flex;
  flex-direction: column;
  gap: 6px;
  align-items: center;
}

.mode-badge,
.state-badge {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 3px 10px;
  border-radius: 999px;
  font-size: 0.78rem;
  font-weight: 500;
  border: 1px solid transparent;
}

.mode-badge {
  background: rgba(96, 165, 250, 0.15);
  color: #60a5fa;
  border-color: rgba(96, 165, 250, 0.35);
}
.mode-badge.mode-ptt {
  background: rgba(148, 163, 184, 0.15);
  color: #94a3b8;
  border-color: rgba(148, 163, 184, 0.35);
}
.mode-badge.mode-continuous {
  background: rgba(251, 146, 60, 0.15);
  color: #fb923c;
  border-color: rgba(251, 146, 60, 0.35);
}

.state-badge {
  background: rgba(34, 197, 94, 0.15);
  color: #22c55e;
  border-color: rgba(34, 197, 94, 0.35);
}
.state-badge.state-processing {
  background: rgba(168, 85, 247, 0.15);
  color: #a855f7;
  border-color: rgba(168, 85, 247, 0.35);
}
.state-badge.state-speaking {
  background: rgba(34, 211, 238, 0.15);
  color: #22d3ee;
  border-color: rgba(34, 211, 238, 0.35);
}
.state-badge.state-wake_detected {
  background: rgba(250, 204, 21, 0.15);
  color: #facc15;
  border-color: rgba(250, 204, 21, 0.35);
}

.state-dot {
  width: 6px;
  height: 6px;
  background: currentColor;
  border-radius: 50%;
}

.recording-state {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 8px;
  margin-top: 2px;
}

.pulse-dot {
  width: 10px;
  height: 10px;
  background: #ef4444;
  border-radius: 50%;
  animation: pulse 1s ease-in-out infinite;
}

@keyframes pulse {
  0%, 100% { opacity: 1; transform: scale(1); }
  50% { opacity: 0.5; transform: scale(0.8); }
}

.recording-text {
  color: #ef4444;
  font-size: 0.9rem;
  font-weight: 500;
}

.tts-state,
.busy-state {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 8px;
  color: #22d3ee;
  font-size: 0.85rem;
}

.busy-state {
  color: #a855f7;
}

.busy-spinner {
  width: 10px;
  height: 10px;
  border: 2px solid rgba(168, 85, 247, 0.3);
  border-top-color: #a855f7;
  border-radius: 50%;
  animation: spin 0.8s linear infinite;
}

@keyframes spin {
  to { transform: rotate(360deg); }
}

.partial-text {
  color: var(--text-secondary, #aaa);
  font-size: 0.85rem;
  font-style: italic;
}

.final-text {
  color: #22c55e;
  font-size: 0.9rem;
}

.voice-error {
  color: #f59e0b;
  font-size: 0.85rem;
}

.voice-timeout {
  color: #fb923c;
  font-size: 0.85rem;
}
</style>