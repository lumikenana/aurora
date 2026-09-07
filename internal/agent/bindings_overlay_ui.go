package agent

// Overlay Edit Mode / HUD 锁定 + 窗口几何 bindings（UI 重构）。
//
// 状态模型（与 overlayVisibleState 同一模式）：
//   - Locked 是持久化配置（config.Overlay.Locked），默认 true（HUD 是安全默认）
//   - 进程内 overlayLockedState 是当前生效值，由 SetOverlayLocked /
//     SetOverlayLockedAndBroadcast（main.go 快捷键路径）维护
//   - 每次变更广播 overlayLockChanged 事件，Overlay / 主窗口前端实时跟随
//   - 锁定态切换同时驱动 native 窗口态：HUD → SetResizable(false) +
//     SetIgnoreMouseEvents(true)；Edit Mode → SetResizable(true) +
//     SetIgnoreMouseEvents(false)
//
// 几何模型（Native Window is the only Geometry Source of Truth）：
//   - 拖拽 / 缩放由 Wails v3 native 机制处理：CSS --wails-draggable: drag
//     触发 OS 拖动（drag.js）；SetResizable(true) 启用 5px 边缘 native resize
//   - OS 完成移动 / 缩放后，main.go 注册 events.Common.WindowDidMove /
//     WindowDidResize hook → SchedulePersistOverlayGeometry（300ms debounce）
//     → PersistOverlayGeometry（clamp + 写 config + Save + 广播
//     overlayConfigChanged）
//   - WindowClosing / WindowHide 前 FlushPendingOverlayGeometry
//   - 前端在 drag / resize 过程中不发任何 IPC（无 SetPosition / SetSize /
//     commit），架构上不依赖前端 bridge 的同步性
//   - MoveOverlayWindow / ResizeOverlayWindow / CommitOverlayGeometry bindings
//     仍保留（向后兼容 / 调试用），但前端 Edit Mode 不再调用

import (
	"fmt"
	"sync"
	"time"

	"github.com/lipipi/aurora/internal/config"
	"github.com/lipipi/aurora/internal/log"
	"github.com/lipipi/aurora/internal/wailsruntime"
)

// overlay 几何 clamp 边界（物理像素），防止把窗口拖到不可用大小
const (
	overlayMinWidth  = 240
	overlayMaxWidth  = 960
	overlayMinHeight = 200
	overlayMaxHeight = 900
)

// overlayLockedState：进程内 Overlay 锁定态（HUD vs Edit Mode）。
// 启动时从 config.Overlay.Locked 继承。
var (
	overlayLockedState   bool
	overlayLockedStateMu sync.RWMutex
)

func setOverlayLockedState(locked bool) {
	overlayLockedStateMu.Lock()
	overlayLockedState = locked
	overlayLockedStateMu.Unlock()
}

func getOverlayLockedState() bool {
	overlayLockedStateMu.RLock()
	defer overlayLockedStateMu.RUnlock()
	return overlayLockedState
}

// InitOverlayLockedState 启动时用 config.Overlay.Locked 初始化一次（main.go 调用）。
func InitOverlayLockedState() {
	locked := config.Global().Overlay.Locked
	setOverlayLockedState(locked)
	log.Debugf("initOverlayLockedState: initialized to cfg.Overlay.Locked=%v", locked)
}

// SetOverlayLockedAndBroadcast 更新进程内锁定态 + 广播事件（不经前端 binding
// 的路径：快捷键 / 主窗口前端通过独立 binding 调用后也走这里）。
//
// 同时驱动 native 窗口态（让 OS 行为与 UI 状态机保持一致）：
//   - locked=true（HUD）：SetResizable(false) + SetIgnoreMouseEvents(true)
//     → 不可调尺寸、鼠标穿透到下层（游戏画面）
//   - locked=false（Edit Mode）：SetResizable(true) + SetIgnoreMouseEvents(false)
//     → Wails native 边缘 resize + CSS draggable region 可交互
func SetOverlayLockedAndBroadcast(locked bool) {
	setOverlayLockedState(locked)
	emitEvent("overlayLockChanged", locked)

	// 驱动 native 窗口态。OverlayWindow 启动后才赋值（init 路径早于 window 创建，
	// wailsruntime 的 helper 内部判 nil → no-op）。
	wailsruntime.SetOverlayResizable(!locked)
	wailsruntime.SetOverlayIgnoreMouseEvents(locked)
}

// GetOverlayLockedState 读取进程内锁定态（main.go 路径）。
func GetOverlayLockedState() bool {
	return getOverlayLockedState()
}

// GetOverlayLocked 获取 Overlay 锁定态（前端初始化用）
func (b *Bindings) GetOverlayLocked() bool {
	return getOverlayLockedState()
}

