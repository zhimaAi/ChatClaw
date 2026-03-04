package windows

import (
	"context"
	"encoding/json"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"chatclaw/internal/errs"
	"chatclaw/internal/services/settings"
	"chatclaw/pkg/winsnap"

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

// snapGap returns the pixel gap between the snap window and the target app.
func snapGap() int {
	if runtime.GOOS == "windows" {
		return 0
	}
	return 3
}

type SnapStatus struct {
	State           SnapState `json:"state"`
	EnabledKeys     []string  `json:"enabledKeys"`
	EnabledTargets  []string  `json:"enabledTargets"`
	TargetProcess   string    `json:"targetProcess"`
	LastError       string    `json:"lastError,omitempty"`
	UpdatedAtUnixMs int64     `json:"updatedAtUnixMs"`
}

type SnapAppCandidate struct {
	Name        string `json:"name"`
	ProcessName string `json:"processName"`
	Icon        string `json:"icon,omitempty"`
}

type customSnapAppConfig struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	ProcessName string `json:"processName"`
	Icon        string `json:"icon,omitempty"`
}

const (
	snapCustomAppsSettingKey        = "snap_custom_apps"
	snapCustomKeyPrefix             = "snap_custom_"
	snapDragGuardUntilKey           = "snap_drag_guard_until_unix_ms"
	wakeAttachedGuardAfterSwitch    = 1200 * time.Millisecond
	attachedLowFreqRescanInterval   = 1200 * time.Millisecond
	attachedLowFreqRescanSwitchHits = 5 // Increased from 3 to 5 for more stable switching
	attachedMinSwitchInterval       = 800 * time.Millisecond
)

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
	attachMu sync.Mutex

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
	switchRollbackFrom  string
	switchRollbackTo    string
	switchRollbackUntil time.Time
	wakeAttachedGuardUntil time.Time
	lastSwitchAt time.Time
	lastAttachedRescanAt time.Time
	attachedRescanCandidate string
	attachedRescanHits      int
	obscuredCheckTarget     string // Last target name checked for obscured/non-adjacent state
	obscuredCheckHits       int    // Consecutive obscured/non-adjacent hits for that target
	attachingLock           bool // Lock to prevent concurrent attaching
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

// ListAvailableApps returns currently running foreground applications so frontend
// can let users pick one as a custom snap target.
func (s *SnapService) ListAvailableApps() ([]SnapAppCandidate, error) {
	return listRunningApps()
}

