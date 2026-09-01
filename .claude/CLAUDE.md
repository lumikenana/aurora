# Aurora — Claude Project Memory

本文件是 Aurora（`github.com/lipipi/aurora`，当前目录 `aurora/`）的长期工程规则。
Claude 在本项目执行任何任务前应读此文件；执行任务中如遇规则冲突，**本文件优先于默认行为**。

> 本文件只记录**长期不变**的工程约束。
> 阶段进度、Bug 列表、当前 TODO、变更日志 → `docs/ai/`。
> 代码本身 > 本文件 > `docs/ai/` 旧章节。

---

## 0. 项目身份

- **项目名**: Aurora（仓库根 `aurora/`，Go module `github.com/lipipi/aurora`）
- **形态**: 单进程 Go 后端 + Wails v2 + Vue 3 前端，Windows 桌面应用（不要求单 EXE；发布方案按稳定性、开发速度、性能、维护成本和用户体验选择最优方案）
- **核心栈**: Go / Wails v2 / Vue 3 + Vite + TypeScript / Windows API / OpenAI 兼容 LLM / 火山引擎 ASR+TTS
- **当前焦点**: LLM 游戏助手（已实现：PTT、ASR、TTS、Tool 系统、TFT 游戏插件 + GameContext 采集 + KB 注入）

---

## 1. `.claude` 与 `docs/ai` 的职责分离（不可混淆）

| 内容 | 归属 | 原因 |
|------|------|------|
| 长期工程规则 / 架构不变量 / Token 策略 / 测试纪律 | **`.claude/`**（本文件） | 跨 Phase 稳定 |
| 项目当前状态 / 阶段进度 / 当前 Bug / TODO | `docs/ai/CURRENT_STATE.md` | 高频过期 |
| 变更记录 | `docs/ai/CHANGELOG.md` | 按时间追加 |
| 架构说明 / 模块依赖图 | `docs/ai/ARCHITECTURE.md` | 描述"现在是什么" |
| 架构决策（带原因） | `docs/ai/DECISIONS.md` | 追加 ADR，不改旧 |
| 单次任务执行记录 | 不写 | 只在本次会话里 |

**禁止**在 `.claude/` 中写：

- 阶段号、Phase 状态、当前进度
- 临时 TODO、当前 Bug 列表
- 某次任务的执行记录
- 即将过期或只对当前 Phase 有效的"规则"

---

## 2. 架构不变量（Architectural Invariants）

### 2.1 依赖方向（必须保持）

```
Agent Core
  ↓
Generic Game Contract (internal/game)
  ↓
Game Plugin (internal/game/plugins/<game>)
  ↓
Game Domain (内部可任意具体)
```

**反向依赖（Plugin → Agent）属于架构事故。** 一旦出现循环依赖风险，停下来重新设计边界，不要用 hack 绕过去。

### 2.2 Agent Core 不得依赖任何具体游戏

`internal/agent/**` 中**禁止**：

- `import` `internal/game/plugins/<anything>`
- 出现具体游戏名（`"tft"`、`"lol"`、`"cs2"` 等）的硬编码分支
- 引用具体游戏结构体（`*tft.TFTGameContext`、`TFT Champion/Item/Trait/Composition` 等）
- 在 Agent 包内定义"只对某个游戏有意义"的工具（`tft_tools_wiring.go` 这类文件命名本身就是信号）

Agent 只能依赖：

- `internal/game` 的接口与通用结构（`GamePlugin` / `GameContext` / `Manager` / `ContextProvider`）
- `internal/game/knowledge` 的通用接口
- `internal/tool` 的通用接口
- `internal/llm` / `internal/screenshot` 等横切 Provider

> 通用层 `internal/game` / `internal/game/knowledge` 同样不得知道具体游戏细节
>（`Vayne` / `Infinity Edge` / TFT 利息规则 / TFT 商店等）。
> 这些只能存在于 `internal/game/plugins/<game>/**` 或 `internal/game/knowledge/<game>/**`（如 `knowledge/tft/`）。

### 2.3 新增游戏必须不改 Agent

新游戏 = 新插件包（实现 `game.GamePlugin`） + `main.go` blank import + `config.game.enabled`。
Agent 不知道、也不需要知道这个游戏存在。
**如果增加第二个游戏需要修改 Agent，优先怀疑架构边界，不优先怀疑工作量。**

### 2.4 Domain Logic 留在 Domain

TFT 的：

