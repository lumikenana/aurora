# GameAI - 架构文档

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

### 允许的依赖方向

```
main → app → internal/* → platform-specific
UI ← Wails ← Go bindings → Agent Core
Agent Core depends on:
  - Provider interfaces (not concrete implementations)
  - Tool interfaces
  - GamePlugin interfaces

Concrete implementations:
  - LLM providers → depend on Agent Core interfaces
  - ASR/TTS providers → depend on Agent Core interfaces
  - GamePlugins → depend on GameProvider interface
```

### 禁止的依赖

- ❌ UI 直接调用 Agent Core 内部逻辑（必须通过 Wails bindings）
- ❌ Agent Core 直接依赖具体 LLM 厂商实现（必须依赖 Provider 接口）
- ❌ Agent Core 直接依赖具体游戏实现（必须依赖 GamePlugin 接口）
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

### TTS Provider [INTERFACE]

语音合成抽象：
- 文本转语音接口
- 流式播放支持

### Screenshot Provider [DONE (Windows GDI)]

截图抽象：
- 截取全屏 ✓（GDI: GetDC + BitBlt + GetDIBits，返回 PNG base64）
- 截取指定窗口 [PLANNED]
- 坐标转换 [PLANNED]

### Tool System [DONE]

工具执行系统：
- 定义 Tool 接口（`internal/tool/tool.go`）
- 工具注册机制（`ToolRegistry`，工厂 + init 自注册）
- AI 调用工具执行（Agent Loop：`ChatWithTools`，最多 5 次调用，30s 超时）
- 结果返回给 AI，追加到对话上下文

Available tools:
- `capture_screenshot` - ScreenshotTool（全屏截图）
- `get_current_time` - CurrentTimeTool（当前时间）

### Game Plugin System [INTERFACE] [PLANNED]

游戏支持抽象：
- 每个游戏实现一个 GamePlugin
- 提供游戏知识
- 理解游戏状态
- 提供策略建议

Plugin interface:
- Name()
- DetectGame()
- GetGameContext()
- AnalyzeScreenshot()

### UI (Wails + Vue 3)

前端展示层（`frontend/`，Vue 3 + Vite + TypeScript）：
- 主窗口（对话 / 日志面板）✓
- 快速输入弹出层（QuickInput 组件）✓
- 设置界面（Settings.vue，LLM 配置 provider/model/baseURL/apiKey + 热加载）✓
- 系统托盘（Go 侧实现）✓
- 悬浮球 [PLANNED]

**依赖**:
- 通过 `src/api/backend.ts` 封装调用 Wails bindings
- 通过 `src/api/events.ts` 订阅 Go 事件（llmChunk / llmFinish / log / showQuickInput）
- 不直接依赖内部模块

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
| Audio Capture | ✓ Done | `internal/audio` |
| Speech Recognition | ✓ Done | `internal/speech` |
| PTT Manager | ✓ Done | `internal/input/ptt` |
| 设置 UI（LLM + ASR 配置） | ✓ Done | `frontend/src/components/Settings.vue` |
| 语音指示器 | ✓ Done | `frontend/src/components/VoiceIndicator.vue` |
| 前端（Vue 3） | ✓ Done | `frontend/` |

## 数据流示例

### 语音唤醒 + 提问流程

```
User: "Hey GameAI, 这关怎么过?"
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
