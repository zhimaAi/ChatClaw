//go:build darwin && cgo

package winsnap

/*
#cgo darwin CFLAGS: -x objective-c -fobjc-arc
#cgo darwin LDFLAGS: -framework Cocoa -framework Carbon

#import <Cocoa/Cocoa.h>
#import <Carbon/Carbon.h>

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
	CGEventPost(kCGHIDEventTap, keyDown);
	CFRelease(keyDown);

	usleep(10000); // 10ms

	// Create key up event for V with Cmd modifier
	CGEventRef keyUp = CGEventCreateKeyboardEvent(source, (CGKeyCode)kVK_ANSI_V, false);
	CGEventSetFlags(keyUp, kCGEventFlagMaskCommand);
	CGEventPost(kCGHIDEventTap, keyUp);
	CFRelease(keyUp);

	CFRelease(source);
}

// Simulate Enter key
static void winsnap_simulate_enter() {
	CGEventSourceRef source = CGEventSourceCreate(kCGEventSourceStateHIDSystemState);
	if (!source) return;

	CGEventRef keyDown = CGEventCreateKeyboardEvent(source, (CGKeyCode)kVK_Return, true);
	CGEventPost(kCGHIDEventTap, keyDown);
	CFRelease(keyDown);

	usleep(10000); // 10ms

	CGEventRef keyUp = CGEventCreateKeyboardEvent(source, (CGKeyCode)kVK_Return, false);
	CGEventPost(kCGHIDEventTap, keyUp);
	CFRelease(keyUp);

	CFRelease(source);
}

// Simulate Cmd+Enter
static void winsnap_simulate_cmd_enter() {
	CGEventSourceRef source = CGEventSourceCreate(kCGEventSourceStateHIDSystemState);
	if (!source) return;

	CGEventRef keyDown = CGEventCreateKeyboardEvent(source, (CGKeyCode)kVK_Return, true);
	CGEventSetFlags(keyDown, kCGEventFlagMaskCommand);
	CGEventPost(kCGHIDEventTap, keyDown);
	CFRelease(keyDown);

	usleep(10000); // 10ms

	CGEventRef keyUp = CGEventCreateKeyboardEvent(source, (CGKeyCode)kVK_Return, false);
	CGEventSetFlags(keyUp, kCGEventFlagMaskCommand);
	CGEventPost(kCGHIDEventTap, keyUp);
	CFRelease(keyUp);

	CFRelease(source);
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
// 3. Simulating Cmd+V to paste
// 4. Optionally simulating Enter or Cmd+Enter to send
func SendTextToTarget(targetProcess string, text string, triggerSend bool, sendKeyStrategy string) error {
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

	// Activate target app
	targetName := normalizeMacTargetName(targetProcess)
	cName := C.CString(targetName)
	defer C.free(unsafe.Pointer(cName))
	if !C.winsnap_activate_app(cName) {
		return ErrTargetWindowNotFound
	}

	time.Sleep(100 * time.Millisecond)

	// Simulate Cmd+V to paste
	C.winsnap_simulate_cmd_v()
	time.Sleep(50 * time.Millisecond)

	// Optionally trigger send
	if triggerSend {
		time.Sleep(100 * time.Millisecond)
		// On macOS, most apps use Enter to send, but some use Cmd+Enter
		// We'll use the same strategy names but map ctrl_enter to cmd_enter
		if sendKeyStrategy == "ctrl_enter" {
			C.winsnap_simulate_cmd_enter()
		} else {
			C.winsnap_simulate_enter()
		}
	}

	return nil
}

// PasteTextToTarget sends text to the target application's edit box without triggering send.
func PasteTextToTarget(targetProcess string, text string) error {
	return SendTextToTarget(targetProcess, text, false, "")
}
