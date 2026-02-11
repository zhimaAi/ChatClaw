//go:build windows

package textselection

import (
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

// MouseHookWatcher uses global mouse hook to detect text selection.
// Principle: when mouse drag is detected, simulate Ctrl+C to copy selected text, then read clipboard.
type MouseHookWatcher struct {
	mu                  sync.Mutex
	hook                uintptr
	callback            func(text string, x, y int32)
	onDragStart         func(x, y int32)                       // Callback when drag starts (legacy, no PID)
	onDragStartWithPid  func(x, y int32, frontAppPid int32)    // Callback when drag starts (with frontmost app PID)
	showPopupCallback   func(x, y int32, originalAppPid int32) // New design for macOS, not used on Windows
	closed              bool
	ready               chan struct{}

	// Drag detection
	isDragging    bool
	dragStartX    int32
	dragStartY    int32
	dragStartTime time.Time // Record drag start time for duration filtering
	lastMouseX    int32
	lastMouseY    int32
	dragDistance   int32 // Minimum drag distance threshold (in physical pixels)
}

var (
	procSetWindowsHookExW   = modUser32.NewProc("SetWindowsHookExW")
	procUnhookWindowsHookEx = modUser32.NewProc("UnhookWindowsHookEx")
	procCallNextHookEx      = modUser32.NewProc("CallNextHookEx")
	procSendInput           = modUser32.NewProc("SendInput")
	procGetAsyncKeyState    = modUser32.NewProc("GetAsyncKeyState")
	procGetDoubleClickTime  = modUser32.NewProc("GetDoubleClickTime")

	mouseHookInstance   *MouseHookWatcher
	mouseHookInstanceMu sync.Mutex

	mouseHookCBOnce sync.Once
	mouseHookCB     uintptr
)

const (
	whMouseLL     = 14
	wmLButtonDown = 0x0201
	wmLButtonUp   = 0x0202
	wmMouseMove   = 0x0200

	inputKeyboard = 1
	keyEventKeyUp = 0x0002
	vkControl     = 0x11
	vkC           = 0x43
)

type msllHookStruct struct {
	Pt          point
	MouseData   uint32
	Flags       uint32
	Time        uint32
	DwExtraInfo uintptr
}

type keyboardInput struct {
	Type uint32
	Ki   keyBdInput
}

type keyBdInput struct {
	WVk         uint16
	WScan       uint16
	DwFlags     uint32
	Time        uint32
	DwExtraInfo uintptr
	_           [8]byte // padding
}

// NewMouseHookWatcher creates a new mouse hook watcher.
func NewMouseHookWatcher(
	callback func(text string, x, y int32),
	onDragStartWithPid func(x, y int32, frontAppPid int32),
	showPopupCallback func(x, y int32, originalAppPid int32),
) *MouseHookWatcher {
	return &MouseHookWatcher{
		callback:           callback,
		onDragStartWithPid: onDragStartWithPid,
		showPopupCallback:  showPopupCallback,
		ready:              make(chan struct{}),
		dragDistance:       30, // Minimum drag distance (physical px) to prevent accidental triggers
	}
}

// Helper function stubs (only used on macOS)
func activateAppByPidDarwin(pid int32) {}
func simulateCmdCDarwin()              {}
func getClipboardTextDarwin() string   { return "" }

// Start starts the watcher.
func (w *MouseHookWatcher) Start() error {
	mouseHookInstanceMu.Lock()
	mouseHookInstance = w
	mouseHookInstanceMu.Unlock()

	go w.run()
	<-w.ready
	return nil
}

// Stop stops the watcher.
func (w *MouseHookWatcher) Stop() {
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
}

func (w *MouseHookWatcher) run() {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	mouseHookCBOnce.Do(func() {
		mouseHookCB = syscall.NewCallback(lowLevelMouseProc)
	})
	hook, _, _ := procSetWindowsHookExW.Call(
		uintptr(whMouseLL),
		mouseHookCB,
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
	var m msg
	for {
		ret, _, _ := procGetMessageW.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		if int32(ret) <= 0 {
			break
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
		procDispatchMessageW.Call(uintptr(unsafe.Pointer(&m)))
	}
}

func lowLevelMouseProc(nCode int32, wParam uintptr, lParam uintptr) uintptr {
	if nCode >= 0 {
		mouseHookInstanceMu.Lock()
		w := mouseHookInstance
		mouseHookInstanceMu.Unlock()

		if w != nil {
			switch wParam {
			case wmLButtonDown:
				// Use GetPhysicalCursorPos to always get physical screen coordinates,
				// regardless of the hook thread's DPI awareness context.
				// hookStruct.Pt may return system-DPI-logical coords on some setups,
				// which causes incorrect positioning on multi-monitor with different DPIs.
				physX, physY := GetPhysicalCursorPos()
				w.mu.Lock()
				w.isDragging = true
				w.dragStartX = physX
				w.dragStartY = physY
				w.dragStartTime = time.Now()
				onDragStartWithPid := w.onDragStartWithPid
				mouseX := physX
				mouseY := physY
				w.mu.Unlock()

				// Notify drag start with mouse position for caller to check if inside popup
				// On Windows, we pass -1 as PID (not needed, popup doesn't steal focus)
				if onDragStartWithPid != nil {
					go onDragStartWithPid(mouseX, mouseY, -1)
				}

			case wmMouseMove:
				physX, physY := GetPhysicalCursorPos()
				w.mu.Lock()
				w.lastMouseX = physX
				w.lastMouseY = physY
				w.mu.Unlock()

			case wmLButtonUp:
				physX, physY := GetPhysicalCursorPos()
				w.mu.Lock()
				if w.isDragging {
					dx := physX - w.dragStartX
					dy := physY - w.dragStartY
					distance := dx*dx + dy*dy
					dragDuration := time.Since(w.dragStartTime)

					mouseX := physX
					mouseY := physY

					// Multi-layer heuristic filtering to reduce false triggers:
					//
					// Filter 1: minimum drag distance (30px in physical pixels).
					// Filter 2: minimum drag duration (150ms) — quick clicks/flicks are not text selection.
					// Filter 3: reject mostly-vertical drags (|dy| > 3*|dx| && |dx| < 20px)
					//           — likely window dragging, scrollbar, or screenshot tool.

					absDx := dx
					if absDx < 0 {
						absDx = -absDx
					}
					absDy := dy
					if absDy < 0 {
						absDy = -absDy
					}

					passDistance := distance > w.dragDistance*w.dragDistance
					passDuration := dragDuration >= 150*time.Millisecond
					passDirection := !(absDy > 3*absDx && absDx < 20)

					if passDistance && passDuration && passDirection {
						w.mu.Unlock()
						// Delay processing to let system complete selection
						go func() {
							time.Sleep(120 * time.Millisecond)
							w.handlePossibleSelection(mouseX, mouseY)
						}()
					} else {
						w.mu.Unlock()
					}
				} else {
					w.mu.Unlock()
				}
				w.mu.Lock()
				w.isDragging = false
				w.mu.Unlock()
			}
		}
	}

	ret, _, _ := procCallNextHookEx.Call(0, uintptr(nCode), wParam, lParam)
	return ret
}

func (w *MouseHookWatcher) handlePossibleSelection(mouseX, mouseY int32) {
	// Check if current focus window belongs to our application
	// If so, skip processing (frontend JavaScript already handles it)
	if isOwnWindowFocused() {
		return
	}

	w.mu.Lock()
	showPopupCallback := w.showPopupCallback
	callback := w.callback
	w.mu.Unlock()

	// Check if using new mode (show popup then copy)
	if showPopupCallback != nil {
		// New mode: only show popup, don't execute Ctrl+C
		// originalAppPid is set to -1 on Windows to indicate "other app"
		showPopupCallback(mouseX, mouseY, -1)
		return
	}

	// Old mode: copy then show popup

	// Simulate Ctrl+C
	simulateCtrlC()

	// Wait for clipboard to update
	time.Sleep(100 * time.Millisecond)

	// Read new clipboard content
	newClipboard := getClipboardText()

	// If clipboard has meaningful text, show popup
	// Skip if only whitespace (e.g., user selected image/screenshot, not text)
	// Allow same text selection - user may want to use the same text again
	newClipboard = strings.TrimSpace(newClipboard)
	if newClipboard != "" {
		if callback != nil {
			callback(newClipboard, mouseX, mouseY)
		}
	}
}

// isOwnWindowFocused checks if the current focus window belongs to our application.
func isOwnWindowFocused() bool {
	// Get foreground window
	foregroundHwnd, _, _ := procGetForegroundWindow.Call()
	if foregroundHwnd == 0 {
		return false
	}

	// Get foreground window's process ID
	var foregroundPid uint32
	procGetWindowThreadProcId.Call(foregroundHwnd, uintptr(unsafe.Pointer(&foregroundPid)))

	// Get current process ID
	currentPid, _, _ := procGetCurrentProcessId.Call()

	return uintptr(foregroundPid) == currentPid
}

// simulateCtrlC simulates pressing Ctrl+C.
func simulateCtrlC() {
	inputs := make([]keyboardInput, 4)

	// Ctrl down
	inputs[0] = keyboardInput{
		Type: inputKeyboard,
		Ki: keyBdInput{
			WVk: vkControl,
		},
	}

	// C down
	inputs[1] = keyboardInput{
		Type: inputKeyboard,
		Ki: keyBdInput{
			WVk: vkC,
		},
	}

	// C up
	inputs[2] = keyboardInput{
		Type: inputKeyboard,
		Ki: keyBdInput{
			WVk:     vkC,
			DwFlags: keyEventKeyUp,
		},
	}

	// Ctrl up
	inputs[3] = keyboardInput{
		Type: inputKeyboard,
		Ki: keyBdInput{
			WVk:     vkControl,
			DwFlags: keyEventKeyUp,
		},
	}

	procSendInput.Call(
		uintptr(len(inputs)),
		uintptr(unsafe.Pointer(&inputs[0])),
		uintptr(unsafe.Sizeof(inputs[0])),
	)
}

// ==================== Functions for service.go to call ====================

// simulateCtrlCWindows simulates Ctrl+C (for service.go to call).
func simulateCtrlCWindows() {
	simulateCtrlC()
}

// getClipboardTextWindows gets clipboard text (for service.go to call).
func getClipboardTextWindows() string {
	return getClipboardText()
}
