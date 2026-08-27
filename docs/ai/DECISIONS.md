# GameAI - 架构决策记录

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
- 用户只需要运行一个 GameAI.exe
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
