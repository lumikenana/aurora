# Aurora - 当前状态

## Current Phase

**语音模式重构 Phase 6 完成（2026-09-10）：互斥 VoiceMode 移除，PTT / WakeWord 改为并行 Trigger（ADR-041）。① `VoiceMode` 枚举 / `SetMode` / `GetMode` / `CycleMode` / `ParseVoiceMode` / `WithOnModeChange` 全链路删除（backend / bindings / shortcuts / tray / frontend / tests）；② WakeWord Trigger 开关复用 `wakeword.enabled`（默认开启），PTT/Text 始终可用无开关；③ `VC.Start` 自动启动 WakeWord 后台监听，`SetWakeDetector` 注入后立即启动（修复旧"注入后需再切模式"缺陷）；④ 配置迁移：旧 `voice_mode=wakeword/continuous` → `wakeword.enabled=true`，保存后旧键自然淘汰；⑤ 托盘"语音模式"radio、Settings 模式单选、状态栏模式 chip、`cycle_voice_mode` 快捷键、`voiceModeChange` 事件全部移除；⑥ 原 `mode==ModeWakeWord` 门控改为 Conversation/Turn 状态门控，生命周期语义等价。新增 3 并行 Trigger 测试 + 重写 config 3 测试，input 64 测试全绿、race 全绿，agent/tool 全绿，前端 build 通过。ASR/LLM/TTS 流式链路 0 改动。详见 CHANGELOG 2026-09-10 Phase 6 条目。**
**Phase 6 真机验证修复（2026-09-10）：Windows 实测暴露 2 个 fake 测试未建模的 bug，均已修复 —— ① PTT 松开后共享 AudioCapture 被 `Stop()` 且无人重启，WakeWord detector 永久断粮（修复：采集所有权归 VC，`PTTFinished` → `resumeWakewordAudioLocked` 恢复采集）；② PTT 空 ASR 结果不提交 UserInput → Agent 不运行 → standalone Turn 永久卡 TurnProcessing（修复：空 final → `PTTEmptyResult` → `CancelTurn` → Idle）。新增 4 测试，input 包 race 全绿。详见 CHANGELOG 2026-09-10 fix 条目。**
**语音模式重构 Phase 5 完成（2026-09-10）：重构收尾 —— ① 配置迁移清理（`voice_mode` 加载/保存归一化为 ptt/wakeword，旧 continuous 值与 `continuous_*` 键保存后自然淘汰，`ParseVoiceMode` 读取兼容保留）；② 生命周期审计 + 1 处修复（`CancelTurn` 消费 pending exit：RequestEnd 后 ASR 空结果不再回 Waiting 干等 30s，ADR-040）；③ Windows E2E 关键节点日志（Turn started / EnterWaiting / Preempt / trigger source）；④ Text 单入口确认（Main Chat / QuickInput 均经 `onUserInput(SourceText)`）；⑤ VC nil-safe 确认（无 detector/audioCap/全 nil 均有测试）。新增 10 测试（config 3 + input 7），input 包 61 测试全绿，race 全绿。ASR/LLM/TTS 流式链路 0 改动，0 前端改动，0 新配置字段。详见 CHANGELOG 2026-09-10 Phase 5 条目 + ADR-040。**
**语音模式重构 Phase 4 完成（2026-09-10）：Conversation 自然语言退出 + Listening 超时 + Text Trigger 生命周期。① `exit_conversation_mode` LLM Tool（`ConversationManager.RequestEnd()` —— 当前 Turn 正常完成后 Conversation 才结束，不进 Waiting；无 Conversation 安全 no-op；经 `ToolRegistry.Add` 核心工具路径注册，不走 `cfg.Tool.Enabled` 过滤）；② Listening 超时 15s（与 Waiting 30s 独立；仅 WakeWord Conversation；interim 重置；修复"唤醒后完全不说话永久卡 Listening"）；③ Text Trigger 接入统一 Turn 生命周期（`onUserInput(SourceText)` → `VC.StartTextTurn()` = PTTPreempt + standalone Turn + 立即 InputComplete；0 前端改动）。ASR/LLM/TTS 流式链路 0 改动。详见 CHANGELOG 2026-09-10 Phase 4 条目 + ADR-039。**
**语音模式重构 Phase 3 完成（2026-09-10）：旧 Continuous 模式整体移除（ModeContinuous 枚举 / continuous.go 的 session 循环与超时 ticker / 退出关键词 / VC 全部 Continuous 分支 / `WithContinuousSettings` / 配置字段 `continuous_timeout_seconds` + `continuous_exit_commands`）。语音模式收敛为 PTT / WakeWord 两模式；多轮对话能力由 Phase 2 Conversation 生命周期承载；旧配置 `"continuous"` 由 `ParseVoiceMode` 兼容映射为 `"wakeword"`（旧 JSON 键自动忽略，完整配置迁移属 Phase 5）；Conversation 当前仅靠 Waiting 30s 超时结束（自然语言退出属 Phase 4）。ASR/LLM/TTS 流式链路 0 改动，Phase 2 全部行为保持。详见 CHANGELOG 2026-09-10 Phase 3 条目。**
**语音模式重构 Phase 2 完成（2026-09-10）：PTT / WakeWord 接入统一 Conversation/Turn 生命周期。WakeWord 唤醒从"单轮识别"变为"多轮 Conversation"（TTS 结束 → Waiting 继续听下一轮，30s 无输入结束）；PTT 为独立 standalone Turn（完成 → Idle，不进 Waiting）；PTT 可打断 Conversation（停 TTS → PTT Turn → Idle，不恢复原对话）；唤醒词 Pause/Resume 复用 event gating（不重写 detector）；Continuous 保留 legacy 分支待 Phase 3 删除。ASR/LLM/TTS 流式链路 0 改动。详见 CHANGELOG 2026-09-10 Phase 2 条目。**
**UI 全面重构已完成（2026-09-04）：Design System 统一 + Main Window 多视图（Chat/Game/Overlay/Shortcuts/Settings/Logs + 底部状态栏）+ Overlay Edit/Lock 双模式（拖拽/缩放/自定义几何持久化，锁定态单一真相在 agent，游戏启动自动上锁）+ 统一快捷键系统（ShortcutManager registry 9 action，前端可配置/冲突检测，运行时真实生效，散落硬编码注册点全部删除）+ Settings toast 化 + Emergency Hide（Ctrl+Shift+H）。0 新前端依赖。详见 CHANGELOG 2026-09-04 条目 + ADR-035/036。**
**Overlay 稳定性修复完成（2026-09-09）：几何持久化抑制机制 + 透明度保持 + 任务栏隐藏 + UI 简化（单一开关 + 自动保存）。详见下方 Latest Bugfix 条目。**
**UI Polish 完成（2026-09-09）：页面宽度修复（6 view 加 width:100%）+ Settings 合并 Overlay/Shortcuts 为 4 Tab（AI/Voice/Overlay/Shortcuts）+ 侧边栏精简为 Chat/Game/Settings/Logs + 底部状态栏指示灯（PTT 红/TTS 琥珀闪烁/思考紫/Overlay 青/就绪绿）+ 语音模式胶囊 + CLAUDE.md §18 Git 工作流。详见 CHANGELOG 2026-09-09 UI Polish 条目。**
**Enable Overlay 按钮 bugfix（2026-09-09）：UI Polish 重构把 `<Settings />` 改为 `<SettingsView>` 的 slot，但 event listener 仍在 `<SettingsView>` 上，Vue 3 slot emit 不冒泡导致 Enable Overlay 失效。修复：把 props 和 listener 移到 slot 内 `<Settings />` 上。详见 CHANGELOG 2026-09-09 Bugfix 条目。**
**音频设备选择 + VoiceController 解耦修复（2026-09-10）：Settings → Voice Tab 新增 Input/Output Device 下拉选择器（Windows waveIn/waveOut 枚举）；输入设备配置在下次 Start() 生效，输出设备配置 Phase 1 仅保存（Phase 2 waveOut 替代 Oto 后生效）；VoiceController 创建与 WakeWord 可用性解耦 —— 只要存在 audio+speech provider 就创建 VC，WakeWord detector 可选注入，修复未启用唤醒词功能时所有输入方式（文字输入 / PTT / 唤醒词 / 连续对话）的自动 TTS 都不工作的问题。详见 CHANGELOG 2026-09-10 条目。**
**macOS 验证绿：`go build ./...` / `go test ./internal/input/`（61 测试）/ `go test -race ./internal/input/` / `go test -race ./internal/config/` / `go test ./internal/tool/... ./internal/agent/` / `go vet ./...` / `npm run build`（三入口产物）/ `go test ./...` 仅 3 个 pre-existing tft 失败。**
**⚠️ 未验证（诚实记录）：Windows E2E — NOT PERFORMED。清单：① 全局快捷键注册/冲突/修改后真实生效 ② Overlay Edit Mode 拖拽/缩放（DPI 换算、跨显示器边界）③ 游戏启动自动上锁时序 ④ EmergencyHideAll ⑤ 透明/圆角/Edit chrome 渲染 ⑥ 锁定态与几何重启恢复 ⑦（Phase 4 新增）自然语言退出时序（Turn 完成后才结束）⑧ Listening 超时（唤醒后不说话 15s 退出）⑨ 文本输入后 TTS 状态恢复 ⑩（Phase 5 新增）旧 continuous 配置升级后归一化保存 ⑪ 退出意图 + ASR 空结果路径（CancelTurn 消费 pending exit）⑫ 生命周期关键节点日志输出。**
**既有问题（干净 HEAD 同样存在，与 Phase 4 无关）：`internal/game/plugins/tft` 3 测试失败；`internal/agent` 的 `TestSchedulePersistOverlayGeometry_Debounces` race。**
**有意不动：VoiceIndicator 语音状态配色（语义专用 palette）；Settings AI/Voice 双 tab 结构（只保留真实设置项）；Overlay (0,0) 位置视为未自定义（配置向后兼容优先）。**

---

## Latest Bugfix (2026-09-09) — Overlay 几何持久化间歇性失效修复

**症状**：启动程序时 Overlay 位置/尺寸有时正确有时错误，无规律。

**根因**：启动或显示 Overlay 时调用 `SetOverlayGeometry` / `ShowOverlayWindow` 会触发 Windows 的 `WindowDidMove` / `WindowDidResize` 事件。main.go 无条件注册的 hook 会调用 `SchedulePersistOverlayGeometry`，把程序自己搬窗产生的瞬态 bounds（如 0,0 或默认位置）当成用户拖拽结果落盘，覆盖正确配置。

**修复**（3 层防护）：
1. **抑制机制**：`wailsruntime.SuppressGeometryPersist(duration)` — 程序化搬窗期间屏蔽 `SchedulePersistOverlayGeometry`。`SetOverlayGeometry` / `ShowOverlayWindow` 内部自动启用 500ms 抑制
2. **硬过滤**：`SchedulePersistOverlayGeometry` 和 `PersistOverlayGeometry` 增加可见性 + 维度过滤（窗口不可见或 w<=0/h<=0 时直接 return）
3. **启动时序**：初始 Show 从 `ApplicationStarted` 移到 `WebViewNavigationCompleted`（窗口真正就绪后）

**关键文件**：`internal/wailsruntime/wailsruntime.go` / `internal/agent/bindings_overlay_ui.go` / `main.go`

**验证**：`go build ./...` ✓ / `go test ./internal/agent/` ✓（15/15 测试通过）/ 用户冷启动测试位置稳定恢复。

## Latest Bugfix (2026-09-09) — Overlay 透明度在 Locked 模式下 Hide→Show 后失效

**症状**：Locked（HUD）模式下关闭再打开 Overlay，页面透明消失（变成不透明）。Unlock 模式正常。

**根因**：`SetIgnoreMouseEvents(true)` + `SetResizable(false)` 只在切换 Lock 时设置一次。Hidden→Visible 切换后 Windows/Wails 可能重置扩展样式或合成状态。`applyOverlayDWMTransparency` 在 Show 前调用，但 Show 后没有按 locked 状态重套 native 属性。

**修复**：
1. `ShowOverlayWindow` 在 `Show()` 之后再次调用 `applyOverlayDWMTransparency()`
2. `main.go` 在 `overlayVisibilityChanged(true)` 和 `WebViewNavigationCompleted` 的 Show 后调用 `SetOverlayLockedAndBroadcast(GetOverlayLockedState())` 重套 locked native 属性

**关键文件**：`internal/wailsruntime/wailsruntime.go` / `main.go`

**验证**：用户测试 Locked/Unlock 模式下 Hide→Show 透明均正常。

## Latest Bugfix (2026-09-09) — Overlay 窗口隐藏任务栏图标

**症状**：Overlay 窗口开启时在 Windows 任务栏显示图标。

**修复**：通过 Win32 API 设置 `WS_EX_TOOLWINDOW` 扩展样式 + 移除 `WS_EX_APPWINDOW`。`applyOverlayToolWindowStyle()` 在 `ShowOverlayWindow` 中调用（Show 前后各一次确保生效），使用 `SetWindowPos(SWP_FRAMECHANGED)` 使样式更改立即生效。

**关键文件**：`internal/wailsruntime/overlay_dwm_windows.go` / `overlay_dwm_other.go`（非 Windows no-op 存根）/ `internal/wailsruntime/wailsruntime.go`

**验证**：用户测试 Overlay 不再出现在任务栏。

