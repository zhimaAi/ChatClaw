//go:build darwin && !cgo

package textselection

// MouseHookWatcher fallback for darwin when CGO is disabled.
type MouseHookWatcher struct {
	callback              func(text string, x, y int32)
	onDragStartWithPid    func(x, y int32, frontAppPid int32)
	showPopupCallback     func(x, y int32, originalAppPid int32)
}

func NewMouseHookWatcher(
	callback func(text string, x, y int32),
	onDragStartWithPid func(x, y int32, frontAppPid int32),
	showPopupCallback func(x, y int32, originalAppPid int32),
) *MouseHookWatcher {
	return &MouseHookWatcher{
		callback:              callback,
		onDragStartWithPid:    onDragStartWithPid,
		showPopupCallback:     showPopupCallback,
	}
}

func (w *MouseHookWatcher) Start() error { return nil }
func (w *MouseHookWatcher) Stop()        {}
