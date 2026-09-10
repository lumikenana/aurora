package input

import (
	"fmt"
	"strings"
	"sync"

	"github.com/lipipi/aurora/internal/log"
)

// ShortcutAction 快捷键动作 ID（前端 Shortcut 页面按此 ID 分组展示）
type ShortcutAction string

const (
	// ShortcutQuickInput 切换快速输入窗口
	ShortcutQuickInput ShortcutAction = "quick_input"
	// ShortcutPTT Push-To-Talk（按住说话）
	ShortcutPTT ShortcutAction = "ptt"
	// ShortcutToggleMainWindow 显示 / 隐藏主窗口
	ShortcutToggleMainWindow ShortcutAction = "toggle_main_window"
	// ShortcutToggleOverlay 显示 / 隐藏 Overlay
	ShortcutToggleOverlay ShortcutAction = "toggle_overlay"
	// ShortcutToggleOverlayLock 锁定 / 解锁 Overlay（锁定 = HUD 模式，禁拖拽）
	ShortcutToggleOverlayLock ShortcutAction = "toggle_overlay_lock"
	// ShortcutEmergencyHide 紧急隐藏：隐藏所有 Aurora 窗口（后台服务不受影响）
	ShortcutEmergencyHide ShortcutAction = "emergency_hide"
	// ShortcutAnalyzeScreen 截屏并让 AI 分析当前画面
	ShortcutAnalyzeScreen ShortcutAction = "analyze_screen"
	// ShortcutStopSpeaking 停止当前语音播报 / 打断 AI
	ShortcutStopSpeaking ShortcutAction = "stop_speaking"
)

// AllShortcutActions 全部动作（注册时遍历顺序稳定）
var AllShortcutActions = []ShortcutAction{
	ShortcutQuickInput,
	ShortcutPTT,
	ShortcutToggleMainWindow,
	ShortcutToggleOverlay,
	ShortcutToggleOverlayLock,
	ShortcutEmergencyHide,
	ShortcutAnalyzeScreen,
	ShortcutStopSpeaking,
}

// IsValidShortcutAction action 是否存在
func IsValidShortcutAction(a string) bool {
	for _, known := range AllShortcutActions {
		if string(known) == a {
			return true
		}
	}
	return false
}

// DefaultShortcuts 默认快捷键组合（combo 字符串与 Wails v3 accelerator 语法一致）
func DefaultShortcuts() map[string]string {
	return map[string]string{
		string(ShortcutQuickInput):        "Ctrl+Alt+R",
		string(ShortcutPTT):               "Ctrl+Shift+R",
		string(ShortcutToggleMainWindow):  "Ctrl+Alt+A",
		string(ShortcutToggleOverlay):     "Ctrl+Alt+O",
		string(ShortcutToggleOverlayLock): "Ctrl+Alt+L",
		string(ShortcutEmergencyHide):     "Ctrl+Alt+H",
		string(ShortcutAnalyzeScreen):     "Ctrl+Alt+G",
		string(ShortcutStopSpeaking):      "Ctrl+Alt+X",
	}
}

// ---- combo 解析 / 校验 / 归一化 ----
//
// combo 字符串形如 "Ctrl+Shift+R"。与 Wails v3 accelerator 保持同一语法，
// 同时解析出 Win32 修饰键 + 虚拟键码（供 PTT 释放轮询 / 冲突归一化使用）。

// parseCombo 解析 combo 字符串。
// 校验规则（非法即返回 error，不允许无感注册系统热键）：
//   - 至少两部分（修饰键 + 主键）
//   - 主键不能是修饰键本身
//   - 必须带至少一个修饰键（F1-F24 功能键除外）
func parseCombo(combo string) (mods Modifier, vk int, err error) {
	parts := strings.Split(combo, "+")
	if len(parts) < 2 {
		return 0, 0, fmt.Errorf("shortcut must include a modifier and a key (e.g. Ctrl+Alt+R)")
	}

	keyPart := ""
	for _, raw := range parts {
		p := strings.TrimSpace(raw)
		if p == "" {
			return 0, 0, fmt.Errorf("shortcut contains an empty part")
		}
		switch strings.ToLower(p) {
		case "ctrl", "control":
			mods |= ModCtrl
		case "shift":
			mods |= ModShift
		case "alt", "option":
			mods |= ModAlt
		case "win", "super", "meta", "cmd":
			mods |= ModWin
		default:
			if keyPart != "" {
				return 0, 0, fmt.Errorf("shortcut has more than one non-modifier key")
			}
			keyPart = p
		}
	}
	if keyPart == "" {
		return 0, 0, fmt.Errorf("shortcut has no main key (only modifiers)")
	}

	vk, ok := virtualKeyCode(keyPart)
	if !ok {
		return 0, 0, fmt.Errorf("unsupported key: %s", keyPart)
	}

	// 无修饰键仅允许 F1-F24（否则会吞掉用户所有普通按键）
	if mods == 0 && !isFunctionKey(vk) {
		return 0, 0, fmt.Errorf("shortcut needs at least one modifier (Ctrl / Alt / Shift / Win) unless it is a function key")
	}
	return mods, vk, nil
}