## Latest Enhancement (2026-09-09) — Overlay UI 简化 + 自动保存

**改进**：
1. **移除"Show Overlay Now"按钮**：只保留"Enable Overlay"开关
2. **移除 Save 按钮**：所有配置变化立即生效 + 自动持久化（50ms 防抖）
3. **关闭按钮行为**：Overlay 关闭按钮只临时隐藏（不改 `cfg.Overlay.Enabled`），下次启动仍按 Enable 开关决定
4. **托盘菜单**：Exit 按钮移到最底部

**关键文件**：`frontend/src/views/OverlayView.vue` / `frontend/src/App.vue` / `internal/agent/bindings.go`（新增 `HideOverlayTemporary()`）/ `main.go` / `internal/tray/tray_windows.go`

**验证**：用户测试所有配置立即生效，无延迟，无循环更新。

---

## Latest Bugfix (2026-09-07) — Overlay 白边根除：DWM 层透明（根因 B）

**症状**：上一轮（根因 A，前端层）修复后，面板四周仍有一圈白边/实色底，Opacity=1.0 时最明显（深色面板 + 边缘白框）。

**根因（B 类，native 层）**：白边来自 **Windows DWM 合成层**，不是页面 CSS，也不是 WebView2。排查链条：
- 页面层已透明：`overlay.css` html/body transparent（根因 A 已修）+ WebView2 default background 由 Wails 对 `BackgroundTypeTransparent` 窗口强制 `chromium.SetBackgroundColour(0,0,0,0)`（wails v3 beta.16 `webview_window_windows.go:361`）→ 页面透明像素真正落盘为 alpha=0
- `.overlay-root` 四周留 8px 透明 margin（阴影 + resize edge 预留），这条透明带透出的内容 = DWM 在窗口上叠加的非透明合成 → Win11 对所有顶层窗口（含 WS_POPUP）默认画 **1px 系统色边框（浅色主题≈白色）+ DWM 圆角**；且 Wails 对 Frameless 窗口默认 `DwmExtendFrameIntoClientArea`（"frameless decorations"）进一步强制启用 DWM frame 外观
- 结论：HWND / WebView2 / 页面三层都已经透明，白边是 DWM 在透明窗口上画的系统 chrome，不是任何一层的不透明底色

**修复**（3 文件，Go only）：
- `main.go`：Overlay 窗口 `Windows.DisableFramelessWindowDecorations: true` — 关闭 Wails 默认的 `DwmExtendFrameIntoClientFrame`；Frameless 的 5px native resize 走 `resizeBorderHitTest`，不依赖 decorations，不受影响
- `internal/wailsruntime/overlay_dwm_windows.go`（新，windows tag）+ `overlay_dwm_other.go`（非 Windows no-op 存根）：`ShowOverlayWindow` 时对 Overlay HWND 设 `DWMWA_WINDOW_CORNER_PREFERENCE=DONOTROUND` + `DWMWA_BORDER_COLOR=NONE`（幂等；失败静默 — 旧系统退回默认外观）。面板圆角/描边完全由前端 CSS 续接
- 面板自身圆角 / 描边仍由前端 CSS 绘制（`border-radius: 10px` + 1px 描边），native 层不再叠加任何非透明系统 chrome

**验证**：`go build ./...` ✓ / Windows 交叉编译 `GOOS=windows go build ./internal/wailsruntime/` ✓ / `go test ./internal/agent/` ✓ / 0 前端改动。⚠️ Windows E2E 待重测：0.3 / 0.6 / 1.0 三档透明度下边缘无白边、无实色底、桌面/游戏画面透出、文字可读、拖拽/缩放/Lock 正常。

## Latest Bugfix (2026-09-07) — Overlay Opacity 真正生效（透明背景修复）

**症状**：Settings → Overlay 的 Opacity 滑块对真实 native Overlay 窗口无效——只改面板颜色深浅，窗口永远不透明。

**根因（A 类）**：`overlay-main.ts` import 全局 `style.css`，其 `html, body { background-color: var(--bg-primary) }` 把整个 WebView 涂成不透明深色。`.overlay-root` 的 `rgba(28,32,44, α)` 叠在不透明 body 之上 → **alpha 被压平**。窗口侧（`BackgroundTypeTransparent`）与 config 链路（`SaveOverlayConfig` → 广播 → refetch → `--overlay-opacity`）均正常。

**修复**（3 文件，前端 only，commit `efefe3a`）：
- 新增 `frontend/src/overlay.css`：`html, body { background: transparent }`，仅 overlay 入口 import（主窗口 / QuickInput 不受影响）；`overlay-main.ts` 在 `style.css` 之后 import（同特异性后者胜出，已验证 dist 加载顺序）
- `Overlay.vue`：删 `backdrop-filter`（WebView 看不到身后桌面，恒无效，且 WebView2 透明窗口 + backdrop-filter 有渲染成不透明的已知问题）；加双层 text-shadow 暗描边（α≈0.3 时文字可读性，继承全部子文本）
- 设计要点：面板背景透明（走 `--overlay-opacity`）与文字清晰（不随面板变淡）分离

**验证**：`npm run build` ✓ 三入口产物；0 Go 改动。⚠️ Windows E2E 待重测：0.3 / 0.6 / 1.0 三档透明度 + 低档文字可读性。PR #15。

---

## Latest Bugfix (2026-09-07) — Anchor 移除 + Main Win 全拉满右边

**PR #12 后续 Windows E2E 反馈两条清理任务。**

| # | 用户反馈 | 修复 |
|---|---------|------|
| **A** | Anchor 4 角定位冗余：Unlock 拖动 = 任意位置 | 整个 Anchor 特性删除 —— 字段、UI、CSS、TS interface、Go 逻辑、wailsruntime API 全部移除。默认位置硬编码 bottom-right + 16px margin（与现状一致，零行为变化） |
| **B** | Main Win 右边缝隙宽：Chat 780 / Game 720 / Shortcuts 720 `max-width` 左对齐 | 4 个 view 全部去掉 `max-width` + 居中 `margin: 0 auto` —— 内容撑到右边缘（左右 padding 仍 ~28px） |

**Anchor 移除范围**（共 9 个改动文件 + 1 个自动生成）：

| 层 | 文件 | 改动 |
|----|------|------|
| Go cfg | `internal/config/config.go` | 删 `OverlayConfig.Anchor` 字段 + `DefaultOverlayConfig()` 初始化；doc comment 解释硬编码默认 |
| Go binding | `internal/agent/bindings.go` | 删 `normalizeAnchor()` 函数；`GetOverlayConfig()` map 删 `"anchor"` 键；`SaveOverlayConfig()` 签名删 `anchor string` 参数（7 参 → 6 参） |
| Go overlay UI | `internal/agent/bindings_overlay_ui.go` | `CommitOverlayGeometry` / `ResetOverlayGeometry` 调 `SetOverlayGeometry(...)`（无 anchor 参数）；`GetOverlayGeometry` map 删 `"anchor"`；log msg 改 "default position" |
| Go test | `internal/agent/bindings_overlay_ui_test.go` | `TestPersistOverlayGeometry_ZeroMeansAnchor` → `_ZeroMeansDefault`（纯改名，逻辑不变） |
| Go wailsruntime | `internal/wailsruntime/wailsruntime.go` | `SetOverlayAnchorPosition(anchor)` 重构为私有 `setDefaultOverlayPosition()`（硬编码 bottom-right）；`SetOverlayGeometry(anchor, x, y, w, h)` → `SetOverlayGeometry(x, y, w, h)`（4 参）；`overlayAnchorMargin` → `overlayDefaultMargin` |
| Go main | `main.go` | 3 处 `SetOverlayGeometry(cfg.Overlay.Anchor, ...)` → `SetOverlayGeometry(cfg.Overlay.X, ...)` |
| Frontend TS | `frontend/src/api/backend.ts` | `OverlayConfig` 删 `anchor` 字段；`OverlayGeometry` 删 `anchor` 字段；`saveOverlayConfig` 调用删 `cfg.anchor` 参数 |
| Frontend View | `frontend/src/views/OverlayView.vue` | 删整个 Anchor setting-row + `.anchor-grid/.anchor-btn*` CSS（40 行）；删 `.preview-stage :class="stage-${draft.anchor}"`（dead code）；toast "reset to anchor" → "reset to default" |
| Frontend Component | `frontend/src/components/OverlayPreview.vue` | 删 `.preview-root.anchor-*` 4 CSS 类（16 行）；template `:class="[anchor-...]"` 删；`transformStyle` 改 hardcoded `transformOrigin: 'top left'`；`position: absolute; top: 12px; left: 12px;` 直接写 |
| Frontend defaults | `frontend/src/App.vue` + `frontend/src/overlay-main.ts` | 删 `anchor: 'bottom-right'` placeholder |
| **Generated** | `frontend/bindings/.../bindings.ts` | `wails3 generate bindings -ts` 重跑（`SaveOverlayConfig` 签名变了 1 参） |

**View 全拉满范围**（4 文件）：

| 文件 | 改动 |
|------|------|
| `frontend/src/components/Conversation.vue` | 删 `.conversation` 的 `max-width: 780px` + `margin: 0 auto` |
| `frontend/src/views/ChatView.vue` | 删 `.streaming-hint` 的 `max-width: 780px` + `margin: var(--space-2) auto 0` |
| `frontend/src/views/GameView.vue` | 删 `.game-card` 的 `max-width: 720px` |
| `frontend/src/views/ShortcutsView.vue` | 删 `.shortcuts-card` + `.shortcuts-note` 的 `max-width: 720px` |

**不动的 view**（已满宽）：OverlayView（2 列 grid）、SettingsView / Settings、LogsView / LogPanel。

**向后兼容**：
- 现有 `config.json` 残留 `"anchor": "top-right"` → Go `encoding/json` 自动忽略（不报错；Go 1.10+ 默认行为）
- 不需要 migration code
- 默认位置 = bottom-right + 16px（与现状一致 → 现有用户零感知）

**架构影响**（CLAUDE.md §2）：
- 0 不变量违反（CLAUDE.md §2）
- 0 Agent / Tool / Game / Input 改动
- `OverlayConfig` Go struct 字段减 1（向后兼容 schema 演进，CLAUDE.md §11 不要求 ADR）
- 0 新前端依赖
- `wails3 generate bindings` 重新生成（自动，跟手）

**验证结果**：
- `cd frontend && npm run build` ✓ 93 modules，三入口产物
- `go build ./...` ✓
- `go test ./internal/agent/...` ✓ 全部通过：
  - 3 个架构门禁 PASS（`TestAgent_NoGamePluginDependency` / `TestTool_NoGamePluginDependency` / `TestGenericGame_NoGamePluginDependency`）
  - 10 个 Overlay 几何测试 PASS（`TestPersistOverlayGeometry_*` ×5 / `TestSchedulePersistOverlayGeometry_*` ×2 / `TestFlushPendingOverlayGeometry_*` ×2 / `TestSetOverlayLockedAndBroadcast_UpdatesState`），含重命名后的 `TestPersistOverlayGeometry_ZeroMeansDefault`
- `wails3 generate bindings -ts` ✓ 1 warning（pre-existing `encoding/json` warning，与本次无关）

**Windows E2E 待用户重测**：
- 启动后未拖过 → Overlay 在 bottom-right（与旧版一致）
- 拖动后位置保存
- Reset → 回到 bottom-right
- Settings → **不再有** Anchor 4 角选择器
- Preview → 固定在 preview-frame top-left
- 主窗口拉宽到 ~1400px → Chat / Game / Shortcuts 内容撑到右边缘

**有意不动**（按 §6 / 用户 spec）：
- 不引入 `--content-max-width` 设计 token（用户选"全拉满"非"token 化"）
- Chat 消息行变长 → 可读性下降（用户已确认）
- 不动 Chat / Agent / Voice / QuickInput / Input / TTS / ASR / F2 回调

---

## Latest Bugfix (2026-09-07) — Windows Overlay 验证反馈 3 个 UX 修复

**PR #12 落地 Windows 后用户测试反馈的 3 个 issue 修复。**

| # | 用户反馈 | 修复 |
|---|---------|------|
| **F2** | 拖动 Overlay 窗口时 "Enable Overlay" 开关自动关闭（实际窗口仍在） | `OverlayView.vue` watch 加 `if (isDirty.value) return` —— 用户有未保存改动时不被后端 reload 覆盖 |
| **F4** | Opacity 滑块只改变颜色深浅，不改变透明度 | `Overlay.vue` + `OverlayPreview.vue` 背景色亮一档（`rgba(15,17,22,α)` → `rgba(28,32,44,α)`）+ 增强 `backdrop-filter: blur(16px) saturate(140%)`；alpha 变化现在能透出背后游戏画面 |
| **F5** | Preview 窗口没意义、位置不对（实际 native 是横向，preview 是竖向） | Preview 反映 `cfg.Overlay.Width/Height`（= 实际 native 尺寸），按 `.preview-frame` 可用空间等比缩放（`transform: scale`，`transform-origin` 与 anchor 配对 → 角点位置不变） |

**保留不改**（用户决策）：
- **#3 "Enable Overlay" vs "Show Overlay Now" 重复**：保留两个开关。`Enable Overlay` = 永久配置（cfg 持久化），`Show Overlay Now` = 临时 toggle（与托盘菜单 / Ctrl+Alt+T 同一状态口）。区分有意义，但需要让文案/UI 更明显（**非本次范围**）
- **#6 i18n**：范围太大（设计 token 提取、文本集中、组件模板化、运行时切换），记入"未做"列表，等用户开新 Phase

