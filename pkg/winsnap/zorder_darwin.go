//go:build darwin && cgo

package winsnap

/*
#cgo darwin CFLAGS: -x objective-c -fobjc-arc
#cgo darwin LDFLAGS: -framework Cocoa -framework CoreGraphics

#import <Cocoa/Cocoa.h>
#import <CoreGraphics/CGWindow.h>

static char* winsnap_strdup_nsstring(NSString *s) {
	if (!s) return NULL;
	const char *utf8 = [s UTF8String];
	if (!utf8) return NULL;
	return strdup(utf8);
}

// Check if frontmost app is our own app
static bool winsnap_is_self_frontmost() {
	@autoreleasepool {
		NSRunningApplication *frontApp = [[NSWorkspace sharedWorkspace] frontmostApplication];
		NSRunningApplication *selfApp = [NSRunningApplication currentApplication];
		if (!frontApp || !selfApp) return false;
		return [frontApp.bundleIdentifier isEqualToString:selfApp.bundleIdentifier];
	}
}

// Get window width from NSWindow frame (in points)
static CGFloat winsnap_get_window_width(void *nsWindowPtr) {
	if (!nsWindowPtr) return 0;
	@autoreleasepool {
		NSWindow *win = (__bridge NSWindow *)nsWindowPtr;
		return win.frame.size.width;
	}
}

// Get window height from NSWindow frame (in points)
static CGFloat winsnap_get_window_height(void *nsWindowPtr) {
	if (!nsWindowPtr) return 0;
	@autoreleasepool {
		NSWindow *win = (__bridge NSWindow *)nsWindowPtr;
		return win.frame.size.height;
	}
}

// Get the frontmost app's localized name (or executable name if localized name is empty).
// Returns NULL if no frontmost app or error.
static char* winsnap_get_frontmost_app_name() {
	@autoreleasepool {
		NSRunningApplication *frontApp = [[NSWorkspace sharedWorkspace] frontmostApplication];
		if (!frontApp) return NULL;
		NSString *name = frontApp.localizedName;
		if (!name || name.length == 0) {
			name = frontApp.executableURL.lastPathComponent;
		}
		if (!name || name.length == 0) return NULL;
		return winsnap_strdup_nsstring(name);
	}
}

// Get the frontmost app's PID.
// Returns 0 if no frontmost app.
static pid_t winsnap_get_frontmost_app_pid() {
	@autoreleasepool {
		NSRunningApplication *frontApp = [[NSWorkspace sharedWorkspace] frontmostApplication];
		if (!frontApp) return 0;
		return frontApp.processIdentifier;
	}
}

// Check if a given pid has any visible on-screen window (layer 0, reasonable size).
// This is used to determine if the target app is "visible" even when not frontmost.
static bool winsnap_pid_has_visible_window(pid_t pid) {
	if (pid <= 0) return false;

	CFArrayRef list = CGWindowListCopyWindowInfo(kCGWindowListOptionOnScreenOnly | kCGWindowListExcludeDesktopElements, kCGNullWindowID);
	if (!list) return false;

	bool found = false;
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

		// Check window size - skip tiny windows (likely hidden or auxiliary)
		CFDictionaryRef bounds = (CFDictionaryRef)CFDictionaryGetValue(dict, kCGWindowBounds);
		if (!bounds) continue;
		CGRect cgRect;
		if (!CGRectMakeWithDictionaryRepresentation(bounds, &cgRect)) continue;
		if (cgRect.size.width < 100 || cgRect.size.height < 100) continue;

		found = true;
		break;
	}
	CFRelease(list);
	return found;
}

// Find pid by app name (localized name or executable name)
static pid_t winsnap_find_pid_by_name_zorder(const char *name) {
	if (!name) return 0;
	NSString *target = [NSString stringWithUTF8String:name];
	if (!target || target.length == 0) return 0;

	// Normalize: drop path, .app, .exe suffixes
	target = [target lastPathComponent];
	NSString *lower = [target lowercaseString];
	if ([lower hasSuffix:@".app"]) {
		target = [target substringToIndex:(target.length - 4)];
	}
	lower = [target lowercaseString];
	if ([lower hasSuffix:@".exe"]) {
		target = [target substringToIndex:(target.length - 4)];
	}

	for (NSRunningApplication *app in [[NSWorkspace sharedWorkspace] runningApplications]) {
		if (!app || app.terminated) continue;
		NSString *loc = app.localizedName;
		NSString *exe = app.executableURL.lastPathComponent;
		if (loc.length && [[loc lowercaseString] isEqualToString:[target lowercaseString]]) {
			return app.processIdentifier;
		}
		if (exe.length && [[exe lowercaseString] isEqualToString:[target lowercaseString]]) {
			return app.processIdentifier;
		}
	}
	return 0;
}

static void winsnap_free_cstring(char *s) {
	if (s) free(s);
}
*/
import "C"

