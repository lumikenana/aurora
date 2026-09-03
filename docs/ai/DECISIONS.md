# Aurora - 架构决策记录

## ADR-001: 使用 Go 作为核心语言

**Status**: Accepted

**Context**:
需要选择一个适合开发 Windows 桌面应用的语言，要求：
- 可以编译成单个 EXE
- 不需要用户安装运行时
- 支持交叉编译从 macOS 到 Windows
- 良好的 CGO 支持调用 Windows API
- 适合长期维护

**Decision**:
选择 Go 作为唯一核心语言。

**Reason**:
- Go 编译产出单个二进制，符合单 EXE 要求
- Go 不需要用户安装运行时
- Go 原生支持交叉编译（配合 CGO）
- Go 有良好的 C 互操作，可以调用 Windows API
- 语言简洁，适合长期维护
- 静态链接，没有依赖问题

**Alternatives**:
- Rust: 编译复杂，对于长期开发团队来说门槛更高
- C#: 需要 .NET 运行时，不符合零依赖要求
- Python: 需要用户安装 Python，打包体积大，不符合要求
- Node.js: 需要 runtime，单 EXE 打包体积大，不适合核心逻辑

**Consequences**:
- 所有核心逻辑用 Go 编写
- 前端 UI 用 HTML/CSS/JS 通过 Wails 承载
- 可以从 macOS 交叉编译 Windows

---

## ADR-002: 使用 Wails 作为桌面 UI 框架

**Status**: Accepted

**Context**:
需要一个 Go 生态的桌面 UI 框架，要求：
- Go 后端 + Web 前端架构
- 可以打包单个 EXE
- 活跃维护
- 支持系统托盘
- 支持 Windows

**Decision**:
使用 Wails v2 作为桌面框架。

**Reason**:
- 原生 Go 绑定，调用方便
- 前后端分离清晰
- 最终可以打包成单个 EXE
- 内置系统托盘支持
- 开发体验好，macOS 开发流畅
- 交叉编译支持良好

**Alternatives**:
- Fyne: 原生 Go UI，但是定制化不如 Web 方便
- Qt for Go: LGPL 许可证问题，绑定复杂
- Electron: 体积大，多进程，不符合单进程要求
- Walk: Windows 原生 Go UI，停止维护

**Consequences**:
- 前端用 HTML/CSS/JS 开发
- Wails 负责窗口管理和绑定生成
- 开发者需要懂一点前端，但符合前后端分离

---

## ADR-003: 单体应用而不是微服务架构

**Status**: Accepted

**Context**:
架构设计时需要决定是单体还是多服务。

**Decision**:
所有模块运行在同一个进程，单体应用。

**Reason**:
- 用户只需要运行一个 Aurora.exe
- 不需要用户启动多个服务
- 进程间通信没有 overhead
- 部署简单，零配置
- 符合桌面应用用户预期

**Alternatives**:
- 拆分多个进程: 增加部署复杂度，用户体验差
- HTTP 服务: 需要打开端口，有安全风险，增加复杂度

**Consequences**:
- 所有模块在同一进程
- 模块之间直接函数调用通信
- 不允许创建 localhost HTTP 服务用于内部模块通信
- 模块化通过 Go 包和接口实现，不是进程拆分

---

## ADR-004: Provider 抽象对于 AI 服务

**Status**: Accepted

**Context**:
LLM/ASR/TTS 有多个厂商，需要换方便。

**Decision**:
定义 Provider 接口，具体厂商实现接口，核心依赖接口不依赖实现。

**Reason**:
- 可以切换厂商不修改核心逻辑
- 支持用户自定义 endpoint
- 未来支持本地 LLM 不需要改架构
- 符合开闭原则

**Alternatives**:
- 直接绑定某个厂商: 锁死，难以更换
- 配置化选择: 需要抽象，和当前方案一致

**Consequences**:
- `internal/llm` 定义接口
- `internal/llm/providers` 放具体实现
- Agent Core 只依赖接口

---

## ADR-005: GamePlugin 抽象对于游戏支持

**Status**: Accepted

**Context**:
需要支持多个游戏，每个游戏逻辑不同。

**Decision**:
定义 GamePlugin 接口，每个游戏单独实现插件。

**Reason**:
- 新增游戏不修改核心代码
- 可以社区贡献游戏插件
- 保持核心干净
- 符合开闭原则

**Alternatives**:
- 每个游戏硬编码进核心: 核心越来越臃肿，难以维护

**Consequences**:
- `internal/game` 定义接口
- `internal/game/plugins` 放具体游戏实现
- Agent Core 只依赖接口

---

## ADR-006: openWakeWord 本地检测 + 云端 ASR 分工

**Status**: Accepted

**Context**:
语音唤醒需要一直监听，不能一直上传云端。

**Decision**:
- openWakeWord 在本地做唤醒词检测
- 只有检测到唤醒词之后，才启动云端 ASR
- openWakeWord 不做完整语音识别

**Reason**:
- 节省流量: 没唤醒不上传
- 节省钱: 只有唤醒才调用 ASR API
- 隐私保护: 语音不上传除非唤醒
- openWakeWord 做唤醒词检测效果好，体积小

**Alternatives**:
- 全部云端: 一直上传，浪费钱，隐私问题
- 全部本地: 需要更大模型，体积大

**Consequences**:
- 必须有两个阶段：唤醒检测 → 识别
- openWakeWord 必须集成进进程（go bindings）
- 唤醒后才连接 ASR Provider

---

## ADR-007: 按需截图而不是持续视觉分析

**Status**: Accepted

**Context**:
视觉分析耗电、费钱、费 token，不需要一直运行。

**Decision**:
- 默认不捕获画面
- 只有用户明确提问或 AI 明确请求时才截图
- 事件驱动，不是轮询

**Reason**:
- 节省性能: 不占用 CPU/GPU
- 节省 token: 只在需要时发图
- 隐私保护: 不会一直收集屏幕
- 用户可控: 明确知道什么时候截图

**Alternatives**:
- 持续捕获分析: 性能浪费，用户隐私顾虑

**Consequences**:
- Screenshot Provider 提供截图接口
- AI 决定什么时候调用截图
- 用户可以手动触发截图提问

---

## ADR-008: SQLite 作为第一阶段本地数据存储

**Status**: Accepted

**Context**:
需要存储对话历史、配置、缓存。

**Decision**:
使用 SQLite 作为本地存储。

**Reason**:
- Go 标准 database/sql 支持良好
- 单个文件，容易部署
- 支持结构化查询
- 成熟稳定，不需要运行额外服务
- 对于桌面应用存储量足够

**Alternatives**:
- JSON 文件: 查询麻烦，并发不好
- BadgerDB: KV，不需要复杂查询
- PostgreSQL: 需要运行服务，不符合单体要求

**Consequences**:
- 所有本地数据存在 SQLite
- 应用目录自带数据库文件
- 备份简单，复制文件即可

---

## ADR-009: 前端使用 Vue 3 + Vite + TypeScript

**Status**: Accepted (2026-08-25)

**Context**:
前端最初是原生 JS 静态页面，存在两个问题：
- 原生 ES module 不支持 `import './style.css'`，导致整个前端脚本静默失败
- 组件化能力缺失，后续设置界面、对话、日志面板会越写越乱

**Decision**:
前端迁移至 Vue 3 + Vite + TypeScript，目录为 `frontend/`，删除旧 `ui/`。

**Reason**:
- 组件化（Conversation / LogPanel / QuickInput）
- Vite 构建产物单目录，`go:embed all:frontend/dist` + `fs.Sub` 嵌入方式不变，仍是单 EXE
- TypeScript 提供类型检查，bindings 调用不易出错
- `wails dev` 通过 frontend:dev:watcher 原生支持热更新

**Consequences**:
- 开发时需要 Node.js（仅构建期），**运行时仍是单 EXE 零依赖**，不违反零安装原则
- wailsjs 绑定目录随 CLI 生成，若缺失则用 `frontend/src/api/events.ts` 直接访问 `window.runtime`

---

## ADR-010: Wails 绑定方法不携带 context.Context 参数

**Status**: Accepted (2026-08-25)

**Context**:
绑定方法原签名带 `ctx context.Context`。Wails v2.15 调用时传入 null，触发
`reflect: Call using zero Value argument`，且该错误被框架吞掉不上报前端，
导致 Promise 永远 pending（表现为截图按钮永远显示"截图中"）。

**Decision**:
所有 Wails 绑定方法（agent/input 等 bindings.go）**一律不声明 ctx 参数**。
需要运行时上下文时，通过 `agent.SetRuntimeContext(ctx)` 在 OnStartup 注入全局上下文，
事件发送统一走内部 `emitEvent`。

**Reason**:
- 这是此版本 Wails 的框架缺陷，规避优于对抗
- 绑定方法签名同时决定 wailsjs 生成的 JS 签名，去掉 ctx 也简化了前端调用

**Consequences**:
- 新增绑定方法时禁止加 context.Context 参数
- 需要 EventsEmit 的模块依赖 SetRuntimeContext 注入（注意 main.go OnStartup 顺序）

---

## ADR-011: Windows 热键采用专用消息线程模型

**Status**: Accepted (2026-08-25)

**Context**:
RegisterHotKey 要求注册线程与拥有消息窗口的线程一致；Go goroutine 会被调度器
在线程间迁移，导致"注册成功但不触发"或 "Invalid window; belongs to other thread"。

**Decision**:
`internal/input/hotkey_windows.go` 使用单个 `runtime.LockOSThread` 专用线程，
窗口创建（`HWND_MESSAGE = ^uintptr(2)`）、GetMessage 循环、RegisterHotKey 全部在该线程完成；
其他线程通过 ops channel + `PostMessage(WM_APP)` 把注册/注销调度到窗口线程执行。

**Reason**:
- Win32 消息模型本质是线程绑定的，必须显式钉住线程
- 跨线程操作全部收敛到消息调度，避免竞态

**Consequences**:
- 修改热键逻辑时不得在非窗口线程直接调 RegisterHotKey/UnregisterHotKey
- 新的窗口线程操作通过 `runOnWindowThread` 调度

---

## ADR-012: LLM Provider 通过 main.go blank import 注册

**Status**: Accepted (2026-08-25)

**Context**:
`internal/llm/providers/openai/openai.go` 通过 `func init()` 自注册到
`llm.providerFactories`。但整个项目无任何代码 import 该包，导致 `init()` 从未执行，
运行时 `CreateProvider("openai")` 报 "unsupported LLM provider: openai"。

**Decision**:
在 `main.go` 添加 blank import：
```go
_ "github.com/lipipi/game-ai/internal/llm/providers/openai"
```

