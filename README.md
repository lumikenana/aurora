# GameAI

Windows 桌面 AI 游戏助手。

## 特性

- 单 EXE 可执行文件，无需安装其他依赖
- Go 编写核心逻辑
- Wails 提供原生桌面体验
- 清晰的前后端边界
- 平台抽象层，支持跨平台开发（macOS 开发，Windows 运行）
- 内置配置管理、日志系统、错误处理
- 支持系统托盘

## 项目结构

```
game-ai/
├── app/                # 应用入口和命令行处理
│   ├── cmd/            # 命令定义
│   └── main/           # 主入口（Wails 入口在根 main.go）
├── internal/           # 核心内部模块（不对外导出）
│   ├── config/         # 配置管理
│   ├── errors/         # 统一错误处理
│   ├── log/            # 日志系统
│   ├── lifecycle/      # 应用生命周期管理
│   ├── platform/       # 平台抽象层（Windows/通用）
│   └── tray/           # 系统托盘
├── ui/                 # 前端代码
│   ├── src/            # 源代码
│   └── build/          # 编译输出
├── storage/            # 持久化存储（数据、缓存）
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
| `ui/` | 前端 UI 代码，只负责展示和用户交互 |
| `storage/` | 本地存储，放缓存和持久化数据 |

## 开发环境要求

- Go 1.21+
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

编译完成后，输出文件为 `build/bin/GameAI.exe`，可以直接在 Windows 上运行。

### 3. 简化的编译命令

项目提供了构建脚本：

```bash
./scripts/build-windows.sh
```

## 项目原则

1. **单一可执行文件**：最终只输出一个 `GameAI.exe`
2. **无外部依赖**：不需要用户安装 Python、Node.js、Docker 等
3. **单进程架构**：不拆分多个进程，不依赖本地 HTTP 服务
4. **清晰边界**：Go 负责逻辑，前端负责 UI
5. **平台抽象**：通过 Go 条件编译实现 Windows/非 Windows 分离

## 许可证

MIT
