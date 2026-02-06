package textselection

import (
	"runtime"
	"strings"
	"sync"
	"time"

	"willchat/internal/services/settings"
	"willchat/internal/services/windows"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

const (
	// WindowTextSelection is the name of the text selection popup window.
	WindowTextSelection = "textselection"
	// SettingKeyTextSelectionEnabled is the settings key for enabling text selection.
	// Must match the key used in frontend: ToolsSettings.vue
	SettingKeyTextSelectionEnabled = "enable_selection_search"
)

// TextSelectionService provides text selection popup functionality.
// It uses mouse hook mode: detect selection drag -> copy to clipboard -> show popup.
type TextSelectionService struct {
	mu sync.RWMutex

	app        *application.App
	mainWindow *application.WebviewWindow
	popWindow  *application.WebviewWindow
	popOptions application.WebviewWindowOptions

	// getSnapState returns current snap state. This is injected at construction
	// time to avoid exposing SnapService in Wails bindings.
	getSnapState func() windows.SnapState

	// wakeSnapWindow wakes the snap window (brings it to front).
	// Used when snap window is visible (attached or standalone) and user clicks text selection popup.
	wakeSnapWindow func()

	// Last selection action (button click) payload.
	// Used as a fallback for winsnap window to pull the latest action on startup
	// (avoids losing the first event when the winsnap window is created on-demand).
	lastActionID   int64
	lastActionText string

	// Currently selected text
	selectedText string
	// Popup position and size
	popX, popY int
	popWidth   int
	popHeight  int
	// Whether popup is currently shown (logical state).
	popupActive bool

	// Original app PID (used to wake original app and execute copy on button click)
	originalAppPid int32

	// Auto-hide timer
	hideTimer *time.Timer

	// Mouse hook watcher
	mouseHookWatcher *MouseHookWatcher

	// Click outside watcher
	clickOutsideWatcher *ClickOutsideWatcher

	// Whether the service is enabled
	enabled bool

	// Last button click time (for debouncing duplicate clicks)
	lastClickTime time.Time
}

// New creates a new TextSelectionService.
func New() *TextSelectionService {
	return NewWithSnapStateGetter(nil)
}

// NewWithSnapStateGetter creates a new TextSelectionService with an optional
// snap state getter. If nil, snap state is treated as stopped.
func NewWithSnapStateGetter(getter func() windows.SnapState) *TextSelectionService {
	return NewWithSnapCallbacks(getter, nil)
}

// NewWithSnapCallbacks creates a new TextSelectionService with snap state getter
// and snap window wake callback. Both can be nil.
func NewWithSnapCallbacks(getState func() windows.SnapState, wakeSnap func()) *TextSelectionService {
	return &TextSelectionService{
		popWidth:  140,
		popHeight: 50,
		getSnapState: func() windows.SnapState {
			if getState == nil {
				return windows.SnapStateStopped
			}
			return getState()
		},
		wakeSnapWindow: wakeSnap,
	}
}

// Attach is called by bootstrap after creating windows.
func (s *TextSelectionService) Attach(app *application.App, mainWindow *application.WebviewWindow, popOptions application.WebviewWindowOptions) {
	s.mu.Lock()
	s.app = app
	s.mainWindow = mainWindow
	s.popOptions = popOptions
	s.mu.Unlock()

	// Listen for frontend text selection events
	app.Event.On("text-selection:show", func(e *application.CustomEvent) {
		data, ok := e.Data.(map[string]any)
		if !ok {
			return
		}
		text, _ := data["text"].(string)
		x, _ := data["x"].(float64)
		y, _ := data["y"].(float64)
		if text == "" {
			return
		}
		s.Show(text, int(x), int(y))
	})

	app.Event.On("text-selection:hide", func(_ *application.CustomEvent) {
		s.Hide()
	})

	// Listen for click outside events (triggered by hook thread, executed in main thread)
	app.Event.On("text-selection:click-outside", func(_ *application.CustomEvent) {
		s.Hide()
	})

	// Listen for popup button click events
	app.Event.On("text-selection:button-click", func(_ *application.CustomEvent) {
		s.handleButtonClick()
	})

	// Listen for system-level text selection events (triggered by hook thread, executed in main thread)
	app.Event.On("text-selection:show-at-screen-pos", func(e *application.CustomEvent) {
		data, ok := e.Data.(map[string]any)
		if !ok {
			return
		}
		text, _ := data["text"].(string)
		x, _ := data["x"].(int)
		y, _ := data["y"].(int)
		if text == "" {
			return
		}
		s.showAtScreenPosInternal(text, x, y)
	})

	// Start click outside watcher
	s.startClickOutsideWatcher()
}

// SyncFromSettings reads the text selection setting and enables/disables the service.
func (s *TextSelectionService) SyncFromSettings() (bool, error) {
	enabled := settings.GetBool(SettingKeyTextSelectionEnabled, false)
	s.mu.Lock()
	wasEnabled := s.enabled
	s.enabled = enabled
	app := s.app
	s.mu.Unlock()

	if app != nil {
		app.Logger.Info("TextSelectionService.SyncFromSettings", "enabled", enabled, "wasEnabled", wasEnabled)
	}

	if enabled && !wasEnabled {
		if app != nil {
			app.Logger.Info("TextSelectionService: starting mouse hook watcher")
		}
		s.startWatcher()
	} else if !enabled && wasEnabled {
		if app != nil {
			app.Logger.Info("TextSelectionService: stopping mouse hook watcher")
		}
		s.stopWatcher()
	}

	return enabled, nil
}

// IsEnabled returns whether the service is enabled.
func (s *TextSelectionService) IsEnabled() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.enabled
}

