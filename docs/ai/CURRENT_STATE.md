# GameAI - 当前状态

## Current Phase

**统一 VoiceController + 三种语音模式 + TTS 已完成。文本 + 语音 + TTS 完整闭环。**

## Completed

✓ 项目目录结构、Go module、Wails v2 配置
✓ 配置管理 / 日志系统 / 统一错误处理 / 生命周期管理 / 平台抽象层
✓ 系统托盘（Windows 实现 + ready 同步）
✓ **Input 模块** - 全局快捷键（专用消息线程模型，ADR-011）
✓ **默认快捷键 Ctrl+Alt+R** 呼出快速输入框（已 E2E 验证）
✓ **快速输入弹出层** - Enter 提交 / Esc 关闭 / 提交后自动关闭
✓ **LLM Provider** - OpenAI 兼容实现，已配置火山引擎豆包 doubao-seed-2.0-lite（ARK API，`/api/plan/v3`）
✓ **Agent Core** - 对话管理 + 流式响应（E2E 验证通过）
✓ **Screenshot 模块** - Windows GDI 全屏截图（1920x1080 约 100ms，E2E 验证通过）
✓ **前端 Vue 3 + Vite + TypeScript 迁移**（ADR-009），旧 ui/ 已删除
✓ Wails 绑定方法全部去除 ctx 参数（ADR-010）
✓ 单 EXE 构建（`wails build`，Windows 本地开发）
✓ **Tool 系统** - 统一 Tool 接口 + ToolRegistry + Agent Loop + screenshot/current_time 工具
✓ **设置 UI** - LLM 配置界面（provider / model / baseURL / apiKey）+ 热加载
✓ **LLM 配置保存 + 热加载** - SaveLLMConfig 支持 agentInstance 为 nil 时的完整初始化
✓ **SSE 流式解析修复** - 正确解析 `data:` 前缀，兼容豆包等思考模型（`thinking.type=disabled`）
✓ **Audio 模块** - `AudioCapture` 接口 + Windows waveIn `CALLBACK_WINDOW` 实现（消息窗口模式）
✓ **Speech 模块** - `SpeechRecognizer` 接口 + 火山引擎流式 ASR 2.0 Provider（二进制 WebSocket 协议）
✓ **PTT Push-To-Talk** - Ctrl+Shift+R 按住录音 + GetAsyncKeyState 释放检测 + 实时 partial transcript
✓ **语音输入端到端** - AudioCapture → SpeechRecognizer → PTTManager → InputManager → Agent（与文本共享同一管道）
✓ **VoiceIndicator 前端组件** - 录音状态指示 + 实时识别文本显示
✓ **语音配置 UI** - Settings 中添加 ASR provider / endpoint / resource ID / API Key 配置
✓ **Agent onUserInput 改为流式** - 语音和文本输入统一走 `ChatStream` + `llmChunk` 事件
✓ **火山引擎 ASR 协议调试全修复** - WAV 头、WIM_DATA 常量、JSON 解析、双提交问题等全部修复 ✓ E2E 验证通过
✓ **openWakeWord 唤醒词检测集成** - 本地 ONNX Runtime 推理 + 状态机 + 自动下载模型 + 前端配置 UI
✓ **唤醒词全链路修复** - ONNX Runtime "Error setting ORT API base: 2" 修复（sync.Once 一次性初始化）+ 前端下载进度条 ✓
✓ **TTS 语音合成 (Phase 1)** - 火山引擎 HTTP TTS + Windows mciSendString 播放 + 临时文件 base64 解码
✓ **统一 VoiceController (Phase 2)** - PTT / WakeWord / Continuous 三模式线程安全切换 + 状态机 + agentBusy/ttsPlaying 双重保护
✓ **连续对话模式** - 免唤醒循环 ASR → Agent → TTS，30s 无输入自动退出，关键词 "结束对话" 退出
✓ **TTS + VoiceController 串联 (Phase 3)** - llmFinish → SpeakTTS → ttsPlaying=true（回声抑制）→ NotifyTTSEnd → Continuous 模式自动续接
✓ **托盘子菜单 (Phase 4)** - 语音模式 (radio 单选：唤醒/连续/PTT) + 模式变更同步更新勾选
✓ **模式循环热键 (Phase 5)** - Ctrl+Alt+V 循环 PTT → WakeWord → Continuous → PTT
✓ **前端 UI (Phase 6)** - VoiceIndicator 显示模式徽章 + 状态徽章 + TTS 播放指示 + Agent 处理中 spinner
✓ **设置 UI 扩展** - TTS 配置（provider/voiceType/speedRatio/apiKey）+ 测试播放 + 模式单选按钮
✓ **配置持久化 (Phase 7)** - voice_mode / continuous_timeout / continuous_exit_commands / tts 配置全部持久化

## In Progress

None.

## Not Started

- [ ] TTS Provider 接口和实现
- [ ] 窗口截图（当前仅全屏）+ 截图与 LLM 联动（带图提问）
- [ ] GamePlugin 系统
- [ ] SQLite 存储集成（对话历史保存）
- [ ] 开机启动等 platform 模块实际接线

## Known Issues