// WakeAttached brings the attached target window and the winsnap window to the front,
// then returns keyboard focus to the winsnap window.
// This is called when user clicks on the winsnap window - we want to:
// 1. Bring target window to front (so it's not hidden by other apps)
// 2. Return focus to winsnap so user can continue typing
// If the winsnap window is invalid/closed, it will be recreated on the next loop tick.
func (s *SnapService) WakeAttached() error {
	if s.isDragGuardActive() {
		return nil
	}

	s.mu.Lock()
	w := s.win
	target := s.currentTarget
	guardUntil := s.wakeAttachedGuardUntil
	s.mu.Unlock()

	if w == nil || target == "" {
		return nil
	}
	if time.Now().Before(guardUntil) {
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

	// Get enabled targets to validate targetToShow
	s.mu.Lock()
	enabledTargets := append([]string(nil), s.enabledTargets...)
	s.mu.Unlock()

	// Validate targetToShow: only restore if it's still enabled
	if targetToShow != "" {
		isEnabled := false
		for _, enabled := range enabledTargets {
			if strings.EqualFold(enabled, targetToShow) {
				isEnabled = true
				break
			}
		}
		if !isEnabled {
			targetToShow = ""
		}
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

		// Window recreated: try to restore attachment to last target (only if still enabled)
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
	// But first check if target is still enabled
	if state == SnapStateAttached && target != "" {
		isTargetEnabled := false
		for _, enabled := range enabledTargets {
			if strings.EqualFold(enabled, target) {
				isTargetEnabled = true
				break
			}
		}
		if isTargetEnabled {
			_ = winsnap.ShowTargetWindowNoActivate(w, target)
			_ = winsnap.WakeStandaloneWindow(w)
			return
		}
		// Target is no longer enabled, fall through to detach
	}

	// If hidden but was previously attached to a target, try to restore attached state.
	// This handles the case where user minimized everything and then uses text selection popup.
	// Only restore if targetToShow is still enabled (already validated above).
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

// DetachToStandalone detaches the winsnap window from its current target and
// moves it to a standalone position (right side of screen). If other snap app
// toggles are still enabled, the polling loop keeps running so the window can
// re-attach when one of those apps becomes foreground.
func (s *SnapService) DetachToStandalone() error {
	// 1. Stop the current attach controller only (NOT the polling loop yet)
	s.mu.Lock()
	if s.ctrl != nil {
		_ = s.ctrl.Stop()
		s.ctrl = nil
	}
	oldState := s.status.State
	s.currentTarget = ""
	s.lastAttachedTarget = ""
	s.lastWinsnapMinimized = false
	s.status.TargetProcess = ""
	s.status.LastError = ""
	w := s.win
	s.mu.Unlock()

	// 2. Move to standalone position (NOT hide)
	if w != nil {
		s.moveToStandalone(w)
	}

	// 3. Re-read settings — the caller already disabled the current target's
	//    toggle, but other apps may still be enabled (e.g. WeCom when DingTalk
	//    was just cancelled). Sync the enabled lists from the settings cache.
	enabledKeys, enabledTargets := readSnapTargetsFromSettings()

	s.mu.Lock()
	s.enabledKeys = enabledKeys
	s.enabledTargets = enabledTargets
	s.status.EnabledKeys = append([]string(nil), enabledKeys...)
	s.status.EnabledTargets = append([]string(nil), enabledTargets...)
	s.status.State = SnapStateStandalone
	s.touchLocked("")
	s.mu.Unlock()

	// 4. Keep polling when there are still enabled targets so standalone mode can
	// automatically re-attach when another target app becomes foreground.
	// Only stop polling when no targets are enabled at all.
	if len(enabledTargets) == 0 {
		s.mu.Lock()
		cancel := s.loopCancel
		s.loopCancel = nil
		s.mu.Unlock()
		if cancel != nil {
			cancel()
		}
	} else {
		_ = s.ensureRunning()
	}

	// 5. Emit state-changed event so frontend can update UI
	if oldState != SnapStateStandalone {
		s.app.Event.Emit("snap:state-changed", map[string]interface{}{
			"state":         string(SnapStateStandalone),
			"targetProcess": "",
		})
	}
	s.app.Event.Emit("snap:settings-changed", s.GetStatus())

	return nil
}

// FindSnapTarget scans ALL supported snap target applications (regardless of
// whether their settings toggles are enabled) and returns the settings key
// (e.g. "snap_wechat") of the top-most visible window. Returns empty string
// if no supported app is visible.
func (s *SnapService) FindSnapTarget() (string, error) {
	allKeys := []string{"snap_wechat", "snap_wecom", "snap_qq", "snap_dingtalk", "snap_feishu", "snap_douyin"}
	var allTargets []string
	for _, key := range allKeys {
		allTargets = append(allTargets, snapTargetsForKey(key)...)
	}
	for _, app := range loadCustomSnapAppsFromSettings() {
		if app.ProcessName != "" {
			allTargets = append(allTargets, app.ProcessName)
		}
	}
	allTargets = uniqueStrings(allTargets)

	target, found, err := winsnap.TopMostVisibleProcessName(allTargets)
	if err != nil {
		// Ignore ErrSelfIsFrontmost — treat as "no target found"
		if err == winsnap.ErrSelfIsFrontmost {
			return "", nil
		}
		return "", err
	}
	if !found || target == "" {
		return "", nil
	}
	return snapKeyForTarget(target), nil
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

// ensureWindow creates the winsnap window (if not already created), installs
// event hooks, and stores the reference. Safe to call multiple times.
func (s *SnapService) ensureWindow() (*application.WebviewWindow, error) {
	if err := s.winSvc.Show(WindowWinsnap); err != nil {
		return nil, err
	}
	s.mu.Lock()
	// We can safely call ensure here because we're in the same package; do NOT expose
	// any *application.WebviewWindow types to the frontend bindings.
	w, err := s.winSvc.ensure(WindowWinsnap)
	if err != nil {
		s.mu.Unlock()
		return nil, err
	}
	s.installWindowHooksLocked(w)
	s.mu.Unlock()
	return w, nil
}

// ensureRunning starts the snap-detection loop. The winsnap window is NOT
// created here — it is lazily created inside step() only when a visible target
// application is found. This avoids a brief window flash when the user toggles
// a snap setting on.
func (s *SnapService) ensureRunning() error {
	s.mu.Lock()
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

	// Hide the window off-screen instead of showing it at a standalone position.
	// This avoids a brief flash when the user toggles snap settings off.
	// If text selection or other features need the window later, WakeWindow()
	// will bring it back (or recreate it if needed).
	if w != nil {
		_ = winsnap.MoveOffscreen(w)
	}
	s.mu.Lock()
	s.status.State = SnapStateStopped
	s.touchLocked("")
	s.mu.Unlock()

	return nil
}

// CloseSnapWindow hides the snap window and detaches from the current target.
// The caller (frontend) is responsible for disabling the current target's
// toggle in settings before calling this method.
//
// After hiding, the service re-reads persistent settings. If other snap
// toggles are still enabled, the polling loop keeps running so the window
// can reappear when one of those apps becomes foreground.
func (s *SnapService) CloseSnapWindow() error {
	// 1. Stop everything: polling loop + attach controller + hide window
	_ = s.stop()

	// IMPORTANT: CloseSnapWindow is a user-initiated "turn off" action from UI.
	// Even if some snap toggles remain enabled in settings, we should NOT restart
	// the polling loop here, otherwise the window may immediately "reopen" and
	// re-attach, which feels like a restart to users.
	//
	// Also clear lastAttachedTarget so WakeWindow (used by text selection) will
	// not restore attachment to an old target due to settings-cache race.
	s.mu.Lock()
	s.lastAttachedTarget = ""
	s.mu.Unlock()

	// 2. Re-read persistent settings — the caller only disabled the current
	//    target's toggle; other apps may still be enabled.
	enabledKeys, enabledTargets := readSnapTargetsFromSettings()

	s.mu.Lock()
	s.enabledKeys = enabledKeys
	s.enabledTargets = enabledTargets
	s.status.EnabledKeys = append([]string(nil), enabledKeys...)
	s.status.EnabledTargets = append([]string(nil), enabledTargets...)
	s.mu.Unlock()

	// 3. Emit events so frontend updates UI
	s.app.Event.Emit("snap:state-changed", map[string]interface{}{
		"state":         string(SnapStateStopped),
		"targetProcess": "",
	})
	s.app.Event.Emit("snap:settings-changed", s.GetStatus())

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
	// Run step immediately. The window is created lazily inside step() only
	// when a visible target is found, so we no longer wait for readyCh here.
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
	if s.isDragGuardActive() {
		return
	}

	// Re-read enabledTargets from settings on each step to ensure we have the latest list
	// This fixes the issue where disabling a snap app toggle doesn't immediately update the list
	enabledKeys, enabledTargetsFromSettings := readSnapTargetsFromSettings()

	s.mu.Lock()
	// Update enabledTargets if they changed
	s.enabledKeys = enabledKeys
	s.enabledTargets = enabledTargetsFromSettings
	s.status.EnabledKeys = append([]string(nil), enabledKeys...)
	s.status.EnabledTargets = append([]string(nil), enabledTargetsFromSettings...)
	
	enabledTargets := append([]string(nil), enabledTargetsFromSettings...)
	w := s.win
	currentTarget := s.currentTarget
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
		// Window doesn't exist yet or was closed. Only create it when a visible
		// target is found (lazy creation), avoiding a brief flash on settings toggle.
		target, found, err := winsnap.TopMostVisibleProcessName(enabledTargets)
		if err != nil {
			if err != winsnap.ErrSelfIsFrontmost {
				s.mu.Lock()
				s.status.LastError = err.Error()
				s.touchLocked(err.Error())
				s.mu.Unlock()
			}
			return
		}
		if !found || target == "" {
			return // No target visible, skip window creation
		}
		// Target found — create the window on demand.
		if _, err := s.ensureWindow(); err != nil {
			s.mu.Lock()
			s.status.LastError = err.Error()
			s.touchLocked(err.Error())
			s.mu.Unlock()
		}
		// Window just created; let it initialize. Next tick will attach.
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
					Gap:               snapGap(),
					App:               s.app,
					Window:            w,
				})
			}()
		}
	}

	frontTarget, frontFound, frontErr := winsnap.FrontMostTargetProcessName(enabledTargets)
	if frontErr != nil {
		// Check if our own app is frontmost (user interacting with our app's window)
		// In this case, preserve current snap state - don't hide or change anything.
		if frontErr == winsnap.ErrSelfIsFrontmost {
			return
		}
		s.mu.Lock()
		s.status.LastError = frontErr.Error()
		s.touchLocked(frontErr.Error())
		s.mu.Unlock()
		return
	}

	// In attached state, use strict foreground-driven switching:
	// - self foreground: keep current attachment (handled above by ErrSelfIsFrontmost)
	// - target foreground: switch only when front target differs from current target
	// - non-target/unknown foreground: keep current attachment, do not run fallback scan
	//
	// This avoids mis-switching when rapidly toggling between target and non-target apps.
	if stateForRestore == SnapStateAttached && currentTarget != "" {
		// Check if currentTarget is still in enabledTargets. If not, detach immediately.
		// This handles the case when user disables a snap app toggle while it's currently attached.
		isCurrentTargetEnabled := false
		for _, enabled := range enabledTargets {
			if strings.EqualFold(enabled, currentTarget) {
				isCurrentTargetEnabled = true
				break
			}
		}
		if !isCurrentTargetEnabled {
			// Current target is no longer enabled, detach immediately
			// Stop controller and clear state, but preserve lastSwitchAt to prevent rapid switching
			s.mu.Lock()
			if s.ctrl != nil {
				_ = s.ctrl.Stop()
				s.ctrl = nil
			}
			oldState := s.status.State
			oldTarget := s.status.TargetProcess
			enabledTargets := append([]string(nil), s.enabledTargets...)
			
			// Clear lastAttachedTarget if it's no longer enabled
			if s.lastAttachedTarget != "" {
				isLastAttachedEnabled := false
				for _, enabled := range enabledTargets {
					if strings.EqualFold(enabled, s.lastAttachedTarget) {
						isLastAttachedEnabled = true
						break
					}
				}
				if !isLastAttachedEnabled {
					s.lastAttachedTarget = ""
				}
			}
			
			s.currentTarget = ""
			s.lastWinsnapMinimized = false
			s.switchRollbackFrom = ""
			s.switchRollbackTo = ""
			s.switchRollbackUntil = time.Time{}
			s.wakeAttachedGuardUntil = time.Time{}
			// Preserve lastSwitchAt to prevent rapid switching
			// s.lastSwitchAt = time.Time{}
			s.lastAttachedRescanAt = time.Time{}
			s.attachedRescanCandidate = ""
			s.attachedRescanHits = 0
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
			return
		}
		if frontFound && frontTarget != "" {
			if strings.EqualFold(frontTarget, currentTarget) {
				now := time.Now()
				doRescan := false
				s.mu.Lock()
				if s.lastAttachedRescanAt.IsZero() || now.Sub(s.lastAttachedRescanAt) >= attachedLowFreqRescanInterval {
					s.lastAttachedRescanAt = now
					doRescan = true
				}
				s.mu.Unlock()
				if doRescan {
					// First, verify currentTarget is still enabled before any re-attach
					isCurrentTargetEnabled := false
					for _, enabled := range enabledTargets {
						if strings.EqualFold(enabled, currentTarget) {
							isCurrentTargetEnabled = true
							break
						}
					}
					if !isCurrentTargetEnabled {
						// Current target is no longer enabled, skip rescan
						return
					}

					// Low-frequency runtime consistency check: if state says we are attached
					// to currentTarget but runtime geometry says target is obscured/non-adjacent,
					// force a re-attach to currentTarget to realign controller and UI state.
					// To avoid acting on a single mis-detection, we require multiple consecutive
					// obscured/non-adjacent results before triggering re-attach.
					if obscured, oerr := winsnap.IsTargetObscured(w, currentTarget); oerr == nil {
						s.mu.Lock()
						if obscured {
							if strings.EqualFold(s.obscuredCheckTarget, currentTarget) {
								s.obscuredCheckHits++
							} else {
								s.obscuredCheckTarget = currentTarget
								s.obscuredCheckHits = 1
							}
						} else {
							// Reset counter when target is not obscured.
							if strings.EqualFold(s.obscuredCheckTarget, currentTarget) {
								s.obscuredCheckHits = 0
							} else {
								s.obscuredCheckTarget = currentTarget
								s.obscuredCheckHits = 0
							}
						}
						hits := s.obscuredCheckHits
						canSwitch := s.lastSwitchAt.IsZero() || now.Sub(s.lastSwitchAt) >= attachedMinSwitchInterval
						attachingLocked := s.attachingLock
						s.mu.Unlock()

						if obscured {
							// Only re-attach when obscured has been observed for enough consecutive checks.
							if hits < attachedLowFreqRescanSwitchHits {
								return
							}
							if canSwitch && !attachingLocked {
								s.attachTo(w, currentTarget)
							}
							return
						}
					}

					// Only check z-order if frontTarget is still currentTarget
					// This prevents switching to other apps when currentTarget is foreground
					// The rescan is only for detecting if currentTarget is obscured, not for switching to other apps
					// If we want to switch to another app, it should be detected by frontTarget change, not by z-order
					// So we skip the z-order check here to prevent flickering
					// Reset candidate when frontTarget matches currentTarget
					s.mu.Lock()
					s.attachedRescanCandidate = ""
					s.attachedRescanHits = 0
					s.mu.Unlock()
				}
				return
			}
			now := time.Now()
			s.mu.Lock()
			canSwitch := s.lastSwitchAt.IsZero() || now.Sub(s.lastSwitchAt) >= attachedMinSwitchInterval
			attachingLocked := s.attachingLock

			// Require frontTarget to be stable for multiple consecutive steps before switching.
			// This prevents brief foreground flickers (e.g. Feishu stealing focus for a moment
			// under DingTalk) from causing the snap window to rapidly jump between targets.
			if strings.EqualFold(s.attachedRescanCandidate, frontTarget) {
				s.attachedRescanHits++
			} else {
				s.attachedRescanCandidate = frontTarget
				s.attachedRescanHits = 1
			}
			hits := s.attachedRescanHits
			s.mu.Unlock()

			if !canSwitch || attachingLocked {
				return
			}
			// Only switch when the same frontTarget has been observed consistently enough times.
			if hits < attachedLowFreqRescanSwitchHits {
				return
			}
			s.attachTo(w, frontTarget)
			return
		}
		// Keep current attachment when foreground is not a target app.
		return
	}

	// In non-attached state (standalone/hidden), check switch interval before attaching
	if frontFound && frontTarget != "" {
		now := time.Now()
		s.mu.Lock()
		canSwitch := s.lastSwitchAt.IsZero() || now.Sub(s.lastSwitchAt) >= attachedMinSwitchInterval
		attachingLocked := s.attachingLock
		s.mu.Unlock()
		if !canSwitch || attachingLocked {
			return
		}
		s.attachTo(w, frontTarget)
		return
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

	// Check switch interval before attaching to prevent rapid switching
	now := time.Now()
	s.mu.Lock()
	canSwitch := s.lastSwitchAt.IsZero() || now.Sub(s.lastSwitchAt) >= attachedMinSwitchInterval
	attachingLocked := s.attachingLock
	s.mu.Unlock()
	if !canSwitch || attachingLocked {
		return
	}
	s.attachTo(w, target)
}

