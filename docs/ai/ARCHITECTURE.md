# Aurora - 架构文档

## 系统整体架构

```
┌─────────────────────────────────────────────────────────────┐
│                     User Input                               │
│  (Hotkey / Voice Wake / PTT / Mouse)                        │
└────────────────────────────┬────────────────────────────────┘
                             │
                             ▼
┌─────────────────────────────────────────────────────────────┐
│                    Input Handler                            │
│  - Global shortcut listener  [DONE]                        │
│  - OpenWakeWord detector       [PLANNED]                     │
└────────────────────────────┬────────────────────────────────┘
                             │
                             ▼
┌─────────────────────────────────────────────────────────────┐
│                     Agent Core                               │
│  - Conversation state management                            │
│  - Tool orchestration                                        │
│  - Context management                                        │
└────────────────────────────┬────────────────────────────────┘
             ┌───────────────┼───────────────┐
             │               │               │
             ▼               ▼               ▼
┌──────────────────┐┌──────────────────┐┌──────────────────┐
│  LLM Provider    ││  Speech Provider ││  Tool System    │
│  (LLM API)       ││  (ASR + TTS)     ││  (Screenshot etc)│
└──────────────────┘└──────────────────┘└──────────────────┘
                             │
                             ▼
┌─────────────────────────────────────────────────────────────┐
│                      Output                                 │
│  - UI 显示回答                                                 │
│  - TTS 语音回答                                               │
└────────────────────────────┬────────────────────────────────┘
                             │
                             ▼
┌─────────────────────────────────────────────────────────────┐
│                        UI (Wails)                            │
│  - Main window                                              │
│  - System tray                                              │
│  - Settings                                                 │
└─────────────────────────────────────────────────────────────┘
```

## 模块依赖规则

### 允许的依赖方向（Phase 17-A 重新定位）

```
main → app → internal/* → platform-specific
UI ← Wails ← Go bindings → Agent Core

Agent Core 依赖（仅接口层）:
  - Provider 接口（llm / speech / tts / screenshot）
  - Tool 接口（registry / tool）
  - GamePlugin 接口（internal/game）

具体实现:
  - LLM providers    → 依赖 internal/llm 接口
  - ASR/TTS providers → 依赖 internal/speech 接口
  - GamePlugins      → 依赖 internal/game 接口 + 自注册

Game Plugin 内部依赖方向（Phase 17-A）：
  internal/agent  ─→  internal/game (interfaces)
                         │
                         ▼
                  internal/game/plugins/<game>
                         │  ├── 自身 tools/  (init() self-register)
                         │  ├── 自身 tfttypes/ (data types leaf)
                         │  ├── 自身 composition/ economy/ equipment/
                         │  │   preparation/ gamestate/ grounder/
                         │  │   (analyzers, same package as plugin)
                         │  └── 依赖 internal/tool（仅注册，不反向）
                         │
                         ▼
                  internal/game/knowledge/<game>
                         │
                         ▼
                  internal/game/knowledge  (generic types only)
```

**关键不变量**（CLAUDE.md §2 + §15）：

- `internal/agent/**` **禁止** import `internal/game/plugins/*`
- `internal/agent/**` **禁止** 出现 `tft / lol / cs2` 等具体游戏名字面量
- `internal/tool/**` 与 `internal/tool/tools/**` **禁止** import `internal/game/plugins/*`
- `internal/game/**` **禁止** import `internal/game/plugins/*`
- `internal/game/knowledge/**` **禁止** 出现具体英雄名 / 装备名 / 羁绊名 / 游戏规则字面量
- Plugin 不得反向依赖 Agent Core（Plugin → Agent 属于架构事故）

**回归门禁**：`go test ./internal/agent/ -run Architecture` 三个测试断言上述 import 拓扑，任何重新引入插件耦合的 PR 立即 fail。

### 禁止的依赖

- ❌ UI 直接调用 Agent Core 内部逻辑（必须通过 Wails bindings）
- ❌ Agent Core 直接依赖具体 LLM 厂商实现（必须依赖 Provider 接口）
- ❌ Agent Core 直接依赖具体游戏实现（必须依赖 GamePlugin 接口）
- ❌ 通用层（internal/game / internal/tool/tools）持有具体游戏细节
- ❌ Plugin → Agent 反向依赖
- ❌ 跨模块循环依赖
- ❌ 平台无关代码直接依赖 Windows API

