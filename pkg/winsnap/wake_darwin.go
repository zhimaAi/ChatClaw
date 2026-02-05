//go:build darwin && cgo

package winsnap

/*
#cgo darwin CFLAGS: -x objective-c -fobjc-arc
#cgo darwin LDFLAGS: -framework Cocoa

#import <Cocoa/Cocoa.h>

static NSString *winsnap_trim(NSString *s) {
	if (!s) return @"";
	return [s stringByTrimmingCharactersInSet:[NSCharacterSet whitespaceAndNewlineCharacterSet]];
}

static NSString *winsnap_normalize_target_name(const char *name) {
	if (!name) return @"";
	NSString *raw = [NSString stringWithUTF8String:name];
	NSString *t = winsnap_trim(raw);
	if (t.length == 0) return @"";
	t = [t lastPathComponent];
	NSString *lower = [t lowercaseString];
	if ([lower hasSuffix:@".app"]) {
		t = [t substringToIndex:(t.length - 4)];
		lower = [t lowercaseString];
	}
	if ([lower hasSuffix:@".exe"]) {
		t = [t substringToIndex:(t.length - 4)];
	}
	return t;
}

static pid_t winsnap_find_pid_by_name_local(const char *name) {
	NSString *target = winsnap_normalize_target_name(name);
	if (target.length == 0) return 0;
	for (NSRunningApplication *app in [[NSWorkspace sharedWorkspace] runningApplications]) {
		if (!app || app.terminated) continue;
		NSString *loc = winsnap_trim(app.localizedName);
		NSString *exe = winsnap_trim(app.executableURL.lastPathComponent);
		if (loc.length && [loc caseInsensitiveCompare:target] == NSOrderedSame) {
			return app.processIdentifier;
		}
		if (exe.length && [exe caseInsensitiveCompare:target] == NSOrderedSame) {
			return app.processIdentifier;
		}
	}
	return 0;
}

static void winsnap_activate_pid(pid_t pid) {
	if (pid <= 0) return;
	NSRunningApplication *app = [NSRunningApplication runningApplicationWithProcessIdentifier:pid];
	if (!app) return;
	[app activateWithOptions:NSApplicationActivateIgnoringOtherApps];
}

static void winsnap_activate_current_app(void) {
	NSRunningApplication *app = [NSRunningApplication currentApplication];
	if (!app) return;
	[app activateWithOptions:NSApplicationActivateIgnoringOtherApps];
}

// Order a window with the given title to front without activating the entire application.
// Dispatches to main thread asynchronously to ensure thread safety.
// Uses window title to find the window safely, avoiding stale pointer issues.
static void winsnap_order_window_front_by_title(const char *title) {
	if (!title) return;
	NSString *targetTitle = [NSString stringWithUTF8String:title];
	if (targetTitle.length == 0) return;

	dispatch_async(dispatch_get_main_queue(), ^{
		// Find the window by its title on the main thread
		for (NSWindow *win in [NSApp windows]) {
			NSString *winTitle = [win title];
			if (winTitle && [winTitle isEqualToString:targetTitle]) {
				if ([win isVisible]) {
					[win orderFrontRegardless];
				}
				return;
			}
		}
	});
}
*/
import "C"

import (
	"errors"
	"unsafe"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// EnsureWindowVisible is a no-op on macOS; visibility is handled by the window manager.
func EnsureWindowVisible(_ *application.WebviewWindow) error {
	return nil
}

// WakeAttachedWindow on macOS:
// 1) Activate the target app so its window comes to front
// 2) Order only the winsnap window to front (not entire app) to avoid activating main window
// The winsnap window itself is kept at normal level to avoid covering other apps.
//
// Returns ErrWinsnapWindowInvalid if the winsnap window is nil or has been closed/released.
func WakeAttachedWindow(self *application.WebviewWindow, targetProcessName string) error {
	if self == nil {
		return ErrWinsnapWindowInvalid
	}
	if targetProcessName == "" {
		return errors.New("winsnap: TargetProcessName is empty")
	}
	name := normalizeMacTargetName(targetProcessName)
	if name == "" {
		return errors.New("winsnap: TargetProcessName is empty")
	}
	cname := C.CString(name)
	pid := C.winsnap_find_pid_by_name_local(cname)
	C.free(unsafe.Pointer(cname))
	if pid <= 0 {
		return ErrTargetWindowNotFound
	}
	// Activate target app (e.g., WeChat)
	C.winsnap_activate_pid(pid)
	// Only order the winsnap window to front, not the entire app (avoid activating main window)
	// Use window title to find the window safely, avoiding stale pointer issues.
	title := self.Title()
	if title == "" {
		// Window may have been closed or is in an invalid state
		return ErrWinsnapWindowInvalid
	}
	ctitle := C.CString(title)
	C.winsnap_order_window_front_by_title(ctitle)
	C.free(unsafe.Pointer(ctitle))
	return nil
}
