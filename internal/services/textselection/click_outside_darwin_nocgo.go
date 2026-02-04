//go:build darwin && !cgo

package textselection

import "sync"

// ClickOutsideWatcher fallback for darwin when CGO is disabled.
type ClickOutsideWatcher struct {
	mu       sync.Mutex
	callback func(x, y int32)
	closed   bool
	ready    chan struct{}
}

func NewClickOutsideWatcher(callback func(x, y int32)) *ClickOutsideWatcher {
	return &ClickOutsideWatcher{
		callback: callback,
		ready:    make(chan struct{}),
	}
}

func (w *ClickOutsideWatcher) Start() error {
	close(w.ready)
	return nil
}

func (w *ClickOutsideWatcher) Stop() {
	w.mu.Lock()
	w.closed = true
	w.mu.Unlock()
}

func (w *ClickOutsideWatcher) SetPopupRect(x, y, width, height int32) {}
func (w *ClickOutsideWatcher) ClearPopupRect()                        {}