// startClickOutsideWatcher starts the click outside watcher.
func (s *TextSelectionService) startClickOutsideWatcher() {
	s.clickOutsideWatcher = NewClickOutsideWatcher(func(x, y int32) {
		// Hide popup when clicked outside
		// Note: must execute window operations in main thread, so trigger via event
		if s.app != nil {
			s.app.Event.Emit("text-selection:click-outside", nil)
		}
	})
	go s.clickOutsideWatcher.Start()
}

// startWatcher starts the mouse hook watcher.
func (s *TextSelectionService) startWatcher() {
	// Old callback (copy-then-show mode) - kept for compatibility but not used
	_ = func(text string, x, y int32) {
		text = strings.TrimSpace(text)
		if text == "" {
			return
		}
		s.ShowAtScreenPos(text, int(x), int(y))
	}

	// Callback when drag starts (hide popup if click is not inside popup)
	// frontAppPid: the PID of the frontmost app at the moment of mouseDown (before our app gets activated)
	onDragStartWithPid := func(mouseX, mouseY int32, frontAppPid int32) {
		// Check if click is inside popup area
		s.mu.RLock()
		popX := s.popX
		popY := s.popY
		popW := s.popWidth
		popH := s.popHeight
		w := s.popWindow
		active := s.popupActive
		s.mu.RUnlock()

		// If popup doesn't exist or is not visible, no need to process
		if w == nil || !active {
			return
		}

		// On macOS coordinates are physical pixels, need to convert popup size to physical pixels
		popWPx := popW
		popHPx := popH
		marginPx := int32(15)
		if runtime.GOOS == "darwin" {
			scale := getDPIScale()
			popWPx = int(float64(popW) * scale)
			popHPx = int(float64(popH) * scale)
			marginPx = int32(15 * scale)
		}

		// Check if click is inside popup area (with some tolerance)
		inPopup := mouseX >= int32(popX)-marginPx && mouseX <= int32(popX+popWPx)+marginPx &&
			mouseY >= int32(popY)-marginPx && mouseY <= int32(popY+popHPx)+marginPx

		// If click is inside popup, trigger button click (matching demo project behavior)
		if inPopup {
			// On macOS, save the frontmost app PID before our app gets activated by the click.
			// This is critical for lazy-copy mode: we need to re-activate this app to copy text.
			if runtime.GOOS == "darwin" && frontAppPid > 0 {
				s.mu.Lock()
				s.originalAppPid = frontAppPid
				s.mu.Unlock()
			}
			go s.handleButtonClick()
			return
		}

		// Click is outside popup, hide popup
		if s.app != nil {
			s.app.Event.Emit("text-selection:hide", nil)
		}
	}

	// New mode: show popup only (no clipboard copy), copy on button click.
	// This avoids polluting the user's clipboard during text selection.
	showPopupOnly := func(mouseX, mouseY int32, originalAppPid int32) {
		s.mu.Lock()
		s.selectedText = ""        // Clear text - will be fetched on button click
		s.originalAppPid = originalAppPid // Record original app PID for later copy
		s.mu.Unlock()

		s.showPopupOnlyAtScreenPos(int(mouseX), int(mouseY))
	}

	s.mu.Lock()
	// Use showPopupOnly mode: detect drag -> show popup (no copy) -> copy on button click
	s.mouseHookWatcher = NewMouseHookWatcher(nil, onDragStartWithPid, showPopupOnly)
	s.mu.Unlock()

	go s.mouseHookWatcher.Start()
}

