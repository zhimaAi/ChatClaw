//go:build windows

package textselection

import (
	"github.com/wailsapp/wails/v3/pkg/application"
)

var (
	procGetWindowLongPtrW = modUser32.NewProc("GetWindowLongPtrW")
	procSetWindowLongPtrW = modUser32.NewProc("SetWindowLongPtrW")
	procSetWindowPosTS    = modUser32.NewProc("SetWindowPos")
)

const (
	gwlExstyle = ^uintptr(19) // -20

	wsExNoactivate = 0x08000000

	swpNoMove       = 0x0002
	swpNoSize       = 0x0001
	swpNoZOrder     = 0x0004
	swpNoActivate   = 0x0010
	swpFrameChanged = 0x0020
)

// tryConfigurePopupNoActivate makes the popup window non-activating to avoid
// WebView2 Focus crashes on click (WS_EX_NOACTIVATE).
func tryConfigurePopupNoActivate(w *application.WebviewWindow) {
	if w == nil {
		return
	}
	h := uintptr(w.NativeWindow())
	if h == 0 {
		return
	}

	ex, _, _ := procGetWindowLongPtrW.Call(h, gwlExstyle)
	newEx := ex | wsExNoactivate
	if newEx != ex {
		_, _, _ = procSetWindowLongPtrW.Call(h, gwlExstyle, newEx)
	}

	// Apply style without activation.
	_, _, _ = procSetWindowPosTS.Call(
		h,
		0,
		0, 0, 0, 0,
		uintptr(swpNoMove|swpNoSize|swpNoActivate|swpFrameChanged|swpNoZOrder),
	)
}
