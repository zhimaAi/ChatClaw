//go:build windows

package textselection

import (
	"runtime"
	"sync"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// ClipboardWatcher monitors system clipboard changes.
type ClipboardWatcher struct {
	mu       sync.Mutex
	hwnd     windows.HWND
	callback func(text string, x, y int32)
	closed   bool
	ready    chan struct{}
}

var (
	modUser32   = windows.NewLazySystemDLL("user32.dll")
	modKernel32 = windows.NewLazySystemDLL("kernel32.dll")
	modShcore   = windows.NewLazySystemDLL("shcore.dll")

	procRegisterClassExW              = modUser32.NewProc("RegisterClassExW")
	procCreateWindowExW               = modUser32.NewProc("CreateWindowExW")
	procDestroyWindow                 = modUser32.NewProc("DestroyWindow")
	procDefWindowProcW                = modUser32.NewProc("DefWindowProcW")
	procGetMessageW                   = modUser32.NewProc("GetMessageW")
	procTranslateMessage              = modUser32.NewProc("TranslateMessage")
	procDispatchMessageW              = modUser32.NewProc("DispatchMessageW")
	procPostQuitMessage               = modUser32.NewProc("PostQuitMessage")
	procAddClipboardFormatListener    = modUser32.NewProc("AddClipboardFormatListener")
	procRemoveClipboardFormatListener = modUser32.NewProc("RemoveClipboardFormatListener")
	procOpenClipboard                 = modUser32.NewProc("OpenClipboard")
	procCloseClipboard                = modUser32.NewProc("CloseClipboard")
	procGetClipboardData              = modUser32.NewProc("GetClipboardData")
	procIsClipboardFormatAvailable    = modUser32.NewProc("IsClipboardFormatAvailable")
	procGetCursorPos                  = modUser32.NewProc("GetCursorPos")
	procGetDpiForSystem               = modUser32.NewProc("GetDpiForSystem")
	procGetDeviceCaps                 = modUser32.NewProc("GetDeviceCaps")
	procGetDC                         = modUser32.NewProc("GetDC")
	procReleaseDC                     = modUser32.NewProc("ReleaseDC")

	procGlobalLock   = modKernel32.NewProc("GlobalLock")
	procGlobalUnlock = modKernel32.NewProc("GlobalUnlock")
)

const (
	wmClipboardUpdate = 0x031D
	wmDestroy         = 0x0002
	cfUnicodeText     = 13

	csHRedraw = 0x0002
	csVRedraw = 0x0001
)

type wndClassExW struct {
	CbSize        uint32
	Style         uint32
	LpfnWndProc   uintptr
	CbClsExtra    int32
	CbWndExtra    int32
	HInstance     windows.Handle
	HIcon         windows.Handle
	HCursor       windows.Handle
	HbrBackground windows.Handle
	LpszMenuName  *uint16
	LpszClassName *uint16
	HIconSm       windows.Handle
}

type point struct {
	X int32
	Y int32
}

type msg struct {
	HWnd    windows.HWND
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Pt      point
}

var (
	clipboardWatcherInstance *ClipboardWatcher
	clipboardWatcherMu       sync.Mutex
)

// NewClipboardWatcher creates a new clipboard watcher.
func NewClipboardWatcher(callback func(text string, x, y int32)) *ClipboardWatcher {
	return &ClipboardWatcher{
		callback: callback,
		ready:    make(chan struct{}),
	}
}

// Start starts the watcher.
func (w *ClipboardWatcher) Start() error {
	clipboardWatcherMu.Lock()
	clipboardWatcherInstance = w
	clipboardWatcherMu.Unlock()

	go w.run()
	<-w.ready
	return nil
}

// Stop stops the watcher.
func (w *ClipboardWatcher) Stop() {
	w.mu.Lock()
	if w.closed {
		w.mu.Unlock()
		return
	}
	w.closed = true
	hwnd := w.hwnd
	w.mu.Unlock()

	if hwnd != 0 {
		procRemoveClipboardFormatListener.Call(uintptr(hwnd))
		procDestroyWindow.Call(uintptr(hwnd))
	}
}

func (w *ClipboardWatcher) run() {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	className, _ := syscall.UTF16PtrFromString("ClipboardWatcherClass")
	windowName, _ := syscall.UTF16PtrFromString("ClipboardWatcher")

	wc := wndClassExW{
		CbSize:        uint32(unsafe.Sizeof(wndClassExW{})),
		Style:         csHRedraw | csVRedraw,
		LpfnWndProc:   syscall.NewCallback(clipboardWndProc),
		LpszClassName: className,
	}

	procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))

	hwnd, _, _ := procCreateWindowExW.Call(
		0,
		uintptr(unsafe.Pointer(className)),
		uintptr(unsafe.Pointer(windowName)),
		0,
		0, 0, 0, 0,
		0, 0, 0, 0,
	)

	w.mu.Lock()
	w.hwnd = windows.HWND(hwnd)
	w.mu.Unlock()

	if hwnd != 0 {
		procAddClipboardFormatListener.Call(hwnd)
	}

	close(w.ready)

	if hwnd == 0 {
		return
	}

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

