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

	// Optional callback for clicks inside the popup (used for context menu)
	insideCallback func(x, y int32)
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

// SetInsideCallback sets a callback for left-clicks inside the popup area.
func (w *ClickOutsideWatcher) SetInsideCallback(cb func(x, y int32)) {
	w.mu.Lock()
	w.insideCallback = cb
	w.mu.Unlock()
}

// ClearInsideCallback removes the inside-click callback.
func (w *ClickOutsideWatcher) ClearInsideCallback() {
	w.mu.Lock()
	w.insideCallback = nil
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
			x, y := GetPhysicalCursorPos()

			w.mu.Lock()
			popupRect := w.popupRect
			outsideCB := w.callback
			insideCB := w.insideCallback
			w.mu.Unlock()

			if popupRect.Right > popupRect.Left && popupRect.Bottom > popupRect.Top {
				inside := x >= popupRect.Left && x <= popupRect.Right &&
					y >= popupRect.Top && y <= popupRect.Bottom
				if inside {
					if insideCB != nil {
						go insideCB(x, y)
					}
				} else {
					if outsideCB != nil {
						go outsideCB(x, y)
					}
				}
			}
		}
	}

	ret, _, _ := procCallNextHookEx.Call(0, uintptr(nCode), wParam, lParam)
	return ret
}
