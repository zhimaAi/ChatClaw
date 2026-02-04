//go:build darwin && cgo

package textselection

/*
#cgo darwin CFLAGS: -x objective-c -fobjc-arc
#cgo darwin LDFLAGS: -framework Cocoa -framework ApplicationServices -framework Carbon

#import <Cocoa/Cocoa.h>
#import <ApplicationServices/ApplicationServices.h>
#import <Carbon/Carbon.h>

// Global variables
static CFMachPortRef mouseEventTap = NULL;
static CFRunLoopSourceRef runLoopSource = NULL;
static CFRunLoopRef tapRunLoop = NULL;
static bool isDragging = false;
static CGPoint dragStartPoint;
static int dragDistanceThreshold = 5;

// Go callback declarations
extern void mouseHookDarwinCallback(int x, int y);
extern void mouseHookDarwinShowPopup(int x, int y, int originalAppPid);
extern void mouseHookDarwinDragStartCallback(int x, int y);
extern void mouseHookDarwinLogCallback(int distanceInt, int thresholdSq, int passed);
extern void mouseHookDarwinMouseDownCallback(int x, int y);
extern void mouseHookDarwinTapDisabledCallback(int reason);

// Activate app by PID
static void activateAppByPid(pid_t pid) {
	NSRunningApplication *app = [NSRunningApplication runningApplicationWithProcessIdentifier:pid];
	if (app) {
		[app activateWithOptions:NSApplicationActivateIgnoringOtherApps];
	}
}

// Get clipboard text
static const char* mouseHookGetClipboardText() {
	@autoreleasepool {
		NSPasteboard *pasteboard = [NSPasteboard generalPasteboard];
		NSString *text = [pasteboard stringForType:NSPasteboardTypeString];
		if (text == nil) {
			return NULL;
		}
		return strdup([text UTF8String]);
	}
}

// Simulate Cmd+C (with small delay between keys for better compatibility)
static void simulateCmdC() {
	CGEventSourceRef source = CGEventSourceCreate(kCGEventSourceStateHIDSystemState);
	if (!source) return;

	CGEventRef cmdDown = CGEventCreateKeyboardEvent(source, kVK_Command, true);
	CGEventRef cDown = CGEventCreateKeyboardEvent(source, kVK_ANSI_C, true);
	CGEventRef cUp = CGEventCreateKeyboardEvent(source, kVK_ANSI_C, false);
	CGEventRef cmdUp = CGEventCreateKeyboardEvent(source, kVK_Command, false);

	CGEventSetFlags(cDown, kCGEventFlagMaskCommand);
	CGEventSetFlags(cUp, kCGEventFlagMaskCommand);

	// Add delay between keys for better app compatibility
	CGEventPost(kCGHIDEventTap, cmdDown);
	usleep(10000); // 10ms
	CGEventPost(kCGHIDEventTap, cDown);
	usleep(10000); // 10ms
	CGEventPost(kCGHIDEventTap, cUp);
	usleep(10000); // 10ms
	CGEventPost(kCGHIDEventTap, cmdUp);

	CFRelease(cmdDown);
	CFRelease(cDown);
	CFRelease(cUp);
	CFRelease(cmdUp);
	CFRelease(source);
}

// Event callback
static CGEventRef mouseEventCallback(CGEventTapProxy proxy, CGEventType type, CGEventRef event, void *refcon) {
	if (type == kCGEventTapDisabledByTimeout || type == kCGEventTapDisabledByUserInput) {
		// Log tap disabled event
		int reason = (type == kCGEventTapDisabledByTimeout) ? 1 : 2;
		dispatch_async(dispatch_get_main_queue(), ^{
			mouseHookDarwinTapDisabledCallback(reason);
		});
		// Re-enable tap
		if (mouseEventTap) {
			CGEventTapEnable(mouseEventTap, true);
		}
		return event;
	}

	CGPoint location = CGEventGetLocation(event);

	switch (type) {
		case kCGEventLeftMouseDown: {
			isDragging = true;
			dragStartPoint = location;
			// Get Cocoa coordinates and convert to screen pixel coordinates
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
			NSRect frame = screen.frame;
			CGFloat scale = screen.backingScaleFactor;
			CGFloat screenTopY = frame.origin.y + frame.size.height;
			int mouseX = (int)((mouseLoc.x - frame.origin.x) * scale);
			int mouseY = (int)((screenTopY - mouseLoc.y) * scale);

			// Log mouse down event
			dispatch_async(dispatch_get_main_queue(), ^{
				mouseHookDarwinMouseDownCallback(mouseX, mouseY);
			});

			// Notify drag start with mouse position
			dispatch_async(dispatch_get_main_queue(), ^{
				mouseHookDarwinDragStartCallback(mouseX, mouseY);
			});
			break;
		}

		case kCGEventLeftMouseUp:
			if (isDragging) {
				CGFloat dx = location.x - dragStartPoint.x;
				CGFloat dy = location.y - dragStartPoint.y;
				CGFloat distance = dx*dx + dy*dy;
				int thresholdSq = dragDistanceThreshold * dragDistanceThreshold;

				// Log drag distance
				int distInt = (int)distance;
				int passedCheck = (distance > thresholdSq) ? 1 : 0;
				dispatch_async(dispatch_get_main_queue(), ^{
					mouseHookDarwinLogCallback(distInt, thresholdSq, passedCheck);
				});

		if (distance > thresholdSq) {
			// Possibly selecting text, delay processing
			dispatch_after(dispatch_time(DISPATCH_TIME_NOW, 50 * NSEC_PER_MSEC), dispatch_get_main_queue(), ^{
				// Check if current focus app is our own
				// If so, frontend JavaScript already handles it
				NSRunningApplication *frontApp = [[NSWorkspace sharedWorkspace] frontmostApplication];
				NSRunningApplication *selfApp = [NSRunningApplication currentApplication];
				if ([frontApp.bundleIdentifier isEqualToString:selfApp.bundleIdentifier]) {
					// Current focus is our app, skip (frontend handles it)
					return;
				}

				// Record original app's PID for later wakeup
				pid_t originalAppPid = frontApp.processIdentifier;

				// Get Cocoa mouse position
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

				// Calculate pixel coordinates relative to screen top-left
				NSRect frame = screen.frame;
				CGFloat scale = screen.backingScaleFactor;
				CGFloat screenTopY = frame.origin.y + frame.size.height;
				int x = (int)((mouseLoc.x - frame.origin.x) * scale);
				int y = (int)((screenTopY - mouseLoc.y) * scale);

				// Use callback (copy then show popup mode)
				mouseHookDarwinCallback(x, y);
			});
		}
			}
			isDragging = false;
			break;

		default:
			break;
	}

	return event;
}

// Start mouse event monitoring
static bool startMouseEventTap() {
	CGEventMask eventMask = (1 << kCGEventLeftMouseDown) | (1 << kCGEventLeftMouseUp);

	mouseEventTap = CGEventTapCreate(
		kCGSessionEventTap,
		kCGHeadInsertEventTap,
		kCGEventTapOptionListenOnly,
		eventMask,
		mouseEventCallback,
		NULL
	);

	if (!mouseEventTap) {
		NSLog(@"Failed to create event tap. Check Accessibility permissions.");
		return false;
	}

	runLoopSource = CFMachPortCreateRunLoopSource(kCFAllocatorDefault, mouseEventTap, 0);
	tapRunLoop = CFRunLoopGetCurrent();
	CFRunLoopAddSource(tapRunLoop, runLoopSource, kCFRunLoopCommonModes);
	CGEventTapEnable(mouseEventTap, true);

	return true;
}

// Stop mouse event monitoring
static void stopMouseEventTap() {
	if (mouseEventTap) {
		CGEventTapEnable(mouseEventTap, false);
		if (runLoopSource && tapRunLoop) {
			CFRunLoopRemoveSource(tapRunLoop, runLoopSource, kCFRunLoopCommonModes);
		}
		CFRelease(mouseEventTap);
		mouseEventTap = NULL;
	}
	if (runLoopSource) {
		CFRelease(runLoopSource);
		runLoopSource = NULL;
	}
	tapRunLoop = NULL;
}

static void freeMouseHookString(char *str) {
	if (str) free(str);
}
*/
import "C"

