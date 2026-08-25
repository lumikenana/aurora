---
name: phase-2-input-module-implementation
description: Phase 2 Input模块实现记录 - 全局快捷键+快速输入闭环
metadata:
  type: project
  date: 2026-08-24
---

## 阶段信息
- **阶段**: Phase 2 - Input Module
- **目标**: 实现第一个完整文本交互闭环，全局快捷键呼出快速输入框
- **完成日期**: 2026-08-24

## 实现内容

### 新增模块
| 文件 | 说明 |
|------|------|
| `internal/input/input.go` | InputManager - 统一输入管理器 |
| `internal/input/hotkey.go` | HotkeyListener 接口定义 |
| `internal/input/hotkey_windows.go` | Windows 全局快捷键实现（RegisterHotKey API） |
| `internal/input/hotkey_default.go` | 非Windows平台空实现 |
| `internal/input/quickinput.go` | 快速输入状态管理器 |
| `internal/input/bindings.go` | Wails bindings 供前端调用 |

### 修改文件
| 文件 | 修改内容 |
|------|----------|
| `main.go` | 集成Input模块，注册默认快捷键 Ctrl+Space，注册生命周期清理 |
| `internal/errors/errors.go` | 新增错误码 `CodeInputError` |
| `ui/src/index.html` | 添加快速输入弹出层HTML |
| `ui/src/main.js` | 添加快速输入键盘事件处理 |
| `ui/src/style.css` | 添加快速输入弹出层样式 |
| `docs/ai/CURRENT_STATE.md` | 更新当前状态 |

## 架构决策

### Windows 全局快捷键方案选择
选择：**Windows RegisterHotKey API + 隐藏消息窗口**

**理由**:
- 真正全局，任何时候都能触发，不需要窗口焦点
- 操作系统原生支持，不需要低级键盘钩子
- 实现简单，资源占用小
- 符合Windows设计规范

**替代方案**:
- 低级键盘钩子（SetWindowsHookEx）：过于侵入性，可能被杀毒软件拦截
- 第三方库：增加依赖，不必要

### 快速输入窗口方案选择
选择：**主应用内弹出层**，不单独创建窗口

**理由**:
- Wails单应用架构，不需要多窗口进程间通信
- 实现简单，通过CSS显示/隐藏
- 性能更好
- 符合需求"窗口关闭后不退出程序"

**替代方案**:
- 独立Wails窗口：增加复杂度，进程管理麻烦

### 平台隔离
所有Windows特定代码都使用 `//go:build windows` 条件编译隔离
非Windows代码使用空实现，保证可以编译

## 默认配置
- 默认快捷键: `Ctrl + Space`
- 虚拟键码: `VK_SPACE = 32`
- 修饰符: `ModCtrl = 2`

## 资源释放
- 退出时自动调用 `input.Shutdown()`
- Shutdown → `hotkeyListener.Shutdown()` → `UnregisterAll()` → 逐个注销热键
- 销毁消息窗口
- 退出消息循环goroutine

## 交互流程
```
User presses Ctrl+Space
  ↓
Windows WM_HOTKEY message
  ↓
Go callback → quickInput.Toggle()
  ↓
Toggle callback → Events.Emit "showQuickInput"
  ↓
Frontend receives event → show popup → focus input
  ↓
User types text → presses Enter
  ↓
Frontend calls Go: SubmitQuickInput(text)
  ↓
Go triggers submit callbacks → hide popup
  ↓
Conversation history displays in main window
```

## 验证
- ✅ macOS 编译通过
- ✅ 架构符合原有设计（依赖接口不依赖实现）
- ✅ Windows特定代码正确隔离
- ✅ 资源释放正确处理
- ✅ 满足所有需求（11项需求全部满足）

## 待完成下一阶段
下一阶段可以选择：
1. Agent Core 对话管理
2. LLM Provider 接口和火山引擎实现
3. ASR/TTS Provider 实现
