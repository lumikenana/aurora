# Aurora

Windows 桌面 AI 助手。智能语音唤醒，随时响应你的提问。

## 特性

- 开箱即用的 Windows 桌面应用，安装简单
- Go 编写核心逻辑
- Wails 提供原生桌面体验
- 清晰的前后端边界
- 平台抽象层，支持跨平台开发（macOS 开发，Windows 运行）
- 内置配置管理、日志系统、错误处理
- 支持系统托盘
- **openWakeWord 本地唤醒词检测** - 后台持续监听，只在检测到唤醒词后启动云端 ASR
- **火山引擎流式 ASR** - 一句话识别，低延迟
- **Push-To-Talk** - 可选快捷键录音模式

## 项目结构

```
aurora/
├── app/                # 应用入口和命令行处理
│   ├── cmd/            # 命令定义
│   └── main/           # 主入口（Wails 入口在根 main.go）
├── frontend/           # 前端 Vue 3 代码
├── internal/           # 核心内部模块（不对外导出）
│   ├── agent/          # Agent 核心：对话管理、流式响应
│   ├── audio/          # 音频采集接口
│   ├── config/         # 配置管理
│   ├── errors/         # 统一错误处理
│   ├── input/          # 热键、快速输入、PTT、VoiceController 状态机
│   ├── log/            # 日志系统
│   ├── llm/            # LLM Provider 接口
│   ├── screenshot/     # 全屏截图
│   ├── speech/         # ASR 语音识别接口
│   ├── tool/           # Tool 系统
│   ├── wakeword/       # 唤醒词检测接口
│   │   └── providers/
│   │       └── openwakeword/  # openWakeWord ONNX 实现
│   └── tray/           # 系统托盘
├── docs/ai/            # 开发文档、架构记录
├── scripts/           # 辅助脚本
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
| `internal/` | 所有核心业务逻辑，Go 私有包，不对外暴露 |
| `internal/config` | 配置读写、默认配置、配置文件路径 |
| `internal/errors` | 统一错误定义、错误码、堆栈跟踪 |
| `internal/log` | 日志系统，同时输出到控制台和文件 |
| `internal/lifecycle` | 生命周期钩子、信号处理、优雅退出 |
| `internal/platform` | 平台抽象，Windows 特有功能用条件编译 |
| `internal/tray` | 系统托盘接口，Windows 使用 systray，其他平台空实现 |
| `frontend/` | 前端 UI 代码，只负责展示和用户交互 |

## 开发环境要求

- Go 1.26+
- Wails v2
- Node.js 可选（前端开发需要）

## macOS 开发

### 1. 安装 Wails

```bash
go install github.com/wailsapp/wails/v2/cmd/wails@latest
```

### 2. 安装依赖

```bash
go mod download
cd frontend && npm install
```

### 3. 开发运行

```bash
wails dev
```

### 4. 构建开发版本

```bash
wails build
```

## Windows 交叉编译（从 macOS）

### 1. 安装 Windows 交叉编译工具链

```bash
# macOS 使用 Homebrew 安装 MinGW-w64
brew install mingw-w64
```

### 2. 设置环境变量并编译

```bash
CGO_ENABLED=1 GOOS=windows GOARCH=amd64 CC=x86_64-w64-mingw32-gcc wails build
```

编译完成后，输出文件为 `build/bin/Aurora.exe`，可以直接在 Windows 上运行。

### 3. 简化的编译命令

项目提供了构建脚本：

```bash
./scripts/build-windows.sh
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

## 许可证

MIT
