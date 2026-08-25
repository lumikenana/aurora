# GameAI - 开发变更日志

## 2026-08-25

### Bug Fixes: LLM 配置 + 流式响应全链路修复

本阶段修复了从首次启动配置 API Key 到流式响应显示的多个 bug，打通端到端链路。

Fixed:
- **首次启动无法配置 API Key**（`agent.go` + `bindings.go`）:
  - `agent.Init()` 在 LLM provider 创建失败时直接 `return err`，导致 `agentInstance` 始终为 nil
  - `SaveLLMConfig` 发现 `agentInstance == nil` 时只保存配置文件到磁盘，从不创建 agent 实例
  - 修复: `Init()` 不再因 LLM 失败而中止，仍创建 agent（`llmProvider = nil`）；`SaveLLMConfig` 在 `agentInstance == nil` 时完整初始化 agent
- **Provider 未注册**（`main.go`）:
  - `openai` 包在整个项目中从未被 import，`init()` 从未执行，`providerFactories` map 里无 "openai"
  - 修复: 在 `main.go` 添加 blank import `_ "github.com/lipipi/game-ai/internal/llm/providers/openai"`
- **API 404 错误**（`openai.go`）:
  - 代码把 `BaseURL` 直接当完整请求 URL，未拼接 `/chat/completions` 路径
  - 修复: 新增 `buildURL(path)` 方法自动拼接，兼容已含完整路径的情况
  - 默认 Base URL 从错误的 `/api/v1` 改为正确的 `/api/plan/v3`（火山引擎 ARK OpenAI 兼容地址）
- **SSE 流解析永远返回空响应**（`openai.go`）:
  - `line[:5] != "data"` 永远为 true（5 字符切片 vs 4 字符字符串），导致每一行都被跳过
  - 修复: 改为 `line[:4] != "data"`，并兼容 `"data:"` 和 `"data: "` 两种 SSE 格式
- **豆包思考模型返回空 content**（`openai.go`）:
  - `doubao-seed-2.0-lite` 默认开启 thinking，SSE 返回 `content: ""` + `reasoning_content: "..."`
  - 修复: 请求体显式添加 `thinking.type = "disabled"` 关闭思考模式

Added:
- `SaveLLMConfig` 增加详细日志（入参脱敏、配置状态、保存结果、provider 创建结果、IsConfigured）
- `GetConfigStatus` 增加 `agentInstance` / `llmProvider` 为 nil 的保护，避免 panic
- `maskAPIKeyForLog()` 辅助函数，日志中脱敏 API Key
- 前端 `handleSubmit` / `llmChunk` / `llmFinish` 调试日志（临时）

Changed:
- `frontend/src/components/Settings.vue` - 默认 Base URL 改为 `https://ark.cn-beijing.volces.com/api/plan/v3`

Architecture:
- Provider 自注册模式（`init()` + blank import）遵循 Go 标准做法（类似 `database/sql` + driver）
- 因 `openai` 包依赖 `llm` 包，不能在 `llm/factory.go` 内 import（会循环依赖），必须在 `main.go` 引入

---

## 2026-08-25

### Phase 5: Tool System - 轻量级 Agent 工具系统

本阶段实现了轻量级工具调用系统，让 LLM 可以根据用户请求调用工具获取额外信息。

Added:
- 创建 `internal/tool/` 工具核心包
  - `tool.go` - 定义 `Tool` 统一接口 + `ToolResult` 结果类型
  - `registry.go` - `ToolRegistry` 工具注册表，工厂注册模式
  - `config.go` - 工具模块配置
- 创建 `internal/tool/tools/` 具体工具实现
  - `screenshot_tool.go` - **capture_screenshot** 全屏截图工具，返回 JPEG base64
  - `current_time_tool.go` - **get_current_time** 获取当前时间工具
  - `init.go` - 工具自注册
- **Agent 集成**：
  - `internal/agent/agent.go` 添加 `toolRegistry` 依赖
  - 新增 `ChatWithTools` Agent Loop：用户提问 → LLM 判断 → 调用工具 → 结果入上下文 → LLM 回答
  - 系统提示词自动追加工具定义
  - 最大工具调用限制 5 次防止循环，每个工具调用 30 秒超时
  - `onUserInput` 自动使用 `ChatWithTools` 如果有工具启用

Changed:
- `internal/config/config.go` - 全局 Config 添加 `Tool tool.Config` 字段
- `docs/ai/CURRENT_STATE.md` - 更新当前状态

Architecture:
- 完全遵循现有项目架构模式：接口定义在核心包，具体工具在子包，工厂 + init 自注册
- Agent 只依赖 `ToolRegistry` 接口，不依赖具体工具
- 添加新工具不需要修改核心代码，只需要新建文件 + 注册
- 所有工具执行带超时，完整错误处理

