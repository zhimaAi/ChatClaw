//go:build windows

package textselection

import (
	"runtime"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

// MouseHookWatcher uses global mouse hook to detect text selection.
// Principle: when mouse drag is detected, simulate Ctrl+C to copy selected text, then read clipboard.
type MouseHookWatcher struct {
	mu                sync.Mutex
	hook              uintptr
	callback          func(text string, x, y int32)
	onDragStart       func(x, y int32)                       // Callback when drag starts
	showPopupCallback func(x, y int32, originalAppPid int32) // New design for macOS, not used on Windows
	closed            bool
	ready             chan struct{}

	// Drag detection
	isDragging   bool
	dragStartX   int32
	dragStartY   int32
	lastMouseX   int32
	lastMouseY   int32
	dragDistance int32 // Minimum drag distance threshold
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
	onDragStart func(x, y int32),
	showPopupCallback func(x, y int32, originalAppPid int32),
) *MouseHookWatcher {
	return &MouseHookWatcher{
		callback:          callback,
		onDragStart:       onDragStart,
		showPopupCallback: showPopupCallback,
		ready:             make(chan struct{}),
		dragDistance:      5, // Minimum drag distance to prevent accidental triggers
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

	cb := syscall.NewCallback(lowLevelMouseProc)
	hook, _, _ := procSetWindowsHookExW.Call(
		uintptr(whMouseLL),
		cb,
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
			hookStruct := (*msllHookStruct)(unsafe.Pointer(lParam))

			switch wParam {
			case wmLButtonDown:
				w.mu.Lock()
				w.isDragging = true
				w.dragStartX = hookStruct.Pt.X
				w.dragStartY = hookStruct.Pt.Y
				onDragStart := w.onDragStart
				// Convert to logical pixels
				scale := getDPIScale()
				mouseX := int32(float64(hookStruct.Pt.X) / scale)
				mouseY := int32(float64(hookStruct.Pt.Y) / scale)
				w.mu.Unlock()

				// Notify drag start with mouse position for caller to check if inside popup
				if onDragStart != nil {
					go onDragStart(mouseX, mouseY)
				}

			case wmMouseMove:
				w.mu.Lock()
				w.lastMouseX = hookStruct.Pt.X
				w.lastMouseY = hookStruct.Pt.Y
				w.mu.Unlock()

			case wmLButtonUp:
				w.mu.Lock()
				if w.isDragging {
					dx := hookStruct.Pt.X - w.dragStartX
					dy := hookStruct.Pt.Y - w.dragStartY
					distance := dx*dx + dy*dy

					// Convert to logical pixels
					scale := getDPIScale()
					mouseX := int32(float64(hookStruct.Pt.X) / scale)
					mouseY := int32(float64(hookStruct.Pt.Y) / scale)

					// Check if there's enough drag distance (indicating possible text selection)
					if distance > w.dragDistance*w.dragDistance {
						w.mu.Unlock()
						// Delay processing to let system complete selection
						go func() {
							time.Sleep(50 * time.Millisecond)
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

	// Save current clipboard content
	oldClipboard := getClipboardText()

	// Simulate Ctrl+C
	simulateCtrlC()

	// Wait for clipboard to update
	time.Sleep(100 * time.Millisecond)

	// Read new clipboard content
	newClipboard := getClipboardText()

	// If clipboard content changed, successfully copied selected text
	if newClipboard != "" && newClipboard != oldClipboard {
		if callback != nil {
			callback(newClipboard, mouseX, mouseY)
		}
	} else {
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