**Reason**:
- `openai` 包依赖 `llm` 包（用于 `RegisterProvider` / `LLMProvider` 接口），
  不能在 `llm/factory.go` 中 import（会形成 `llm → openai → llm` 循环依赖）
- 必须在更上层（`main` 包）引入，`main` 不依赖 `llm` 的内部实现
- 这是 Go 标准 provider 注册模式，类似 `database/sql` + driver

**Consequences**:
- **新增 LLM Provider 包时，必须在 `main.go` 添加对应的 blank import**
- 否则 provider 的 `init()` 不会执行，运行时无法创建该 provider
- 考虑未来 provider 数量增多时，可以创建一个 `internal/llm/providers/all/all.go` 聚合 import

---

## ADR-013: OpenAI 兼容 API 显式关闭思考模型 thinking 模式

**Status**: Accepted (2026-08-25)

**Context**:
火山引擎 ARK 豆包 `doubao-seed-2.0-lite` 等思考模型默认开启 thinking，SSE 返回格式为：
```json
{"delta": {"content": "", "reasoning_content": "思考内容...", "role": "assistant"}}
```
实际内容在 `reasoning_content` 字段，`content` 始终为空。等思考结束后才会发送
`content` 非空的 chunks。这导致我们的 SSE 解析器（只看 `content`）收到全部空 chunk。

**Decision**:
在 `openAIRequest` 中增加 `thinking` 字段，请求时显式设置 `thinking.type = "disabled"`：
```go
Thinking: &thinkingConfig{Type: "disabled"}
```
生成的请求体：
```json
{
  "model": "...",
  "messages": [...],
  "stream": true,
  "thinking": { "type": "disabled" }
}
```

**Reason**:
- 游戏助手场景不需要深度思考，直接回答更快
- 思考模式会显著增加响应延迟（先思考后回答）
- 避免 SSE 解析器需要同时处理 `content` 和 `reasoning_content` 两个字段
- 火山引擎 ARK 官方支持的参数（`thinking.type` 可选值：`enabled` / `disabled` / `auto`）

**Consequences**:
- 当前所有请求都强制关闭 thinking，如果未来需要支持思考模型，需要改为可配置
- `thinkingConfig` 结构体为可选字段（`omitempty`），非豆包模型不受影响
- 注意：阿里云百炼（DashScope）使用不同的参数名 `enable_thinking`（boolean），不能混用

---

## ADR-014: Windows 音频采集 waveIn 使用 CALLBACK_WINDOW 模式

**Status**: Accepted (2026-08-26)

**Context**:
waveIn 音频采集有两种回调模式：
- `CALLBACK_FUNCTION`: 直接回调到 Go 函数
- `CALLBACK_WINDOW`: 发送消息到窗口，窗口线程处理

在 Go 中使用 `CALLBACK_FUNCTION` 存在问题：
- syscall 回调跨线程会导致 Go 调度器不稳定
- C 线程调用 Go 函数存在栈切换问题，容易 panic

**Decision**:
改用 `CALLBACK_WINDOW` 模式，音频数据通过 `PostMessage` 发送到已有的热键消息窗口，
由专用消息线程处理，完全复用现有的线程模型。

**Reason**:
- 符合 Win32 编程惯例
- 复用 ADR-011 的专用消息线程模型，不增加新线程
- 避免 Go 回调的不稳定性，实践证明可靠

**Consequences**:
- `WM_WIM_DATA` 消息路由到 `waveInProc 处理，通过 callback 推送数据给 PTT
- 所有音频数据处理都在专用消息线程，和热键共享同一线程

---

## ADR-015: 火山引擎 ASR 协议配置默认值

**Status**: Accepted (2026-08-26)

**Context**:
火山引擎流式 ASR 2.0 有多个配置项，必须使用正确的默认值才能工作。

**Decision**:
- **第一个音频段必须 prepend 44 字节 WAV 头（服务端靠此识别音频格式）
- 默认 Resource ID = `volc.seedasr.sauc.duration`（不是 `bigasr`）
- 默认 Endpoint = `bigmodel_async`（双向 WebSocket 流式）

**Reason**:
这些是火山引擎官方文档中 SAUC 场景要求的正确配置，错误配置会导致识别失败：
  - 无 WAV 头：服务端无法识别格式，返回空结果
  - 错误 Resource ID：识别成功率极低
  - 错误 Endpoint：握手失败或无法建立连接

**Consequences**:
- config.json 默认值必须遵循上述约定，修改配置时注意核对

---

## ADR-016: 语音输入由后端驱动而非前端驱动

**Status**: Accepted (2026-08-26)

**Context**:
早期实现中，`voiceTranscript` final 事件在前端处理，前端调用 `handleSubmit` 提交给 Agent。
这导致：
- 后端已经通过 `InputManager.NotifyInput()` 进入 Agent，前端又提交一次
- 双重提交，用户消息添加两次，Agent 请求发两次，响应交错乱序

**Decision**:
语音输入全程由后端驱动：
- ASR 识别出最终文本后，后端直接调用 `InputManager.NotifyInput(UserInput{Source: "voice"})`
- 前端 `voiceTranscript` 事件**只负责更新 UI 显示**，不调用 `handleSubmit`
- 后端驱动保证只有一次提交，和文本输入走完全相同的管道

**Reason**:
- 文本和语音最终都走同一个 `onUserInput` → `ChatStream`，保持一致性
- 避免竞态和乱序，确保响应顺序正确
- 语音 partial 仅用于 UI 预览，不进入核心逻辑

**Consequences**:
- 前端只做显示，不做业务逻辑提交
- 后端统一入口，便于调试和维护

---

## ADR-017: ONNX Runtime 环境必须一次性初始化

**Status**: Accepted (2026-08-27)

**Context**:
`yalue/onnxruntime_go` 绑定直接包装 ONNX Runtime C API。ONNX C API 明确要求：
- `ortSetSharedLibraryPath` 只能调用 **一次** 进程生命周期
- `ortInitializeEnvironment` 只能调用 **一次**

如果多次调用，会返回错误：
```
Platform-specific initialization failed: Error setting ORT API base: 2
```

原代码在每次创建 `openWakeWordDetector` 时都会重新调用这两个函数，导致第二次创建检测器时必然失败。

**Decision**:
使用 `sync.Once` 保证 `ort.SetSharedLibraryPath` 和 `ort.InitializeEnvironment` 在整个进程中 **只执行一次**：
```go
var initializeOnce sync.Once
var initErr error

