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
│  - Global shortcut listener  [PLANNED]                     │
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

### Input System [PLANNED]

负责接收用户输入：
- 全局快捷键监听
- 唤醒词检测 (openWakeWord)
- PTT 按键处理
- 语音激活检测 (VAD)

**依赖**: 无特殊依赖，可直接调用系统 API

### Agent Core [PLANNED]

核心调度层：
- 对话状态管理
- 上下文窗口管理
- 工具调用编排
- 多轮对话管理

**依赖**:
- Provider 接口
- Tool 接口
- GamePlugin 接口
- config, log, errors, storage

**不依赖**:
- 具体 Provider 实现
- 具体 GamePlugin 实现
- UI 层

### LLM Provider [INTERFACE]

大语言模型抽象：
- 统一 ChatCompletion 接口
- 流式输出支持
- 工具调用支持

**当前计划支持**:
- 火山引擎豆包
- OpenAI 兼容接口

**实现**依赖 Agent Core 定义的接口，Agent Core 不依赖具体实现。

### Speech Recognition Provider [INTERFACE]

语音识别抽象：
- 流式识别接口
- 一键识别接口

**分工**:
- openWakeWord: 本地唤醒词检测（必须）
- Provider: 云端 ASR（唤醒后才启动）

### TTS Provider [INTERFACE]

语音合成抽象：
- 文本转语音接口
- 流式播放支持

### Screenshot Provider [INTERFACE] [PLANNED]

截图抽象：
- 截取指定窗口/全屏
- 返回图像数据
- 坐标转换

### Tool System [PLANNED]

工具执行系统：
- 定义 Tool 接口
- 工具注册机制
- AI 调用工具执行
- 结果返回给 AI

Available tools:
- ScreenshotTool
- GameStateQueryTool
- SearchKnowledgeTool

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

### UI (Wails)

前端展示层：
- 主窗口
- 设置界面
- 系统托盘
- 悬浮球

**依赖**:
- 通过 bindings 调用 Go 后端接口
- 不直接依赖内部模块

### Storage

本地存储：
- SQLite 存储对话历史
- SQLite 存储配置
- 缓存游戏知识库

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
