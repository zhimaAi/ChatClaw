package windows

import (
	"context"
	"runtime"
	"sort"
	"sync"
	"time"

	"willchat/internal/errs"
	"willchat/internal/services/settings"
	"willchat/pkg/winsnap"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

type SnapState string

const (
	SnapStateStopped    SnapState = "stopped"
	SnapStateAttached   SnapState = "attached"
	SnapStateHidden     SnapState = "hidden"
	SnapStateStandalone SnapState = "standalone" // Window is visible but not attached to any target
)

type SnapStatus struct {
	State           SnapState `json:"state"`
	EnabledKeys     []string  `json:"enabledKeys"`
	EnabledTargets  []string  `json:"enabledTargets"`
	TargetProcess   string    `json:"targetProcess"`
	LastError       string    `json:"lastError,omitempty"`
	UpdatedAtUnixMs int64     `json:"updatedAtUnixMs"`
}

// SnapService manages the single "winsnap" window and dynamically attaches it to
// the top-most window among all enabled target applications.
//
// Rules:
// - Each toggle is independent (not mutually exclusive).
// - Any enabled toggle keeps winsnap window running.
// - Attach to the highest z-order visible window among enabled apps.
// - If no enabled app is visible, move winsnap window off-screen and mark state hidden.
type SnapService struct {
	app    *application.App
	winSvc *WindowService

	mu sync.Mutex

	enabledKeys    []string
	enabledTargets []string

	win       *application.WebviewWindow
	readyOnce sync.Once
	readyCh   chan struct{}

	ctrl                 winsnap.Controller
	currentTarget        string
	lastAttachedTarget   string // Remembers the last attached target when hidden, for wake restoration
	lastWinsnapMinimized bool

	loopCancel context.CancelFunc

	status SnapStatus
}

func NewSnapService(app *application.App, winSvc *WindowService) (*SnapService, error) {
	if app == nil {
		return nil, errs.New("error.app_required")
	}
	if winSvc == nil {
		return nil, errs.New("error.window_service_required")
	}
	s := &SnapService{
		app:    app,
		winSvc: winSvc,
		readyCh: func() chan struct{} {
			ch := make(chan struct{})
			return ch
		}(),
		status: SnapStatus{
			State: SnapStateStopped,
		},
	}
	return s, nil
}

func (s *SnapService) GetStatus() SnapStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.status
}

// WakeAttached brings the attached target window and the winsnap window to the front,
// then returns keyboard focus to the winsnap window.
// This is called when user clicks on the winsnap window - we want to:
// 1. Bring target window to front (so it's not hidden by other apps)
// 2. Return focus to winsnap so user can continue typing
// If the winsnap window is invalid/closed, it will be recreated on the next loop tick.
func (s *SnapService) WakeAttached() error {
	s.mu.Lock()
	w := s.win
	target := s.currentTarget
	s.mu.Unlock()

	if w == nil || target == "" {
		return nil
	}
	// Use WakeAttachedWindowWithRefocus to return focus to winsnap after syncing z-order
	err := winsnap.WakeAttachedWindowWithRefocus(w, target)
	if err == winsnap.ErrWinsnapWindowInvalid {
		// Winsnap window became invalid - trigger recreation
		s.handleWinsnapWindowInvalid(target)
		return nil // Don't return error, window will be recreated
	}
	return err
}

