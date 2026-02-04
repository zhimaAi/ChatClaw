//go:build darwin && cgo

package textselection

/*
#cgo darwin CFLAGS: -x objective-c -fobjc-arc
#cgo darwin LDFLAGS: -framework Cocoa

#import <Cocoa/Cocoa.h>

// Activate current application (bring to front)
static void textselection_activate_current_app(void) {
	NSRunningApplication *app = [NSRunningApplication currentApplication];
	if (!app) return;
	[app activateWithOptions:NSApplicationActivateIgnoringOtherApps];
}

// Activate application by PID
static void textselection_activate_app_by_pid(pid_t pid) {
	if (pid <= 0) return;
	NSRunningApplication *app = [NSRunningApplication runningApplicationWithProcessIdentifier:pid];
	if (!app) return;
	[app activateWithOptions:NSApplicationActivateIgnoringOtherApps];
}
*/
import "C"

import (
	"github.com/wailsapp/wails/v3/pkg/application"
)

// forceActivateWindow on macOS uses NSRunningApplication to bring the app to front.
// This is more reliable than Wails Focus() which may not work when the app is not frontmost.
func forceActivateWindow(w *application.WebviewWindow) {
	if w == nil {
		return
	}
	// Activate the current application to bring all its windows to front
	C.textselection_activate_current_app()
	// Also call Wails Focus to ensure the specific window gets focus
	w.Focus()
}

// ActivateAppByPid activates an application by its process ID.
func ActivateAppByPid(pid int32) {
	C.textselection_activate_app_by_pid(C.int(pid))
}
