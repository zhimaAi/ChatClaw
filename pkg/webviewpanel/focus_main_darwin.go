//go:build darwin && !ios && cgo

package webviewpanel

/*
#cgo darwin CFLAGS: -fobjc-arc
#cgo darwin LDFLAGS: -framework Cocoa -framework WebKit

#include <stdlib.h>
#include "panel_darwin.h"
*/
import "C"

import "unsafe"

// FocusMainWebview tries to move focus back to the host (main) WKWebView inside the parent window.
// Workaround: when switching focus between embedded panels (WKWebView) and the host webview,
// clicks on HTML inputs may require an extra "blank click" to activate the host webview.
func FocusMainWebview(parentHwnd uintptr) {
	if parentHwnd == 0 {
		return
	}
	C.wvpanel_focus_main_webview(unsafe.Pointer(parentHwnd))
}

