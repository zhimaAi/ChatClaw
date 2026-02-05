//go:build darwin && !ios

package floatingball

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa

void floatingballEnableHoverTracking(void* nsWindow);
*/
import "C"

import (
	"unsafe"

	"github.com/wailsapp/wails/v3/pkg/application"
)

func enableMacHoverTracking(win *application.WebviewWindow) {
	if win == nil {
		return
	}
	nw := win.NativeWindow()
	if nw == nil {
		return
	}
	C.floatingballEnableHoverTracking(unsafe.Pointer(nw))
}

