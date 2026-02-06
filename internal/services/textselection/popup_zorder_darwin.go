//go:build darwin && cgo

package textselection

/*
#cgo darwin CFLAGS: -x objective-c -fobjc-arc
#cgo darwin LDFLAGS: -framework Cocoa

#import <Cocoa/Cocoa.h>

// Force popup window to be topmost without activating it.
// Uses orderFrontRegardless to bring window to front without changing key window.
static void textselection_force_popup_topmost(void *nsWindow) {
	if (!nsWindow) return;

	dispatch_async(dispatch_get_main_queue(), ^{
		NSWindow *win = (__bridge NSWindow *)nsWindow;
		if (!win || ![win isKindOfClass:[NSWindow class]]) return;

		// Set window level to popup menu level (above all normal windows including winsnap)
		// NSPopUpMenuWindowLevel = 101, which is higher than NSFloatingWindowLevel (3)
		[win setLevel:NSPopUpMenuWindowLevel];

		// Bring window to front without activating the app
		[win orderFrontRegardless];
	});
}
*/
import "C"

import "github.com/wailsapp/wails/v3/pkg/application"

// forcePopupTopMostNoActivate ensures the popup is visible above other windows
// without stealing focus on macOS.
func forcePopupTopMostNoActivate(w *application.WebviewWindow) {
	if w == nil {
		return
	}
	nativeHandle := w.NativeWindow()
	if nativeHandle == nil {
		return
	}
	C.textselection_force_popup_topmost(nativeHandle)
}