import (
	"errors"
	"unsafe"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// TopMostVisibleProcessName returns the target application that is currently frontmost (if any),
// or falls back to checking if any target app has a visible window.
// On macOS, we prioritize the frontmost app among targets to ensure proper snap behavior
// when the user switches between multiple target apps.
// Returns ErrSelfIsFrontmost if our own app is frontmost (caller should preserve current state).
func TopMostVisibleProcessName(targetProcessNames []string) (processName string, found bool, err error) {
	if len(targetProcessNames) == 0 {
		return "", false, nil
	}

	// If our own app is frontmost (user clicked on winsnap window), preserve current state
	if C.winsnap_is_self_frontmost() {
		return "", false, ErrSelfIsFrontmost
	}

	// First, check if the frontmost app is one of our target apps by comparing PIDs
	// This is more reliable than name comparison as it avoids localized name mismatches
	frontPID := C.winsnap_get_frontmost_app_pid()
	if frontPID > 0 {
		for _, raw := range targetProcessNames {
			n := normalizeMacTargetName(raw)
			if n == "" {
				continue
			}
			cname := C.CString(n)
			targetPID := C.winsnap_find_pid_by_name_zorder(cname)
			C.free(unsafe.Pointer(cname))

			// Check if frontmost app's PID matches this target's PID
			if targetPID > 0 && targetPID == frontPID {
				// Verify it has a visible window
				if C.winsnap_pid_has_visible_window(targetPID) {
					return raw, true, nil
				}
			}
		}
	}

	// Fallback: Check each target app to see if it has a visible window on screen
	// This handles cases where frontmost app is not a target but a target app is visible
	for _, raw := range targetProcessNames {
		n := normalizeMacTargetName(raw)
		if n == "" {
			continue
		}
		cname := C.CString(n)
		pid := C.winsnap_find_pid_by_name_zorder(cname)
		C.free(unsafe.Pointer(cname))

		if pid <= 0 {
			continue
		}

		// Check if this app has a visible window
		if C.winsnap_pid_has_visible_window(pid) {
			return raw, true, nil
		}
	}
	return "", false, nil
}

// MoveOffscreen hides the winsnap window on macOS.
// Unlike Windows which moves the window off-screen, macOS uses Hide() for reliable hiding
// because moving off-screen may still show the window in some cases (e.g., multiple displays).
func MoveOffscreen(window *application.WebviewWindow) error {
	if window == nil {
		return errors.New("winsnap: Window is nil")
	}
	// On macOS, use Hide() for reliable hiding instead of moving off-screen.
	// Moving off-screen can still show the window edge on some display configurations.
	window.Hide()
	return nil
}

// MoveToStandalone moves the window to a standalone position (right side of primary screen).
// This is used when the window is no longer attached to any target but should remain visible.
func MoveToStandalone(window *application.WebviewWindow) error {
	if window == nil {
		return errors.New("winsnap: Window is nil")
	}
	// Show window first if hidden
	window.Show()

	// Get window size using native NSWindow frame (more reliable than window.Size())
	ptr := window.NativeWindow()
	if ptr == nil {
		return errors.New("winsnap: native window is nil")
	}

	var width, height int
	cWidth := C.winsnap_get_window_width(ptr)
	cHeight := C.winsnap_get_window_height(ptr)
	width = int(cWidth)
	height = int(cHeight)

	if width <= 0 {
		width = 400
	}
	if height <= 0 {
		height = 720
	}

	// Get screen bounds from Wails
	screens, err := window.GetScreen()
	if err != nil || screens == nil {
		// Fallback: use reasonable defaults
		window.SetPosition(1920-width-20, (1080-height)/2)
		return nil
	}

	// Position: right side with 20px margin, vertically centered
	workRight := screens.Bounds.X + screens.Bounds.Width
	workTop := screens.Bounds.Y
	workHeight := screens.Bounds.Height

	posX := workRight - width - 20
	posY := workTop + (workHeight-height)/2

	window.SetPosition(posX, posY)
	return nil
}