// SetOverlayLocked 设置 Overlay 锁定态：
//   - 更新进程内状态 + 广播 overlayLockChanged（Overlay 实时切换 Edit/HUD）
//   - 持久化到 config.Overlay.Locked（重启后保持）
func (b *Bindings) SetOverlayLocked(locked bool) {
	log.Debugf("SetOverlayLocked: %v", locked)
	SetOverlayLockedAndBroadcast(locked)

	cfg := config.Global()
	if cfg.Overlay.Locked != locked {
		cfg.Overlay.Locked = locked
		if err := config.Save(); err != nil {
			log.Errorf("SetOverlayLocked: failed to persist: %v", err)
		}
	}
}

// MoveOverlayWindow 实时移动 overlay 窗口（Edit Mode 拖拽中，不持久化）。
func (b *Bindings) MoveOverlayWindow(x int, y int) {
	wailsruntime.MoveOverlayWindow(x, y)
}

// ResizeOverlayWindow 实时调整 overlay 窗口尺寸（Edit Mode 缩放中，不持久化）。
func (b *Bindings) ResizeOverlayWindow(w int, h int) {
	wailsruntime.ResizeOverlayWindow(w, h)
}

// CommitOverlayGeometry 提交 overlay 几何（拖拽/缩放结束后调用一次）：
// clamp → 写 config.Overlay.X/Y/Width/Height → Save → 应用到窗口 → 广播。
// x=y=0 视为"未自定义位置"（回默认位置，wailsruntime.setDefaultOverlayPosition
// 硬编码 bottom-right + 16px margin）。拖到屏幕 (0,0) 的场景会退化为默认位置
// （已知边界，接受 —— Win11 多显示器边界附近的 (0,0) 极少出现）。
func (b *Bindings) CommitOverlayGeometry(x int, y int, w int, h int) error {
	if w > overlayMaxWidth {
		w = overlayMaxWidth
	}
	if w > 0 && w < overlayMinWidth {
		w = overlayMinWidth
	}
	if h > overlayMaxHeight {
		h = overlayMaxHeight
	}
	if h > 0 && h < overlayMinHeight {
		h = overlayMinHeight
	}

	cfg := config.Global()
	cfg.Overlay.X = x
	cfg.Overlay.Y = y
	cfg.Overlay.Width = w
	cfg.Overlay.Height = h

	if err := config.Save(); err != nil {
		log.Errorf("CommitOverlayGeometry: failed to save: %v", err)
		return err
	}

	wailsruntime.SetOverlayGeometry(x, y, w, h)
	emitEvent("overlayConfigChanged", nil)
	log.Debugf("CommitOverlayGeometry: committed x=%d y=%d w=%d h=%d", x, y, w, h)
	return nil
}

// ResetOverlayGeometry 清空自定义几何（回到默认位置 bottom-right + 16px +
// 默认 preset 尺寸 360x420）。
func (b *Bindings) ResetOverlayGeometry() error {
	cfg := config.Global()
	cfg.Overlay.X = 0
	cfg.Overlay.Y = 0
	cfg.Overlay.Width = 0
	cfg.Overlay.Height = 0

	if err := config.Save(); err != nil {
		log.Errorf("ResetOverlayGeometry: failed to save: %v", err)
		return err
	}

	wailsruntime.SetOverlayGeometry(0, 0, 0, 0)
	emitEvent("overlayConfigChanged", nil)
	log.Debug("ResetOverlayGeometry: reset to default position")
	return nil
}

// GetOverlayGeometry 返回 overlay 窗口当前几何（物理像素）+ 配置态，
// 供 Overlay Edit Mode 初始化 / 主窗口 Overlay 页展示。
func (b *Bindings) GetOverlayGeometry() map[string]interface{} {
	cfg := config.Global()
	x, y, w, h := wailsruntime.GetOverlayWindowBounds()
	return map[string]interface{}{
		"x":           x,
		"y":           y,
		"width":       w,
		"height":      h,
		"config_x":    cfg.Overlay.X,
		"config_y":    cfg.Overlay.Y,
		"config_w":    cfg.Overlay.Width,
		"config_h":    cfg.Overlay.Height,
		// anchor 字段已移除（2026-09-07）。默认位置在 wailsruntime.setDefaultOverlayPosition 硬编码。
		"locked":      getOverlayLockedState(),
		"min_width":   overlayMinWidth,
		"min_height":  overlayMinHeight,
		"max_width":   overlayMaxWidth,
		"max_height":  overlayMaxHeight,
		"default_w":   360,
		"default_h":   420,
		"custom_geom": cfg.Overlay.X != 0 || cfg.Overlay.Y != 0 || cfg.Overlay.Width != 0 || cfg.Overlay.Height != 0,
	}
}

// StopSpeaking 立即停止 TTS 播放并回到空闲态（stop_speaking 快捷键 + 前端按钮）。
// 复用 VoiceController.PTTPreempt 的抢占语义（停 TTS / 停连续会话 / 状态归 Idle）。
func (b *Bindings) StopSpeaking() error {
	if voiceControllerRef == nil {
		return fmt.Errorf("voice controller not ready")
	}
	voiceControllerRef.PTTPreempt()
	log.Debug("StopSpeaking: TTS preempted")
	return nil
}

