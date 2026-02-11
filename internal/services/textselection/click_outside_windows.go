//go:build windows

package textselection

import (
	"runtime"
	"sync"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// clickRect used for click outside detection.
type clickRect struct {
	Left   int32
	Top    int32
	Right  int32
	Bottom int32
}

// ClickOutsideWatcher monitors clicks outside the popup.
type ClickOutsideWatcher struct {
	mu       sync.Mutex
	hook     uintptr
	callback func(x, y int32)
	closed   bool
	ready    chan struct{}

	// Popup area
	popupRect clickRect
}

// clickMsg message structure.
type clickMsg struct {
	HWnd    windows.HWND
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Pt      clickPoint
}

type clickPoint struct {
	X int32
	Y int32
}

// clickHookStruct mouse hook structure.
type clickHookStruct struct {
	Pt          clickPoint
	MouseData   uint32
	Flags       uint32
	Time        uint32
	DwExtraInfo uintptr
}

var (
	clickOutsideInstance   *ClickOutsideWatcher
	clickOutsideInstanceMu sync.Mutex

	clickOutsideCBOnce sync.Once
	clickOutsideCB     uintptr
)

// NewClickOutsideWatcher creates a new click outside watcher.
func NewClickOutsideWatcher(callback func(x, y int32)) *ClickOutsideWatcher {
	return &ClickOutsideWatcher{
		callback: callback,
		ready:    make(chan struct{}),
	}
}

// Start starts the watcher.
func (w *ClickOutsideWatcher) Start() error {
	clickOutsideInstanceMu.Lock()
	clickOutsideInstance = w
	clickOutsideInstanceMu.Unlock()

	go w.run()
	<-w.ready
	return nil
}

// Stop stops the watcher.
func (w *ClickOutsideWatcher) Stop() {
	w.mu.Lock()
	if w.closed {
		w.mu.Unlock()
		return
	}
	w.closed = true
	hook := w.hook
	w.hook = 0
	w.mu.Unlock()

	if hook != 0 {
		procUnhookWindowsHookEx.Call(hook)
	}

	clickOutsideInstanceMu.Lock()
	if clickOutsideInstance == w {
		clickOutsideInstance = nil
	}
	clickOutsideInstanceMu.Unlock()
}

// SetPopupRect sets the popup area.
func (w *ClickOutsideWatcher) SetPopupRect(x, y, width, height int32) {
	w.mu.Lock()
	w.popupRect = clickRect{
		Left:   x,
		Top:    y,
		Right:  x + width,
		Bottom: y + height,
	}
	w.mu.Unlock()
}

// ClearPopupRect clears the popup area.
func (w *ClickOutsideWatcher) ClearPopupRect() {
	w.mu.Lock()
	w.popupRect = clickRect{}
	w.mu.Unlock()
}

func (w *ClickOutsideWatcher) run() {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	clickOutsideCBOnce.Do(func() {
		clickOutsideCB = syscall.NewCallback(clickOutsideMouseProc)
	})
	hook, _, _ := procSetWindowsHookExW.Call(
		uintptr(whMouseLL),
		clickOutsideCB,
		0,
		0,
	)

	w.mu.Lock()
	w.hook = hook
	w.mu.Unlock()

	close(w.ready)

	if hook == 0 {
		return
	}

	// Message loop
	var m clickMsg
	for {
		ret, _, _ := procGetMessageW.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		if int32(ret) <= 0 {
			break
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
		procDispatchMessageW.Call(uintptr(unsafe.Pointer(&m)))
	}
}

func clickOutsideMouseProc(nCode int32, wParam uintptr, lParam uintptr) uintptr {
	if nCode >= 0 && wParam == wmLButtonDown {
		clickOutsideInstanceMu.Lock()
		w := clickOutsideInstance
		clickOutsideInstanceMu.Unlock()

		if w != nil {
			hookStruct := (*clickHookStruct)(unsafe.Pointer(lParam))

			// Get DPI scale factor, convert to logical pixels (use per-monitor DPI for multi-monitor support)
			scale := getDPIScaleForPoint(hookStruct.Pt.X, hookStruct.Pt.Y)
			x := int32(float64(hookStruct.Pt.X) / scale)
			y := int32(float64(hookStruct.Pt.Y) / scale)

			w.mu.Lock()
			popupRect := w.popupRect
			callback := w.callback
			w.mu.Unlock()

			// Check if click is outside popup
			if popupRect.Right > popupRect.Left && popupRect.Bottom > popupRect.Top {
				if x < popupRect.Left || x > popupRect.Right ||
					y < popupRect.Top || y > popupRect.Bottom {
					if callback != nil {
						go callback(x, y)
					}
				}
			}
		}
	}

	ret, _, _ := procCallNextHookEx.Call(0, uintptr(nCode), wParam, lParam)
	return ret
}
