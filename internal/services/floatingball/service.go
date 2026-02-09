package floatingball

import (
	"os"
	"strings"
	"sync"
	"time"

	"willchat/internal/define"
	"willchat/internal/services/settings"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

type DockSide string

const (
	DockNone  DockSide = ""
	DockLeft  DockSide = "left"
	DockRight DockSide = "right"
)

const (
	windowName = "floatingball"

	// UI/behavior tuning (DIP pixels)
	ballSize        = 64
	defaultMargin   = 0
	edgeSnapGap     = 24
	collapsedWidth  = 32
	collapsedVisible = 18

	snapDebounce   = 180 * time.Millisecond
	rehideDebounce = 450 * time.Millisecond
	idleDockDelay  = 5 * time.Second

	// 首次 Show 后延迟定位，避免 impl 未就绪导致 SetPosition 失效
	postShowRepositionDelay = 80 * time.Millisecond
	postShowRepositionTries = 25

	// Drag clamp tuning:
	// When users drag towards a secondary display, the native window manager may move the window
	// across displays and our "clamp back to primary" logic can cause visible flicker.
	// We rate-limit clamp operations during dragging and allow tiny overshoots.
	dragClampMinInterval = 70 * time.Millisecond
	dragClampEpsilonDip  = 10
)

// FloatingBallService 悬浮球服务（暴露给前端调用）
//
// 职责：
// - 创建/显示一个独立的悬浮球窗口（AlwaysOnTop、无边框、透明）
// - 监听 WindowDidMove：拖动到屏幕边缘后自动贴边并半隐藏
// - 鼠标移入/移出：贴边状态下展开/回缩
// - 双击：唤起主窗口
type FloatingBallService struct {
	app        *application.App
	mainWindow *application.WebviewWindow

	mu  sync.Mutex
	win *application.WebviewWindow

	visible bool
	dock    DockSide
	hovered bool
	collapsed bool
	appActive bool
	dragging bool
	dragStartX int
	dragStartY int
	dragMoved  bool
	dragEndX   int
	dragEndY   int

	// remember last position/state to avoid re-centering on every Show/SetVisible call
	hasLastState bool
	lastRelX     int
	lastRelY     int
	lastDock     DockSide
	lastCollapsed bool

	// macOS: expanding from collapsed may cause a spurious immediate "leave" during resize/move.
	// We ignore only very short leave events right after enter.
	lastHoverEnterAt         time.Time
	lastHoverEnterWasCollapsed bool

	ignoreMoveUntil time.Time
	snapTimer       *time.Timer
	rehideTimer     *time.Timer
	idleDockTimer   *time.Timer
	repositionTimer *time.Timer
	repositionTries int

	// windows: enforce size after resize requests (webview2/frameless can lag)
	sizeEnforceTimer *time.Timer
	sizeEnforceW     int
	sizeEnforceH     int
	sizeEnforceTries int
	sizeEnforceWhy   string

	// Primary display work area cache.
	// We prefer app.Screen.GetPrimary(), but on some platforms / early lifecycle this can be nil/empty.
	// Once we have a valid work area, we keep using it to enforce "primary display only" behavior.
	hasPrimaryWorkArea bool
	primaryWorkArea    application.Rect
	primaryPhysicalWorkArea application.Rect
	primaryScaleFactor      float32
	primaryWorkAreaSource   string
	loggedApproxPhysical    bool
	loggedScreenProbe       bool
}

func (s *FloatingBallService) debugEnabled() bool {
	// Enable via environment variable (preferred for local debugging):
	//   WILLCHAT_DEBUG_FLOATINGBALL=1
	// Or via settings cache:
	//   debug_floatingball=true
	//
	// Note: On macOS, launching the app from Finder/Spotlight typically won't inherit shell env vars.
	// For development builds, we default to enabled to make diagnosis easier.
	v := strings.ToLower(strings.TrimSpace(os.Getenv("WILLCHAT_DEBUG_FLOATINGBALL")))
	switch v {
	case "0", "false", "no", "n", "off":
		return false
	case "1", "true", "yes", "y", "on":
		return true
	}
	if settings.GetBool("debug_floatingball", false) {
		return true
	}
	// Default to enabled in non-production builds.
	return strings.ToLower(strings.TrimSpace(define.Env)) != "production"
}

func (s *FloatingBallService) debugLog(msg string, fields map[string]any) {
	if !s.debugEnabled() {
		return
	}
	if s.app == nil || s.app.Logger == nil {
		return
	}
	args := make([]any, 0, 2+len(fields)*2)
	args = append(args, "service", "floatingball")
	for k, v := range fields {
		args = append(args, k, v)
	}
	s.app.Logger.Info(msg, args...)
}

func dipToPhysical(dip int, scaleFactor float32) int {
	if scaleFactor <= 0 {
		return dip
	}
	// Round to nearest int to reduce drift.
	return int(float32(dip)*scaleFactor + 0.5)
}

func physicalToDip(physical int, scaleFactor float32) int {
	if scaleFactor <= 0 {
		return physical
	}
	return int(float32(physical)/scaleFactor + 0.5)
}

func normaliseWorkAreaDip(screen *application.Screen) (application.Rect, bool) {
	if screen == nil {
		return application.Rect{}, false
	}
	wa := screen.WorkArea
	if wa.Width <= 0 || wa.Height <= 0 {
		return application.Rect{}, false
	}
	sf := screen.ScaleFactor
	if sf <= 0 {
		sf = 1
	}
	// Heuristic: WorkArea should be DIP and roughly match Bounds magnitude.
	// If WorkArea looks "scaled" (e.g. doubled on retina), convert it back to DIP.
	b := screen.Bounds
	if sf > 1.1 && b.Width > 0 && b.Height > 0 {
		if wa.Width > b.Width+2 || wa.Height > b.Height+2 {
			return application.Rect{
				X:      physicalToDip(wa.X, sf),
				Y:      physicalToDip(wa.Y, sf),
				Width:  physicalToDip(wa.Width, sf),
				Height: physicalToDip(wa.Height, sf),
			}, true
		}
	}
	return wa, true
}

// safeRelativePositionLocked returns a best-effort position relative to the *primary* screen WorkArea.
// Across platforms / multi-monitor setups, coordinate spaces can vary. We normalise values into the plausible
// WorkArea-relative range to avoid false edge-snaps.
func (s *FloatingBallService) safeRelativePositionLocked() (int, int) {
	if s.win == nil {
		return 0, 0
	}
	work, ok := s.workAreaLocked()
	if !ok {
		return s.win.RelativePosition()
	}

	// Prefer native frame on macOS to avoid any unit/coordinate mismatches in Wails.
	if fr, ok2 := getNativeQuartzFrame(s.win); ok2 {
		return fr.X - work.X, fr.Y - work.Y
	}

	// Fallback: Wails DIP bounds.
	b := s.win.Bounds()
	return b.X - work.X, b.Y - work.Y
}

func NewFloatingBallService(app *application.App, mainWindow *application.WebviewWindow) *FloatingBallService {
	return &FloatingBallService{
		app:        app,
		mainWindow: mainWindow,
		visible:    true,
		dock:       DockNone,
		appActive:  true,
	}
}

// InitFromSettings 根据 settings 内存缓存初始化悬浮球显示状态
func (s *FloatingBallService) InitFromSettings() {
	visible := settings.GetBool("show_floating_window", true)
	_ = s.SetVisible(visible)
}

// IsVisible 返回悬浮球窗口是否可见
func (s *FloatingBallService) IsVisible() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.visible && s.win != nil && s.win.IsVisible()
}