func (s *SnapService) isDragGuardActive() bool {
	v, ok := settings.GetValue(snapDragGuardUntilKey)
	if !ok {
		return false
	}

	untilMs, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64)
	if err != nil || untilMs <= 0 {
		return false
	}

	return time.Now().UnixMilli() < untilMs
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
	enabledTargets := append([]string(nil), s.enabledTargets...)

	// Remember the last attached target for wake restoration.
	// This allows WakeWindow to restore attached state instead of going standalone.
	// Only remember if it's still enabled.
	if oldState == SnapStateAttached && s.currentTarget != "" {
		isCurrentTargetEnabled := false
		for _, enabled := range enabledTargets {
			if strings.EqualFold(enabled, s.currentTarget) {
				isCurrentTargetEnabled = true
				break
			}
		}
		if isCurrentTargetEnabled {
			s.lastAttachedTarget = s.currentTarget
		} else {
			// Clear lastAttachedTarget if it's no longer enabled
			s.lastAttachedTarget = ""
		}
	}

	s.currentTarget = ""
	s.lastWinsnapMinimized = false
	s.switchRollbackFrom = ""
	s.switchRollbackTo = ""
	s.switchRollbackUntil = time.Time{}
	s.wakeAttachedGuardUntil = time.Time{}
	// Don't clear lastSwitchAt here - preserve switch interval to prevent rapid switching
	// s.lastSwitchAt = time.Time{}
	s.lastAttachedRescanAt = time.Time{}
	s.attachedRescanCandidate = ""
	s.attachedRescanHits = 0
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
	s.attachMu.Lock()
	defer s.attachMu.Unlock()

	// Set attaching lock to prevent concurrent attaching
	s.mu.Lock()
	if s.attachingLock {
		// Already attaching, skip
		s.mu.Unlock()
		return
	}
	s.attachingLock = true
	prevTarget := s.currentTarget
	prevState := s.status.State
	s.mu.Unlock()

	// Ensure lock is released even if attach fails
	defer func() {
		s.mu.Lock()
		s.attachingLock = false
		s.mu.Unlock()
	}()

	// Re-read enabledTargets from settings to ensure we have the latest list
	// This fixes the issue where disabling a snap app toggle doesn't immediately stop attaching
	_, enabledTargetsFromSettings := readSnapTargetsFromSettings()

	// Check if targetProcess is still enabled before attaching
	isEnabled := false
	for _, enabled := range enabledTargetsFromSettings {
		if strings.EqualFold(enabled, targetProcess) {
			isEnabled = true
			break
		}
	}
	if !isEnabled {
		// Target is no longer enabled, don't attach
		return
	}

	// Check if target is currently foreground
	frontTarget, frontFound, _ := winsnap.FrontMostTargetProcessName(enabledTargetsFromSettings)
	isForeground := frontFound && strings.EqualFold(frontTarget, targetProcess)

	// Get z-order info (top-most visible target)
	topTarget, topFound, _ := winsnap.TopMostVisibleProcessName(enabledTargetsFromSettings)
	zOrderInfo := "unknown"
	if topFound && topTarget != "" {
		zOrderInfo = topTarget
		if strings.EqualFold(topTarget, targetProcess) {
			zOrderInfo = targetProcess + " (top)"
		}
	}

	// Get switch lock time info
	s.mu.Lock()
	lastSwitchAt := s.lastSwitchAt
	now := time.Now()
	switchLockTimeRemaining := time.Duration(0)
	if !lastSwitchAt.IsZero() {
		elapsed := now.Sub(lastSwitchAt)
		if elapsed < attachedMinSwitchInterval {
			switchLockTimeRemaining = attachedMinSwitchInterval - elapsed
		}
	}
	s.mu.Unlock()

	// Print switch debug info
	if s.app != nil && s.app.Logger != nil {
		s.app.Logger.Info("SnapService switch attach",
			"state", prevState,
			"targetProcess", targetProcess,
			"isForeground", isForeground,
			"enabledTargets", enabledTargetsFromSettings,
			"zOrderInfo", zOrderInfo,
			"switchLockTimeRemaining", switchLockTimeRemaining,
		)
	}

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
		// Verify runtime attachment consistency. If winsnap is not adjacent to
		// the supposed target, force a controller rebuild to recover.
		// To reduce false positives from a single mis-detection, we require multiple
		// consecutive obscured/non-adjacent results before tearing down the controller.
		obscured, err := winsnap.IsTargetObscured(w, targetProcess)
		if err == nil {
			s.mu.Lock()
			if obscured {
				if strings.EqualFold(s.obscuredCheckTarget, targetProcess) {
					s.obscuredCheckHits++
				} else {
					s.obscuredCheckTarget = targetProcess
					s.obscuredCheckHits = 1
				}
			} else {
				// Reset counter when target is confirmed adjacent (not obscured).
				if strings.EqualFold(s.obscuredCheckTarget, targetProcess) {
					s.obscuredCheckHits = 0
				} else {
					s.obscuredCheckTarget = targetProcess
					s.obscuredCheckHits = 0
				}
			}
			hits := s.obscuredCheckHits
			s.mu.Unlock()

			// If target is not obscured/non-adjacent, or obscured but not yet
			// stable for enough checks, keep current controller as-is.
			if !obscured || hits < attachedLowFreqRescanSwitchHits {
				return
			}
		}
		s.mu.Lock()
		if s.ctrl != nil {
			_ = s.ctrl.Stop()
			s.ctrl = nil
		}
		s.mu.Unlock()
	} else {
		// Stop current controller (if any) before switching target.
		if s.ctrl != nil {
			_ = s.ctrl.Stop()
			s.ctrl = nil
		}
		s.mu.Unlock()
	}

	// Restore winsnap if it was minimized (e.g. by Win+D); when a target becomes visible again, show the snap window.
	if err := winsnap.EnsureWindowVisible(w); err == winsnap.ErrWinsnapWindowInvalid {
		s.handleWinsnapWindowInvalid(targetProcess)
		return
	}

	c, err := winsnap.AttachRightOfProcess(winsnap.AttachOptions{
		TargetProcessName: targetProcess,
		Gap:               snapGap(),
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
	now = time.Now() // Update now to current time for lastSwitchAt
	s.lastSwitchAt = now // Always update lastSwitchAt when attaching to enforce min interval
	s.lastAttachedRescanAt = time.Time{}
	s.attachedRescanCandidate = ""
	s.attachedRescanHits = 0
	if prevTarget != "" && !strings.EqualFold(prevTarget, targetProcess) {
		s.wakeAttachedGuardUntil = now.Add(wakeAttachedGuardAfterSwitch)
	} else {
		s.wakeAttachedGuardUntil = time.Time{}
	}
	// Clear rollback fields - rollback logic appears to be unused/removed
	// Keeping fields for backward compatibility but clearing them
	s.switchRollbackFrom = ""
	s.switchRollbackTo = ""
	s.switchRollbackUntil = time.Time{}
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
					Gap:               snapGap(),
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
	for _, app := range loadCustomSnapAppsFromSettings() {
		if app.ProcessName == "" {
			continue
		}
		items = append(items, item{
			key:     customSnapSettingKey(app.ID),
			targets: []string{app.ProcessName},
		})
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
			return []string{"飞书", "Feishu", "feishu", "Lark", "lark", "com.bytedance.feishu", "com.bytedance.Lark", "com.electron.lark"}
		case "snap_douyin":
			return []string{"抖音", "Douyin", "douyin"}
		default:
			return nil
		}
	}
}