func clipboardWndProc(hwnd uintptr, uMsg uint32, wParam, lParam uintptr) uintptr {
	switch uMsg {
	case wmClipboardUpdate:
		text := getClipboardText()
		if text != "" {
			// Get mouse position immediately during message handling for accuracy
			x, y := GetCursorPos()

			clipboardWatcherMu.Lock()
			w := clipboardWatcherInstance
			clipboardWatcherMu.Unlock()
			if w != nil && w.callback != nil {
				w.callback(text, x, y)
			}
		}
		return 0
	case wmDestroy:
		procPostQuitMessage.Call(0)
		return 0
	}

	ret, _, _ := procDefWindowProcW.Call(hwnd, uintptr(uMsg), wParam, lParam)
	return ret
}

func getClipboardText() string {
	// Check if text format is available
	r1, _, _ := procIsClipboardFormatAvailable.Call(uintptr(cfUnicodeText))
	if r1 == 0 {
		return ""
	}

	// Open clipboard
	r1, _, _ = procOpenClipboard.Call(0)
	if r1 == 0 {
		return ""
	}
	defer procCloseClipboard.Call()

	// Get data
	hData, _, _ := procGetClipboardData.Call(uintptr(cfUnicodeText))
	if hData == 0 {
		return ""
	}

	// Lock memory
	ptr, _, _ := procGlobalLock.Call(hData)
	if ptr == 0 {
		return ""
	}
	defer procGlobalUnlock.Call(hData)

	// Convert to string
	text := windows.UTF16PtrToString((*uint16)(unsafe.Pointer(ptr)))
	return text
}

// GetCursorPos gets the current mouse position (returns logical pixel coordinates, accounting for DPI scaling).
func GetCursorPos() (x, y int32) {
	var pt point
	procGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))

	// Get DPI scale factor
	scale := getDPIScale()
	if scale > 1.0 {
		// Convert physical pixels to logical pixels
		return int32(float64(pt.X) / scale), int32(float64(pt.Y) / scale)
	}
	return pt.X, pt.Y
}

// Cached DPI scale to avoid expensive GDI calls in high-frequency callbacks
var (
	cachedDPIScale     float64 = 0
	cachedDPIScaleMu   sync.RWMutex
	dpiScaleInitOnce   sync.Once
)

// getDPIScale gets the system DPI scale factor.
// Uses cached value to avoid expensive GDI calls in mouse hook callbacks.
func getDPIScale() float64 {
	// Initialize cache on first call
	dpiScaleInitOnce.Do(func() {
		refreshDPIScaleInternal()
	})

	cachedDPIScaleMu.RLock()
	scale := cachedDPIScale
	cachedDPIScaleMu.RUnlock()

	if scale > 0 {
		return scale
	}
	return 1.0
}

// RefreshDPIScale updates the cached DPI scale value.
// Call this when display settings might have changed.
func RefreshDPIScale() {
	refreshDPIScaleInternal()
}

func refreshDPIScaleInternal() {
	scale := 1.0

	// Try using GetDpiForSystem (Windows 10 1607+)
	if procGetDpiForSystem.Find() == nil {
		dpi, _, _ := procGetDpiForSystem.Call()
		if dpi > 0 {
			scale = float64(dpi) / 96.0
		}
	} else {
		// Fallback: use GetDeviceCaps
		const logPixelsX = 88
		hdc, _, _ := procGetDC.Call(0)
		if hdc != 0 {
			dpi, _, _ := procGetDeviceCaps.Call(hdc, logPixelsX)
			procReleaseDC.Call(0, hdc)
			if dpi > 0 {
				scale = float64(dpi) / 96.0
			}
		}
	}

	cachedDPIScaleMu.Lock()
	cachedDPIScale = scale
	cachedDPIScaleMu.Unlock()
}