// stopWatcher stops the mouse hook watcher.
func (s *TextSelectionService) stopWatcher() {
	s.mu.Lock()
	if s.mouseHookWatcher != nil {
		s.mouseHookWatcher.Stop()
		s.mouseHookWatcher = nil
	}
	s.mu.Unlock()
}

// Show shows the popup at the specified position (for in-app use, coordinates relative to main window content area).
func (s *TextSelectionService) Show(text string, clientX, clientY int) {
	if !s.IsEnabled() {
		return
	}
	s.mu.Lock()
	s.selectedText = text
	s.originalAppPid = 0 // In-app selection, reset PID, use existing text
	app := s.app
	mainW := s.mainWindow

	// Cancel previous hide timer
	if s.hideTimer != nil {
		s.hideTimer.Stop()
		s.hideTimer = nil
	}
	s.mu.Unlock()

	if app == nil || mainW == nil {
		return
	}

	// Get main window position (screen coordinates)
	winX, winY := mainW.Position()
	_, _ = mainW.Size() // Get but don't use

	var finalX, finalY int

	if runtime.GOOS == "darwin" {
		// macOS uses the same coordinate system as other apps
		scale := getDPIScale()
		popWidthPx := int(float64(s.popWidth) * scale)
		popHeightPx := int(float64(s.popHeight) * scale)
		offsetPx := int(10 * scale)

		// macOS window title bar height (points)
		titleBarHeight := 28
		titleBarHeightPx := int(float64(titleBarHeight) * scale)

		// Convert DOM clientX/clientY to screen pixel coordinates
		clientXPx := int(float64(clientX) * scale)
		clientYPx := int(float64(clientY) * scale)

		// Screen coordinates = window position + title bar height + client area offset
		screenX := int(float64(winX)*scale) + clientXPx
		screenY := int(float64(winY)*scale) + titleBarHeightPx + clientYPx

		// Show popup above mouse
		finalX = screenX - popWidthPx/2
		finalY = screenY - popHeightPx - offsetPx
	} else {
		// Windows: standard screen coordinates (Y increases downward)
		titleBarHeight := 32 // Windows standard title bar height (pixels)

		screenX := winX + clientX
		screenY := winY + titleBarHeight + clientY

		finalX = screenX - s.popWidth/2
		finalY = screenY - s.popHeight - 10

		// Ensure popup doesn't exceed screen bounds (multi-monitor aware)
		finalX, finalY = clampToWorkArea(finalX, finalY, s.popWidth, s.popHeight, screenX, screenY)
	}

	// Update popup's recorded position
	s.mu.Lock()
	s.popX = finalX
	s.popY = finalY
	s.mu.Unlock()

	s.showPopupAt(finalX, finalY)
}

// ShowAtScreenPos shows the popup at the specified screen position (for system-level monitoring).
// Note: this method ensures window operations are executed in main thread via event system.
func (s *TextSelectionService) ShowAtScreenPos(text string, screenX, screenY int) {
	if !s.IsEnabled() {
		return
	}
	s.mu.Lock()
	app := s.app
	s.mu.Unlock()

	if app == nil {
		return
	}

	// Execute window operations in main thread via event system
	app.Event.Emit("text-selection:show-at-screen-pos", map[string]any{
		"text": text,
		"x":    screenX,
		"y":    screenY,
	})
}