// normalizeCombo 将 combo 归一化为规范形式（Ctrl+Shift+Alt+Win+Key），
// 用于冲突检测："Alt+Ctrl+R" 与 "Ctrl+Alt+R" 视为同一组合。
func normalizeCombo(combo string) (string, error) {
	mods, vk, err := parseCombo(combo)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	if mods&ModCtrl != 0 {
		b.WriteString("Ctrl+")
	}
	if mods&ModShift != 0 {
		b.WriteString("Shift+")
	}
	if mods&ModAlt != 0 {
		b.WriteString("Alt+")
	}
	if mods&ModWin != 0 {
		b.WriteString("Win+")
	}
	b.WriteString(virtualKeyName(vk))
	return b.String(), nil
}

// isFunctionKey F1-F24
func isFunctionKey(vk int) bool {
	return vk >= 0x70 && vk <= 0x87
}

// virtualKeyName 虚拟键码 → 规范键名（用于归一化输出）。
// 键码表内无重复键码，任意命中名都稳定。
func virtualKeyName(vk int) string {
	for name, code := range virtualKeyTableMap {
		if code == vk {
			return name
		}
	}
	return fmt.Sprintf("VK%02X", vk)
}

// virtualKeyTableMap 支持的主键 → Windows 虚拟键码
var virtualKeyTableMap = buildVirtualKeyTable()

// virtualKeyCode 键名 → 虚拟键码（大小写不敏感）
func virtualKeyCode(name string) (int, bool) {
	vk, ok := virtualKeyTableMap[strings.ToUpper(name)]
	return vk, ok
}

func buildVirtualKeyTable() map[string]int {
	t := map[string]int{}
	// A-Z
	for i := 0; i < 26; i++ {
		t[string(rune('A'+i))] = 0x41 + i
	}
	// 0-9
	for i := 0; i < 10; i++ {
		t[string(rune('0'+i))] = 0x30 + i
	}
	// F1-F24
	for i := 0; i < 24; i++ {
		t[fmt.Sprintf("F%d", i+1)] = 0x70 + i
	}
	// 常用控制键
	extra := map[string]int{
		"SPACE":       0x20,
		"TAB":         0x09,
		"HOME":        0x24,
		"END":         0x23,
		"PAGE UP":     0x21,
		"PAGE DOWN":   0x22,
		"INSERT":      0x2D,
		"DELETE":      0x2E,
		"BACKSPACE":   0x08,
		"UP":          0x26,
		"DOWN":        0x28,
		"LEFT":        0x25,
		"RIGHT":       0x27,
		"PRINTSCREEN": 0x2C,
		"SCROLL LOCK": 0x91,
		"PAUSE":       0x13,
	}
	for k, v := range extra {
		t[k] = v
	}
	return t
}

// ---- ShortcutManager ----

// ShortcutDriver 全局快捷键注册的底层驱动（由 main.go 注入，
// 委派到 Wails v3 GlobalShortcut；internal/input 不 import wails）。
type ShortcutDriver interface {
	Register(combo string, cb func()) error
	Unregister(combo string) error
}

// FuncShortcutDriver 函数式 driver 构造器（main.go 用闭包委托到 v3 GlobalShortcut）
func FuncShortcutDriver(
	register func(combo string, cb func()) error,
	unregister func(combo string) error,
) ShortcutDriver {
	return &funcShortcutDriver{register: register, unregister: unregister}
}

type funcShortcutDriver struct {
	register   func(combo string, cb func()) error
	unregister func(combo string) error
}

func (d *funcShortcutDriver) Register(combo string, cb func()) error {
	return d.register(combo, cb)
}

func (d *funcShortcutDriver) Unregister(combo string) error {
	return d.unregister(combo)
}

// ShortcutConflictError 组合键冲突（与其他 action 重复）
type ShortcutConflictError struct {
	Action  ShortcutAction // 已占用该组合的 action
	Attempt string         // 冲突的组合
}

func (e *ShortcutConflictError) Error() string {
	return fmt.Sprintf("shortcut %s is already assigned to action %q", e.Attempt, e.Action)
}

// ShortcutManager 统一快捷键注册表：
//   - 单一注册系统（全部经 driver → v3 GlobalShortcut，OS 拥有热键）
//   - 配置驱动（config.Shortcuts），支持运行时修改 + 重注册
//   - 冲突检测（归一化后比较）
type ShortcutManager struct {
	mu       sync.Mutex
	driver   ShortcutDriver
	handlers map[ShortcutAction]func()
	combos   map[ShortcutAction]string // 当前生效组合（归一化形式）
}

var (
	shortcutManagerMu sync.Mutex
	shortcutManager   *ShortcutManager
)

// SetShortcutManager 设置全局 manager（main.go 启动时调用一次）
func SetShortcutManager(m *ShortcutManager) {
	shortcutManagerMu.Lock()
	defer shortcutManagerMu.Unlock()
	shortcutManager = m
}