// WakeWindow brings the winsnap window to the front, regardless of snap state.
// This is used by text selection service:
// - In attached state: shows target window (no activate) and wakes winsnap window.
// - In hidden state with last attached target: shows target, restores attached state, wakes winsnap.
// - In standalone state: wakes only the winsnap window.
// - If the winsnap window does not exist (never created / closed), it will be recreated on-demand.
//
// IMPORTANT: Focus always stays on winsnap window. Target window is only shown (not activated).
func (s *SnapService) WakeWindow() {
	// Ensure the winsnap window exists even when snapping loop is not running.
	// Product requirement: text selection popup should always interact with winsnap.
	s.mu.Lock()
	w := s.win
	target := s.currentTarget
	state := s.status.State
	lastAttached := s.lastAttachedTarget
	s.mu.Unlock()

	// Determine which target to show (current or last attached)
	targetToShow := target
	if targetToShow == "" {
		targetToShow = lastAttached
	}

	// If window is nil or invalid, recreate it via WindowService.
	if w == nil || w.NativeWindow() == nil || uintptr(w.NativeWindow()) == 0 {
		// Show() will create the window if needed (FocusOnShow is false).
		_ = s.winSvc.Show(WindowWinsnap)
		s.mu.Lock()
		ww, err := s.winSvc.ensure(WindowWinsnap)
		if err == nil && ww != nil {
			s.installWindowHooksLocked(ww)
			w = ww
			s.win = ww
		}
		s.mu.Unlock()

		if w == nil || w.NativeWindow() == nil || uintptr(w.NativeWindow()) == 0 {
			return
		}

		// Window recreated: try to restore attachment to last target
		if targetToShow != "" {
			// Show target window without activating
			_ = winsnap.ShowTargetWindowNoActivate(w, targetToShow)
			// Re-establish attachment
			s.attachTo(w, targetToShow)
			// Wake winsnap window (get focus)
			_ = winsnap.WakeStandaloneWindow(w)
			return
		}

		// No target: standalone mode
		s.moveToStandalone(w)
		s.mu.Lock()
		s.status.State = SnapStateStandalone
		s.status.TargetProcess = ""
		s.touchLocked("")
		s.mu.Unlock()
		_ = winsnap.WakeStandaloneWindow(w)
		return
	}

	// Window exists

	// If currently attached, show target window (no activate), focus on winsnap
	if state == SnapStateAttached && target != "" {
		_ = winsnap.ShowTargetWindowNoActivate(w, target)
		_ = winsnap.WakeStandaloneWindow(w)
		return
	}

	// If hidden but was previously attached to a target, try to restore attached state.
	// This handles the case where user minimized everything and then uses text selection popup.
	if (state == SnapStateHidden || state == SnapStateStopped) && targetToShow != "" {
		// Show target window without activating
		err := winsnap.ShowTargetWindowNoActivate(w, targetToShow)
		if err == nil {
			// Re-establish attachment
			s.attachTo(w, targetToShow)
			// Wake winsnap window (get focus)
			_ = winsnap.WakeStandaloneWindow(w)
			return
		}
		// If show failed (target not found), fall through to standalone mode.
	}

	// If it was hidden (off-screen) with no attached target, bring it back to standalone position.
	if state == SnapStateHidden || state == SnapStateStopped {
		s.moveToStandalone(w)
		s.mu.Lock()
		// Keep state consistent for downstream routing / UI.
		s.status.State = SnapStateStandalone
		s.status.TargetProcess = ""
		s.touchLocked("")
		s.mu.Unlock()
	}

	// Standalone or other state: wake the winsnap window only.
	// This properly activates the app on macOS (NSRunningApplication) and brings window to front on Windows.
	_ = winsnap.WakeStandaloneWindow(w)
}

// NotifySettingsChanged broadcasts a snap settings changed event to all windows.
// This is intended for cross-window UI refresh (main window settings page, etc.).
func (s *SnapService) NotifySettingsChanged() {
	if s.app == nil {
		return
	}
	s.app.Event.Emit("snap:settings-changed", nil)
}

// SendTextToTarget sends text to the currently attached target application.
// If triggerSend is true, it will also simulate the send key (Enter or Ctrl+Enter based on settings).
func (s *SnapService) SendTextToTarget(text string, triggerSend bool) error {
	s.mu.Lock()
	target := s.currentTarget
	state := s.status.State
	s.mu.Unlock()

	if state != SnapStateAttached || target == "" {
		return errs.New("error.no_attached_target")
	}

	// Get send key strategy from settings
	sendKeyStrategy := "enter"
	if v, ok := settings.GetValue("send_key_strategy"); ok && v != "" {
		sendKeyStrategy = v
	}

	// Get click settings for this target (for apps that need click to focus input box)
	noClick, clickOffsetX, clickOffsetY := getClickSettingsForTarget(target)

	return winsnap.SendTextToTarget(target, text, triggerSend, sendKeyStrategy, noClick, clickOffsetX, clickOffsetY)
}