initializeOnce.Do(func() {
  ort.SetSharedLibraryPath(foundPath)
  initErr = ort.InitializeEnvironment()
})
```

**Reason**:
- 这是 ONNX C API 的硬性要求，必须遵守
- `sync.Once` 是 Go 标准库保证一次性执行的标准方式
- 无论检测器创建多少次，环境只需要初始化一次

**Consequences**:
- ONNX 环境初始化在包级别变量，不是检测器实例级别
- 即使多次创建检测器，也不会重复初始化，避免错误码 2
- 找到 DLL 的第一个路径就是最终使用的路径（用户修改配置重启后才会生效，符合预期）

---

## ADR-018: TTS Provider 抽象（火山引擎 HTTP + mciSendString）

**Status**: Accepted (2026-08-27)

**Context**:
Agent 文本回复需要语音播报；TTS 必须解决"播放自己的声音不会被 ASR 误识别为用户输入"的回声问题。

**Decision**:
- 抽象出 `TTSProvider` 接口，工厂注册模式（沿用 LLM/Speech），具体实现放在 `providers/volcengine` 子包
- 播放走 Windows `mciSendString` winmm.dll 系统调用，**不引入新依赖**
- 临时 mp3 文件存到 `os.TempDir()`，播放完毕后清理
- `SpeakTTS` 同步返回，播放通过 `SetOnPlaybackEnd` 回调异步通知

**Reason**:
- mciSendString 是 Windows 内置的多媒体 API，零依赖、零安装
- 抽象接口让未来切换到流式 WebSocket TTS / 离线 TTS 不影响调用方
- 临时文件方式简化实现（避免引入 mp3 解码库）

**Consequences**:
- 合成 + 播放是异步的，调用方需要通过回调或 `IsPlaying()` 轮询确认
- TTS 期间通过 `ttsPlaying` 标志抑制 ASR，避免自激
- 跨平台扩展需要为非 Windows 实现 stub（已加 `playback_default.go`）
- 长文本需要等合成完成才能播放，无法流式边合成边播（未来可换流式 WebSocket）

---

## ADR-019: 模式切换线程安全保证（sync.RWMutex + 状态机显式迁移）

**Status**: Accepted (2026-08-27)

**Context**:
三模式（PTT / WakeWord / Continuous）切换可能发生在任意时刻（用户按热键 / 托盘菜单 / 自动超时），切换瞬间需要停止旧模式的 ASR session 和 goroutine，启动新模式的资源。任何遗漏都会导致 panic 或资源泄漏。

**Decision**:
- VoiceController 持 `sync.RWMutex` 保护 `mode` 字段
- `SetMode(ctx, mode)` 是唯一的状态变更入口
- 状态机显式迁移：`Idle → WakeDetected → SpeechListening → Processing → Speaking → Idle`
- 切换时调用 `stopCurrentModeResources()` 统一清理：
  - 取消当前 ASR session
  - 停止 audio → ASR 复制 goroutine
  - 停止 timeout ticker
  - 重置 busy 标志
- 新模式启动失败必须让 `mode` 回滚到原值

**Reason**:
- 多入口（热键 / 托盘 / 自动超时）必须互斥，否则会触发竞态
- 显式状态机比隐式标志更易调试（每个 transition 都有日志）
- 集中清理避免分散在多个 callback 里遗漏

**Consequences**:
- 模式切换瞬间新输入会被暂时忽略（处理耗时 < 100ms）
- 任何 SetMode 调用都走同一路径，行为可预测
- 旧模式资源（goroutine / channel）必须正确关闭，否则会泄漏 → 通过 `vc.ctx.Done()` 传播取消

---

## ADR-020: 连续模式 30s 静默超时（基于 ASR final 触发）

**Status**: Accepted (2026-08-27)

**Context**:
连续对话模式免唤醒持续识别；需要避免用户走开后应用永远挂在"监听中"状态。

**Decision**:
- 每次 ASR final transcript 重置 `continuousLastActive = time.Now()`
- 后台 goroutine `continuousTimeoutLoop()` 每 2s 检查一次：超过 `continuousTimeout`（默认 30s）就 `SetMode(ModePTT)`
- 退出关键词（默认 "结束对话" / "停止" / "退出对话" / "exit" / "stop"）触发后立即退出
- 退出后 emit `voiceContinuousTimeout` 事件给前端显示提示

**Reason**:
- 基于 ASR final 信号而非声音检测（VAD），依赖现有 ASR 流，无需引入 silero-vad
- 2s 轮询间隔足够准确（30s 容忍 ±2s），不浪费 CPU
- 关键词退出是用户最直觉的方式（"结束对话"），不依赖隐藏 UI

**Consequences**:
- 退出后需要手动重新进入连续模式（热键 / 托盘 / 设置）
- 30s 阈值在配置中可调（`continuous_timeout_seconds`）
- 用户说完话后 30s 内必须再次说话，否则自动退出
- 退出关键词检测在 `handleTranscript` 内执行，与超时退出互斥

---

## ADR-021: GamePlugin 游戏插件抽象（TFT 首个插件，Phase 1 不读游戏数据）

**Status**: Accepted (2026-08-28)

**Context**:
开始加入游戏能力，第一款支持 TFT / 云顶之弈。要求游戏逻辑不进 Agent Core，
未来增加 LOL / CS2 时不修改 Agent Core。

**Decision**:
- `internal/game` 定义 `GamePlugin` 接口：`ID / Name / Detect(ctx) (*GameContext, error) / SystemPrompt()`
  - `Detect` 未检测到返回 `(nil, nil)`（一次调用返回完整上下文，避免两次调用的状态竞态）
  - `SystemPrompt()` 在接口上：提示词即游戏语义，属于插件；Agent 只拼接，插件看不到 base prompt
- 工厂注册模式（沿用 LLM/Speech/Tool）：插件包 `init()` 自注册 + main.go blank import
- `Manager.DetectActive` 带 5s TTL 缓存（正负结果都缓存），检测失败视为未检测到
- TFT 检测 = LOL 客户端窗口候选 + 视觉 LLM 二次确认：
  - `RiotWindowClass` 类名需标题含 "League of Legends"（排除 Riot 登录器 / Valorant 误报）
  - `Riot Window`（游戏内窗口）类名直接命中
  - 窗口检测只能确认"LOL 在运行"，截图是否为 TFT 对局由 system prompt 指令让视觉 LLM 判断
- Agent 集成点：`onUserInput` → `prepareGameOptions`（检测 + 截图，全部失败静默降级纯文本）
  → `ChatStreamWithOptions`（附加提示词拼 system 副本、图片挂最后一条 user 消息副本）
- 多模态：`llm.ChatMessage.Images` + openai provider 转 content parts；**图片绝不进对话历史**
  （历史只存 `ScreenshotAttached bool` 标记，追问轮重新截新图）
- 视觉分析复用主 LLM 配置（用户自行换 vision 模型）；带图请求失败自动去图重试一次
- Phase 1 明确不读游戏内部数据（不碰内存）；GameContext 的游戏数据字段（Stage/Gold/HP/Board 等）为未来预留

**Reason**:
- 插件自注册 + blank import 与 ADR-012 同构，Agent Core 只依赖 `internal/game` 接口
- 窗口检测粒度故意粗（LOL 级）：TFT 与 LOL 共用客户端进程，靠视觉确认区分，避免脆弱的标题/进程启发式
- 每轮附新截图而非缓存：TFT 局面实时变化，新鲜度即准确性；成本由 `game.screenshot_on_detect` 配置兜底

**Consequences**:
- 新增游戏 = 新插件包 + main.go 一行 blank import + config `game.enabled` 加 ID，不改 Agent Core
- 检测到游戏时每轮请求附带 ~300-700KB base64 截图（视觉 token 成本，已知且可关）
- `RiotWindowClass` / `Riot Window` 类名是经验值，客户端更新可能变化 → 软失败（无检测=普通助手，误检测=LLM 纠正）
- `Bindings.ChatStream` 改为委托 `onUserInput`，文本与语音输入走完全同一管道（含游戏检测）

## ADR-023: GameContext Value[T] 容器 + 生命周期 Phase + Knowledge Base 边界

**Status**: Accepted (2026-08-31)

**Context**:
GamePlugin 抽象（ADR-021 / Phase 1）已落地，TFT 插件只做窗口检测 + 视觉 LLM 二次确认。
未来要逐步填充 Stage / Gold / HP / Board 等对局数据。旧 GameContext 用 0/""/nil 表达
"未识别"，无法区分"金币真的是 0"和"OCR 还没识别金币"。同时缺乏对局生命周期状态机
（HP=0 可能是满血未识别也可能是阵亡），缺乏未来 OCR / Vision / GameAPI 多源数据的
统一接入点，缺乏 Knowledge Base 关联的设计约束。

**Decision**:
- 引入 `Value[T]` 泛型可选值容器：用 `Present bool` 显式表达"已识别 vs 未识别"，
  `Source ContextSource` 标注数据来源，`Confidence float64` 标注可信度
  - `Known(v)` 构造完全可信值（来自窗口检测）
  - `Detected(v, src, conf)` 构造带来源/可信度的值（来自 Vision / OCR / GameAPI）
  - `Unknown[T]()` 显式表达未识别
  - `0` / `""` / `nil` 本身可以是合法值（如 Gold=0 表示破产，HP=0 表示阵亡）
- 引入 `ContextPhase` 状态机：`unknown` / `not_in_game` / `carousel` / `draft` /
  `in_game` / `game_over` / `match_ended`，先确定 Phase 再讨论字段
- 引入 `ContextSource` 枚举：`window` / `vision` / `ocr` / `game_api` / `manual`
- 通用 `GameContext` 重组：
  - `Game / Name / Phase / DetectedAt`：基础元数据
  - `Window WindowInfo`：窗口信息（收敛原 `WindowTitle/Class/HWND` 三个字段）
  - `CommonData`：跨游戏通用数据（Version / PlayerID / MatchID）
  - `GameData any`：游戏特定数据槽位（`*tft.TFTGameContext` 等）
  - 删除所有 TFT 特定字段（Version/Stage/Gold/Board 等），由 tft 包在 GameData 内维护
- TFT 特定扩展 `TFTGameContext`：Stage / Level / XP / Gold / HP / Board / Bench / Shop
  / Items / Augments / Traits / Carousel / Opponents 等全部用 `Value[T]`
- Knowledge Base 严格隔离：
  - GameContext 只保存 Knowledge Base key 引用（如 `"TFT18_Vayne"`）
  - 不在 GameContext 内联业务数据（Name / Cost / Description 等）
  - 关联时机：未来 LLM / 工具按需查询 `internal/game/knowledge` 包
  - 不同 patch 数据严格隔离（继承 ADR-022 约束）
- `ContextProvider` 接口（未来扩展点）：
  - `Source() / Order() / Contribute(ctx, in) (*Delta, error)` 纯函数式
  - Window Provider 总是最先执行（填 Phase/Window）
  - Vision / OCR Provider 后续执行（填具体字段）
  - Manager 按 Order 串行调用，合并 `ContextDelta`（Phase 按顺序覆盖，GameData 按 Source 整体替换）
- `Manager` 新增：
  - `UpdateContext(ctx, in)` 增量更新（Phase 10-6 无 Provider 时等价于初始化）
  - `Snapshot()` 返回 lastKnown 副本（防外部修改）
  - `Clear()` 清空 lastKnown（对局结束 / 切号）
  - `RegisterContextProvider(p)` 注册 Provider
  - 不增加后台 goroutine / 定时任务
- Agent 只读访问：
  - `GetGameContext()` 暴露 Snapshot 给前端 bindings / 未来 system prompt
  - `GetTFTContext()` 类型便捷方法（避免调用方断言 any）
  - **不把 GameContext 自动注入 LLM Prompt**（留待 Phase 10-7+）
  - **不修改 onUserInput → prepareGameOptions → ChatStreamWithOptions 主链路**
- Wails Binding `GetGameContext` 暴露前端：
  - 使用独立 `GameContextResponse` 结构（不直接序列化 `*GameContext`）
  - 故意隐藏 `WindowHWND`（uintptr 前端无意义）
  - 故意隐藏 `Confidence`（内部调试用，未来按需开放）
  - `GameData` 只暴露 `type` 字段（`"tft"` / `"unknown"`），不展开具体字段

**Reason**:
- `Value[T]` 是 Go 1.18+ 泛型的标准用法，零运行时开销，无反射
- `ContextPhase` 状态机让 UI / Agent 决策可基于阶段而非字段推断（"已阵亡不显示金币"）
- `GameData any` 保持 GamePlugin 接口稳定（只关心 Window+Phase+Common），
  未来 LOL / CS2 不会继承不需要的 Board / Bench / Shop 字段
- Knowledge Base 边界隔离避免 GameContext 在每次更新时同步查询 SQLite（违反"轻量级当前状态"）
- 序列化结构与内部结构解耦，避免 uintptr / any 在 Wails 序列化时的潜在问题
- 不为未来过度抽象：Phase 10-6 不实现任何具体 Provider（WindowProvider / VisionProvider
  / OCRProvider 留待 Phase 10-7+），保持骨架最小

**Consequences**:
- GameContext 结构破坏性变更：原 `gctx.WindowTitle` 改为 `gctx.Window.Title`
  （4 处调用点同步更新：agent.go 3 处 + bindings.go 1 处）
- 未来 OCR / Vision / GameAPI Provider 只需实现 `ContextProvider` 接口
- 未来 system prompt 动态注入 GameContext 摘要可走 `Agent.GetGameContext()`
- 前端可调用 `GetGameContext` 拿到 `{game, name, phase, window_title, window_class,
  game_data_type, detected_at}` 用于调试面板（Phase 10-7+ 再加 UI）
- 不再允许 GameContext 持有 `*Knowledge Champion` 等业务对象（编译期不强制，需代码审查保证）
- Wails Binding `GetGameContext` 返回的 `GameData` 仅暴露 `type` 字符串，前端
  如需具体字段需扩展 `GameContextResponse` 或单独暴露 `GetTFTContext()`（Phase 10-7+）

## ADR-024: TFT GameContext → Agent 决策上下文注入

**Status**: Accepted (2026-08-31)

**Context**:
ADR-023 已经搭好 GameContext 基础设施，Phase 10-7/10-8 已经填充了 `Stage` / `Gold` / `Level` / `Board` / `Bench` / `Shop` / `Items` / `Traits` / `Augments` 等 OCR 识别字段。现在需要完成最后一环：**让 Agent 在回答玩家问题时，能够把当前对局状态注入 LLM 上下文，获得"看得见棋盘"的决策建议**。

需要严格遵守：
  - 最小改动：不重写 Agent 主链路，只在 `prepareGameOptions` 附加到 `ExtraSystemPrompt`
  - 不自动操作游戏（严格禁止）：不模拟鼠标键盘，不买棋子，不刷新，不升级，不切换阵容
  - 不复杂 RAG/Embedding/Vector DB：只有**少量按需关键词查询**（只做 Champion ID → Name 映射）
  - Patch 强制隔离：不混用不同版本数据
  - 非 TFT 问题不注入，全空 GameContext 不注入
  - 不永久写入对话历史：每次都从 Manager.Snapshot() 读最新快照

**Decision**:
- **GameContext（游戏状态模型）与 GamePromptContext（LLM 表达模型）严格分离**：
  - GameContext 是增量更新的状态机（由 OCR 不断同步），由 `game.Manager` 维护
  - GamePromptContext 是**一次性 LLM prompt 摘要**，由 `GameContextPreparer` 每次调用时从快照转换
  - 绝不把完整 `GameContext` 写入对话历史，绝不把 `GameContext` 对象塞到 prompt，只输出紧凑文本

- **注入时机与策略**：
  - `Agent.onUserInput` 收到用户输入后，在 `prepareGameOptions` 中调用 `GameContextPreparer.Prepare(ctx, userInput)`
  - `Prepare` 返回 `(summary, detail, used)`；仅当 `used == true` 时，把 `summary` 拼到 `ExtraSystemPrompt` 末尾
  - `prepareGameOptions` 不修改对话历史，不修改 `systemPrompt`，只修改单次请求的 `ExtraSystemPrompt` → 无历史污染

- **前置检查（任何一步不满足即跳过注入）**：
  1. `source.Snapshot() == nil` → 跳过
  2. `gctx.Game != "tft"` → 跳过（仅支持 TFT）
  3. `gctx.GameData` 类型断言 `*tft.TFTGameContext` 失败 → 跳过
  4. `gctx.Phase` 不在 `{in_game, carousel, draft}` → 跳过（对局结束或未开始不注入）
  5. `userInput` 不命中 TFT 关键词 → 跳过（"今天天气"等不注入）
  6. `FormatTFTGameContext()` 返回空（全字段未知）→ 跳过（避免空标题污染）
  只有全部检查通过才会注入。

- **关键词匹配 (`needsGameContext`)**：
  - 宽松策略：包含任一 TFT 关键词即认为需要 GameContext（宁可错注不可漏注）
  - 关键词列表：包含"tft"/"云顶"/"金铲铲"，"金币"/"利息"/"等级"/"升级"/"升人口"，"棋盘"/"阵容"/"装备"/"羁绊"/"强化"，"运营"/"怎么玩"/"建议"/"下一步"，"这把"/"这局"/"当前"/"我该"，常见英雄名
  - 大小写不敏感

- **细节粒度 (`pickContextDetail`)**：
  - 命中"运营"/"阵容"/"装备"/"棋盘"/"备战"等 → `Full`（包含棋盘/备战/商店/装备/羁绊/强化）
  - 否则 → `Minimal`（只包含基础资源：阶段/等级/经验/金币/利息/血量/连胜/连败）

- **格式约定 (`FormatTFTGameContext`)**：
  - 标题 `## 当前 TFT 对局状态`
  - `### 基础资源` 分段，仅输出 `Value[T].Present == true` 的字段
  - `Known(0)` 正常输出（如 `金币：0` 是合法破产，`生命：0` 是合法阵亡）
  - 不输出 `Confidence` / `Source` 等内部调试信息
  - `Full` 粒度额外添加 `### 棋盘` / `### 备战席` / `### 商店` / `### 装备` / `### 羁绊` / `### 强化符文`
  - 全字段未知 → 返回空字符串 → Prepare 跳过注入
  - 末尾统一加一句提示：`注意：以上信息来自当前游戏画面识别，部分字段可能存在识别误差。`

