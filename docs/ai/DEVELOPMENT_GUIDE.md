# GameAI - AI 开发指南

本文档定义了 AI Agent 开发 GameAI 项目时必须遵守的规则。

## 开发流程

### 每次开始工作前

1. **阅读 `docs/ai/PROJECT.md`** - 理解项目目标和约束
2. **阅读 `docs/ai/ARCHITECTURE.md`** - 理解当前架构和模块依赖
3. **阅读 `docs/ai/DECISIONS.md`** - 理解已经做出的架构决策
4. **阅读 `docs/ai/CURRENT_STATE.md`** - 理解当前实际状态，哪些完成了哪些没做
5. **阅读 `docs/ai/CHANGELOG.md`** - 理解最近变更
6. **检查实际代码状态** - `git status`，看有没有未提交修改
7. **理解清楚需求后再开始编码** - 不猜测，不假设，有疑问问用户

### 每次完成工作后

1. **运行构建** - `go build .` 确保编译通过
2. **修复所有编译错误** - 不提交不编译的代码
3. **检查架构约束** - 确认没有违反 ADR 决策
4. **更新 `docs/ai/CURRENT_STATE.md`** - 更新已完成/进行中
5. **更新 `docs/ai/CHANGELOG.md`** - 记录重要变更
6. **如果有新的重要架构决策，更新 `docs/ai/DECISIONS.md`** - 追加新 ADR
7. **汇报修改了哪些文件** - 清晰列出
8. **汇报测试结果** - 构建是否成功
9. **汇报已知问题** - 诚实说清楚哪些还没做，哪些有问题
10. **停止** - 不自动进入下一阶段

## AI Agent 行为规则

### 必须严格遵守

✅ **允许**:
- 在现有架构下增加功能
- 修复 Bug
- 重构已有代码保持架构不变
- 提出架构改进建议

❌ **禁止**:

- **不要为了方便随意增加第三方依赖** - 评估必要性，能不用就不用
- **不要擅自引入微服务** - 违反单体应用决策
- **不要擅自引入 Docker** - 用户不需要
- **不要擅自引入 Python runtime** - 最终产物不能依赖 Python
- **不要擅自引入 Node runtime** - 最终产物不能依赖 Node
- **不要创建 localhost HTTP server** 来规避模块设计 - 模块之间直接调用
- **不要为了完成任务破坏已有架构** - 如果需要改架构先说明
- **如果发现架构存在问题，先说明，不要静默重构** - 让用户决策
- **不要删除已有功能来完成新功能** - 增量开发
- **不要修改已有 API 行为，除非明确说明要改** - 向后兼容
- **不要把 API Key 写入代码或日志** - 配置由用户通过 UI 输入
- **Windows 特定代码必须与跨平台代码隔离** - 使用 `//go:build windows`
- **不要假设未来功能已经实现** - 只基于当前真实代码工作
- **不要把 TODO 当成已经完成的功能** - CURRENT_STATE 必须如实描述

## 阶段开发原则

一次只完成一个明确阶段。

**完成当前阶段任务后，停止。等待下一条指令。**

不要自动推测用户想要下一阶段做什么，不要自动进入下一阶段。

## 架构设计规则

1. **接口定义在核心层** - Provider 接口在 `internal/xxx`，具体实现在 `internal/xxx/providers`
2. **核心依赖接口，不依赖实现** - Agent Core 只依赖 Provider 接口
3. **Windows 代码必须隔离** - 条件编译，不污染跨平台代码
4. **新增游戏支持走 GamePlugin** - 不修改核心
5. **新增 AI 厂商走 Provider** - 不修改核心

## 平台/框架已知陷阱（踩过的坑，勿重蹈）

1. **Wails 绑定方法禁止声明 `context.Context` 参数**（ADR-010）- v2.15 传 null 触发 reflect 错误且被吞掉，前端 Promise 永久 pending。运行时上下文用 `agent.SetRuntimeContext(ctx)` 在 OnStartup 注入。
2. **Win32 消息窗口必须线程绑定**（ADR-011）- 窗口创建/GetMessage/RegisterHotKey 必须同一 `LockOSThread` 线程；跨线程操作用 `runOnWindowThread` 调度。
3. **`HWND_MESSAGE` 是 `(HWND)-3`** 即 `^uintptr(2)`，不是 0xffff。
4. **GDI 截图**：GetDIBits 的 hdc 参数不能传 NULL；BitBlt 后先 SelectObject 恢复旧位图再 GetDIBits；显示器尺寸用 GetSystemMetrics，别用 DEVMODE（结构体布局易错）。
5. **systray.Run 是异步的** - 添加菜单项前必须等 ready channel。
6. **go:embed 前缀问题** - embed 目录带路径前缀时必须 `fs.Sub` 取子目录作为根。
7. **Go `[]byte` 到前端** - Wails 序列化后直接是 base64 字符串。
8. **调试手段**：`FrontendEcho` 确认前端是否加载；Wails 吞 ERR 日志时可重定向 stdout 捕获；日志文件在 `%APPDATA%/GameAI/`。

## 提交原则

- 一个阶段一个提交
- 不把多个阶段功能混在一起
- 每个文件修改有明确目的

## 记忆文档原则

- 记忆文档只记录决策、状态、约束
- 代码才是事实来源
- 不要把大量代码复制进记忆文档
- 保持文档简洁
- 当前状态必须如实反映代码现状
- 不要把计划写成已经完成