## 模块说明

### Input System [DONE (Windows)]

负责接收用户输入：
- 全局快捷键监听（Ctrl+Alt+R，专用消息线程模型，ADR-011）
- **PTT 按键处理** [DONE] — Ctrl+Shift+R 按住录音，松开识别
- 唤醒词检测 (openWakeWord) [PLANNED]
- 语音激活检测 (VAD) [PLANNED]

**依赖**: 无特殊依赖，可直接调用系统 API

### Agent Core [DONE]

核心调度层：
- 对话状态管理
- 上下文窗口管理
- 流式响应分发（llmChunk / llmFinish 事件）
- 工具调用编排 [PLANNED]

**依赖**:
- Provider 接口
- Tool 接口
- GamePlugin 接口
- config, log, errors, storage

**不依赖**:
- 具体 Provider 实现
- 具体 GamePlugin 实现
- UI 层

**注意**: Wails bindings 方法禁止 ctx 参数（ADR-010）

### LLM Provider [DONE]

大语言模型抽象：
- 统一 ChatCompletion 接口（流式 + 非流式）
- 工具调用支持（通过 `ChatWithTools` Agent Loop 实现）
- 思考模型支持（豆包等 thinking 模型通过 `thinking.type` 参数控制，ADR-013）

**已实现**: `internal/llm/providers/openai` - OpenAI 兼容
（已验证火山引擎豆包 doubao-seed-2.0-lite / ARK API，需 blank import 注册，ADR-012）

**实现**依赖 Agent Core 定义的接口，Agent Core 不依赖具体实现。

### Audio Capture [DONE (Windows)]

音频采集抽象：
- `AudioCapture` 接口定义 (`internal/audio`)
- Windows waveIn 实现 (`CALLBACK_WINDOW` 消息模式，Go 回调更稳定)
- 配置：16kHz/16bit/mono，200ms 分段
- **音频设备选择（Phase 1）**：
  - `AudioDevice` 结构（ID/Name/Index）：ID 当前使用 WinMM 设备名称（szPname）作为匹配标识，Index 仅运行时使用不持久化
  - `DeviceConfig`（`InputDeviceID` / `OutputDeviceID`）：空字符串或 `"default"` 表示跟随系统默认（WAVE_MAPPER），其他值表示设备名称精确匹配，匹配失败 fallback WAVE_MAPPER
  - Windows 设备枚举：`waveInGetDevCapsW` / `waveOutGetDevCapsW`，非 Windows 平台 no-op 存根
  - Wails bindings：`ListAudioDevices()` / `GetAudioDeviceConfig()` / `SaveAudioDeviceConfig(inputDeviceID, outputDeviceID)`
  - Settings UI：Voice Tab 提供 Input Device / Output Device 下拉选择器
  - 输入设备在下次 `Start()` 时生效；输出设备 Phase 1 仅保存配置，Phase 2（waveOut 替代 Oto）后生效

### Speech Recognition Provider [DONE]

语音识别抽象：
- `SpeechRecognizer` 接口定义 (`internal/speech`)
- 流式识别接口，支持 partial / final 结果回调
- **已实现**: `internal/speech/providers/volcengine` - 火山引擎流式 ASR 2.0

**架构分工**:
- `AudioCapture` 负责采集 PCM 音频数据（不关心识别）
- `PTTManager` 协调状态和数据流 (`internal/input/ptt`)
- `SpeechRecognizer` 负责协议编解码和云端识别
- 最终结果 `InputManager.NotifyInput` 进入 Agent，与文本输入同管道
- openWakeWord: 本地唤醒词检测（[PLANNED]）

### TTS Provider [DONE (Volcengine WebSocket V3 + Oto PCM Streaming)]

