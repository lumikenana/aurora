<script setup lang="ts">
// ShortcutsView - 快捷键管理（UI 重构新增页面）
//
// 职责：
//   - 展示全部 9 个快捷键 action（当前组合键 + 说明）
//   - 捕获式修改：点击 → 按下新组合键（Escape 取消）
//   - 冲突 / 非法组合反馈（后端 ShortcutConflictError / parseCombo 校验）
//   - 恢复默认（确认后 ResetShortcuts）
//
// 不负责：快捷键的实际注册（后端 ShortcutManager 运行时生效 + 持久化）。

import { ref, onMounted, onUnmounted } from 'vue'
import {
  getShortcuts,
  setShortcut,
  resetShortcuts,
  type ShortcutAction,
} from '../api/backend'
import { useToast } from '../composables/useToast'

const toast = useToast()

interface ShortcutRow {
  action: ShortcutAction
  label: string
  desc: string
}

const rows: ShortcutRow[] = [
  { action: 'quick_input', label: 'Quick Input', desc: 'Open the quick ask window' },
  { action: 'ptt', label: 'Push to Talk', desc: 'Hold to talk with voice' },
  { action: 'cycle_voice_mode', label: 'Cycle Voice Mode', desc: 'PTT → WakeWord' },
  { action: 'toggle_main_window', label: 'Show / Hide Main Window', desc: 'Toggle the Aurora main window' },
  { action: 'toggle_overlay', label: 'Show / Hide Overlay', desc: 'Toggle the in-game overlay' },
  { action: 'toggle_overlay_lock', label: 'Lock / Unlock Overlay', desc: 'Switch between HUD and Edit Mode' },
  { action: 'emergency_hide', label: 'Emergency Hide', desc: 'Hide all Aurora windows at once (services keep running)' },
  { action: 'analyze_screen', label: 'Analyze Screen', desc: 'Ask Aurora about the current game screen' },
  { action: 'stop_speaking', label: 'Stop Speaking', desc: 'Immediately stop TTS playback' },
]

const combos = ref<Record<string, string>>({})
const loading = ref(true)

// ---- 捕获状态 ----
const capturing = ref<ShortcutAction | null>(null)
const captureKeys = ref<string[]>([])
const captureError = ref('')

const isMac = navigator.platform.toUpperCase().includes('MAC')

// Windows 优先展示（目标平台）；Mac 上显示 Cmd 替代提示
const MOD_LABELS: Record<string, string> = isMac
  ? { Ctrl: '⌃', Shift: '⇧', Alt: '⌥', Win: '⌘' }
  : { Ctrl: 'Ctrl', Shift: 'Shift', Alt: 'Alt', Win: 'Win' }

function fmtCombo(combo: string): string[] {
  return combo.split('+').map((k) => MOD_LABELS[k] ?? k)
}

async function load() {
  loading.value = true
  try {
    combos.value = await getShortcuts()
  } catch (err) {
    console.error('[shortcuts] load failed:', err)
    toast.error('Failed to load shortcuts')
  } finally {
    loading.value = false
  }
}

function startCapture(action: ShortcutAction) {
  if (capturing.value === action) return
  capturing.value = action
  captureKeys.value = []
  captureError.value = ''
}

function cancelCapture() {
  capturing.value = null
  captureKeys.value = []
  captureError.value = ''
}

const MODIFIER_KEYS = new Set([
  'Control', 'Shift', 'Alt', 'Meta',
  'ControlLeft', 'ControlRight', 'ShiftLeft', 'ShiftRight',
  'AltLeft', 'AltRight', 'MetaLeft', 'MetaRight',
])

function keyDisplayName(e: KeyboardEvent): string {
  if (/^F\d{1,2}$/.test(e.key)) return e.key
  if (e.code.startsWith('Key')) return e.code.slice(3)
  if (e.code.startsWith('Digit')) return e.code.slice(5)
  const named: Record<string, string> = {
    Space: 'Space', Home: 'Home', End: 'End', PageUp: 'PgUp', PageDown: 'PgDn',
    Insert: 'Ins', Delete: 'Del', ArrowUp: 'Up', ArrowDown: 'Down',
    ArrowLeft: 'Left', ArrowRight: 'Right',
  }
  return named[e.code] ?? ''
}