import (
	"sync"
	"time"
)

// MouseHookWatcher macOS global mouse hook.
type MouseHookWatcher struct {
	mu                sync.Mutex
	callback          func(text string, x, y int32)          // Old callback (with text), kept for compatibility
	showPopupCallback func(x, y int32, originalAppPid int32) // New callback: only show popup, record original app
	onDragStart       func(x, y int32)                       // Callback when drag starts
	closed            bool
	ready             chan struct{}
	stopCh            chan struct{}
}

var (
	mouseHookDarwinInstance   *MouseHookWatcher
	mouseHookDarwinInstanceMu sync.Mutex
)

// NewMouseHookWatcher creates a new mouse hook watcher.
// showPopupCallback: new design - only show popup, don't copy. Copy on button click.
func NewMouseHookWatcher(
	callback func(text string, x, y int32),
	onDragStart func(x, y int32),
	showPopupCallback func(x, y int32, originalAppPid int32),
) *MouseHookWatcher {
	return &MouseHookWatcher{
		callback:          callback,
		onDragStart:       onDragStart,
		showPopupCallback: showPopupCallback,
		ready:             make(chan struct{}),
	}
}

// Start starts the watcher.
func (w *MouseHookWatcher) Start() error {
	mouseHookDarwinInstanceMu.Lock()
	mouseHookDarwinInstance = w
	mouseHookDarwinInstanceMu.Unlock()

	w.mu.Lock()
	w.stopCh = make(chan struct{})
	w.mu.Unlock()

	go w.run()
	<-w.ready
	return nil
}

