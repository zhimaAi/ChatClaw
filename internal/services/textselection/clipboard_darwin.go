//go:build darwin && cgo

package textselection

/*
#cgo darwin CFLAGS: -x objective-c -fobjc-arc
#cgo darwin LDFLAGS: -framework Cocoa -framework ApplicationServices

#import <Cocoa/Cocoa.h>
#import <ApplicationServices/ApplicationServices.h>

// Get clipboard text
static const char* getClipboardText() {
	@autoreleasepool {
		NSPasteboard *pasteboard = [NSPasteboard generalPasteboard];
		NSString *text = [pasteboard stringForType:NSPasteboardTypeString];
		if (text == nil) {
			return NULL;
		}
		return strdup([text UTF8String]);
	}
}

// Get clipboard change count
static NSInteger getClipboardChangeCount() {
	return [[NSPasteboard generalPasteboard] changeCount];
}

// Get cursor position (returns pixel coordinates relative to screen, top-left origin)
// Reference floatingball's coordinate transformation logic
static void getCursorPosition(int *x, int *y) {
	// Get Cocoa mouse position (points, bottom-left origin, global coordinates)
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

	// Get screen parameters
	NSRect frame = screen.frame;
	CGFloat scale = screen.backingScaleFactor;

	// Calculate coordinates relative to screen top-left (Cocoa Y is bottom-left origin, need to flip)
	// screenTopY = frame.origin.y + frame.size.height (screen top's Cocoa Y coordinate)
	CGFloat screenTopY = frame.origin.y + frame.size.height;
	CGFloat relativeX = mouseLoc.x - frame.origin.x;
	CGFloat relativeY = screenTopY - mouseLoc.y;  // Flip Y

	// Convert to pixels
	*x = (int)(relativeX * scale);
	*y = (int)(relativeY * scale);
}

// Get screen scale factor
static double getScreenScale() {
	// Get scale factor of screen containing mouse
	NSPoint mouseLoc = [NSEvent mouseLocation];
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
	return screen.backingScaleFactor;
}

// Free C string
static void freeString(char *str) {
	if (str) free(str);
}
*/
import "C"

import (
	"sync"
	"time"
	"unsafe"
)

// ClipboardWatcher macOS clipboard watcher.
type ClipboardWatcher struct {
	mu              sync.Mutex
	callback        func(text string, x, y int32)
	closed          bool
	ready           chan struct{}
	lastChangeCount int64
	stopCh          chan struct{}
}

// NewClipboardWatcher creates a new clipboard watcher.
func NewClipboardWatcher(callback func(text string, x, y int32)) *ClipboardWatcher {
	return &ClipboardWatcher{
		callback: callback,
		ready:    make(chan struct{}),
	}
}

// Start starts the watcher.
func (w *ClipboardWatcher) Start() error {
	w.mu.Lock()
	w.stopCh = make(chan struct{})
	w.lastChangeCount = int64(C.getClipboardChangeCount())
	w.mu.Unlock()

	go w.pollClipboard()
	close(w.ready)
	return nil
}

// Stop stops the watcher.
func (w *ClipboardWatcher) Stop() {
	w.mu.Lock()
	if w.closed {
		w.mu.Unlock()
		return
	}
	w.closed = true
	if w.stopCh != nil {
		close(w.stopCh)
	}
	w.mu.Unlock()
}

func (w *ClipboardWatcher) pollClipboard() {
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-w.stopCh:
			return
		case <-ticker.C:
			w.checkClipboard()
		}
	}
}

func (w *ClipboardWatcher) checkClipboard() {
	currentCount := int64(C.getClipboardChangeCount())

	w.mu.Lock()
	lastCount := w.lastChangeCount
	callback := w.callback
	w.mu.Unlock()

	if currentCount != lastCount {
		w.mu.Lock()
		w.lastChangeCount = currentCount
		w.mu.Unlock()

		// Get clipboard text
		cText := C.getClipboardText()
		if cText != nil {
			text := C.GoString(cText)
			C.freeString(cText)

			if text != "" && callback != nil {
				// Get mouse position
				var cx, cy C.int
				C.getCursorPosition(&cx, &cy)
				callback(text, int32(cx), int32(cy))
			}
		}
	}
}

// GetCursorPos gets the current mouse position.
func GetCursorPos() (x, y int32) {
	var cx, cy C.int
	C.getCursorPosition(&cx, &cy)
	return int32(cx), int32(cy)
}

// getDPIScale gets the scale factor on macOS (Retina display).
func getDPIScale() float64 {
	return float64(C.getScreenScale())
}

// cgo helper - avoid unused import
var _ = unsafe.Pointer(nil)