**改动（共 4 文件，~+130 / -17 行）**：

| # | 文件 | 改动 |
|---|------|------|
| C1 | `frontend/src/views/OverlayView.vue` | 顶部 `import` 加 `onBeforeUnmount`；新增 `previewFrameRef` + `previewScale` + `previewTargetW/H` computed + `updatePreviewScale()` + `watch([W, H])` + `ResizeObserver`（`onMounted` 建立 / `onBeforeUnmount` 断开）；`watch(props.config)` callback 加 `if (isDirty.value) return` 守卫（注释解释原因）；`.preview-frame` 加 `ref`；`<OverlayPreview :scale="previewScale">` |
| C2 | `frontend/src/components/OverlayPreview.vue` | `defineProps` 改 `withDefaults` 加 `scale?: number = 1`；新增 `transformStyle` computed（`transform: scale(s)` + `transform-origin` 按 anchor 选 `top-left/right/bottom-left/right`），`s ≥ 0.999` 时返回空对象（性能 + 不生成无用 inline style）；新增 `rootStyle = overlayStyle + transformStyle`；template `:style="overlayStyle"` → `:style="rootStyle"`；`overlayStyle` 额外注入 `--preview-width` / `--preview-height`（来自 `cfg.Overlay.Width/Height`，clamp 到 `overlayMin/MaxWidth/Height` 一致范围；0 走 280x420 默认） |
| C3 | `frontend/src/components/Overlay.vue` | `.overlay-root` 背景 `rgba(15,17,22,0.85)` → `rgba(28,32,44,var(--overlay-opacity, 0.85))` + 增强 `backdrop-filter: blur(16px) saturate(140%)`；alpha 变化现在透出后面游戏画面，opacity 滑块视觉反馈明显 |
| C4 | `frontend/src/api/backend.ts` | `OverlayConfig` TS interface 加 `width?: number` + `height?: number`（后端 `GetOverlayConfig` 已返回，0 = 未自定义 / Anchor 定位；TS 之前没声明 → untyped 字段） |

**架构影响**（CLAUDE.md §2）：
- 0 不变量违反（CLAUDE.md §2）
- 0 Agent / Tool / Game / Input 改动
- 0 Go 侧改动
- 0 binding schema 改动（OverlayConfig 加 `width/height` 是 TS interface 扩展，Go 侧 struct 本就支持——之前是前端没声明类型）
- 0 新前端依赖

**验证结果**：
- `cd frontend && npm run build` ✓ 93 modules，三入口产物（`index.html` / `quickinput.html` / `overlay.html`）全部生成
- `go build ./...` ✓（macOS ld warning 是 dev box deployment target，非错误）
- 0 单元测试改动（纯 Vue 模板 + TS 扩展，无 Go 改动）

**Windows E2E 待用户重测**（macOS dev box 无法跑）：
- 拖动 Overlay 窗口 → 开关不再被清零（draft 不被覆盖）
- 拉 Opacity 滑块 → 后面游戏画面明显透出（不再只是颜色深浅变化）
- Preview 反映 native 窗口实际尺寸（横纵比匹配），且按窗口大小自动 fit

**有意不动**（按 §6 / 用户 spec）：
- i18n 留作未来 Phase
- "Enable Overlay" / "Show Overlay Now" 文案改进（让两开关区别更明显）留作未来 Phase
- Preview 内不模拟 native 拖拽 chrome（edit-bar / resize-handle 不出现；Preview 是纯 HUD 视觉）
## Latest Bugfix (2026-09-07) — Windows 周期游戏检测 fatal error 修复

**症状**：运行时 `fatal error: too many callback functions`，栈指向 `internal/game.enumWindows`（window_windows.go:53）。
**根因**：`enumWindows()` 每次调用执行 `syscall.NewCallback(闭包)`。Go runtime 的 Windows callback 表上限 2000；`GetActiveGame()`（main.go 5s 轮询）→ `Manager.DetectActive`（TTL 5s）→ TFT `detectTFTWindow` → `FindWindowByClassAndTitle` 周期触发枚举，槽位逐渐耗尽即 fatal。
**修复**：callback 固化为包级单例（`windowsEnumCallback`，进程内只注册一次）；本次枚举的 match/found 上下文封装为 `enumContext`，经 `lParam` 指针传入 callback（每次调用独立实例，并发枚举无共享可变状态）。`FindWindowByClassAndTitle` / `FindWindowByClass` 对外行为不变。
**测试**：新增 `window_windows_test.go`（lParam 通路冒烟 / 3000 次重复枚举 / 16 goroutine × 50 并发防串扰）与 `tft/detect_windows_test.go`（真实检测链路重复 + 并发），需 Windows 上 `go test -race ./internal/game/... ./internal/game/plugins/tft/` 执行。
**遗留**：`internal/audio/audio_windows.go:347` 的 `NewCallback(audioWndProc)` 在每次音频 `createMessageWindow`（Stop+Start 循环）时重复注册，注册的是非闭包函数（Go runtime 对相同函数指针有去重），且重启频率低——暂观察，未修。

---

## Latest Bugfix (2026-09-07) — Overlay Native Interaction 修复

**Overlay 拖拽 / Resize 实现替换为 Wails v3 native 机制：前一次 CSS-transform 方案在 Windows E2E 出现严重回归（拖拽时 native window 不动只动 WebView DOM；落点漂移），已删除并用 Wails v3 官方 frameless + draggable region + native 5px 边缘 resize 路径重做。**

**症状**（前次 `4245a05` 修复，CSS transform-based 拖拽）：
1. **首拖闪烁** —— `transform: translate()` 期间 WebView DOM 移动但 native window 静止，pointerup 时 commit 一次性 SetPosition 触发窗口"跳"到目标位置
2. **第一次尝试修后**：native window 完全不动，光标和窗口位置关系错乱（CSS transform 把内容挪走了，window 仍在原处）
3. **第二次尝试修后**：不再闪烁但出现坐标漂移（落点随机 jump）—— 多次 SetPosition 异步竞争，OS 位置和 CSS 累计 delta 不同步

**根因**：把"拖拽窗口"用"挪 WebView 内部 DOM"模拟。架构上错误 —— Native window 是唯一 Geometry Source of Truth，前端永远不应该用 transform 模拟窗口移动。任何 `pointermove → SetPosition` 链路都不可靠（IPC 异步 + 累计 delta 漂移）。

**修复**（Wails v3 官方机制，Native Window is the only Geometry Source of Truth）：

| 路径 | 机制 |
|------|------|
| 拖拽 | CSS `--wails-draggable: drag` 在 header 上 → Wails v3 drag.js (`@wailsio/runtime/dist/drag.js:99`) 读取 → 触发 `wails:drag` IPC → **OS 直接挪 window** |
| Resize | `WebviewWindow.SetResizable(true)` + Wails 5px 边缘 + 10px 角落 native 命中检测（drag.js:222-289）→ **OS 直接调 window 大小** |
| 几何持久化 | Go 侧注册 `events.Common.WindowDidMove` / `WindowDidResize` hooks（main.go）→ 300ms Go-side debounce (`agent.SchedulePersistOverlayGeometry`) → clamp + 写 cfg + Save + 广播 `overlayConfigChanged` |
| 关闭 / 隐藏前 flush | `events.Common.WindowClosing` + `WindowHide` hooks → `agent.FlushPendingOverlayGeometry()` 落盘最后一次位置 |
| HUD click-through | `locked=true` 时 `wailsruntime.SetOverlayIgnoreMouseEvents(true)` → 鼠标穿透到游戏画面 |
| Edit Mode 启用 resize | `locked=false` 时 `wailsruntime.SetOverlayResizable(true)` → Wails 边缘 resize 可用 |

**改动**（共 4 文件，约 +250 / -180 行）：

| # | 文件 | 改动 |
|---|------|------|
| F1 | `frontend/src/components/Overlay.vue` | 删全部拖拽 / resize JS（`beginDrag` / `onDragMove` / `onDragEnd` / `DragState` / `dpr()` / `rootRef` / `getOverlayGeometry` / `commitOverlayGeometry`）；header `:style` 加 `--wails-draggable: ${editMode ? 'drag' : 'no-drag'}`（Wails drag.js 读这个 CSS 决定是否 native drag）；删 resize-handle DOM（OS 边缘检测接管）；close 按钮移到 edit-bar（HUD click-through 时不可达，符合用户 spec） |
| F2 | `internal/wailsruntime/wailsruntime.go` | 新增 `SetOverlayResizable(bool)` + `SetOverlayIgnoreMouseEvents(bool)` 包装 v3 `WebviewWindow.SetResizable` / `SetIgnoreMouseEvents`（`webview_window.go:574, 1802`） |
| F3 | `internal/agent/bindings_overlay_ui.go` | `SetOverlayLockedAndBroadcast` 额外驱动 native 窗口态：locked → `SetResizable(false)+SetIgnoreMouseEvents(true)`，!locked → `SetResizable(true)+SetIgnoreMouseEvents(false)`；新增 `SchedulePersistOverlayGeometry(x,y,w,h)`（300ms debounce）+ `FlushPendingOverlayGeometry()`（WindowClosing/Hide 前 flush）+ `PersistOverlayGeometry(x,y,w,h)`（clamp + 写 cfg + changed=false 短路 + 广播） |
| F4 | `main.go` | 删 `DisableResize: true`（让 locked 动态控制）；新增 `Windows.WindowDidMoveDebounceMS: 50`（OS-level 50ms 合并，避免 pointermove 60Hz 直打到 Go）；注册 `WindowDidMove` / `WindowDidResize` hooks → `SchedulePersistOverlayGeometry`；`WindowClosing` 已有 hook 增加 `FlushPendingOverlayGeometry`；新增 `WindowHide` hook flush；`ApplicationStarted` 块初始调用 `SetOverlayLockedAndBroadcast(GetOverlayLockedState())` 应用启动时 native 态 |

**关键决策**（与前次错误的根本区别）：
- **Native Window is the only Geometry Source of Truth**：删 `rootRef.transform` 模拟，删 `pointermove → SetPosition`，删前端 per-move IPC
- **Edit / Locked 切换驱动 native 态而非仅 UI 态**：从 `SetOverlayLockedAndBroadcast` 一处入口同时广播事件 + 调 `SetResizable` + `SetIgnoreMouseEvents`，所有路径（前端 binding / 快捷键 / 游戏自动上锁）都自然覆盖
- **OS 已完成窗口移动 → Go 300ms 静默窗口合并 → 写盘**：避免拖拽过程中频繁 Save 抖动；最后位置在 WindowClosing / WindowHide 前 flush 强制落盘
- **HUD 模式 click-through**：删除 HUD 模式下的 close 按钮（HUD 整个窗口透传，按钮本来就不可达），edit-bar 收编所有 chrome 按钮

**架构影响**（CLAUDE.md §2）：
- 0 架构不变量违反
- 0 Agent / Tool / Game / Input 改动
- 0 前端 binding schema 改动（`CommitOverlayGeometry` / `MoveOverlayWindow` / `ResizeOverlayWindow` 仍保留供向后兼容 / 调试；前端 Edit Mode 已不调）
- 0 OverlayConfig schema 改动（用现有 `X / Y / Width / Height` 字段）
- 0 新前端依赖

**验证结果**：
- `cd frontend && npm run build` ✓ 93 modules
- `go build ./...` ✓
- `go vet ./internal/agent/... ./internal/wailsruntime/...` ✓
- `go test ./internal/agent/...` ✓ 新增 10 个测试覆盖 clamp / no-change 短路 / debounce 静默合并 / flush 即时落盘 / no-pending no-op / 并发安全 / locked 状态切换：
  - `TestPersistOverlayGeometry_ClampsToMax` / `_ClampsToMin` / `_ZeroMeansAnchor` / `_NoChangeSkipsSave` / `_DetectsChange`
  - `TestSchedulePersistOverlayGeometry_Debounces` / `_ConcurrentSafe`
  - `TestFlushPendingOverlayGeometry_ImmediateWrite` / `_NoPendingIsNoop`
  - `TestSetOverlayLockedAndBroadcast_UpdatesState`

**⚠️ Windows E2E 未跑**（macOS dev box 限制）：
- Wails 5px 边缘 / 10px 角落 native resize 在 Windows + WebView2 上的实际命中区需验证
- `--wails-draggable: drag` 在 AlwaysOnTop + BackgroundTypeTransparent 窗口上的 drag 启动时延需验证
- `SetIgnoreMouseEvents(true)` + 透明 WebView2 的 click-through 兼容性需 Windows 验证（如冲突 → 单独处理 native overlay hit-test，不通过 JS 模拟）
- DPI 换算（devicePixelRatio 在 Windows 不同 DPI 显示器上的行为）需验证

**有意不动**（按 §6 / 用户 spec）：
- 前次修复中的 VoiceIndicator 删除 / Overlay 拆分（`OverlayPreview.vue`）保留，不重复
- 拖拽不实现 snap-to-anchor / 多显示器边界 clamp
- HUD 模式不做 partial click-through（chrome 可点 + 内容 click-through 不可达 → 选纯 click-through）
- 不动 Chat / Agent / Voice / QuickInput / Input / TTS / ASR / F2 回调