语音合成抽象（Phase 19 流式升级）：
- `TTSProvider` 接口：`Synthesize / Stop / IsPlaying / IsConfigured / SetOnPlaybackEnd`
- 工厂注册模式（沿用 LLM/Speech），具体实现放在 `providers/volcengine`
- **通用音频播放层**（`internal/audio`）：
  - `Format` 音频格式定义（SampleRate/Channels/SampleFormat）
  - `Stream` 音频流抽象（Reader/Format/Close）
  - `OtoPlayer` 基于 Oto v3.4.1 的 PCM 流式播放器（24kHz/mono/s16le）
  - `PlaybackService` 音频播放服务（Oto PCM streaming + MCI fallback）
  - `PlaybackSession` 每次播放独立会话（独立的 sync.Once 确保 OnEnd exactly once）
- **Volcengine TTS V3 流式协议**：
  - WebSocket 双向流式（`wss://openspeech.bytedance.com/api/v3/tts/bidirection`）
  - PCM 流式传输：WebSocket 接收 PCM 块 → io.Pipe → Oto Player → 音频设备
  - 不创建临时文件，不等待完整音频，首音延迟 210-450ms（比旧流程降低 70-90%）
  - MCI fallback：Oto 初始化失败或格式不支持时自动降级到 MCI 文件播放
- **回声抑制**：`voiceController.ttsPlaying` 标志在 `copyAudioToASR` 中检查，播放期间丢弃音频帧

**架构层次**：
```
Agent
  ↓
TTS Provider (Volcengine)
  ├─ PCM streaming path (Oto)
  └─ MP3 fallback path (MCI)
  ↓
PlaybackService
  ├─ OtoPlayer (PCM streaming)
  └─ MCI fallback (file-based)
```

### Voice Controller [DONE — Phase 6 起并行 Trigger（PTT / WakeWord / Text）]

并行 Trigger 统一管理（ADR-019/020/041；旧互斥 `VoiceMode` 枚举 + `SetMode`/`CycleMode` 已于 Phase 6 移除，旧 ModeContinuous 已于 Phase 3 移除）：
- **不存在"当前语音模式"**：PTT / WakeWord / Text 是三个并行 Trigger，共享统一 Turn/Conversation 生命周期，用户无需理解模式概念
- **Trigger 开关**：WakeWord Trigger = `wakeword.enabled`（默认开启）；PTT / Text Trigger 始终启用（无开关，不为对称而加）
- **配置迁移**：旧互斥 `voice_mode` 键已从 Config struct 移除；`config.Init` 加载时 `migrateLegacyVoiceMode` 把旧值 `"wakeword"` / `"continuous"` 迁移为 `wakeword.enabled=true`（`"ptt"` 保持原值），保存后旧键自然淘汰
- **WakeWord Trigger 启动**：`VC.Start` 时 detector（若已配置）自动开始后台监听；`SetWakeDetector` 注入（模型异步下载完成）后立即启动，无需重启/切换
- 显式状态机：`Idle → WakeDetected → SpeechListening → Processing → Speaking → Idle`
- **agentBusy 保护**：Agent 处理中抑制新一轮 ASR 输入
- **ttsPlaying 保护**：TTS 播放中抑制 ASR（回声）

**Phase 2 起 Trigger 行为（2026-09-10）**：
- **PTT** → 独立 standalone Turn：`PTTPreempt`（打断当前 Conversation/TTS）→ `StartPTTTurn` → ASR → `PTTFinished`（输入结束）→ Agent → TTS → Turn Complete → **Idle**（不进 Waiting）
- **WakeWord** → Conversation：唤醒 → `StartConversation` + 第一个 Turn → 每轮 TTS 结束 → Waiting + `startConversationSession()` 重启 ASR 听下一轮 → 30s 无输入 / Interrupt → Conversation 结束 → Idle
- **唤醒词 Pause/Resume** = event gating：detector 后台持续运行，Conversation / PTT Turn 期间事件在 `handleWakeDetected` 被 lifecycle 门控忽略，回 Idle 后自动恢复（不重写 detector）
- **PTT 打断 Conversation**：`PTTPreempt` → `convMgr.Interrupt()` + 停 TTS → PTT standalone Turn → Idle，**不恢复原 Conversation**
- **ASR 空结果**：`CancelTurn()` 丢弃该 Turn → 回 Waiting（timer 重启），不再卡死 Processing

