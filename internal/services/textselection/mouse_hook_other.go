//go:build !windows && !darwin

package textselection

// MouseHookWatcher placeholder implementation for non-Windows/macOS platforms.
type MouseHookWatcher struct {
	callback          func(text string, x, y int32)
	onDragStart       func(x, y int32)
	showPopupCallback func(x, y int32, originalAppPid int32)
}

// NewMouseHookWatcher creates a new mouse hook watcher (not supported on non-Windows/macOS platforms).
func NewMouseHookWatcher(
	callback func(text string, x, y int32),
	onDragStart func(x, y int32),
	showPopupCallback func(x, y int32, originalAppPid int32),
) *MouseHookWatcher {
	return &MouseHookWatcher{
		callback:          callback,
		onDragStart:       onDragStart,
		showPopupCallback: showPopupCallback,
	}
}

// Start starts the watcher (no-op on non-Windows/macOS platforms).
func (w *MouseHookWatcher) Start() error {
	return nil
}

// Stop stops the watcher (no-op on non-Windows/macOS platforms).
func (w *MouseHookWatcher) Stop() {
}

// Helper function stubs (macOS)
func activateAppByPidDarwin(pid int32) {}
func simulateCmdCDarwin()              {}
func getClipboardTextDarwin() string   { return "" }

// Helper function stubs (Windows)
func simulateCtrlCWindows()           {}
func getClipboardTextWindows() string { return "" }