---

## Latest Bugfix (2026-09-07) — **已废弃（superseded by 上方 2026-09-07 修复）**

~~**UI 重构清扫：VoiceIndicator 残留 / Overlay 重构不干净 / 拖拽闪烁与落点不准（3 个 bug 一起修）。**~~

> **更正**：本条目记录的"拖拽改用 CSS transform 实时反馈 + 单次 `commitOverlayGeometry`"方案在 Windows E2E 出现严重回归（拖拽不挪 window、闪烁、落点漂移）。已删除该方案，按 Wails v3 官方 native 机制重做。详见上方 2026-09-07 修复条目。
>
> 保留本节作为历史记录：以下 F1-F6 的 VoiceIndicator 删除 / Overlay 拆分（`OverlayPreview.vue`）**仍有效**，未回退；仅 F4 中"拖拽改用 CSS transform"的描述被上方修复取代。

**症状**（2026-09-04 `9d6cd32` UI 全面重构遗留）：
1. **主界面底部仍浮着唤醒模式 / Speaking / Recording / TTS / 错误 / 超时小卡片** —— `VoiceIndicator` 是旧组件没移除，重复了 status bar（已有 AI:Thinking/Ready）和 Settings 的 Voice Mode radio
2. **Overlay 重构不干净** —— `Overlay.vue` 同时承担"独立 native window"和"主窗口配置页 Preview"两种形态，靠 `native` prop 切 + CSS 互覆盖（`position: fixed` + `anchor-*` vs `position: absolute; inset: 8px`）。Preview 走 `position: fixed` 会逃出 `.preview-frame` (`position: absolute; inset: 0; pointer-events: none`) 容器
3. **解锁 Overlay 后拖拽窗口不停闪烁 + 落点不准** —— `onDragMove` 每个 `pointermove` 都调 `moveOverlayWindow`（Wails IPC 异步），视觉反馈严重滞后于光标；`onDragEnd` 用 `start.startWinX + dx` 算落点（假设窗口还在 startWinX），但中间多个 async IPC 已把窗口挪到不同位置 → 落点漂移

**根因**：
- Issue 1: 删组件时漏了 mount/import/comment 三处
- Issue 2: 形态分离没做彻底 —— native 是 OS 窗口填满（inset 8px），preview 必须在 preview-frame 容器内（anchor 4 角），两者 CSS 互覆盖不可维护
- Issue 3: 每像素 IPC 路径（5-30ms 跨进程往返）根本不适合做实时视觉反馈

**修复**（共 5 文件，约 +260 / -320 行）：

| # | 文件 | 改动 |
|---|------|------|
| F1 | `frontend/src/components/VoiceIndicator.vue` | **整文件删除**（300 行） |
| F2 | `frontend/src/App.vue` | 删 import (L26) + 改注释 (L8) + 删 mount (L341) |
| F3 | `frontend/src/components/OverlayPreview.vue` | **新建** —— 纯视觉 HUD 模拟，bounded 在 preview-frame 内（`position: absolute` + `anchor-*`），无 IPC、无 edit-mode chrome、无 close 按钮；复制必要的样式（~180 行） |
| F4 | `frontend/src/components/Overlay.vue` | 重写为 native-only：删 `native` prop + `position: fixed` + `z-index: 9000` + `width: 360px` + `max-width/height` + `anchor-*` 4 个 CSS 类 + `.native` 分支；`editMode` 简化为 `!props.locked`；拖拽改用 CSS transform 实时反馈 + 单次 `commitOverlayGeometry`；删 `moveOverlayWindow` / `resizeOverlayWindow` import |
| F5 | `frontend/src/views/OverlayView.vue` | 改 import (`Overlay` → `OverlayPreview`) + mount 标签 |
| F6 | `frontend/src/overlay-main.ts` | 删 `native: true` 透传（prop 已移除） |

**拖拽闪烁根因 + 修复原理**：
- 之前：每像素 `pointermove` → IPC → 后端 `SetPosition` → 窗口异步移动 → 浏览器接收位置变化 → 重新渲染（5-30ms 跨进程往返）→ 视觉上"光标在前，窗口在后"拖影
- 现在：每像素 `pointermove` → CSS `transform: translate(dx, dy)` 同步（60fps 浏览器原生渲染）→ 完全跟手；`pointerup` 时**单次** `commitOverlayGeometry(startWinX + dx, startWinY + dy, 0, 0)` → Go 侧 `wailsruntime.SetOverlayGeometry` 内部走 `SetPosition`（已确认 `internal/wailsruntime/wailsruntime.go:219-222`）+ 持久化 + 广播；落点 = 用户光标释放位置（窗口期间没动过，起点=真实位置）
- Resize 同样改用 CSS `width/height` 实时视觉反馈（不缩放内容）+ pointerup 时单次 commit

**架构影响**（CLAUDE.md §2）：
- 0 架构不变量违反
- 0 Agent / Tool / Game / Input 改动
- Go IPC API 完全保留（`MoveOverlayWindow` / `ResizeOverlayWindow` / `CommitOverlayGeometry` 三函数未动），仅前端调用频率变化
- `Overlay.vue` 仍然只依赖 `internal/game` 通用层（不引入游戏名）
- `OverlayPreview.vue` 完全独立文件，不与 `Overlay.vue` 共享 CSS（CLAUDE.md §9：CSS 同样不预先抽公共层；后续若 drift 再抽）

**验证结果**：
- `cd frontend && npm run build` ✓ 93 modules，三入口产物（`index.html` / `quickinput.html` / `overlay.html`）+ `OverlayPreview` + `Overlay` 都编译通过
- `go build ./...` ✓（macOS ld warning 是 dev box deployment target 警告，非错误）
- `go test ./internal/agent/... ./internal/config/... ./internal/wailsruntime/...` ✓
- `go test ./...` ✓ 3 个 pre-existing tft 失败不变（`item_resolver` ×2 + `ocr_parse` ×1，来自 Phase 14-C/14-D，与本次无关）

**未做（按 §6 留作 Optional / Future）**：
- O1: QuickInput 走同样的 native + preview 拆分（非用户报告问题）
- O2: 拖拽加 snap-to-anchor（智能吸附 4 角）
- O3: 多显示器 + DPI 异构边界 clamp
- O4: 抽 `OverlayShared.css` 公共层（等未来 drift 再做）

**macOS 验证**：`npm run build` + `go build` + agent/wailsruntime 单元测试全过。
**⚠️ Windows E2E 未跑**（macOS 开发机）：拖拽跟手 / DPI 换算 / 跨显示器边界需 Windows 验证。

---

## Latest Bugfix (2026-09-07)

**Tray Exit 进程 hang 修复完成。**

**症状**：`wails dev` 启动 → 托盘"退出" → **Go 进程不退**（terminal 卡住）。任务管理器无残留进程（无 webview2 子进程泄漏），纯 Go runtime 因残留 goroutine 而 hang。

**根因**：
- Tray Exit 走 `main.go` 的 `SetOnExitHandler → wailsruntime.Quit() → App.Quit()`，Wails v3 `App.Quit()` 销毁 webview 后让 `v3App.Run()` 返回，`main()` 返回
- **`lifecycle.RegisterOnExit` 注册的所有清理 hook 一次都不执行**（它们只在 `lifecycle.Exit(0)` 路径里跑，而 tray Exit 根本不调它）
- 最致命残留：`main.go` 的 5s game detect 循环（`for { time.Sleep(5s) }`，**无任何 cancel**），单独一个就能让 Go runtime 永不退出
- 次要残留：wakeword auto-download goroutine 用 `context.Background()`，退出后下载继续

**修复**（共 8 处改动，~120 行）：

| # | 文件 | 改动 |
|---|------|------|
| R1 | `main.go:170-177` | Tray Exit handler 改走 `lifecycle.Exit(0)`（替代 `wailsruntime.Quit()`）|
| R2/R7 | `internal/lifecycle/lifecycle.go` | 新增 `QuitCh() <-chan struct{}`（中心化退出信号）+ `QuitContext() (ctx, cancel)`（用于替代 `context.Background()`）+ `CloseQuitCh()` 幂等关闭；`Exit(code)` 顺序改为 `CloseQuitCh → runExitHooks → os.Exit`（先发信号让 goroutine 收通知，hooks 跑完 os.Exit 兜底）|
| R3 | `main.go:807-848` | 5s game detect 循环改用 `select { case <-lifecycle.QuitCh(): return; case <-time.After(5s): }`；`agentBindings.IsReady()` 后二次检查 QuitCh（防止访问正在销毁的 agent）|
| R4 | `main.go:372-407` | wakeword auto-download goroutine 改用 `lifecycle.QuitContext()` 派生 ctx（下载可被取消）；下载完成后 `select` 二次检查，避免"下载完时 app 刚退出"路径启动 VoiceController |
| R5 | `internal/input/shortcuts.go` | 新增 `UnregisterAll()` 方法：持锁快照 + 清空 map + 锁外逐个调 `driver.Unregister`（避免回调路径持锁死锁）|
| R6 | `main.go:455-507` | 清理 hook 重构：1) `sm.UnregisterAll()`（必须在 Wails Quit 前，driver handle 失效后再调会失败）→ 2) `wailsruntime.Quit()`（非阻塞）→ 3) PTT cancel → 4) VoiceController.Stop → 5) TTS.Stop → 6) input.Shutdown → 7) agent.Shutdown → 8) platform/config/log Shutdown |
| R8 | `internal/input/shortcuts_test.go`（新增）| 5 个 UnregisterAll 单元测试：empty / all-registered / idempotent / driver-error-resilient / deadlock-via-probe（持锁调 driver 会被 1s 超时捕获）|

**关键顺序**：
- `UnregisterAll` 必须在 `wailsruntime.Quit()` 之前 — 后者销毁 v3App 后 driver 的 GlobalShortcut 句柄失效
- `lifecycle.Exit(0)` 必须先 `CloseQuitCh()` 再 `runExitHooks()` — 让所有 select 监听 QuitCh 的 goroutine 在 hooks 跑期间就收到信号
- `os.Exit(0)` 在最末兜底 — 即使某个 hook 卡住 / goroutine 没退，进程也会强制结束

**架构影响**（CLAUDE.md §2）：
- `internal/lifecycle` 新增 3 个公共 API，被 `main.go` 引用 — **Agent / Tool / Game / Input / Audio / TTS 都不依赖 lifecycle**，方向正确
- `internal/input/shortcuts.go` 新增方法 — 包内变化
- 3 个架构门禁测试 intact（`TestAgent_NoGamePluginDependency` / `TestTool_NoGamePluginDependency` / `TestGenericGame_NoGamePluginDependency` 全部 PASS）
- 0 新依赖、0 接口变更

**验证结果**：
- `go build ./...` ✓
- `go test ./internal/input/...` ✓ 5/5 新测试 PASS
- `go test ./internal/agent/ -run NoGamePluginDependency` ✓ 3/3 架构门禁 PASS
- `go vet ./...` ✓
- `go test ./...` ✓ 除 3 个 pre-existing tft 失败（`item_resolver` ×2 + `ocr_parse` ×1，Phase 14-C/14-D 遗留，与本次无关）

**未做（按 §6 留作 Optional / Future）**：
- O1: TTS Volcengine `recvLoop` 在 `Stop()` 时显式 `conn.Close()`（与本 bug 无关）
- O2: `agent.streamFinalAnswer` 加 `select { case <-lifecycle.QuitCh(): return }` 提前退出
- O3: `agent.Shutdown` 从 no-op 升级为实际清理 in-flight LLM call
- O4: Hook 内部加 `time.After(2*time.Second)` 总超时兜底
- F1: 退出流程重构成显式 lifecycle phases
- F2: Windows msedgewebview2.exe 子进程残留治理（与本 bug 独立）

**Windows E2E**：未在本环境验证（macOS 开发机），需用户在 Windows 上 `wails dev` 跑一遍确认。

---

## Earlier Phases (recap)

**Phase 8F 已完成：Overlay 独立 native window。**
**Overlay 从 Main webview 内嵌 div 迁出为独立 frameless + always-on-top + transparent native webview（`overlay.html`，Vite 第三入口，360×420 + DisableResize，可交互）；Anchor 语义升级为主屏 WorkArea 四角（`wailsruntime.SetOverlayAnchorPosition`，配置字段不变）；可见性仍由 `overlayVisibleState` 单一真相驱动（`overlayVisibilityChanged` → main.go 订阅 Show/Hide 窗口）；新增 `chatUserMessage` 事件（`Agent.onUserInput` 入口同步 emit，覆盖文本/语音/QuickInput 所有输入源）供 Overlay webview 组装 user 侧对话；Overlay.vue 仅加 `native` prop 样式分支（业务逻辑零改动），OverlayView Preview 保留在 Main webview；Enabled ≠ Visible 设计不变。**
**不做（8F 显式排除）：click-through / 多显示器跟随 / 自由定位 / 新配置项 / exclusive fullscreen 保证。**
**macOS 验证绿：`go build ./...` / `go test ./...`（仅 3 个 pre-existing tft 失败）/ `npm run build`（三入口产物）/ `wails3 build` → bin/Aurora。**
**⚠️ 未验证（诚实记录）：Windows E2E — NOT PERFORMED。清单：① overlay 悬浮于游戏（仅 Windowed / Borderless Windowed；exclusive fullscreen 不保证）② Show 是否抢游戏焦点（v3 无 no-activate 选项）③ Event.Emit 是否到达 overlay webview ④ 任务栏是否出现 overlay 窗口条目 ⑤ 透明圆角渲染 ⑥ 主屏定位 / DPI 换算。**
**Phase 8 Migration Final Audit（2026-09-03）遗留待决策（只审未改）：**
**[F1] Settings 保存 Overlay `Enabled` 不再立即显示窗口（8F 行为差异，需决策是否 Save 后触发 `SetOverlayVisibleAndBroadcast`）；[F2] ✅ 已修复（2026-09-03）：`SaveLLMConfig` 重建 agent 时重复注册输入回调 → `InputManager.RegisterCallback` 改为 `SetCallback`（替换语义）+ `callbacks` 加 RWMutex，Agent 侧收敛到统一入口 `registerInputCallback`（见 CHANGELOG Bugfix 条目）；[F3-F9] 死代码清理候选（wails.json v2 配置 / main.go App 脚手架 / QuickInput Submit 旧链路 / HotkeyToggleQuickInput 常量 / gameDetect+ttsStart+ttsEnd+voiceContinuousTimeout 死监听 / backend.ts 未用导出），详见 2026-09-03 Final Audit 会话记录。**