**Phase 4 起 Trigger 行为补充（2026-09-10，ADR-039）**：
- **Text** → 独立 standalone Turn（与 PTT 语义完全一致，唯一区别是输入来源）：`agent.onUserInput(SourceText)`（Main Chat / QuickInput 单一汇聚点）→ `VC.StartTextTurn()` = `PTTPreempt` + `StartStandaloneTurn` + 立即 `InputComplete`（文本已就绪无需 ASR）→ Agent → TTS → Turn Complete → **Idle**。不创建 Conversation；与 PTT / WakeWord Trigger 并行可用；VC 不存在（无音频环境）时退化为无生命周期管理
- **自然语言退出**：`exit_conversation_mode` LLM Tool → `convMgr.RequestEnd()` → 当前 Turn 正常完成后 Conversation → Idle（不进 Waiting）；无 Conversation 时安全 no-op。不恢复本地退出关键词

### Conversation Manager [DONE - Phase 1+2 of Voice Refactoring]

统一的对话生命周期管理（2026-09-10，Voice Mode Refactoring Phase 1）：

**核心概念**：
- **Trigger**：用户输入触发方式
  - PTT（Push-to-Talk）：手动按键触发单次 Turn
  - WakeWord：语音唤醒触发 Conversation（多轮对话）
  - Text：文本输入触发单次 Turn
- **Turn**：单次交互生命周期
  - 状态：`Inactive → Input → Processing → Responding → Complete`
  - 流程：用户输入 → ASR → LLM → TTS → 完成
- **Conversation**：多轮对话生命周期
  - 状态：`Idle → Listening → Processing → Responding → Waiting → (循环或 Idle)`
  - 由 WakeWord 触发进入，30 秒超时或用户主动退出时结束
  - PTT 不创建 Conversation，只创建单个 Turn

**状态机**：
- `ConversationState`：管理对话级别状态（Idle/Listening/Processing/Responding/Waiting）
- `TurnState`：管理单次交互状态（Inactive/Input/Processing/Responding/Complete）
- 两个状态机独立但协调工作

**超时机制（Phase 4 起两个独立超时，ADR-039）**：
- **Waiting 超时（30s）**：等待"下一轮开始"。Waiting 状态启动定时器；用户新输入（interim）停止定时器；超时 → Conversation 结束 → Idle
- **Listening 超时（15s）**：等待"本轮开始说话"，仅作用于 WakeWord Conversation（"唤醒后一直不说话"的兜底）。timer 生命周期集中在 `setConvStateLocked`（进入 ConvListening 且 Conversation 活跃且非 standalone Turn → 启动；离开 → 停止）；ASR interim 经 `ResetListeningTimer()` 重置；超时 → Conversation 结束 → Idle
- PTT/Text standalone Turn 经过 ConvListening 但不参与 Listening 超时（输入由用户手动控制）
- 两个 timer 均使用 generation 快照 + 触发时状态校验，防止旧 Timer 误关新 Conversation

**Conversation 结束路径（Phase 5 起四种，均最多触发一次 onConvEnd）**：
1. Waiting 超时（30s 无新输入）
2. Listening 超时（15s 未开始说话）
3. 自然语言退出：`exit_conversation_mode` LLM Tool → `RequestEnd()` 标记 `endingRequested` → 当前 Turn 正常完成（`ResponseComplete`）后结束，不进入 Waiting；无进行中 Turn（Waiting）时立即结束；无活跃 Conversation 时安全 no-op（幂等）
4. pending exit + 异常结束（Phase 5，ADR-040）：`RequestEnd` 标记后 ASR 空结果走 `CancelTurn()` → 直接结束（不进 Waiting、不重启 timer）；无 exit 请求时 `CancelTurn` 保持回原 Waiting 语义

**关键特性**：
- 线程安全：所有状态访问通过 Mutex 保护
- 回调系统：onStateChange / onTurnStart / onTurnEnd / onConvStart / onConvEnd
- 回调异步执行：避免持锁调用导致死锁
- 并发安全：支持多 goroutine 并发访问

