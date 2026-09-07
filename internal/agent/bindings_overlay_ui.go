package agent

// Overlay Edit Mode / HUD 锁定 + 窗口几何 bindings（UI 重构）。
//
// 状态模型（与 overlayVisibleState 同一模式）：
//   - Locked 是持久化配置（config.Overlay.Locked），默认 true（HUD 是安全默认）
//   - 进程内 overlayLockedState 是当前生效值，由 SetOverlayLocked /
//     SetOverlayLockedAndBroadcast（main.go 快捷键路径）维护
//   - 每次变更广播 overlayLockChanged 事件，Overlay / 主窗口前端实时跟随
//
// 几何模型：
//   - MoveOverlayWindow / ResizeOverlayWindow：Edit Mode 拖拽/缩放时的实时
//     反馈，只动窗口，不写 config
//   - CommitOverlayGeometry：拖拽/缩放结束后持久化（含 clamp），
//     0,0 位置 = 未自定义（走 Anchor 四角）
//   - ResetOverlayGeometry：清空自定义几何，回到 Anchor 四角 + 默认尺寸

import (
	"fmt"
	"sync"

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
func SetOverlayLockedAndBroadcast(locked bool) {
	setOverlayLockedState(locked)
	emitEvent("overlayLockChanged", locked)
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
// x=y=0 视为"未自定义位置"（回 Anchor 四角）——与 config 语义一致，
// 因此拖到屏幕最左上角 (0,0) 的场景会退化为 Anchor 定位（已知边界，接受）。
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

	wailsruntime.SetOverlayGeometry(cfg.Overlay.Anchor, x, y, w, h)
	emitEvent("overlayConfigChanged", nil)
	log.Debugf("CommitOverlayGeometry: committed x=%d y=%d w=%d h=%d", x, y, w, h)
	return nil
}

// ResetOverlayGeometry 清空自定义几何（回到 Anchor 四角 + 默认 preset 尺寸）。
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

	wailsruntime.SetOverlayGeometry(cfg.Overlay.Anchor, 0, 0, 0, 0)
	emitEvent("overlayConfigChanged", nil)
	log.Debug("ResetOverlayGeometry: reset to anchor preset")
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
		"anchor":      cfg.Overlay.Anchor,
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