// SetVisible 设置悬浮球窗口是否可见
func (s *FloatingBallService) SetVisible(visible bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.visible = visible
	if !visible {
		// 关闭时不主动创建窗口，避免“唤醒主页面”时意外弹出悬浮球
		if s.win == nil {
			s.stopTimersLocked()
			s.dock = DockNone
			s.hovered = false
			s.collapsed = false
			return nil
		}
		s.stopTimersLocked()
		// remember current state (if window exists)
		if s.win != nil {
			x, y := s.safeRelativePositionLocked()
			s.hasLastState = true
			s.lastRelX, s.lastRelY = x, y
			s.lastDock = s.dock
			s.lastCollapsed = s.collapsed
		}
		s.win.Hide()
		s.dock = DockNone
		s.hovered = false
		s.collapsed = false
		s.dragging = false
		s.dragMoved = false
		return nil
	}

	win := s.ensureLocked()
	if win == nil {
		return nil
	}

	s.stopTimersLocked()
	s.hovered = false
	s.dragging = false
	s.dragMoved = false
	// do NOT reset dock/collapsed on non-initial show; preserve last state if available
	if !s.hasLastState {
		s.dock = DockNone
		s.collapsed = false
	} else {
		s.dock = s.lastDock
		s.collapsed = s.lastCollapsed
	}

	win.Show()
	// 首次显示时，impl 可能还没 ready；用重试机制确保定位最终生效
	s.scheduleRepositionLocked()
	// 不抢占用户焦点：初始化/切换开启仅显示，不主动 Focus()
	s.scheduleIdleDockLocked()
	return nil
}

// Hover 通知后端鼠标是否移入悬浮球（用于贴边展开/回缩）
func (s *FloatingBallService) Hover(entered bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.win == nil {
		return
	}

	now := time.Now()
	enterAgeMs := int64(-1)
	if !s.lastHoverEnterAt.IsZero() {
		enterAgeMs = now.Sub(s.lastHoverEnterAt).Milliseconds()
	}
	s.debugLog("Hover", map[string]any{
		"entered": entered,
		"dragging": s.dragging,
		"dock": s.dock,
		"collapsed": s.collapsed,
		"appActive": s.appActive,
		"visible": s.visible,
		"enterAgeMs": enterAgeMs,
		"enterWasCollapsed": s.lastHoverEnterWasCollapsed,
	})

	// Cancel any pending idle dock
	if s.idleDockTimer != nil {
		s.idleDockTimer.Stop()
		s.idleDockTimer = nil
	}

	// Cancel any pending re-hide
	if s.rehideTimer != nil {
		s.rehideTimer.Stop()
		s.rehideTimer = nil
	}

	// Ignore very short leave right after enter *only if* we expanded from collapsed,
	// otherwise users won't be able to move away quickly to re-hide.
	if !entered && s.lastHoverEnterWasCollapsed && !s.lastHoverEnterAt.IsZero() && now.Sub(s.lastHoverEnterAt) <= 250*time.Millisecond {
		s.debugLog("Hover:ignore_leave", map[string]any{
			"enterAgeMs": now.Sub(s.lastHoverEnterAt).Milliseconds(),
		})
		s.hovered = true
		return
	}

	s.hovered = entered

	// Dragging: ignore hover enter/leave side effects, otherwise mouseleave during drag
	// may schedule a re-hide that teleports the window back to the docked edge.
	if s.dragging {
		s.debugLog("Hover:skip_dragging", map[string]any{})
		return
	}

	if entered {
		s.lastHoverEnterAt = now
		s.lastHoverEnterWasCollapsed = s.collapsed
		s.expandLocked()
		return
	}

	// Mouse left: if not docked yet, wait idleDockDelay then dock+shrink
	if s.dock == DockNone {
		s.scheduleIdleDockLocked()
		return
	}

	// Only auto re-hide when currently docked
	s.rehideTimer = time.AfterFunc(rehideDebounce, func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		s.rehideLocked()
	})
}