**实现文件**：
- `internal/input/conversation.go`：ConversationManager 实现
- `internal/input/conversation_test.go` + `conversation_phase4_test.go` + `conversation_phase5_test.go`：34 个 ConversationManager 单元测试
- `internal/input/voice_controller_test.go` + phase5 测试：17 个 VoiceController 集成测试（fakes）
- `internal/tool/tools/conversation_control.go`：`exit_conversation_mode` 工具（薄适配层，经 `ToolRegistry.Add` 核心工具路径注册，不走 `cfg.Tool.Enabled` 过滤；依赖经 `ConversationExitRequester` 最小接口反转）

**与 VoiceController 的关系（Phase 2 已完成接入，2026-09-10）**：
- VoiceController 持有 `convMgr *ConversationManager`，驱动所有 Conversation/Turn 状态推进
- Phase 2 扩展的 ConversationManager API：
  - `StartStandaloneTurn()`：PTT 独立 Turn（Complete → Idle，无 Waiting / timer）；stale turn 自动重置
  - `Interrupt()`：立即终止 Conversation / standalone Turn（PTT 抢占、模式切换共用），generation 防护
  - `CancelTurn()`：丢弃输入阶段 Turn（ASR 空结果），Conversation 回 Waiting 并重启 timer
  - `IsTurnInProgress()` / `IsStandaloneTurnActive()`：lifecycle 门控查询
- 接入点：`handleWakeDetected`（唤醒 → Conversation）、`handleTranscript`（interim → StartTurn 停 timer + ResetListeningTimer / final → InputComplete / 空 → CancelTurn）、`NotifyAgentComplete`（→ Responding）、`NotifyTTSEnd`（standalone → Idle / Conversation → Waiting + 重启 ASR / endingRequested → Idle）、`PTTPreempt`（Interrupt）、`PTTFinished`（InputComplete）、`onUserInput(SourceText)`（→ StartTextTurn）
- Phase 4 扩展的 ConversationManager API：`RequestEnd()`（自然语言退出，延迟到当前 Turn 完成）、`ResetListeningTimer()`（interim 活动重置 Listening 超时）
- 旧 Continuous 模式（免唤醒循环识别）已于 Phase 3（2026-09-10）整体移除：多轮能力由 Conversation 生命周期承载

**详见**：`docs/ai/VOICE_MODE_REFACTOR_PLAN.md`

### Screenshot Provider [DONE (Windows GDI)]

截图抽象：
- 截取全屏 ✓（GDI: GetDC + BitBlt + GetDIBits，返回 PNG base64）
- 截取指定窗口 [PLANNED]
- 坐标转换 [PLANNED]

### Tool System [DONE]

工具执行系统（Phase 17-A 重新定位）：
- 定义 Tool 接口（`internal/tool/tool.go`）
- 工具注册机制（`ToolRegistry`，工厂 + init 自注册）
- AI 调用工具执行（Agent Loop：`ChatWithTools`，最多 5 次调用，30s 超时）
- 结果返回给 AI，追加到对话上下文
- **通用工具注册表** `internal/tool/tools/` 只挂载通用工具（不混入任何游戏专属工具）
- **游戏专属工具** 放在 `internal/game/plugins/<game>/tools/`，在包 `init()` 中自注册

**通用工具**（`internal/tool/tools/`）：

| Tool | 来源 |
|------|------|
| `capture_screenshot` | ScreenshotTool（全屏截图） |
| `get_current_time` | CurrentTimeTool（当前时间） |

**游戏插件工具**（在插件包 init() 自注册）：

| Plugin | 工具 |
|--------|------|
| `internal/game/plugins/tft/tools/` | `get_tft_game_state` + `get_tft_champion` + `get_tft_item` + `get_tft_trait` + `get_tft_augment` |

**架构保证**：
- `internal/tool/tools/init.go` grep 不到 `get_tft_*`
- `internal/tool/tools/` grep 不到 `internal/game/plugins/*` import
- `go test ./internal/tool/...` 中 `TestTool_NoGamePluginDependency` 守门

### Game Plugin System [DONE (Phase 17-A, ADR-021 / ADR-023 / ADR-027)]