---

## Earlier Phases (recap)

**Phase 8E 已完成：QuickInput 独立 native window + v3 GlobalShortcut。**
**QuickInput 从 Main webview 内嵌 overlay 迁出为独立 frameless + always-on-top + transparent native webview（`quickinput.html`，Vite multi-page 第二入口）；`QuickInputManager` 改为 driver 注入模式（`internal/input` 零 wails 依赖，driver 由 main.go 桥接 `wailsruntime.Show/HideQuickInputWindow`）；Ctrl+Alt+R 从 Win32 RegisterHotKey 迁到 `v3App.GlobalShortcut.Register`（PTT / CycleMode 仍走 Win32）；窗口 singleton：启动 Hidden，Show/Focus 复用，Esc/Enter/Close 均 Hide 不销毁；提交后不 Show Main Window（Main 由用户手动打开）。**
**提交链路：QuickInput.vue emit('submit') → quickinput-main.ts → `chatStream(text)`（getConfigStatus 前置校验）→ `cancelQuickInput()` 关窗；`llmChunk`/`llmFinish` 事件广播到所有 webview，回复渲染在 Main Chat。**
**macOS 验证绿：`go build ./...` / `go test ./...`（仅 3 个 pre-existing tft 失败）/ `npm run build`（双 entry 产物）/ `wails3 build` → bin/Aurora。**
**⚠️ 未验证（诚实记录）：Windows GUI smoke test — NOT PERFORMED（Ctrl+Alt+R 全局热键、QuickInput 窗口显示/聚焦/透明样式、frameless 拖拽行为均需 Windows 实测）；`GOOS=windows go build` 交叉编译在 macOS 因 store 包 cgo 约束失败（pre-existing，与 8E 无关，baseline 同样失败）。**

---

## Earlier Phases (recap)

**Phase 8D 已完成：tray 迁移 Wails v3 native `app.SystemTray`。**
**Phase 8 (8A + 8B + 8C) 已完成：Wails v2 → v3 迁移（bootstrap + bindings + events）。**
**后端 `wails.Run` → `application.New(...).Run()`（wails/v3 v3.0.0-beta.16）；全部 ~39 个 binding 方法经 `wails3 generate bindings` 生成到 `frontend/bindings/`（ESM + `@wailsio/runtime`）；~23 个事件 emit 收敛到 `internal/wailsruntime` bridge。业务代码（agent / tray / input / game plugins）零修改。**
**macOS 验证全绿：`go build ./...` / `go test ./...`（仅 3 个 pre-existing tft 失败）/ `wails3 build` + `wails3 package` → `bin/Aurora.app`。**
**⚠️ 未验证（诚实记录）：Windows GUI smoke test — NOT PERFORMED（macOS 开发环境）；`wails3 build -platform windows/amd64` 交叉编译 — NOT PERFORMED。Windows 机器上需按 Phase 8 手工清单验证（Main Window / Chat 流式 / Settings / Logs / hide-on-close / tray Exit / Ctrl+Alt+R QuickInput）。**
**本轮不动（留 follow-up）：tray 迁移 `app.SystemTray`（8D）、独立 QuickInput window（8E）、独立 Overlay window（8F）、GlobalShortcut 迁 `app.GlobalShortcut`。**

---

## Earlier Phases (recap)

**Phase 7 已完成：Final UI Consistency & Windows Product Polish。**
**Frontend 全面 token 化（status / space / radius / shadow / interaction）。全局 keyboard focus 用 `--focus-ring`；Chat / Logs / Settings / OverlayView / App 视觉一致。**
**不动：QuickInput / Overlay / VoiceIndicator（Phase 7 task F 明确）。**
**0 架构变更、0 新依赖、0 Pinia/Vue Router 引入。**

---

## Earlier Phases (recap)

**Phase 19 已完成：低延迟流式 TTS 播放。**

**Phase 19 已完成：低延迟流式 TTS 播放。**
**从"完整音频下载 → 临时文件 → MCI 播放"升级为"PCM 流式传输 → 直接播放"，首音延迟降低 70-90%。**
**新增 `internal/audio` 通用音频播放层，实现 OtoPlayer（PCM streaming）+ PlaybackService（带 MCI fallback）。**
**Volcengine TTS 支持 PCM 流式传输，通过 io.Pipe 连接 WebSocket 和播放器，不创建临时文件。**
**架构保持：Agent 不知道 Oto/MCI 细节，TTS Provider 产出 AudioStream，PlaybackService 负责播放。**
**13 个单元测试全部通过，所有现有测试通过（3个 pre-existing failures 与本次无关）。**

**Phase 18-B 已完成：Universal Game Companion 架构验证与收口。**
**架构结论：PASS — Aurora Core 已经真正与具体游戏解耦。**
**游戏能力 Phase 1 已实现（GamePlugin 抽象 + TFT 插件 + 视觉分析链路，构建通过，待 Windows E2E 验证）。**
**Phase 10-6 已完成：TFT GameContext 基础设施（Value[T] + 生命周期 Phase + Provider 扩展接口）。**
**Phase 10-7 已完成：TFT GameContext 第一阶段数据采集（Gold/HP/Level/Stage OCR + 字段级增量 Merge）。**
**Phase 10-8 已完成：TFT 完整 GameContext 扩展（XP/Board/Bench/Shop/Items/Traits/Augments/Carousel + Champion ID 归一化）。**
**Phase 11-1 已完成：TFT AI 决策基础能力 — 5 个只读 LLM 工具（`get_tft_game_state` + 4 个 KB 查询工具），强制 patch 隔离，不修改 Agent Loop。**
**Phase 12 已完成：TFT GameContext → Agent 智能决策闭环（第一版）—— 当前对局状态 → 关键词匹配 → 紧凑 prompt 摘要 → 按需 KB 名称映射 → 注入 ExtraSystemPrompt。**
**Phase 13 已完成：完整 E2E OCR wiring — 窗口检测 → 指定窗口截图 → UpdateContext → OCR Provider → 增量更新 GameContext → 注入 prompt。**
**Phase 14-A 已完成：TFT Champion 识别 — Champion 名称解析 + KB 归一化 → canonical ID 写入 GameContext。**
**Phase 14-B 已完成：TFT Item 识别 — 装备名称解析 + KB 归一化 → canonical ItemID 写入 GameContext.Board[].Items。**
**Phase 14-C 已完成：TFT Bench/Shop Champion 识别 — BenchSlot/ShopSlot canonical ID 写入 + "Name N" 格式解析 + Patch 隔离 + 增量 Merge + Confidence 传递。**
**Phase 14-D 已完成：真实视觉识别 + 端到端验证 — VisionOCREngine 包装 LLM 多模态能力（豆包 Vision）+ FieldAwareOCREngine 接口 + 5 player 字段 + 28 board/9 bench/5 shop/items/traits/3 augments/7 carousel 格子通过 stub LLM E2E 验证 + 严格 patch 隔离 + Unknown 不覆盖 + Confidence 传递 + 失败优雅降级。代码验证通过；真实 Windows E2E 待用户执行。**
**Phase 15 已完成：Knowledge Grounding Layer — 把 *tft.TFTGameContext 投影为 GroundedGameContext（Champion/Item/Trait/Augment 全部带 name/cost/description/effects/role）+ GroundedMeta（patch / setKey / KB 状态 / patch_fallback）+ 严格 3 concentric ring patch 隔离（SQL PK + Service.queryScoped + ID prefix check）+ Unknown / WrongSet / KBUnavailable 显式标注（不静默纠正，不跨 set 替代）+ Grounder 嵌入 GameContextPreparer（grounder==nil 时回退 Phase 12 行为）+ 24 个测试全通过（13 Resolve + 11 Format）。**
**Phase 16-A 已完成：Composition & Synergy Understanding — `internal/game/knowledge/composition` 确定性 CompositionAnalyzer（0 LLM 调用）+ `CompositionFingerprint`（Board Champion ID/Star/Item；排除 Shop/Gold/XP/Bench）+ sync.RWMutex 单槽缓存 + trait 状态机（Active/Inactive/Unknown，当前/下一阶 breakpoint 来自 KB `Trait.Effects`）+ Core Candidate 评分（Likely ≥0.7 / Candidate ≥0.3 / Unknown；权重 HighCost=0.4 / Star3=0.6 / ManyItems=0.3 / HasItems=0.2 / UniqueTrait=0.3（≥2 英雄守卫避免单英雄误判））+ `composition.Format` Markdown 渲染（拼到 grounder.Format 之后，ExtraSystemPrompt 注入）+ 集成到 `GameContextPreparer`（analyzer nil → 跳过；grounder nil → 走 KB 回退）+ 4 个集成测试（已挂/未挂/空 Board/nil grounder）+ 30+ 单测覆盖 11 个 spec 场景。**

**Phase 16-B 已完成：Economy / Equipment / Preparation Intelligence — `internal/game/knowledge/economy|equipment|preparation|gamestate` 三个独立 Analyzer（每个 0 LLM 调用，独立 sync.RWMutex 缓存 + Fingerprint 失效）+ `EconomyFingerprint`（Stage/Phase/Gold/Interest/XP/Streak/HP）+ `EquipmentFingerprint`（Board 装备 ID/Name/Type/From + Pocket 散件；排除 Bench/Shop/Gold）+ `PreparationFingerprint`（Bench Champion/Star + Shop Champion/Cost；排除 Board 装备 / 经济）+ `GameStateAnalysis` 组合器（持有 3 个独立子分析器 + Composition 引用透传 + `AnalysisOptions{NeedEquipment,NeedEconomy,NeedPreparation,NeedComposition}` 控制粒度）+ `economy.Format` / `equipment.Format` / `preparation.Format` / `gamestate.Format` 紧凑 Markdown 渲染（每包独立 format.go；Format 移到各包遵循就近原则）+ `Agent.rebuildPreparer()` 在 Init / SaveLLMConfig hot-reload / SetKnowledgeProvider 三个入口自动重建（grounder 仅在 knowledgeProvider 实现 `GroundingRepository` 时装配）+ `Agent.prepareGameOptions(ctx,userInput) ChatOptions` 接口就位（Phase 16-B 范围内仍返回零值 Options — 把 GameStateAnalysis 切片 → PromptBuilder 属于 Agent Context Scope 阶段，超出本阶段 Required Scope）+ `Manager.LastKnown() *GameContext`（Phase 16-B 新增，不触发检测，仅返回 ContextProvider 写入的快照）+ `Message.ScreenshotAttached bool` 字段 + `ChatOptions{ExtraSystemPrompt,ImageDataURIs}` 结构体 + `NewGameContextPreparer` 5 参签名 `(src, kb, grounder, comp, gsa)` + 4 个包 35+ 单测覆盖 Gold/Interest/Level/XP/ActiveStreak/Board 装备聚合/Pocket 散件/Core 关联/Unknown 类型/WrongSet/Bench-Star 分解/升星距离三档（<3 / 3..8 / >=9）/Summary/Fingerprint 缓存命中与独立失效（含 Gold 变化不重算装备的独立性验证）/Format 空/数据 14 场景。**