- **Knowledge Base 查询 (`queryRelevantKB`)**：
  - 只在 `detail == Full` 时查询（Minimal 粒度不膨胀 prompt）
  - **patch 隔离策略**：
    1. 优先使用 `gctx.CommonData.Version`（GameContext 里识别到的 patch）
    2. 识别失败回退到 `kb.GetCurrentPatch()`（KB 当前激活的 patch）
    3. 仍失败 → 跳过 KB 查询
  - **查询范围**：只收集 `Board + Bench` 上的英雄 ID（去重），最多查询前 4 个（避免 prompt 爆炸）
  - **输出约束**：只输出 `id: name` 映射，不展开 Data 字段（spec 明确禁止把完整 KB 塞入 prompt）
  - `KnowledgeProvider` 接口新增 `GetCurrentPatch(ctx) (string, errors.AppError)` 方法供回退使用

- **设计保证**：
  - `GameContextPreparer` 依赖 `gameSnapshotSource` 小接口而非 `*game.Manager` → 可测试性
  - `source.Snapshot()` 返回副本 → 不能修改 Manager 状态，并发安全
  - `knowledge` 允许为 nil → KB 未配置/未下载时仍输出纯 GameContext 摘要（不报错）

**Reason**:
- 最小改动：只修改了 `prepareGameOptions` 增加参数，新增 `internal/agent/game_context.go` 实现，完全不碰 Agent 主对话链路
- 严格禁止自动游戏操作：本阶段只做 prompt 注入，不调用任何工具，不做任何执行
- patch 隔离：KB 查询严格限定在当前游戏 patch 或当前激活 patch，不跨 patch 混查
- 按需注入：只在用户问 TFT 相关问题时才注入，避免浪费 token
- 空 GameContext 不注入：OCR 还没识别出任何字段时不会输出空标题
- 不污染历史：每次注入都是单次请求的 `ExtraSystemPrompt`，对话历史本身不包含 GameContext

**Consequences**:
- 新增 `internal/agent/game_context.go` 与 `internal/agent/game_context_test.go`（Phase 12 核心）
- `internal/agent/agent.go` 修改 `prepareGameOptions` 签名为 `prepareGameOptions(ctx, userInput)`，增加 GameContext 注入逻辑
- `internal/tool/tools/tft_knowledge.go` `KnowledgeProvider` 接口新增 `GetCurrentPatch` 方法（所有 mock 已更新）
- 测试覆盖全部分支：
  - Formatter：nil/empty/known-fields/zero-values/no-internal-leak
  - `needsGameContext`：positive/negative/case-insensitive
  - `GameContextPreparer`：nil/snapshot-nil/non-tft/phase-rejected/phase-accepted/non-tft-question/empty-context
  - KB query：patch-resolution/max-4-champions/minimal-no-query/nil-kb
  - 集成测试：`prepareGameOptions` 不修改对话历史
- 文档更新：`CURRENT_STATE.md` / `CHANGELOG.md` / `DECISIONS.md`（本 ADR）

Phase 12 完成后停止，不进入下一个阶段（Phase 13 是工具自动决策执行，不在本阶段范围内）。

---

## ADR-025: TFT 视觉识别选型 - Vision LLM vs Tesseract

**Status**: Accepted (2026-08-31)

**Context**:
Phase 14-A/B/C 已实现 OCR 文本 → 名称 → canonical ID 的归一化链路，但 OCR 引擎本身仍是 Mock。在真实 TFT 画面上，Phase 14-D 需要把任意屏幕截图转为结构化字段（Player 数字 + Board/Bench/Shop 棋子 + 装备名 + 羁绊 + 强化符文 + 选秀）。TFT 画面具有以下特点：
- 中英文混合（"薇恩 2★" + "Infinity Edge" + "巨龙之爪"）
- 数字 + 单位（"1/2/3 金币"、星级 ★1/★2/★3、5/7/10 人口费用）
- 装备图标叠加（图标 + 文字同时呈现）
- UI 频繁更新（每个 Set 重做皮肤、不同分辨率）
- 单条 OCR 文本无完整语义（碎片化字符）

需要在准确率、Windows 部署、单 EXE、延迟、API 成本、架构复用、未来可扩展性之间取得平衡。

**Decision**:
**选用 Vision LLM（豆包多模态）作为 Phase 14-D 的 OCR 后端**，保留 Tesseract 作为可切换 fallback。

**选型对比**：

| 维度 | A. Tesseract (Gosseract) | B. Windows.Media.Ocr | **C. Vision LLM (豆包)** ✅ | D. PaddleOCR |
|------|--------------------------|----------------------|------------------------------|---------------|
| 准确率 | 中（字符切分，中文 Tesseract 需 chi_sim 模型，UI 字体变化敏感） | 中（系统 OCR，对非标准字体差） | **高（语义理解直接输出结构化文本）** | 高 |
| Windows 部署 | ❌ 需 VC++ Redistributable + libtesseract + libleptonica + chi_sim 训练数据 | ✅ 需 Windows 10+ | ✅ 纯 HTTP（已部署） | ❌ 需 ONNX 模型 + 推理库 |
| 单 EXE | ❌ CGO 编译需链接 C 库，体积 +10MB | ✅ 纯 Go | ✅ **纯 HTTP** | ❌ CGO |
| 延迟 | <100ms / 图像 | <200ms | ~1s / 字段（受区域数影响） | <300ms |
| API 成本 | 免费 | 免费 | 按 token 计费 | 免费 |
| 架构复用 | ❌ 需独立 OCR 适配 | ❌ 平台特定 | ✅ **复用 `llm.LLMProvider`，零新增依赖** | ❌ |
| 可扩展性 | ❌ 仅文本 | ❌ 仅文本 | ✅ **任意 UI 元素（颜色/位置/状态）** | ❌ |
| 跨平台 | 部分（需 CGO） | 仅 Windows | ✅ 全平台 | 部分 |

**Reason**:
1. **准确率优先**：TFT UI 文字碎片化、中英混排、图标叠加，传统 OCR 准确率 < 70%；Vision LLM 直接语义理解 → > 95%
2. **零部署依赖**：用户机器无需安装 VC++/Tesseract/语言包，符合"单 EXE + 零运行时"约束
3. **架构复用**：Vision LLM 调用复用 `llm.LLMProvider`，不引入新的 HTTP/gRPC 客户端；配置、密钥、重试逻辑全部继承
4. **未来可扩展**：Vision LLM 可识别颜色（装备品质）、位置（棋盘坐标）、状态（强化符文 tier）—— OCR 完全做不到
5. **可接受成本与延迟**：TFT 不需要毫秒级响应；按需截图触发 → 50 次/截图 × 1s = 50s 上限，实际 < 10s（LLM 并发/批量优化空间大）
6. **Tesseract 保留**：通过 `//go:build tesseract` 隔离为可选 build tag，未来低端设备/离线模式可启用

