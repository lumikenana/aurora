# 语音模式重构计划 v2：从互斥模式到 Trigger + Turn + Conversation

**状态**：v2 草案（待 review）  
**日期**：2026-09-10（v2 修订）  
**作者**：Claude + lipipi 讨论

---

## 0. 重构边界声明

### 0.1 本次重构不做什么

1. **不重新设计 ASR 流式链路**  
   Aurora 当前已经使用流式 ASR（WebSocket 持续发送音频块，服务端实时返回识别结果）。本次重构不会将流式识别改为"录音后上传"模式。

2. **不重新实现 VAD**  
   当前使用的 ASR 服务已经具备 VAD 能力，能够判断"一句话什么时候结束"。本次重构不会在客户端实现本地 VAD。

3. **不重新设计 LLM/TTS 流式链路**  
   当前 LLM 和 TTS 已经是流式处理。本次重构不会改变这部分链路。

4. **不改变 Agent Core 架构**  
   Agent 的对话管理、工具调用、上下文管理等保持不变。

### 0.2 本次重构做什么

**核心目标**：将当前三种互斥的语音模式（PTT / WakeWord / Continuous）重构为**可组合的触发方式 + 统一的对话生命周期**。

具体来说：
- 明确三个核心概念：**Trigger**（如何触发输入）、**Turn**（一轮交互）、**Conversation**（多轮对话）
- 明确两个不同的生命周期：**ASR/VAD 的句子结束** vs **Conversation 的对话结束**
- 移除 Continuous 作为独立模式的概念，将其能力融入 Conversation 生命周期
- 用自然语言理解替代硬编码的退出关键词

---

## 1. 核心概念定义

### 1.1 Trigger：如何触发 Aurora

Trigger 是用户发起一次语音或文本输入的方式。当前支持的 Trigger：

| Trigger | 类型 | 行为 |
|---------|------|------|
| **PTT** | 手动触发 | 按住按键开始录音，松开结束本轮输入 |
| **WakeWord** | 被动触发 | 检测到唤醒词后，进入 Conversation 模式 |
| **Text** | 手动触发 | 直接在文本框输入并提交 |

**关键区别**：
- **PTT** 触发的是**单轮 Turn**：按住说话 → 松开 → 等待回复 → 回到 Idle
- **WakeWord** 触发的是**Conversation**：唤醒 → 多轮对话 → 超时或主动退出 → 回到 Idle
- **Text** 始终可用，不受 Trigger 配置影响（不是配置项）

### 1.2 Turn：一轮交互

Turn 是用户和 Aurora 之间的一次完整交互：

```
用户输入（语音或文本）
    ↓
ASR 流式识别（如果是语音）
    ↓
ASR/VAD 判断"这句话结束"
    ↓
将最终文本发送给 LLM
    ↓
LLM 流式生成回复
    ↓
TTS 流式播放回复（如果启用）
    ↓
Turn 结束
```

**关键点**：
- Turn 的"结束"由 ASR/VAD 决定（服务端 VAD，不是本地 VAD）
- Turn 的粒度是"一句话"，不是"一段对话"
- 每个 Turn 都是独立的 LLM 调用

### 1.3 Conversation：多轮对话

Conversation 是连续的多个 Turn：

```
Conversation 开始（由 WakeWord 触发）
    ↓
Turn 1：用户说话 → Aurora 回复
    ↓
等待用户继续说话（Conversation 仍然活跃）
    ↓
Turn 2：用户说话 → Aurora 回复
    ↓
...
    ↓
Conversation 结束条件：
  - 用户主动退出（自然语言表达）
  - 超时（30 秒无新输入）
  - 错误/异常
    ↓
回到 Idle
```

**关键点**：
- Conversation 由 WakeWord 触发进入
- Conversation 可以包含多个 Turn
- Conversation 的"结束"与 Turn 的"结束"是不同的概念
- PTT 不进入 Conversation，每次 PTT 都是独立的 Turn

