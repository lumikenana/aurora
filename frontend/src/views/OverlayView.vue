<script setup lang="ts">
// OverlayView - Overlay 控制中心（UI 重构）
//
// 职责：
//   - Overlay 开关 + 实时 Show/Hide（与托盘同一状态口）
//   - 锁定态切换（HUD ↔ Edit Mode，与快捷键/Overlay 内 Lock 按钮同一状态口）
//   - 外观配置（opacity / font size / 过滤开关，SaveOverlayConfig 热重载）
//   - 几何管理（Anchor / 自定义位置信息 / Reset）
//   - 样式 Preview
//
// 不负责：Edit Mode 的拖拽/缩放交互（在 native Overlay 窗口内完成）。

import { ref, computed, watch, onMounted } from 'vue'
import type { OverlayConfig, ChatMessage } from '../api/backend'
import Overlay from '../components/Overlay.vue'
import {
  getOverlayLocked,
  setOverlayLocked,
  getOverlayGeometry,
  resetOverlayGeometry,
  type OverlayGeometry,
} from '../api/backend'
import { EventsOn } from '../api/events'
import { useToast } from '../composables/useToast'

const toast = useToast()

const props = defineProps<{
  config: OverlayConfig
  visible: boolean
  messages: ChatMessage[]
}>()

const emit = defineEmits<{
  (e: 'save', cfg: OverlayConfig): void
  (e: 'toggle-visible', visible: boolean): void
}>()

// 草稿：编辑但未保存的状态
const draft = ref<OverlayConfig>({ ...props.config })
const saving = ref(false)
const saved = ref(false)

watch(
  () => props.config,
  (next) => {
    draft.value = { ...next }
  },
  { deep: true },
)

const isDirty = computed(() => {
  return JSON.stringify(draft.value) !== JSON.stringify(props.config)
})

async function save() {
  saving.value = true
  saved.value = false
  try {
    emit('save', { ...draft.value })
    setTimeout(() => {
      saving.value = false
      saved.value = true
      setTimeout(() => { saved.value = false }, 2000)
    }, 300)
  } catch (err) {
    console.error('[OverlayView.save] failed:', err)
    saving.value = false
  }
}

function toggleVisible() {
  emit('toggle-visible', !props.visible)
}

// ---- 锁定态 ----
const locked = ref(true)

async function toggleLock() {
  const next = !locked.value
  try {
    await setOverlayLocked(next)
  } catch (err) {
    console.error('[OverlayView] setOverlayLocked failed:', err)
    toast.error('Failed to change lock state')
  }
}

// 后端是单一真相：订阅事件（快捷键 / Overlay 内 Lock 按钮 / 主窗口切换）
function onLockChanged(v: unknown) {
  locked.value = !!v
}

// ---- 几何 ----
const geometry = ref<OverlayGeometry | null>(null)

async function loadGeometry() {
  try {
    geometry.value = await getOverlayGeometry()
  } catch (err) {
    console.error('[OverlayView] load geometry failed:', err)
  }
}

async function onResetGeometry() {
  try {
    await resetOverlayGeometry()
    toast.success('Overlay position reset to anchor')
  } catch (err) {
    console.error('[OverlayView] reset geometry failed:', err)
    toast.error('Failed to reset overlay position')
  }
}

onMounted(() => {
  loadGeometry()
  // 初始锁定态 + 订阅事件（后端是单一真相：快捷键 / Overlay 内 Lock 按钮都会广播）
  getOverlayLocked()
    .then((v) => { locked.value = v })
    .catch(() => { /* ignore */ })
  EventsOn('overlayLockChanged', onLockChanged)
  // 拖拽/缩放提交后几何信息变化 → 刷新展示
  EventsOn('overlayConfigChanged', loadGeometry)
})

// Preview 使用只读 mock（不与实际 Overlay HUD 冲突）
const previewMessages = computed<ChatMessage[]>(() => {
  if (props.messages.length > 0) {
    return props.messages.slice(-3)
  }
  // 没消息时给一个 mock 预览，让用户看到 HUD 样式
  return [
    { role: 'user', content: 'Should I level to 8 now?' },
    { role: 'assistant', content: 'Based on your economy (42g) and 4-cost shop, you can safely level. Keepstreak bonus active.' },
  ]
})
</script>