// PasteTextToTarget pastes text to the currently attached target application's edit box.
// This does not trigger the send action.
func (s *SnapService) PasteTextToTarget(text string) error {
	s.mu.Lock()
	target := s.currentTarget
	state := s.status.State
	s.mu.Unlock()

	if state != SnapStateAttached || target == "" {
		return errs.New("error.no_attached_target")
	}

	// Get click settings for this target
	noClick, clickOffsetX, clickOffsetY := getClickSettingsForTarget(target)

	return winsnap.PasteTextToTarget(target, text, noClick, clickOffsetX, clickOffsetY)
}

// SyncFromSettings reads snap toggles from settings cache, then starts/stops
// snapping loop accordingly.
func (s *SnapService) SyncFromSettings() (SnapStatus, error) {
	enabledKeys, enabledTargets := readSnapTargetsFromSettings()

	s.mu.Lock()
	s.enabledKeys = enabledKeys
	s.enabledTargets = enabledTargets
	s.status.EnabledKeys = append([]string(nil), enabledKeys...)
	s.status.EnabledTargets = append([]string(nil), enabledTargets...)
	s.status.LastError = ""
	s.touchLocked("")
	shouldRun := len(enabledTargets) > 0
	s.mu.Unlock()

	if !shouldRun {
		_ = s.stop()
		status := s.GetStatus()
		// Emit from backend because winsnap window may close during sync,
		// and frontend code after SyncFromSettings may never run.
		s.app.Event.Emit("snap:settings-changed", status)
		return status, nil
	}

	if err := s.ensureRunning(); err != nil {
		return s.GetStatus(), err
	}
	status := s.GetStatus()
	s.app.Event.Emit("snap:settings-changed", status)
	return status, nil
}

func (s *SnapService) ensureRunning() error {
	// Ensure window exists & shown (do not focus).
	if err := s.winSvc.Show(WindowWinsnap); err != nil {
		return err
	}

	s.mu.Lock()
	// We can safely call ensure here because we're in the same package; do NOT expose
	// any *application.WebviewWindow types to the frontend bindings.
	w, err := s.winSvc.ensure(WindowWinsnap)
	if err != nil {
		s.mu.Unlock()
		return err
	}
	s.installWindowHooksLocked(w)

	alreadyLooping := s.loopCancel != nil
	if alreadyLooping {
		s.mu.Unlock()
		return nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	s.loopCancel = cancel
	s.mu.Unlock()

	go s.loop(ctx)
	return nil
}

func (s *SnapService) stop() error {
	s.mu.Lock()
	cancel := s.loopCancel
	s.loopCancel = nil
	s.mu.Unlock()
	if cancel != nil {
		cancel()
	}

	s.mu.Lock()
	if s.ctrl != nil {
		_ = s.ctrl.Stop()
		s.ctrl = nil
	}
	s.currentTarget = ""
	s.lastWinsnapMinimized = false
	s.status.TargetProcess = ""
	s.status.LastError = ""
	w := s.win
	s.mu.Unlock()

	// Instead of closing the window, move it to a standalone position (centered on screen)
	// so it can wait for new snap relationships
	if w != nil {
		s.moveToStandalone(w)
		s.mu.Lock()
		s.status.State = SnapStateStandalone
		s.touchLocked("")
		s.mu.Unlock()
	} else {
		s.mu.Lock()
		s.status.State = SnapStateStopped
		s.touchLocked("")
		s.mu.Unlock()
	}

	return nil
}

// moveToStandalone moves the winsnap window to a standalone position (centered on screen).
func (s *SnapService) moveToStandalone(w *application.WebviewWindow) {
	if w == nil {
		return
	}
	// Move window to center of primary screen
	winsnap.MoveToStandalone(w)
}

func (s *SnapService) loop(ctx context.Context) {
	// Wait until the winsnap window is ready (or at least shown) to ensure a valid native handle.
	s.mu.Lock()
	readyCh := s.readyCh
	s.mu.Unlock()
	select {
	case <-readyCh:
	case <-ctx.Done():
		return
	}

	// Run immediately then on a ticker.
	s.step()
	ticker := time.NewTicker(400 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			s.step()
		case <-ctx.Done():
			return
		}
	}
}