- 棋子 / 装备 / 羁绊 / 强化
- 阵容 / 经济 / 备战 / 利息
- 规则（升星成本、连胜金币、商店概率）
- 知识库 / Composition / Grounder / 装备分析

必须留在 `internal/game/plugins/tft/` 与 `internal/game/knowledge/tft/`（含 `composition/`、`equipment/`、`economy/`、`preparation/`、`gamestate/`、`grounder/` 等子包）。
**不要为"看起来方便"把 TFT domain 逻辑搬到 Agent 或 Generic 层。**

### 2.5 抽象是有成本的，不要为了抽象而抽象

新增 interface 之前必须能回答其中至少一个：

1. 真的存在多个实现？
2. 真的存在跨层依赖边界（不只是同包内）？
3. 是否降低耦合 / 提高可测性？
4. 是否长期有价值（不是为当前任务一时方便）？

如果只是为了"看起来更高级"，**不要创建**。
Domain 内部（如 `TFTGameContext` 内）允许直接用具体 struct，不必每个类型都有 interface。

---

## 3. LLM 使用原则

### 3.1 LLM 是推理与语言能力，不是本地计算器

下列事情**优先本地完成**，不要调用 LLM：

- 查找 / 匹配 / 归一化
- 计数 / 聚合 / 排序 / 过滤
- 指纹 / 缓存键计算
- 状态比较 / 规则评估
- 数据标准化 / ID 解析
- 知识库 grounding / 字段补全
- 装备合成表 / 利息计算 / 升星成本

LLM 适合：

- 推理（"现在该不该 D 牌"）
- 解释（"为什么这把要拉 8"）
- 理解模糊意图
- 自然语言生成
- 高层战略分析

### 3.2 理想链路

```
Raw State
  ↓
Local Processing（确定性）
  ↓
Structured State
  ↓
Minimal Context
  ↓
LLM
  ↓
Answer
```

而不是"raw state 直接扔 LLM，让 LLM 重新算一切"。

### 3.3 默认：减少 LLM 调用次数

优先 `1 local computation + 1 necessary LLM call`。
避免 `LLM → LLM → LLM → LLM` 链式调用，除非任务明确需要多阶段推理。

---

## 4. Token 是一级工程成本

任何决策都要考虑：**Context Token + Prompt Token + Output Token + LLM Call Count**。
不能只看"代码能不能跑"。

### 4.1 最小 Context

Agent 不应默认收到"完整 GameContext + 完整 KB + 完整历史 + 完整所有分析"。
应按用户问题 scope 选最小相关切片：

- 阵容问题 → Composition
- 装备问题 → Equipment + 相关英雄
- 经济问题 → Economy
- 商店/备战 → Preparation + 棋盘/备战席

**如果一个字段不影响当前任务，就不要发送。**
不要因为"多知道一点总没坏处"就塞无关数据。

### 4.2 不要让 LLM 重新推导已知信息

如果 `GameContext` / `KB` / `Cache` / `Analysis` 里已有：

- Gold = 32
- Level = 6
- Vayne ★2

不要让 LLM 再从自然语言 / 图像里推导。
直接以结构化字段注入。

### 4.3 缓存优先

重复计算 → 优先 fingerprint + 缓存 + 增量更新 + memoization，
而不是重算 / 重发 prompt。

### 4.4 增量分析

如果只有 `Gold` 变了，**不要**重算 `Composition / Equipment / Knowledge`。
每个分析模块的失效应只由它真正依赖的字段触发。
分析模块内部可以独立维护 fingerprint 缓存（参见 `internal/game/knowledge/composition` / `equipment` / `economy` / `preparation` 已有模式）。

### 4.5 缓存要简单

不要一开始就引入：

- 分布式缓存
- 复杂失效框架
- 复杂持久化层

默认：进程内 `sync.RWMutex` 单槽 / 简单 map / 文件级（按需），保证可解释、可测试、有明确依赖。

---

## 5. 数据保真

### 5.1 Unknown 保持 Unknown

如果数据不存在 / 未识别 / 未授权 → `Unknown` / `Unresolved` / `Unavailable`，保持这种状态。

**禁止**：

- LLM 猜测
- 模糊匹配凑数
- 跨版本 fallback
- 跨游戏 fallback

除非项目明确规定允许。

知识库缺数据 → 标 Unknown + 修数据 pipeline，**不要**让 LLM 编。

### 5.2 Patch / Set 隔离

对版本化知识的游戏（TFT 等）：