// SetDragging 通知后端当前是否处于拖拽中。
// 拖拽中不自动贴边/缩小，避免“需要重复多次移动才会移动到屏幕外/贴边行为打断拖拽”。
func (s *FloatingBallService) SetDragging(dragging bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	prev := s.dragging
	s.dragging = dragging
	// Idempotent: ignore redundant calls (e.g. blur/visibility handlers calling SetDragging(false) again)
	if prev == dragging {
		s.debugLog("SetDragging:noop", map[string]any{"prev": prev, "now": dragging})
		return
	}
	if s.win == nil || !s.visible {
		s.debugLog("SetDragging(no_window)", map[string]any{
			"prev": prev, "now": dragging, "visible": s.visible,
		})
		return
	}

	relX, relY := s.safeRelativePositionLocked()
	b := s.win.Bounds()
	s.debugLog("SetDragging", map[string]any{
		"prev": prev, "now": dragging,
		"dock": s.dock, "collapsed": s.collapsed,
		"relX": relX, "relY": relY,
		"w": b.Width, "h": b.Height,
	})

	if dragging {
		s.dragEndX, s.dragEndY = 0, 0
		// 记录拖拽起点，用于区分“点击”和“真实拖动”
		s.dragStartX, s.dragStartY = relX, relY
		s.dragMoved = false
		// 拖拽中取消自动贴边/缩小相关计时器
		if s.snapTimer != nil {
			s.snapTimer.Stop()
			s.snapTimer = nil
		}
		if s.idleDockTimer != nil {
			s.idleDockTimer.Stop()
			s.idleDockTimer = nil
		}
		if s.rehideTimer != nil {
			s.rehideTimer.Stop()
			s.rehideTimer = nil
		}
		return
	}

	// 拖拽结束：防止之前的 mouseleave 触发 rehide 把窗口拉回边缘
	if s.rehideTimer != nil {
		s.rehideTimer.Stop()
		s.rehideTimer = nil
	}

	// 拖拽结束：用“起点 vs 当前点”的位置差来判定是否真实拖动，
	// 避免由于 ignoreMoveUntil/平台事件顺序导致 dragMoved 没被置位，从而跳过 snap，
	// 进而 dock 状态残留（会让窗口被误判为仍贴边，最终强制贴回边缘）。
	relX2, relY2 := s.safeRelativePositionLocked()
	s.dragEndX, s.dragEndY = relX2, relY2
	if abs(relX2-s.dragStartX) <= 2 && abs(relY2-s.dragStartY) <= 2 {
		s.dragMoved = false
		s.debugLog("drag_end_snap:skip_no_move", map[string]any{
			"startX": s.dragStartX, "startY": s.dragStartY,
			"endX": relX2, "endY": relY2,
		})
		return
	}
	s.dragMoved = true
	// Real drag: immediately detach from any previous dock to avoid flicker.
	// Otherwise, during the 60ms delay, other logic (blur/rehide) may see dock=right and
	// collapse/teleport it to the edge, and then snap will move it back ("flash").
	s.dock = DockNone

	// 拖拽结束：稍作延迟等待系统最终位置稳定，然后立刻判断贴边/对齐（不在这里缩小）
	time.AfterFunc(60*time.Millisecond, func() {
		s.dragEndSnap()
	})
}

func (s *FloatingBallService) dragEndSnap() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.win == nil || !s.visible {
		s.debugLog("drag_end_snap:skip", map[string]any{"visible": s.visible, "hasWin": s.win != nil})
		return
	}
	s.debugLog("drag_end_snap:run", map[string]any{})
	if s.dragEndX != 0 || s.dragEndY != 0 {
		s.snapAfterMoveAtLocked(s.dragEndX, s.dragEndY)
		// If app is inactive and we ended up docked, collapse immediately (no flicker).
		if !s.appActive && s.dock != DockNone {
			work, ok := s.workAreaLocked()
			if ok {
				y := clamp(s.dragEndY, 0, work.Height-ballSize)
				s.collapseToYLocked(y)
			} else {
				s.collapseToYLocked(s.dragEndY)
			}
		}
		return
	}
	s.snapAfterMoveLocked()
}

// SetAppActive 通知后端应用是否处于激活状态（用于失焦时自动缩小贴边）
func (s *FloatingBallService) SetAppActive(active bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.appActive = active
	if s.win == nil || !s.visible {
		return
	}

	// 失去焦点：如果已经贴边，则立即缩小为把手，且关闭所有待执行的展开/回缩
	if !active {
		// Dragging: do not collapse during/around drag end, otherwise it may look like
		// the window "teleports" back to the docked edge.
		if s.dragging {
			return
		}
		if s.rehideTimer != nil {
			s.rehideTimer.Stop()
			s.rehideTimer = nil
		}
		if s.idleDockTimer != nil {
			s.idleDockTimer.Stop()
			s.idleDockTimer = nil
		}
		if s.dock != DockNone {
			_, relY := s.safeRelativePositionLocked()
			s.collapseToYLocked(relY)
		}
	}
}

// CloseFromUI 前端点击关闭按钮
func (s *FloatingBallService) CloseFromUI() {
	_ = s.SetVisible(false)
}

// OpenMainFromUI 前端双击悬浮球，唤起主窗口
func (s *FloatingBallService) OpenMainFromUI() {
	if s.mainWindow == nil {
		return
	}
	s.mainWindow.UnMinimise()
	s.mainWindow.Show()
	s.mainWindow.Focus()
}

func (s *FloatingBallService) ensureLocked() *application.WebviewWindow {
	if s.app == nil {
		return nil
	}
	if s.win != nil {
		return s.win
	}

	// 创建时就设置为屏幕最右侧贴边 + 垂直居中（避免首次显示跑到默认位置）
	relX, relY := s.defaultPositionLocked()
	x, y := relX, relY
	if work, ok := s.workAreaLocked(); ok {
		x = work.X + relX
		y = work.Y + relY
	}
	s.debugLog("floatingball:create:init_pos", map[string]any{
		"relX": relX, "relY": relY,
		"absX": x, "absY": y,
		"workArea": s.primaryWorkArea,
		"workSource": s.primaryWorkAreaSource,
	})

	w := s.app.Window.NewWithOptions(application.WebviewWindowOptions{
		Name:          windowName,
		Title:         "WillChat",
		Width:         ballSize,
		Height:        ballSize,
		MinWidth:      collapsedWidth,
		MaxWidth:      ballSize,
		MinHeight:     ballSize,
		MaxHeight:     ballSize,
		InitialPosition: application.WindowXY,
		X:               x,
		Y:               y,
		DisableResize: true,
		Frameless:     true,
		AlwaysOnTop:   true,
		Hidden:        true,
		URL:           "/floatingball.html",

		BackgroundType: floatingBallBackgroundType(),
		// 鼠标事件必须保留，否则无法交互
		IgnoreMouseEvents: false,

		Windows: application.WindowsWindow{
			// NOTE: Don't use WindowMask on Windows. Wails implements WindowMask via UpdateLayeredWindow,
			// which can visually separate the layered bitmap from the embedded WebView2 surface
			// (white circle / logo / close button appear as "split windows").
			HiddenOnTaskbar: true,
			// Avoid extra shadow/rounded-corner decorations in frameless mode.
			DisableFramelessWindowDecorations: true,
			// When using translucent background, prefer no-backdrop to emulate transparency.
			BackdropType: application.None,
		},
		Mac: application.MacWindow{
			Backdrop:     application.MacBackdropTransparent,
			DisableShadow: true,
			WindowLevel:  application.MacWindowLevelFloating,
			// 不依赖 titlebar drag，前端使用 --wails-draggable
			InvisibleTitleBarHeight: 0,
		},
		Linux: application.LinuxWindow{
			WindowIsTranslucent: true,
		},
	})

	// 监听移动事件（拖拽贴边隐藏）
	w.RegisterHook(events.Common.WindowDidMove, func(_ *application.WindowEvent) {
		s.onWindowDidMove()
	})
	// 显示后再次兜底定位（部分平台首次 SetPosition 可能被忽略）
	w.RegisterHook(events.Common.WindowShow, func(_ *application.WindowEvent) {
		s.mu.Lock()
		defer s.mu.Unlock()
		if s.win == nil || !s.visible {
			return
		}
		// macOS: ensure hover works even when window is non-activating
		enableMacHoverTracking(s.win)
		// windows: ensure true frameless (WS_POPUP) so small 64x64 sizing works
		enableWindowsPopupStyle(s.win, s)
		s.scheduleRepositionLocked()

		// Post-show verification: on some systems the window manager may adjust the window frame
		// asynchronously after Show(). We verify and clamp once after a short delay.
		time.AfterFunc(220*time.Millisecond, func() {
			s.mu.Lock()
			defer s.mu.Unlock()
			if s.win == nil || !s.visible {
				return
			}
			s.debugLog("floatingball:show:after", map[string]any{
				"bounds": s.win.Bounds(),
				"dock": s.dock,
				"collapsed": s.collapsed,
				"workArea": s.primaryWorkArea,
				"workSource": s.primaryWorkAreaSource,
			})
			// If it somehow ended up off-primary, clamp it back.
			_, _, _ = s.clampToPrimaryDipLocked("show_after")
		})
	})

	s.win = w
	return s.win
}

