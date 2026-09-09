# Aurora

Windows 桌面 AI 助手。智能语音唤醒，随时响应你的提问。

## 特性

- 开箱即用的 Windows 桌面应用，安装简单
- Go 编写核心逻辑
- Wails v3 提供原生桌面体验
- **LLM 游戏助手** - 支持 TFT 等游戏的智能问答，基于游戏实时状态提供建议
- **游戏 Overlay** - 透明悬浮窗口，游戏时实时显示 AI 建议（HUD 模式 / Edit Mode 双模式，支持拖拽/缩放/自定义位置，自动隐藏任务栏图标）
- **知识库系统** - 自动导入游戏数据，支持版本化知识管理
- **Tool 系统** - LLM 可调用工具获取游戏状态、知识库信息
- **openWakeWord 本地唤醒词检测** - 后台持续监听，只在检测到唤醒词后启动云端 ASR
- **火山引擎流式 ASR** - 一句话识别，低延迟
- **火山引擎 TTS** - 语音合成，自然流畅
- **Push-To-Talk** - 可选快捷键录音模式
- **统一快捷键系统** - 9 个可配置快捷键，支持冲突检测
- **系统托盘** - 快速访问常用功能
- 平台抽象层，支持跨平台开发（macOS 开发，Windows 运行）
- 内置配置管理、日志系统、错误处理

## 项目结构

```
aurora/
├── frontend/           # 前端 Vue 3 + TypeScript 代码（三入口：main/overlay/quickinput）
├── internal/           # 核心内部模块（不对外导出）
│   ├── agent/          # Agent 核心：对话管理、流式响应、Tool 调用、Overlay UI bindings
│   ├── audio/          # 音频采集接口
│   ├── config/         # 配置管理
│   ├── game/           # 游戏插件框架（通用 GamePlugin/GameContext 接口）
│   │   ├── knowledge/  # 知识库系统（TFT 数据、CommunityDragon/DDragon 解析）
│   │   └── plugins/    # 具体游戏实现（如 TFT）
│   ├── input/          # 热键、快速输入、PTT、VoiceController 状态机、ShortcutManager
│   ├── lifecycle/      # 应用生命周期管理（优雅退出、hook 系统）
│   ├── llm/            # LLM Provider 接口（OpenAI 兼容）
│   ├── log/            # 日志系统
│   ├── platform/       # 平台抽象，Windows 特有功能
│   ├── screenshot/     # 全屏截图（GDI）
│   ├── speech/         # ASR 语音识别接口（火山引擎）
│   ├── storage/        # SQLite 存储层（知识库持久化）
│   ├── tool/           # Tool 系统（LLM 工具调用框架）
│   ├── tray/           # 系统托盘（Wails v3 native）
│   ├── tts/            # TTS 语音合成接口（火山引擎）
│   ├── wailsruntime/   # Wails v3 单一 bridge（窗口操作、事件广播）
│   └── wakeword/       # 唤醒词检测（openWakeWord）
├── docs/ai/            # 开发文档、架构记录、变更日志
├── app/                # 应用入口和命令行处理
├── cmd/                # 独立 CLI 工具（数据导入等）
├── scripts/            # 辅助脚本和工具
├── go.mod              # Go module
├── go.sum              # Go 依赖校验
├── wails.json          # Wails 配置
├── main.go             # Wails 应用主入口
└── README.md           # 本文件
```

### 目录职责说明

| 目录 | 职责 |
|------|------|
| `app/` | 应用启动入口、命令行参数处理 |
| `internal/agent/` | Agent 核心：对话管理、LLM 调用、Tool 执行 |
| `internal/game/` | 游戏插件框架：通用接口 + TFT 插件 + 知识库 |
| `internal/storage/` | SQLite 存储层，知识库持久化 |
| `internal/tool/` | Tool 系统框架，注册和调度 LLM 工具 |
| `internal/llm/` | LLM Provider 接口（OpenAI 兼容） |
| `internal/speech/` | ASR 语音识别（火山引擎流式） |
| `internal/tts/` | TTS 语音合成（火山引擎） |
| `internal/config` | 配置读写、默认配置、配置文件路径 |
| `internal/log` | 日志系统，同时输出到控制台和文件 |
| `internal/platform` | 平台抽象，Windows 特有功能 |
| `internal/tray` | 系统托盘接口 |
| `frontend/` | 前端 UI 代码，只负责展示和用户交互 |