**Phase 17-A 已完成：Universal Game Companion Core / Architecture Reorientation — 不再是 TFT AI，而是一个游戏无关的 Universal Game AI Companion，TFT 只是第一个/最成熟的插件。Agent Core 与 `internal/tool/tools` 完全脱离对 `internal/game/plugins/tft` 的直接 import，改为通过通用接口协作：**
- **新增 3 个通用接口**（`internal/game/game.go`）：`GameContextRenderer`（插件可把 GameContext 渲染为 prompt 摘要）/ `ToolContributor`（插件可注册自己的工具）/ `SetupReceiver`（插件可接收 GameState / KB 依赖）+ `SetupDeps{GameState, Knowledge any}`
- **TFT 数据类型下沉**到 `internal/game/plugins/tft/tfttypes/`（独立子包，含 type alias 让 `tft.TFTGameContext` 仍可用），打破 tft → composition → grounder → tft 的父子互引 cycle
- **6 个 analyzer 子包**（composition / grounder / economy / equipment / preparation / gamestate）从 `internal/game/knowledge/tft/` 下沉到 `internal/game/plugins/tft/`（CLAUDE.md §2.4 允许）
- **5 个 TFT 工具**（`get_tft_game_state` + 4 个 KB 工具）从 `internal/tool/tools` 下沉到 `internal/game/plugins/tft/tools/`，由 tft 包 init() 通过 `tool.RegisterTool` 自注册
- **`tft.Plugin` 实现** `GameContextRenderer` / `ToolContributor` / `SetupReceiver` 三个可选接口（`AttachDependencies` 注入 GameState + KB + 构造 `PromptRenderer`；`RegisterTools` 把 deps 注入到 tft 自己的 5 个工具；`RenderGameContext` 委托给内部 `PromptRenderer`）
- **`internal/agent/game_context.go` 退化为 ~80 行薄包装**：`GameContextPreparer.Prepare()` 迭代 `manager.Plugins()` → 对实现 `GameContextRenderer` 的插件调用 `RenderGameContext`，第一个 used=true 的决定结果
- **`internal/agent/tft_tools_wiring.go` 删除**（原本含硬编码 5 个工具名 + `managerSnapshotProvider` 适配器）；适配器移到 agent 通用 `wirePlugins()` 入口
- **`internal/agent/bindings.go` 的 `*tft.TFTGameContext` 类型 switch 删除**，改为读 `gctx.Game`（由 plugin 自己的 `ID()` 填）
- **架构回归门禁**（`internal/agent/architecture_test.go`）：`TestAgent_NoGamePluginDependency` / `TestTool_NoGamePluginDependency` / `TestGenericGame_NoGamePluginDependency` 三个 `go list` 断言，防止后续 PR 重新引入耦合
- **测试结果**：`go build ./...` 通过；Phase 17-A 范围 22 个新 prompt 测试 + 3 个架构门禁 + Agent Loop 通用回归 + tft 子包原有 30+ 测试全部通过；3 个 pre-existing tft 测试失败（item_resolver / ocr_parse），与本阶段无关，按 §6 留待后续
- **架构效果**：新增 DoudizhuPlugin / LoLPlugin 时，Agent Core / `internal/tool/tools` 零修改；只需新建 `internal/game/plugins/<game>/` 包（含 tools 子包）+ main.go blank import + `config.game.enabled`

**Phase 17-B 已完成：Universal Companion End-to-End Loop — 打通"听 → 想 → 看 → 回答 → 说"的完整 E2E 闭环。**
- **核心修复**：
  - `Message` 结构体新增 `Images []llm.ImageContent` 字段（`json:"-"`，避免大图片序列化到对话历史）
  - `convertMessages()` 透传 Images 到 `llm.ChatMessage.Images`，让 OpenAI provider 正确转 content parts
  - `addToolResultToConversation()` 识别 `ToolResultTypeImage`：填充 `Message.Images`（dataURI + MIME）+ 设置 `ScreenshotAttached=true`，让下次 LLM 调用能看到截图
  - 文本/JSON 工具结果保持原有 JSON 序列化行为（向后兼容）
- **主流程切换**：`onUserInput` 从 `ChatStream`（流式，不支持工具）改为 `chatStreamWithTools`（工具循环 + 模拟流式）
- **新增 `chatStreamWithTools(ctx)` 方法**：
  - 循环：非流式 LLM 调用 → 解析 `<|ToolCall|>` → 如果需要工具 → 执行（30s 超时）→ 结果加入对话 → 重复
  - 如果不需要工具 → 最终答案 → 调用 `streamFinalAnswer()`
  - 最大 5 次工具调用（防止无限循环）
  - 错误优雅降级：LLM 调用失败 / 工具执行失败都不 panic，把错误加入对话继续
- **新增 `streamFinalAnswer(answer)` 方法**：
  - 把完整答案按 5 rune 块 + 20ms 间隔推送 `llmChunk` 事件（模拟流式 UX）
  - 结束时发 `llmFinish` + 调用 `postResponseHook`（触发 TTS）
- **系统提示词强化**：`buildSystemPromptWithTools` 增加 `capture_screenshot` 使用时机指导
  - 何时使用：视觉问题（"看看屏幕" / "现在怎么走" / "帮我看看" / "这个界面什么意思"）
  - 何时不使用：普通聊天 / 解释性问题 / 抽象问答 / 已看过画面的追问
- **Token 策略**：
  - 普通问题：1 次 LLM 调用，0 截图
  - 视觉问题：2-3 次 LLM 调用（判断+执行+最终），1 次截图
  - 图片不写入对话历史 JSON（`json:"-"`），只在当次请求发送
- **测试结果**：14 个新测试全通过（工具结果图片/文本/空图/convertMessages 透传/chatStreamWithTools 普通聊天/截图调用/截图失败/max 次数限制/系统提示词/Message 不序列化 Images/混合内容）+ 3 个架构门禁 + Agent Loop 通用回归 + tft 子包原有测试全部通过
- **Pre-existing failures**（与本阶段无关）：`item_resolver`（2）+ `ocr_parse`（1）= 3 个，来自 Phase 14-C/14-D
- **架构保持**：Agent 不 import `internal/game/plugins/*`；Screenshot 是 Generic Tool；Unknown Game 可执行视觉问答；TFT Specialist 无回归

**Phase 17-C 已完成：GameAI → Aurora Identity Migration — 把产品正式名称从旧称 "GameAI" 全面收敛到 "Aurora"，不触碰任何功能架构。**
- **用户数据目录一次性迁移**（`internal/config/config.go`）：新增 `legacyAppDataDir()` + `migrateLegacyAppDataDir()`；新目录 `Aurora`（Win `%APPDATA%\Aurora` / macOS `~/Library/Application Support/Aurora` / Linux `~/.local/share/aurora`）优先；若新目录不存在但旧 `GameAI` 目录存在则 `os.Rename` 一次性迁移；任何失败（跨卷/权限/目标已存在）保留旧目录完整并退回使用旧路径，确保用户数据（config.json / knowledge.db / 日志 / 唤醒模型）不丢失；迁移成功后续运行直接命中新目录
- **Windows 开机自启注册表迁移**（`internal/platform/platform_windows.go`）：`autoStartValue = "Aurora"` + 新增 `legacyAutoStartValue = "GameAI"`；`SetAutoStart(true)` 先删旧值再写新值；`SetAutoStart(false)` 同时清理新旧两个值；`IsAutoStart()` 优先查新值并兼容查询旧值，避免升级后状态跳变
- **应用身份字符串**：`main.go` Title=`Aurora` + 启动日志；`app/cmd/root.go` 版本/帮助/命令名 `aurora`；`app/main/main.go` 启动日志
- **Agent 系统提示词**（`internal/agent/agent.go`）：默认 prompt 身份由 `你是 GameAI` 改为 `你是 Aurora，一个专业的 PC 游戏 AI 助手`
- **日志**（`internal/log/log.go`）：初始化消息 `Aurora logger initialized`；日志文件名前缀 `aurora_YYYYMMDD.log`（旧 `gameai_*.log` 文件保留不动，随目录迁移一起进入新位置）
- **Win32 内部标识**（仅 `//go:build windows` 文件）：`GameAIAudioWindow/GameAI Audio` → `AuroraAudioWindow/Aurora Audio`；`GameAIHotkeyWindow/GameAI Hotkey Listener` → `AuroraHotkeyWindow/Aurora Hotkey Listener`
- **TTS/ASR 内部标识**：MCI alias `gameaitts` → `auroratts`（`internal/tts/playback_windows.go` 5 处）；Volcengine TTS 临时文件前缀 `gameai_tts_` → `aurora_tts_`；Volcengine ASR UID `gameai_user` → `aurora_user`
- **前端**：`index.html` title、`package.json`/`package-lock.json` name、`App.vue` 标题/欢迎/页脚/初始化日志、`Settings.vue` TTS 测试文本 + `%APPDATA%\Aurora\...` 路径占位符；唤醒词示例改为 "hey aurora" 并在文案中注明默认 mycroft 模型不识别该词，需用户自行训练（不假装已支持）
- **脚本 / gitignore**：`scripts/build-windows.sh` 全部 `GameAI.exe` → `Aurora.exe`（与 `wails.json` 已产出的 `Aurora` 保持一致）；`.gitignore` 新增 `aurora-data/` 并保留 `gameai-data/` 兼容旧本地调试残留
- **文档**：`PROJECT.md` / `ARCHITECTURE.md` / `CURRENT_STATE.md` / `DECISIONS.md` / `DEVELOPMENT_GUIDE.md` 标题与当前产品描述全部改为 Aurora；`CHANGELOG.md` 仅改大标题 `# Aurora - 开发变更日志`；**历史条目（旧模块路径 `github.com/lipipi/game-ai` / 历史 MCI alias `gameaitts` / 历史 bug 描述）全部保留原貌**，不篡改历史事实
- **本地目录**：`/Users/lipipi/code/ai-agent/game-ai` → `/Users/lipipi/code/ai-agent/aurora`（代码/文档/构建全部验证通过后最后执行）
- **GitHub Repository**：⚠️ 当前环境未安装 `gh` CLI，无法自动执行 rename；需要用户手动在 GitHub Web 把 `lumikenana/game-ai` 改为 `lumikenana/aurora`，然后 `git remote set-url origin git@github.com:lumikenana/aurora.git`
- **验证结果**：
  - `go build ./...` ✓（macOS + `GOOS=windows GOARCH=amd64 CGO_ENABLED=1` 交叉编译 ✓）
  - `go test ./...` 在改动范围内全部通过；tft 子包 3 个 pre-existing 失败（`item_resolver` ×2 + `ocr_parse` ×1）与本次迁移无关，来自 Phase 14-C/14-D
  - 架构门禁 intact：`go list -deps ./internal/agent | grep 'game/plugins'` 为空；`go list -deps ./internal/tool/... | grep 'game/plugins'` 为空
  - 最终 `git grep -nE 'GameAI|gameai|game-ai'` 剩余 21 条，全部属于：历史 CHANGELOG/DECISIONS 记录（6 条）+ 兼容性迁移代码（config.go 8 条 + platform_windows.go 5 条）+ `.gitignore` 兼容项（2 条）
- **Go module**：保持 `github.com/lipipi/aurora`（未触动）；`go list -m` 验证 ✓
- **未触动**：
  - Go module path（已是 aurora）
  - 任何功能架构 / Agent Loop / Tool Loop / LLM Provider / TFT Plugin / Knowledge Base
  - 第三方协议字段（无发现强制约束的 `gameai_*` 固定字段）
  - 唤醒词模型（仅 UI 文案更新，明确注明需用户自训）
  - GitHub repository 名（工具不足，留待用户手动）

## Completed

