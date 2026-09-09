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
	"github.com/lipipi/aurora/internal/lifecycle"
	"github.com/lipipi/aurora/internal/log"
	"github.com/lipipi/aurora/internal/platform"
	"github.com/lipipi/aurora/internal/speech"
	"github.com/lipipi/aurora/internal/tray"
	"github.com/lipipi/aurora/internal/tts"
	"github.com/lipipi/aurora/internal/wailsruntime"
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

// cfgOverlayW/H: overlay 窗口初始尺寸（config 自定义值优先，否则 360x420 preset）。
// 与 wailsruntime overlayWindowWidth/Height 常量保持一致。
func cfgOverlayW(cfg *config.Config) int {
	if cfg.Overlay.Width > 0 {
		return cfg.Overlay.Width
	}
	return 360
}

func cfgOverlayH(cfg *config.Config) int {
	if cfg.Overlay.Height > 0 {
		return cfg.Overlay.Height
	}
	return 420
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
	// Overlay 锁定态（HUD vs Edit Mode）同样启动时继承 config.Overlay.Locked
	agent.InitOverlayLockedState()

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
	// 必须走 lifecycle.Exit(0)：让所有 lifecycle.RegisterOnExit 钩子跑完 + 显式
	// UnregisterAll v3 GlobalShortcut + 强制 os.Exit(0) 兜底任何残留 goroutine。
	// 之前直接调 wailsruntime.Quit() 走 Wails 自身流程，导致 lifecycle hooks 一次
	// 都不会执行（5s game detect 等无 cancel 的 goroutine 残留 → 进程 hang）。
	tray.Current().SetOnExitHandler(func() {
		log.Info("Tray Exit: triggering lifecycle.Exit(0)")
		lifecycle.Exit(0)
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
				// 用 lifecycle.QuitContext() 替代 context.Background()，让下载
				// 在应用退出时可被取消，避免退出后下载继续 / 下载完成后还要启动
				// VoiceController 的"退出后启动"残留路径。
				dlCtx, dlCancel := lifecycle.QuitContext()
				go func() {
					defer dlCancel()
					progress := func(percent int, msg string) {
						log.Infof("Wake word download: %d%% - %s", percent, msg)
						wailsruntime.EmitEvent("wakewordDownloadProgress", map[string]interface{}{
							"percent": percent,
							"message": msg,
						})
					}
					if err := wakeword.AutoDownloadIfNeeded(dlCtx, cfg.WakeWord, config.GetAppDataDir(), progress); err != nil {
						if dlCtx.Err() != nil {
							log.Info("Wake word auto-download cancelled by app quit")
						} else {
							log.Errorf("Wake word auto-download failed: %v", err)
							wailsruntime.EmitEvent("wakewordDownloadError", map[string]string{"error": err.Error()})
						}
						return
					}
					// 下载成功但应用已退出 — 不再启动 VoiceController
					select {
					case <-lifecycle.QuitCh():
						log.Info("Wake word downloaded but app is quitting, not starting voice controller")
						return
					default:
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

	// UI 重构：所有全局快捷键（含 PTT / CycleMode / QuickInput）统一由
	// input.ShortcutManager 接管（单一注册系统、可配置、冲突检测），
	// 在 v3App 创建后统一注册（见下方 v3App 创建后段）。
	// 旧 Win32 RegisterHotKey 路径（PTT Ctrl+Shift+R / CycleMode Ctrl+Alt+V）
	// 与硬编码 v3 GlobalShortcut（Ctrl+Alt+R）均已移除，避免双注册冲突。

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
	// 顺序至关重要（见 plan）：
	//   1. UnregisterAll v3 GlobalShortcut — 必须在 Wails Quit 之前；
	//      内部 driver 调用 v3App.GlobalShortcut.Unregister，Wails 销毁后
	//      handle 失效会导致下一次启动 OS 报 "hotkey already registered"。
	//   2. wailsruntime.Quit() — 通知 Wails 销毁窗口（非阻塞）。
	//   3. PTT / VoiceController / TTS 停 — 释放麦克风 / 唤醒词 / 语音播 goroutine。
	//   4. input.Shutdown / agent / platform / config / log 依次关。
	// lifecycle.Exit(0) 已先 CloseQuitCh()，所有 select 监听 QuitCh() 的 goroutine
	// 会在 hook 执行期间收到信号；os.Exit(0) 兜底任何仍未退出的 goroutine。
	lifecycle.RegisterOnExit(func() {
		log.Info("Cleaning up...")

		// 1. 注销所有 v3 GlobalShortcut（OS 级唯一清理机会）
		if sm := input.GetShortcutManager(); sm != nil {
			sm.UnregisterAll()
		}

		// 2. Flush Overlay 待落盘几何。
		//    注意：App.Quit() → cleanup() 会遍历所有 window 调 Close()，因此 Overlay 的
		//    WindowClosing hook 在 shutdown 路径下也会触发。WindowClosing 内会再 flush
		//    一次（幂等 no-op），并且已通过 lifecycle.IsShuttingDown() 区分，
		//    不会在退出时误把 Overlay.Enabled 写成 false。
		//    必须在 wailsruntime.Quit() 之前调——窗口还在时 Position()/Size() 才准。
		agent.FlushPendingOverlayGeometry()

		// 3. 通知 Wails 退出（会触发所有窗口的 WindowClosing 事件）
		wailsruntime.Quit()

		// 3. PTT / 麦克风释放
		if ptt := input.GetPTT(); ptt != nil {
			ptt.CancelRecording()
		}

		// 4. VoiceController 停止（cancel ctx → eventLoop / copyAudioToASR / continuousTimeoutLoop 全退）
		if voiceControllerRef != nil {
			voiceControllerRef.Stop()
		}

		// 5. TTS 播放停止
		if ttsProvider != nil {
			ttsProvider.Stop()
		}

		// 6. 旧 Win32 热键 + 输入管理器
		input.Shutdown()

		// 7. Agent Core
		agent.Shutdown()

		// 8. 平台 / 配置 / 日志
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
		Name:             "main",
		Title:            "Aurora",
		Width:            cfg.WindowWidth,
		Height:           cfg.WindowHeight,
		BackgroundColour: application.NewRGBA(30, 30, 30, 255),
		BackgroundType:   application.BackgroundTypeSolid,
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
		if lifecycle.IsShuttingDown() {
			// 应用整体退出：不 Cancel（不阻止关闭），也不需要同步状态机。
			log.Info("QuickInput WindowClosing during shutdown → skipping hide")
			return
		}
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

	// Phase 8F: 创建 native Overlay window。
	//   - Frameless + AlwaysOnTop + BackgroundTypeTransparent：
	//     HUD companion 风格
	//   - Hidden: 可见性由 overlayVisibleState 驱动（overlayVisibilityChanged 订阅），
	//     启动时在 ApplicationStarted 里按初始状态决定是否 Show
	//   - 默认位置硬编码：主屏 WorkArea bottom-right + 16px margin
	//     （wailsruntime.setDefaultOverlayPosition，2026-09-07 Anchor 移除后）
	//   - Resize 由 locked 态动态驱动：Edit Mode SetResizable(true) + Wails 5px
	//     边缘 native resize；HUD SetResizable(false) + SetIgnoreMouseEvents(true)
	//     全 click-through。DisableResize 默认不设。
	//   - 拖拽完全交给 Wails v3 native：CSS --wails-draggable: drag 触发 OS 拖动
	//   - WindowDidMove / WindowDidResize hook 由 main.go 单独注册到 overlayWindow
	//     （见下），300ms Go-side debounce 持久化几何
	// 关闭按钮 → SetOverlayVisibleAndBroadcast(false) 走统一状态口（托盘自动同步），
	// 订阅方收到事件后调 HideOverlayWindow —— 窗口已隐藏时 Hide 是幂等 no-op，
	// 且 Hide 不会反向触发事件，无递归/重复关闭问题。
	// 注意：App.Quit() → cleanup() 也会遍历所有 window 调 Close() 触发 WindowClosing，
	// 此时 WindowClosing handler 必须通过 lifecycle.IsShuttingDown() 区分，
	// 不能在退出路径下把 Overlay.Enabled 误写成 false。
	overlayWindow := v3App.Window.NewWithOptions(application.WebviewWindowOptions{
		Name:  "overlay",
		Title: "Aurora Overlay",
		URL:   "overlay.html",
		// config.Overlay.Width/Height 非零时为用户自定义尺寸（Edit Mode 持久化），
		// 否则 360x420 preset（与 wailsruntime overlayWindowWidth/Height 常量一致）
		Width:            cfgOverlayW(cfg),
		Height:           cfgOverlayH(cfg),
		Frameless:        true,
		AlwaysOnTop:      true,
		Hidden:           true,
		BackgroundType:   application.BackgroundTypeTransparent,
		BackgroundColour: application.NewRGBA(0, 0, 0, 0),
		// 50ms OS-level 事件合并（避免 pointermove 60Hz 直接打到 Go），
		// 真实持久化 debounce 在 agent.SchedulePersistOverlayGeometry（300ms）
		Windows: application.WindowsWindow{
			WindowDidMoveDebounceMS: 50,
			// 关闭 Wails 对 Frameless 窗口默认的 DwmExtendFrameIntoClientArea
			// （"frameless decorations"）：它会给透明窗口叠加 DWM frame 外观
			// （Win11 1px 系统边框 + 圆角）→ 透明 margin 带透出白边
			// （2026-09-07 白边根因 B 的 native 侧另一半；DWM 属性部分由
			// wailsruntime.applyOverlayDWMTransparency 在 Show 时设置）。
			// Frameless 的 5px native resize 走 resizeBorderHitTest，
			// 不依赖 decorations，不受影响。
			DisableFramelessWindowDecorations: true,
		},
	})
	wailsruntime.OverlayWindow = overlayWindow
	overlayWindow.RegisterHook(events.Common.WindowClosing, func(e *application.WindowEvent) {
		// 先 flush 几何：无论用户主动关还是应用整体退出，最后位置/尺寸都要落盘。
		agent.FlushPendingOverlayGeometry()

		if lifecycle.IsShuttingDown() {
			// 应用整体退出路径：App.Quit() → cleanup() → 遍历所有 window 调 Close()
			// → 触发 WindowClosing。此时不应该改变 Overlay.Enabled 持久化状态，
			// 也不需要 Cancel（进程反正要退出，窗口由 OS 回收）。
			// 保持用户退出前的可见性偏好：开着退出 → 下次还开着；关着退出 → 下次还关着。
			log.Info("Overlay WindowClosing during shutdown → preserving Enabled state, skipping hide")
			return
		}

		// 用户主动关闭 Overlay（点击关闭按钮 / Alt+F4 等）：Cancel + 转 Hide，
		// 并持久化 Enabled=false，下次启动不自动显示。
		log.Info("Overlay window close requested → hiding via SetOverlayVisibleAndBroadcast(false)")
		e.Cancel()
		agent.SetOverlayVisibleAndBroadcast(false)
	})

	// Phase 8F + UI 重构：OS 完成窗口移动 / 缩放 → 写 cfg + 广播。
	// Native Window is the only Geometry Source of Truth：前端 Edit Mode
	// 不发任何 geometry IPC（无 SetPosition / SetSize / commit）；拖拽 / resize
	// 完全由 Wails v3 native 机制处理，OS 实际完成窗口状态变更后通过这两个
	// hook 通知 Go 侧。Go 侧负责 300ms debounce 合并 + clamp + Save + 广播。
	overlayWindow.RegisterHook(events.Common.WindowDidMove, func(_ *application.WindowEvent) {
		x, y, w, h := wailsruntime.GetOverlayWindowBounds()
		agent.SchedulePersistOverlayGeometry(x, y, w, h)
	})
	overlayWindow.RegisterHook(events.Common.WindowDidResize, func(_ *application.WindowEvent) {
		x, y, w, h := wailsruntime.GetOverlayWindowBounds()
		agent.SchedulePersistOverlayGeometry(x, y, w, h)
	})
	// WindowHide（HUD 切换 / 快捷键隐藏 / 关闭按钮）：同样 flush
	// （与 WindowClosing 路径不同的入口，但语义相同：落盘最后位置）
	overlayWindow.RegisterHook(events.Common.WindowHide, func(_ *application.WindowEvent) {
		agent.FlushPendingOverlayGeometry()
	})

	// [DIAGNOSTIC] Bug B 诊断：追踪 Overlay WebView 导航完成时间点
	// 2026-09-09 修复：启动时的初始 Show 从 ApplicationStarted 移到这里。
	// 原因：ApplicationStarted 时窗口尚未真正创建（size=0x0），Show 是空操作；
	// ~1s 后窗口实现化时 OS 给出默认位置触发 DidMove/DidResize，此时 500ms
	// suppress 早已过期，坏位置被写入 config 覆盖用户配置。
	// WebViewNavigationCompleted 时窗口已就绪，SetGeometry + Show 能真正生效，
	// 且 suppress 能覆盖后续的瞬态 OS 事件。
	overlayWindow.RegisterHook(events.Windows.WebViewNavigationCompleted, func(_ *application.WindowEvent) {
		owW, owH := overlayWindow.Size()
		log.Debugf("[DIAG-BugB] Overlay WebViewNavigationCompleted: visible=%v, size=%dx%d",
			overlayWindow.IsVisible(), owW, owH)

		// 启动时的初始显示：仅当 overlayVisibleState==true（继承自 cfg.Overlay.Enabled）
		// 且窗口尚未可见（避免与后续 overlayVisibilityChanged 事件重复触发）
		if agent.GetOverlayVisibleState() && !overlayWindow.IsVisible() {
			log.Debugf("[DIAG-BugB] WebViewNavigationCompleted: applying initial geometry and showing, cfg(x=%v,y=%v,w=%v,h=%v)",
				cfg.Overlay.X, cfg.Overlay.Y, cfg.Overlay.Width, cfg.Overlay.Height)
			// SetOverlayGeometry 和 ShowOverlayWindow 内部都会调用 SuppressGeometryPersist，
			// 防止后续 OS 瞬态事件被 SchedulePersistOverlayGeometry 误持久化
			wailsruntime.SetOverlayGeometry(cfg.Overlay.X, cfg.Overlay.Y, cfg.Overlay.Width, cfg.Overlay.Height)
			wailsruntime.ShowOverlayWindow()
			log.Debugf("[DIAG-BugB] WebViewNavigationCompleted: after show, visible=%v", overlayWindow.IsVisible())
		}
	})

	// Phase 8F: overlay 可见性 → native 窗口 Show/Hide。
	// overlayVisibleState 的唯一写入口仍是 agent 的 SetOverlayVisible /
	// SetOverlayVisibleAndBroadcast（前端关闭按钮 / 托盘菜单共用）；
	// 这里只订阅事件把状态投影到窗口，不引入第二个状态源。
	wailsruntime.SubscribeEvent("overlayVisibilityChanged", func(visible ...interface{}) {
		if len(visible) == 0 {
			return
		}
		v, ok := visible[0].(bool)
		if !ok {
			log.Debugf("overlayVisibilityChanged: non-bool payload %T", visible[0])
			return
		}
		if v {
			// Show 前先按当前几何定位（首次 Show 或 Reset 后都正确）
			log.Debugf("[DIAG-BugB] overlayVisibilityChanged show: before, visible=%v", overlayWindow.IsVisible())
			wailsruntime.SetOverlayGeometry(cfg.Overlay.X, cfg.Overlay.Y, cfg.Overlay.Width, cfg.Overlay.Height)
			wailsruntime.ShowOverlayWindow()
			log.Debugf("[DIAG-BugB] overlayVisibilityChanged show: after, visible=%v", overlayWindow.IsVisible())
		} else {
			log.Debugf("[DIAG-BugB] overlayVisibilityChanged hide: before, visible=%v", overlayWindow.IsVisible())
			wailsruntime.HideOverlayWindow()
			log.Debugf("[DIAG-BugB] overlayVisibilityChanged hide: after, visible=%v", overlayWindow.IsVisible())
		}
	})

	// Phase 8F: overlay 配置保存 → 几何字段可能变化 → 重新定位窗口。
	// 其他字段（opacity/font_size/过滤开关）由 Overlay webview 监听同一事件
	// refetch 配置实时应用，Go 侧只关心窗口几何。
	// 2026-09-07 Anchor 字段移除：cfg.Overlay.Anchor 不再存在。
	wailsruntime.SubscribeEvent("overlayConfigChanged", func(...interface{}) {
		wailsruntime.SetOverlayGeometry(cfg.Overlay.X, cfg.Overlay.Y, cfg.Overlay.Width, cfg.Overlay.Height)
	})

	// UI 重构：统一快捷键系统。
	// 单一注册系统 = v3 GlobalShortcut（OS 拥有，系统级）；input.ShortcutManager
	// 通过 driver 接口委托注册/注销（internal/input 不 import wails，CLAUDE.md §2）。
	// 组合键来自 config.Shortcuts（可配置 + 冲突检测 + 运行时重注册 + 持久化），
	// 不在任何地方硬编码具体按键。
	// driver：委托到 v3 GlobalShortcut（v3ShortcutDriver 闭包实现，见下方）
	shortcutDriver := input.FuncShortcutDriver(
		func(combo string, cb func()) error {
			return v3App.GlobalShortcut.Register(combo, cb)
		},
		func(combo string) error {
			return v3App.GlobalShortcut.Unregister(combo)
		},
	)
	shortcutMgr := input.NewShortcutManager(shortcutDriver)
	input.SetShortcutManager(shortcutMgr)

	// 注入 bindings 事件广播（input 包不 import wails）
	inputBindings.SetEventEmitter(func(name string, data ...any) {
		wailsruntime.EmitEvent(name, data...)
	})
	inputBindings.SetShortcutPersistence(func(combos map[string]string) {
		cfg.Shortcuts = combos
		if err := config.Save(); err != nil {
			log.Errorf("Failed to persist shortcuts: %v", err)
		}
	})

	// 9 个 action 的运行时 handler（语义与旧硬编码路径一致，额外新增
	// toggle_main_window / toggle_overlay_lock / emergency_hide / analyze_screen / stop_speaking）
	shortcutMgr.SetHandlers(map[input.ShortcutAction]func(){
		input.ShortcutQuickInput: func() {
			quickInput.Toggle()
		},
		input.ShortcutPTT: func() {
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
		input.ShortcutCycleVoiceMode: func() {
			if voiceControllerRef == nil {
				log.Debug("Cycle mode shortcut: voice controller not ready")
				return
			}
			newMode := voiceControllerRef.CycleMode(context.Background())
			log.Infof("Cycle mode shortcut: switched to %s", newMode.DisplayName())
		},
		input.ShortcutToggleMainWindow: func() {
			if wailsruntime.MainWindow != nil && wailsruntime.MainWindow.IsVisible() {
				wailsruntime.HideMainWindow()
			} else {
				wailsruntime.ShowMainWindow()
			}
		},
		input.ShortcutToggleOverlay: func() {
			next := !agent.GetOverlayVisibleState()
			log.Debugf("Overlay toggle shortcut: %v -> %v", !next, next)
			agent.SetOverlayVisibleAndBroadcast(next)
		},
		input.ShortcutToggleOverlayLock: func() {
			next := !agent.GetOverlayLockedState()
			log.Debugf("Overlay lock toggle shortcut: %v -> %v", !next, next)
			agentBindings.SetOverlayLocked(next)
		},
		input.ShortcutEmergencyHide: func() {
			log.Info("Emergency hide shortcut triggered")
			wailsruntime.EmergencyHideAll()
		},
		input.ShortcutAnalyzeScreen: func() {
			if !agentBindings.IsReady() {
				log.Debug("Analyze screen shortcut: agent not ready")
				return
			}
			if err := agentBindings.ChatStream("请分析当前游戏画面，给出简短建议"); err != nil {
				log.Errorf("Analyze screen shortcut failed: %v", err)
			}
		},
		input.ShortcutStopSpeaking: func() {
			if voiceControllerRef == nil {
				return
			}
			voiceControllerRef.PTTPreempt()
		},
	})

	// 应用 config.Shortcuts 注册全部快捷键（逐项失败只记日志，不阻塞启动）
	shortcutMgr.ApplyAll(cfg.Shortcuts)

	// PTT 释放轮询同步当前组合键（ptt_windows.go 按组合检测释放）
	if ptt := input.GetPTT(); ptt != nil {
		if mods, vk, ok := shortcutMgr.PTTCombo(); ok {
			ptt.SetPTTCombo(mods, vk)
		}
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
		if lifecycle.IsShuttingDown() {
			// 应用整体退出：不 Cancel（不阻止关闭）。
			// App.Quit() → cleanup() 会遍历所有 window 调 Close()，
			// 此时进程即将退出，不需要再 Hide。
			log.Info("Main window closing during shutdown → proceeding with quit")
			return
		}
		log.Info("Main window close requested → hiding instead of quitting (use tray Exit to quit)")
		e.Cancel() // 阻止默认 close（窗口销毁），改为隐藏
		wailsruntime.HideMainWindow()
	})

	// 替换 v2 OnStartup：订阅 ApplicationStarted 事件。
	v3App.Event.OnApplicationEvent(events.Common.ApplicationStarted, func(_ *application.ApplicationEvent) {
		app.startup(wailsruntime.GetContext())
		agent.SetRuntimeContext(wailsruntime.GetContext())
		log.Debug("Wails v3 application started")

		// Phase 8F: overlay 初始可见性 —— overlayVisibleState 启动时已从
		// cfg.Overlay.Enabled 继承（agent.InitOverlayVisibleState）。
		// 2026-09-09 修复：实际的 SetGeometry + Show 移到 WebViewNavigationCompleted，
		// 因为 ApplicationStarted 时窗口尚未真正创建（size=0x0），Show 是空操作，
		// 反而让后续 OS 实现化触发的 DidMove/DidResize 把默认位置写入 config。
		// 这里只保留日志，确认状态继承正确。
		if agent.GetOverlayVisibleState() {
			log.Debugf("[DIAG-BugB] Startup: overlayVisibleState=true, deferred to WebViewNavigationCompleted, cfg(x=%v,y=%v,w=%v,h=%v)",
				cfg.Overlay.X, cfg.Overlay.Y, cfg.Overlay.Width, cfg.Overlay.Height)
		}

		// 应用 overlay 初始 native 窗口态：与 cfg.Overlay.Locked 保持一致。
		// InitOverlayLockedState 在更早阶段（仅写进程内变量，无窗口），
		// 这里在 OverlayWindow 已创建后驱动 SetResizable / SetIgnoreMouseEvents。
		agent.SetOverlayLockedAndBroadcast(agent.GetOverlayLockedState())
		log.Debugf("Overlay native state applied at startup: locked=%v", agent.GetOverlayLockedState())

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

		// UI 重构：补全 gameDetect 事件 —— 前端 App.vue 一直监听此事件但后端
		// 从未发射（审计发现的死监听）。5s 轮询 GetActiveGame，状态变化时广播；
		// 同时实现"游戏启动自动锁定 Overlay"（Edit Mode → HUD），游戏结束后
		// 只解除由自动锁定加上的锁（不覆盖用户手动锁定的选择）。
		gameAutoLocked := false
		go func() {
			lastActive := false
			for {
				if agentBindings.IsReady() {
					game := agentBindings.GetActiveGame()
					active, _ := game["active"].(bool)
					if active != lastActive {
						wailsruntime.EmitEvent("gameDetect", game)
						lastActive = active
					}
					// 自动锁定 / 解锁（仅处理自动锁定引入的状态，不碰用户手动锁定）
					if active && !agent.GetOverlayLockedState() && !gameAutoLocked {
						agentBindings.SetOverlayLocked(true)
						gameAutoLocked = true
						log.Info("Game detected → overlay auto-locked (HUD mode)")
					} else if !active && gameAutoLocked {
						agentBindings.SetOverlayLocked(false)
						gameAutoLocked = false
						log.Info("Game ended → overlay auto-unlocked")
					}
				}
				// 关键：select 监听 lifecycle.QuitCh() 让退出时此 goroutine 立即退出，
				// 否则 5s 死循环 + 永 no-cancel 会让 Go runtime 永不退出（Wails 已销毁 window 也没用）。
				select {
				case <-lifecycle.QuitCh():
					log.Debug("gameDetect loop: quitting")
					return
				case <-time.After(5 * time.Second):
				}
			}
		}()
	})

	if err := v3App.Run(); err != nil {
		log.Fatalf("Failed to run Wails app: %v", err)
	}
}