// showAtScreenPosInternal is the internal method that executes actual show operation in main thread.
func (s *TextSelectionService) showAtScreenPosInternal(text string, screenX, screenY int) {
	s.mu.Lock()
	s.selectedText = text
	s.originalAppPid = -1 // Mark as "other app selection" (not in-app selection)
	s.popX = screenX
	s.popY = screenY
	app := s.app

	// Cancel previous hide timer
	if s.hideTimer != nil {
		s.hideTimer.Stop()
		s.hideTimer = nil
	}
	s.mu.Unlock()

	if app == nil {
		return
	}

	app.Logger.Info("TextSelectionService.showAtScreenPosInternal", "text", text[:min(len(text), 20)], "screenX", screenX, "screenY", screenY)

	var finalX, finalY int

	if runtime.GOOS == "darwin" {
		// macOS coordinate handling (matching demo project approach):
		// - Mouse hook (CGEventTap) returns physical pixels
		// - Use physical pixels consistently for both SetPosition and click detection
		// Note: Despite Wails docs saying SetPosition uses logical points,
		//       the demo project uses physical pixels and it works correctly.
		scale := getDPIScale()

		// screenX/screenY are in physical pixels
		// Calculate popup position in pixels
		popWidthPx := int(float64(s.popWidth) * scale)
		popHeightPx := int(float64(s.popHeight) * scale)
		offsetPx := int(10 * scale)

		// Use pixel coordinates directly (same as demo project)
		finalX = screenX - popWidthPx/2
		finalY = screenY - popHeightPx - offsetPx
	} else {
		// Windows: use logical pixels
		finalX = screenX - s.popWidth/2
		// Above mouse = mouseY - height - offset (Y increases downward)
		finalY = screenY - s.popHeight - 10

		// Ensure popup doesn't exceed screen bounds (multi-monitor aware)
		finalX, finalY = clampToWorkArea(finalX, finalY, s.popWidth, s.popHeight, screenX, screenY)
	}

	s.showPopupAt(finalX, finalY)
}

// showPopupAt shows the popup at the specified screen position.
// On macOS, x/y are in physical pixels. On Windows, they are in logical pixels.
func (s *TextSelectionService) showPopupAt(x, y int) {
	s.mu.Lock()
	s.popX = x
	s.popY = y
	s.popupActive = true
	popW := s.popWidth
	popH := s.popHeight
	s.mu.Unlock()

	// Set window position
	s.ensurePopWindow(x, y)

	// Update click outside watcher's popup area
	// On macOS, both x/y and click detection use physical pixels
	if s.clickOutsideWatcher != nil {
		rectW := popW
		rectH := popH
		if runtime.GOOS == "darwin" {
			// Convert popup size to physical pixels
			scale := getDPIScale()
			rectW = int(float64(popW) * scale)
			rectH = int(float64(popH) * scale)
		}
		s.clickOutsideWatcher.SetPopupRect(int32(x), int32(y), int32(rectW), int32(rectH))
	}
}

// showPopupOnlyAtScreenPos shows the popup at the specified screen position WITHOUT copying text.
// Text will be copied when the user clicks the popup button.
// This is the new "lazy copy" mode that avoids polluting the user's clipboard during selection.
func (s *TextSelectionService) showPopupOnlyAtScreenPos(screenX, screenY int) {
	if !s.IsEnabled() {
		return
	}
	s.mu.Lock()
	app := s.app
	// Cancel previous hide timer
	if s.hideTimer != nil {
		s.hideTimer.Stop()
		s.hideTimer = nil
	}
	s.mu.Unlock()

	if app == nil {
		return
	}

	var finalX, finalY int

	if runtime.GOOS == "darwin" {
		scale := getDPIScale()
		popWidthPx := int(float64(s.popWidth) * scale)
		popHeightPx := int(float64(s.popHeight) * scale)
		offsetPx := int(10 * scale)

		finalX = screenX - popWidthPx/2
		finalY = screenY - popHeightPx - offsetPx
	} else {
		finalX = screenX - s.popWidth/2
		finalY = screenY - s.popHeight - 10

		// Ensure popup doesn't exceed screen bounds (multi-monitor aware)
		finalX, finalY = clampToWorkArea(finalX, finalY, s.popWidth, s.popHeight, screenX, screenY)
	}

	s.showPopupAt(finalX, finalY)
}