---

## 2. 生命周期状态机

### 2.1 Conversation 状态

```
Idle
  ↓ (WakeWord 检测到)
Listening
  ↓ (ASR 返回最终结果)
Processing
  ↓ (LLM 开始生成回复)
Responding
  ↓ (TTS 播放完成)
Waiting
  ↓ (用户继续说话 / 超时 / 主动退出)
Listening 或 Idle
```

### 2.2 状态说明

| 状态 | 含义 | 持续时间 |
|------|------|----------|
| **Idle** | 没有活跃的 Conversation | 直到 WakeWord 触发 |
| **Listening** | 当前 Turn 正在接收用户输入 | 直到 ASR 返回最终结果 |
| **Processing** | 当前 Turn 已结束，LLM 正在生成回复 | 直到 LLM 开始输出 |
| **Responding** | Aurora 正在通过 TTS 播放回复 | 直到 TTS 播放完成 |
| **Waiting** | Conversation 仍然活跃，等待用户继续输入 | 直到用户说话或超时 |

### 2.3 超时机制

- **30 秒超时**只作用于 **Waiting → Idle** 的转换
- 含义：Conversation 已经活跃，但用户长时间没有新的输入
- **不是**：ASR 检测静音 30 秒（那是 VAD 的工作）

### 2.4 PTT 的特殊性

PTT 不进入 Conversation 状态机，它的流程是：

```
Idle
  ↓ (PTT 按下)
PTT_Active（暂停 WakeWord detector）
  ↓ (PTT 松开)
Turn（ASR → LLM → TTS）
  ↓ (Turn 完成)
Idle（恢复 WakeWord detector）
```

**关键区别**：
- PTT 是一次性的 Turn，不进入 Conversation
- PTT 期间 WakeWord detector 被暂停
- PTT 完成后回到 Idle，不是 Waiting

---

## 3. Trigger 行为详解

### 3.1 PTT 行为

```
用户按下 PTT 按键
    ↓
暂停 WakeWord detector（如果启用）
    ↓
开始流式 ASR（发送音频块到服务端）
    ↓
用户说话
    ↓
ASR 实时返回识别结果（interim）
    ↓
用户松开 PTT 按键
    ↓
结束 ASR 会话（等待最终结果）
    ↓
ASR 返回最终结果（definite）
    ↓
发送给 LLM
    ↓
LLM 生成回复
    ↓
TTS 播放回复
    ↓
恢复 WakeWord detector（如果启用）
    ↓
回到 Idle
```

**关键点**：
- PTT 是"按住说话，松开结束"
- PTT 不会自动进入 Conversation 模式
- PTT 完成后，系统回到 Idle，等待下一次触发

### 3.2 WakeWord 行为

```
Idle 状态
    ↓
WakeWord detector 持续监听
    ↓
检测到唤醒词（"Hey Aurora"）
    ↓
暂停 WakeWord detector（避免重复触发）
    ↓
进入 Conversation 状态（Listening）
    ↓
开始流式 ASR
    ↓
用户说话
    ↓
ASR/VAD 判断这句话结束
    ↓
ASR 返回最终结果
    ↓
进入 Processing 状态
    ↓
LLM 生成回复
    ↓
进入 Responding 状态
    ↓
TTS 播放回复
    ↓
进入 Waiting 状态
    ↓
等待用户继续说话（Conversation 仍然活跃）
    ↓
用户继续说话
    ↓
回到 Listening 状态
    ↓
...（重复 Turn）
    ↓
Conversation 结束条件满足
    ↓
回到 Idle 状态
    ↓
恢复 WakeWord detector
```

**关键点**：
- WakeWord 触发的是 Conversation，不是单轮 Turn
- Conversation 期间 WakeWord detector 被暂停
- Conversation 可以包含多个 Turn
- Conversation 结束后，WakeWord detector 恢复

### 3.3 PTT + WakeWord 并存