```
Game + Set + Patch + Canonical ID
```

必须保持明确隔离。**不能因为"名字看起来一样"就用其他 Patch 的数据**。
三道闸（已实践于 `internal/game/knowledge/grounder`）：

1. SQL 主键以 `set + patch` 复合
2. Service `queryScoped` 强制传入 scope
3. 内存 ID 前缀检查（`TFT18_*` vs `TFT16_*`）

如果 Patch 未知 → 跳过注入，不要降级到"最接近的 Patch"。

### 5.3 Value[T] 表达"未知 vs 已知零"

`HP = 0`（阵亡）≠ `HP 未识别`。
**不要**用零值 / 空串表达 Unknown。
项目已有 `game.Value[T]` 模式，新字段默认走 `Value[T]`，不要倒退到裸类型。

---

## 6. 范围控制（Scope Control）

每个任务开始前区分：

```
Required   ← 只实现这些
Optional   ← 显式说明 + 等待用户决定
Future     ← 不实现，不预留"以后做"的接口/参数
Out of Scope ← 记录到报告，不动手
```

**默认只实现 Required。** 完成当前任务后**停止**，等待下一条指令。
**不要**自动推测下一步，**不要**顺手修 unrelated bug / 顺手做 refactor。

如果发现：

- 业务架构问题（违反第 2 节不变量）
- 无关 bug
- 既存架构债

→ 写入最终报告，**不要**自动扩大当前任务。
除非它**直接阻塞当前任务或直接违反第 2 节**——此时停下来报告，等用户决策。

---

## 7. 遇到架构冲突的处理

如果当前实现违反第 2 节不变量（即使不是当前任务引入的）：

```
Stop
 ↓
Explain architectural conflict（哪条不变量 + 在哪 + 为什么严重）
 ↓
Propose minimal correction（最小修复范围）
 ↓
Wait for user decision
 ↓
Continue
```

**不要**为了快速完成 feature 继续往上堆代码。

---

## 8. 修改前先理解

执行任何任务前：

```
调用链 → 依赖关系 → 数据流 → 生命周期 → 接口边界 → 现有实现
```

不要看到一个问题就立即改代码。
架构问题尤其如此。

---

## 9. 复用优先

修改前先搜：

- 现有接口 / helper / resolver / analyzer / repository / utility / test
- 是否已有 Resolver / Repository / Cache / Context Builder

如果已有能力能复用 → **复用**，不要因为"当前任务方便"就重写一套。
重复造轮子是历史负担的来源之一。

---

## 10. 测试纪律

每个修改尽可能：

```
修改 → Targeted Tests → Package Tests → go test ./...
```

按影响范围执行 `go build ./...`，涉及前端时 `wails build`。

测试**不能只验证 Happy Path**，至少覆盖：

- Empty / Unknown / nil
- Invalid input
- Partial data
- Duplicate
- Boundary
- Patch / Version mismatch
- Cache hit / cache invalidation
- Fingerprint 变化 / 不变

**对跨游戏架构改动**：必须同时测试 generic contract 和至少一个具体游戏 plugin，避免只在 TFT 上跑通就收工。

---

## 11. 文档纪律

代码修改后：

| 改动类型 | 更新位置 |
|----------|----------|
| 架构 / 依赖方向 / 公共契约变化 / 重要设计决策 | `docs/ai/ARCHITECTURE.md` + `docs/ai/DECISIONS.md`（追加 ADR） |
| 阶段变化 / Feature 完成 / Bug 修复 | `docs/ai/CURRENT_STATE.md` + `docs/ai/CHANGELOG.md` |
| **只是本项目工程规则细化** | 评估后追加到本文件（必须满足"长期有效"） |

**不要**：

- 在 `.claude/` 写 Phase 进度
- 在 `docs/ai/` 写"长期工程规则"（应该在本文件）
- 把大量代码复制进文档
- 计划写成"已完成"

---

## 12. Claude 标准任务流程

每次任务默认遵循：

```
1. 读本文件
        ↓
2. 读相关 docs/ai 文档（按任务范围，不全读）
        ↓
3. 检视实际代码状态（git status + 关键文件）
        ↓
4. 识别架构 / 依赖影响（特别是第 2 节）
        ↓
5. 定义最小 Scope（Required / Optional / Future / OOS）
        ↓
6. 搜索可复用现有能力（第 9 节）
        ↓
7. 实现最小正确改动
        ↓
8. 增 / 改测试
        ↓
9. 跑验证（go build / go test / 按需 wails build）
        ↓
10. 更新对应文档（第 11 节）
        ↓
11. 报告：变更 + 原因 + 测试 + 架构影响 + Token 影响 + 遗留问题
```

