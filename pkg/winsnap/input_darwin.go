//go:build darwin && cgo

package winsnap

/*
#cgo darwin CFLAGS: -x objective-c -fobjc-arc
#cgo darwin LDFLAGS: -framework Cocoa -framework Carbon

#import <Cocoa/Cocoa.h>
#import <Carbon/Carbon.h>
#include <time.h>
#include <mach/mach_time.h>

// Helper to get current uptime timestamp in nanoseconds for CGEventSetTimestamp
// This is required on macOS Sequoia and later for CGEventPost to work reliably
// Falls back to mach_absolute_time() if clock_gettime_nsec_np fails
static uint64_t current_uptime_nsec() {
	uint64_t ts = clock_gettime_nsec_np(CLOCK_UPTIME_RAW);
	if (ts == 0) {
		// Fallback: use mach_absolute_time (needs conversion but works for timestamps)
		ts = mach_absolute_time();
	}
	return ts;
}

// Set clipboard text
static bool winsnap_set_clipboard_text(const char *text) {
	if (!text) return false;
	@autoreleasepool {
		NSPasteboard *pasteboard = [NSPasteboard generalPasteboard];
		[pasteboard clearContents];
		NSString *str = [NSString stringWithUTF8String:text];
		if (!str) return false;
		return [pasteboard setString:str forType:NSPasteboardTypeString];
	}
}

// Activate app by process name
static bool winsnap_activate_app(const char *name) {
	if (!name) return false;
	@autoreleasepool {
		NSString *target = [NSString stringWithUTF8String:name];
		if (!target || target.length == 0) return false;

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
			if ((loc.length && [[loc lowercaseString] isEqualToString:[target lowercaseString]]) ||
				(exe.length && [[exe lowercaseString] isEqualToString:[target lowercaseString]])) {
				[app activateWithOptions:NSApplicationActivateIgnoringOtherApps];
				return true;
			}
		}
		return false;
	}
}

// Simulate Cmd+V (paste)
static void winsnap_simulate_cmd_v() {
	CGEventSourceRef source = CGEventSourceCreate(kCGEventSourceStateHIDSystemState);
	if (!source) return;

	// Create key down event for V with Cmd modifier
	CGEventRef keyDown = CGEventCreateKeyboardEvent(source, (CGKeyCode)kVK_ANSI_V, true);
	CGEventSetFlags(keyDown, kCGEventFlagMaskCommand);
	CGEventSetTimestamp(keyDown, current_uptime_nsec()); // Required for macOS Sequoia+
	CGEventPost(kCGHIDEventTap, keyDown);
	CFRelease(keyDown);

	usleep(10000); // 10ms

	// Create key up event for V with Cmd modifier
	CGEventRef keyUp = CGEventCreateKeyboardEvent(source, (CGKeyCode)kVK_ANSI_V, false);
	CGEventSetFlags(keyUp, kCGEventFlagMaskCommand);
	CGEventSetTimestamp(keyUp, current_uptime_nsec()); // Required for macOS Sequoia+
	CGEventPost(kCGHIDEventTap, keyUp);
	CFRelease(keyUp);

	CFRelease(source);
}

// Simulate Enter key using CGEventPostToPid to send directly to target process.
// Some apps (like DingTalk) filter events from kCGEventSourceStateHIDSystemState,
// so we need to post directly to the target process.
static void winsnap_simulate_enter_to_pid(pid_t targetPid) {
	// Use kCGEventSourceStateCombinedSessionState which is more compatible with apps
	// that filter synthetic events
	CGEventSourceRef source = CGEventSourceCreate(kCGEventSourceStateCombinedSessionState);
	if (!source) {
		// Fallback to HID system state
		source = CGEventSourceCreate(kCGEventSourceStateHIDSystemState);
		if (!source) return;
	}

	CGEventRef keyDown = CGEventCreateKeyboardEvent(source, (CGKeyCode)kVK_Return, true);
	CGEventSetTimestamp(keyDown, current_uptime_nsec());
	
	CGEventRef keyUp = CGEventCreateKeyboardEvent(source, (CGKeyCode)kVK_Return, false);
	CGEventSetTimestamp(keyUp, current_uptime_nsec());

	if (targetPid > 0) {
		// Post directly to target process - this bypasses app-level event filtering
		CGEventPostToPid(targetPid, keyDown);
		usleep(20000); // 20ms - slightly longer for process-targeted events
		CGEventPostToPid(targetPid, keyUp);
	} else {
		// Fallback to system-wide post
		CGEventPost(kCGHIDEventTap, keyDown);
		usleep(10000);
		CGEventPost(kCGHIDEventTap, keyUp);
	}

	CFRelease(keyDown);
	CFRelease(keyUp);
	CFRelease(source);
}

// Legacy function for compatibility
static void winsnap_simulate_enter() {
	winsnap_simulate_enter_to_pid(0);
}

// Simulate Cmd+Enter to target process
static void winsnap_simulate_cmd_enter_to_pid(pid_t targetPid) {
	CGEventSourceRef source = CGEventSourceCreate(kCGEventSourceStateCombinedSessionState);
	if (!source) {
		source = CGEventSourceCreate(kCGEventSourceStateHIDSystemState);
		if (!source) return;
	}

	// For Cmd+Enter, we need to send Cmd down, then Return down/up, then Cmd up
	CGEventRef cmdDown = CGEventCreateKeyboardEvent(source, (CGKeyCode)kVK_Command, true);
	CGEventSetTimestamp(cmdDown, current_uptime_nsec());

	CGEventRef keyDown = CGEventCreateKeyboardEvent(source, (CGKeyCode)kVK_Return, true);
	CGEventSetFlags(keyDown, kCGEventFlagMaskCommand);
	CGEventSetTimestamp(keyDown, current_uptime_nsec());

	CGEventRef keyUp = CGEventCreateKeyboardEvent(source, (CGKeyCode)kVK_Return, false);
	CGEventSetFlags(keyUp, kCGEventFlagMaskCommand);
	CGEventSetTimestamp(keyUp, current_uptime_nsec());

	CGEventRef cmdUp = CGEventCreateKeyboardEvent(source, (CGKeyCode)kVK_Command, false);
	CGEventSetTimestamp(cmdUp, current_uptime_nsec());

	if (targetPid > 0) {
		CGEventPostToPid(targetPid, cmdDown);
		usleep(10000);
		CGEventPostToPid(targetPid, keyDown);
		usleep(20000);
		CGEventPostToPid(targetPid, keyUp);
		usleep(10000);
		CGEventPostToPid(targetPid, cmdUp);
	} else {
		CGEventPost(kCGHIDEventTap, cmdDown);
		usleep(10000);
		CGEventPost(kCGHIDEventTap, keyDown);
		usleep(10000);
		CGEventPost(kCGHIDEventTap, keyUp);
		usleep(10000);
		CGEventPost(kCGHIDEventTap, cmdUp);
	}

	CFRelease(cmdDown);
	CFRelease(keyDown);
	CFRelease(keyUp);
	CFRelease(cmdUp);
	CFRelease(source);
}

// Legacy function for compatibility
static void winsnap_simulate_cmd_enter() {
	winsnap_simulate_cmd_enter_to_pid(0);
}

// Get PID by process name (for sending keys to specific process)
static pid_t winsnap_get_pid_by_name(const char *name) {
	if (!name) return 0;
	@autoreleasepool {
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
			if ((loc.length && [[loc lowercaseString] isEqualToString:[target lowercaseString]]) ||
				(exe.length && [[exe lowercaseString] isEqualToString:[target lowercaseString]])) {
				return app.processIdentifier;
			}
		}
		return 0;
	}
}

// Get the main window frame of a running application by process name
// Returns false if app not found or window not available
static bool winsnap_get_app_window_frame(const char *name, CGRect *outFrame) {
	if (!name || !outFrame) return false;
	@autoreleasepool {
		NSString *target = [NSString stringWithUTF8String:name];
		if (!target || target.length == 0) return false;

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

		pid_t targetPid = 0;
		for (NSRunningApplication *app in [[NSWorkspace sharedWorkspace] runningApplications]) {
			if (!app || app.terminated) continue;
			NSString *loc = app.localizedName;
			NSString *exe = app.executableURL.lastPathComponent;
			if ((loc.length && [[loc lowercaseString] isEqualToString:[target lowercaseString]]) ||
				(exe.length && [[exe lowercaseString] isEqualToString:[target lowercaseString]])) {
				targetPid = app.processIdentifier;
				break;
			}
		}
		if (targetPid <= 0) return false;

		// Get window list and find the main window of target app
		CFArrayRef windowList = CGWindowListCopyWindowInfo(kCGWindowListOptionOnScreenOnly | kCGWindowListExcludeDesktopElements, kCGNullWindowID);
		if (!windowList) return false;

		bool found = false;
		CFIndex count = CFArrayGetCount(windowList);
		for (CFIndex i = 0; i < count && !found; i++) {
			CFDictionaryRef dict = (CFDictionaryRef)CFArrayGetValueAtIndex(windowList, i);
			CFNumberRef pidRef = (CFNumberRef)CFDictionaryGetValue(dict, kCGWindowOwnerPID);
			if (!pidRef) continue;
			pid_t wpid = 0;
			CFNumberGetValue(pidRef, kCFNumberIntType, &wpid);
			if (wpid != targetPid) continue;

			// Check window layer - only consider normal windows (layer 0)
			CFNumberRef layerRef = (CFNumberRef)CFDictionaryGetValue(dict, kCGWindowLayer);
			if (layerRef) {
				int layer = 0;
				CFNumberGetValue(layerRef, kCFNumberIntType, &layer);
				if (layer != 0) continue;
			}

			// Get window bounds
			CFDictionaryRef boundsDict = (CFDictionaryRef)CFDictionaryGetValue(dict, kCGWindowBounds);
			if (!boundsDict) continue;
			CGRect rect;
			if (CGRectMakeWithDictionaryRepresentation(boundsDict, &rect)) {
				// Skip tiny windows
				if (rect.size.width >= 200 && rect.size.height >= 200) {
					*outFrame = rect;
					found = true;
				}
			}
		}
		CFRelease(windowList);
		return found;
	}
}

// Get current mouse position in CG coordinate system
static CGPoint winsnap_get_mouse_position() {
	CGEventRef event = CGEventCreate(NULL);
	CGPoint pos = CGEventGetLocation(event);
	CFRelease(event);
	return pos;
}

// Move mouse to specified position
static void winsnap_move_mouse(CGFloat x, CGFloat y) {
	CGPoint point = CGPointMake(x, y);
	CGEventSourceRef source = CGEventSourceCreate(kCGEventSourceStateHIDSystemState);
	if (!source) return;

	CGEventRef moveEvent = CGEventCreateMouseEvent(source, kCGEventMouseMoved, point, kCGMouseButtonLeft);
	CGEventSetTimestamp(moveEvent, current_uptime_nsec());
	CGEventPost(kCGHIDEventTap, moveEvent);
	CFRelease(moveEvent);
	CFRelease(source);
}

// Simulate mouse click at screen coordinates
// x, y are in CG coordinate system (origin at top-left of primary display)
// restorePosition: if true, restore mouse to original position after click
static void winsnap_simulate_click_with_restore(CGFloat x, CGFloat y, bool restorePosition) {
	// Save original mouse position
	CGPoint origPos = winsnap_get_mouse_position();

	CGPoint point = CGPointMake(x, y);
	CGEventSourceRef source = CGEventSourceCreate(kCGEventSourceStateHIDSystemState);
	if (!source) return;

	// Create mouse down event
	CGEventRef mouseDown = CGEventCreateMouseEvent(source, kCGEventLeftMouseDown, point, kCGMouseButtonLeft);
	CGEventSetTimestamp(mouseDown, current_uptime_nsec());
	CGEventPost(kCGHIDEventTap, mouseDown);
	CFRelease(mouseDown);

	usleep(50000); // 50ms between down and up

	// Create mouse up event
	CGEventRef mouseUp = CGEventCreateMouseEvent(source, kCGEventLeftMouseUp, point, kCGMouseButtonLeft);
	CGEventSetTimestamp(mouseUp, current_uptime_nsec());
	CGEventPost(kCGHIDEventTap, mouseUp);
	CFRelease(mouseUp);

	CFRelease(source);

	// Restore mouse position after click
	if (restorePosition) {
		usleep(50000); // 50ms delay before restoring
		winsnap_move_mouse(origPos.x, origPos.y);
	}
}

// Legacy function for compatibility
static void winsnap_simulate_click(CGFloat x, CGFloat y) {
	winsnap_simulate_click_with_restore(x, y, false);
}

// Click on the input area of a target app window
// offsetX: pixels from left edge (0 = center horizontally)
// offsetY: pixels from bottom (0 = use default 120)
// Returns true if click was performed
// Note: This function restores the mouse position after clicking
static bool winsnap_click_input_area(const char *name, int offsetX, int offsetY) {
	CGRect frame;
	if (!winsnap_get_app_window_frame(name, &frame)) {
		return false;
	}

	// Default offset from bottom for input box
	int clickOffsetY = offsetY > 0 ? offsetY : 120;

	// Calculate click position
	CGFloat x, y;
	if (offsetX > 0) {
		x = frame.origin.x + offsetX;
		// Make sure within window
		if (x > frame.origin.x + frame.size.width - 10) {
			x = frame.origin.x + frame.size.width / 2;
		}
	} else {
		// Center horizontally
		x = frame.origin.x + frame.size.width / 2;
	}

	// Y from bottom of window
	y = frame.origin.y + frame.size.height - clickOffsetY;
	// Make sure within window
	if (y < frame.origin.y + 100) {
		y = frame.origin.y + frame.size.height / 2;
	}

	// Click and restore mouse position (same behavior as Windows)
	winsnap_simulate_click_with_restore(x, y, true);
	return true;
}
*/
import "C"

