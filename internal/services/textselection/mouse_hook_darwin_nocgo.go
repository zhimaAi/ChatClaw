//go:build darwin && !cgo

package textselection

// MouseHookWatcher fallback for darwin when CGO is disabled.
type MouseHookWatcher struct {
	callback          func(text string, x, y int32)
	onDragStart       func(x, y int32)
	showPopupCallback func(x, y int32, originalAppPid int32)
}

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

func (w *MouseHookWatcher) Start() error { return nil }
func (w *MouseHookWatcher) Stop()        {}