<template>
  <div class="overlay-view">
    <header class="view-header">
      <h1>Game Overlay</h1>
      <p class="view-subtitle">
        Low-distraction AI companion shown over your game
      </p>
    </header>

    <div class="overlay-content">
      <section class="settings-section">
        <div class="setting-row">
          <div class="setting-label">
            <span class="setting-name">Enable Overlay</span>
            <span class="setting-desc">Show Aurora's conversation as a HUD over your game</span>
          </div>
          <label class="toggle">
            <input type="checkbox" v-model="draft.enabled" />
            <span class="toggle-slider"></span>
          </label>
        </div>

        <div class="setting-row" :class="{ disabled: !draft.enabled }">
          <div class="setting-label">
            <span class="setting-name">Show Overlay Now</span>
            <span class="setting-desc">Toggle visibility without changing saved settings</span>
          </div>
          <button class="btn" :disabled="!draft.enabled" @click="toggleVisible">
            {{ visible ? 'Hide' : 'Show' }}
          </button>
        </div>

        <div class="setting-row" :class="{ disabled: !draft.enabled }">
          <div class="setting-label">
            <span class="setting-name">Lock Overlay</span>
            <span class="setting-desc">
              HUD mode: pure display. Unlock to drag / resize the overlay window.
            </span>
          </div>
          <button class="btn" :disabled="!draft.enabled" @click="toggleLock">
            {{ locked ? 'Unlock (Edit Mode)' : 'Lock (HUD)' }}
          </button>
        </div>

        <div class="setting-row">
          <div class="setting-label">
            <span class="setting-name">Opacity</span>
            <span class="setting-desc">Lower = less intrusive, but harder to read</span>
          </div>
          <div class="setting-control">
            <input type="range" v-model.number="draft.opacity" min="0.30" max="1.00" step="0.05" />
            <span class="setting-value">{{ draft.opacity.toFixed(2) }}</span>
          </div>
        </div>

        <div class="setting-row">
          <div class="setting-label">
            <span class="setting-name">Font Size</span>
            <span class="setting-desc">Adjust readability for your screen</span>
          </div>
          <div class="setting-control">
            <div class="segment">
              <button
                v-for="opt in ['small', 'medium', 'large'] as const"
                :key="opt"
                :class="['segment-btn', { active: draft.font_size === opt }]"
                @click="draft.font_size = opt"
              >{{ opt }}</button>
            </div>
          </div>
        </div>

        <div class="setting-row">
          <div class="setting-label">
            <span class="setting-name">Anchor</span>
            <span class="setting-desc">
              Where the overlay sits when no custom position is set
            </span>
          </div>
          <div class="setting-control">
            <div class="anchor-grid">
              <button
                v-for="opt in ['top-left', 'top-right', 'bottom-left', 'bottom-right'] as const"
                :key="opt"
                :class="['anchor-btn', `anchor-${opt}`, { active: draft.anchor === opt }]"
                @click="draft.anchor = opt"
                :title="opt"
              ></button>
            </div>
          </div>
        </div>

        <div class="setting-row" :class="{ disabled: !geometry?.custom_geom }">
          <div class="setting-label">
            <span class="setting-name">Custom Position</span>
            <span class="setting-desc">
              {{ geometry?.custom_geom
                ? `x=${geometry.config_x} y=${geometry.config_y} · ${geometry.config_w}×${geometry.config_h}`
                : 'Not set — drag the overlay header in Edit Mode to set a custom position' }}
            </span>
          </div>
          <button class="btn" :disabled="!geometry?.custom_geom" @click="onResetGeometry">
            Reset
          </button>
        </div>

        <div class="setting-row">
          <div class="setting-label">
            <span class="setting-name">Auto Scroll</span>
            <span class="setting-desc">Scroll to newest message automatically</span>
          </div>
          <label class="toggle">
            <input type="checkbox" v-model="draft.auto_scroll" />
            <span class="toggle-slider"></span>
          </label>
        </div>

        <div class="setting-row">
          <div class="setting-label">
            <span class="setting-name">Show Your Messages</span>
            <span class="setting-desc">Display user messages in the overlay</span>
          </div>
          <label class="toggle">
            <input type="checkbox" v-model="draft.show_user_messages" />
            <span class="toggle-slider"></span>
          </label>
        </div>

        <div class="setting-row">
          <div class="setting-label">
            <span class="setting-name">Show Aurora's Replies</span>
            <span class="setting-desc">Display assistant messages in the overlay</span>
          </div>
          <label class="toggle">
            <input type="checkbox" v-model="draft.show_assistant_messages" />
            <span class="toggle-slider"></span>
          </label>
        </div>

        <div class="setting-actions-row">
          <button class="btn btn-primary" :disabled="saving || !isDirty" @click="save">
            {{ saving ? 'Saving…' : 'Save Overlay Settings' }}
          </button>
          <span v-if="geometry?.custom_geom" class="saved-mark">position customized</span>
        </div>
      </section>

      <section class="preview-section">
        <div class="preview-header">
          <h2>Preview</h2>
          <span class="preview-hint">This is a visual preview, not the live HUD.</span>
        </div>
        <div class="preview-stage" :class="`stage-${draft.anchor}`">
          <div class="preview-frame">
            <Overlay
              v-if="draft.enabled"
              :messages="previewMessages"
              :config="draft"
              @close.prevent
            />
            <div v-else class="preview-disabled">
              <p>Overlay disabled</p>
              <p class="muted">Enable the toggle above to see the preview.</p>
            </div>
          </div>
        </div>
      </section>
    </div>
  </div>