// snapKeyForTarget returns the settings key (e.g., "snap_dingtalk") for a given target process name.
// Uses case-insensitive comparison because Windows process names returned by
// QueryFullProcessImageName may differ in case from our hardcoded target names
// (e.g., "douyin.exe" vs "Douyin.exe").
func snapKeyForTarget(targetProcess string) string {
	keys := []string{"snap_wechat", "snap_wecom", "snap_qq", "snap_dingtalk", "snap_feishu", "snap_douyin"}
	for _, key := range keys {
		targets := snapTargetsForKey(key)
		for _, t := range targets {
			if strings.EqualFold(t, targetProcess) {
				return key
			}
		}
	}
	for _, app := range loadCustomSnapAppsFromSettings() {
		if app.ProcessName == "" {
			continue
		}
		if strings.EqualFold(app.ProcessName, targetProcess) {
			return customSnapSettingKey(app.ID)
		}
	}
	return ""
}

func customSnapSettingKey(id string) string {
	trimmed := strings.TrimSpace(id)
	if trimmed == "" {
		return ""
	}
	return snapCustomKeyPrefix + trimmed
}

func loadCustomSnapAppsFromSettings() []customSnapAppConfig {
	raw, ok := settings.GetValue(snapCustomAppsSettingKey)
	if !ok || strings.TrimSpace(raw) == "" {
		return nil
	}
	var apps []customSnapAppConfig
	if err := json.Unmarshal([]byte(raw), &apps); err != nil {
		return nil
	}

	// Filter out any custom apps that duplicate built-in snap targets (e.g. older configs
	// created before the UI started blocking built-in apps from being added as "custom").
	// Otherwise, disabling a built-in toggle (like DingTalk) may still leave an enabled
	// custom entry for the same process name, and the winsnap window will keep attaching.
	builtIn := make(map[string]struct{}, 32)
	for _, k := range []string{"snap_wechat", "snap_wecom", "snap_qq", "snap_dingtalk", "snap_feishu", "snap_douyin"} {
		for _, t := range snapTargetsForKey(k) {
			tt := strings.ToLower(strings.TrimSpace(t))
			if tt == "" {
				continue
			}
			builtIn[tt] = struct{}{}
		}
	}

	out := make([]customSnapAppConfig, 0, len(apps))
	seen := make(map[string]struct{}, len(apps))
	for _, app := range apps {
		id := strings.TrimSpace(app.ID)
		processName := strings.TrimSpace(app.ProcessName)
		if id == "" || processName == "" {
			continue
		}
		if _, dup := builtIn[strings.ToLower(processName)]; dup {
			continue
		}
		key := strings.ToLower(id) + "##" + strings.ToLower(processName)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, customSnapAppConfig{
			ID:          id,
			Name:        strings.TrimSpace(app.Name),
			ProcessName: processName,
			Icon:        strings.TrimSpace(app.Icon),
		})
	}
	return out
}

