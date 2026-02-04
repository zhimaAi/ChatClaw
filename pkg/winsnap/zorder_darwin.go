//go:build darwin && cgo

package winsnap

/*
#cgo darwin CFLAGS: -x objective-c -fobjc-arc
#cgo darwin LDFLAGS: -framework Cocoa

#import <Cocoa/Cocoa.h>

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

static char* winsnap_frontmost_localized_name() {
	@autoreleasepool {
		NSRunningApplication *app = [[NSWorkspace sharedWorkspace] frontmostApplication];
		if (!app) return NULL;
		NSString *name = app.localizedName;
		return winsnap_strdup_nsstring(name);
	}
}

static char* winsnap_frontmost_executable_name() {
	@autoreleasepool {
		NSRunningApplication *app = [[NSWorkspace sharedWorkspace] frontmostApplication];
		if (!app) return NULL;
		NSString *exe = app.executableURL.lastPathComponent;
		return winsnap_strdup_nsstring(exe);
	}
}

static void winsnap_free_cstring(char *s) {
	if (s) free(s);
}
*/
import "C"

import (
	"errors"
	"strings"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// TopMostVisibleProcessName returns the "frontmost" application name among targets.
// On macOS, "top-most" is approximated as the current frontmost application.
// Returns ErrSelfIsFrontmost if our own app is frontmost (caller should preserve current state).
func TopMostVisibleProcessName(targetProcessNames []string) (processName string, found bool, err error) {
	if len(targetProcessNames) == 0 {
		return "", false, nil
	}

	// If our own app is frontmost (user clicked on winsnap window), preserve current state
	if C.winsnap_is_self_frontmost() {
		return "", false, ErrSelfIsFrontmost
	}

	cLoc := C.winsnap_frontmost_localized_name()
	cExe := C.winsnap_frontmost_executable_name()
	var loc, exe string
	if cLoc != nil {
		loc = C.GoString(cLoc)
		C.winsnap_free_cstring(cLoc)
	}
	if cExe != nil {
		exe = C.GoString(cExe)
		C.winsnap_free_cstring(cExe)
	}

	locN := strings.ToLower(normalizeMacTargetName(loc))
	exeN := strings.ToLower(normalizeMacTargetName(exe))
	if locN == "" && exeN == "" {
		return "", false, nil
	}

	for _, raw := range targetProcessNames {
		n := strings.ToLower(normalizeMacTargetName(raw))
		if n == "" {
			continue
		}
		if (locN != "" && n == locN) || (exeN != "" && n == exeN) {
			// Return the original target name so AttachRightOfProcess can find PID.
			return raw, true, nil
		}
	}
	return "", false, nil
}

// MoveOffscreen moves the given window far outside the visible desktop area.
// This is used to represent "hidden (snapping) state" without closing the window.
func MoveOffscreen(window *application.WebviewWindow) error {
	if window == nil {
		return errors.New("winsnap: Window is nil")
	}
	// Wails macOS coordinates are in screen points; moving far negative is enough.
	window.SetPosition(-32000, -32000)
	return nil
}