不要跳过 **Inspect / Scope / Reuse / Test** 任何一步。

---

## 13. Token-efficient 自身

执行任务时**不要**：

- 读整个项目
- 读所有 docs / 全部历史
- 为理解小函数把整个 repo 塞进上下文

采用：搜索 → 相关文件 → 相关符号 → 相关调用方。
**不要**每次修改后重复读不变文件（已确认信息保留在上下文即可）。

输出**也不要**默认复制大段代码 / 完整文件 / 完整日志（除非用户明确要求）。
默认报告结构见第 12 节第 11 步。

---

## 14. 平台 / 框架已知陷阱（已踩，勿重蹈）

来自 `docs/ai/DEVELOPMENT_GUIDE.md` 的硬教训（必要时回查）：

1. Wails 绑定方法**禁止 `context.Context` 参数**（ADR-010）— v2.15 传 null 触发 reflect 错误并被吞，前端 Promise 永久 pending。运行时 ctx 走 `agent.SetRuntimeContext(ctx)` 在 `OnStartup` 注入。
2. Win32 消息窗口**必须线程绑定**（ADR-011）— `CreateWindow / GetMessage / RegisterHotKey` 同一 `LockOSThread` 线程；跨线程 `runOnWindowThread` 调度。
3. `HWND_MESSAGE = (HWND)-3 = ^uintptr(2)`，不是 `0xffff`。
4. GDI 截图：`GetDIBits` 的 hdc 不能 NULL；`BitBlt` 后先 `SelectObject` 恢复旧位图再 `GetDIBits`；显示器尺寸用 `GetSystemMetrics`，**不要**用 `DEVMODE`。
5. `systray.Run` 是异步的 — 加菜单项前必须等 ready channel。
6. `go:embed` 目录带前缀时必须 `fs.Sub` 取子目录作根。
7. Go `[]byte` 到前端 — Wails 序列化后是 base64 字符串。
8. 调试：日志文件在 `%APPDATA%/GameAI/`；`FrontendEcho` 确认前端是否加载；Wails 吞 ERR 时重定向 stdout 捕获。

---

## 15. 反例清单（出现即停下来）

下面这些是本项目**长期不被允许**的写法，命中即视为违反不变量：

- `internal/agent/**` 中 `import` 任何 `internal/game/plugins/**`
- `internal/agent/**` 中 `import` 任何具体游戏 domain 包
- `internal/agent/**` 中出现 `switch gameID` / `if game == "tft"` 分支
- `internal/agent/**` 中定义文件命名带具体游戏名（`*_tft_*.go`、`*_lol_*.go` 等）
- `internal/agent/**` 中对具体游戏 struct 做类型断言 / 强制类型转换
- 通用层（`internal/game` / `internal/game/knowledge`）中出现具体英雄名 / 装备名 / 羁绊名 / 游戏规则字面量
- Plugin → Agent 反向依赖
- Agent ↔ Plugin 循环依赖
- "为了看起来更高级" 而新增的 interface / wrapper / 抽象层
- 任务范围外顺手修的 bug / refactor
- 把"LLM 多知道一点没坏处"作为扩 Context 的理由
- `Unknown` 字段被填成零值 / 空串 / 猜的值

---

## 16. 长效规则更新门槛

本文件允许追加新规则，但**必须**满足：

- 跨多个未来任务长期有效
- 不会随阶段 / 项目进度过期
- 属于工程约束而非项目状态

如果只对当前 Phase 有效 → 放 `docs/ai/`，不要追加到这里。
**默认保守：能不增加规则就不增加。**

---

## 17. 快速自检（开始任务前默念）

- Agent 是否依赖了具体游戏？（否 → 继续；是 → 停）
- 这件事能否本地算？（能 → 别用 LLM）
- Context 是不是已经最小？
- 有没有可复用的现有实现？
- 改动是否在 Required 范围内？
- Unknown 是否保持 Unknown？
- Patch / Set 是否隔离？
- 新增的 interface 是否满足第 2.5 节四个问题之一？

---

> 本文件由 `bootstrap-claude-memory` 任务创建。
> 内容为长期工程规则，不含项目状态。
> 任何"现在做到第几期 / 当前 Bug / 临时 TODO"都不属于本文件。
