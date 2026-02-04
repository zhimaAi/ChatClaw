//go:build !windows && !darwin

package textselection

import (
	"sync"
)

// ClipboardWatcher placeholder implementation for non-Windows/macOS platforms.
type ClipboardWatcher struct {
	mu       sync.Mutex
	callback func(text string, x, y int32)
	closed   bool
	ready    chan struct{}
}

// NewClipboardWatcher creates a new clipboard watcher.
func NewClipboardWatcher(callback func(text string, x, y int32)) *ClipboardWatcher {
	return &ClipboardWatcher{
		callback: callback,
		ready:    make(chan struct{}),
	}
}

// Start starts the watcher (no-op on non-Windows/macOS platforms).
func (w *ClipboardWatcher) Start() error {
	close(w.ready)
	return nil
}

// Stop stops the watcher (no-op on non-Windows/macOS platforms).
func (w *ClipboardWatcher) Stop() {
	w.mu.Lock()
	w.closed = true
	w.mu.Unlock()
}

// GetCursorPos gets the current mouse position (returns 0,0 on non-Windows/macOS platforms).
func GetCursorPos() (x, y int32) {
	return 0, 0
}

// getDPIScale gets the DPI scale factor (returns 1.0 on non-Windows/macOS platforms).
func getDPIScale() float64 {
	return 1.0
}