Notes:
- 默认启用 `capture_screenshot` 和 `get_current_time` 两个工具
- `capture_screenshot` 复用现有的 `screenshot.ScreenshotProvider`
- 当前不修改流式响应的工具调用支持，只在非流式 `Chat` 基础上实现 Agent Loop
- 工具调用格式使用分隔符 `<|ToolCall|>...<|/ToolCall|>`，简单可靠

---

## 2026-08-25

### Phase 4: Windows 部署验证 + 前端 Vue 3 迁移 + Screenshot 模块

本阶段在 Windows 本机构建运行，端到端验证了全链路，修复了一系列仅 Windows 环境暴露的 bug。

Added:
- 创建 `internal/screenshot/` 截图模块
  - `screenshot.go` - `ScreenshotProvider` 接口（CaptureFull / CaptureWindow）
  - `screenshot_windows.go` - Windows GDI 实现（GetDC + BitBlt + GetDIBits）
  - `screenshot_default.go` - 非Windows空实现
  - `factory.go` - Provider 注册工厂
- 前端迁移至 **Vue 3 + Vite + TypeScript**（`frontend/`）
  - `src/api/backend.ts` - Wails bindings 封装层
  - `src/api/events.ts` - 事件订阅封装（直接使用 `window.runtime`）
  - 组件：`Conversation` / `LogPanel` / `QuickInput`（v-model 弹出层）
  - `App.vue` - console 重定向到日志面板、截图测试按钮
- `scripts/screenshot-test/` - 独立截图验证程序（带 10s 死锁检测）
- 诊断工具：`FrontendEcho` 绑定方法（确认前端脚本是否加载成功）

Changed:
- `main.go`
  - embed 资源改为 `frontend/dist`，用 `fs.Sub(assets, "frontend/dist")` 修正根路径
  - 默认快捷键从 Ctrl+Space 改为 **Ctrl+Alt+R**（VK_R=82，注册在 main.go）
  - OnStartup 中调用 `agent.SetRuntimeContext(ctx)` 注入 Wails 运行时上下文
  - quick input toggle 时调用 WindowUnminimise + WindowShow 前置主窗口
- `internal/agent/bindings.go` - **移除所有绑定方法的 context.Context 参数**（见 ADR-010）
- `internal/input/hotkey_windows.go` - 重写为专用线程模型（见 ADR-011）
- `internal/tray/tray_windows.go` - systray ready channel 同步，消除初始化竞态
- `wails.json` - 前端命令对接 Vite（dev:watcher / dev:serverUrl auto）

Removed:
- 删除旧 `ui/` 原生 JS 前端（已由 `frontend/` 完全替代）

Fixed:
- embed 报 "no index.html"：embed 路径 + fs.Sub 修正
- 消息窗口创建失败 "Invalid window handle"：`HWND_MESSAGE` 应为 `(HWND)-3`（`^uintptr(2)`），非 0xffff；需传 hInstance
- RegisterHotKey "Invalid window; belongs to other thread"：窗口创建、消息循环、热键注册必须同一线程
- goroutine 线程迁移导致消息循环失效：`runtime.LockOSThread` 专用线程 + PostMessage(WM_APP) 跨线程调度
- systray "Invalid menu handle"：Run 异步未就绪时添加菜单项，加 ready channel + waitReady
- 前端完全不执行：原生 JS module 不支持 `import './style.css'`（Vue 迁移后消除）
- 截图 panic "index out of range"：DEVMODE 结构体字段位宽错误（uint16 应为 uint32）导致分辨率读到 0，改用 GetSystemMetrics
- GetDIBits "parameter is incorrect"：hdc 不能传 NULL；BitBlt 后需先 SelectObject 恢复旧位图再 GetDIBits
- **截图按钮永久"截图中"**：Wails v2.15 对绑定方法 ctx 参数传 null 触发 `reflect: Call using zero Value argument` 且错误被吞，Promise 永远 pending（ADR-010）
- 多实例运行导致热键 "already registered" 误判

Notes:
- 开发环境已切换为 **Windows 本地开发**（`wails build` / `wails dev`），不再依赖 macOS 交叉编译
- LLM 已配置火山引擎豆包 `doubao-seed-2.0-lite`（OpenAI 兼容，ARK API），流式对话验证通过
- E2E 验证通过：热键（模拟按键）、托盘菜单、流式对话、全屏截图（1920x1080，约 100ms）
- Go `[]byte` 经 Wails 序列化到前端直接是 base64 字符串，无需手动转换

---

## 2026-08-25

### Phase 3: LLM Provider Architecture + Agent Core

Added:
- 创建 `internal/llm/` LLM Provider 架构
  - `llm.go` - 定义核心接口 `LLMProvider`、`ChatCompletionRequest`、`ChatMessage`、`TokenUsage`
  - `factory.go` - Provider 工厂，使用注册模式避免循环导入
  - `internal/llm/providers/openai/` - OpenAI 兼容 Provider 实现
  - 支持非流式和流式两种响应模式
- 创建 `internal/agent/` Agent Core
  - `agent.go` - Agent 核心，管理对话历史，调度 LLM
  - `bindings.go` - Wails bindings 供前端调用
