# Aurora - AI 开发执行指南

本文档定义 AI Agent 开发 Aurora 时的**执行流程与操作规范**。

说明"如何执行任务"，不重复定义长期规则。

---

## 一、文档职责（三层信息体系）

```text
.claude/CLAUDE.md
    ↓
长期、简洁、强约束（架构不变量、Token 策略、测试纪律）
AI 执行任务时优先读取

docs/ai/
    ↓
项目事实、架构、决策、当前状态、历史
按任务相关性选择读取

DEVELOPMENT_GUIDE.md（本文件）
    ↓
开发执行流程与操作规范
说明"如何执行任务"
```

**禁止在多个文件重复维护同一份规则。**

如果规则已存在于 `.claude/CLAUDE.md`，本文件只引用其原则，不复制大段内容。

---

## 二、每次开始工作前

### 1. 优先读取 `.claude` 长期规则

```text
.claude/CLAUDE.md
```

如果存在，优先读取。其中的规则优先于本文件。

### 2. 根据任务相关性选择 docs

**不要机械读取全部。** 只读取当前任务真正相关的：

```text
PROJECT.md        — 项目目标与约束
ARCHITECTURE.md   — 当前架构与模块依赖
DECISIONS.md      — 架构决策（ADR）
CURRENT_STATE.md  — 当前状态、已完成、进行中
CHANGELOG.md      — 历史变更
```

**按任务类型选择：**

| 任务类型 | 建议读取 |
|----------|----------|
| 小 Bug 修复 | 相关代码 + CURRENT_STATE（如需上下文） |
| 架构任务 | ARCHITECTURE + DECISIONS |
| Phase 进度任务 | CURRENT_STATE |
| 新功能 | 根据涉及模块选择 |
| 纯文档修改 | 通常无需读 docs |

### 3. 检查实际代码

```bash
git status
```

然后只定位与任务相关的代码。

**优先代码 > 文档。** 文档可能过期，代码是事实来源。

---

## 三、每次完成工作后

1. **运行适当范围的构建与测试**（见第六节）
2. **修复所有编译错误** — 不提交不编译的代码
3. **检查架构约束** — 确认没有违反 ADR / `.claude` 规则
4. **更新 `docs/ai/CURRENT_STATE.md`** — 更新已完成/进行中
5. **更新 `docs/ai/CHANGELOG.md`** — 记录重要变更
6. **如果有新的重要架构决策，更新 `docs/ai/DECISIONS.md`** — 追加新 ADR
7. **汇报** — 修改了哪些文件、测试结果、已知问题
8. **停止** — 不自动进入下一阶段

---

## 四、Token 与上下文控制

AI Agent 必须优先考虑上下文和 Token 成本。

### 核心原则

> **最小必要上下文完成任务。**

不要为了"完整理解"而消耗没有必要的 Token。

### 必须遵守

- 不要为了"了解项目"读取整个仓库
- 不要递归阅读所有源文件
- 不要重复读取已经确认过的文件
- 优先使用目录结构、grep、符号搜索定位目标
- 先定位，再深入阅读
- 只加载与当前任务直接相关的代码
- 不要把大型日志、生成文件、构建产物加载进上下文
- 不要为了一个小修改重新阅读整个项目
- 已经确定的事实不要反复验证
- 修改前理解边界，修改后只验证受影响范围 + 必要的全局回归

---

## 五、文档更新原则

| 文档 | 职责 |
|------|------|
| `.claude/CLAUDE.md` | 长期 AI 行为规则（跨 Phase 稳定） |
| `docs/ai/CURRENT_STATE.md` | 当前真实状态 |
| `docs/ai/CHANGELOG.md` | 历史变更（按时间追加） |
| `docs/ai/DECISIONS.md` | 架构决策（ADR，追加不改旧） |
| `docs/ai/ARCHITECTURE.md` | 当前架构描述 |
| `DEVELOPMENT_GUIDE.md` | 开发执行流程（本文件） |

**禁止：**

> 在多个文件重复维护同一份规则。

如果规则已存在于 `.claude`，本文件只引用其原则，不复制大段内容。

---

## 六、构建与验证

根据修改类型决定验证范围：

| 修改类型 | 验证方式 |
|----------|----------|
| 纯文档 | 通常不需要构建 |
| 小范围代码修改 | `go build ./...` + 相关包测试 |
| 接口 / 架构修改 | `go build ./...` + `go test ./...` |
| 前端修改 | `wails build` 或按需 |

**默认：**

```bash
go build ./...
go test ./...
```

如果项目有更合适的构建命令（如 `wails build`），根据任务需要执行。

**不要为了每一个纯文档修改都执行完整构建。**

**也不要在未完成代码的情况下提交。**

---

## 七、架构设计规则

### 基本原则

1. **接口定义在核心层** — Provider 接口在 `internal/xxx`，具体实现在 `internal/xxx/providers`
2. **核心依赖接口，不依赖实现** — Agent Core 只依赖 Provider 接口
3. **Windows 代码必须隔离** — 条件编译（`//go:build windows`），不污染跨平台代码
4. **新增游戏支持走 GamePlugin** — 不修改核心
5. **新增 AI 厂商走 Provider** — 不修改核心

### Game / Core 解耦（详见 CLAUDE.md §2）

Aurora Core 不依赖具体游戏实现。

Agent 不得直接依赖 `internal/game/plugins/*`。

通用能力必须通过：

- Generic Tool
- Game Contract（`internal/game`）
- Provider Interface
- Plugin Interface（`GameContextRenderer` / `ToolContributor` / `SetupReceiver`）

进行连接。

具体游戏逻辑只能存在于对应 Game Plugin / Specialist 层。

新增游戏的目标：