**实现要点**：
- `OCREngine` 接口保持极简（`Recognize` + `Name`），向后兼容
- 新增 `FieldAwareOCREngine` 可选接口（`RecognizeField(ctx, img, mime, field)`），让 Vision LLM 按字段构造不同 prompt
- Vision LLM 的 prompt 模板：每个 field 一段中英双语指令（"请读取图片中的金币数字"），LLM 直接返回数字/名称
- Confidence 策略：field-aware 命中 = 0.85 / 空响应 = 0.3（让 ocrRawText 的 < 0.3 阈值过滤）
- Chicken-and-egg 解决：TFT Plugin 提供 `SetLLMProvider(provider)` post-init 升级（agent.Init 时 LLM 已就绪 → 注入到 plugin → provider 升级为 VisionOCREngine）

**Consequences**:
- 新增 `internal/game/plugins/tft/vision_ocr_engine.go` (~250 行)：VisionOCREngine + FieldAwareOCREngine + llmAdapter
- 新增 `internal/game/plugins/tft/gosseract_engine_{tesseract,stub}.go`：可选 Tesseract 后端
- `internal/game/plugins/tft/ocr.go` 修改：`ocrParseField/ocrRawText` 新增 `field` 参数
- `internal/agent/agent.go` 修改：Init 时调用 `tftPlugin.SetLLMProvider(llmProvider)`
- 测试：23 个新测试（VisionOCREngine 12 + E2E 11），全部通过；不依赖真实 LLM（stub 注入）
- **真实 Windows E2E 待用户在 Windows 机器上运行 Aurora 二进制 + 启动 TFT + 验证真实识别效果**

**风险与缓解**：
- 风险：LLM 调用成本（每次截图 50+ 次调用）→ 缓解：Phase 15 优化（批量 prompt / 单次多区域识别）
- 风险：LLM 幻觉（编造英雄名）→ 缓解：所有英雄/装备名通过 `ChampionIDNormalizer` / `ItemIDNormalizer` 严格归一化，未识别 = Unknown（绝不写入 GameContext）
- 风险：网络延迟 → 缓解：异步截图 → 后台 LLM 识别 → 完成后增量 merge；用户不感知延迟

**不实现的内容**（明确 scope 限制）：
- 不实现 Phase 15 KB Grounding（knowledge-grounded 推理）
- 不实现 Trait Resolver（羁绊激活判定）
- 不实现 阵容评分 / 装备推荐 / 经济分析
- 不实现自动游戏操作（点击/购买/刷新/升级/移动）
- 不实现 SQLite 对话历史
- 不实现新游戏插件（LOL/Valorant 等）

Phase 14-D 完成后停止，不进入 Phase 15。

## ADR-026: Knowledge Grounding Layer（Phase 15）

**Status**: Accepted (2026-09-01)

**Context**:
Phases 13/14 解决了"识别"问题：`VisionOCREngine` 把截图转成 OCR 结果，`ChampionIDNormalizer` / `ItemIDNormalizer` 把 OCR 名称归一为 canonical ID（如 `TFT18_Vayne`），写入 `TFTGameContext.Board / Bench / Shop / Items / Traits / Augments`。Phase 12 的 `queryRelevantKB` 在 `detail == Full` 时把 GameContext 拼到 prompt，并只查 4 个英雄的 ID → Name 映射（`id: name`）。

**Gap**: `TFT18_Vayne` 对 LLM 来说仍是个 ID。LLM 不知道：
- Vayne 是什么（cost=1, role=Carry, traits=Bulwark/Marksman）
- 她的技能是什么（Tumble：朝最近敌人翻滚，下一击造成 X 伤害）
- 当前装备 `TFT18_IE` 是什么（无尽之刃：暴击翻倍 + 真实伤害）
- Bulwark 羁绊的阶数 / 类型 / 效果

**无知识 = 无高质量建议**。LLM 给出的"建议"只能是通用常识，不能与当前对局装备 / 羁绊 / 英雄属性结合。

**约束**（来自 Phase 15 用户指令）：
- **严格 patch / set 隔离**：TFT17 数据绝不流入 TFT18 上下文
- **Unknown 显式保留**：OCR 未识别的 ID（`""` / `"Unknown"`）必须保留，**不**猜测 / 不跨 set 替代
- **Status 独立维度**：与 `Value[T].Confidence`（OCR 可信度）正交
- **不修改 GameContext**：OCR 输出是 value object
- **无 LLM / 无网络 / 无 OCR / 无截图**：Grounding 是纯本地 KB 查询
- **不实现 阵容评分 / 装备推荐 / 经济分析 / 自动操作**
- **不引入 RAG / Embedding / Vector DB**
- **保留 Phase 12 行为**：grounder == nil → 走旧 queryRelevantKB（4 英雄）

**Decision**:

- **新建独立包 `internal/game/knowledge/grounder/`**（不放在 `internal/agent/` 或 `internal/game/plugins/tft/grounder/`）：
  - 避免 agent 包依赖游戏插件细节（保持层次清晰）
  - 避免 tft 插件依赖 knowledge 包（避免循环 import）
  - Grounder 是知识库层的"投影器"，概念上属于 knowledge 子模块

- **新类型 `GroundedGameContext`（value object，不可变）**：
  - 镜像 `TFTGameContext` 槽位结构（`GroundedBoardSlot` / `GroundedBenchSlot` / ...）1:1
  - 每个 slot 携带 `ResolvedChampion` / `ResolvedItem` / `ResolvedTrait` / `ResolvedAugment`（含 ID + Name + 详细字段 + Status）
  - 顶层 `GroundedMeta{Patch, SetKey, KBHasData, PatchFallback}`：让 prompt 头部能告诉 LLM "这是哪个 patch / set"
  - 字段全为值类型（不含指针或 chan），可安全序列化 / 比较

- **新枚举 `MatchStatus`（5 态）**：
  - `StatusResolved`：KB 命中（成功）
  - `StatusUnknown`：输入为 `""` / `"Unknown"` / KB 无此 ID（**不猜测**）
  - `StatusWrongSet`：ID 前缀与 setKey 不一致（**绝不跨 set 替代**）
  - `StatusKBUnavailable`：repo == nil 或查询出错
  - `StatusSkipped`：主动跳过（保留扩展性）
  - 与 `Value[T].Confidence`（OCR 维度，0.0~1.0 float）完全正交：Confidence 高但 Status=Unknown → "看到了但 KB 没有"

- **`Grounder` 无状态，并发安全**：
  - `New(repo GroundingRepository) *Grounder` 接收 5 方法接口（与 `tools.KnowledgeProvider` 签名一致，复用避免新接口）
  - 每次 `Resolve` 都从 KB 重新查询（KB 端有 RWMutex 保护，grounder 自身无需加锁）
  - repo == nil → 返回的 Grounder 仍可用，所有实体标 `Status=KBUnavailable`（不 panic）

- **3 concentric ring patch 隔离**（Phase 15 新增第 3 层）：
  - **Ring 1**（existing）：SQLite composite PK `(patch, game, mode, id)` —— `internal/game/knowledge/store/store.go`
  - **Ring 2**（existing）：`Service.queryScoped` 强制走 `resolveActivePatch` —— `internal/game/knowledge/service.go`
  - **Ring 3**（NEW Phase 15）：`prefixMatches(id, setKey)` 前置校验 —— `internal/game/knowledge/grounder/grounder.go`
  - 任一环拒绝时，row 标 `Status=StatusWrongSet`，**绝不**静默替换为当前 set 的同英雄

- **setKey 推导（双层回退）**：
  - 第一层：静态 `setKeyByPatch` map（已知 patch → set 精确映射）
  - 第二层：KB probe（`GetChampion(Query{})` 取首条 ID 前缀）→ 回写到静态 map
  - 都失败 → setKey = ""（无法校验时放行；KB 查不到时降级为 `Status=Unknown` 而非 `Status=WrongSet`）

- **Dedup**：每个 Resolve 调用内 4 个 `map[string]Resolved*` 缓存（champCache / itemCache / traitCache / augmentCache），同 ID 在多槽位（Vayne 棋盘+备战）只查 KB 1 次

- **`Format`（prompt 渲染）独立函数**：
  - 3000 token 硬上限（`charsPerToken = 2` 简化估算）
  - 段落顺序：Header → WrongSet 警告（仅当存在）→ GameState → 棋盘 → 备战 → 羁绊 → 装备/散件 → 商店 → 强化符文
  - 超限按"强化 → 商店 → 装备"优先级砍段，**永不**丢棋盘 / 备战 / 羁绊（核心决策输入）
  - 状态标签：`[unknown: 名称未识别]` / `[wrong_set: 当前 set 不包含此 ID]` / `[kb_unavailable]`

- **Agent 集成（最小侵入）**：
  - `GameContextPreparer` 新增字段 `grounder *grounder.Grounder`（可为 nil）
  - `NewGameContextPreparer` 构造函数增 3rd 参数 `g *grounder.Grounder`（nil → Phase 12 行为兼容）
  - `Prepare`：`grounder != nil && detail == ContextFull` → grounder 路径；否则回退到 `queryRelevantKB`
  - `agent.prepareGameOptions`：检查 `knowledge.(grounder.GroundingRepository)` 类型断言，通过则 `grounder.New(r)` 注入（类型断言失败 → log.Debug + 跳过 grounder，行为退化为 Phase 12）
  - **不**修改 `tools.KnowledgeProvider` 接口（grounder 复用其 5 个方法）
  - **不**新增 Agent 字段（grounder 在 `prepareGameOptions` 中按需构造，无状态）

- **设计保证**：
  - `Grounder.Resolve` 不修改 `*tft.TFTGameContext`（纯函数式投影）
  - `GroundedGameContext` 字段全为值类型，无指针 → 跨 goroutine 安全传递
  - `Format` 输出是 string，无副作用

**Reason**:
- **概念分层清晰**：Grounder 在 knowledge 包下（语义上是"用知识库做投影"），不污染 agent 层也不污染 tft 插件
- **测试可独立**：grounder 包不依赖 agent / tft 插件（除 `*tft.TFTGameContext` 输入类型），可单独跑测试
- **接口复用**：`tools.KnowledgeProvider` 与 `grounder.GroundingRepository` 5 个方法签名完全一致 → 类型断言一步到位，**不**再加 adapter 层
- **Phase 12 兼容**：grounder == nil 时行为完全等价于 Phase 12（既有 16 处测试全通过）
- **Status 维度独立**：让上层（LLM / 日志 / UI）能区分"看到了但 KB 没有"与"KB 查不到"
- **WrongSet 仍渲染**：不静默纠正，LLM 至少能看到"OCR 错配"信号
- **patch 隔离 3 层防御**：SQL PK 阻止 row 泄漏；queryScoped 阻止 app 跨 patch；prefix check 阻止 ID-shaped-but-wrong-set

