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

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/runtime"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
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

	// Phase 5: 托盘 Overlay 菜单 — 区分 Enabled（持久化配置）和 Visible（运行时态）。
	// 1. 菜单点击：读取 agent 进程内可见性状态 → 取反 → 写回 + 广播 overlayVisibilityChanged
	//    事件，前端与 main.go 自身都会收到。
	// 2. main.go 同时订阅 overlayVisibilityChanged：收到后刷新托盘菜单文本。
	//    覆盖了"前端关闭 Overlay 时托盘菜单文本也要同步"的场景。
	tray.Current().SetOnToggleOverlayHandler(func() {
		if runtimeCtx == nil {
			log.Debug("Tray overlay toggle: runtime not ready")
			return
		}
		// 读取进程内真实可见性（不是 config.Overlay.Enabled）
		current := agent.GetOverlayVisibleState()
		next := !current
		log.Debugf("Tray overlay toggle: %v -> %v", current, next)
		agent.SetOverlayVisibleAndBroadcast(next)
		// 注意：tray 菜单文本由下面的 EventsOn("overlayVisibilityChanged") 统一刷新，
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
	for _, m := range []input.VoiceMode{input.ModePTT, input.ModeWakeWord, input.ModeContinuous} {
		mode := m // 捕获循环变量
		if err := tray.Current().AddSubMenu("语音模式", mode.DisplayName(), func() {
			if voiceControllerRef == nil {
				log.Debug("Tray voice mode: voice controller not ready")
				return
			}
			if err := voiceControllerRef.SetMode(context.Background(), mode); err != nil {
				log.Errorf("Failed to set voice mode from tray: %v", err)
			}
		}); err != nil {
			log.Errorf("Failed to add tray submenu %s: %v", mode.DisplayName(), err)
		}
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
						if runtimeCtx != nil {
							runtime.EventsEmit(runtimeCtx, "voiceStateChange", map[string]interface{}{
								"state": state.String(),
								"code":  int(state),
							})
						}
					}),
					input.WithOnModeChange(func(mode input.VoiceMode) {
						if runtimeCtx != nil {
							runtime.EventsEmit(runtimeCtx, "voiceModeChange", map[string]interface{}{
								"mode":      mode.String(),
								"code":      int(mode),
								"mode_name": mode.DisplayName(),
							})
						}
						// 同步更新托盘子菜单 radio 状态
						for _, m := range []input.VoiceMode{input.ModePTT, input.ModeWakeWord, input.ModeContinuous} {
							_ = tray.Current().SetMenuItemChecked("语音模式", m.DisplayName(), m == mode)
						}
						// 持久化模式选择
						cfg.VoiceMode = mode.String()
						if err := config.Save(); err != nil {
							log.Warnf("Failed to persist voice mode: %v", err)
						}
					}),
					input.WithOnTranscript(func(t speech.Transcript) {
						if runtimeCtx != nil {
							runtime.EventsEmit(runtimeCtx, "voiceTranscript", map[string]interface{}{
								"text":       t.Text,
								"is_final":   t.IsFinal,
								"session_id": t.SessionID,
							})
						}
					}),
					input.WithOnWakeDetected(func() {
						if runtimeCtx != nil {
							runtime.EventsEmit(runtimeCtx, "wakeWordDetected", nil)
							// Bring window to front when wake detected
							runtime.WindowUnminimise(runtimeCtx)
							runtime.WindowShow(runtimeCtx)
						}
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

				// 把引用注入到 agent bindings（前端 GetVoiceMode/SetVoiceMode 使用）
				agent.SetVoiceController(voiceControllerRef)

				// 同步托盘子菜单 radio 状态
				for _, m := range []input.VoiceMode{input.ModePTT, input.ModeWakeWord, input.ModeContinuous} {
					_ = tray.Current().SetMenuItemChecked("语音模式", m.DisplayName(), m == initialMode)
				}

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
						if runtimeCtx != nil {
							runtime.EventsEmit(runtimeCtx, "wakewordDownloadProgress",
								map[string]interface{}{
									"percent": percent,
									"message": msg,
								})
						}
					}
					if err := wakeword.AutoDownloadIfNeeded(context.Background(), cfg.WakeWord, config.GetAppDataDir(), progress); err != nil {
						log.Errorf("Wake word auto-download failed: %v", err)
						if runtimeCtx != nil {
							runtime.EventsEmit(runtimeCtx, "wakewordDownloadError",
								map[string]string{"error": err.Error()})
						}
						return
					}
					log.Info("Wake word: all files downloaded successfully")
					if runtimeCtx != nil {
						runtime.EventsEmit(runtimeCtx, "wakewordDownloadComplete", nil)
					}
					// Download finished - start voice controller now
					startVoiceController()
				}()
			} else {
				// All files already downloaded - start voice controller immediately
				startVoiceController()
			}
		}
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

	// 快速输入显示/隐藏切换回调 - 通知前端显示/隐藏
	//
	// Phase 2 重构（Quick Input 与 Main Window 解耦）：
	// 不再调用 runtime.WindowShow / WindowUnminimise / WindowActivate。
	// Quick Input 必须独立于 Main Window 工作 —— 不能因为用户按全局快捷键
	// 提问，就把 Main Window 拉到前台而打扰用户（任务 §6）。
	//
	// 已知限制（不在本阶段范围，留给 Window Lifecycle Phase）：
	// Quick Input 当前渲染在 Main Window 的 webview 内，因此当 Main Window
	// 被隐藏 / 最小化时 Quick Input 也无法被用户看到。
	// 完整解决需要把 Quick Input 提升为独立 Wails Window（任务 §6 Case B/C/D）。
	// 本阶段保证 Main Window 可见时 Quick Input 行为正确，且不再强制 Show 主窗口。
	//
	// 同时在 hide 时也 emit showQuickInput(show=false)，
	// 修复 Quick Input toggle 状态在前后端 desync 的既有 bug：
	// 旧实现只在 show=true 时 emit，导致前端 visible 状态卡在 true。
	quickInput.RegisterToggleCallback(func(show bool) {
		if runtimeCtx == nil {
			log.Debug("Runtime not ready, cannot notify front")
			return
		}
		log.Debugf("QuickInput toggle: show=%v", show)
		runtime.EventsEmit(runtimeCtx, "showQuickInput", show)
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
		Title:  "Aurora",
		Width:  cfg.WindowWidth,
		Height: cfg.WindowHeight,
		AssetServer: &assetserver.Options{
			Assets: uiFS,
		},
		BackgroundColour: &options.RGBA{R: 30, G: 30, B: 30, A: 1},
		// Phase 5 Window Lifecycle：关闭主窗口 = 隐藏而非退出。
		// Aurora 是 desktop companion，需要保持进程运行以便：
		//   - 全局快捷键 (Ctrl+Alt+R) 继续工作
		//   - 托盘菜单可用
		//   - Voice / WakeWord 后台监听继续
		// 真正的退出只能通过托盘 Exit。
		HideWindowOnClose: true,
		OnBeforeClose: func(ctx context.Context) (prevent bool) {
			// 即使用户点了 X，也仅隐藏主窗口，不退出进程。
			// 阻止 Wails 默认的 close 流程，并把窗口隐藏。
			log.Info("Main window close requested → hiding instead of quitting (use tray Exit to quit)")
			runtime.WindowHide(ctx)
			return true
		},
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

			// Phase 5: 订阅 overlayVisibilityChanged — 用于刷新托盘菜单文本。
			// 触发场景：
			//   - 用户点击托盘菜单（自身 toggle）
			//   - 前端通过 setOverlayVisible 调用（前端 UI 关闭 Overlay）
			// 两种路径都会广播此事件；订阅后能保持托盘菜单文本与真实状态一致。
			runtime.EventsOn(ctx, "overlayVisibilityChanged", func(visible ...interface{}) {
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
		},
		OnShutdown: app.shutdown,
		Bind: bindings,
	})

	if err != nil {
		log.Fatalf("Failed to run Wails app: %v", err)
	}
}
