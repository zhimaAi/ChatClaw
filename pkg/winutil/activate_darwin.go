//go:build darwin && cgo

package winutil

/*
#cgo darwin CFLAGS: -x objective-c -fobjc-arc
#cgo darwin LDFLAGS: -framework Cocoa

#import <Cocoa/Cocoa.h>

// Activate current application (bring to front)
static void winutil_activate_current_app(void) {
	NSRunningApplication *app = [NSRunningApplication currentApplication];
	if (!app) return;
	[app activateWithOptions:NSApplicationActivateIgnoringOtherApps];
}
*/
import "C"

import (
	"github.com/wailsapp/wails/v3/pkg/application"
)

// ForceActivateWindow on macOS uses NSRunningApplication to bring the app to front.
// This is more reliable than Wails Focus() which may not work when the app is not frontmost.
// Safely handles the case when the window has been closed/released.
func ForceActivateWindow(w *application.WebviewWindow) {
	if w == nil {
		return
	}
	// Activate the current application to bring all its windows to front
	C.winutil_activate_current_app()
	// Check if window is still valid before calling Focus
	// On macOS, NativeWindow() returns nil for closed windows
	if w.NativeWindow() == nil {
		return
	}
	// Also call Wails Focus to ensure the specific window gets focus
	w.Focus()
}