// Stop stops the watcher.
func (w *MouseHookWatcher) Stop() {
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

	C.stopMouseEventTap()

	mouseHookDarwinInstanceMu.Lock()
	if mouseHookDarwinInstance == w {
		mouseHookDarwinInstance = nil
	}
	mouseHookDarwinInstanceMu.Unlock()
}

func (w *MouseHookWatcher) run() {
	started := C.startMouseEventTap()
	if !started {
		close(w.ready)
		return
	}

	close(w.ready)

	// Run RunLoop
	C.CFRunLoopRun()
}

//export mouseHookDarwinDragStartCallback
func mouseHookDarwinDragStartCallback(x, y C.int) {
	mouseHookDarwinInstanceMu.Lock()
	w := mouseHookDarwinInstance
	mouseHookDarwinInstanceMu.Unlock()

	if w == nil {
		return
	}

	w.mu.Lock()
	onDragStart := w.onDragStart
	w.mu.Unlock()

	if onDragStart != nil {
		onDragStart(int32(x), int32(y))
	}
}

//export mouseHookDarwinLogCallback
func mouseHookDarwinLogCallback(distanceInt C.int, thresholdSq C.int, passed C.int) {
	// Log callback (for debugging, empty implementation now)
}

//export mouseHookDarwinMouseDownCallback
func mouseHookDarwinMouseDownCallback(x C.int, y C.int) {
	// mouseDown event callback (for debugging, empty implementation now)
}

//export mouseHookDarwinTapDisabledCallback
func mouseHookDarwinTapDisabledCallback(reason C.int) {
	// tap disabled callback (for debugging, empty implementation now)
}

//export mouseHookDarwinShowPopup
func mouseHookDarwinShowPopup(x, y, originalAppPid C.int) {
	mouseHookDarwinInstanceMu.Lock()
	w := mouseHookDarwinInstance
	mouseHookDarwinInstanceMu.Unlock()

	if w == nil {
		return
	}

	// Check if using new mode (show popup then copy)
	w.mu.Lock()
	showPopupCallback := w.showPopupCallback
	w.mu.Unlock()

	if showPopupCallback != nil {
		// New mode: only show popup, don't execute Cmd+C
		showPopupCallback(int32(x), int32(y), int32(originalAppPid))
	} else {
		// Old mode: copy then show popup
		mouseHookDarwinCallback(x, y)
	}
}

//export mouseHookDarwinCallback
func mouseHookDarwinCallback(x, y C.int) {
	mouseHookDarwinInstanceMu.Lock()
	w := mouseHookDarwinInstance
	mouseHookDarwinInstanceMu.Unlock()

	if w == nil {
		return
	}

	// Save old clipboard content
	oldClipboard := ""
	cOld := C.mouseHookGetClipboardText()
	if cOld != nil {
		oldClipboard = C.GoString(cOld)
		C.freeMouseHookString(cOld)
	}

	// Try multiple times to simulate Cmd+C (some apps may need more time to respond)
	var newClipboard string
	for attempt := 1; attempt <= 3; attempt++ {
		// Simulate Cmd+C
		C.simulateCmdC()

		// Wait for clipboard to update (gradually increase wait time)
		time.Sleep(time.Duration(100*attempt) * time.Millisecond)

		// Get new clipboard content
		cNew := C.mouseHookGetClipboardText()
		if cNew != nil {
			newClipboard = C.GoString(cNew)
			C.freeMouseHookString(cNew)

			if newClipboard != "" && newClipboard != oldClipboard {
				break
			}
		}
	}

	if newClipboard != "" && newClipboard != oldClipboard {
		w.mu.Lock()
		callback := w.callback
		w.mu.Unlock()

		if callback != nil {
			callback(newClipboard, int32(x), int32(y))
		}
	}
}

// ==================== Helper functions for service.go to call ====================

// activateAppByPidDarwin activates app by PID.
func activateAppByPidDarwin(pid int32) {
	C.activateAppByPid(C.int(pid))
}

// simulateCmdCDarwin simulates Cmd+C.
func simulateCmdCDarwin() {
	C.simulateCmdC()
}

// getClipboardTextDarwin gets clipboard text.
func getClipboardTextDarwin() string {
	cText := C.mouseHookGetClipboardText()
	if cText == nil {
		return ""
	}
	defer C.freeMouseHookString(cText)
	return C.GoString(cText)
}

// Windows function stubs (not used on macOS)
func simulateCtrlCWindows()           {}
func getClipboardTextWindows() string { return "" }