func (s *FloatingBallService) onWindowDidMove() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.win == nil {
		return
	}
	if !s.visible {
		return
	}

	// ignoreMoveUntil 用于屏蔽“代码主动 SetPosition/SetSize”导致的 move。
	// IMPORTANT: do not clamp/snap during this window, otherwise edge-hide/collapse moves may get overridden.
	if !s.dragging && time.Now().Before(s.ignoreMoveUntil) {
		return
	}

	// Always enforce "primary display only" on any move event.
	// This covers cases where native dragging occurs without frontend calling SetDragging(true).
	if !s.dragging {
		if clamped, relX, relY := s.clampToPrimaryDipLocked("move"); clamped {
			// After clamping, immediately re-run snap logic (DIP) so dock state stays consistent.
			s.snapAfterMoveAtLocked(relX, relY)
			return
		}
	}
	// 拖拽中不自动贴边/缩小
	if s.dragging {
		// Rate-limit clamp while dragging to reduce flicker.
		// Still record movement detection below.
		if time.Now().Before(s.ignoreMoveUntil) {
			relX, relY := s.safeRelativePositionLocked()
			if abs(relX-s.dragStartX) > 2 || abs(relY-s.dragStartY) > 2 {
				s.dragMoved = true
			}
			s.debugLog("WindowDidMove:skip_dragging", map[string]any{})
			return
		}

		// Hard constraint: keep the floating ball on the primary display only.
		// We allow the small "half-hidden" offset when collapsed+docked.
		_, _, _ = s.clampToPrimaryDipLocked("drag")

		// 记录是否发生有效移动（阈值 2px）
		relX, relY := s.safeRelativePositionLocked()
		if abs(relX-s.dragStartX) > 2 || abs(relY-s.dragStartY) > 2 {
			s.dragMoved = true
		}
		s.debugLog("WindowDidMove:skip_dragging", map[string]any{})
		return
	}
	// ignoreMoveUntil 用于屏蔽“代码主动 SetPosition/SetSize”导致的 move
	if time.Now().Before(s.ignoreMoveUntil) {
		return
	}

	if s.snapTimer != nil {
		s.snapTimer.Stop()
		s.snapTimer = nil
	}
	s.snapTimer = time.AfterFunc(snapDebounce, func() {
		s.snapAfterMove()
	})
}

// clampToPrimaryDipLocked clamps the window into the primary WorkArea (DIP).
// Returns whether a clamp was applied, plus the resulting (primary-workarea-relative) DIP coords.
func (s *FloatingBallService) clampToPrimaryDipLocked(reason string) (bool, int, int) {
	if s.win == nil || !s.visible {
		return false, 0, 0
	}
	work, ok := s.workAreaLocked()
	if !ok {
		return false, 0, 0
	}

	// Current absolute position (Quartz-like coords): prefer native frame on macOS.
	b := s.win.Bounds()
	absX, absY := b.X, b.Y
	if fr, ok2 := getNativeQuartzFrame(s.win); ok2 {
		absX, absY = fr.X, fr.Y
		// Use native width/height as well (can differ during resize/collapse).
		b.Width, b.Height = fr.Width, fr.Height
	}
	minX := work.X
	maxX := work.X + work.Width - b.Width
	if s.collapsed && s.dock == DockLeft {
		minX = work.X - (b.Width - collapsedVisible)
	}
	if s.collapsed && s.dock == DockRight {
		maxX = work.X + work.Width - collapsedVisible
	}
	minY := work.Y
	maxY := work.Y + work.Height - b.Height

	cx := clamp(absX, minX, maxX)
	cy := clamp(absY, minY, maxY)
	relXDip := cx - work.X
	relYDip := cy - work.Y
	if cx == absX && cy == absY {
		return false, relXDip, relYDip
	}

	// During dragging, allow tiny overshoots to avoid jitter at the boundary.
	if reason == "drag" {
		dx := abs(absX - cx)
		dy := abs(absY - cy)
		if dx <= dragClampEpsilonDip && dy <= dragClampEpsilonDip {
			return false, relXDip, relYDip
		}
	}

	s.debugLog("floatingball:clamp_primary_dip", map[string]any{
		"reason":    reason,
		"source":    s.primaryWorkAreaSource,
		"dock":      s.dock,
		"collapsed": s.collapsed,
		"work":      work,
		"bounds":    b,
		"minX":      minX, "maxX": maxX, "minY": minY, "maxY": maxY,
		"fromX":     absX, "fromY": absY,
		"toX":       cx, "toY": cy,
		"relXDip":   relXDip, "relYDip": relYDip,
	})

	// Apply an ignore window after we move the window in code.
	// For dragging, use a shorter window to reduce clamp frequency (less flicker).
	if reason == "drag" {
		s.ignoreMoveUntil = time.Now().Add(dragClampMinInterval)
	} else {
		s.ignoreMoveUntil = time.Now().Add(250 * time.Millisecond)
	}
	if setNativeQuartzFrame(s.win, cx, cy, b.Width, b.Height) {
		s.debugLog("floatingball:clamp_primary_dip:native", map[string]any{
			"reason": reason,
			"toX": cx, "toY": cy, "w": b.Width, "h": b.Height,
		})
	} else {
		s.win.SetBounds(application.Rect{X: cx, Y: cy, Width: b.Width, Height: b.Height})
	}
	s.debugLog("floatingball:clamp_primary_dip:after", map[string]any{
		"afterBounds": s.win.Bounds(),
	})
	return true, relXDip, relYDip
}

