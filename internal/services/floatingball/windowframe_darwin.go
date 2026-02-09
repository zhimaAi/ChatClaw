//go:build darwin && !ios

package floatingball

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa

#include <stdbool.h>

bool floatingballGetWindowQuartzFrame(void *nsWindowPtr, int *outX, int *outY, int *outW, int *outH);
bool floatingballSetWindowQuartzFrame(void *nsWindowPtr, int qx, int qy, int w, int h);
*/
import "C"

import (
	"unsafe"

	"github.com/wailsapp/wails/v3/pkg/application"
)

func getNativeQuartzFrame(win *application.WebviewWindow) (application.Rect, bool) {
	if win == nil {
		return application.Rect{}, false
	}
	nw := win.NativeWindow()
	if nw == nil {
		return application.Rect{}, false
	}
	var x, y, w, h C.int
	ok := C.floatingballGetWindowQuartzFrame(unsafe.Pointer(nw), &x, &y, &w, &h)
	if !ok {
		return application.Rect{}, false
	}
	r := application.Rect{X: int(x), Y: int(y), Width: int(w), Height: int(h)}
	if r.Width <= 0 || r.Height <= 0 {
		return application.Rect{}, false
	}
	return r, true
}

func setNativeQuartzFrame(win *application.WebviewWindow, x, y, w, h int) bool {
	if win == nil {
		return false
	}
	nw := win.NativeWindow()
	if nw == nil {
		return false
	}
	ok := C.floatingballSetWindowQuartzFrame(unsafe.Pointer(nw), C.int(x), C.int(y), C.int(w), C.int(h))
	return bool(ok)
}