// getClickSettingsForTarget returns the configured click settings for a target process
// Returns noClick (skip mouse click), offsetX, offsetY
func getClickSettingsForTarget(targetProcess string) (noClick bool, offsetX, offsetY int) {
	key := snapKeyForTarget(targetProcess)
	if key == "" {
		return false, 0, 0
	}
	// Setting key format: snap_[app]_no_click, snap_[app]_click_offset_x/y
	// Douyin defaults to no-click mode; others default to click mode.
	noClick = settings.GetBool(key+"_no_click", defaultNoClickForKey(key))
	offsetX = settings.GetInt(key+"_click_offset_x", 0)
	offsetY = settings.GetInt(key+"_click_offset_y", 0)
	// If not configured (0 or empty), fall back to per-app defaults to match frontend UX.
	// This is important on macOS where the click implementation otherwise falls back to a generic value.
	if offsetY <= 0 {
		offsetY = defaultClickOffsetYForKey(key)
	}
	return noClick, offsetX, offsetY
}

// defaultNoClickForKey returns whether the app defaults to no-click mode.
// Douyin defaults to no-click (input keeps focus automatically);
// all other apps default to click mode.
//
// Keep this consistent with frontend defaults in SnapSettings.vue.
func defaultNoClickForKey(key string) bool {
	switch key {
	case "snap_douyin":
		return true
	default:
		return false
	}
}

// defaultClickOffsetYForKey returns the default click Y offset (pixels from bottom)
// used to focus the input box in the target app.
//
// Keep this consistent with frontend defaults in SnapSettings.vue.
func defaultClickOffsetYForKey(key string) int {
	switch key {
	case "snap_feishu", "snap_douyin":
		return 50
	default:
		return 120
	}
}