</template>

<style scoped>
.overlay-view {
  display: flex;
  flex-direction: column;
  height: 100%;
  min-height: 0;
}

.view-header {
  padding: var(--space-5) var(--space-7) var(--space-3);
  border-bottom: 1px solid var(--border-color);
  flex-shrink: 0;
}

.view-header h1 {
  font-size: 20px;
  font-weight: 600;
  color: var(--text-primary);
}

.view-subtitle {
  font-size: 12px;
  color: var(--text-secondary);
  margin-top: 2px;
}

.overlay-content {
  flex: 1;
  display: grid;
  grid-template-columns: minmax(0, 1fr) minmax(0, 1fr);
  gap: var(--space-5);
  padding: var(--space-5) var(--space-7);
  overflow: auto;
  min-height: 0;
}

@media (max-width: 1100px) {
  .overlay-content {
    grid-template-columns: 1fr;
  }
}

.settings-section {
  display: flex;
  flex-direction: column;
  background: var(--bg-secondary);
  border: 1px solid var(--border-color);
  border-radius: var(--radius-lg);
  padding: var(--space-1) 0;
  align-self: start;
}

.setting-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: var(--space-3) var(--space-4);
  gap: var(--space-4);
}

.setting-row.disabled {
  opacity: 0.5;
}

.setting-label {
  flex: 1;
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.setting-name {
  font-size: 13px;
  font-weight: 500;
  color: var(--text-primary);
}

.setting-desc {
  font-size: 11px;
  color: var(--text-muted);
  line-height: 1.4;
}

.setting-control {
  display: flex;
  align-items: center;
  gap: var(--space-2);
}

.setting-value {
  font-size: 12px;
  font-family: ui-monospace, monospace;
  color: var(--text-muted);
  min-width: 36px;
  text-align: right;
}

input[type='range'] {
  width: 140px;
  accent-color: var(--accent-color);
}

.anchor-grid {
  display: grid;
  grid-template-columns: repeat(2, 18px);
  grid-template-rows: repeat(2, 18px);
  gap: var(--space-1);
  padding: var(--space-1);
  background: var(--bg-primary);
  border: 1px solid var(--border-color);
  border-radius: var(--radius-md);
}

.anchor-btn {
  background: transparent;
  border: 0;
  border-radius: var(--radius-sm);
  cursor: pointer;
  transition: background var(--transition-fast);
  position: relative;
}

.anchor-btn::after {
  content: '';
  position: absolute;
  width: 10px;
  height: 10px;
  background: var(--text-muted);
  border-radius: 2px;
  top: 50%;
  left: 50%;
  transform: translate(-50%, -50%);
  opacity: 0.4;
}

.anchor-btn.active {
  background: var(--accent-color);
}

.anchor-btn.active::after {
  background: var(--text-on-accent);
  opacity: 1;
}

.setting-actions-row {
  display: flex;
  align-items: center;
  gap: var(--space-3);
  padding: var(--space-3) var(--space-4) var(--space-4);
  border-top: 1px solid var(--border-color);
  margin-top: var(--space-1);
}

.saved-mark {
  color: var(--status-info);
  font-size: 11px;
}

.preview-section {
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
  min-height: 0;
}

.preview-header {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  padding: 0 var(--space-1);
}

.preview-header h2 {
  font-size: 13px;
  font-weight: 600;
  color: var(--text-primary);
  text-transform: uppercase;
  letter-spacing: 0.05em;
}

.preview-hint {
  font-size: 11px;
  color: var(--text-muted);
}

.preview-stage {
  position: relative;
  flex: 1;
  min-height: 320px;
  background:
    linear-gradient(135deg, rgba(60, 70, 90, 0.3) 0%, rgba(30, 35, 50, 0.4) 100%),
    repeating-linear-gradient(
      45deg,
      transparent,
      transparent 12px,
      var(--surface-inset) 12px,
      var(--surface-inset) 24px
    );
  border: 1px solid var(--border-color);
  border-radius: var(--radius-lg);
  overflow: hidden;
}

.preview-frame {
  position: absolute;
  inset: 0;
  pointer-events: none;
}

.preview-disabled {
  position: absolute;
  inset: 0;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  color: var(--text-muted);
  font-size: 13px;
  gap: var(--space-1);
}

.preview-disabled .muted {
  font-size: 11px;
  opacity: 0.7;
}
</style>
