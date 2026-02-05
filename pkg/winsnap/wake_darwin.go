//go:build darwin && cgo

package winsnap

/*
#cgo darwin CFLAGS: -x objective-c -fobjc-arc
#cgo darwin LDFLAGS: -framework Cocoa -framework ApplicationServices -framework CoreGraphics

#import <Cocoa/Cocoa.h>
#import <ApplicationServices/ApplicationServices.h>
#import <CoreGraphics/CGWindow.h>

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

// Activate target app and then order winsnap window above it, then refocus winsnap.
// This function handles the timing issue where activating the target app
// changes the z-order, potentially hiding the winsnap window.
// It activates the target, waits briefly for z-order to stabilize,
// ensures winsnap is ordered above the target, then returns focus to winsnap.
// The refocusWinsnap parameter controls whether to return focus to winsnap window.
static int winsnap_wake_and_order_above(pid_t targetPid, int selfWindowNumber, int targetWindowNumber, int refocusWinsnap) {
	if (targetPid <= 0 || selfWindowNumber <= 0) return 0;

	// Activate target app first
	NSRunningApplication *app = [NSRunningApplication runningApplicationWithProcessIdentifier:targetPid];
	if (!app) return 0;
	[app activateWithOptions:NSApplicationActivateIgnoringOtherApps];

	// Order winsnap window above target on main thread
	// Use dispatch_async with a small delay to ensure target activation completes
	__block int result = 0;
	dispatch_sync(dispatch_get_main_queue(), ^{
		// Find the winsnap window
		for (NSWindow *win in [NSApp windows]) {
			if ((int)[win windowNumber] == selfWindowNumber) {
				if ([win isVisible]) {
					// Bring winsnap to front regardless of current z-order
					[win orderFrontRegardless];

					// If we have a target window number, order above it
					if (targetWindowNumber > 0) {
						[win orderWindow:NSWindowAbove relativeTo:targetWindowNumber];
					}
					result = 1;
				}
				return;
			}
		}
	});

	// Return focus to winsnap window if requested
	// This ensures user can continue typing in winsnap after clicking on it
	if (result && refocusWinsnap) {
		// Re-activate current app (winsnap) to get keyboard focus back
		NSRunningApplication *selfApp = [NSRunningApplication currentApplication];
		if (selfApp) {
			[selfApp activateWithOptions:NSApplicationActivateIgnoringOtherApps];
		}
		// Make winsnap window key window on main thread
		dispatch_sync(dispatch_get_main_queue(), ^{
			for (NSWindow *win in [NSApp windows]) {
				if ((int)[win windowNumber] == selfWindowNumber) {
					[win makeKeyAndOrderFront:nil];
					return;
				}
			}
		});
	}

	return result;
}

static void winsnap_activate_current_app(void) {
	NSRunningApplication *app = [NSRunningApplication currentApplication];
	if (!app) return;
	[app activateWithOptions:NSApplicationActivateIgnoringOtherApps];
}

// Find the main window number of a given pid using CGWindowListCopyWindowInfo.
// Returns 0 if not found.
static int winsnap_find_main_window_number_for_pid(pid_t pid) {
	if (pid <= 0) return 0;

	CFArrayRef list = CGWindowListCopyWindowInfo(kCGWindowListOptionOnScreenOnly | kCGWindowListExcludeDesktopElements, kCGNullWindowID);
	if (!list) return 0;

	int result = 0;
	double bestArea = 0.0;
	CFIndex n = CFArrayGetCount(list);
	for (CFIndex i = 0; i < n; i++) {
		CFDictionaryRef dict = (CFDictionaryRef)CFArrayGetValueAtIndex(list, i);
		CFNumberRef pidRef = (CFNumberRef)CFDictionaryGetValue(dict, kCGWindowOwnerPID);
		if (!pidRef) continue;
		pid_t wpid = 0;
		CFNumberGetValue(pidRef, kCFNumberIntType, &wpid);
		if (wpid != pid) continue;

		// Check window layer - only consider normal windows (layer 0)
		CFNumberRef layerRef = (CFNumberRef)CFDictionaryGetValue(dict, kCGWindowLayer);
		if (layerRef) {
			int layer = 0;
			CFNumberGetValue(layerRef, kCFNumberIntType, &layer);
			if (layer != 0) continue; // Skip non-normal windows (menus, tooltips, etc.)
		}

		CFDictionaryRef bounds = (CFDictionaryRef)CFDictionaryGetValue(dict, kCGWindowBounds);
		if (!bounds) continue;
		CGRect cgRect;
		if (!CGRectMakeWithDictionaryRepresentation(bounds, &cgRect)) continue;

		// Pick the largest window as the "main" window
		double area = (double)cgRect.size.width * (double)cgRect.size.height;
		if (area > bestArea) {
			bestArea = area;
			CFNumberRef numRef = (CFNumberRef)CFDictionaryGetValue(dict, kCGWindowNumber);
			if (numRef) {
				CFNumberGetValue(numRef, kCFNumberIntType, &result);
			}
		}
	}
	CFRelease(list);
	return result;
}

// Order a window with the given window number to front without activating the entire application.
// Dispatches to main thread synchronously to ensure thread safety.
// Uses window number to find the window safely, avoiding stale pointer issues.
// Returns 0 if window not found, 1 if successfully ordered.
static int winsnap_order_window_front_by_number(int windowNumber) {
	if (windowNumber <= 0) return 0;

	__block int result = 0;
	dispatch_sync(dispatch_get_main_queue(), ^{
		// Find the window by its window number on the main thread
		for (NSWindow *win in [NSApp windows]) {
			if ((int)[win windowNumber] == windowNumber) {
				if ([win isVisible]) {
					[win orderFrontRegardless];
					result = 1;
				}
				return;
			}
		}
	});
	return result;
}

// Order winsnap window above the target window (by window number).
// This ensures proper z-order relationship between winsnap and target.
// Uses orderFrontRegardless first to ensure the window is visible, then orders above target.
// Returns 0 if window not found, 1 if successfully ordered.
static int winsnap_order_window_above_target(int selfWindowNumber, int targetWindowNumber) {
	if (selfWindowNumber <= 0) return 0;

	__block int result = 0;
	dispatch_sync(dispatch_get_main_queue(), ^{
		// Find the winsnap window by its window number on the main thread
		for (NSWindow *win in [NSApp windows]) {
			if ((int)[win windowNumber] == selfWindowNumber) {
				if ([win isVisible]) {
					// First, bring the window to front regardless of current state
					// This ensures the window is visible even if target activation changed z-order
					[win orderFrontRegardless];

					// Then, if we have a target window, order just above it
					// This maintains the relationship: target visible, winsnap on top
					if (targetWindowNumber > 0) {
						[win orderWindow:NSWindowAbove relativeTo:targetWindowNumber];
					}
					result = 1;
				}
				return;
			}
		}
	});
	return result;
}

// Get window number from NSWindow pointer. Returns 0 if invalid.
static int winsnap_get_window_number(void *nsWindow) {
	if (!nsWindow) return 0;
	__block int result = 0;
	dispatch_sync(dispatch_get_main_queue(), ^{
		NSWindow *win = (__bridge NSWindow *)nsWindow;
		if (win && [win isKindOfClass:[NSWindow class]]) {
			result = (int)[win windowNumber];
		}
	});
	return result;
}
*/
import "C"