function onCaptureKeydown(e: KeyboardEvent) {
  // 仅在捕获态拦截按键（否则会吞掉全页面键盘事件）
  if (!capturing.value) return
  e.preventDefault()
  e.stopPropagation()

  if (e.key === 'Escape') {
    cancelCapture()
    return
  }

  const key = keyDisplayName(e)
  if (!key) return

  captureKeys.value = [
    e.ctrlKey ? 'Ctrl' : null,
    e.shiftKey ? 'Shift' : null,
    e.altKey ? 'Alt' : null,
    e.metaKey ? 'Win' : null,
    key,
  ].filter(Boolean) as string[]

  // 仅按下了修饰键（无主键）→ 显示当前按住的修饰键但不提交
  if (MODIFIER_KEYS.has(e.key)) {
    captureKeys.value = [
      e.ctrlKey ? 'Ctrl' : null,
      e.shiftKey ? 'Shift' : null,
      e.altKey ? 'Alt' : null,
      e.metaKey ? 'Win' : null,
    ].filter(Boolean) as string[]
    captureError.value = 'Keep holding the modifier and press a main key'
    return
  }

  // 组合校验：至少一个修饰键（F 键除外）
  const mods = captureKeys.value.slice(0, -1)
  if (mods.length === 0 && !/^F\d{1,2}$/.test(key)) {
    captureError.value = 'Add at least one modifier (Ctrl / Alt / Shift / Win), or use an F-key'
    return
  }

  finishCapture(captureKeys.value.join('+'))
}

async function finishCapture(combo: string) {
  const action = capturing.value
  if (!action) return
  capturing.value = null
  captureKeys.value = []
  try {
    combos.value = await setShortcut(action, combo)
    toast.success(`Shortcut updated: ${combo}`)
    captureError.value = ''
  } catch (err) {
    const msg = (err as Error).message || String(err)
    console.warn('[shortcuts] set failed:', msg)
    toast.error(msg)
    captureError.value = msg
  }
}

function prettyCombo(combo: string): string[] {
  return fmtCombo(combo)
}

const confirmReset = ref(false)

async function onResetDefaults() {
  confirmReset.value = true
}

async function doResetDefaults() {
  confirmReset.value = false
  try {
    combos.value = await resetShortcuts()
    toast.success('Shortcuts restored to defaults')
  } catch (err) {
    console.error('[shortcuts] reset failed:', err)
    toast.error('Failed to restore defaults')
  }
}

onMounted(() => {
  load()
  window.addEventListener('keydown', onCaptureKeydown, true)
})

onUnmounted(() => {
  window.removeEventListener('keydown', onCaptureKeydown, true)
})
</script>

<template>
  <div class="shortcuts-view">
    <header class="view-header">
      <h1>Shortcuts</h1>
      <p class="view-subtitle">
        System-wide hotkeys — work even when Aurora is in the background
      </p>
    </header>

    <div class="shortcuts-content">
      <div class="shortcuts-card">
        <div class="shortcuts-toolbar">
          <span class="shortcuts-hint">
            Click a row to rebind. Press <kbd>Esc</kbd> to cancel.
          </span>
          <button class="btn" @click="onResetDefaults">Restore Defaults</button>
        </div>

        <div
          v-for="row in rows"
          :key="row.action"
          :class="['shortcut-row', { capturing: capturing === row.action }]"
          role="button"
          tabindex="0"
          @click="startCapture(row.action)"
          @keydown.enter="startCapture(row.action)"
        >
          <div class="shortcut-label">
            <span class="shortcut-name">{{ row.label }}</span>
            <span class="shortcut-desc">{{ row.desc }}</span>
          </div>
          <div class="shortcut-combo">
            <template v-if="capturing === row.action">
              <kbd v-for="(k, i) in captureKeys" :key="i" class="combo-kbd live">{{ k }}</kbd>
              <span v-if="captureKeys.length === 0" class="combo-waiting">
                Press new combination…
              </span>
            </template>
            <template v-else>
              <kbd
                v-for="(k, i) in prettyCombo(combos[row.action] ?? '')"
                :key="i"
                class="combo-kbd"
              >{{ k }}</kbd>
            </template>
          </div>
        </div>

        <div v-if="captureError" class="capture-error" role="alert">
          {{ captureError }}
        </div>
      </div>

      <p class="shortcuts-note">
        Changes take effect immediately and persist across restarts.
        If a combination is taken by another app, registration for that shortcut fails silently —
        pick a free combination.
      </p>
    </div>

    <!-- 恢复默认确认 -->
    <div v-if="confirmReset" class="modal-scrim" @click.self="confirmReset = false">
      <div class="modal-card" role="dialog" aria-modal="true" aria-labelledby="reset-title">
        <h2 id="reset-title">Restore Defaults</h2>
        <p>Reset all shortcuts to their default combinations? Your custom bindings will be lost.</p>
        <div class="modal-actions">
          <button class="btn" @click="confirmReset = false">Cancel</button>
          <button class="btn btn-danger" @click="doResetDefaults">Reset All</button>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.shortcuts-view {
  display: flex;
  flex-direction: column;
  width: 100%;
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

.shortcuts-content {
  flex: 1;
  overflow-y: auto;
  padding: var(--space-5) var(--space-7);
  min-height: 0;
}

.shortcuts-card {
  background: var(--bg-secondary);
  border: 1px solid var(--border-color);
  border-radius: var(--radius-lg);
  overflow: hidden;
  /* max-width: 720px 已移除（2026-09-07，全拉满右边） */
}

.shortcuts-toolbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--space-3);
  padding: var(--space-3) var(--space-4);
  border-bottom: 1px solid var(--border-color);
}

