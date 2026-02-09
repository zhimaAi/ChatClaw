//go:build darwin && !ios

package floatingball

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa

#include <stdbool.h>

bool floatingballPrimaryWorkArea(int *outX, int *outY, int *outW, int *outH, float *outScale);
*/
import "C"

import "github.com/wailsapp/wails/v3/pkg/application"

func primaryWorkAreaNative() (application.Rect, float32, bool) {
	var x, y, w, h C.int
	var scale C.float
	ok := C.floatingballPrimaryWorkArea(&x, &y, &w, &h, &scale)
	if !ok {
		return application.Rect{}, 1, false
	}
	rect := application.Rect{X: int(x), Y: int(y), Width: int(w), Height: int(h)}
	sf := float32(scale)
	if sf <= 0 {
		sf = 1
	}
	if rect.Width <= 0 || rect.Height <= 0 {
		return application.Rect{}, sf, false
	}
	return rect, sf, true
}

