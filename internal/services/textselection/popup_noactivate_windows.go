//go:build windows

package textselection

import (
	"sync"
	"syscall"
	"unsafe"

	"github.com/wailsapp/wails/v3/pkg/application"
)

var (
	procSetWindowLongPtrWNA = modUser32.NewProc("SetWindowLongPtrW")
	procCallWindowProcW     = modUser32.NewProc("CallWindowProcW")
)

const (
	gwlpWndProc = ^uintptr(3) // GWLP_WNDPROC = -4

	wmMouseActivate = 0x0021
	wmActivate      = 0x0006
	wmNCActivate    = 0x0086
	wmSetFocus      = 0x0007

	maNoActivate = 3
	waInactive   = 0
)

// Store original window procedures and hooked windows
var (
	hookedWindows   = make(map[uintptr]uintptr) // hwnd -> original wndproc
	hookedWindowsMu sync.Mutex
)

// popupWndProc intercepts activation-related messages to prevent the popup
// from being activated when clicked.
// This avoids triggering Wails' internal Focus() call which fails on popup windows.
func popupWndProc(hwnd, msg, wParam, lParam uintptr) uintptr {
	hookedWindowsMu.Lock()
	originalWndProc := hookedWindows[hwnd]
	hookedWindowsMu.Unlock()

	if originalWndProc == 0 {
		// Should not happen, but handle gracefully
		return 0
	}

	switch msg {
	case wmMouseActivate:
		// Return MA_NOACTIVATE to prevent window activation from mouse click
		return maNoActivate

	case wmActivate:
		// If being activated (wParam != WA_INACTIVE), block it
		if wParam&0xFFFF != waInactive {
			// Don't call the original WndProc for activation
			// This prevents Wails from calling Focus()
			return 0
		}

	case wmNCActivate:
		// Prevent non-client area activation
		if wParam != 0 {
			// Block activation, but allow deactivation
			return 0
		}

	case wmSetFocus:
		// Block WM_SETFOCUS to prevent Wails from calling Focus() on WebView2
		// which causes "The parameter is incorrect" error
		return 0
	}

	// Call the original window procedure for all other messages
	ret, _, _ := procCallWindowProcW.Call(originalWndProc, hwnd, msg, wParam, lParam)
	return ret
}

var popupWndProcCallback = syscall.NewCallback(popupWndProc)

// tryConfigurePopupNoActivate hooks the popup window's WndProc to intercept
// activation messages and prevent window activation.
//
// This prevents the popup from being activated when clicked, which avoids
// triggering Wails' internal Focus() call that fails with WebView2 error
// "The parameter is incorrect".
func tryConfigurePopupNoActivate(w *application.WebviewWindow) {
	if w == nil {
		return
	}
	h := uintptr(w.NativeWindow())
	if h == 0 {
		return
	}

	hookedWindowsMu.Lock()
	defer hookedWindowsMu.Unlock()

	// Check if already hooked
	if _, exists := hookedWindows[h]; exists {
		return
	}

	// Replace the window procedure
	originalWndProc, _, _ := procSetWindowLongPtrWNA.Call(h, gwlpWndProc, popupWndProcCallback)
	if originalWndProc != 0 {
		hookedWindows[h] = originalWndProc
	}
}

// removePopupSubclass restores the original window procedure.
// Called when the window is being destroyed.
func removePopupSubclass(hwnd uintptr) {
	if hwnd == 0 {
		return
	}

	hookedWindowsMu.Lock()
	originalWndProc, exists := hookedWindows[hwnd]
	if exists {
		delete(hookedWindows, hwnd)
	}
	hookedWindowsMu.Unlock()

	if exists && originalWndProc != 0 {
		// Restore original window procedure
		procSetWindowLongPtrWNA.Call(hwnd, gwlpWndProc, originalWndProc)
	}
}

// Ensure unsafe is used
var _ = unsafe.Pointer(nil)
