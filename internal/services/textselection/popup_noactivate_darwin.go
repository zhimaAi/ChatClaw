//go:build darwin && cgo

package textselection

/*
#cgo darwin CFLAGS: -x objective-c -fobjc-arc
#cgo darwin LDFLAGS: -framework Cocoa

#import <Cocoa/Cocoa.h>

// Configure popup window to not activate when shown.
// On macOS, we set the window level to floating and configure collection behavior.
// Note: canBecomeKeyWindow and canBecomeMainWindow are read-only properties,
// so we cannot set them directly. Instead, we rely on window level and
// orderFrontRegardless to achieve similar behavior.
static void textselection_configure_popup_noactivate(void *nsWindow) {
	if (!nsWindow) return;

	dispatch_async(dispatch_get_main_queue(), ^{
		NSWindow *win = (__bridge NSWindow *)nsWindow;
		if (!win || ![win isKindOfClass:[NSWindow class]]) return;

		// Set window level to floating (above normal windows but below modal panels)
		// This helps the popup stay visible without activating
		[win setLevel:NSFloatingWindowLevel];

		// Set collection behavior to allow the window to be visible in all spaces
		// and not be managed by Mission Control
		[win setCollectionBehavior:NSWindowCollectionBehaviorCanJoinAllSpaces |
		                           NSWindowCollectionBehaviorStationary |
		                           NSWindowCollectionBehaviorIgnoresCycle];

		// Disable the window from appearing in the window menu
		[win setExcludedFromWindowsMenu:YES];
	});
}
*/
import "C"

import "github.com/wailsapp/wails/v3/pkg/application"

// tryConfigurePopupNoActivate configures the popup window to not steal focus on macOS.
// This is called after creating the popup window.
func tryConfigurePopupNoActivate(w *application.WebviewWindow) {
	if w == nil {
		return
	}
	nativeHandle := w.NativeWindow()
	if nativeHandle == nil {
		return
	}
	C.textselection_configure_popup_noactivate(nativeHandle)
}

// removePopupSubclass is a no-op on macOS (no subclassing needed).
func removePopupSubclass(_ uintptr) {}