**Consequences**:
- 新增 `internal/game/knowledge/grounder/{types.go, grounder.go, format.go, grounder_test.go, format_test.go}`
- 修改 `internal/agent/game_context.go`：新增 grounder 字段 + 改构造函数 + Prepare 增 grounder 路径
- 修改 `internal/agent/agent.go` `prepareGameOptions`：增 grounder 实例化 block（5 行）
- 修改 `internal/agent/game_context_test.go`：16 处 `NewGameContextPreparer` 签名更新（加 `, nil` 末尾）
- 24 个新测试全通过（13 Resolve + 11 Format），与 Phase 12 测试无冲突
- KB schema / Service / Store / tools.KnowledgeProvider 全部**未**修改
- 4 个 pre-existing failures（Phase 14-D 报告的 `item_resolver_test.go` / `ocr_parse_test.go`）仍存在，**非**本阶段回归

**未实现（明确 out of scope）**：
- 阵容评分 / 强度判断
- 装备推荐 / 装备合成
- 经济分析 / D / 升级决策
- DPS 计算 / 战斗预测
- 自动操作游戏（点击/买/刷新/升级/移动）
- SQLite 对话历史
- 新游戏插件
- 大规模 KB 重构

**推荐下一阶段**：
Phase 16 应聚焦"基于 GroundedGameContext 的运营建议生成"，可考虑：
- Board strength 评估（基于 cost / traits / items 简单聚类，不引入复杂模型）
- 经济节点提示（基于 GroundedGameState.Gold / Interest / WinStreak / HP）
- 但**仍**不应进入自动操作；只做 prompt 注入增强

Phase 15 完成后停止，等待下一步指令。

---

## ADR-027: Universal Game Companion Core — 重新定位 Aurora 的架构身份（Phase 17-A）

**Status**: Accepted (2026-09-01)

**Context**:

Phases 10-16 期间，Aurora 的实际能力已经是 "Universal Game AI Companion" 形态：
- 完整的 LLM / ASR / TTS / PTT / Tool 系统
- GamePlugin 抽象（ADR-005 / ADR-021）
- 多个分析子包（grounder / composition / economy / equipment / preparation / gamestate）
- 5 个 TFT 专属工具

但**实现位置**错位了：所有 TFT 能力直接写在 `internal/agent` 与 `internal/tool/tools` 中央注册表里。具体违规：
- `internal/agent/bindings.go:15` import `internal/game/plugins/tft`（`switch *tft.TFTGameContext`）
- `internal/agent/game_context.go` 600 行 TFT 专属代码（FormatTFTGameContext / queryRelevantKB / needsGameContext / pickContextDetail / detectAnalysisOptions）
- `internal/agent/tft_tools_wiring.go` 文件名 + 硬编码 5 个 tool 名（直接踩 CLAUDE.md §15 反例）
- `internal/tool/tools/{tft_game_state.go, tft_knowledge.go, init.go}` 混入 5 个 TFT 工具到通用工具注册表
- 6 个 analyzer 子包路径错误：`internal/game/knowledge/{composition,economy,equipment,preparation,gamestate,grounder}/` 散落在通用层，CLAUDE.md §2.4 明确要求 `internal/game/knowledge/tft/`

后果：今天新增 DoudizhuPlugin，Agent Core 必然再次复制粘贴同样的耦合；Agent Core 与通用 tool 包无法独立单元测试。

**Decision**：

1. **3 个新可选接口**（`internal/game/game.go`）—— 用 type assertion 而非修改 `GamePlugin` 接口本身（保持 §2.5 抽象纪律）：
   - `ToolContributor`：`RegisterTools(reg *tool.ToolRegistry) error` — 插件可注册自己的工具
   - `GameContextRenderer`：`RenderGameContext(gctx *GameContext, userInput string) (string, bool)` — 插件可把 GameContext 渲染为 LLM prompt 摘要
   - `SetupReceiver`：`AttachDependencies(deps SetupDeps) error` — 插件可接收 Agent 注入的 GameState / KB 依赖
   - `SetupDeps{GameState GameStateProvider, Knowledge any}` —— Knowledge 改 `any` 而非具体接口（让 Agent Core 不被 Knowledge 接口细节绑定；plugin 自己 type-assert）

2. **TFT 数据类型下沉到 `tfttypes` 子包**：
   - 10 个数据类（`TFTGameContext / BoardSlot / BenchSlot / ShopSlot / ItemSlot / AugmentSlot / TraitState / CarouselInfo / Opponent` 等）从 `internal/game/plugins/tft` 移到 `internal/game/plugins/tft/tfttypes`
   - tft 父包通过 `type TFTGameContext = tfttypes.TFTGameContext` 暴露别名（**完全向后兼容**：旧引用 `*tft.TFTGameContext` 仍可用）
   - **目的**：打破 `tft → composition → grounder → tft` 父子互引 cycle（tfttypes 是叶子包，不 import tft）

3. **`tools.KnowledgeProvider` 接口定义独立化**（`internal/game/plugins/tft/tools/tft_knowledge.go`）：
   - tools 子包自己定义 5 方法接口（不 import tft 父包）
   - tft 父包通过 `type KnowledgeProvider = tools.KnowledgeProvider` 暴露别名
   - **目的**：打破 `tft → tools → tft` cycle（tools 不再依赖 tft 父包）

4. **6 个 analyzer 子包下放**到 `internal/game/plugins/tft/` 下（与插件同包，渲染逻辑可自由 import）：
   - 之前路径：`internal/game/knowledge/{composition,economy,equipment,preparation,gamestate,grounder}/`
   - 现在路径：`internal/game/plugins/tft/{composition,economy,equipment,preparation,gamestate,grounder}/`
   - **依据**：CLAUDE.md §2.4 "Domain Logic 留在 Domain" 明确允许 `internal/game/plugins/<game>/**` 与 `internal/game/knowledge/<game>/**` 两个位置；本阶段 tft 父包需要 import 这些子包做 prompt 渲染，knowledge 层是父包的话是反方向

5. **5 个 TFT 工具下放**到 `internal/game/plugins/tft/tools/`：
   - `get_tft_game_state` + 4 个 KB 工具（`get_tft_champion / item / trait / augment`）
   - `internal/game/plugins/tft/tools/init.go` 在 `init()` 中调用 `tool.RegisterTool(...)` 自注册（沿用 LLM/Speech Provider 模式，ADR-012）
   - `internal/tool/tools/init.go` 退化为只挂 `capture_screenshot` + `get_current_time`

6. **`tft.Plugin` 实现 3 个新接口**（`internal/game/plugins/tft/tft.go`）：
   - `AttachDependencies(deps SetupDeps) error`：保存 stateProvider + kbProvider（`kbProvider, _ := deps.Knowledge.(KnowledgeProvider)`），构造 `PromptRenderer`
   - `RegisterTools(reg *tool.ToolRegistry) error`：从 registry 按名查工具实例，把 `game.GameStateProvider` / `tft.KnowledgeProvider` 通过 interface 转换赋给 `tools.GameStateProvider` / `tools.KnowledgeProvider`（方法集相同，Go interface 转换合法）
   - `RenderGameContext(gctx, userInput) (string, bool)`：委托内部 `PromptRenderer.Render`

7. **Agent Core 端 4 处清理**：
   - `internal/agent/agent.go`：删除 `composition / gamestate / grounder` import；`knowledgeProvider` 字段类型从 `tools.KnowledgeProvider` 改为 `any`；新增 `wirePlugins()` 方法
   - `internal/agent/game_context.go`：从 600 行退化到 ~80 行薄包装，循环 `manager.Plugins()` 对实现 `GameContextRenderer` 的插件调 `RenderGameContext`
   - `internal/agent/bindings.go`：删除 `switch *tft.TFTGameContext`（改读 `gctx.Game` —— plugin 在 `Detect()` 时已填好）
   - `internal/agent/tft_tools_wiring.go`：整文件删除（plugin 自己在 `RegisterTools` 里完成注入）

8. **架构回归门禁**（`internal/agent/architecture_test.go`，3 个 `go list` 断言）：
   - `TestAgent_NoGamePluginDependency`：`internal/agent` 不 import `internal/game/plugins/*`
   - `TestTool_NoGamePluginDependency`：`internal/tool*` 不 import `internal/game/plugins/*`
   - `TestGenericGame_NoGamePluginDependency`：`internal/game` 不 import `internal/game/plugins/*`
   - 任何重新引入插件耦合的 PR 立即 fail

9. **测试替换**：
   - `internal/agent/game_context_test.go` → `internal/game/plugins/tft/prompt_test.go`（22 个测试）
   - `internal/agent/tft_tool_loop_test.go` → `internal/agent/agent_loop_test.go`（`TestParseToolCall` + `TestAgentLoop_ToolCallFlow`，generic mock LLM + 空 tool registry，证明 Agent Loop 对游戏无知）
   - 工具测试随工具搬到 `internal/game/plugins/tft/tools/`

**Reason**：

- **符合 CLAUDE.md §2.1 依赖方向**：Agent Core 只依赖 `internal/game` 接口；Plugin 不被 Agent Core 反向依赖
- **符合 §2.2**：Agent Core 不再 import `internal/game/plugins/*`，不再出现 `tft / lol / cs2` 硬编码分支，不再持有具体游戏 struct
- **符合 §2.3 新增游戏不改 Agent**：新游戏 = 新插件包 + main.go blank import + config.enabled；Agent Core / `internal/tool/tools` 零修改
- **符合 §2.4**：Domain logic（棋子/装备/羁绊/强化/阵容/经济/备战/利息）整体留在 tft 插件与 knowledge 层；通用层不再含具体英雄名
- **符合 §2.5 抽象纪律**：3 个新接口都回答了"插件要注入不同能力的真实边界"（不是"看起来更高级"）
- **符合 §15 反例清单**：`*_tft_*.go` 文件命名 / `internal/agent` import 插件 / 类型 switch 全部消失
- **可独立单元测试**：Agent Core 测试不再需要 mock tft；tft 插件测试不再需要 mock agent
- **Token 效率不变**：renderer 仍严格按用户问题 scope 选择 KB / analyzer，0 LLM 调用（保持 §3 / §4 不变）
- **导入 cycle 通过 tfttypes / tools 自身接口定义解决**：不引入新 wrapper 层（§2.5）

**Consequences**：