```
Idle 状态
    ↓
WakeWord detector 持续监听
    ↓
[情况 1] 用户说唤醒词
    ↓
进入 Conversation 模式
    ↓
...（多轮对话）
    ↓
Conversation 结束
    ↓
回到 Idle

[情况 2] 用户按下 PTT
    ↓
暂停 WakeWord detector
    ↓
PTT Turn（单轮）
    ↓
Turn 完成
    ↓
恢复 WakeWord detector
    ↓
回到 Idle（不进入 Conversation）
```

**关键区别**：
- PTT 在 Conversation 期间也可以触发（打断当前 Conversation）
- PTT 完成后不进入 Conversation，而是回到 Idle
- WakeWord 在 PTT 期间被暂停

---

## 4. 自然语言退出机制

### 4.1 退出工具设计

新增工具 `exit_conversation_mode`，用于结束当前 Conversation。

**工具定义**：
```go
type ExitConversationTool struct {
    conversationManager *ConversationManager
}

func (t *ExitConversationTool) Name() string {
    return "exit_conversation_mode"
}

func (t *ExitConversationTool) Description() string {
    return "结束当前的连续对话模式。仅当用户明确表达想要结束对话、停止交流、离开时调用。"
}
```

### 4.2 系统提示词指导

```
当用户明确表达想要结束对话时，调用 exit_conversation_mode 工具。

明确的退出表达：
- "今天就到这里吧"
- "我先去忙了"
- "就到这里"
- "我不聊了"
- "再见" / "拜拜"
- "结束对话"

不要在以下情况调用：
- "谢谢" / "好的" / "明白了"（这些只是确认，不是退出）
- 普通的问答（除非用户明确说要结束）

调用工具后，先完成当前 Turn 的回复，再结束 Conversation。
```

### 4.3 退出时序

```
用户说："今天就到这里吧"
    ↓
LLM 理解意图，调用 exit_conversation_mode 工具
    ↓
工具标记 Conversation 为"即将结束"（不立即结束）
    ↓
LLM 继续生成回复（"好的，再见！"）
    ↓
TTS 播放回复
    ↓
当前 Turn 完成
    ↓
Conversation 正式结束
    ↓
回到 Idle 状态
    ↓
恢复 WakeWord detector
```

**关键点**：
- 工具调用不会立即中断当前 Turn
- 当前 Turn 的回复会正常完成
- Turn 完成后，Conversation 才真正结束

---

## 5. 配置迁移

### 5.1 旧配置

```json
{
  "voice_mode": "continuous"
}
```

### 5.2 新配置

```json
{
  "triggers": {
    "ptt_enabled": true,
    "wakeword_enabled": true
  },
  "conversation": {
    "timeout_seconds": 30
  }
}
```

### 5.3 迁移策略

| 旧配置 | 新配置 | 行为变化 |
|--------|--------|----------|
| `voice_mode: "ptt"` | `ptt_enabled: true, wakeword_enabled: false` | 无变化 |
| `voice_mode: "wakeword"` | `ptt_enabled: true, wakeword_enabled: true` | **行为变化**：旧版是"唤醒后单轮"，新版是"唤醒后多轮 Conversation" |
| `voice_mode: "continuous"` | `ptt_enabled: true, wakeword_enabled: true` | **行为变化**：旧版是"立即开始监听"，新版是"等待唤醒后进入 Conversation" |

**重要说明**：
- `wakeword` 和 `continuous` 的迁移**不是等价的**
- 旧版 `continuous` 是"免唤醒，直接监听"
- 新版没有"免唤醒直接监听"的模式
- 如果用户需要类似行为，需要额外实现"自动进入 Conversation"的功能（不在本次重构范围内）

### 5.4 迁移代码