.shortcuts-hint {
  font-size: 12px;
  color: var(--text-muted);
}

.shortcut-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--space-4);
  padding: var(--space-3) var(--space-4);
  cursor: pointer;
  transition: background var(--transition-fast);
  border-bottom: 1px solid var(--border-subtle);
}
.shortcut-row:last-of-type {
  border-bottom: 0;
}

.shortcut-row:hover {
  background: var(--hover-overlay);
}

.shortcut-row.capturing {
  background: var(--status-info-bg);
  outline: 1px solid var(--status-info-bd);
  outline-offset: -1px
}

.shortcut-label {
  display: flex;
  flex-direction: column;
  gap: 2px;
  min-width: 0;
}

.shortcut-name {
  font-size: 13px;
  font-weight: 500;
  color: var(--text-primary);
}

.shortcut-desc {
  font-size: 11px;
  color: var(--text-muted);
}

.shortcut-combo {
  display: flex;
  align-items: center;
  gap: var(--space-1);
  flex-shrink: 0;
}

.combo-kbd {
  background: var(--surface-0);
  border: 1px solid var(--border-default);
  border-bottom-width: 2px;
  border-radius: var(--radius-sm);
  padding: 2px 7px;
  font-size: 11px;
  font-family: ui-monospace, monospace;
  color: var(--text-default);
}

.combo-kbd.live {
  border-color: var(--accent-color);
  color: var(--accent-color);
}

.combo-waiting {
  font-size: 12px;
  color: var(--status-info);
  font-style: italic;
}

.capture-error {
  margin: var(--space-2) var(--space-4) var(--space-3);
  padding: var(--space-2) var(--space-3);
  background: var(--status-danger-bg);
  border: 1px solid var(--status-danger-bd);
  border-radius: var(--radius-md);
  color: var(--status-danger);
  font-size: 12px;
  line-height: 1.5;
}

.shortcuts-note {
  margin-top: var(--space-4);
  font-size: 11px;
  color: var(--text-faint);
  /* max-width: 720px 已移除（2026-09-07，全拉满右边） */
  line-height: 1.5;
}

/* ---- 确认 modal ---- */
.modal-scrim {
  position: fixed;
  inset: 0;
  background: var(--surface-scrim);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 100;
}

.modal-card {
  background: var(--bg-secondary);
  border: 1px solid var(--border-color);
  border-radius: var(--radius-lg);
  padding: var(--space-5);
  width: 360px;
  box-shadow: var(--shadow-lg);
  animation: fade-up 0.15s ease;
}

.modal-card h2 {
  font-size: 15px;
  font-weight: 600;
  color: var(--text-primary);
  margin-bottom: var(--space-2);
}

.modal-card p {
  font-size: 12px;
  color: var(--text-secondary);
  line-height: 1.6;
  margin-bottom: var(--space-4);
}

.modal-actions {
  display: flex;
  justify-content: flex-end;
  gap: var(--space-2);
}
</style>