import (
	"errors"
	"unsafe"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// EnsureWindowVisible shows the winsnap window on macOS.
// Since MoveOffscreen uses Hide() on macOS, we need to use Show() to make it visible again.
func EnsureWindowVisible(window *application.WebviewWindow) error {
	if window == nil {
		return ErrWinsnapWindowInvalid
	}
	// On macOS, since we use Hide() in MoveOffscreen, we need Show() to restore visibility.
	// Unlike Windows which just restores from minimized state, macOS needs explicit Show().
	window.Show()
	return nil
}

// WakeStandaloneWindow brings the winsnap window to front when it's in standalone state
// (visible but not attached to any target app).
// On macOS, this uses NSRunningApplication to activate the current app,
// then focuses the specific window.
func WakeStandaloneWindow(window *application.WebviewWindow) error {
	if window == nil {
		return ErrWinsnapWindowInvalid
	}
	nativeHandle := window.NativeWindow()
	if nativeHandle == nil {
		return ErrWinsnapWindowInvalid
	}
	// Activate current application (bring to front)
	C.winsnap_activate_current_app()
	// Show and focus the window
	window.Show()
	window.Focus()
	return nil
}

// WakeAttachedWindow on macOS:
// 1) Activate the target app so its window comes to front
// 2) Order the winsnap window just above the target window using orderFrontRegardless
// The winsnap window itself is kept at normal level to avoid covering other apps.
//
// This function is called when:
// - User clicks on the winsnap window (via frontend pointerdown event)
// - Winsnap window gains focus (detected by ErrSelfIsFrontmost in step loop)
//
// Returns ErrWinsnapWindowInvalid if the winsnap window is nil or has been closed/released.
func WakeAttachedWindow(self *application.WebviewWindow, targetProcessName string) error {
	return wakeAttachedWindowInternal(self, targetProcessName, false)
}

// WakeAttachedWindowWithRefocus is like WakeAttachedWindow but returns focus to the
// winsnap window after synchronizing z-order. This is used when the user clicks on
// the winsnap window - we want to bring the target window to front (so it's not hidden
// by other apps) but then return keyboard focus to winsnap so the user can type.
func WakeAttachedWindowWithRefocus(self *application.WebviewWindow, targetProcessName string) error {
	return wakeAttachedWindowInternal(self, targetProcessName, true)
}

func wakeAttachedWindowInternal(self *application.WebviewWindow, targetProcessName string, refocus bool) error {
	if self == nil {
		return ErrWinsnapWindowInvalid
	}

	// Get window number from native handle for safe lookups
	// This avoids using potentially stale NSWindow pointers
	nativeHandle := self.NativeWindow()
	if nativeHandle == nil {
		return ErrWinsnapWindowInvalid
	}

	selfWindowNumber := int(C.winsnap_get_window_number(nativeHandle))
	if selfWindowNumber <= 0 {
		// Window may have been closed or is in an invalid state
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
	defer C.free(unsafe.Pointer(cname))
	pid := C.winsnap_find_pid_by_name_local(cname)
	if pid <= 0 {
		return ErrTargetWindowNotFound
	}

	// Find the main window number of the target app for proper z-ordering
	targetWindowNumber := int(C.winsnap_find_main_window_number_for_pid(pid))

	// Activate target app and order winsnap window above it in one operation
	// This handles the timing issue where activating target may change z-order
	refocusFlag := C.int(0)
	if refocus {
		refocusFlag = 1
	}
	result := C.winsnap_wake_and_order_above(pid, C.int(selfWindowNumber), C.int(targetWindowNumber), refocusFlag)
	if result == 0 {
		// Window not found or not visible
		return ErrWinsnapWindowInvalid
	}
	return nil
}