// ---- 几何持久化（Native Window → Go） ----
//
// 旧路径（前端主导）：Edit Mode 拖拽 / resize → pointermove 每像素 IPC
// → SetPosition / SetSize → pointerup 时 CommitOverlayGeometry（clamp +
// 持久化 + SetPosition 落窗 + 广播）。问题：每像素 IPC 异步导致视觉拖影
// + 落点漂移（OS 位置与 DOM 累计 delta 不同步）。
//
// 新路径（OS 主导）：拖拽 / resize 完全由 Wails v3 native 机制处理（CSS
// --wails-draggable: drag + Wails 5px 边缘 resize），OS 直接挪窗；Go 侧
// events.Common.WindowDidMove / WindowDidResize 触发后 300ms 静默窗口
// 写 config + Save + 广播 overlayConfigChanged。前端 Edit Mode 不发任何
// geometry IPC，彻底消除闪烁 / 漂移。

// overlayGeomDebounce 300ms debounce + WindowClosing/Hide 强制 flush。
const overlayGeometryDebounce = 300 * time.Millisecond

var (
	overlayGeomMu      sync.Mutex
	overlayGeomTimer   *time.Timer
	overlayGeomPending bool
	overlayGeomX       int
	overlayGeomY       int
	overlayGeomW       int
	overlayGeomH       int
)

// SchedulePersistOverlayGeometry 调度一次 300ms 后的几何持久化。多次调用
// 在静默窗口内合并为最后一次（避免拖拽 / resize 期间频繁 Save + 广播）。
//
// 唯一调用方：main.go 注册的 events.Common.WindowDidMove / WindowDidResize
// hook。OS 已完成窗口移动 / 缩放，本函数只负责把最终位置写入 config。
func SchedulePersistOverlayGeometry(x, y, w, h int) {
	overlayGeomMu.Lock()
	overlayGeomX, overlayGeomY = x, y
	overlayGeomW, overlayGeomH = w, h
	overlayGeomPending = true
	if overlayGeomTimer != nil {
		overlayGeomTimer.Stop()
	}
	overlayGeomTimer = time.AfterFunc(overlayGeometryDebounce, func() {
		overlayGeomMu.Lock()
		if !overlayGeomPending {
			overlayGeomMu.Unlock()
			return
		}
		x, y, w, h := overlayGeomX, overlayGeomY, overlayGeomW, overlayGeomH
		overlayGeomPending = false
		overlayGeomMu.Unlock()
		PersistOverlayGeometry(x, y, w, h)
	})
	overlayGeomMu.Unlock()
}

// FlushPendingOverlayGeometry 立即落盘任何待处理的几何（不等待 debounce）。
// 由 WindowClosing / WindowHide hook 调用，避免静默窗口期内关闭窗口导致
// 最后一次位置 / 尺寸丢失。
//
// 无 pending 时 no-op。返回是否实际写入了（用于日志）。
func FlushPendingOverlayGeometry() bool {
	overlayGeomMu.Lock()
	if overlayGeomTimer != nil {
		overlayGeomTimer.Stop()
		overlayGeomTimer = nil
	}
	if !overlayGeomPending {
		overlayGeomMu.Unlock()
		return false
	}
	x, y, w, h := overlayGeomX, overlayGeomY, overlayGeomW, overlayGeomH
	overlayGeomPending = false
	overlayGeomMu.Unlock()
	PersistOverlayGeometry(x, y, w, h)
	return true
}

// PersistOverlayGeometry 写入 overlay 几何到 config（含 clamp + Save +
// 广播 overlayConfigChanged）。**不调用 SetPosition / SetSize**——OS 已
// 完成窗口移动，本函数只负责落盘 + 通知前端。
//
// x=y=0 视为"未自定义"（与 CommitOverlayGeometry 同语义）。
func PersistOverlayGeometry(x, y, w, h int) {
	if w > overlayMaxWidth {
		w = overlayMaxWidth
	}
	if w > 0 && w < overlayMinWidth {
		w = overlayMinWidth
	}
	if h > overlayMaxHeight {
		h = overlayMaxHeight
	}
	if h > 0 && h < overlayMinHeight {
		h = overlayMinHeight
	}

	cfg := config.Global()
	changed := cfg.Overlay.X != x || cfg.Overlay.Y != y ||
		cfg.Overlay.Width != w || cfg.Overlay.Height != h
	cfg.Overlay.X = x
	cfg.Overlay.Y = y
	cfg.Overlay.Width = w
	cfg.Overlay.Height = h

	if !changed {
		// 位置 / 尺寸未实际变化（重复事件或初始化），跳过 Save + 广播
		return
	}

	if err := config.Save(); err != nil {
		log.Errorf("PersistOverlayGeometry: failed to save: %v", err)
		return
	}
	emitEvent("overlayConfigChanged", nil)
	log.Debugf("PersistOverlayGeometry: committed x=%d y=%d w=%d h=%d", x, y, w, h)
}