// Hide hides the popup.
// Safely handles the case when the window has been closed/released.
// On Windows: moves window off-screen (w.Hide() causes WebView2 Focus error).
// On macOS: uses w.Hide() for reliable hiding (moving off-screen may still be visible).
func (s *TextSelectionService) Hide() {
	s.mu.Lock()
	if s.hideTimer != nil {
		s.hideTimer.Stop()
		s.hideTimer = nil
	}
	w := s.popWindow
	s.popX = -9999
	s.popY = -9999
	s.popupActive = false
	// Don't clear popWindow reference, reuse window
	s.mu.Unlock()

	if w != nil {
		// Check if window is still valid before calling SetPosition
		nativeHandle := w.NativeWindow()
		if nativeHandle != nil && uintptr(nativeHandle) != 0 {
			if runtime.GOOS == "darwin" {
				// macOS: use Hide() for reliable hiding
				// Moving off-screen may still show window edge on some display configurations
				w.Hide()
			} else {
				// Windows: Don't use w.Hide(), because Wails Hide() internally calls Focus,
				// causing WebView2 error. Move window off-screen to "hide" instead.
				w.SetPosition(-9999, -9999)
			}
		} else {
			// Window has been closed, clear the reference
			s.mu.Lock()
			if s.popWindow == w {
				s.popWindow = nil
			}
			s.mu.Unlock()
		}
	}

	// Clear click outside watcher's popup area
	if s.clickOutsideWatcher != nil {
		s.clickOutsideWatcher.ClearPopupRect()
	}
}

// handleButtonClick handles the popup button click event.
// Includes debounce logic to prevent duplicate triggers within 500ms.
//
// Product requirement:
// - Text selection popup should always interact with winsnap window (never main window).
// - If winsnap window does not exist, create it and wake it as a standalone window.
// - Copy text on button click (lazy copy mode) to avoid polluting clipboard during selection.
func (s *TextSelectionService) handleButtonClick() map[string]any {
	s.mu.Lock()
	// Debounce: ignore clicks within 500ms of the last click
	now := time.Now()
	if now.Sub(s.lastClickTime) < 500*time.Millisecond {
		s.mu.Unlock()
		return map[string]any{"error": "debounced"}
	}
	s.lastClickTime = now

	text := s.selectedText
	originalAppPid := s.originalAppPid
	app := s.app
	wakeSnapWindow := s.wakeSnapWindow
	s.mu.Unlock()

	if app == nil {
		return map[string]any{"error": "app is nil"}
	}

	// If text is empty, we're in "lazy copy" mode - need to copy now.
	// The popup was shown without copying; now we simulate Ctrl+C/Cmd+C to get the selected text.
	// This works because the popup doesn't steal focus, so the original app still has the selection.
	if text == "" && originalAppPid != 0 {
		text = s.copyAndGetSelectedText()
		if text == "" {
			// No text could be copied, hide popup and return
			go s.Hide()
			return map[string]any{"error": "no text selected"}
		}
		// Update selectedText for GetLastButtonAction fallback
		s.mu.Lock()
		s.selectedText = text
		s.mu.Unlock()
	}

	if text == "" {
		return map[string]any{"error": "no text selected"}
	}

	// Use Unix milliseconds to keep JS number precision (safe integer).
	actionID := time.Now().UnixMilli()
	s.mu.Lock()
	s.lastActionID = actionID
	s.lastActionText = text
	s.mu.Unlock()

	// Product requirement: always interact with winsnap (never main window).
	// Ensure and wake the winsnap window first (it may be created on-demand).
	if wakeSnapWindow != nil {
		wakeSnapWindow()
	}

	// Send text to winsnap window. Payload contains an id for deduplication.
	app.Event.Emit("text-selection:send-to-snap", map[string]any{
		"id":   actionID,
		"text": text,
	})

	// Delay hide popup
	go func() {
		time.Sleep(150 * time.Millisecond)
		s.Hide()
	}()

	return map[string]any{
		"id":   actionID,
		"text": text,
	}
}