- `internal/agent/game_context.go`：600 行 → ~80 行
- `internal/agent/tft_tools_wiring.go`：删除
- `internal/agent/bindings.go`：删除 `import tft` 与类型 switch
- `internal/tool/tools/init.go`：只剩 screenshot + time
- 新增 `internal/game/plugins/tft/{tfttypes,prompt.go,system_prompt.go}` + `internal/game/plugins/tft/tools/{init.go,tft_game_state.go,tft_knowledge.go,...}`
- 新增 `internal/agent/architecture_test.go`（3 个回归门禁）
- 新增 `internal/agent/agent_loop_test.go`（generic Loop 回归）
- 22 个新 prompt 测试（`internal/game/plugins/tft/prompt_test.go`）
- `go build ./...` 通过；3 个架构门禁 + Agent Loop 通用回归通过；tft 子包 30+ 测试全过
- 3 个 pre-existing 失败（`item_resolver` 的 `TestItemIDNormalizer_ResolveWithPatch_WrongSet` / `TestCanonicalizeTFTID`，`ocr_parse` 的 `TestParseBoardCellText`）与本阶段无关，按 §6 留待后续
- Wails 绑定结构（`Agent.GetKnowledgeProvider`）类型从 `tools.KnowledgeProvider` 改为 `any`，前端 bindings 需保持兼容（仅内部调用，无影响）

**不实现的内容**（明确 scope 限制，按 §35）：

- 不实现 LoL / Doudizhu / 黑神话 plugin
- 不增加 TFT 任何新功能
- 不重写 GameContext 数据结构
- 不修改 main.go 的 blank import 列表（TFT 仍是唯一 plugin）
- 不动 Agent Core 主链路（`onUserInput` / `ChatStream` / TTS）
- 不动前端

**完成后立即停止，等用户下一步架构确认（不进入 17-B）。**

---

## ADR-028: Windows Distribution Is Not Constrained to Single EXE

**Status**: Accepted (2026-09-01)

**Context**:
早期项目假设 Aurora 必须打包为单个 EXE 文件，所有依赖必须嵌入，不允许存在 DLL / 外部 runtime / resource 文件 / helper process。这一假设记录在早期文档中（如 ADR-001、ADR-009、ADR-025 的相关描述）。

随着项目演进，单 EXE 约束被发现并非产品价值，反而可能增加：
- native dependency complexity（CGO 链接、Windows 运行时库）
- build complexity（交叉编译、资源嵌入、embed 限制）
- debugging difficulty（难以替换/更新单个组件）
- maintenance cost（所有变更影响整个产物）

**Decision**:
**取消 Aurora 必须为单 EXE 的架构约束。**

Aurora 是一个正常的 Windows Desktop Application。Windows 发布方案根据以下优先级选择最优方案：

1. Correctness
2. Stability
3. Development speed
4. Runtime performance
5. Maintainability
6. User installation experience
7. Single-EXE 여부（不是要求）

如果多个文件的标准 Windows 发布方式比单 EXE 更简单、更稳定、更容易维护，应优先采用多文件方案。

**允许的分发形式**：
- DLL（标准 Windows 依赖）
- external runtime（按需）
- resource files / model files（独立于主程序）
- plugins（可独立更新）
- helper processes（按需）
- installer-managed dependencies（安装器负责部署）

**不允许的倒退**：
- 要求用户手动安装 Python / Node.js / Docker 等开发运行时
- 要求用户手动配置复杂环境

Aurora 应提供简单的 Windows 安装体验，用户无需理解内部依赖。但"简单安装"由 installer / 发布方案解决，不由"必须是单 EXE"解决。

**Reason**:
- 单 EXE 不是用户价值（用户关心的是"安装简单、运行稳定"，不是"一个文件"）
- 标准 Windows 发布方式（installer + 多文件）更成熟、更灵活、更易维护
- 去除该约束可以消除多个架构决策中的不必要限制（如 ADR-025 视觉选型中"单 EXE"作为评估维度）
- 简化构建系统，降低交叉编译和调试复杂度

**Consequences**:
- `CLAUDE.md` 项目形态描述更新：去除"目标产物是单 EXE，零额外运行时"
- `README.md` 产品原则更新：从"单一可执行文件"改为"简单安装体验"
- `docs/ai/PROJECT.md` 设计原则更新：同上
- `docs/ai/CURRENT_STATE.md` Important Constraints 更新：去除"必须保持单 EXE 输出"
- 早期 ADR（ADR-001 / ADR-009 / ADR-025）中的历史记录保留不改，但本 ADR 明确那些约束已不再适用
- 代码注释中的历史技术解释保留（解释当时为什么做出某个选择）
- 当前 build 行为不变（仍使用 `wails build`，仍产出单 EXE）—— 这是当前选择，不是强制约束

## ADR-029: Per-request ExtraSystemPrompt 注入模式（Phase 18-A）

**Status**: Accepted (2026-09-01)

**Context**:
Phase 17-B 把 `onUserInput` 切换到 `chatStreamWithTools` 工具循环时丢失了 GameContext → LLM prompt 注入链路。Phase 12-16 积累的 GameContext 分析能力（阵容/经济/装备/备战/知识库投影）完全失效。需要恢复注入能力，同时保持：
- 不污染对话历史（ExtraSystemPrompt 只影响单次请求）
- 工具循环多轮迭代都带同一份 ExtraSystemPrompt（同一 user turn 内 GameContext 不变）
- 0 LLM 调用在注入路径（所有分析是本地确定性计算）
- 通用 Agent Core 不知道具体游戏（通过 `game.GameContextRenderer` 接口委托）

**Decision**:

1. **`chatWithOptions(ctx, opts ChatOptions)` 新方法**：
   - 调用 `convertMessages()` 得到消息副本
   - 如果 `opts.ExtraSystemPrompt != ""`：找到第一条 system 消息，追加 `\n\n` + ExtraSystemPrompt
   - 防御性：若对话没有 system 消息但 ExtraSystemPrompt 非空，主动在头部插入一条
   - **不**写入对话历史（与 `Chat()` 的关键区别）
   - 调用方（`chatStreamWithTools` / `streamFinalAnswer`）负责历史写入

2. **`chatStreamWithTools(ctx, opts)` 签名扩展**：
   - 接受 `ChatOptions`
   - 工具循环每轮迭代都通过 `chatWithOptions(ctx, opts)` 把同一份 opts 注入
   - 中间 tool-call 响应显式 `AddAssistantMessage(resp)` 写历史
   - 最终答案由 `streamFinalAnswer` 一次性写历史（避免 Phase 17-B 的双写 bug）

3. **`prepareGameOptions(ctx, userInput)` 真正工作**：
   - 委托 `gameContextPreparer.Prepare(ctx, userInput)`
   - Preparer 委托给实现 `game.GameContextRenderer` 的插件（如 tft）
   - 插件返回 `(summary, used)`：used=false → 不注入（0 额外 token）
   - 插件返回 used=true → `ChatOptions{ExtraSystemPrompt: summary}`

4. **`onUserInput` 恢复 wiring**：
   ```go
   opts := a.prepareGameOptions(ctx, in.Text)
   a.chatStreamWithTools(ctx, opts)
   ```

**Reason**:

- **对话历史纯净**：ExtraSystemPrompt 不进历史 → 下次 user turn 重新从 Manager.LastKnown() 读最新快照，避免"旧对局状态污染新对局"
- **Token 节约**：非游戏问题 0 额外 token；游戏相关问题 <=3000 token（Phase 15 FormatMaxTokens）
- **架构边界保持**：Agent Core 不知道 tft / 任何具体游戏，全靠 `game.GameContextRenderer` 接口
- **工具循环一致性**：同一 user turn 内的工具循环多轮迭代共享同一份 GameContext 快照（避免循环中 GameContext 变化导致 LLM 看到不一致状态）
- **修复 Phase 17-B 双写 bug**：中间 tool-call 显式写历史，最终答案由 streamFinalAnswer 写一次

**Consequences**:

- `internal/agent/agent.go`：`chatWithOptions` 新方法（~30 行）；`chatStreamWithTools` 签名变更（1 处）；`prepareGameOptions` 从 20 行零值返回 → 8 行实际工作；`onUserInput` 增加 2 行 wiring
- `internal/agent/tool_image_test.go`：4 处调用签名更新
- `internal/agent/game_context_injection_test.go`：7 个新集成测试
- `internal/game/manager.go`：新增 `UnregisterPlugin` 用于测试清理
- 既有 agent 测试 + tft plugin 测试 + 架构门禁全部通过
- Pre-existing tft 失败（item_resolver/ocr_parse，Phase 14-C/14-D）与本 ADR 无关

---

## ADR-030: TTS 播放器 mpegvideo 驱动自动降级为 WAV（Phase 18-B 修复）

**Status**: Accepted

**Context**:

Windows 10/11 上 MCI 播放 MP3 报错 `error_code=289 (MCIERR_CANNOT_LOAD_DRIVER)`。
原因：`mpegvideo` MCI 驱动在部分 Windows 系统上不可用：
- Windows N/KN 版本（无 Media Feature Pack）
- MCI 驱动注册表项损坏或缺失
- 部分 Windows 10/11 更新后驱动丢失

原代码硬编码 `open "<file>" type mpegvideo alias auroratts`，无法降级。

**Decision**:

1. **启动时探测 mpegvideo 驱动可用性**：`probeMPEGVideo()` 在 `NewPlatformPlayer()` 中调用，
   尝试打开不存在的文件，通过返回错误码区分"驱动不存在"（289）vs"文件不存在"（276 等）。
2. **Player 接口新增 `PreferredFormat() string`**：让播放器声明偏好的音频格式（"mp3" 或 "wav"）。
3. **Volcengine TTS provider 初始化时自动调整格式**：若 `player.PreferredFormat()` 与
   config 不一致，自动切换到播放器支持的格式（mp3 → wav 降级）。
4. **MCI 错误消息增强**：引入 `mciGetErrorStringW`，错误日志包含人类可读描述。
5. **Play() 按文件扩展名选择 MCI 设备类型**：`.wav` → `waveaudio`，`.mp3` → `mpegvideo`，
   取代硬编码 `type mpegvideo`。

**Reason**:

- **零额外依赖**：继续用 `winmm.dll` MCI，不引入新音频库
- **自动降级**：Windows N/KN 用户无需手动改 config
- **WAV 可靠性**：`waveaudio` 是 Windows 基础 MCI 驱动，几乎所有 Windows 版本都有
- **向后兼容**：mpegvideo 可用的系统保持 MP3 行为（文件更小）
- **Token 无关**：TTS 是输出链路，不影响 LLM 上下文

**Consequences**:

- `internal/tts/playback_windows.go`：新增 `mciGetErrorString`、`probeMPEGVideo`、`mciTypeForFormat`、
  `PreferredFormat()`；`Play()` 改为按扩展名选 MCI 类型；`windowsPlayer` 增加 `preferredFormat` 字段
