<script setup lang="ts">
import { ref, watch, nextTick } from 'vue'
import type { ChatMessage } from '../api/backend'

const props = defineProps<{
  messages: ChatMessage[]
}>()

const container = ref<HTMLElement | null>(null)

// 新消息时滚动到底部
watch(
  () => props.messages.map(m => m.content).join(''),
  async () => {
    await nextTick()
    if (container.value) {
      container.value.scrollTop = container.value.scrollHeight
    }
  },
)
</script>

<template>
  <div class="conversation-container" ref="container">
    <div
      v-for="(msg, i) in messages"
      :key="i"
      class="message"
      :class="msg.role"
    >
      <div class="message-header">{{ msg.role === 'user' ? '你' : 'AI' }}</div>
      <div class="message-content">{{ msg.content }}</div>
    </div>
  </div>
</template>

<style scoped>
.conversation-container {
  margin-top: 2rem;
  background-color: var(--bg-secondary);
  border-radius: 8px;
  padding: 1rem;
  border: 1px solid var(--border-color);
  max-height: 400px;
  overflow-y: auto;
}

.message {
  margin-bottom: 1rem;
  padding: 0.75rem;
  border-radius: 6px;
}

.message.user {
  background-color: var(--bg-primary);
  border-left: 3px solid var(--accent-color);
}

.message.assistant {
  background-color: var(--bg-primary);
  border-left: 3px solid #2ecc71;
}

.message-header {
  font-size: 0.8rem;
  color: var(--text-secondary);
  margin-bottom: 0.25rem;
}

.message-content {
  font-size: 0.95rem;
  line-height: 1.5;
  white-space: pre-wrap;
  word-wrap: break-word;
}
</style>
