//go:build windows

package floatingball

import (
	"unsafe"

	"github.com/wailsapp/wails/v3/pkg/application"
	"golang.org/x/sys/windows"
)

const (
	gwlStyle        = -16
	wsOverlappedWin = 0x00CF0000
	wsPopup         = 0x80000000
	swpNoMove       = 0x0002
	swpNoSize       = 0x0001
	swpNoZOrder     = 0x0004
	swpNoActivate   = 0x0010
	swpFrameChanged = 0x0020
)

var (
	user32               = windows.NewLazySystemDLL("user32.dll")
	procGetWindowLongPtr = user32.NewProc("GetWindowLongPtrW")
	procSetWindowLongPtr = user32.NewProc("SetWindowLongPtrW")
	procSetWindowPos     = user32.NewProc("SetWindowPos")
)

func enableWindowsPopupStyle(win *application.WebviewWindow, s *FloatingBallService) {
	if win == nil || s == nil {
		return
	}
	nw := win.NativeWindow()
	if nw == nil {
		return
	}
	hwnd := uintptr(unsafe.Pointer(nw))

	style, _, _ := procGetWindowLongPtr.Call(hwnd, uintptr(gwlStyle))
	newStyle := (style &^ wsOverlappedWin) | wsPopup
	if newStyle != style {
		_, _, _ = procSetWindowLongPtr.Call(hwnd, uintptr(gwlStyle), newStyle)
		// Apply frame change so Windows recalculates non-client metrics.
		_, _, _ = procSetWindowPos.Call(
			hwnd,
			0,
			0, 0, 0, 0,
			uintptr(swpNoMove|swpNoSize|swpNoZOrder|swpNoActivate|swpFrameChanged),
		)
	}

	b := win.Bounds()
	s.debugLog("win:style", map[string]any{
		"styleOld": style,
		"styleNew": newStyle,
		"boundsW":  b.Width,
		"boundsH":  b.Height,
	})
}