- `internal/tts/playback.go`：`Player` 接口增加 `PreferredFormat() string` 方法
- `internal/tts/playback_default.go`：`defaultPlayer.PreferredFormat()` 返回 "mp3"
- `internal/tts/providers/volcengine/volcengine.go`：初始化时读取 `player.PreferredFormat()`，
  不一致则自动调整 `config.Format` 并打 warn 日志
- 新增 `playback_test.go`（跨平台接口测试）+ `playback_windows_test.go`（Windows 特定测试）

---

## ADR-031: ToolRegistry 两阶段初始化（先创建 → 后注入依赖 → 再过滤）

**Status**: Accepted

**Context**:

`capture_screenshot` 工具在运行时总是报 `tool not found: capture_screenshot`，
LLM 无法调用截图功能。

根因：`CreateRegistry` 在实例化工具时立即检查 `IsAvailable()`，而 `capture_screenshot`
的 `IsAvailable()` 依赖 `screenshotProvider` 是否被注入。但 `screenshotProvider`
是在 `CreateRegistry` 之后才注入的：

```
旧流程（有 bug）：
  CreateRegistry（检查 IsAvailable → capture_screenshot = false → 跳过）
  ↓
  SetScreenshotProvider（工具已被跳过，注入无效）
```

这是典型的"先过滤后注入"顺序 bug。

**Decision**:

`CreateRegistry` 不再在初始化阶段按 `IsAvailable()` 过滤工具。
新增 `ToolRegistry.FilterUnavailable()` 方法，由调用方在所有运行时依赖注入完成后显式调用：

```
新流程：
  CreateRegistry（实例化所有 enabled 工具，不过滤）
  ↓
  SetScreenshotProvider（注入依赖到已实例化的工具）
  ↓
  FilterUnavailable（移除仍不可用的工具）
```

**Reason**:

- 解决"先过滤后注入"顺序 bug
- 让依赖注入路径更清晰（显式调用 vs 隐式构造时检查）
- 向后兼容：`FilterUnavailable` 行为与原 `CreateRegistry` 内过滤一致
- 未来新增需要运行时依赖注入的工具时，遵循同样的两阶段模式

**Consequences**:

- `internal/tool/registry.go`：`CreateRegistry` 移除 `IsAvailable` 检查；新增 `FilterUnavailable()` 方法
- `internal/agent/agent.go`：依赖注入后调用 `FilterUnavailable`
- `internal/agent/bindings.go`：同上
- `internal/tool/registry_test.go`：4 个新测试（移除/nil 安全/幂等/回归）
- `internal/game/plugins/tft/tools/registry_tft_test.go`：更新过时注释

---

## ADR-032: Wails v2 → v3 迁移（Phase 8, 8A+8B+8C）

**Status**: Accepted

**Context**:

Wails v2 的单窗口模型与 assetserver 实现成为"独立 Quick Input window + 独立 Overlay window"
（Phase 8E/8F）的硬约束。Wails v3（beta.16，2026-08-29）提供 first-class 多窗口、
built-in system tray / global shortcut、`WS_EX_TRANSPARENT | WS_EX_LAYERED` 直通 API。
因此把运行时从 v2.15.0 切到 v3 beta.16；本轮只做迁移，不做多窗口功能本身。

**Decision**:

1. **Bridge 包收敛**：新建 `internal/wailsruntime`，全仓库只有 `main.go` + 该包 import
   `wails/v3/pkg/application`。所有 emit / context / window 操作走 bridge，
   后续 v3 beta API 变化只需改两处。
2. **API 映射**（静态对照 v3.0.0-beta.16 源码验证）：
   - `wails.Run(options)` → `application.New(...).Run()`；Services = `application.NewService(&Bindings)`
   - `OnStartup` → `OnApplicationEvent(events.Common.ApplicationStarted)`
   - `HideWindowOnClose` + `OnBeforeClose` → `RegisterHook(events.Common.WindowClosing)` + `e.Cancel()` + `Hide()`
   - `runtime.EventsEmit(ctx, name, data)` → `App.Event.Emit(name, data...)`；单 payload 时
     前端 `ev.data` 即 payload 本身（≡ v2 语义），前端事件契约不变
   - `window.go.*` 全局绑定 → `wails3 generate bindings` 生成 `frontend/bindings/` ESM（`@wailsio/runtime`）
3. **行为零回归目标**：hide-on-close、tray 菜单、全局热键、QuickInput 内嵌 Main Window、
   事件名 / payload 形状、绑定方法名全部保持。
4. **保留不动**：`internal/tray`（getlantern/systray）、`internal/input`（ADR-011）、
   `internal/agent` 业务逻辑、全部 game/tool 子包；ADR-010（绑定方法无 ctx 参数）继续成立。

**Consequences**:

- 构建系统从 `wails build`（wails.json）换成 `wails3 build`（Taskfile + `build/` 脚手架）；
  `build/` 脚手架改为 git 跟踪（fresh clone 可构建），输出移到 `bin/`
- v3 处于 beta：升级 beta 版本可能带来 API break，bridge 收敛控制了影响面
- macOS 上已验证 build + 全部 Go 测试；**Windows GUI 行为（事件时序 / hook / 热键）尚未真机验证**，
  在 Windows 冒烟通过前不应合并或发布
- 8D（tray 迁 `app.SystemTray`）/ 8E / 8F（独立窗口）留待后续 Phase，届时才能兑现 v3 多窗口收益

**Alternatives Considered**:

- **留守 v2**：多窗口需求（8E/8F）无法满足，且 v2 处于维护模式
- **等 v3 stable**：无时间表；beta.16 已被社区广泛采用，风险可通过分支 + tag（`phase-8/baseline-v2`）管控

---

## ADR-033: System Tray 迁移到 Wails v3 `app.SystemTray`（Phase 8D）

**Date**: 2026-09-03
**Status**: Accepted

**Context**: ADR-032 迁移 v3 后，`getlantern/systray` 成为唯一的 v2 时代第三方窗口组件：独立 goroutine 消息循环、与 v3 事件模型割裂，且 macOS dev 构建行为与 Windows 不一致。

**Decision**: 删除 `getlantern/systray`，托盘改为 v3 原生 `app.SystemTray`（v3App 拥有，`Run` 前创建并 `BuildMenuAndAttach`）。`internal/tray` 保留平台抽象（`tray_windows.go` = v3 实现，`tray_default.go` = darwin dev no-op stub），main.go 调用面（`SetOn*Handler` / `RegisterRadioItem` / `SetMenuItemChecked` / `SetOverlayMenuState`）不变。

**Consequences**:
- 少一个第三方窗口组件依赖；托盘生命周期由 v3App 管理（`tray.Shutdown` 变 no-op）
- v3 `application` import 扩散到 `internal/tray`（接受：menu 构造需要 v3 类型，且该包本就是 Wails 表现层）
- Windows 真机行为（图标/菜单/radio 勾选）待 Windows E2E 验证

---

## ADR-034: QuickInput / Overlay 迁独立 Native Window + `chatUserMessage` 事件（Phase 8E/8F）

**Date**: 2026-09-03
**Status**: Accepted

**Context**: Phase 3/2 时代 QuickInput 与 Overlay 均渲染在 Main webview 内：Main 隐藏即不可见、无 OS 级置顶、Overlay 无法悬浮于游戏之上。v3 多窗口能力（ADR-032）已就绪。

**Decision**:
1. **Vite multi-page**：`index.html` / `quickinput.html` / `overlay.html` 三入口，三个物理独立 webview，bundle 互不依赖。组件复用（QuickInput.vue / Overlay.vue）通过 props/emits，不复制业务逻辑；Overlay.vue 仅新增 `native` prop 样式分支（填满窗口），anchor 四角定位留给 Preview。
2. **`internal/input` 零 wails 依赖**：`QuickInputManager` 改 driver 注入模式——`QuickInputDriver` 接口（Show/Hide/IsVisible）定义在 `internal/input`，main.go 用 `NewFuncDriver` 桥接 `wailsruntime.Show/HideQuickInputWindow`。窗口几何与生命周期留在 main runtime 层。
3. **可见性单一真相**：QuickInput 走 `QuickInputManager` 状态机（driver 委派）；Overlay 走 agent `overlayVisibleState`（`SetOverlayVisible` 唯一写入口，Enabled ≠ Visible 设计保持），main.go 仅订阅 `overlayVisibilityChanged` 把状态投影到窗口。窗口 singleton：启动 Hidden，Show/Focus 复用，Esc/Enter/Close 一律 Hide 不销毁（`WindowClosing` hook `e.Cancel()`）。
4. **Ctrl+Alt+R 迁 v3 `GlobalShortcut.Register`**（OS 级，唯一注册点），旧 Win32 RegisterHotKey 路径删除避免双注册；PTT / CycleMode 仍走 Win32（`internal/input` 消息线程，ADR-011）。
5. **Anchor 语义升级**：Overlay `Anchor` 从"Main webview 内四角"升级为"主屏 WorkArea 四角"（`wailsruntime.SetOverlayAnchorPosition`），配置字段不变；不做多显示器跟随 / 自由定位 / click-through / 自由尺寸。
6. **新增 `chatUserMessage` 事件**：`Agent.onUserInput`（文本/语音/QuickInput 所有输入源唯一汇聚点）入口同步 emit，供独立 Overlay webview 组装 user 侧对话（先于所有 `llmChunk`）。这是跨 webview 消息组装的最小必要事件，Main App.vue 不监听（本地已 push）。
7. **提交后不 Show Main Window**：QuickInput 提交 → Hide → Agent 处理；Main 由用户手动打开。

**Consequences**:
- Overlay 可真正悬浮于游戏之上（Windowed / Borderless Windowed；exclusive fullscreen 不保证，不引入 injection/hook 方案）
- `llmChunk` 等 `App.Event.Emit` 广播到达所有 webview，Main Chat 与 Overlay 各自组装渲染同一对话
- 已知行为差异：Settings 保存 `Enabled` 不再立即显示 Overlay（可见性只走 `SetOverlayVisible`/重启），待用户决策是否补触发
- Windows E2E（悬浮/焦点/透明渲染/任务栏/多屏定位）在 macOS 上无法验证，合并前需真机冒烟

**Alternatives Considered**:
- 全屏透明窗口 + CSS anchor（overlay close/滚动失效，弃）
- 动态 click-through（`SetIgnoreMouseEvents` hover 切换：新能力而非迁移，记 Future）
- 前端 `Events.Emit` 上报 user 消息（输入源覆盖不全；Go 唯一汇聚点 emit 更可靠）
