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
*/
import "C"

import (
	"errors"
	"unsafe"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// WakeAttachedWindow on macOS:
// 1) Activate the target app so its window comes to front
// 2) Activate current app again so the user can continue interacting with winsnap
// The winsnap window itself is kept at normal level to avoid covering other apps.
func WakeAttachedWindow(_ *application.WebviewWindow, targetProcessName string) error {
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
	C.winsnap_activate_pid(pid)
	C.winsnap_activate_current_app()
	return nil
}