import (
	"errors"
	"time"
	"unsafe"
)

// SendTextToTarget sends text to the target application by:
// 1. Copying text to clipboard
// 2. Activating target window
// 3. Clicking on input area to focus (unless noClick is true)
// 4. Simulating Cmd+V to paste
// 5. Optionally simulating Enter or Cmd+Enter to send (directly to target process)
func SendTextToTarget(targetProcess string, text string, triggerSend bool, sendKeyStrategy string, noClick bool, clickOffsetX, clickOffsetY int) error {
	if targetProcess == "" {
		return errors.New("winsnap: target process is empty")
	}
	if text == "" {
		return errors.New("winsnap: text is empty")
	}

	// Copy text to clipboard
	cText := C.CString(text)
	defer C.free(unsafe.Pointer(cText))
	if !C.winsnap_set_clipboard_text(cText) {
		return errors.New("winsnap: failed to set clipboard text")
	}

	// Activate target app and get its PID for direct key sending
	targetName := normalizeMacTargetName(targetProcess)
	cName := C.CString(targetName)
	defer C.free(unsafe.Pointer(cName))
	if !C.winsnap_activate_app(cName) {
		return ErrTargetWindowNotFound
	}

	// Get target PID for direct key sending (some apps filter synthetic events)
	targetPid := C.winsnap_get_pid_by_name(cName)

	time.Sleep(150 * time.Millisecond)

	// Click on input area to focus (unless noClick is true)
	// This is needed because most apps don't auto-focus the input box when activated
	if !noClick {
		C.winsnap_click_input_area(cName, C.int(clickOffsetX), C.int(clickOffsetY))
		time.Sleep(150 * time.Millisecond)
	}

	// Simulate Cmd+V to paste
	C.winsnap_simulate_cmd_v()
	time.Sleep(100 * time.Millisecond)

	// Optionally trigger send - use CGEventPostToPid to send directly to target process
	// This bypasses app-level event filtering that some apps (like DingTalk) use
	if triggerSend {
		time.Sleep(150 * time.Millisecond)
		// On macOS, most apps use Enter to send, but some use Cmd+Enter
		// We'll use the same strategy names but map ctrl_enter to cmd_enter
		if sendKeyStrategy == "ctrl_enter" {
			C.winsnap_simulate_cmd_enter_to_pid(targetPid)
		} else {
			C.winsnap_simulate_enter_to_pid(targetPid)
		}
	}

	return nil
}

// PasteTextToTarget sends text to the target application's edit box without triggering send.
// noClick and clickOffsetX/Y are ignored on macOS as focus handling is different
func PasteTextToTarget(targetProcess string, text string, noClick bool, clickOffsetX, clickOffsetY int) error {
	return SendTextToTarget(targetProcess, text, false, "", noClick, clickOffsetX, clickOffsetY)
}