## 开发环境要求

- Go 1.26+
- Wails v3
- Node.js 可选（前端开发需要）

## macOS 开发

### 1. 安装 Wails

```bash
go install github.com/wailsapp/wails/v3/cmd/wails3@latest
```

### 2. 安装依赖

```bash
go mod download
cd frontend && npm install
```

### 3. 开发运行

```bash
wails3 dev
```

### 4. 构建开发版本

```bash
wails3 build
```

## Windows 交叉编译（从 macOS）

### 1. 安装 Windows 交叉编译工具链

```bash
# macOS 使用 Homebrew 安装 MinGW-w64
brew install mingw-w64
```

### 2. 设置环境变量并编译

```bash
CGO_ENABLED=1 GOOS=windows GOARCH=amd64 CC=x86_64-w64-mingw32-gcc wails3 build
```

编译完成后，输出文件为 `build/bin/Aurora.exe`，可以直接在 Windows 上运行。

### 3. 简化的编译命令

项目提供了构建脚本：

```bash
./scripts/build-windows.sh
```

## 生成 Wails 绑定

当修改了 Go 端的 binding 函数签名后，需要重新生成 TypeScript 绑定：

```bash
wails3 generate bindings -ts
```

## 使用唤醒词功能

### 启用方式

1. 打开设置 → 唤醒词配置
2. 勾选 **启用唤醒词**
3. 默认使用 `hey_mycroft_v0.1.onnx`（唤醒词 "Hey Mycroft"）
4. 保存配置 → 重启应用

### 更换唤醒词

openWakeWord 官方提供以下预训练模型：

| 唤醒词 | 模型文件名 |
|--------|-----------|
| Hey Mycroft | `hey_mycroft_v0.1.onnx` |
| Alexa | `alexa_v0.1.onnx` |
| Hey Jarvis | `hey_jarvis_v0.1.onnx` |
| Hey Rhasspy | `hey_rhasspy_v0.1.onnx` |

下载地址: https://github.com/dscripka/openWakeWord/releases

下载后在设置中填入模型完整路径即可。

### 自定义训练

参考官方训练指南: https://github.com/dscripka/openWakeWord#training

## 项目原则

1. **简单安装体验**: 提供开箱即用的 Windows 安装体验，用户无需手动配置复杂运行时依赖
2. **无额外运行时**: 不需要用户安装 Python、Node.js、Docker 等
3. **单进程架构**: 不拆分多个进程，不依赖本地 HTTP 服务
4. **清晰边界**: Go 负责逻辑，前端负责 UI
5. **平台抽象**: 通过 Go 条件编译实现 Windows/非 Windows 分离

## Overlay 游戏助手

Aurora 提供透明悬浮的 Overlay 窗口，可在游戏时实时显示 AI 建议：

### 双模式切换

- **HUD 模式**（默认）：纯显示，鼠标穿透，不影响游戏操作
- **Edit Mode**：可拖拽/缩放，调整 Overlay 位置和大小

### 特性

- **透明背景**：DWM 层透明处理，边缘无白边
- **自定义位置**：拖拽后自动记忆，重启恢复
- **不占用任务栏**：使用 `WS_EX_TOOLWINDOW` 样式
- **自动锁定**：游戏启动时自动切换到 HUD 模式
- **即时保存**：所有配置变化立即生效，无需手动保存

### 快捷键

- `Ctrl+Alt+R`：切换 Overlay 显示/隐藏
- `Ctrl+Alt+L`：切换 HUD/Edit Mode
- 更多快捷键可在设置中配置

## 开发

## 许可证

MIT
