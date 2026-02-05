//go:build darwin && cgo

package textselection

/*
#cgo darwin CFLAGS: -x objective-c -fobjc-arc
#cgo darwin LDFLAGS: -framework Cocoa

#import <Cocoa/Cocoa.h>

// Configure popup window to not activate when shown.
// On macOS, we use NSPanel with NSNonactivatingPanelMask style.
// However, since Wails creates NSWindow, we configure it to behave like a panel.
static void textselection_configure_popup_noactivate(void *nsWindow) {
	if (!nsWindow) return;

	dispatch_async(dispatch_get_main_queue(), ^{
		NSWindow *win = (__bridge NSWindow *)nsWindow;
		if (!win || ![win isKindOfClass:[NSWindow class]]) return;

		// Set window level to floating (above normal windows but below modal panels)
		[win setLevel:NSFloatingWindowLevel];

		// Configure window to not become key or main window
		// This prevents the popup from stealing focus from other apps
		[win setCanBecomeKeyWindow:NO];
		[win setCanBecomeMainWindow:NO];

		// Disable window shadow for a cleaner popup appearance (optional)
		// [win setHasShadow:YES];

		// Set collection behavior to allow the window to be visible in all spaces
		// and not be managed by Mission Control
		[win setCollectionBehavior:NSWindowCollectionBehaviorCanJoinAllSpaces |
		                           NSWindowCollectionBehaviorStationary |
		                           NSWindowCollectionBehaviorIgnoresCycle];
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