func (s *SnapService) step() {
	s.mu.Lock()
	enabledTargets := append([]string(nil), s.enabledTargets...)
	w := s.win
	wasMinimized := s.lastWinsnapMinimized
	targetForRestore := s.currentTarget
	stateForRestore := s.status.State
	s.mu.Unlock()

	if len(enabledTargets) == 0 {
		// If toggles became empty while loop is still running, stop.
		_ = s.stop()
		return
	}
	if w == nil {
		// Window got closed; re-create.
		if err := s.ensureRunning(); err != nil {
			s.mu.Lock()
			s.status.LastError = err.Error()
			s.touchLocked(err.Error())
			s.mu.Unlock()
		}
		return
	}

	// Detect restore from minimised state (Windows Win+D / taskbar restore).
	// NOTE: w.IsVisible() may stay true even when minimized, so we must use IsIconic.
	if isMin, _ := winsnap.IsWindowMinimized(w); isMin != wasMinimized {
		s.mu.Lock()
		s.lastWinsnapMinimized = isMin
		s.mu.Unlock()
		// Only resync on true -> false transitions.
		if wasMinimized && !isMin && targetForRestore != "" && stateForRestore == SnapStateAttached {
			go func() {
				time.Sleep(80 * time.Millisecond) // allow restore layout to settle
				_ = winsnap.SyncRightOfProcessNow(winsnap.AttachOptions{
					TargetProcessName: targetForRestore,
					Gap:               0,
					App:               s.app,
					Window:            w,
				})
			}()
		}
	}

	target, found, err := winsnap.TopMostVisibleProcessName(enabledTargets)
	if err != nil {
		// Check if our own app is frontmost (user interacting with our app's window)
		// In this case, preserve current snap state - don't hide or change anything.
		if err == winsnap.ErrSelfIsFrontmost {
			// Simply preserve current state when our app is frontmost.
			// Don't do any target obscured checking or z-order manipulation here
			// to avoid potential crashes when windows are being moved.
			return
		}
		// Other errors: snapping is not supported or failed.
		s.mu.Lock()
		s.status.LastError = err.Error()
		s.touchLocked(err.Error())
		s.mu.Unlock()
		return
	}

	// If currently in standalone state, don't hide even if no target is found.
	// Standalone mode means the winsnap window is intentionally visible without attachment.
	// This is used when text selection popup wakes the winsnap window on-demand.
	if !found || target == "" {
		s.mu.Lock()
		currentState := s.status.State
		s.mu.Unlock()

		// In standalone state, keep the window visible - don't hide it
		if currentState == SnapStateStandalone {
			return
		}

		s.hideOffscreen(w)
		return
	}
	s.attachTo(w, target)
}

func (s *SnapService) hideOffscreen(w *application.WebviewWindow) {
	s.mu.Lock()
	// Stop any existing controller first.
	if s.ctrl != nil {
		_ = s.ctrl.Stop()
		s.ctrl = nil
	}
	oldState := s.status.State
	oldTarget := s.status.TargetProcess

	// Remember the last attached target for wake restoration.
	// This allows WakeWindow to restore attached state instead of going standalone.
	if oldState == SnapStateAttached && s.currentTarget != "" {
		s.lastAttachedTarget = s.currentTarget
	}

	s.currentTarget = ""
	s.lastWinsnapMinimized = false
	s.status.State = SnapStateHidden
	s.status.TargetProcess = ""
	s.status.LastError = ""
	s.touchLocked("")
	s.mu.Unlock()

	_ = winsnap.MoveOffscreen(w)

	// Emit state-changed event if state actually changed
	if oldState != SnapStateHidden || oldTarget != "" {
		s.app.Event.Emit("snap:state-changed", map[string]interface{}{
			"state":         string(SnapStateHidden),
			"targetProcess": "",
		})
	}
}