> **尽可能增加 Plugin，而不是修改 Aurora Core。**

不允许因为 TFT 当前功能最丰富，就把 Aurora Core 设计成 TFT-centric。

---

## 八、Universal Game AI 定位

Aurora 的目标：

> **Universal Game AI Companion**

核心能力应保持游戏无关：

```text
Speech / LLM / Agent / Tool Calling / Screenshot / Vision
TTS / Memory / Input / Lifecycle
```

游戏专属能力：

```text
TFT / LOL / 斗地主 / 黑神话 / 未来其他游戏
```

必须通过 Plugin / Specialist 提供。

新增游戏时：

> **优先验证"插件是否足够"，而不是修改 Core。**

如果增加新游戏需要修改 Agent Core，优先怀疑架构边界，不优先怀疑工作量。

---

## 九、Windows 发布原则

Aurora 是正常的 Windows Desktop Application。

**不要求 Single EXE。**

允许：

```text
EXE / DLL / resources / models / plugins
runtime dependencies / helper processes
```

只要整体方案稳定、简单、可维护、用户安装体验良好。

**不要为了单 EXE：**

- 复杂化 build pipeline
- 强行静态链接
- 修改第三方库
- 嵌入不适合嵌入的 runtime
- 增加不必要的启动器
- 增加维护成本

**发布优先级：**

1. Correctness
2. Stability
3. Development speed
4. Runtime performance
5. Maintainability
6. User installation experience
7. Single-EXE 여부 — 不是要求

详见 ADR-028。

---

## 十、第三方 Runtime 规则

不要为了方便擅自要求用户预装 Python runtime。

不要为了方便擅自要求用户预装 Node runtime。

如果某个功能确实需要 runtime，必须评估：

- 是否可以避免
- 是否可以随应用一起分发
- 是否有更简单的原生方案
- 对安装、性能、维护和发布的影响

不得仅因为"单文件 EXE"而拒绝合理的外部 runtime / DLL / resource / helper process。

最终 Windows 发布方案以：

> 正确性、稳定性、开发速度、性能、维护成本、用户体验

为优先。**不要求 Single EXE。**

用户不应需要手动安装 Python / Node.js / Docker 等**开发运行时**。

---

## 十一、架构变更规则

### Scope 内修复，Scope 外先确认

如果任务本身要求解决架构问题，允许在当前任务范围内进行必要的架构修改。

如果修改会：

- 扩大当前 Phase 范围
- 改变已有 ADR
- 改变核心模块边界
- 引入新的重大技术路线

**必须先向用户说明。**

禁止为了完成一个小任务而顺手进行无关的大规模重构。

---

## 十二、Phase 不是永久架构

```text
Phase 是项目开发阶段。

Phase 中的实现不能自动升级为长期架构原则。
```

只有：

- ADR（`docs/ai/DECISIONS.md`）
- `.claude` 长期规则（`CLAUDE.md`）
- 明确确认的架构决策

才属于长期约束。

---

## 十三、提交原则

- 一个阶段一个提交
- 不把多个阶段功能混在一起
- 每个文件修改有明确目的

如果用户明确要求多个独立修改一起完成，可以根据实际任务组织提交，不得为了形式制造无意义 commit。

---

## 十四、AI Agent 行为规则

### 允许

- 在现有架构下增加功能
- 修复 Bug
- 重构已有代码保持架构不变
- 提出架构改进建议

### 禁止

- 不要为了方便随意增加第三方依赖 — 评估必要性
- 不要擅自引入微服务 — 违反单体应用决策
- 不要创建 localhost HTTP server 来规避模块设计 — 模块之间直接调用
- 不要为了完成任务破坏已有架构 — 如果需要改架构先说明
- 不要删除已有功能来完成新功能 — 增量开发
- 不要修改已有 API 行为，除非明确说明要改 — 向后兼容
- 不要把 API Key 写入代码或日志 — 配置由用户通过 UI 输入
- 不要假设未来功能已经实现 — 只基于当前真实代码工作
- 不要把 TODO 当成已经完成的功能 — CURRENT_STATE 必须如实描述

---

## 十五、阶段开发原则

一次只完成一个明确阶段。

**完成当前阶段任务后，停止。等待下一条指令。**

不要自动推测用户想要下一阶段做什么，不要自动进入下一阶段。

---

## 十六、平台/框架已知陷阱（踩过的坑，勿重蹈）

1. **Wails 绑定方法禁止声明 `context.Context` 参数**（ADR-010）— v2.15 传 null 触发 reflect 错误且被吞掉，前端 Promise 永久 pending。运行时上下文用 `agent.SetRuntimeContext(ctx)` 在 OnStartup 注入。
2. **Win32 消息窗口必须线程绑定**（ADR-011）— 窗口创建/GetMessage/RegisterHotKey 必须同一 `LockOSThread` 线程；跨线程操作用 `runOnWindowThread` 调度。
3. **`HWND_MESSAGE` 是 `(HWND)-3`** 即 `^uintptr(2)`，不是 0xffff。
4. **GDI 截图**：GetDIBits 的 hdc 参数不能传 NULL；BitBlt 后先 SelectObject 恢复旧位图再 GetDIBits；显示器尺寸用 GetSystemMetrics，别用 DEVMODE（结构体布局易错）。
5. **systray.Run 是异步的** — 添加菜单项前必须等 ready channel。
6. **go:embed 前缀问题** — embed 目录带路径前缀时必须 `fs.Sub` 取子目录作为根。
7. **Go `[]byte` 到前端** — Wails 序列化后直接是 base64 字符串。
8. **调试手段**：`FrontendEcho` 确认前端是否加载；Wails 吞 ERR 日志时可重定向 stdout 捕获；日志文件在 `%APPDATA%/Aurora/`。