func (s *FloatingBallService) snapAfterMove() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.snapAfterMoveLocked()
}

func (s *FloatingBallService) snapAfterMoveLocked() {
	if s.win == nil || !s.visible {
		return
	}

	relX, relY := s.safeRelativePositionLocked()
	s.snapAfterMoveAtLocked(relX, relY)
}

func (s *FloatingBallService) snapAfterMoveAtLocked(relX, relY int) {
	if s.win == nil || !s.visible {
		return
	}
	bounds := s.win.Bounds()
	width := bounds.Width
	height := bounds.Height

	work, ok := s.workAreaLocked()
	if !ok {
		return
	}

	// Clamp Y into work area first (relative)
	y := clamp(relY, 0, work.Height-height)

	// Snap + collapse if near left/right edges (relative)
	if relX <= edgeSnapGap {
		s.dock = DockLeft
		s.debugLog("snap:DockLeft", map[string]any{"relX": relX, "edgeSnapGap": edgeSnapGap})
		// 仅贴边对齐（保持完整大小）；缩小交给失焦/鼠标移出/idle 逻辑
		s.expandToYLocked(y)
		s.scheduleIdleDockLocked()
		return
	}
	if relX+width >= work.Width-edgeSnapGap {
		s.dock = DockRight
		s.debugLog("snap:DockRight", map[string]any{"relX": relX, "width": width, "workW": work.Width, "edgeSnapGap": edgeSnapGap})
		// 仅贴边对齐（保持完整大小）；缩小交给失焦/鼠标移出/idle 逻辑
		s.expandToYLocked(y)
		s.scheduleIdleDockLocked()
		return
	}

	// Not docked: keep within work area and clear dock state
	s.dock = DockNone
	if s.collapsed {
		s.debugLog("snap:undock_expand", map[string]any{"relX": relX, "relY": relY})
		s.expandToYLocked(y)
		return
	}
	x := clamp(relX, 0, work.Width-width)
	s.debugLog("snap:none", map[string]any{"x": x, "y": y, "relX": relX, "relY": relY})
	s.setRelativePositionLocked(x, y)

	// 移动结束后，若鼠标未 hover，超过一段时间自动贴边缩小
	s.scheduleIdleDockLocked()
}

func (s *FloatingBallService) resetToDefaultPositionLocked() {
	if s.win == nil || s.app == nil {
		return
	}

	x, y := s.defaultPositionLocked()
	s.debugLog("floatingball:reset:default", map[string]any{
		"relX": x, "relY": y,
		"workArea": s.primaryWorkArea,
		"workSource": s.primaryWorkAreaSource,
	})
	s.dock = DockNone
	s.collapsed = false
	s.setSizeLocked(ballSize, ballSize)
	s.setRelativePositionLocked(x, y)
}

func (s *FloatingBallService) defaultPositionLocked() (int, int) {
	work, ok := s.workAreaLocked()
	if !ok {
		return 0, 0
	}
	// relative to WorkArea (0,0)
	x := work.Width - ballSize - defaultMargin // 贴右边（默认无边距）
	y := (work.Height - ballSize) / 2
	return x, y
}

func (s *FloatingBallService) scheduleRepositionLocked() {
	if s.win == nil || !s.visible {
		return
	}
	// cancel previous
	if s.repositionTimer != nil {
		s.repositionTimer.Stop()
		s.repositionTimer = nil
	}
	s.repositionTries = 0
	s.repositionTimer = time.AfterFunc(postShowRepositionDelay, func() {
		s.repositionTick()
	})
}

func (s *FloatingBallService) repositionTick() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.win == nil || !s.visible {
		return
	}
	s.repositionTries++

	// Wait until we have a usable WorkArea, otherwise default positioning
	// may fall back to (0,0) and "stick" at top-left on some machines.
	if _, ok := s.workAreaLocked(); ok {
		s.restoreOrDefaultLocked()
		return
	}
	if s.repositionTries >= postShowRepositionTries {
		// 最后兜底：即使拿不到 screen，也尝试设置一次位置
		s.restoreOrDefaultLocked()
		return
	}

	// retry
	s.repositionTimer = time.AfterFunc(postShowRepositionDelay, func() {
		s.repositionTick()
	})
}

func (s *FloatingBallService) restoreOrDefaultLocked() {
	if s.win == nil {
		return
	}
	// If we have a last known state, restore it; otherwise use default.
	if s.hasLastState {
		s.debugLog("restore:last_state", map[string]any{
			"x": s.lastRelX, "y": s.lastRelY, "dock": s.lastDock, "collapsed": s.lastCollapsed,
		})
		s.dock = s.lastDock
		s.collapsed = s.lastCollapsed
		if s.collapsed {
			s.setSizeLocked(collapsedWidth, ballSize)
		} else {
			s.setSizeLocked(ballSize, ballSize)
		}
		s.setRelativePositionLocked(s.lastRelX, s.lastRelY)
		return
	}
	s.resetToDefaultPositionLocked()
}

func (s *FloatingBallService) expandLocked() {
	if s.win == nil || s.dock == DockNone {
		return
	}

	work, ok := s.workAreaLocked()
	if !ok {
		return
	}
	_, relY := s.safeRelativePositionLocked()
	bounds := s.win.Bounds()
	y := clamp(relY, 0, work.Height-bounds.Height)

	s.expandToYLocked(y)
}

func (s *FloatingBallService) rehideLocked() {
	if s.win == nil || s.dock == DockNone {
		return
	}

	work, ok := s.workAreaLocked()
	if !ok {
		return
	}
	_, relY := s.safeRelativePositionLocked()
	bounds := s.win.Bounds()
	y := clamp(relY, 0, work.Height-bounds.Height)

	s.collapseToYLocked(y)
}

func (s *FloatingBallService) scheduleIdleDockLocked() {
	if s.win == nil || !s.visible {
		return
	}
	// 未 hover 时生效（无论是否已贴边），用于“停留一段时间后自动缩小”
	if s.hovered {
		return
	}
	if s.collapsed {
		return
	}

	if s.idleDockTimer != nil {
		s.idleDockTimer.Stop()
		s.idleDockTimer = nil
	}
	s.idleDockTimer = time.AfterFunc(idleDockDelay, func() {
		s.mu.Lock()
		defer s.mu.Unlock()

		if s.win == nil || !s.visible {
			return
		}
		if s.hovered || s.collapsed {
			return
		}
		if !s.win.IsVisible() {
			return
		}

		// 自动缩小：若已贴边则直接缩小；若未贴边则仅在靠近边缘时贴边并缩小
		work, ok := s.workAreaLocked()
		if !ok {
			return
		}
		relX, relY := s.safeRelativePositionLocked()
		b := s.win.Bounds()
		width := b.Width
		height := b.Height
		y := clamp(relY, 0, work.Height-height)

		if s.dock == DockLeft || s.dock == DockRight {
			s.rehideLocked()
			return
		}
		// decide side by proximity
		if relX <= edgeSnapGap {
			s.dock = DockLeft
			s.collapseToYLocked(y)
			return
		}
		if relX+width >= work.Width-edgeSnapGap {
			s.dock = DockRight
			s.collapseToYLocked(y)
			return
		}
	})
}

