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

// Set popup window position using Cocoa points directly.
// This bypasses Wails' SetPosition which uses [window screen] for conversion,
// causing incorrect positioning when moving between screens with different sizes/DPI.
// Parameters: x/y are global Cocoa coordinates (Y from bottom-left of primary screen).
static void textselection_set_popup_position(void *nsWindow, int x, int y) {
	if (!nsWindow) return;

	NSWindow *win = (__bridge NSWindow *)nsWindow;
	if (!win || ![win isKindOfClass:[NSWindow class]]) return;

	NSRect frame = [win frame];
	frame.origin.x = (CGFloat)x;
	frame.origin.y = (CGFloat)y;
	[win setFrame:frame display:YES];
}
*/
import "C"

import "github.com/wailsapp/wails/v3/pkg/application"

// hidePopupNative hides the popup window using the platform's native hide mechanism.
// On macOS, w.Hide() is safe and reliable.
func hidePopupNative(w *application.WebviewWindow) {
	if w == nil {
		return
	}
	w.Hide()
}

// setPopupPositionPhysical is only used on Windows; no-op on macOS.
func setPopupPositionPhysical(_ *application.WebviewWindow, _, _, _, _ int) {}

// getPopupWindowRect is only used on Windows; returns zero on macOS.
func getPopupWindowRect(_ *application.WebviewWindow) (int32, int32, int32, int32) {
	return 0, 0, 0, 0
}

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

// setPopupPositionCocoa sets the popup window position using Cocoa points directly.
// This correctly handles multi-monitor setups by using Cocoa's unified global coordinate system.
func setPopupPositionCocoa(w *application.WebviewWindow, x, y int) {
	if w == nil {
		return
	}
	nativeHandle := w.NativeWindow()
	if nativeHandle == nil {
		return
	}
	C.textselection_set_popup_position(nativeHandle, C.int(x), C.int(y))
}
