<script setup lang="ts">
import { ref } from 'vue'
import { EventsOn } from '../api/events'

const recording = ref(false)
const partialText = ref('')
const finalText = ref('')
const errorMsg = ref('')

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
</script>

<template>
  <div v-if="recording || finalText || errorMsg" class="voice-indicator">
    <div v-if="errorMsg" class="voice-error">
      ⚠️ {{ errorMsg }}
    </div>
    <div v-if="recording" class="recording-state">
      <span class="pulse-dot"></span>
      <span class="recording-text">录音中... 松开 Ctrl+Shift+R 结束</span>
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
  min-width: 200px;
  max-width: 500px;
  text-align: center;
  backdrop-filter: blur(8px);
  box-shadow: 0 4px 16px rgba(0, 0, 0, 0.3);
}

.recording-state {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 8px;
  margin-bottom: 4px;
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

.partial-text {
  color: var(--text-secondary, #aaa);
  font-size: 0.85rem;
  margin-top: 4px;
  font-style: italic;
}

.final-text {
  color: #22c55e;
  font-size: 0.9rem;
  margin-top: 4px;
}

.voice-error {
  color: #f59e0b;
  font-size: 0.85rem;
}
</style>