func (s *FloatingBallService) stopTimersLocked() {
	if s.snapTimer != nil {
		s.snapTimer.Stop()
		s.snapTimer = nil
	}
	if s.rehideTimer != nil {
		s.rehideTimer.Stop()
		s.rehideTimer = nil
	}
	if s.idleDockTimer != nil {
		s.idleDockTimer.Stop()
		s.idleDockTimer = nil
	}
	if s.repositionTimer != nil {
		s.repositionTimer.Stop()
		s.repositionTimer = nil
	}
	if s.sizeEnforceTimer != nil {
		s.sizeEnforceTimer.Stop()
		s.sizeEnforceTimer = nil
	}
}

func (s *FloatingBallService) setPositionLocked(x, y int) {
	if s.win == nil {
		return
	}
	s.ignoreMoveUntil = time.Now().Add(250 * time.Millisecond)
	s.win.SetPosition(x, y)
}

func (s *FloatingBallService) setPhysicalBoundsLocked(x, y, w, h int) {
	if s.win == nil {
		return
	}
	s.ignoreMoveUntil = time.Now().Add(250 * time.Millisecond)
	s.debugLog("floatingball:setPhysicalBounds", map[string]any{
		"x": x, "y": y, "w": w, "h": h,
	})
	s.win.SetPhysicalBounds(application.Rect{X: x, Y: y, Width: w, Height: h})
}

func (s *FloatingBallService) setRelativePositionLocked(x, y int) {
	if s.win == nil {
		return
	}
	s.ignoreMoveUntil = time.Now().Add(250 * time.Millisecond)
	work, ok := s.workAreaLocked()
	if !ok {
		s.win.SetRelativePosition(x, y)
		return
	}

	b := s.win.Bounds()
	absX := work.X + x
	absY := work.Y + y
	s.debugLog("floatingball:setRelativePosition", map[string]any{
		"source":  s.primaryWorkAreaSource,
		"relDipX": x, "relDipY": y,
		"work":    work,
		"bounds":  b,
		"toX":     absX, "toY": absY,
	})
	if setNativeQuartzFrame(s.win, absX, absY, b.Width, b.Height) {
		s.debugLog("floatingball:setRelativePosition:native", map[string]any{
			"toX": absX, "toY": absY, "w": b.Width, "h": b.Height,
		})
	} else {
		s.win.SetBounds(application.Rect{X: absX, Y: absY, Width: b.Width, Height: b.Height})
	}

	// Post-apply trace to detect coordinate-space mismatches.
	after := s.win.Bounds()
	s.debugLog("floatingball:setRelativePosition:after", map[string]any{
		"afterBounds": after,
	})
}

func (s *FloatingBallService) setSizeLocked(width, height int) {
	if s.win == nil {
		return
	}
	s.ignoreMoveUntil = time.Now().Add(250 * time.Millisecond)
	s.win.SetSize(width, height)
	s.requestSizeEnforceLocked(width, height, "setSize")
}

func (s *FloatingBallService) requestSizeEnforceLocked(w, h int, why string) {
	if !isWindows() || s.win == nil || !s.visible {
		return
	}
	if s.sizeEnforceTimer != nil {
		s.sizeEnforceTimer.Stop()
		s.sizeEnforceTimer = nil
	}
	s.sizeEnforceW = w
	s.sizeEnforceH = h
	s.sizeEnforceTries = 0
	s.sizeEnforceWhy = why
	s.sizeEnforceTimer = time.AfterFunc(80*time.Millisecond, func() {
		s.sizeEnforceTick()
	})
}

func (s *FloatingBallService) sizeEnforceTick() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !isWindows() || s.win == nil || !s.visible {
		return
	}
	s.sizeEnforceTries++
	wantW, wantH := s.sizeEnforceW, s.sizeEnforceH
	got := s.win.Bounds()
	if abs(got.Width-wantW) <= 1 && abs(got.Height-wantH) <= 1 {
		return
	}
	s.debugLog("size:enforce", map[string]any{
		"why": s.sizeEnforceWhy,
		"try": s.sizeEnforceTries,
		"wantW": wantW, "wantH": wantH,
		"gotW": got.Width, "gotH": got.Height,
	})
	s.ignoreMoveUntil = time.Now().Add(250 * time.Millisecond)
	s.win.SetSize(wantW, wantH)

	// Re-apply docked positioning using desired size.
	work, ok := s.workAreaLocked()
	if ok {
		_, relY := s.safeRelativePositionLocked()
		y := clamp(relY, 0, work.Height-wantH)
		x := 0
		switch s.dock {
		case DockLeft:
			if s.collapsed {
				x = -(wantW - collapsedVisible)
			} else {
				x = 0
			}
		case DockRight:
			if s.collapsed {
				x = work.Width - collapsedVisible
			} else {
				x = work.Width - wantW
			}
		}
		s.setRelativePositionLocked(x, y)
	}

	if s.sizeEnforceTries < 5 {
		s.sizeEnforceTimer = time.AfterFunc(120*time.Millisecond, func() {
			s.sizeEnforceTick()
		})
	}
}

func (s *FloatingBallService) expandToYLocked(y int) {
	if s.win == nil {
		return
	}
	work, ok := s.workAreaLocked()
	if !ok {
		return
	}
	s.collapsed = false
	desiredW, desiredH := ballSize, ballSize
	s.setSizeLocked(desiredW, desiredH)
	b := s.win.Bounds()

	y = clamp(y, 0, work.Height-desiredH)
	x := 0
	switch s.dock {
	case DockLeft:
		x = 0
	case DockRight:
		x = work.Width - desiredW
	}
	s.debugLog("expand", map[string]any{
		"dock": s.dock, "x": x, "y": y,
		"wantW": desiredW, "wantH": desiredH,
		"boundsW": b.Width, "boundsH": b.Height,
	})
	s.setRelativePositionLocked(x, y)
}