游戏支持抽象（Phase 17-A 重新定位为 **Universal Game Companion Core**）：

**核心接口**（`internal/game`）：

```go
type GamePlugin interface {
    ID() string
    Name() string
    Detect(ctx context.Context) (*GameContext, error)  // 未检测到返回 (nil, nil)
    SystemPrompt() string                                // 插件自带 system prompt
}

// 3 个可选接口（type assertion 触发，零方法污染）
type GameContextRenderer interface {
    RenderGameContext(ctx *GameContext, userInput string) (summary string, used bool)
}

type ToolContributor interface {
    RegisterTools(reg *tool.ToolRegistry) error
}

type SetupReceiver interface {
    AttachDependencies(deps SetupDeps) error
}

type SetupDeps struct {
    GameState  GameStateProvider
    Knowledge  any  // 插件自己 type-assert
}
```

**Plugin 目录结构**（以 tft 为例）：

```
internal/game/plugins/tft/
├── tft.go                    # Plugin 主结构 + 3 个可选接口实现
├── prompt.go                 # PromptRenderer（~600 行，原 internal/agent/game_context.go）
├── system_prompt.go          # 插件自带的 system prompt 内容
├── types_alias.go            # type alias re-export tfttypes（向后兼容）
├── tfttypes/                 # 数据类型叶子包（不 import tft）
│   └── types.go              # TFTGameContext / BoardSlot / ... 10 个类型
├── knowledge_provider.go     # type KnowledgeProvider = tools.KnowledgeProvider
├── tools/                    # 插件工具（init() 自注册到全局 ToolRegistry）
│   ├── init.go
│   ├── tft_game_state.go
│   ├── tft_knowledge.go
│   └── *_test.go
├── composition/              # 阵容分析（同包，允许 import tfttypes）
├── economy/                  # 经济分析
├── equipment/                # 装备分析
├── preparation/              # 备战分析
├── gamestate/                # 对局状态分析
└── grounder/                 # 知识库投影（Phase 15）
```

**Agent 集成**（Phase 17-A 后）：

```
onUserInput
  ↓
prepareGameOptions
  ↓
GameContextPreparer.Prepare
  ↓  迭代 manager.Plugins()
  ↓  对实现 game.GameContextRenderer 的插件 → 调 RenderGameContext
  ↓  第一个 used=true 的结果作为 ExtraSystemPrompt 追加
  ↓
ChatStreamWithOptions (system prompt 副本 / screenshot 只进当次请求)
```

```
Init / SaveLLMConfig
  ↓
wirePlugins
  ↓  迭代 manager.Plugins()
  ↓  实现 game.SetupReceiver    → 注入 state + KB deps
  ↓  实现 game.ToolContributor  → 注册自己的工具
  ↓
ready
```

**已实现插件**：

| Plugin | 位置 | 状态 |
|--------|------|------|
| TFT（云顶之弈）| `internal/game/plugins/tft` | ✓ 完整闭环（Phase 17-A 后所有能力完整保留） |

**新增游戏流程**：
1. 新建 `internal/game/plugins/<game>/` 包
2. 实现 `GamePlugin` 接口 + 任意可选接口
3. 若需要工具：在子包 `init()` 中调 `tool.RegisterTool`
4. `main.go` 添加 blank import
5. `config.game.enabled` 加 ID

**不修改** Agent Core / `internal/tool/tools` / `internal/game`。

### UI (Wails + Vue 3)

前端展示层（`frontend/`，Vue 3 + Vite + TypeScript），运行于 **Wails v3 (v3.0.0-beta.16)**（Phase 8 由 v2 迁移，见 ADR-032）：
- 主窗口（对话 / 日志面板）✓
- 快速输入弹出层（QuickInput 组件，仍内嵌 Main Window）✓
- 设置界面（Settings.vue，LLM 配置 provider/model/baseURL/apiKey + 热加载）✓
- 系统托盘（Go 侧实现，`getlantern/systray`，未迁 v3 SystemTray）✓
- 悬浮球 [PLANNED]