func (s *SnapService) attachTo(w *application.WebviewWindow, targetProcess string) {
	s.mu.Lock()
	if s.currentTarget == targetProcess && s.ctrl != nil {
		// Already attached to this target. The darwinFollower's activation observer
		// (NSWorkspaceDidActivateApplicationNotification) handles z-order when
		// the target app becomes frontmost. We just need to ensure the winsnap
		// window is visible (not hidden) so the observer can adjust its z-order.
		s.mu.Unlock()
		// Ensure winsnap is visible (in case it was hidden by MoveOffscreen)
		// This is needed because winsnap_order_above_target checks [selfWin isVisible]
		_ = winsnap.EnsureWindowVisible(w)
		return
	}
	// Stop current controller (if any) before switching target.
	if s.ctrl != nil {
		_ = s.ctrl.Stop()
		s.ctrl = nil
	}
	s.mu.Unlock()

	// Restore winsnap if it was minimized (e.g. by Win+D); when a target becomes visible again, show the snap window.
	if err := winsnap.EnsureWindowVisible(w); err == winsnap.ErrWinsnapWindowInvalid {
		s.handleWinsnapWindowInvalid(targetProcess)
		return
	}

	c, err := winsnap.AttachRightOfProcess(winsnap.AttachOptions{
		TargetProcessName: targetProcess,
		Gap:               0,
		FindTimeout:       800 * time.Millisecond,
		App:               s.app,
		Window:            w,
	})
	if err != nil {
		// Check if winsnap window is invalid/closed - need to recreate
		if err == winsnap.ErrWinsnapWindowInvalid {
			s.handleWinsnapWindowInvalid(targetProcess)
			return
		}
		// Treat as temporarily hidden; next tick will retry.
		s.mu.Lock()
		s.status.LastError = err.Error()
		s.touchLocked(err.Error())
		s.mu.Unlock()
		return
	}

	s.mu.Lock()
	oldState := s.status.State
	oldTarget := s.status.TargetProcess
	s.ctrl = c
	s.currentTarget = targetProcess
	s.lastAttachedTarget = "" // Clear: now actively attached, no need to remember
	s.lastWinsnapMinimized, _ = winsnap.IsWindowMinimized(w)
	s.status.State = SnapStateAttached
	s.status.TargetProcess = targetProcess
	s.status.LastError = ""
	s.touchLocked("")
	s.mu.Unlock()

	// Emit state-changed event if state actually changed
	if oldState != SnapStateAttached || oldTarget != targetProcess {
		s.app.Event.Emit("snap:state-changed", map[string]interface{}{
			"state":         string(SnapStateAttached),
			"targetProcess": targetProcess,
		})
	}

	// Immediately sync z-order after attaching to ensure the winsnap window
	// is visible above the target window. Without this, the winsnap may appear
	// behind other windows until the target is moved or activated.
	// IMPORTANT: Do NOT activate the target app here. Attaching can happen in the background
	// (e.g. when a target window is visible but not currently focused). Activating would steal
	// focus from the user's current app, which is not desired on macOS.
	//
	// We only adjust z-order when the target app is already frontmost.
	if err := winsnap.SyncAttachedZOrderNoActivate(w, targetProcess); err == winsnap.ErrWinsnapWindowInvalid {
		// Winsnap window became invalid after attach - recreate
		s.handleWinsnapWindowInvalid(targetProcess)
	}
}

