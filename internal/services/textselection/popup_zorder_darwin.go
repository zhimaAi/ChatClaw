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

// Show popup at the correct position for multi-monitor setups.
// This function does everything in a single main-thread dispatch to avoid
// race conditions with Wails' Show() method:
//   1. Find the screen containing the mouse position
//   2. Clamp the popup to that screen's visible frame
//   3. Set the window frame (position + size)
//   4. Set window level to popup menu level
//   5. Order front without activating
//
// Parameters:
//   mouseX/mouseY: mouse position in Cocoa points (global, Y from bottom)
//   popWidth/popHeight: popup dimensions in Cocoa points
static void textselection_show_popup_clamped(void *nsWindow, int mouseX, int mouseY, int popWidth, int popHeight) {
	if (!nsWindow) return;

	dispatch_async(dispatch_get_main_queue(), ^{
		NSWindow *win = (__bridge NSWindow *)nsWindow;
		if (!win || ![win isKindOfClass:[NSWindow class]]) return;

		// Find the screen containing the mouse position
		NSPoint mousePt = NSMakePoint((CGFloat)mouseX, (CGFloat)mouseY);
		NSScreen *screen = nil;
		for (NSScreen *s in [NSScreen screens]) {
			if (NSPointInRect(mousePt, s.frame)) {
				screen = s;
				break;
			}
		}
		if (screen == nil) {
			screen = [NSScreen mainScreen];
		}

		// Get screen's visible frame (excludes menu bar and dock)
		NSRect visibleFrame = [screen visibleFrame];

		// Calculate popup position: centered above mouse
		// In Cocoa coordinate system, Y increases upward, so "above" = mouseY + offset
		CGFloat popX = (CGFloat)mouseX - (CGFloat)popWidth / 2.0;
		CGFloat popY = (CGFloat)mouseY + 10.0;

		// Clamp to visible frame
		if (popX < NSMinX(visibleFrame)) {
			popX = NSMinX(visibleFrame);
		}
		if (popX + popWidth > NSMaxX(visibleFrame)) {
			popX = NSMaxX(visibleFrame) - popWidth;
		}
		if (popY + popHeight > NSMaxY(visibleFrame)) {
			popY = NSMaxY(visibleFrame) - popHeight;
		}
		if (popY < NSMinY(visibleFrame)) {
			// If below screen bottom, show below mouse instead
			popY = (CGFloat)mouseY - (CGFloat)popHeight - 10.0;
		}

		NSLog(@"[POPUP-DEBUG-MAC] screen=%@ visibleFrame=(%g,%g,%g,%g) mouse=(%d,%d) final=(%g,%g) size=(%d,%d)",
			screen.localizedName,
			NSMinX(visibleFrame), NSMinY(visibleFrame),
			NSWidth(visibleFrame), NSHeight(visibleFrame),
			mouseX, mouseY, popX, popY, popWidth, popHeight);

		// Set window frame (position + size in one call)
		NSRect frame = NSMakeRect(popX, popY, (CGFloat)popWidth, (CGFloat)popHeight);
		[win setFrame:frame display:YES];

		// Force topmost without activating
		[win setLevel:NSPopUpMenuWindowLevel];
		[win orderFrontRegardless];
	});
}
*/
import "C"

import "github.com/wailsapp/wails/v3/pkg/application"

// showPopupNative shows the popup window.
// On macOS, w.Show() is safe — focus is managed via orderFrontRegardless in Cocoa.
func showPopupNative(w *application.WebviewWindow) {
	if w == nil {
		return
	}
	w.Show()
}

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

// showPopupClampedCocoa shows the popup window at the correct screen position with clamping.
// This performs screen detection, clamping, positioning, and topmost in a single atomic
// main-thread operation, avoiding race conditions with Wails' Show() method.
// mouseX/mouseY: mouse position in Cocoa points (global, Y from bottom)
// popWidth/popHeight: popup dimensions in Cocoa points
func showPopupClampedCocoa(w *application.WebviewWindow, mouseX, mouseY, popWidth, popHeight int) {
	if w == nil {
		return
	}
	nativeHandle := w.NativeWindow()
	if nativeHandle == nil {
		return
	}
	C.textselection_show_popup_clamped(nativeHandle, C.int(mouseX), C.int(mouseY), C.int(popWidth), C.int(popHeight))
}