**依赖**:
- 通过 `src/api/backend.ts` 封装调用 Wails bindings（v3：`wails3 generate bindings` 生成的 `frontend/bindings/` ESM 模块，`@wailsio/runtime` `Call.ByID`）
- 通过 `src/api/events.ts` 订阅 Go 事件（llmChunk / llmFinish / log / overlayConfigChanged 等；v3：`Events.On`，payload = `ev.data`）
- 不直接依赖内部模块

### Multi-Window 布局（Phase 8D–8F）

三个物理独立 webview + v3 SystemTray，均为 singleton、启动 Hidden（Main 除外）、关闭一律 Hide 不销毁：

| 窗口 | 入口 | 形态 | 可见性驱动 |
|------|------|------|-----------|
| Main | `index.html` | 常规窗口 | hide-on-close（tray Exit 才退出） |
| QuickInput | `quickinput.html`（8E） | Frameless + AlwaysOnTop, 560×180 | `QuickInputManager` 状态机 → driver（`input.NewFuncDriver` 桥接 wailsruntime）；Ctrl+Alt+R = v3 `GlobalShortcut.Register`（OS 级，唯一注册点） |
| Overlay | `overlay.html`（8F） | Frameless + AlwaysOnTop + Transparent + DisableResize, 360×420 默认（UI 重构起使用持久化几何 cfg.Overlay.W/H） | `overlayVisibleState`（agent 唯一写入口）→ `overlayVisibilityChanged` 订阅 Show/Hide；几何 = Anchor fallback + 自定义位置优先（`SetOverlayGeometry(anchor,x,y,w,h)`）；锁定态 `overlayLockedState`（agent 唯一写入口）→ `overlayLockChanged` 广播三端（Main Window / 快捷键 / Overlay 内 Lock 按钮）；Edit Mode（native && !locked）下 header 拖拽 / resize 手柄 → Move/ResizeOverlayWindow（live）→ CommitOverlayGeometry（Go clamp + 持久化 + `overlayConfigChanged`） |

Vite multi-page：`rollupOptions.input` 三入口（index / quickinput / overlay），bundle 互不依赖。
事件广播（`App.Event.Emit`）到达所有 webview；`chatUserMessage`（`Agent.onUserInput` 入口 emit，所有输入源唯一汇聚点）专供 Overlay webview 组装 user 侧对话。
提交链路：QuickInput 提交 → `chatStream` → `llmChunk`/`llmFinish` 广播 → Main Chat 与 Overlay 各自渲染；提交后不 Show Main Window。

### Runtime Bridge (`internal/wailsruntime`)

Phase 8 新增的 v3 运行时收敛层 — `wails/v3` import 只存在于 `main.go`、本包、`internal/tray`（8D 起，v3 SystemTray menu 构造需要 application 类型）：

- `App` / `MainWindow` / `QuickInputWindow`（8E） / `OverlayWindow`（8F） — 由 `main.go` bootstrap 后注入的句柄
- `EmitEvent(name, data...)` — 后端事件广播（App 未启动时 no-op）
- `SubscribeEvent(name, cb)` — 后端事件订阅（返回 unsubscribe）
- `GetContext()` — app 长生命周期 context（启动前 Background）
- `Quit()` / `ShowMainWindow()` / `HideMainWindow()`
- `Show/Hide/ToggleQuickInputWindow()`（8E） / `Show/HideOverlayWindow()` + `SetOverlayAnchorPosition()`（8F）
- `Move/ResizeOverlayWindow()` / `SetOverlayGeometry()` / `EmergencyHideAll()`（UI 重构 2026-09-04：EmergencyHide 隐藏 Overlay+QuickInput+Main 三窗口，后台服务不受影响）
- hide-on-close：`RegisterHook(events.Common.WindowClosing)` + `e.Cancel()`（替代 v2 `HideWindowOnClose`）
- startup：`OnApplicationEvent(events.Common.ApplicationStarted)`（替代 v2 `OnStartup`）

其余内部包（agent/input/game 等）不 import wails；`internal/input` 通过 `QuickInputDriver` 接口接收窗口操作（main.go 注入实现），`internal/agent` 只暴露状态与事件（`overlayVisibleState` / `chatUserMessage`），窗口 Show/Hide 全部由 main.go 的 main runtime 层订阅完成。