✓ 项目目录结构、Go module、Wails v2 配置
✓ 配置管理 / 日志系统 / 统一错误处理 / 生命周期管理 / 平台抽象层
✓ 系统托盘（Windows 实现 + ready 同步）
✓ **Input 模块** - 全局快捷键（专用消息线程模型，ADR-011）
✓ **默认快捷键 Ctrl+Alt+R** 呼出快速输入框（已 E2E 验证）
✓ **快速输入弹出层** - Enter 提交 / Esc 关闭 / 提交后自动关闭
✓ **LLM Provider** - OpenAI 兼容实现，已配置火山引擎豆包 doubao-seed-2.0-lite（ARK API，`/api/plan/v3`）
✓ **Agent Core** - 对话管理 + 流式响应（E2E 验证通过）
✓ **Screenshot 模块** - Windows GDI 全屏截图（1920x1080 约 100ms，E2E 验证通过）
✓ **前端 Vue 3 + Vite + TypeScript 迁移**（ADR-009），旧 ui/ 已删除
✓ Wails 绑定方法全部去除 ctx 参数（ADR-010）
✓ 单 EXE 构建（`wails build`，Windows 本地开发）
✓ **Tool 系统** - 统一 Tool 接口 + ToolRegistry + Agent Loop + screenshot/current_time 工具
✓ **设置 UI** - LLM 配置界面（provider / model / baseURL / apiKey）+ 热加载
✓ **LLM 配置保存 + 热加载** - SaveLLMConfig 支持 agentInstance 为 nil 时的完整初始化
✓ **SSE 流式解析修复** - 正确解析 `data:` 前缀，兼容豆包等思考模型（`thinking.type=disabled`）
✓ **Audio 模块** - `AudioCapture` 接口 + Windows waveIn `CALLBACK_WINDOW` 实现（消息窗口模式）
✓ **Speech 模块** - `SpeechRecognizer` 接口 + 火山引擎流式 ASR 2.0 Provider（二进制 WebSocket 协议）
✓ **PTT Push-To-Talk** - Ctrl+Shift+R 按住录音 + GetAsyncKeyState 释放检测 + 实时 partial transcript
✓ **语音输入端到端** - AudioCapture → SpeechRecognizer → PTTManager → InputManager → Agent（与文本共享同一管道）
✓ **VoiceIndicator 前端组件** - 录音状态指示 + 实时识别文本显示
✓ **语音配置 UI** - Settings 中添加 ASR provider / endpoint / resource ID / API Key 配置
✓ **Agent onUserInput 改为流式** - 语音和文本输入统一走 `ChatStream` + `llmChunk` 事件
✓ **火山引擎 ASR 协议调试全修复** - WAV 头、WIM_DATA 常量、JSON 解析、双提交问题等全部修复 ✓ E2E 验证通过
✓ **openWakeWord 唤醒词检测集成** - 本地 ONNX Runtime 推理 + 状态机 + 自动下载模型 + 前端配置 UI
✓ **唤醒词全链路修复** - ONNX Runtime "Error setting ORT API base: 2" 修复（sync.Once 一次性初始化）+ 前端下载进度条 ✓
✓ **TTS 语音合成 (Phase 1)** - 火山引擎 V3 WebSocket 双向流式 + Windows mciSendString 播放（原 HTTP V1 方案已被 V3 取代）
✓ **统一 VoiceController (Phase 2)** - PTT / WakeWord / Continuous 三模式线程安全切换 + 状态机 + agentBusy/ttsPlaying 双重保护
~~✓ **连续对话模式**~~ **已于语音重构 Phase 3 移除（2026-09-10）** - 免唤醒循环识别退出架构，多轮对话能力由 WakeWord → Conversation 生命周期承载
✓ **TTS + VoiceController 串联 (Phase 3)** - llmFinish → SpeakTTS → ttsPlaying=true（回声抑制）→ NotifyTTSEnd → Continuous 模式自动续接
✓ **TTS V3 双向流式 WebSocket 修复** - 音色/资源匹配（zh_female_xiaohe_uranus_bigtts + seed-tts-2.0）+ 官方载荷结构 + FinishSession 顺序 + 服务端帧布局解析（[4B hdr][4B event][4B sidLen][sid][4B payloadLen][payload]），E2E 收到合法 MP3
✓ **waveIn 录音停止崩溃修复** - Stop 顺序重排 + wndProc 状态守卫 + 消息窗口销毁（消除 0xc0000005 与 HWND 泄漏）
✓ **托盘子菜单 (Phase 4)** - 语音模式 (radio 单选：唤醒/连续/PTT) + 模式变更同步更新勾选
✓ **模式循环热键 (Phase 5)** - Ctrl+Alt+V 循环 PTT → WakeWord → Continuous → PTT
✓ **前端 UI (Phase 6)** - VoiceIndicator 显示模式徽章 + 状态徽章 + TTS 播放指示 + Agent 处理中 spinner
✓ **设置 UI 扩展** - TTS 配置（provider/resource_id/speaker/format/sample_rate/speech_rate/loudness_rate/pitch/apiKey）+ 测试播放 + 模式单选按钮
✓ **配置持久化 (Phase 7)** - voice_mode / continuous_timeout / continuous_exit_commands / tts 配置全部持久化
✓ **TTS 临时文件竞态修复** - 清理从 `defer os.Remove` 挪到 player.OnEnd（播放真正结束后），消除"文件被删导致无声"的竞态
✓ **MCI close/stop 幂等化** - aliasOpen 状态跟踪，消除 close 未打开设备的 263 噪音日志
✓ **RegisterClassExW 先查再注** - GetClassInfoExW 检查已注册则跳过，消除 Stop+Start 后的 WARN
✓ **TTS 全链路 E2E 用户验证** - 对话后语音回答正常播放（2026-08-27）
✓ **GamePlugin 抽象 (ADR-021)** - `internal/game`：接口 + Manager（5s TTL 缓存 DetectActive）+ 工厂注册 + Win32 窗口枚举助手（EnumWindows，供各插件复用）
✓ **TFT 插件** - `internal/game/plugins/tft`：LOL 客户端窗口检测（RiotWindowClass+标题 / Riot Window）+ TFT 教练 system prompt + init 自注册
✓ **LLM 多模态** - `llm.ChatMessage.Images` + openai provider content parts 转换（带图消息转 `[{text},{image_url}]` 数组）
✓ **Agent 游戏集成** - `onUserInput` → `prepareGameOptions`（游戏检测 + 全屏截图）→ `ChatStreamWithOptions`（附加提示词 + 图片只进本次请求）；非视觉模型自动去图重试；图片绝不进对话历史
✓ **配置持久化** - `game.enabled` / `game.screenshot_on_detect`
✓ **前端游戏徽章** - header 显示当前检测到的游戏（gameDetect 事件 + GetActiveGame 绑定）
✓ **Bindings.ChatStream 统一管道** - 文本输入改为委托 onUserInput，与语音完全同路径（含游戏检测）
✓ **TFT 知识库 SQLite 数据结构 (Phase 10-1)** - `internal/storage` SQLite 基础抽象 + migration 机制 + patches/champions/traits/items 四张核心表
✓ **TFT 知识查询分层实现 (Phase 10-2)** - 遵循 `GameKnowledgeProvider → TFTKnowledgeProvider → TFTRepository → SQLite` 分层设计，实现结构化数据查询
✓ **Riot Data Dragon TFT 数据导入 (Phase 10-3)** - `internal/game/knowledge/ddragon` Data Dragon 解析器 + `cmd/import-tft-dd` 导入工具 + `sets` 表 + 完整 `Set/Patch/Mode` 三维度设计，成功导入 16.17.1 标准模式全部核心数据
✓ **TFT CommunityDragon 数据整合 (Phase 10-5-B)** - `internal/game/knowledge/cdparser` CommunityDragon 本地解析器 + `cmd/import-tft-cd` 双数据源整合导入工具，完成 Set 识别+有效数据过滤+物品分类，成功将 16.17.1 SetKey 从 unknown 更新为 set18
✓ **TFT GameContext 基础设施 (Phase 10-6, ADR-023)** - `Value[T]` 可选值容器（`Known/Detected/Unknown/IsKnown/OrZero/OrFallback`）+ `ContextPhase` 状态机 + `ContextSource` 来源枚举 + `CommonData` 通用游戏数据 + `ContextProvider/ContextInput/ContextDelta` 未来 OCR/Vision/GameAPI 扩展接口 + `TFTGameContext` TFT 特定数据结构（Stage/Level/Gold/HP/Board/Bench/Shop/Items/Augments/Traits/Carousel/Opponents 全部 Value 化）+ Manager `UpdateContext/Snapshot/Clear/RegisterContextProvider` + Agent `GetGameContext/GetTFTContext` 只读访问 + Wails `GetGameContext` Binding（隐藏 WindowHWND/Confidence，按 type 暴露 GameData 槽位）
✓ **TFT 第一阶段数据采集 (Phase 10-7)** - `CaptureModeSpecifiedWindow` 接线 + `TFTScreenRegions` UI 区域定义（参考 1920x1080）+ `ScaleRegions` 按比例缩放 + `OCREngine` 抽象 + `MockOCREngine`（基于区域尺寸的桩实现）+ `parseGold/HP/Level/Stage` 容错解析器（带范围校验，0 是合法值）+ `TFTOCRProvider`（`ContextProvider` 实现，链路上 screenshot → 区域裁剪 → OCR → 解析 → ContextDelta）+ `GameDataMerger` 接口 + `TFTGameContext.MergeFrom` 字段级增量更新
✓ **TFT 完整 GameContext 扩展 (Phase 10-8)** - `TFTScreenRegions` 扩展 XP / Board (28 格) / Bench (9 格) / Shop (5 格) / Items / Traits / Augments (3 格) / Carousel + `ScaleCells` 多格子缩放 + `ChampionIDNormalizer` 轻量级 ID 归一化（patch-scoped，不查 Knowledge Base）+ `TFTOCRProvider` 多格子识别 + 解析器扩展（XP/Star/ShopCost/ChampionName/AugmentTier/TraitStyle）+ 所有 Phase 10-8 字段的 MergeFrom 增量更新 + Value[[]T] 空集合覆盖语义验证
✓ **TFT GameContext → Agent 智能决策闭环 (Phase 12)** - `GameContextPreparer` 将 `Manager.Snapshot()` 转换为 LLM 可消费的紧凑 prompt 摘要，通过 `ExtraSystemPrompt` 注入 LLM 上下文，严格 patch 隔离，不修改对话历史，只在用户问 TFT 相关问题且字段已识别时才注入，按需查询 KB 获取英雄中文名称（最多 4 个），不自动执行任何游戏操作
✓ **Knowledge Grounding Layer (Phase 15)** - `internal/game/knowledge/grounder` Grounder 把 `*tft.TFTGameContext` 投影为 `GroundedGameContext`（Champion/Item/Trait/Augment 全部带 name/cost/description/effects/role + MatchStatus: Resolved/Unknown/WrongSet/KBUnavailable）+ `GroundedMeta`（patch / setKey / KB 状态 / patch_fallback）+ 严格 3 concentric ring patch 隔离（SQL PK + Service.queryScoped + ID prefix check）+ `grounder.Format` Markdown 渲染（拼到 FormatTFTGameContext 之后）+ 嵌入 `GameContextPreparer`（grounder==nil 时回退 Phase 12 行为）+ 24 个测试
✓ **CompositionAnalyzer (Phase 16-A)** - `internal/game/knowledge/composition` 确定性阵容分析（**0 LLM 调用**）+ `CompositionFingerprint`（Board Champion ID/Star/Item；**排除** Shop/Gold/XP/Bench）+ sync.RWMutex 单槽缓存（不重复计算；fingerprint 不变 → 返回原对象，GeneratedAt 一致）+ trait 聚合（Active/Inactive/Unknown 三态；breakpoint 完全来自 KB `Trait.Effects`，不硬编码 TFT 规则）+ Core Candidate 评分（`Likely ≥0.7` / `Candidate ≥0.3` / `Unknown`；权重 HighCost=0.4 / Star3=0.6 / ManyItems=0.3 / HasItems=0.2 / UniqueTrait=0.3 带 `len(active)>=2` 守卫避免单英雄误判）+ `composition.Format` Markdown 渲染（拼接在 grounder.Format 之后，注入 ExtraSystemPrompt；Empty Board → 跳过整段；Core Candidate 只输出 Likely+Candidate 抑制噪声）+ 集成到 `GameContextPreparer`（analyzer nil → 跳过；grounder nil → 不调用 analyzer）+ 4 个 Preparer 集成测试 + 30+ 单元测试覆盖 11 个 spec 场景（Single Champion / Multiple same trait / Breakpoint 2/4/6 / Multiple traits / Unknown trait / Patch isolation / Star 1-3 / Items as evidence / Fingerprint cache 9 子场景 / Empty board / Partial grounding）+ 测试中验证 0 LLM calls（无网络/无 llm 包 import；构造器纯本地）
✓ **Universal Game Companion Core / Architecture Reorientation (Phase 17-A)** - Aurora 不再是"TFT AI"，而是 Universal Game AI Companion（TFT 仅第一个插件）
✓ **主链路闭环修复 + GameContext 注入链路恢复 (Phase 18-A)** - 修复 Phase 17-B 重构时丢失的 GameContext → LLM 注入链路：`onUserInput` → `prepareGameOptions` → `gameContextPreparer.Prepare` → 插件 `RenderGameContext` → `ExtraSystemPrompt` → `chatWithOptions` 注入到单次 LLM 请求的 system 消息（不改对话历史）；新增 `chatWithOptions` 方法（防御性处理无 system 消息场景）；`chatStreamWithTools` 接受 `ChatOptions`（同一 user turn 内工具循环迭代共享同一份 ExtraSystemPrompt）；顺带修复 Phase 17-B 的"最终答案双写"bug（中间 tool-call 响应显式写历史，最终答案由 `streamFinalAnswer` 写一次）；新增 `game.UnregisterPlugin` 用于测试清理；7 个新集成测试全部通过（`chatWithOptions` / `prepareGameOptions` / 完整链路 / 工具循环保留 / 空 GameContext / 插件 unused / preparer nil）；架构门禁 intact（agent/tool/game 不依赖 plugins/*）
✓ **Universal Game Companion Architecture Validation (Phase 18-B)** - 架构审计 + 通用插件验证：
  - **架构审计通过**：Agent Core / Tool Core / Game Core 均不依赖 `internal/game/plugins/*`
  - **修复 `internal/tool/config.go`**：移除 5 个 TFT 工具名硬编码（`get_tft_*`），改为通用工具默认配置
  - **增强 `tool.CreateRegistry`**：`cfg.Enabled` 为空时自动启用所有已注册工具（支持插件自注册）
  - **5 个通用插件验证测试**：通过 `MockGamePlugin`（不含任何 TFT 概念）证明 Agent Core 可以处理任意游戏
  - **架构验证场景**：无插件 / 单插件 / 多插件 / Screenshot 工具全部验证通过
  - **Token 控制验证**：图片不进历史 / ExtraSystemPrompt 单次生效 / 非游戏问题 0 额外 token
  - **架构结论**：**PASS** — 新增游戏只需 `internal/game/plugins/<game>/` + blank import + config.enabled
  - **3 个通用接口**（`internal/game/game.go`）：`GameContextRenderer` / `ToolContributor` / `SetupReceiver` + `SetupDeps{GameState, Knowledge any}`
  - **TFT 数据类型下沉**：`*TFTGameContext` 等从 `tft` 包移到 `tfttypes` 子包（type alias 让旧引用 `tft.TFTGameContext` 仍可用）
  - **6 个 analyzer 子包下沉**：`composition / grounder / economy / equipment / preparation / gamestate` 从 `internal/game/knowledge/tft/` 移到 `internal/game/plugins/tft/`（CLAUDE.md §2.4 允许两个位置）
  - **5 个 TFT 工具下沉**：`get_tft_game_state` + 4 个 KB 工具从 `internal/tool/tools` 移到 `internal/game/plugins/tft/tools/`，由该包 `init()` 自注册
  - **`tft.Plugin` 实现** 3 个可选接口：`AttachDependencies` 注入 GameState + KB + 构造 `PromptRenderer`；`RegisterTools` 把 deps 注入到 tft 自己的工具；`RenderGameContext` 委托内部 `PromptRenderer`
  - **`internal/agent/game_context.go` 退化为 ~80 行薄包装**：`Prepare` 迭代 `manager.Plugins()` → 调用 `RenderGameContext`
  - **删除** `internal/agent/tft_tools_wiring.go`（硬编码工具名耦合）+ `*tft.TFTGameContext` 类型 switch
  - **架构回归门禁**：`TestAgent_NoGamePluginDependency` / `TestTool_NoGamePluginDependency` / `TestGenericGame_NoGamePluginDependency`（3 个 `go list` 断言，任何重新引入插件耦合的 PR 立即 fail）
  - **新游戏接入成本**：DoudizhuPlugin / LoLPlugin → 新建 `internal/game/plugins/<game>/` 包 + tools 子包 + main.go blank import；Agent Core / `internal/tool/tools` 零修改
  - **测试**：`go build ./...` 通过；22 个新 prompt 测试 + 3 个架构门禁 + Agent Loop 通用回归 + tft 子包 30+ 测试全过；3 个 pre-existing 失败（item_resolver / ocr_parse）与本阶段无关，按 §6 留待后续

## In Progress

**Voice Mode Refactoring - Phase 1 已完成（2026-09-10）**：建立了 Conversation/Turn 生命周期核心抽象。
- **ConversationManager**：统一对话生命周期管理
  - 状态机：Idle → Listening → Processing → Responding → Waiting → Idle
  - Turn 状态机：Inactive → Input → Processing → Responding → Complete
  - 30 秒 Waiting 超时机制（防止旧 Timer 误关新 Conversation）
- **核心概念分离**：
  - Trigger：用户输入触发方式（PTT/WakeWord/Text）
  - Turn：单次交互（输入 → 处理 → 响应）
  - Conversation：多轮对话（多个 Turn + 超时退出）
- **未改动**：VoiceController 现有逻辑、ASR/LLM/TTS 流式链路、Agent Core
- **下一步**：Phase 2 将把 PTT/WakeWord 接入新的 Conversation/Turn 生命周期
- **测试**：13 个单元测试全部通过（生命周期、超时、并发、非法状态转换等）
- **详见**：`docs/ai/VOICE_MODE_REFACTOR_PLAN.md`

## Not Started

- [ ] 游戏能力 Windows E2E 验证（TFT 窗口检测 + 视觉分析全链路，需 vision 模型）
- [ ] 真实 OCR 引擎接入（Phase 10-7/10-8 用 MockOCREngine，未来替换 Tesseract / PaddleOCR / 云端）
- [ ] GameContext 数据填充 Provider（Phase 10-7/10-8 OCR 已覆盖 Gold/HP/Level/Stage/XP/Board/Bench/Shop/Items/Traits/Augments/Carousel；具体棋盘坐标/Champion 视觉识别留待真实 OCR 接入）
- [ ] SQLite 对话历史存储（知识库结构已完成，对话历史表待加）
- [ ] 开机启动等 platform 模块实际接线
- [ ] **i18n**（用户 2026-09-07 反馈）：Vue 模板硬编码英文文案，需要中/英切换。范围大：设计 token 提取、文本集中、组件模板化、运行时切换（vue-i18n 或自实现）。等用户开新 Phase
- [ ] "Enable Overlay" / "Show Overlay Now" 文案/UI 改进：让两开关区别更明显（"永久配置" vs "临时 toggle" 语义；用户决策 2026-09-07 保留两个）

## Known Issues

- 多实例同时运行会热键注册失败（"already registered"），属预期行为，测试时注意 taskkill 旧实例
- 非 Windows 平台热键/托盘/截图/音频均为空实现（设计预期）
- 截图在多显示器场景未充分测试
- 前端调试日志（`[handleSubmit]` / `[llmChunk]` / `[llmFinish]`）暂未清理，用于排障
- 语音协议调试日志（`parseResp`）暂未清理

## Important Constraints

- 不要求用户安装 Python / Node.js / Docker（提供简单的安装体验）
- 不能拆分多个必须独立启动的进程
- 不能创建 localhost HTTP 服务用于内部模块通信
- Windows 特定代码必须用 `//go:build windows` 隔离
- **Wails 绑定方法禁止声明 context.Context 参数**（ADR-010）
- AI 服务必须 Provider 抽象，不能绑定死厂商
- 游戏支持必须 GamePlugin 抽象，不能耦合特定游戏（见 ADR-021）
- **新增游戏插件包必须在 main.go 添加 blank import** 触发 init 注册（见 ADR-021）
- **游戏插件 Phase 1 不读取游戏内部数据**（不碰内存，见 ADR-021）
- **图片绝不进对话历史**（只存 ScreenshotAttached 标记，追问轮重新截图，见 ADR-021）
- 必须按需截图，不能持续分析画面
- Go 负责核心逻辑，前端只负责 UI
- **新增 LLM Provider 包必须在 main.go 添加 blank import** 触发 init 注册（见 ADR-012）
- **OpenAI 兼容 API 请求必须显式关闭豆包等思考模型的 thinking 模式**（见 ADR-013）
- **waveIn 音频采集必须用 `CALLBACK_WINDOW` 模式**（`CALLBACK_FUNCTION` 在 Go 里回调不可靠）（见 ADR-014）
- **火山引擎 ASR 第一个音频段必须 prepend 44 字节 WAV 头**（服务端靠此识别格式）（见 ADR-015）
- **火山引擎 ASR 默认 Resource ID 为 `volc.seedasr.sauc.duration`**（不是 `bigasr`）（见 ADR-015）
- **火山引擎 ASR 默认 Endpoint 为 `bigmodel_async`**（双向流式）（见 ADR-015）
- **语音输入的前端不应再调 `handleSubmit`**，否则双提交导致响应乱序（见 ADR-016）

## Current Architecture

```
main.go (Wails入口, embed frontend/dist, OnStartup 注入 runtime ctx)
  ├── blank import openai provider（触发 init 注册）
  └── app/cmd (命令行处理)
  └── internal/
       ├── config (✓ 配置管理)
       ├── errors (✓ 统一错误)
       ├── log (✓ 日志系统)
       ├── lifecycle (✓ 生命周期)
       ├── platform (✓ 平台抽象，条件编译)
       ├── tray (✓ 系统托盘，ready 同步)
       ├── input (✓ 热键 + 快速输入 + PTT，专用消息线程)
       ├── llm (✓ Provider 接口 + openai 兼容实现，thinking 模式可配置)
       ├── agent (✓ Agent Core + Wails bindings + 设置 UI 热加载)
       ├── screenshot (✓ GDI 全屏截图)
       ├── game (✓ GamePlugin 接口 + Manager + GameContextRenderer / ToolContributor / SetupReceiver + Win32 窗口枚举助手)
       │    └── plugins/tft (✓ 云顶之弈：LOL 窗口检测 + TFT 教练提示词 + 5 个工具 + 6 个 analyzer + PromptRenderer；tfttypes 子包含数据类型)
       │         └── tools (✓ get_tft_game_state + 4 个 KB 工具，自注册)
       └── tool (✓ Tool 接口 + Registry + Agent Loop)
       ├── audio (✓ AudioCapture 接口 + waveIn CALLBACK_WINDOW 实现)
       ├── speech (✓ SpeechRecognizer 接口 + 火山引擎 ASR Provider)
       ├── tts (✓ TTSProvider 接口 + 火山引擎 V3 WebSocket 双向流式 + mciSendString 播放)
       ├── wakeword (✓ openWakeWord 本地 ONNX 检测)
       └── input (✓ PTT + 唤醒 + 连续三模式 VoiceController + 模式循环热键)
frontend/ (✓ Vue 3 + Vite + TypeScript)
  └── src/components/
       ├── App.vue (✓ 主窗口 + 流式对话 + 事件监听)
       ├── Conversation.vue (✓ 对话显示)
       ├── QuickInput.vue (✓ 快速输入弹出层)
       ├── Settings.vue (✓ LLM + 语音识别配置 UI)
       └── VoiceIndicator.vue (✓ PTT 录音状态 + 实时识别文本)
       └── LogPanel.vue (✓ 日志面板)
scripts/ (✓ screenshot-test 独立验证程序)
docs/ai/ (✓ AI 记忆系统)
```

## Next Step

Phase 18-A 已完成：主链路闭环修复 + GameContext 注入链路恢复。

**候选方向**（等待产品路线确认）：

1. **真实 Windows E2E 验证**：用户运行 Aurora → PTT "帮我看看屏幕" → 验证截图+Vision+TTS+GameContext 注入完整链路
2. **接入真实 OCR 引擎**（Tesseract）替换 Mock
3. **SQLite 持久化**：对话历史保存
4. **更多游戏插件**：LOL / CS2（只需新插件包 + blank import，不改 Agent Core）
5. **复杂截图缓存**：连续视觉追问时复用当前截图（MVP 阶段不做）
6. **原生 function calling**：替代文本标记 `<|ToolCall|>`（需 LLM Provider 支持）
7. **指定窗口截图**：当前 ScreenshotTool 只支持全屏，未来可支持指定游戏窗口

## Validation

### 构建

**Windows 本地（当前开发环境）**:
```bash
cd frontend && npm install && npm run build   # 前端（wails build 会自动执行）
wails build                                    # 输出 build/bin/Aurora.exe
```

**开发模式**:
```bash
wails dev
```

### 已验证

**2026-08-25**:
- `wails build` ✓ 单 EXE 产出
- 全局热键 Ctrl+Alt+R → 快速输入弹窗 ✓（模拟按键 E2E）
- 托盘菜单（快速提问/退出）✓
- **流式对话（豆包 doubao-seed-2.0-lite）✓** 前端逐字显示
- **LLM 设置 UI** ✓ 配置保存 + 热加载 + 首次启动友好
- 全屏截图 1920x1080 / 172KB / ~100ms ✓（按钮 E2E + 独立程序）
- 日志面板实时输出 ✓（log 事件 + console 重定向）

**2026-08-26**:
- **PTT 语音输入 E2E ✓** — 按住 Ctrl+Shift+R → 录音 → 实时 partial → 松开 → 最终文本 → Agent 流式回复
- 火山引擎 ASR WebSocket 连接 ✓（`bigmodel_async` endpoint + `volc.seedasr.sauc.duration` resource）
- waveIn CALLBACK_WINDOW 音频采集 ✓（16kHz/16bit/mono，6400 bytes/200ms）
- 音频电平监控 ✓（max amplitude 日志）
- 语音配置 UI ✓（provider/endpoint/resource ID/apiKey 持久化到 config.json）
- 火山引擎 ASR 协议全链路修复 ✓ — WAV 头 / WIM_DATA 常量 / JSON 解析 / 双重提交问题全部解决
- **语音 + 文本双通道工作正常**，均可触发 Agent 流式回复

**2026-09-01 (Phase 17-B Automated)**:
- `go build ./...` ✓ 全项目编译通过
- `go test ./internal/agent/` ✓ 14 个新测试 + 既有测试 + 3 个架构门禁通过
- **工具循环打通**：Agent 现在可调用 capture_screenshot，截图以 dataURI 形式传给 Vision LLM
- **TTS 链路保持**：`postResponseHook` 在工具循环结束后触发，TTS 正常播放
- **普通聊天不截图**：mock LLM 直接返回文本时，工具循环 1 次 LLM 调用即结束，无截图
- **视觉问题截图**：mock LLM 返回 `<|ToolCall|>` 时，截图工具被调用，图片进入下一轮 LLM
- **错误优雅降级**：截图失败时 Agent 不崩溃，给出文字回答
- **Pre-existing failures**：`item_resolver` (2) + `ocr_parse` (1) 与本阶段无关（来自 Phase 14-C/14-D）
- **Manual E2E Pending**：真实 Windows 环境 + 真实 Vision 模型 + 真实语音 的端到端验证待用户执行

### 排障入口

- 日志文件: `%APPDATA%/Aurora/aurora_YYYYMMDD.log`（config 中 log_level=debug 时含诊断日志；旧 `GameAI/gameai_*.log` 已一次性迁移到 `Aurora/`）
- `FrontendEcho` 绑定方法可确认前端脚本是否加载
- Wails 吞掉部分 ERR 日志时，可重定向 stdout 捕获
- `SaveLLMConfig` / `GetConfigStatus` 有详细日志，可追踪配置保存和 provider 状态
- `readStream` 有 SSE 行级日志（`SSE raw line N: ...`），可追踪流式解析
- `parseServerResponse` 有完整协议解析日志（`[parseResp]`），可追踪 ASR 响应解析
- 音频回调有电平日志（`Audio chunk: X bytes, max amplitude: Y`），可判断麦克风是否正常

### 关键配置路径

- 配置文件: `%APPDATA%/Aurora/config.json`
- 火山引擎 ARK (LLM) Base URL: `https://ark.cn-beijing.volces.com/api/plan/v3`
- 火山引擎 ASR Endpoint: `wss://openspeech.bytedance.com/api/v3/sauc/bigmodel_async`
- 火山引擎 ASR Resource ID: `volc.seedasr.sauc.duration`
- 豆包思考模型必须在请求体中显式 `thinking.type = "disabled"`
