# GameAI - 开发变更日志

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
