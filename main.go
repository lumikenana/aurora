package main

import (
	"context"
	"embed"
	"io/fs"
	"os"

	"github.com/lipipi/game-ai/app/cmd"
	"github.com/lipipi/game-ai/internal/agent"
	"github.com/lipipi/game-ai/internal/audio"
	"github.com/lipipi/game-ai/internal/config"
	"github.com/lipipi/game-ai/internal/input"
	"github.com/lipipi/game-ai/internal/log"
	"github.com/lipipi/game-ai/internal/lifecycle"
	"github.com/lipipi/game-ai/internal/platform"
	"github.com/lipipi/game-ai/internal/speech"
	"github.com/lipipi/game-ai/internal/tray"

	// 注册所有内置 LLM Provider（触发各包的 init() 自动注册）
	_ "github.com/lipipi/game-ai/internal/llm/providers/openai"
	// 注册所有内置 Speech Provider
	_ "github.com/lipipi/game-ai/internal/speech/providers/volcengine"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/runtime"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

//go:embed all:frontend/dist
var assets embed.FS

// App Wails应用结构体
type App struct {
}

// NewApp 创建应用
func NewApp() *App {
	return &App{}
}

// startup 在应用启动时触发
func (a *App) startup(ctx context.Context) {
	log.Info("Wails application started")
}

// shutdown 在应用关闭时触发
func (a *App) shutdown(ctx context.Context) {
	log.Info("Wails application shutting down")
}

func main() {
	// 已经在app/main中做了初始化？不，Wails需要一个main入口
	// 重新做一遍初始化，保持结构清晰

	// 初始化日志（先基础初始化，配置加载后再完善）
	log.Init()
	log.Info("Starting GameAI... (build: 2026-08-26-voice-fix)")

	// 初始化配置
	if err := config.Init(); err != nil {
		log.Fatalf("Failed to initialize config: %v", err)
	}

	// 完善日志初始化，使用配置的日志级别和数据目录
	cfg := config.Global()
	log.SetLevelFromString(cfg.LogLevel)
	log.InitWithLogDir(config.GetAppDataDir())

	// 初始化平台
	platform.Init()

	// 初始化输入模块
	if err := input.Init(); err != nil {
		log.Errorf("Failed to initialize input: %v", err)
	}

	// 初始化快速输入管理器
	quickInput := input.InitQuickInput()

	// 获取Wails runtime上下文引用
	var runtimeCtx context.Context

	// 初始化系统托盘
	if err := tray.Init(); err != nil {
		log.Errorf("Failed to initialize tray: %v", err)
	}

	// 托盘：显示主窗口
	tray.Current().SetOnShowHandler(func() {
		if runtimeCtx != nil {
			runtime.WindowUnminimise(runtimeCtx)
			runtime.WindowShow(runtimeCtx)
		}
	})

	// 托盘：退出应用
	tray.Current().SetOnExitHandler(func() {
		if runtimeCtx != nil {
			runtime.Quit(runtimeCtx)
		}
	})

	// 在托盘添加快速提问菜单项
	tray.Current().SetOnToggleQuickInputHandler(func() {
		log.Debug("Tray menu: toggle quick input")
		quickInput.Toggle()
	})

	// 初始化 Agent Core
	if err := agent.Init(*config.Global()); err != nil {
		log.Errorf("Failed to initialize Agent: %v", err)
	}

	// 初始化音频采集
	audioProvider, audioErr := audio.CreateProvider(cfg.Audio)
	if audioErr != nil {
		log.Warnf("Audio capture not available: %v", audioErr)
		audioProvider = nil
	}

	// 初始化语音识别
	speechProvider, speechErr := speech.CreateProvider(cfg.Speech)
	if speechErr != nil {
		log.Warnf("Speech recognizer not available: %v", speechErr)
		speechProvider = nil
	}

	// 初始化 PTT 管理器
	if audioProvider != nil && speechProvider != nil {
		input.InitPTT(audioProvider, speechProvider)
		log.Info("Push-To-Talk initialized")
	} else {
		log.Warn("Push-To-Talk not available (audio or speech provider missing)")
	}

	// 注册默认快捷键 Ctrl+Alt+R
	// 虚拟键码: VK_R = 0x52 = 82
	hotkey := input.Hotkey{
		ID:       input.HotkeyToggleQuickInput,
		Modifier: input.ModCtrl | input.ModAlt,
		Key:      82, // VK_R
		Callback: func() {
			log.Debug("Toggle quick input hotkey triggered")
			quickInput.Toggle()
		},
	}

	if err := input.Current().GetHotkeyListener().RegisterHotkey(hotkey); err != nil {
		log.Errorf("Failed to register toggle quick input hotkey: %v", err)
	}

	// 注册 PTT 热键 Ctrl+Shift+R
	pttHotkey := input.Hotkey{
		ID:       input.HotkeyPTT,
		Modifier: input.ModCtrl | input.ModShift,
		Key:      82, // VK_R
		Callback: func() {
			if ptt := input.GetPTT(); ptt != nil {
				if ptt.IsRecording() {
					return // 已在录音，静默忽略
				}
				if err := ptt.StartRecording(); err != nil {
					log.Errorf("Failed to start PTT recording: %v", err)
					if runtimeCtx != nil {
						runtime.EventsEmit(runtimeCtx, "pttError",
							map[string]string{"message": err.Error()})
					}
				}
			}
		},
	}
	if err := input.Current().GetHotkeyListener().RegisterHotkey(pttHotkey); err != nil {
		log.Errorf("Failed to register PTT hotkey: %v", err)
	}

	// 快速输入显示/隐藏切换回调 - 通知前端显示/隐藏
	quickInput.RegisterToggleCallback(func(show bool) {
		if runtimeCtx == nil {
			log.Debug("Runtime not ready, cannot notify front")
			return
		}
		if show {
			log.Debug("Notify front to show quick input")
			// 把主窗口带到前台，否则快速输入浮层在后台窗口里用户看不到
			runtime.WindowUnminimise(runtimeCtx)
			runtime.WindowShow(runtimeCtx)
			// 通知前端显示快速输入
			runtime.EventsEmit(runtimeCtx, "showQuickInput", nil)
		}
	})

	// PTT 事件回调 - 通知前端录音状态和识别结果
	if ptt := input.GetPTT(); ptt != nil {
		ptt.RegisterStateChangeCallback(func(state input.PTTState) {
			if runtimeCtx != nil {
				runtime.EventsEmit(runtimeCtx, "pttStateChange", map[string]interface{}{
					"recording": state == input.PTTRecording,
				})
			}
		})
		ptt.RegisterTranscriptCallback(func(t speech.Transcript) {
			if runtimeCtx != nil {
				runtime.EventsEmit(runtimeCtx, "voiceTranscript", map[string]interface{}{
					"text":       t.Text,
					"is_final":   t.IsFinal,
					"session_id": t.SessionID,
				})
			}
		})
	}

	// 注册退出清理
	lifecycle.RegisterOnExit(func() {
		log.Info("Cleaning up...")
		// 确保 PTT 录音停止，释放麦克风
		if ptt := input.GetPTT(); ptt != nil {
			ptt.CancelRecording()
		}
		agent.Shutdown()
		input.Shutdown()
		tray.Shutdown()
		platform.Shutdown()
		config.Shutdown()
		log.Shutdown()
		log.Info("Shutdown complete")
	})

	// 如果是命令行模式，执行命令
	if len(os.Args) > 1 {
		if err := cmd.Execute(); err != nil {
			log.Fatalf("Command error: %v", err)
		}
		return
	}

	// 创建应用
	app := NewApp()

	// 创建bindings
	inputBindings := input.NewBindings()
	agentBindings := agent.NewBindings()

	bindings := []interface{}{
		inputBindings,
		agentBindings,
	}

	// 嵌入资源以 frontend/dist（Vite 构建产物）为根目录
	uiFS, err := fs.Sub(assets, "frontend/dist")
	if err != nil {
		log.Fatalf("Failed to create UI asset FS: %v", err)
	}

	// 创建Wails应用
	err = wails.Run(&options.App{
		Title:  "GameAI",
		Width:  cfg.WindowWidth,
		Height: cfg.WindowHeight,
		AssetServer: &assetserver.Options{
			Assets: uiFS,
		},
		BackgroundColour: &options.RGBA{R: 30, G: 30, B: 30, A: 1},
		OnStartup: func(ctx context.Context) {
			app.startup(ctx)
			runtimeCtx = ctx
			agent.SetRuntimeContext(ctx)
			log.Debug("Wails runtime context ready")

			// 设置日志回调，将日志广播到前端显示
			log.SetOnLogCallback(func(level log.LogLevel, message string) {
				var levelStr string
				switch level {
				case log.LevelDebug:
					levelStr = "debug"
				case log.LevelInfo:
					levelStr = "info"
				case log.LevelWarn:
					levelStr = "warn"
				case log.LevelError, log.LevelFatal:
					levelStr = "error"
				}
				runtime.EventsEmit(ctx, "log", map[string]string{
					"level":   levelStr,
					"message": message,
				})
			})
		},
		OnShutdown: app.shutdown,
		Bind: bindings,
	})

	if err != nil {
		log.Fatalf("Failed to run Wails app: %v", err)
	}
}