- 多实例同时运行会热键注册失败（"already registered"），属预期行为，测试时注意 taskkill 旧实例
- 非 Windows 平台热键/托盘/截图/音频均为空实现（设计预期）
- 截图在多显示器场景未充分测试
- 前端调试日志（`[handleSubmit]` / `[llmChunk]` / `[llmFinish]`）暂未清理，用于排障
- 语音协议调试日志（`parseResp`）暂未清理

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
- **新增 LLM Provider 包必须在 main.go 添加 blank import** 触发 init 注册（见 ADR-012）
- **OpenAI 兼容 API 请求必须显式关闭豆包等思考模型的 thinking 模式**（见 ADR-013）
- **waveIn 音频采集必须用 `CALLBACK_WINDOW` 模式**（`CALLBACK_FUNCTION` 在 Go 里回调不可靠）（见 ADR-014）
- **火山引擎 ASR 第一个音频段必须 prepend 44 字节 WAV 头**（服务端靠此识别格式）（见 ADR-015）
- **火山引擎 ASR 默认 Resource ID 为 `volc.seedasr.sauc.duration`**（不是 `bigasr`）（见 ADR-015）
- **火山引擎 ASR 默认 Endpoint 为 `bigmodel_async`**（双向流式）（见 ADR-015）
- **语音输入的前端不应再调 `handleSubmit`**，否则双提交导致响应乱序（见 ADR-016）

## Current Architecture

```
main.go (Wails入口, embed frontend/dist, OnStartup 注入 runtime ctx)
  ├── blank import openai provider（触发 init 注册）
  └── app/cmd (命令行处理)
  └── internal/
       ├── config (✓ 配置管理)
       ├── errors (✓ 统一错误)
       ├── log (✓ 日志系统)
       ├── lifecycle (✓ 生命周期)
       ├── platform (✓ 平台抽象，条件编译)
       ├── tray (✓ 系统托盘，ready 同步)
       ├── input (✓ 热键 + 快速输入 + PTT，专用消息线程)
       ├── llm (✓ Provider 接口 + openai 兼容实现，thinking 模式可配置)
       ├── agent (✓ Agent Core + Wails bindings + 设置 UI 热加载)
       ├── screenshot (✓ GDI 全屏截图)
       └── tool (✓ Tool 接口 + Registry + Agent Loop)
       ├── audio (✓ AudioCapture 接口 + waveIn CALLBACK_WINDOW 实现)
       ├── speech (✓ SpeechRecognizer 接口 + 火山引擎 ASR Provider)
       └── input/ptt (✓ PTT 管理器 + GetAsyncKeyState 释放检测)
frontend/ (✓ Vue 3 + Vite + TypeScript)
  └── src/components/
       ├── App.vue (✓ 主窗口 + 流式对话 + 事件监听)
       ├── Conversation.vue (✓ 对话显示)
       ├── QuickInput.vue (✓ 快速输入弹出层)
       ├── Settings.vue (✓ LLM + 语音识别配置 UI)
       └── VoiceIndicator.vue (✓ PTT 录音状态 + 实时识别文本)
       └── LogPanel.vue (✓ 日志面板)
scripts/ (✓ screenshot-test 独立验证程序)
docs/ai/ (✓ AI 记忆系统)
```

## Next Step

候选方向（按用户指令选择）：

1. **截图 + LLM 联动**: 快捷键截图提问（截图附带给多模态 LLM）
2. **TTS 语音输出**: 让 Agent 回答可以语音播放
3. **SQLite 持久化**: 对话历史保存
4. **GamePlugin 系统**: 特定游戏知识插件

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

### 已验证

**2026-08-25**:
- `wails build` ✓ 单 EXE 产出
- 全局热键 Ctrl+Alt+R → 快速输入弹窗 ✓（模拟按键 E2E）
- 托盘菜单（快速提问/退出）✓
- **流式对话（豆包 doubao-seed-2.0-lite）✓** 前端逐字显示
- **LLM 设置 UI** ✓ 配置保存 + 热加载 + 首次启动友好
- 全屏截图 1920x1080 / 172KB / ~100ms ✓（按钮 E2E + 独立程序）
- 日志面板实时输出 ✓（log 事件 + console 重定向）

**2026-08-26**:
- **PTT 语音输入 E2E ✓** — 按住 Ctrl+Shift+R → 录音 → 实时 partial → 松开 → 最终文本 → Agent 流式回复
- 火山引擎 ASR WebSocket 连接 ✓（`bigmodel_async` endpoint + `volc.seedasr.sauc.duration` resource）
- waveIn CALLBACK_WINDOW 音频采集 ✓（16kHz/16bit/mono，6400 bytes/200ms）
- 音频电平监控 ✓（max amplitude 日志）
- 语音配置 UI ✓（provider/endpoint/resource ID/apiKey 持久化到 config.json）
- 火山引擎 ASR 协议全链路修复 ✓ — WAV 头 / WIM_DATA 常量 / JSON 解析 / 双重提交问题全部解决
- **语音 + 文本双通道工作正常**，均可触发 Agent 流式回复

### 排障入口

- 日志文件: `%APPDATA%/GameAI/gameai_YYYYMMDD.log`（config 中 log_level=debug 时含诊断日志）
- `FrontendEcho` 绑定方法可确认前端脚本是否加载
- Wails 吞掉部分 ERR 日志时，可重定向 stdout 捕获
- `SaveLLMConfig` / `GetConfigStatus` 有详细日志，可追踪配置保存和 provider 状态
- `readStream` 有 SSE 行级日志（`SSE raw line N: ...`），可追踪流式解析
- `parseServerResponse` 有完整协议解析日志（`[parseResp]`），可追踪 ASR 响应解析
- 音频回调有电平日志（`Audio chunk: X bytes, max amplitude: Y`），可判断麦克风是否正常

### 关键配置路径

- 配置文件: `%APPDATA%/GameAI/config.json`
- 火山引擎 ARK (LLM) Base URL: `https://ark.cn-beijing.volces.com/api/plan/v3`
- 火山引擎 ASR Endpoint: `wss://openspeech.bytedance.com/api/v3/sauc/bigmodel_async`
- 火山引擎 ASR Resource ID: `volc.seedasr.sauc.duration`
- 豆包思考模型必须在请求体中显式 `thinking.type = "disabled"`