func (s *FloatingBallService) collapseToYLocked(y int) {
	if s.win == nil {
		return
	}
	work, ok := s.workAreaLocked()
	if !ok {
		return
	}
	s.collapsed = true
	desiredW, desiredH := collapsedWidth, ballSize
	s.setSizeLocked(desiredW, desiredH)
	b := s.win.Bounds()

	y = clamp(y, 0, work.Height-desiredH)
	x := 0
	switch s.dock {
	case DockLeft:
		x = -(desiredW - collapsedVisible)
	case DockRight:
		x = work.Width - collapsedVisible
	}
	s.debugLog("collapse", map[string]any{
		"dock": s.dock, "x": x, "y": y,
		"wantW": desiredW, "wantH": desiredH,
		"boundsW": b.Width, "boundsH": b.Height,
	})
	s.setRelativePositionLocked(x, y)
}

func clamp(v, min, max int) int {
	if max < min {
		return min
	}
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func (s *FloatingBallService) workAreaLocked() (application.Rect, bool) {
	// Product requirement: floating ball is only allowed on the primary display.
	//
	// We cache the primary work area once we can obtain it. This avoids two problems:
	// - Some platforms may temporarily return nil/empty primary screen info at startup.
	// - We must not switch the "reference work area" when the window moves across monitors.
	// If we already have a cached native primary work area, reuse it (stable and avoids log spam).
	if s.hasPrimaryWorkArea && s.primaryWorkAreaSource == "native_primary" &&
		s.primaryWorkArea.Width > 0 && s.primaryWorkArea.Height > 0 {
		return s.primaryWorkArea, true
	}

	// 0) Best-effort native primary work area (macOS).
	// This avoids relying on app.Screen which may be uninitialised (GetAll empty) on some setups.
	if wa, sf, ok := primaryWorkAreaNative(); ok {
		s.primaryWorkArea = wa
		s.primaryScaleFactor = sf
		s.primaryWorkAreaSource = "native_primary"
		s.hasPrimaryWorkArea = true
		s.debugLog("floatingball:workarea:cache", map[string]any{
			"source":      s.primaryWorkAreaSource,
			"workArea":    s.primaryWorkArea,
			"scaleFactor": s.primaryScaleFactor,
		})
		return s.primaryWorkArea, true
	}

	// 1) Preferred: app primary screen. If it becomes available later, override any fallback cache.
	if s.app != nil && s.app.Screen != nil {
		primary := s.app.Screen.GetPrimary()
		// Try GetPrimary() first.
		if primary != nil {
			if wa, ok := normaliseWorkAreaDip(primary); ok {
				s.primaryWorkArea = wa
				s.primaryPhysicalWorkArea = primary.PhysicalWorkArea
				s.primaryScaleFactor = primary.ScaleFactor
				s.primaryWorkAreaSource = "app_primary"
				s.hasPrimaryWorkArea = true
				s.debugLog("floatingball:workarea:cache", map[string]any{
					"source":           s.primaryWorkAreaSource,
					"workArea":         s.primaryWorkArea,
					"bounds":           primary.Bounds,
					"physicalWorkArea": s.primaryPhysicalWorkArea,
					"scaleFactor":      s.primaryScaleFactor,
					"isPrimary":        primary.IsPrimary,
				})
				return s.primaryWorkArea, true
			}
		}

		// Fallback: pick the screen nearest to DIP origin (0,0). In most layouts, this maps to the primary display.
		if sc := s.app.Screen.ScreenNearestDipPoint(application.Point{X: 0, Y: 0}); sc != nil {
			if wa, ok := normaliseWorkAreaDip(sc); ok {
				s.primaryWorkArea = wa
				s.primaryPhysicalWorkArea = sc.PhysicalWorkArea
				s.primaryScaleFactor = sc.ScaleFactor
				s.primaryWorkAreaSource = "app_nearest_origin"
				s.hasPrimaryWorkArea = true
				s.debugLog("floatingball:workarea:cache", map[string]any{
					"source":           s.primaryWorkAreaSource,
					"workArea":         s.primaryWorkArea,
					"bounds":           sc.Bounds,
					"physicalWorkArea": s.primaryPhysicalWorkArea,
					"scaleFactor":      s.primaryScaleFactor,
					"isPrimary":        sc.IsPrimary,
					"screenName":       sc.Name,
					"screenID":         sc.ID,
				})
				return s.primaryWorkArea, true
			}
		}

		// Fallback within ScreenManager: scan all screens and pick IsPrimary.
		// This helps when GetPrimary() is temporarily nil/empty early in lifecycle.
		screens := s.app.Screen.GetAll()
		if len(screens) > 0 {
			// 1) Prefer explicit IsPrimary.
			for _, sc := range screens {
				if sc == nil || !sc.IsPrimary {
					continue
				}
				if wa, ok := normaliseWorkAreaDip(sc); ok {
					s.primaryWorkArea = wa
					s.primaryPhysicalWorkArea = sc.PhysicalWorkArea
					s.primaryScaleFactor = sc.ScaleFactor
					s.primaryWorkAreaSource = "app_primary_scan"
					s.hasPrimaryWorkArea = true
					s.debugLog("floatingball:workarea:cache", map[string]any{
						"source":           s.primaryWorkAreaSource,
						"workArea":         s.primaryWorkArea,
						"bounds":           sc.Bounds,
						"physicalWorkArea": s.primaryPhysicalWorkArea,
						"scaleFactor":      s.primaryScaleFactor,
						"isPrimary":        sc.IsPrimary,
						"screenName":       sc.Name,
						"screenID":         sc.ID,
					})
					return s.primaryWorkArea, true
				}
			}

			// 2) Heuristic fallback: pick the screen whose Bounds origin is closest to (0,0),
			// and prefer larger WorkArea if tie. This matches macOS typical coordinate layout.
			var best *application.Screen
			bestScore := int(^uint(0) >> 1) // max int
			bestArea := -1
			for _, sc := range screens {
				if sc == nil {
					continue
				}
				wa, ok := normaliseWorkAreaDip(sc)
				if !ok {
					continue
				}
				// score: squared distance to origin in DIP (avoid math import)
				dx := sc.Bounds.X
				dy := sc.Bounds.Y
				if dx < 0 {
					dx = -dx
				}
				if dy < 0 {
					dy = -dy
				}
				score := dx*dx + dy*dy
				area := wa.Width * wa.Height
				if best == nil || score < bestScore || (score == bestScore && area > bestArea) {
					best = sc
					bestScore = score
					bestArea = area
				}
			}
			if best != nil {
				if wa, ok := normaliseWorkAreaDip(best); ok {
					s.primaryWorkArea = wa
					s.primaryPhysicalWorkArea = best.PhysicalWorkArea
					s.primaryScaleFactor = best.ScaleFactor
					s.primaryWorkAreaSource = "app_primary_best_guess"
					s.hasPrimaryWorkArea = true
					s.debugLog("floatingball:workarea:cache", map[string]any{
						"source":           s.primaryWorkAreaSource,
						"workArea":         s.primaryWorkArea,
						"bounds":           best.Bounds,
						"physicalWorkArea": s.primaryPhysicalWorkArea,
						"scaleFactor":      s.primaryScaleFactor,
						"isPrimary":        best.IsPrimary,
						"screenName":       best.Name,
						"screenID":         best.ID,
						"score":            bestScore,
						"area":             bestArea,
					})
					return s.primaryWorkArea, true
				}
			}
		} else {
			s.debugLog("floatingball:workarea:getall_empty", map[string]any{
				"source": "app",
			})
		}

		// Diagnostics: why did app.Screen not yield a usable primary?
		if !s.loggedScreenProbe {
			s.loggedScreenProbe = true
			brief := make([]map[string]any, 0, len(screens))
			for _, sc := range screens {
				if sc == nil {
					continue
				}
				brief = append(brief, map[string]any{
					"id":        sc.ID,
					"name":      sc.Name,
					"isPrimary": sc.IsPrimary,
					"scale":     sc.ScaleFactor,
					"bounds":    sc.Bounds,
					"workArea":  sc.WorkArea,
				})
			}
			s.debugLog("floatingball:workarea:primary_unavailable", map[string]any{
				"getPrimaryNil": primary == nil,
				"screenCount":   len(screens),
				"screens":       brief,
			})
		}
	}

	// If we already have a cached value (from fallback), reuse it.
	if s.hasPrimaryWorkArea && s.primaryWorkArea.Width > 0 && s.primaryWorkArea.Height > 0 {
		return s.primaryWorkArea, true
	}

	// 2) Fallback: use the floating window's screen info *once* (typically the initial primary screen).
	// Prefer the main window's screen (more likely to stay on primary). Do NOT use this as a dynamic
	// per-monitor reference; it's only to bootstrap the cache.
	if s.mainWindow != nil {
		if screen, _ := s.mainWindow.GetScreen(); screen != nil {
			if wa, ok := normaliseWorkAreaDip(screen); ok {
				s.primaryWorkArea = wa
				s.primaryPhysicalWorkArea = screen.PhysicalWorkArea
				s.primaryScaleFactor = screen.ScaleFactor
				s.primaryWorkAreaSource = "main_window"
				s.hasPrimaryWorkArea = true
				s.debugLog("floatingball:workarea:cache", map[string]any{
					"source":           s.primaryWorkAreaSource,
					"workArea":         s.primaryWorkArea,
					"bounds":           screen.Bounds,
					"physicalWorkArea": s.primaryPhysicalWorkArea,
					"scaleFactor":      s.primaryScaleFactor,
				})
				return s.primaryWorkArea, true
			}
		}
	}

	if s.win != nil {
		if screen, _ := s.win.GetScreen(); screen != nil {
			if wa, ok := normaliseWorkAreaDip(screen); ok {
				s.primaryWorkArea = wa
				s.primaryPhysicalWorkArea = screen.PhysicalWorkArea
				s.primaryScaleFactor = screen.ScaleFactor
				s.primaryWorkAreaSource = "floating_window"
				s.hasPrimaryWorkArea = true
				s.debugLog("floatingball:workarea:cache", map[string]any{
					"source":           s.primaryWorkAreaSource,
					"workArea":         s.primaryWorkArea,
					"bounds":           screen.Bounds,
					"physicalWorkArea": s.primaryPhysicalWorkArea,
					"scaleFactor":      s.primaryScaleFactor,
				})
				return s.primaryWorkArea, true
			}
		}
	}

	return application.Rect{}, false
}

func (s *FloatingBallService) physicalWorkAreaLocked() (application.Rect, float32, bool) {
	// Ensure cache is populated if possible.
	if !s.hasPrimaryWorkArea || s.primaryWorkArea.Width <= 0 || s.primaryWorkArea.Height <= 0 {
		_, _ = s.workAreaLocked()
	}

	sf := s.primaryScaleFactor
	if sf <= 0 {
		sf = 1
	}

	// Build a "scaled physical" work area from DIP.
	if s.primaryWorkArea.Width > 0 && s.primaryWorkArea.Height > 0 {
		scaled := application.Rect{
			X:      dipToPhysical(s.primaryWorkArea.X, sf),
			Y:      dipToPhysical(s.primaryWorkArea.Y, sf),
			Width:  dipToPhysical(s.primaryWorkArea.Width, sf),
			Height: dipToPhysical(s.primaryWorkArea.Height, sf),
		}

		// Some platforms report PhysicalWorkArea identical to WorkArea even when ScaleFactor>1.
		// In that case, using the reported PhysicalWorkArea would clamp to half the screen.
		if s.primaryPhysicalWorkArea.Width > 0 && s.primaryPhysicalWorkArea.Height > 0 {
			reported := s.primaryPhysicalWorkArea
			looksUnscaled := sf > 1.1 &&
				(reported.Width == s.primaryWorkArea.Width || reported.Height == s.primaryWorkArea.Height) &&
				(scaled.Width > reported.Width || scaled.Height > reported.Height)

			if looksUnscaled {
				if !s.loggedApproxPhysical {
					s.loggedApproxPhysical = true
					s.debugLog("floatingball:workarea:physical:override_unscaled", map[string]any{
						"source":      s.primaryWorkAreaSource,
						"workArea":    s.primaryWorkArea,
						"scaleFactor": sf,
						"reported":    reported,
						"scaled":      scaled,
					})
				}
				return scaled, sf, true
			}

			return reported, sf, true
		}

		// No reported physical work area: use scaled DIP.
		if !s.loggedApproxPhysical {
			s.loggedApproxPhysical = true
			s.debugLog("floatingball:workarea:physical:approx", map[string]any{
				"source":      s.primaryWorkAreaSource,
				"workArea":    s.primaryWorkArea,
				"scaleFactor": sf,
				"scaled":      scaled,
			})
		}
		return scaled, sf, true
	}

	// Fallback: if we only have reported physical work area, return it.
	if s.primaryPhysicalWorkArea.Width > 0 && s.primaryPhysicalWorkArea.Height > 0 {
		return s.primaryPhysicalWorkArea, sf, true
	}

	return application.Rect{}, sf, false
}