// GetShortcutManager 获取全局 manager（未初始化返回 nil）
func GetShortcutManager() *ShortcutManager {
	shortcutManagerMu.Lock()
	defer shortcutManagerMu.Unlock()
	return shortcutManager
}

// NewShortcutManager 创建 manager
func NewShortcutManager(driver ShortcutDriver) *ShortcutManager {
	return &ShortcutManager{
		driver:   driver,
		handlers: map[ShortcutAction]func(){},
		combos:   map[ShortcutAction]string{},
	}
}

// SetHandlers 注入动作回调（main.go 组装，回调可引用延迟初始化的 voiceController 等）
func (m *ShortcutManager) SetHandlers(handlers map[ShortcutAction]func()) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.handlers = handlers
}

// ApplyAll 按配置注册全部快捷键（启动时调用）。
// 单条注册失败只记日志并跳过（如 OS 层被占用），不阻塞启动。
func (m *ShortcutManager) ApplyAll(combos map[string]string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, action := range AllShortcutActions {
		combo := combos[string(action)]
		if combo == "" {
			continue // 该 action 未配置 → 不注册
		}
		normalized, err := normalizeCombo(combo)
		if err != nil {
			log.Errorf("Shortcut %s: invalid combo %q: %v", action, combo, err)
			continue
		}
		if err := m.registerLocked(action, normalized); err != nil {
			log.Errorf("Shortcut %s: failed to register %q: %v", action, normalized, err)
		}
	}
}

// registerLocked 注册单条（调用方持有 mu）
func (m *ShortcutManager) registerLocked(action ShortcutAction, normalized string) error {
	handler := m.handlers[action]
	if handler == nil {
		return fmt.Errorf("no handler wired for action %s", action)
	}
	combo := normalized
	if err := m.driver.Register(combo, handler); err != nil {
		return err
	}
	// 换组合时注销旧组合
	if old, ok := m.combos[action]; ok && old != combo {
		if err := m.driver.Unregister(old); err != nil {
			log.Warnf("Shortcut %s: failed to unregister old combo %q: %v", action, old, err)
		}
	}
	m.combos[action] = combo
	return nil
}

// Combos 返回当前生效组合的快照
func (m *ShortcutManager) Combos() map[string]string {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make(map[string]string, len(m.combos))
	for k, v := range m.combos {
		out[string(k)] = v
	}
	return out
}

// SetShortcut 为 action 设置新组合（校验 + 冲突检测 + 重注册）。
// 冲突返回 *ShortcutConflictError；组合非法返回普通 error。
func (m *ShortcutManager) SetShortcut(action ShortcutAction, combo string) error {
	normalized, err := normalizeCombo(combo)
	if err != nil {
		return err
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if !IsValidShortcutAction(string(action)) {
		return fmt.Errorf("unknown shortcut action: %s", action)
	}
	// 冲突检测
	for other, existing := range m.combos {
		if other != action && existing == normalized {
			return &ShortcutConflictError{Action: other, Attempt: normalized}
		}
	}
	return m.registerLocked(action, normalized)
}

// ResetShortcut 恢复单个 action 的默认组合
func (m *ShortcutManager) ResetShortcut(action ShortcutAction) error {
	def := DefaultShortcuts()[string(action)]
	if def == "" {
		return fmt.Errorf("no default shortcut for action %s", action)
	}
	return m.SetShortcut(action, def)
}

// PTTCombo 解析后的 PTT 组合（供 PTTManager 释放轮询使用）。
// 返回 (mods, vk, ok)；PTT 未注册时 ok=false。
func (m *ShortcutManager) PTTCombo() (Modifier, int, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	combo, ok := m.combos[ShortcutPTT]
	if !ok {
		return 0, 0, false
	}
	mods, vk, err := parseCombo(combo)
	if err != nil {
		return 0, 0, false
	}
	return mods, vk, true
}

// UnregisterAll 注销当前注册的所有快捷键。
// 用于应用退出：避免 OS 残留"已注册"状态影响下次启动（典型表现：
// 下次 wails dev 报 "hotkey already registered"）。
//
// 实现细节：在持锁状态下原子快照所有 combos → 立即清空 map →
// 解锁后逐个调 driver.Unregister。这样：
//   - 持锁期间不调用外部代码（driver），避免回调路径死锁
//   - 即使 Unregister 慢 / 卡，map 已清空，下一次调用是 no-op
//
// 幂等：重复调用安全（第二次 map 为空，循环空跑）。
func (m *ShortcutManager) UnregisterAll() {
	m.mu.Lock()
	snapshot := make(map[ShortcutAction]string, len(m.combos))
	for k, v := range m.combos {
		snapshot[k] = v
	}
	m.combos = make(map[ShortcutAction]string)
	m.mu.Unlock()

	if len(snapshot) == 0 {
		log.Debug("shortcut manager: nothing to unregister")
		return
	}

	for action, combo := range snapshot {
		if err := m.driver.Unregister(combo); err != nil {
			log.Warnf("shortcut manager: unregister %s (%s) failed: %v", action, combo, err)
		}
	}
	log.Debugf("shortcut manager: unregistered all (%d)", len(snapshot))
}