```go
func migrateVoiceConfig(cfg *Config) {
    if cfg.VoiceMode == "" {
        return
    }
    
    switch cfg.VoiceMode {
    case "ptt":
        cfg.Triggers.PTTEnabled = true
        cfg.Triggers.WakeWordEnabled = false
    case "wakeword":
        cfg.Triggers.PTTEnabled = true
        cfg.Triggers.WakeWordEnabled = true
    case "continuous":
        // 注意：行为有变化
        cfg.Triggers.PTTEnabled = true
        cfg.Triggers.WakeWordEnabled = true
    }
    
    cfg.VoiceMode = ""  // 清空旧字段
    config.Save()
}
```

---

## 6. 实现 Phase

### Phase 1：核心生命周期设计

**目标**：设计并实现 Trigger + Turn + Conversation 的核心生命周期

**关键任务**：
1. 定义 Conversation 状态机（Idle / Listening / Processing / Responding / Waiting）
2. 实现 ConversationManager，管理 Conversation 生命周期
3. 实现 30 秒超时逻辑（Waiting → Idle）
4. 不改变现有的 PTT / WakeWord 实现，只是在其之上添加 Conversation 层

**关键文件**：
- `internal/input/conversation.go`（新建）
- `internal/input/voice_controller.go`（重构，整合 Conversation）

**验证**：
- 单元测试：状态机转换正确
- 集成测试：Conversation 超时正确

### Phase 2：PTT / WakeWord 接入统一生命周期

**目标**：让 PTT 和 WakeWord 触发统一的 Conversation/Turn 生命周期

**关键任务**：
1. PTT 触发单轮 Turn（不进入 Conversation）
2. WakeWord 触发 Conversation（多轮对话）
3. PTT 期间暂停 WakeWord detector
4. Conversation 期间暂停 WakeWord detector
5. 处理并发场景（PTT 打断 Conversation）

**关键文件**：
- `internal/input/voice_controller.go`（重构）
- `internal/input/ptt.go`（调整，接入 Conversation）
- `internal/wakeword/`（确保 Pause/Resume 正确）

**验证**：
- PTT 单轮测试
- WakeWord 多轮测试
- PTT + WakeWord 并存测试
- 并发场景测试

### Phase 3：移除 Continuous 独立模式

**目标**：移除 Continuous 作为独立模式的概念

**关键任务**：
1. 移除 `ModeContinuous` 枚举
2. 移除 Continuous 相关的特殊逻辑
3. 更新配置迁移代码
4. 更新文档

**关键文件**：
- `internal/input/voice_controller.go`
- `internal/config/config.go`
- `docs/ai/ARCHITECTURE.md`

**验证**：
- 旧 `continuous` 配置能正确迁移
- 新配置下行为符合预期

### Phase 4：自然语言退出

**目标**：实现 `exit_conversation_mode` 工具，替代硬编码关键词

**关键任务**：
1. 实现 `ExitConversationTool`
2. 在 Agent 中注册工具
3. 更新系统提示词
4. 实现退出时序（当前 Turn 完成后退出）
5. 移除旧的硬编码关键词匹配

**关键文件**：
- `internal/tool/tools/conversation_control.go`（新建）
- `internal/agent/agent.go`（注册工具 + 更新提示词）
- `internal/input/voice_controller.go`（移除旧逻辑）

**验证**：
- 自然语言退出测试（各种表达）
- 不误触发测试（"谢谢"等不应触发）
- 退出时序测试（当前 Turn 正常完成）

### Phase 5：配置与 UI

**目标**：更新配置结构和 UI

**关键任务**：
1. 更新配置结构（`voice_mode` → `triggers`）
2. 实现配置迁移逻辑
3. 更新 Settings UI（移除模式选择，改为 Trigger 复选框）
4. 更新状态栏显示（显示 Conversation 状态）

**关键文件**：
- `internal/config/config.go`
- `frontend/src/components/Settings.vue`
- `frontend/src/api/backend.ts`

**验证**：
- 配置迁移测试
- UI 交互测试

### Phase 6：测试、文档与 Windows E2E

**目标**：完整测试和文档更新

