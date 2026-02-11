//go:build darwin && cgo

package textselection

/*
#cgo darwin CFLAGS: -x objective-c -fobjc-arc
#cgo darwin LDFLAGS: -framework Cocoa -framework ApplicationServices

#import <Cocoa/Cocoa.h>
#import <ApplicationServices/ApplicationServices.h>

// Global variables
static CFMachPortRef clickOutsideEventTap = NULL;
static CFRunLoopSourceRef clickOutsideRunLoopSource = NULL;
static CFRunLoopRef clickOutsideTapRunLoop = NULL;

// Popup area
static int popupLeft = 0;
static int popupTop = 0;
static int popupRight = 0;
static int popupBottom = 0;
static bool hasPopupRect = false;

// Go callback declaration
extern void clickOutsideDarwinCallback(int x, int y);

// Set popup area
static void setClickOutsidePopupRect(int left, int top, int right, int bottom) {
	popupLeft = left;
	popupTop = top;
	popupRight = right;
	popupBottom = bottom;
	hasPopupRect = true;
}

// Clear popup area
static void clearClickOutsidePopupRect() {
	hasPopupRect = false;
}

// Event callback
static CGEventRef clickOutsideEventCallback(CGEventTapProxy proxy, CGEventType type, CGEventRef event, void *refcon) {
	if (type == kCGEventTapDisabledByTimeout || type == kCGEventTapDisabledByUserInput) {
		if (clickOutsideEventTap) {
			CGEventTapEnable(clickOutsideEventTap, true);
		}
		return event;
	}

	if (type == kCGEventLeftMouseDown && hasPopupRect) {
		// Get Cocoa mouse position and convert
		NSPoint mouseLoc = [NSEvent mouseLocation];

		// Find screen containing mouse
		NSScreen *screen = nil;
		for (NSScreen *s in [NSScreen screens]) {
			if (NSPointInRect(mouseLoc, s.frame)) {
				screen = s;
				break;
			}
		}
		if (screen == nil) {
			screen = [NSScreen mainScreen];
		}

		// Calculate global pixel coordinates (use primary screen height for Y-flip)
		CGFloat scale = screen.backingScaleFactor;
		CGFloat primaryH = [NSScreen screens][0].frame.size.height;
		int x = (int)(mouseLoc.x * scale);
		int y = (int)((primaryH - mouseLoc.y) * scale);

		// Check if outside popup
		if (x < popupLeft || x > popupRight || y < popupTop || y > popupBottom) {
			dispatch_async(dispatch_get_main_queue(), ^{
				clickOutsideDarwinCallback(x, y);
			});
		}
	}

	return event;
}

// Start event monitoring
static bool startClickOutsideEventTap() {
	CGEventMask eventMask = (1 << kCGEventLeftMouseDown);

	clickOutsideEventTap = CGEventTapCreate(
		kCGSessionEventTap,
		kCGHeadInsertEventTap,
		kCGEventTapOptionListenOnly,
		eventMask,
		clickOutsideEventCallback,
		NULL
	);

	if (!clickOutsideEventTap) {
		NSLog(@"Failed to create click outside event tap");
		return false;
	}

	clickOutsideRunLoopSource = CFMachPortCreateRunLoopSource(kCFAllocatorDefault, clickOutsideEventTap, 0);
	clickOutsideTapRunLoop = CFRunLoopGetCurrent();
	CFRunLoopAddSource(clickOutsideTapRunLoop, clickOutsideRunLoopSource, kCFRunLoopCommonModes);
	CGEventTapEnable(clickOutsideEventTap, true);

	return true;
}

// Stop event monitoring
static void stopClickOutsideEventTap() {
	if (clickOutsideEventTap) {
		CGEventTapEnable(clickOutsideEventTap, false);
		if (clickOutsideRunLoopSource && clickOutsideTapRunLoop) {
			CFRunLoopRemoveSource(clickOutsideTapRunLoop, clickOutsideRunLoopSource, kCFRunLoopCommonModes);
		}
		CFRelease(clickOutsideEventTap);
		clickOutsideEventTap = NULL;
	}
	if (clickOutsideRunLoopSource) {
		CFRelease(clickOutsideRunLoopSource);
		clickOutsideRunLoopSource = NULL;
	}
	clickOutsideTapRunLoop = NULL;
	hasPopupRect = false;
}
*/
import "C"

import (
	"sync"
)

// ClickOutsideWatcher macOS click outside watcher.
type ClickOutsideWatcher struct {
	mu       sync.Mutex
	callback func(x, y int32)
	closed   bool
	ready    chan struct{}
}

var (
	clickOutsideDarwinInstance   *ClickOutsideWatcher
	clickOutsideDarwinInstanceMu sync.Mutex
)

// NewClickOutsideWatcher creates a new click outside watcher.
func NewClickOutsideWatcher(callback func(x, y int32)) *ClickOutsideWatcher {
	return &ClickOutsideWatcher{
		callback: callback,
		ready:    make(chan struct{}),
	}
}

// Start starts the watcher.
func (w *ClickOutsideWatcher) Start() error {
	clickOutsideDarwinInstanceMu.Lock()
	clickOutsideDarwinInstance = w
	clickOutsideDarwinInstanceMu.Unlock()

	go w.run()
	<-w.ready
	return nil
}

// Stop stops the watcher.
func (w *ClickOutsideWatcher) Stop() {
	w.mu.Lock()
	if w.closed {
		w.mu.Unlock()
		return
	}
	w.closed = true
	w.mu.Unlock()

	C.stopClickOutsideEventTap()

	clickOutsideDarwinInstanceMu.Lock()
	if clickOutsideDarwinInstance == w {
		clickOutsideDarwinInstance = nil
	}
	clickOutsideDarwinInstanceMu.Unlock()
}

// SetPopupRect sets the popup area.
func (w *ClickOutsideWatcher) SetPopupRect(x, y, width, height int32) {
	C.setClickOutsidePopupRect(C.int(x), C.int(y), C.int(x+width), C.int(y+height))
}

// ClearPopupRect clears the popup area.
func (w *ClickOutsideWatcher) ClearPopupRect() {
	C.clearClickOutsidePopupRect()
}

func (w *ClickOutsideWatcher) run() {
	if !C.startClickOutsideEventTap() {
		close(w.ready)
		return
	}

	close(w.ready)
	C.CFRunLoopRun()
}

//export clickOutsideDarwinCallback
func clickOutsideDarwinCallback(x, y C.int) {
	clickOutsideDarwinInstanceMu.Lock()
	w := clickOutsideDarwinInstance
	clickOutsideDarwinInstanceMu.Unlock()

	if w == nil {
		return
	}

	w.mu.Lock()
	callback := w.callback
	w.mu.Unlock()

	if callback != nil {
		callback(int32(x), int32(y))
	}
}