- 扩展 `internal/errors/errors.go` - 新增 LLM 相关错误码
- 扩展 `internal/config/config.go` - 新增 LLM 配置字段
- 扩展 `internal/log/log.go` - 添加 `RandomID` 工具函数
- 更新前端 `ui/src/main.js` - 添加流式对话显示逻辑

Changed:
- `main.go` - 集成 Agent Core，注册 bindings，添加到生命周期清理
- `docs/ai/CURRENT_STATE.md` - 更新当前状态

Architecture:
- 严格遵循 Provider 抽象原则：Agent Core 只依赖接口，不依赖具体实现
- 工厂使用注册模式，避免循环导入
- 支持 OpenAI 标准接口，也兼容火山引擎豆包等 OpenAI 兼容 API
- 同时支持非流式和流式响应，第一阶段实现完整流式
- API Key 通过配置文件保存，从不硬编码，从不写入日志，从不返回前端
- 错误信息不会泄露敏感信息
- 通过 context cancellation 支持请求取消
- 遵循所有 ADR 架构决策，保持单体应用，不创建 HTTP 服务

Security:
- ✅ API Key 不硬编码，由用户配置
- ✅ API Key 不写入日志
- ✅ API Key 不返回给前端
- ✅ 错误信息不泄露 API Key

Notes:
- 已实现完整闭环：用户输入 → Input → Agent → LLM Provider → API → Agent → UI
- 支持流式响应，逐字显示
- 保持对话历史，支持多轮对话
- 可以通过配置选择 Provider，新增 Provider 不需要修改核心代码

---

## 2026-08-25

### Phase 2: Input Module - Global Hotkey + Quick Input

Added:
- 创建 `internal/input/` 输入模块
  - `input.go` - InputManager 统一输入管理器
  - `hotkey.go` - HotkeyListener 接口定义
  - `hotkey_windows.go` - Windows 全局快捷键实现（RegisterHotKey API）
  - `hotkey_default.go` - 非Windows平台空实现
  - `quickinput.go` - 快速输入状态管理器
  - `bindings.go` - Wails bindings 供前端调用
- 前端添加快速输入弹出层
  - `ui/src/index.html` - 添加弹出层HTML
  - `ui/src/main.js` - 添加键盘事件处理
  - `ui/src/style.css` - 添加弹出层样式

Changed:
- `main.go` - 集成Input模块，注册默认快捷键 Ctrl+Space，注册生命周期清理
- `internal/errors/errors.go` - 新增错误码 CodeInputError
- `docs/ai/CURRENT_STATE.md` - 更新当前状态

Architecture:
- Windows 全局快捷键使用 RegisterHotKey API + 隐藏消息窗口
- 不使用低级钩子，避免杀毒软件误报
- Windows代码通过条件编译 `//go:build windows` 隔离
- 非Windows保持编译可用，空实现占位
- 快速输入使用主应用内弹出层，不创建独立窗口
- 遵循 Provider 抽象原则，核心依赖接口不依赖实现

Fixed:
- N/A

Notes:
- 默认快捷键 Ctrl+Space 已经注册
- 所有资源在退出时正确释放，防止泄漏
- 支持重复打开，防止快捷键重复注册
- 满足所有需求：全局监听、失去焦点也能触发、Enter提交、Esc关闭、不退出程序

## 2026-08-24

### Phase 1: Project Skeleton Initialization

Added:
- 初始化 Go module `github.com/lipipi/game-ai`
- 创建项目目录结构
- 添加 app/cmd 命令行处理
- 添加 internal/config 配置管理
- 添加 internal/errors 统一错误处理
- 添加 internal/log 日志系统
- 添加 internal/lifecycle 生命周期管理
- 添加 internal/platform 平台抽象层（条件编译）
- 添加 internal/tray 系统托盘（条件编译）
- 创建 ui/src 前端骨架页面
- 创建 main.go Wails 主入口
- 创建 wails.json Wails 配置
- 创建 scripts/build-windows.sh 交叉编译脚本
- 创建 .gitignore
- 创建 README.md 开发文档
- 创建 docs/ai/ AI 开发记忆系统
  - PROJECT.md - 项目概要
  - ARCHITECTURE.md - 架构文档
  - DECISIONS.md - 架构决策记录
  - CURRENT_STATE.md - 当前状态
  - CHANGELOG.md - 本文件
  - DEVELOPMENT_GUIDE.md - AI 开发指南

Architecture:
- 采用 Go + Wails 架构
- 平台抽象通过 Go 条件编译实现
- 系统托盘 Windows 使用 systray，其他平台空实现
- 配置使用 JSON 文件，跨平台路径处理
- 错误处理统一错误码和堆栈
- 日志同时输出控制台和文件
- 遵循所有 ADR 架构决策

Changed:
- N/A (初始化项目)

Fixed:
- N/A (初始化项目)

Notes:
- 所有基础设施完成，可以开始下一阶段功能开发
- 项目可以正常编译运行
- `wails dev` 可以正常启动开发模式