func (s *SnapService) installWindowHooksLocked(w *application.WebviewWindow) {
	if s.win == w && s.readyCh != nil {
		// Already installed for this window instance.
		return
	}

	s.win = w
	s.readyOnce = sync.Once{}
	s.readyCh = make(chan struct{})

	w.OnWindowEvent(events.Common.WindowRuntimeReady, func(_ *application.WindowEvent) {
		s.mu.Lock()
		defer s.mu.Unlock()
		if s.win != w {
			return
		}
		s.readyOnce.Do(func() { close(s.readyCh) })
	})
	w.OnWindowEvent(events.Common.WindowShow, func(_ *application.WindowEvent) {
		s.mu.Lock()
		if s.win != w {
			s.mu.Unlock()
			return
		}
		s.readyOnce.Do(func() { close(s.readyCh) })
		target := s.currentTarget
		state := s.status.State
		s.lastWinsnapMinimized = false
		s.mu.Unlock()

		// When the winsnap window is restored/shown, force a one-shot resync to the
		// attached target window's position and z-order. Otherwise, the follower
		// only updates on target move/foreground events, which may not fire on restore.
		if target != "" && state == SnapStateAttached {
			go func() {
				time.Sleep(60 * time.Millisecond) // allow restore layout to settle
				_ = winsnap.SyncRightOfProcessNow(winsnap.AttachOptions{
					TargetProcessName: target,
					Gap:               0,
					App:               s.app,
					Window:            w,
				})
			}()
		}
	})
	w.OnWindowEvent(events.Common.WindowClosing, func(_ *application.WindowEvent) {
		s.mu.Lock()
		if s.ctrl != nil {
			_ = s.ctrl.Stop()
			s.ctrl = nil
		}
		if s.win == w {
			s.win = nil
		}
		// Save current target to lastAttachedTarget for restoration when window is reopened
		if s.currentTarget != "" {
			s.lastAttachedTarget = s.currentTarget
		}
		s.currentTarget = ""
		s.lastWinsnapMinimized = false
		// Update state to stopped when window is closed
		// This is important for text selection routing logic
		s.status.State = SnapStateStopped
		s.status.TargetProcess = ""
		s.touchLocked("")
		s.readyOnce.Do(func() { close(s.readyCh) })
		s.mu.Unlock()
	})
}

// handleWinsnapWindowInvalid handles the case when the winsnap window has been closed/released.
// It clears the current state and triggers recreation of the window on the next loop tick.
func (s *SnapService) handleWinsnapWindowInvalid(targetProcess string) {
	s.mu.Lock()
	// Stop current controller
	if s.ctrl != nil {
		_ = s.ctrl.Stop()
		s.ctrl = nil
	}
	// Clear window reference to trigger recreation
	s.win = nil
	s.currentTarget = ""
	s.lastWinsnapMinimized = false
	s.status.State = SnapStateHidden
	s.status.TargetProcess = ""
	s.status.LastError = "winsnap window invalid, will recreate"
	s.touchLocked("")
	s.mu.Unlock()

	// Close the window in WindowService to ensure clean state
	_ = s.winSvc.Close(WindowWinsnap)

	// The next loop tick will detect s.win == nil and call ensureRunning() to recreate
}

func (s *SnapService) touchLocked(lastErr string) {
	s.status.UpdatedAtUnixMs = time.Now().UnixMilli()
	if lastErr != "" {
		s.status.LastError = lastErr
	}
}

func readSnapTargetsFromSettings() (enabledKeys []string, enabledTargets []string) {
	type item struct {
		key     string
		targets []string
	}

	// Multi-target per key to support different executables across versions.
	items := []item{
		{key: "snap_wechat", targets: snapTargetsForKey("snap_wechat")},
		{key: "snap_wecom", targets: snapTargetsForKey("snap_wecom")},
		{key: "snap_qq", targets: snapTargetsForKey("snap_qq")},
		{key: "snap_dingtalk", targets: snapTargetsForKey("snap_dingtalk")},
		{key: "snap_feishu", targets: snapTargetsForKey("snap_feishu")},
		{key: "snap_douyin", targets: snapTargetsForKey("snap_douyin")},
	}

	for _, it := range items {
		if !settings.GetBool(it.key, false) {
			continue
		}
		enabledKeys = append(enabledKeys, it.key)
		enabledTargets = append(enabledTargets, it.targets...)
	}

	// Stable ordering for status/debugging.
	sort.Strings(enabledKeys)
	enabledTargets = uniqueStrings(enabledTargets)
	sort.Strings(enabledTargets)
	return enabledKeys, enabledTargets
}

