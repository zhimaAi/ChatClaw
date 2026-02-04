//go:build !windows && !darwin

package textselection

import (
	"sync"
)

// ClickOutsideWatcher placeholder implementation for non-Windows/macOS platforms.
type ClickOutsideWatcher struct {
	mu       sync.Mutex
	callback func(x, y int32)
	closed   bool
	ready    chan struct{}
}

// NewClickOutsideWatcher creates a new click outside watcher.
func NewClickOutsideWatcher(callback func(x, y int32)) *ClickOutsideWatcher {
	return &ClickOutsideWatcher{
		callback: callback,
		ready:    make(chan struct{}),
	}
}

// Start starts the watcher (no-op on non-Windows/macOS platforms).
func (w *ClickOutsideWatcher) Start() error {
	close(w.ready)
	return nil
}

// Stop stops the watcher (no-op on non-Windows/macOS platforms).
func (w *ClickOutsideWatcher) Stop() {
	w.mu.Lock()
	w.closed = true
	w.mu.Unlock()
}

// SetPopupRect sets the popup area (no-op on non-Windows/macOS platforms).
func (w *ClickOutsideWatcher) SetPopupRect(x, y, width, height int32) {
}

// ClearPopupRect clears the popup area (no-op on non-Windows/macOS platforms).
func (w *ClickOutsideWatcher) ClearPopupRect() {
}
