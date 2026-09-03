package main

import (
	"context"
	"embed"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/lipipi/aurora/app/cmd"
	"github.com/lipipi/aurora/internal/agent"
	"github.com/lipipi/aurora/internal/audio"
	"github.com/lipipi/aurora/internal/config"
	"github.com/lipipi/aurora/internal/game/knowledge"
	"github.com/lipipi/aurora/internal/game/knowledge/cdsource"
	"github.com/lipipi/aurora/internal/game/knowledge/store"
	"github.com/lipipi/aurora/internal/input"
	"github.com/lipipi/aurora/internal/log"
	"github.com/lipipi/aurora/internal/lifecycle"
	"github.com/lipipi/aurora/internal/platform"
	"github.com/lipipi/aurora/internal/speech"
	"github.com/lipipi/aurora/internal/tray"
	"github.com/lipipi/aurora/internal/tts"
	"github.com/lipipi/aurora/internal/wakeword"
	"github.com/lipipi/aurora/internal/wailsruntime"

	// 注册所有内置 LLM Provider（触发各包的 init() 自动注册）
	_ "github.com/lipipi/aurora/internal/llm/providers/openai"
	// 注册所有内置 Speech Provider
	_ "github.com/lipipi/aurora/internal/speech/providers/volcengine"
	// 注册所有内置 WakeWord Provider
	_ "github.com/lipipi/aurora/internal/wakeword/providers/openwakeword"
	// 注册所有内置 TTS Provider
	_ "github.com/lipipi/aurora/internal/tts/providers/volcengine"
	// 注册所有内置游戏插件（GamePlugin，见 ADR-021）
	_ "github.com/lipipi/aurora/internal/game/plugins/tft"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

//go:embed all:frontend/dist
var assets embed.FS

// initKnowledgeService 初始化 TFT 知识库（Phase 11-1）
//
// 失败全部为非致命：知识库不可用时，TFT 知识工具会自动 IsAvailable() = false，
// get_tft_game_state 仍可工作（仅无法关联知识）。
func initKnowledgeService() {
	cfg := config.Global()
	if !cfg.Game.Knowledge.Enabled {
		log.Info("Knowledge base disabled by config")
		return
	}

	dbPath := filepath.Join(config.GetAppDataDir(), "knowledge.db")
	st, err := store.Open(dbPath)
	if err != nil {
		log.Warnf("Failed to open knowledge db at %s: %v (TFT knowledge tools will be unavailable)", dbPath, err)
		return
	}

	rawDir := filepath.Join(config.GetAppDataDir(), "knowledge", "raw")
	src := cdsource.New()
	svc := knowledge.NewService(st, src, cfg.Game.Knowledge, rawDir)

	// 注入到 Agent（agent 会自动分发给已注册的 TFT 知识工具）
	if a := agent.Current(); a != nil {
		a.SetKnowledgeProvider(svc)
		if svc.HasData() {
			if p, err := svc.GetCurrentPatch(context.Background()); err == nil {
				log.Infof("TFT knowledge base ready: active patch=%s (db=%s)", p, dbPath)
			} else {
				log.Infof("TFT knowledge base ready (db=%s)", dbPath)
			}
		} else {
			log.Info("TFT knowledge base empty; tools will return not_found until data is imported")
		}
	}

	// 关闭 hook
	lifecycle.RegisterOnExit(func() {
		_ = st.Close()
	})
}

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
	log.Info("Starting Aurora...")

	// 初始化配置
	if err := config.Init(); err != nil {
		log.Fatalf("Failed to initialize config: %v", err)
	}

	// 完善日志初始化，使用配置的日志级别和数据目录
	cfg := config.Global()
	log.SetLevelFromString(cfg.LogLevel)
	log.InitWithLogDir(config.GetAppDataDir())

	// Phase 5: 预初始化 Overlay 运行时可见性（agent 路径需要）。
	// 在 agent.Init 之前调用，让托盘初始菜单文本能直接读到正确状态，
	// 不必为了初始文本而把 tray setup 整体后移到 agent.Init 之后。
	agent.InitOverlayVisibleState()

	// 初始化平台
	platform.Init()

	// 初始化输入模块
	if err := input.Init(); err != nil {
		log.Errorf("Failed to initialize input: %v", err)
	}

	// 初始化快速输入管理器
	quickInput := input.InitQuickInput()

	// 初始化系统托盘（Phase 8D: v3 native SystemTray 由 v3App 拥有；本调用为 no-op 兼容）
	if err := tray.Init(); err != nil {
		log.Errorf("Failed to initialize tray: %v", err)
	}

	// 托盘：显示主窗口
	tray.Current().SetOnShowHandler(func() {
		wailsruntime.ShowMainWindow()
	})

	// 托盘：退出应用
	tray.Current().SetOnExitHandler(func() {
		wailsruntime.Quit()
	})

	// Phase 5: 托盘 Overlay 菜单 — 区分 Enabled（持久化配置）和 Visible（运行时态）。
	// 1. 菜单点击：读取 agent 进程内可见性状态 → 取反 → 写回 + 广播 overlayVisibilityChanged
	//    事件，前端与 main.go 自身都会收到。
	// 2. main.go 同时订阅 overlayVisibilityChanged：收到后刷新托盘菜单文本。
	//    覆盖了"前端关闭 Overlay 时托盘菜单文本也要同步"的场景。
	tray.Current().SetOnToggleOverlayHandler(func() {
		// 读取进程内真实可见性（不是 config.Overlay.Enabled）
		current := agent.GetOverlayVisibleState()
		next := !current
		log.Debugf("Tray overlay toggle: %v -> %v", current, next)
		agent.SetOverlayVisibleAndBroadcast(next)
		// 注意：托盘菜单文本由下面的 SubscribeEvent("overlayVisibilityChanged") 统一刷新，
		// 这里不直接调用 SetOverlayMenuState，避免双写。
	})

	// 初始化阶段就把托盘菜单文本对齐到 agent 进程内可见性。
	// 必须在 SetOnToggleOverlayHandler 之后调用（否则 SetOverlayMenuState 找不到 item 引用）。
	tray.Current().SetOverlayMenuState(agent.GetOverlayVisibleState())

	// 初始化语音模式引用（被托盘子菜单、热键、post-response hook 共享）
	// 实际 voiceController 在 wakeword 自动下载完成后才创建；此引用为延迟赋值
	var voiceControllerRef *input.VoiceController

	// 在托盘添加快速提问菜单项
	tray.Current().SetOnToggleQuickInputHandler(func() {
		log.Debug("Tray menu: toggle quick input")
		quickInput.Toggle()
	})

	// 注册托盘子菜单：语音模式（3 个 radio 单选项）
	// 注：voiceControllerRef 是延迟创建的变量；callback 内通过闭包读取当前值
	// Phase 8D: v3 native — 用 RegisterRadioItem 注册；BuildMenuAndAttach 时构造 submenu
	for _, m := range []input.VoiceMode{input.ModePTT, input.ModeWakeWord, input.ModeContinuous} {
		mode := m // 捕获循环变量
		tray.RegisterRadioItem("语音模式", mode.DisplayName(), func() {
			if voiceControllerRef == nil {
				log.Debug("Tray voice mode: voice controller not ready")
				return
			}
			if err := voiceControllerRef.SetMode(context.Background(), mode); err != nil {
				log.Errorf("Failed to set voice mode from tray: %v", err)
			}
		})
	}

	// 初始化 Agent Core
	if err := agent.Init(*config.Global()); err != nil {
		log.Errorf("Failed to initialize Agent: %v", err)
	}

	// Phase 11-1：初始化知识库（可选；失败不阻塞主流程）
	initKnowledgeService()

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

	// 初始化 TTS 语音合成（TTS 不可用不影响主流程）
	ttsProvider, ttsErr := tts.CreateProvider(cfg.TTS)
	if ttsErr != nil {
		log.Warnf("TTS provider not available: %v", ttsErr)
		ttsProvider = nil
	} else {
		log.Infof("TTS provider initialized: %s (configured=%v)", ttsProvider.Name(), ttsProvider.IsConfigured())
	}

	// 初始化 PTT 管理器
	if audioProvider != nil && speechProvider != nil {
		input.InitPTT(audioProvider, speechProvider)
		log.Info("Push-To-Talk initialized")
	} else {
		log.Warn("Push-To-Talk not available (audio or speech provider missing)")
	}

	// 初始化 VoiceController 唤醒词检测
	// voiceControllerRef 已在前面声明，托盘子菜单/热键/post-response hook 共享此引用。
	if audioProvider != nil && speechProvider != nil && cfg.WakeWord.Enabled {
		wakeDetector, wakeErr := wakeword.CreateProvider(cfg.WakeWord)
		if wakeErr != nil {
			log.Warnf("Wake word detector not available: %v", wakeErr)
		} else if wakeDetector.IsConfigured() {
			// startVoiceController 构建并启动 VoiceController，同时注册 PTT 协作回调。
			// 无论文件是已就绪还是下载完成后，都通过它启动。
			startVoiceController := func() {
				voiceControllerRef = input.NewVoiceController(
					wakeDetector,
					speechProvider,
					audioProvider,
					input.WithEnabled(cfg.WakeWord.Enabled),
					input.WithCooldown(time.Duration(cfg.WakeWord.CooldownMs)*time.Millisecond),
					input.WithContinuousSettings(
						time.Duration(cfg.ContinuousTimeoutSeconds)*time.Second,
						cfg.ContinuousExitCommands,
					),
					input.WithOnStateChange(func(state input.VoiceState) {
wailsruntime.EmitEvent("voiceStateChange", map[string]interface{}{
	"state": state.String(),
	"code":  int(state),
})
					}),
					input.WithOnModeChange(func(mode input.VoiceMode) {
wailsruntime.EmitEvent("voiceModeChange", map[string]interface{}{
	"mode":      mode.String(),
	"code":      int(mode),
	"mode_name": mode.DisplayName(),
})
						// 同步更新托盘子菜单 radio 状态
						for _, m := range []input.VoiceMode{input.ModePTT, input.ModeWakeWord, input.ModeContinuous} {
							tray.SetMenuItemChecked("语音模式", m.DisplayName(), m == mode)
						}
						// 持久化模式选择
						cfg.VoiceMode = mode.String()
						if err := config.Save(); err != nil {
							log.Warnf("Failed to persist voice mode: %v", err)
						}
					}),
					input.WithOnTranscript(func(t speech.Transcript) {
wailsruntime.EmitEvent("voiceTranscript", map[string]interface{}{
	"text":       t.Text,
	"is_final":   t.IsFinal,
	"session_id": t.SessionID,
})
					}),
					input.WithOnWakeDetected(func() {
wailsruntime.EmitEvent("wakeWordDetected", nil)
// Bring window to front when wake detected
wailsruntime.ShowMainWindow()
						
						log.Info("Wake word detected, bringing window to front")
					}),
					input.WithOnFinalResult(func(text string) {
						log.Infof("Voice ASR final result: %q", text)
						if text != "" {
							// 通知 VoiceController：Agent 开始处理（抑制新一轮输入）
							voiceControllerRef.NotifyAgentStart()
							// Send to agent via input manager
							input.Current().NotifyInput(input.UserInput{
								Text:   text,
								Source: input.SourceVoice,
							})
						}
					}),
				)

				// 注入 TTS Provider（用于 SpeakTTS 播放）
				if ttsProvider != nil {
					voiceControllerRef.SetTTS(ttsProvider)
				}

				if err := voiceControllerRef.Start(context.Background()); err != nil {
					log.Errorf("Failed to start voice controller: %v", err)
					voiceControllerRef = nil
					return
				}

				// 启动后设置初始模式（从配置读取，默认 WakeWord）
				initialMode := input.ParseVoiceMode(cfg.VoiceMode)
				if err := voiceControllerRef.SetMode(context.Background(), initialMode); err != nil {
					log.Warnf("Failed to set initial voice mode %v: %v", initialMode, err)
				}
				// 同步托盘子菜单 radio 状态
				for _, m := range []input.VoiceMode{input.ModePTT, input.ModeWakeWord, input.ModeContinuous} {
					tray.SetMenuItemChecked("语音模式", m.DisplayName(), m == initialMode)
				}

				// 把引用注入到 agent bindings（前端 GetVoiceMode/SetVoiceMode 使用）
				agent.SetVoiceController(voiceControllerRef)

				log.Infof("VoiceController started successfully (mode=%s)", initialMode.DisplayName())

				// When PTT is activated, it preempts wake word
				if ptt := input.GetPTT(); ptt != nil {
					ptt.RegisterPreStartCallback(func() {
						// PTT starts - preempt any ongoing wake word session
						voiceControllerRef.PTTPreempt()
					})
					ptt.RegisterFinishCallback(func() {
						// PTT finished - resume wake word listening
						voiceControllerRef.PTTFinished()
					})
				}
			}

			// Check if we need to auto-download
			if !wakeword.IsAlreadyDownloaded(cfg.WakeWord, config.GetAppDataDir()) {
				log.Info("Wake word: starting auto-download of required files...")
				// Start download in background
				// We don't start voice controller until download completes
				go func() {
					progress := func(percent int, msg string) {
						log.Infof("Wake word download: %d%% - %s", percent, msg)
wailsruntime.EmitEvent("wakewordDownloadProgress", map[string]interface{}{
		"percent": percent,
		"message": msg,
	})
					}
					if err := wakeword.AutoDownloadIfNeeded(context.Background(), cfg.WakeWord, config.GetAppDataDir(), progress); err != nil {
						log.Errorf("Wake word auto-download failed: %v", err)
wailsruntime.EmitEvent("wakewordDownloadError", map[string]string{"error": err.Error()})
						return
					}
					log.Info("Wake word: all files downloaded successfully")
wailsruntime.EmitEvent("wakewordDownloadComplete", nil)
					// Download finished - start voice controller now
					startVoiceController()
				}()
			} else {
				// All files already downloaded - start voice controller immediately
				startVoiceController()
			}
		}
	}

	// Phase 8E: Ctrl+Alt+R 改用 Wails v3 GlobalShortcut（见下方 v3App 创建后段）。
	// 旧 Win32 RegisterHotKey 路径已移除，避免与 v3 GlobalShortcut 双注册冲突。
	// 仍走 Win32 的热键：PTT (Ctrl+Shift+R)、CycleMode (Ctrl+Alt+V)。

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
wailsruntime.EmitEvent("pttError", map[string]string{"message": err.Error()})
				}
			}
		},
	}
	if err := input.Current().GetHotkeyListener().RegisterHotkey(pttHotkey); err != nil {
		log.Errorf("Failed to register PTT hotkey: %v", err)
	}

	// 注册模式循环热键 Ctrl+Alt+V
	// 虚拟键码: VK_V = 0x56 = 86
	cycleModeHotkey := input.Hotkey{
		ID:       input.HotkeyCycleVoiceMode,
		Modifier: input.ModCtrl | input.ModAlt,
		Key:      86, // VK_V
		Callback: func() {
			if voiceControllerRef == nil {
				log.Debug("Cycle mode hotkey: voice controller not ready")
				return
			}
			newMode := voiceControllerRef.CycleMode(context.Background())
			log.Infof("Cycle mode hotkey: switched to %s", newMode.DisplayName())
		},
	}
	if err := input.Current().GetHotkeyListener().RegisterHotkey(cycleModeHotkey); err != nil {
		log.Errorf("Failed to register cycle mode hotkey: %v", err)
	}

	// 注册 Agent Post-Response Hook：流式响应结束后，触发 TTS 播放
	// 通过 voiceControllerRef（闭包）读取，voiceController 可能延迟创建
	agent.Current().SetPostResponseHook(func(text string) {
		if voiceControllerRef == nil {
			return
		}
		// 通知 Agent 完成（用于 Continuous 模式续接）
		voiceControllerRef.NotifyAgentComplete()
		// 触发 TTS 播放（内部会处理 ttsPlaying 回声抑制）
		voiceControllerRef.SpeakTTS(text)
	})

	// Phase 8E: QuickInput 已迁独立 native window，showQuickInput 前端事件链路
	// 整体移除（App.vue 不再监听；窗口可见性由 QuickInputManager → driver 直接
	// 控制 native window，单一真相在后端）。旧 RegisterToggleCallback 注释块
	// （Phase 2 的 Main webview 内嵌方案）已删除。

	// PTT 事件回调 - 通知前端录音状态和识别结果
	if ptt := input.GetPTT(); ptt != nil {
		ptt.RegisterStateChangeCallback(func(state input.PTTState) {
wailsruntime.EmitEvent("pttStateChange", map[string]interface{}{
	"recording": state == input.PTTRecording,
})
		})
		ptt.RegisterTranscriptCallback(func(t speech.Transcript) {
wailsruntime.EmitEvent("voiceTranscript", map[string]interface{}{
	"text":       t.Text,
	"is_final":   t.IsFinal,
	"session_id": t.SessionID,
})
		})
	}

	// 注册退出清理
	lifecycle.RegisterOnExit(func() {
		log.Info("Cleaning up...")
		// 确保 PTT 录音停止，释放麦克风
		if ptt := input.GetPTT(); ptt != nil {
			ptt.CancelRecording()
		}
		// Stop voice controller if running
		if voiceControllerRef != nil {
			voiceControllerRef.Stop()
		}
		// Stop TTS playback
		if ttsProvider != nil {
			ttsProvider.Stop()
		}
		agent.Shutdown()
		input.Shutdown()
		// Phase 8D: tray.Shutdown() 移除 — v3 SystemTray 由 v3App 销毁，
		// tray.Shutdown() 已是 no-op。
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

	// 嵌入资源以 frontend/dist（Vite 构建产物）为根目录
	uiFS, err := fs.Sub(assets, "frontend/dist")
	if err != nil {
		log.Fatalf("Failed to create UI asset FS: %v", err)
	}

	// 创建Wails v3应用
	v3App := application.New(application.Options{
		Name:        "Aurora",
		Description: "Aurora - Windows desktop AI game assistant",
		Assets: application.AssetOptions{
			Handler: application.BundledAssetFileServer(uiFS),
		},
		Services: []application.Service{
			application.NewService(inputBindings),
			application.NewService(agentBindings),
		},
		OnShutdown: func() {
			app.shutdown(wailsruntime.GetContext())
		},
	})

	// 注入到 wailsruntime bridge，使所有 main.go 回调都能访问 app / mainWindow
	wailsruntime.App = v3App

	// 创建主窗口
	mainWindow := v3App.Window.NewWithOptions(application.WebviewWindowOptions{
		Name:              "main",
		Title:             "Aurora",
		Width:             cfg.WindowWidth,
		Height:            cfg.WindowHeight,
		BackgroundColour:  application.NewRGBA(30, 30, 30, 255),
		BackgroundType:    application.BackgroundTypeSolid,
	})
	wailsruntime.MainWindow = mainWindow

	// Phase 8E: 创建 native QuickInput window。
	//   - Frameless + AlwaysOnTop + BackgroundTypeTransparent：原生快速输入样式
	//   - Hidden: 启动时隐藏
	//   - URL: "quickinput.html"（Vite multi-page 第二个 entry；保持 Main / QuickInput
	//          两个明确的前端入口，不在 Main webview 复用 QuickInput.vue 路径）
	// 关闭按钮 hook 与 Main 一致：e.Cancel() + Hide（singleton，不销毁）。
	quickInputWindow := v3App.Window.NewWithOptions(application.WebviewWindowOptions{
		Name:             "quickinput",
		Title:            "Quick Input",
		URL:              "quickinput.html",
		Width:            560,
		Height:           180,
		Frameless:        true,
		AlwaysOnTop:      true,
		Hidden:           true,
		BackgroundType:   application.BackgroundTypeTransparent,
		BackgroundColour: application.NewRGBA(0, 0, 0, 0),
	})
	wailsruntime.QuickInputWindow = quickInputWindow
	quickInputWindow.RegisterHook(events.Common.WindowClosing, func(e *application.WindowEvent) {
		log.Info("QuickInput window close requested → hiding (singleton)")
		e.Cancel()
		wailsruntime.HideQuickInputWindow()
		// 同步状态机（避免下一次 Show 误判）
		if mgr := input.GetQuickInput(); mgr != nil {
			mgr.Hide()
		}
	})

	// 注入 QuickInputManager driver：状态机 Show/Hide 委派到 native window。
	// internal/input 不直接 import wails/v3；通过 driver 接口解耦（CLAUDE.md §2）。
	quickInput.SetDriver(input.NewFuncDriver(
		wailsruntime.ShowQuickInputWindow,
		wailsruntime.HideQuickInputWindow,
		func() bool { return wailsruntime.QuickInputWindow != nil && wailsruntime.QuickInputWindow.IsVisible() },
	))

	// 注册系统级快捷键 Ctrl+Alt+R（v3 GlobalShortcut：系统级热键，OS 拥有）。
	// 替代 8E 之前的 Win32 RegisterHotKey，避免与 v3 GlobalShortcut 双注册。
	// PTT (Ctrl+Shift+R) / CycleMode (Ctrl+Alt+V) 仍走 Win32（out of 8E scope）。
	if err := v3App.GlobalShortcut.Register("Ctrl+Alt+R", func() {
		log.Debug("QuickInput global shortcut (Ctrl+Alt+R) triggered")
		quickInput.Toggle()
	}); err != nil {
		log.Errorf("Failed to register Ctrl+Alt+R global shortcut: %v", err)
	}

	// Phase 8D: 构造 v3 native SystemTray 并 attach menu。
	// v3App 创建后调一次；handlers/RegisterRadioItem 已在 main 早期注册，
	// 这里把它们绑到 v3 MenuItem.OnClick。
	// 必须在 mainWindow 创建之后、v3App.Run 之前。
	traySysTray := v3App.SystemTray.New()
	if err := tray.BuildMenuAndAttach(traySysTray); err != nil {
		log.Errorf("Failed to build system tray menu: %v", err)
	}

	// Phase 5 Window Lifecycle：关闭主窗口 = 隐藏而非退出。
	// Aurora 是 desktop companion，需要保持进程运行以便：
	//   - 全局快捷键 (Ctrl+Alt+R) 继续工作
	//   - 托盘菜单可用
	//   - Voice / WakeWord 后台监听继续
	// 真正的退出只能通过托盘 Exit。
	mainWindow.RegisterHook(events.Common.WindowClosing, func(e *application.WindowEvent) {
		log.Info("Main window close requested → hiding instead of quitting (use tray Exit to quit)")
		e.Cancel() // 阻止默认 close（窗口销毁），改为隐藏
		wailsruntime.HideMainWindow()
	})

	// 替换 v2 OnStartup：订阅 ApplicationStarted 事件。
	v3App.Event.OnApplicationEvent(events.Common.ApplicationStarted, func(_ *application.ApplicationEvent) {
		app.startup(wailsruntime.GetContext())
		agent.SetRuntimeContext(wailsruntime.GetContext())
		log.Debug("Wails v3 application started")

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
			wailsruntime.EmitEvent("log", map[string]string{
				"level":   levelStr,
				"message": message,
			})
		})

		// Phase 5: 订阅 overlayVisibilityChanged — 用于刷新托盘菜单文本。
		// 触发场景：
		//   - 用户点击托盘菜单（自身 toggle）
		//   - 前端通过 setOverlayVisible 调用（前端 UI 关闭 Overlay）
		// 两种路径都会广播此事件；订阅后能保持托盘菜单文本与真实状态一致。
		wailsruntime.SubscribeEvent("overlayVisibilityChanged", func(visible ...interface{}) {
			if len(visible) == 0 {
				return
			}
			v, ok := visible[0].(bool)
			if !ok {
				log.Debugf("overlayVisibilityChanged: non-bool payload %T", visible[0])
				return
			}
			tray.Current().SetOverlayMenuState(v)
			log.Debugf("overlayVisibilityChanged → tray menu updated to visible=%v", v)
		})
	})

	if err := v3App.Run(); err != nil {
		log.Fatalf("Failed to run Wails app: %v", err)
	}
}
