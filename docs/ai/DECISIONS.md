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