### 统一快捷键系统（UI 重构 2026-09-04，ADR-035）

键值唯一来源 `cfg.Shortcuts`（`internal/config`，为空时 ShortcutManager 内置默认值兜底，老配置平滑兼容）：

```
cfg.Shortcuts → ShortcutManager.ApplyAll（internal/input/shortcuts.go，8 action registry）
              → ShortcutDriver（FuncShortcutDriver，main.go 注入）
              → v3 GlobalShortcut.Register/Unregister（OS 级，唯一注册点）
              → 8 handlers（quick_input / ptt / toggle_main_window /
                toggle_overlay / toggle_overlay_lock / emergency_hide / analyze_screen /
                stop_speaking）→ agent bindings / wailsruntime
```

- `SetShortcut(action, combo)`：归一化（修饰键排序去重）→ 冲突检测（`ShortcutConflictError`）→ 持久化回调（main.go 注入，写 cfg + Save）→ 重注册
- `internal/input` 零 wails/config 依赖：驱动/持久化/事件广播全部回调或接口注入
- PTT 底层 combo 由 `shortcutMgr.PTTCombo()` 下发 `ptt.SetPTTCombo` —— 键值不存在第二处硬编码
- 旧 Win32 RegisterHotKey 注册路径已删除，全部统一走 v3 GlobalShortcut
- `cycle_voice_mode` action 已于 Phase 6 移除（互斥 VoiceMode 不存在，CycleMode 语义不成立）；旧配置中残留的 `cycle_voice_mode` 键被 ApplyAll 安全忽略

### Storage

本地存储：
- SQLite 存储对话历史 [PLANNED]
- SQLite 存储配置 [PLANNED]
- 缓存游戏知识库 [PLANNED]

**依赖**: 无特殊，底层依赖 database/sql

### 基础设施（已完成）

| 模块 | 状态 | 位置 |
|------|------|------|
| Config 管理 | ✓ Done | `internal/config` |
| Log 系统 | ✓ Done | `internal/log` |
| 统一错误处理 | ✓ Done | `internal/errors` |
| 生命周期管理 | ✓ Done | `internal/lifecycle` |
| 平台抽象层 | ✓ Done | `internal/platform` |
| 系统托盘 | ✓ Done | `internal/tray` |
| Input（热键+快速输入+PTT） | ✓ Done | `internal/input` |
| LLM Provider（OpenAI 兼容） | ✓ Done | `internal/llm` |
| Agent Core | ✓ Done | `internal/agent` |
| Screenshot（GDI 全屏） | ✓ Done | `internal/screenshot` |
| Tool 系统 | ✓ Done | `internal/tool` |
| Game 插件系统（TFT） | ✓ Done Phase 1 | `internal/game` |
| Audio Capture | ✓ Done | `internal/audio` |
| Speech Recognition | ✓ Done | `internal/speech` |
| PTT Manager | ✓ Done | `internal/input/ptt` |
| 设置 UI（LLM + ASR 配置） | ✓ Done | `frontend/src/components/Settings.vue` |
| 语音指示器 | ✓ Done | `frontend/src/components/VoiceIndicator.vue` |
| 前端（Vue 3） | ✓ Done | `frontend/` |

## 数据流示例

### 语音唤醒 + 提问流程

```
User: "Hey Aurora, 这关怎么过?"
 ↓
openWakeWord (本地检测到唤醒词)
 ↓
InputHandler 通知 Agent Core 唤醒成功
 ↓
Agent Core 启动 ASR Provider 开始流式识别
 ↓
ASR 返回识别文本
 ↓
Agent Core 调用 LLM Provider，带上上下文
 ↓
LLM 返回回答（流式）
 ↓
Agent Core 把回答传给 UI 显示
 ↓
Agent Core 调用 TTS Provider 播放语音
 ↓
User 听到回答，交互完成
```

### 快捷键截图提问流程

```
User 按下快捷键 → 截图提问
 ↓
InputHandler → Agent Core
 ↓
Agent Core 调用 Screenshot Tool → 截图
 ↓
Agent Core 调用 LLM（带截图）
 ↓
LLM 分析回答 → UI 显示 + TTS
```
