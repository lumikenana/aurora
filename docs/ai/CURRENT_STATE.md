# GameAI - 当前状态

## Current Phase

**Phase 4 Completed. Waiting for next phase instruction.**

## Completed

✓ 项目目录结构、Go module、Wails v2 配置
✓ 配置管理 / 日志系统 / 统一错误处理 / 生命周期管理 / 平台抽象层
✓ 系统托盘（Windows 实现 + ready 同步）
✓ **Input 模块** - 全局快捷键（专用消息线程模型，ADR-011）
✓ **默认快捷键 Ctrl+Alt+R** 呼出快速输入框（已 E2E 验证）
✓ **快速输入弹出层** - Enter 提交 / Esc 关闭 / 提交后自动关闭
✓ **LLM Provider** - OpenAI 兼容实现，已配置火山引擎豆包 doubao-seed-2.0-lite（ARK API）
✓ **Agent Core** - 对话管理 + 流式响应（E2E 验证通过）
✓ **Screenshot 模块** - Windows GDI 全屏截图（1920x1080 约 100ms，E2E 验证通过）
✓ **前端 Vue 3 + Vite + TypeScript 迁移**（ADR-009），旧 ui/ 已删除
✓ Wails 绑定方法全部去除 ctx 参数（ADR-010）
✓ 单 EXE 构建（`wails build`，Windows 本地开发）
✓ **Tool 系统** - 统一 Tool 接口 + ToolRegistry + Agent Loop + screenshot/current_time 工具

## In Progress

None.

## Not Started

- [ ] 语音唤醒词检测 (openWakeWord)
- [ ] ASR Provider 接口和实现
- [ ] TTS Provider 接口和实现
- [ ] 窗口截图（当前仅全屏）+ 截图与 LLM 联动（带图提问）
- [ ] Tool 系统
- [ ] GamePlugin 系统
- [ ] SQLite 存储集成（对话历史保存）
- [ ] 设置 UI（API Key 配置界面）
- [ ] 开机启动等 platform 模块实际接线

## Known Issues

- 多实例同时运行会热键注册失败（"already registered"），属预期行为，测试时注意 taskkill 旧实例
- 非 Windows 平台热键/托盘/截图均为空实现（设计预期）
- 截图在多显示器场景未充分测试

## Important Constraints

- 必须保持单 EXE 输出（Node.js 仅构建期依赖，运行时不需要）
- 不能要求用户安装 Python / Node.js / Docker
- 不能拆分多个必须独立启动的进程
- 不能创建 localhost HTTP 服务用于内部模块通信
- Windows 特定代码必须用 `//go:build windows` 隔离
- **Wails 绑定方法禁止声明 context.Context 参数**（ADR-010）
- AI 服务必须 Provider 抽象，不能绑定死厂商
- 游戏支持必须 GamePlugin 抽象，不能耦合特定游戏
- 必须按需截图，不能持续分析画面
- Go 负责核心逻辑，前端只负责 UI

## Current Architecture

```
main.go (Wails入口, embed frontend/dist, OnStartup 注入 runtime ctx)
  └── app/cmd (命令行处理)
  └── internal/
       ├── config (✓ 配置管理)
       ├── errors (✓ 统一错误)
       ├── log (✓ 日志系统)
       ├── lifecycle (✓ 生命周期)
       ├── platform (✓ 平台抽象，条件编译)
       ├── tray (✓ 系统托盘，ready 同步)
       ├── input (✓ 热键 + 快速输入，专用消息线程)
       ├── llm (✓ Provider 接口 + openai 兼容实现)
       ├── agent (✓ Agent Core + Wails bindings)
       └── screenshot (✓ GDI 全屏截图)
frontend/ (✓ Vue 3 + Vite + TypeScript)
scripts/ (✓ screenshot-test 独立验证程序)
docs/ai/ (✓ AI 记忆系统)
```

## Next Step

候选方向（按用户指令选择）：

1. **截图 + LLM 联动**: 快捷键截图提问（截图附带给多模态 LLM）
2. **语音链路**: ASR / TTS Provider + openWakeWord
3. **设置 UI**: API Key 配置界面
4. **SQLite 持久化**: 对话历史保存

## Validation

### 构建

**Windows 本地（当前开发环境）**:
```bash
cd frontend && npm install && npm run build   # 前端（wails build 会自动执行）
wails build                                    # 输出 build/bin/GameAI.exe
```

**开发模式**:
```bash
wails dev
```

### 已验证（2026-08-25）

- `wails build` ✓ 单 EXE 产出
- 全局热键 Ctrl+Alt+R → 快速输入弹窗 ✓（模拟按键 E2E）
- 托盘菜单（快速提问/退出）✓
- 流式对话（豆包 doubao-seed-2.0-lite）✓
- 全屏截图 1920x1080 / 172KB / ~100ms ✓（按钮 E2E + 独立程序）
- 日志面板实时输出 ✓（log 事件 + console 重定向）

### 排障入口

- 日志文件: `%APPDATA%/GameAI/gameai_YYYYMMDD.log`（config 中 log_level=debug 时含诊断日志）
- `FrontendEcho` 绑定方法可确认前端脚本是否加载
- Wails 吞掉部分 ERR 日志时，可重定向 stdout 捕获