// copyAndGetSelectedText simulates Ctrl+C/Cmd+C and reads the clipboard.
// This is used in "lazy copy" mode where we only copy when the user clicks the popup.
//
// On Windows: The popup doesn't steal focus (WS_EX_NOACTIVATE), so the original app
// still has the selection and receives the Ctrl+C.
//
// On macOS: Clicking the popup activates our app, so we must first re-activate the
// original app (using the saved PID) before sending Cmd+C.
func (s *TextSelectionService) copyAndGetSelectedText() string {
	// On macOS, clicking the popup activates our app.
	// We need to re-activate the original app before sending Cmd+C.
	if runtime.GOOS == "darwin" {
		s.mu.RLock()
		pid := s.originalAppPid
		s.mu.RUnlock()

		if pid > 0 {
			// Re-activate the original app so it receives Cmd+C
			activateAppByPidDarwin(pid)
			// Wait for activation to complete
			time.Sleep(100 * time.Millisecond)
		}
	}

	// Simulate Ctrl+C (Windows) or Cmd+C (macOS)
	if runtime.GOOS == "darwin" {
		simulateCmdCDarwin()
	} else {
		simulateCtrlCWindows()
	}

	// Wait for clipboard to update (try multiple times with increasing delays)
	var newClipboard string
	for attempt := 1; attempt <= 3; attempt++ {
		time.Sleep(time.Duration(80*attempt) * time.Millisecond)

		if runtime.GOOS == "darwin" {
			newClipboard = getClipboardTextDarwin()
		} else {
			newClipboard = getClipboardTextWindows()
		}

		newClipboard = strings.TrimSpace(newClipboard)
		// Return clipboard content if not empty, even if it's the same as before.
		// This allows users to select the same text multiple times.
		if newClipboard != "" {
			return newClipboard
		}
	}

	// If clipboard is empty after multiple attempts, return empty
	return ""
}

// GetLastButtonAction returns the last button click action payload.
// This is used by the winsnap window as a fallback on startup to avoid missing the first event.
func (s *TextSelectionService) GetLastButtonAction() map[string]any {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return map[string]any{
		"id":   s.lastActionID,
		"text": s.lastActionText,
	}
}

// GetSelectedText returns the currently selected text.
func (s *TextSelectionService) GetSelectedText() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.selectedText
}

func (s *TextSelectionService) ensurePopWindow(x, y int) {
	s.mu.Lock()
	app := s.app
	w := s.popWindow
	opts := s.popOptions
	s.mu.Unlock()

	if app == nil {
		return
	}

	// Check if existing window is still valid (native handle not nil/0)
	// On macOS, NativeWindow() returns nil for closed windows
	// On Windows, NativeWindow() returns 0 for closed windows
	needCreate := w == nil
	if !needCreate {
		nativeHandle := w.NativeWindow()
		if nativeHandle == nil || uintptr(nativeHandle) == 0 {
			// Window has been closed or released, need to recreate
			needCreate = true
			s.mu.Lock()
			s.popWindow = nil
			s.mu.Unlock()
			w = nil
		}
	}

	// If window doesn't exist or is invalid, create new window
	if needCreate {
		opts.X = x
		opts.Y = y
		opts.InitialPosition = application.WindowXY
		opts.Hidden = true // Create hidden window first

		w = app.Window.NewWithOptions(opts)
		if w == nil {
			return
		}

		// Windows: prevent the popup from taking focus (WebView2 Focus may crash).
		tryConfigurePopupNoActivate(w)

		s.mu.Lock()
		s.popWindow = w
		s.mu.Unlock()

		// Listen for window closing event
		w.RegisterHook(events.Common.WindowClosing, func(_ *application.WindowEvent) {
			// Remove window subclass on Windows before window is destroyed
			if h := w.NativeWindow(); h != nil {
				removePopupSubclass(uintptr(h))
			}
			s.mu.Lock()
			if s.popWindow == w {
				s.popWindow = nil
			}
			s.mu.Unlock()
		})

		// macOS special handling: disable window tiling
		if runtime.GOOS == "darwin" {
			s.disableMacOSWindowTiling(w)
		}
	}

	// Set position and show (need to update position when reusing window)
	// Double-check window is still valid before operations
	nativeHandle := w.NativeWindow()
	if nativeHandle == nil || uintptr(nativeHandle) == 0 {
		// Window became invalid between creation and show, clear reference
		s.mu.Lock()
		if s.popWindow == w {
			s.popWindow = nil
		}
		s.mu.Unlock()
		return
	}

	w.SetPosition(x, y)
	w.Show()
	forcePopupTopMostNoActivate(w)
}

// disableMacOSWindowTiling disables window tiling management on macOS.
func (s *TextSelectionService) disableMacOSWindowTiling(w *application.WebviewWindow) {
	// macOS-specific implementation in separate file
}