**关键任务**：
1. 编写集成测试（覆盖所有场景）
2. Windows E2E 测试
3. 更新 `docs/ai/ARCHITECTURE.md`
4. 更新 `docs/ai/CURRENT_STATE.md`
5. 更新 `docs/ai/CHANGELOG.md`

**测试场景**：
- PTT 单轮
- WakeWord 多轮
- PTT + WakeWord 并存
- 自然语言退出
- 超时退出
- 并发场景（PTT 打断 Conversation）

---

## 7. 风险和注意事项

### 7.1 并发问题

**场景 1：PTT 和 WakeWord 同时触发**
- PTT 优先级更高
- WakeWord 被暂停

**场景 2：Conversation 超时与新输入同时发生**
- 如果在超时前收到新输入，重置超时计时器
- 如果超时已经触发，忽略新输入

**场景 3：Responding 时 PTT 触发**
- 停止当前 TTS 播放
- 暂停 WakeWord detector
- 开始 PTT Turn

**场景 4：Conversation 退出与当前 Turn 同时发生**
- 当前 Turn 正常完成
- Turn 完成后，Conversation 才结束

### 7.2 资源管理

**WakeWord detector**：
- Conversation 期间暂停，减少资源消耗
- PTT 期间暂停，避免冲突

**ASR session**：
- PTT：按下时创建，松开时销毁
- Conversation：进入 Conversation 时创建，退出时销毁

### 7.3 向后兼容

**配置迁移**：
- 旧配置能正确迁移到新配置
- 行为变化需要明确记录

**API 兼容**：
- 旧的 Wails bindings 保持兼容
- 新增 Conversation 相关的 bindings

---

## 8. 本次重构不做什么

1. **不重新实现 ASR 流式链路**  
   当前的流式 ASR 已经工作良好，不需要重新实现。

2. **不重新实现 VAD**  
   服务端 VAD 已经能够正确判断句子结束，不需要本地 VAD。

3. **不重新实现 LLM/TTS 流式链路**  
   当前的流式 LLM 和 TTS 已经工作良好。

4. **不改变 Agent Core 架构**  
   Agent 的对话管理、工具调用等保持不变。

5. **不实现"免唤醒直接监听"模式**  
   旧版的 `continuous` 模式是"免唤醒直接监听"，新版不支持这种模式。如果需要，可以作为未来的增强功能。

6. **不实现复杂的对话管理**  
   本次重构只关注 Trigger + Turn + Conversation 的生命周期，不涉及对话历史、上下文管理等。

---

## 9. 总结

### 核心改进

1. **从互斥模式 → 可组合触发器**  
   PTT 和 WakeWord 可以同时启用，不需要手动切换。

2. **从单轮交互 → 多轮对话**  
   WakeWord 触发的是 Conversation（多轮），不是单轮 Turn。

3. **从硬编码退出 → 自然语言退出**  
   用户可以用自然语言表达退出意图，不需要记住特定关键词。

4. **清晰的生命周期分离**  
   Turn 的结束（ASR/VAD 决定）和 Conversation 的结束（超时/主动退出）是两个不同的概念。

### 预期效果

- ✅ 减少模式切换的心智负担
- ✅ 更自然的对话体验（多轮对话）
- ✅ 更灵活的触发方式（PTT + WakeWord 并存）
- ✅ 更清晰的架构（Trigger / Turn / Conversation 分离）

---

## 10. 待讨论问题

1. **是否需要"免唤醒直接监听"模式？**  
   旧版 `continuous` 支持这种模式，新版不支持。是否需要作为增强功能实现？

2. **PTT 是否应该打断 Conversation？**  
   当前设计是 PTT 可以打断 Conversation，完成后回到 Idle。是否需要改为 PTT 完成后继续 Conversation？

3. **WakeWord 检测反馈**  
   检测到唤醒词时，是否需要声音/视觉反馈？

4. **默认配置**  
   新用户的默认配置是什么？PTT + WakeWord 都启用？

---

**下一步**：等待 review 通过后，开始 Phase 1 实现。
