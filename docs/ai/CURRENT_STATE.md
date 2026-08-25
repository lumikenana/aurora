# GameAI - 当前状态

## Current Phase

**Phase 3 Completed. Waiting for next phase instruction.**

## Completed

✓ 项目目录结构创建完成
✓ Go module 初始化
✓ Wails v2 项目配置
✓ 基础窗口创建
✓ 系统托盘接口架构（Windows实现 + 非Windows空实现）
✓ 应用生命周期管理（信号处理 + 退出钩子）
✓ 配置管理接口（JSON + 跨平台路径）
✓ 日志系统（多级别 + 文件+控制台）
✓ 统一错误处理（错误码 + 堆栈 + Wrap）
✓ 平台抽象层（Go 条件编译分离 Windows/非Windows）
✓ Windows 开机启动注册表操作实现
✓ README 文档（开发 + 交叉编译说明）
✓ macOS 交叉编译 Windows 脚本
✓ 可以编译通过
✓ `wails dev` 可以启动
✓ **Input 模块架构设计** - InputManager + HotkeyListener + QuickInputManager
✓ **Windows 全局快捷键实现** - 使用 Windows RegisterHotKey API
✓ **跨平台空实现** - 非Windows平台保持编译可用
✓ **默认快捷键 Ctrl+Space 注册**
✓ **快速输入弹出层 UI** - 前端实现，支持Enter提交/Esc关闭
✓ **Wails Events 事件通知** - Go触发显示弹窗
✓ **生命周期资源释放** - 退出自动注销所有热键
✓ **LLM Provider 统一接口** - ChatRequest/ChatMessage/ChatResponse/TokenUsage
✓ **OpenAI 兼容 Provider 实现** - 支持标准 OpenAI 和火山引擎豆包
✓ **Agent Core 对话管理** - 对话历史管理，LLM 调度，流式响应
✓ **完整闭环** - 输入 → Agent → LLM → UI
✓ **流式响应架构** - 支持逐字显示

## In Progress

None. Phase 3 completed.

## Not Started

- [ ] 语音唤醒词检测 (openWakeWord)
- [ ] ASR Provider 接口和实现
- [ ] TTS Provider 接口和实现
- [ ] Screenshot 模块
- [ ] Tool 系统
- [ ] GamePlugin 系统
- [ ] SQLite 存储集成（对话历史保存）
- [ ] 设置 UI（API Key 配置界面）
- [ ] openWakeWord 集成

## Known Issues

- 无代码逻辑问题
- 当前只是项目骨架，AI 功能都未实现
- Windows 特定功能（系统托盘、开机启动）在非 Windows 平台是空实现，这是设计预期

## Important Constraints

- 必须保持单 EXE 输出
- 不能要求用户安装 Python / Node.js / Docker
- 不能拆分多个必须独立启动的进程
- 不能创建 localhost HTTP 服务用于内部模块通信
- Windows 特定代码必须用 `//go:build windows` 隔离
- AI 服务必须 Provider 抽象，不能绑定死厂商
- 游戏支持必须 GamePlugin 抽象，不能耦合特定游戏
- 必须等唤醒词检测到才启动云端 ASR
- 必须按需截图，不能持续分析画面
- Go 负责核心逻辑，前端只负责 UI

## Current Architecture

```
main.go (Wails入口)
  └── app/cmd (命令行处理)
  └── internal/
       ├── config (✓ 配置管理)
       ├── errors (✓ 统一错误)
       ├── log (✓ 日志系统)
       ├── lifecycle (✓ 生命周期)
       ├── platform (✓ 平台抽象，条件编译)
       │    ├── platform.go (接口)
       │    ├── platform_windows.go
       │    └── platform_default.go
       └── tray (✓ 系统托盘，条件编译)
            ├── tray.go (接口)
            ├── tray_windows.go
            └── tray_default.go
ui/ (✓ 前端静态页面骨架)
storage/ (✓ 预留存储目录)
scripts/ (✓ 交叉编译脚本)
docs/ai/ (✓ AI 记忆系统)
```

所有 AI 相关模块（Agent Core、LLM、ASR、TTS、Input、Screenshot、GamePlugin）都是 **[PLANNED]**，尚未实现。

## Next Step

下一阶段应该选择一个方向开始实现：

选项建议：

1. **Input 模块**: 实现全局快捷键 + openWakeWord 集成
2. **Provider 层**: 先实现火山引擎 ASR / TTS / LLM Provider
3. **Agent Core**: 实现对话状态管理和基本流程
4. **UI 设置界面**: 实现 API Key 配置界面

根据优先级，推荐下一步做：**Provider 层接口定义 + 火山引擎实现**，或者 **Input 模块**。

## Validation

### 构建校验

**macOS 开发构建**:
```bash
go build -o GameAI
./GameAI
```

**开发模式运行**:
```bash
wails dev
```

**Windows 交叉编译**:
```bash
./scripts/build-windows.sh
```

输出文件: `build/bin/GameAI.exe`

### 人工验证

- 启动后显示主窗口
- 日志输出正常
- 配置文件写入正确位置
- 平台检测正确
- 不崩溃

### 当前状态验证

2026-08-24 测试:
- `go build` ✓ 编译通过
- `wails dev` ✓ 启动正常，窗口显示欢迎页面
- 日志输出 ✓ 正确