func uniqueStrings(in []string) []string {
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		if s == "" {
			continue
		}
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}

func snapTargetsForKey(key string) []string {
	// Windows focus for now; macOS names are best-effort.
	switch runtime.GOOS {
	case "windows":
		switch key {
		case "snap_wechat":
			// WeChat process name differs across versions/channels.
			// NOTE: Windows matching is based on process image name (exe). "Weixin.exe" is used by some channels.
			return []string{"Weixin.exe", "WeChat.exe", "WeChatApp.exe", "WeChatAppEx.exe"}
		case "snap_wecom":
			return []string{"WXWork.exe"}
		case "snap_qq":
			return []string{"QQ.exe", "QQNT.exe"}
		case "snap_dingtalk":
			return []string{"DingTalk.exe"}
		case "snap_feishu":
			return []string{"Feishu.exe", "Lark.exe"}
		case "snap_douyin":
			return []string{"Douyin.exe"}
		default:
			return nil
		}
	default:
		// macOS / others: use app localized names where possible.
		switch key {
		case "snap_wechat":
			// macOS: localized name / executable / common aliases / bundle id.
			return []string{"微信", "Weixin", "weixin", "WeChat", "wechat", "com.tencent.xinWeChat"}
		case "snap_wecom":
			return []string{"企业微信", "WeCom", "wecom", "WeWork", "wework", "WXWork", "wxwork", "qiyeweixin", "com.tencent.WeWorkMac"}
		case "snap_qq":
			return []string{"QQ", "qq", "com.tencent.qq"}
		case "snap_dingtalk":
			return []string{"钉钉", "DingTalk", "dingtalk", "com.alibaba.DingTalkMac"}
		case "snap_feishu":
			return []string{"飞书", "Feishu", "feishu", "Lark", "lark", "com.bytedance.feishu", "com.bytedance.Lark"}
		case "snap_douyin":
			return []string{"抖音", "Douyin", "douyin"}
		default:
			return nil
		}
	}
}

// snapKeyForTarget returns the settings key (e.g., "snap_dingtalk") for a given target process name
func snapKeyForTarget(targetProcess string) string {
	keys := []string{"snap_wechat", "snap_wecom", "snap_qq", "snap_dingtalk", "snap_feishu", "snap_douyin"}
	for _, key := range keys {
		targets := snapTargetsForKey(key)
		for _, t := range targets {
			if t == targetProcess {
				return key
			}
		}
	}
	return ""
}

// getClickSettingsForTarget returns the configured click settings for a target process
// Returns noClick (skip mouse click), offsetX, offsetY
func getClickSettingsForTarget(targetProcess string) (noClick bool, offsetX, offsetY int) {
	key := snapKeyForTarget(targetProcess)
	if key == "" {
		return false, 0, 0
	}
	// Setting key format: snap_[app]_no_click, snap_[app]_click_offset_x/y
	noClick = settings.GetBool(key+"_no_click", false)
	offsetX = settings.GetInt(key+"_click_offset_x", 0)
	offsetY = settings.GetInt(key+"_click_offset_y", 0)
	// If not configured (0 or empty), fall back to per-app defaults to match frontend UX.
	// This is important on macOS where the click implementation otherwise falls back to a generic value.
	if offsetY <= 0 {
		offsetY = defaultClickOffsetYForKey(key)
	}
	return noClick, offsetX, offsetY
}

// defaultClickOffsetYForKey returns the default click Y offset (pixels from bottom)
// used to focus the input box in the target app.
//
// Keep this consistent with frontend defaults in SnapSettings.vue.
func defaultClickOffsetYForKey(key string) int {
	switch key {
	case "snap_feishu":
		return 50
	default:
		return 120
	}
}
