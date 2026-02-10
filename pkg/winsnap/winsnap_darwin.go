//go:build darwin && cgo

package winsnap

/*
#cgo darwin CFLAGS: -x objective-c -fobjc-arc
#cgo darwin LDFLAGS: -framework Cocoa -framework ApplicationServices -framework CoreGraphics

#import <Cocoa/Cocoa.h>
#import <ApplicationServices/ApplicationServices.h>
#import <CoreGraphics/CGWindow.h>
#import <os/lock.h>

typedef struct ScreenInfo {
	int x;
	int y;
	int width;
	int height;
} ScreenInfo;

typedef struct WinsnapFollower {
	pid_t pid;
	int gap;
	void *selfWindow; // NSWindow* (used only during initialization)
	int selfWindowNumber; // Window number for safe lookups after initialization

	AXObserverRef observer;
	AXUIElementRef appElem;
	AXUIElementRef observedWindow;

	CFRunLoopRef runLoop;
	bool stopping;

	// Coalesced target frame updates (AX thread -> main thread)
	os_unfair_lock lock;
	CGPoint latestCocoaOrigin; // Pre-computed Cocoa coordinates for our window
	int latestTargetWindowNumber; // AXWindowNumber of the observed target window
	uint64_t frameGen;
	uint64_t appliedGen;
	bool applyScheduled;

	// Cached coordinate conversion constants (to avoid recomputing on every update)
	CGFloat axOriginX;
	CGFloat axOriginY;
	CGFloat selfWidth;
	CGFloat selfHeight;

	// When true, prefer landscape (w > h) windows over portrait — used for
	// multi-window apps like Douyin where the chat window is landscape.
	bool preferLandscape;

	// Last applied position (for threshold filtering)
	CGPoint lastAppliedOrigin;

	// Observe target app activation to re-assert z-order (keep above target only when target is frontmost)
	void *activationObserver; // token returned by NSNotificationCenter (stored as void* for CGO compatibility)
} WinsnapFollower;

// Helper function to find NSWindow by window number safely
static NSWindow* winsnap_find_window_by_number(int windowNumber) {
	if (windowNumber <= 0) return nil;
	for (NSWindow *win in [NSApp windows]) {
		if ((int)[win windowNumber] == windowNumber) {
			return win;
		}
	}
	return nil;
}

static bool winsnap_get_ax_frame(AXUIElementRef elem, CGRect *outFrame);
// Gets CG window number by matching AX window frame to CGWindowListCopyWindowInfo (macOS has no public kAXWindowNumberAttribute).
static int winsnap_get_ax_window_number(AXUIElementRef win, pid_t pid);
static bool winsnap_is_frontmost_pid(pid_t pid);
static void winsnap_order_above_target(WinsnapFollower *f);
static void winsnap_register_activation_observer(WinsnapFollower *f);
static void winsnap_unregister_activation_observer(WinsnapFollower *f);

static void winsnap_set_err(char **errOut, NSString *msg) {
	if (!errOut) return;
	if (!msg) msg = @"unknown error";
	const char *utf8 = [msg UTF8String];
	if (!utf8) utf8 = "unknown error";
	*errOut = strdup(utf8);
}

static NSString *winsnap_trim(NSString *s) {
	if (!s) return @"";
	return [s stringByTrimmingCharactersInSet:[NSCharacterSet whitespaceAndNewlineCharacterSet]];
}

static NSString *winsnap_normalize_name(const char *name) {
	if (!name) return @"";
	NSString *raw = [NSString stringWithUTF8String:name];
	NSString *t = winsnap_trim(raw);
	if (t.length == 0) return @"";
	// Drop path components if user passes a full path.
	t = [t lastPathComponent];
	// Drop .app suffix if present.
	if ([[t lowercaseString] hasSuffix:@".app"]) {
		t = [t substringToIndex:(t.length - 4)];
	}
	// Drop .exe suffix if user passes Windows-style process name.
	if ([[t lowercaseString] hasSuffix:@".exe"]) {
		t = [t substringToIndex:(t.length - 4)];
	}
	return t;
}

static pid_t winsnap_find_pid_by_name(const char *name) {
	NSString *target = winsnap_normalize_name(name);
	if (target.length == 0) return 0;

	for (NSRunningApplication *app in [[NSWorkspace sharedWorkspace] runningApplications]) {
		if (!app || app.terminated) continue;
		NSString *loc = winsnap_trim(app.localizedName);
		NSString *exe = winsnap_trim(app.executableURL.lastPathComponent);
		NSString *bid = winsnap_trim(app.bundleIdentifier);

		if (loc.length && [loc caseInsensitiveCompare:target] == NSOrderedSame) {
			return app.processIdentifier;
		}
		if (exe.length && [exe caseInsensitiveCompare:target] == NSOrderedSame) {
			return app.processIdentifier;
		}
		if (bid.length && [bid caseInsensitiveCompare:target] == NSOrderedSame) {
			return app.processIdentifier;
		}
	}
	return 0;
}

static bool winsnap_ax_is_trusted(void) {
	NSDictionary *opts = @{(__bridge id)kAXTrustedCheckOptionPrompt: @YES};
	return AXIsProcessTrustedWithOptions((__bridge CFDictionaryRef)opts);
}

static AXUIElementRef winsnap_copy_target_window(AXUIElementRef appElem) {
	if (!appElem) return NULL;
	CFTypeRef val = NULL;

	// Prefer main window.
	if (AXUIElementCopyAttributeValue(appElem, kAXMainWindowAttribute, &val) == kAXErrorSuccess && val) {
		return (AXUIElementRef)val; // already retained
	}
	if (val) CFRelease(val);
	val = NULL;

	// Fallback to focused window.
	if (AXUIElementCopyAttributeValue(appElem, kAXFocusedWindowAttribute, &val) == kAXErrorSuccess && val) {
		return (AXUIElementRef)val;
	}
	if (val) CFRelease(val);
	return NULL;
}

static bool winsnap_window_is_standard(AXUIElementRef win) {
	if (!win) return false;
	CFTypeRef subrole = NULL;
	if (AXUIElementCopyAttributeValue(win, kAXSubroleAttribute, &subrole) != kAXErrorSuccess || !subrole) {
		return false;
	}
	bool ok = (CFGetTypeID(subrole) == CFStringGetTypeID()) &&
		(CFStringCompare((CFStringRef)subrole, kAXStandardWindowSubrole, 0) == kCFCompareEqualTo);
	CFRelease(subrole);
	return ok;
}

static bool winsnap_window_is_visible(AXUIElementRef win) {
	if (!win) return false;
	// Many apps don't expose a "visible" flag. We use "minimized" as a best-effort filter.
	CFTypeRef mini = NULL;
	if (AXUIElementCopyAttributeValue(win, kAXMinimizedAttribute, &mini) != kAXErrorSuccess || !mini) {
		return true;
	}
	bool ok = true;
	if (CFGetTypeID(mini) == CFBooleanGetTypeID()) {
		ok = !CFBooleanGetValue((CFBooleanRef)mini);
	}
	CFRelease(mini);
	return ok;
}

// Pick the best "main chat window" rather than transient dialogs/modals.
// Strategy: choose the largest visible AXStandardWindow from app's AXWindows list.
// When preferLandscape is true, landscape (w > h) windows get a scoring bonus so
// that the chat window is preferred over a portrait video window (e.g. Douyin).
static AXUIElementRef winsnap_copy_best_window_ex(AXUIElementRef appElem, bool preferLandscape) {
	if (!appElem) return NULL;

	CFTypeRef windowsVal = NULL;
	if (AXUIElementCopyAttributeValue(appElem, kAXWindowsAttribute, &windowsVal) == kAXErrorSuccess && windowsVal &&
		CFGetTypeID(windowsVal) == CFArrayGetTypeID()) {
		CFArrayRef arr = (CFArrayRef)windowsVal;
		AXUIElementRef best = NULL;
		double bestScore = -1.0;
		CFIndex n = CFArrayGetCount(arr);
		for (CFIndex i = 0; i < n; i++) {
			AXUIElementRef w = (AXUIElementRef)CFArrayGetValueAtIndex(arr, i);
			if (!w) continue;
			if (!winsnap_window_is_visible(w)) continue;
			if (!winsnap_window_is_standard(w)) continue;
			CGRect fr = CGRectZero;
			if (!winsnap_get_ax_frame(w, &fr)) continue;
			double area = (double)fr.size.width * (double)fr.size.height;
			double score = area;
			// Bonus for landscape windows in multi-window apps (e.g. Douyin chat vs video)
			if (preferLandscape && fr.size.width > fr.size.height) {
				score += 1e9; // Large bonus to ensure landscape wins regardless of area
			}
			if (score > bestScore) {
				bestScore = score;
				best = w;
			}
		}
		if (best) {
			CFRetain(best);
			CFRelease(windowsVal);
			return best;
		}
		CFRelease(windowsVal);
	}

	// Fallback to main/focused when window list is unavailable.
	return winsnap_copy_target_window(appElem);
}

// Convenience wrapper for callers that don't need landscape preference.
static AXUIElementRef winsnap_copy_best_window(AXUIElementRef appElem) {
	return winsnap_copy_best_window_ex(appElem, false);
}

static bool winsnap_get_ax_frame(AXUIElementRef elem, CGRect *outFrame) {
	if (!elem || !outFrame) return false;
	CFTypeRef posVal = NULL;
	CFTypeRef sizeVal = NULL;

	if (AXUIElementCopyAttributeValue(elem, kAXPositionAttribute, &posVal) != kAXErrorSuccess || !posVal) {
		return false;
	}
	if (AXUIElementCopyAttributeValue(elem, kAXSizeAttribute, &sizeVal) != kAXErrorSuccess || !sizeVal) {
		CFRelease(posVal);
		return false;
	}

	CGPoint p = CGPointZero;
	CGSize s = CGSizeZero;
	bool ok1 = AXValueGetValue((AXValueRef)posVal, kAXValueCGPointType, &p);
	bool ok2 = AXValueGetValue((AXValueRef)sizeVal, kAXValueCGSizeType, &s);
	CFRelease(posVal);
	CFRelease(sizeVal);

	if (!ok1 || !ok2) return false;
	*outFrame = (CGRect){ .origin = p, .size = s };
	return true;
}

static int winsnap_get_ax_window_number(AXUIElementRef win, pid_t pid) {
	if (!win || pid <= 0) return 0;
	CGRect axFrame = CGRectZero;
	if (!winsnap_get_ax_frame(win, &axFrame)) return 0;
	CFArrayRef list = CGWindowListCopyWindowInfo(kCGWindowListOptionOnScreenOnly | kCGWindowListExcludeDesktopElements, kCGNullWindowID);
	if (!list) return 0;
	int result = 0;
	CFIndex n = CFArrayGetCount(list);
	for (CFIndex i = 0; i < n; i++) {
		CFDictionaryRef dict = (CFDictionaryRef)CFArrayGetValueAtIndex(list, i);
		CFNumberRef pidRef = (CFNumberRef)CFDictionaryGetValue(dict, kCGWindowOwnerPID);
		if (!pidRef) continue;
		pid_t wpid = 0;
		CFNumberGetValue(pidRef, kCFNumberIntType, &wpid);
		if (wpid != pid) continue;
		CFDictionaryRef bounds = (CFDictionaryRef)CFDictionaryGetValue(dict, kCGWindowBounds);
		if (!bounds) continue;
		CGRect cgRect;
		if (!CGRectMakeWithDictionaryRepresentation(bounds, &cgRect)) continue;
		// Match AX frame (same coordinate system: top-left origin, Y down).
		if (fabs(cgRect.origin.x - axFrame.origin.x) < 2.0 && fabs(cgRect.origin.y - axFrame.origin.y) < 2.0 &&
		    fabs(cgRect.size.width - axFrame.size.width) < 2.0 && fabs(cgRect.size.height - axFrame.size.height) < 2.0) {
			CFNumberRef numRef = (CFNumberRef)CFDictionaryGetValue(dict, kCGWindowNumber);
			if (numRef) CFNumberGetValue(numRef, kCFNumberIntType, &result);
			break;
		}
	}
	CFRelease(list);
	return result;
}

static bool winsnap_is_frontmost_pid(pid_t pid) {
	@autoreleasepool {
		NSRunningApplication *app = [[NSWorkspace sharedWorkspace] frontmostApplication];
		if (!app) return false;
		return app.processIdentifier == pid;
	}
}

static void winsnap_order_above_target(WinsnapFollower *f) {
	if (!f) return;
	if (f->stopping) return;
	if (f->selfWindowNumber <= 0) return;
	if (!winsnap_is_frontmost_pid(f->pid)) return;

	int winNo = 0;
	os_unfair_lock_lock(&f->lock);
	winNo = f->latestTargetWindowNumber;
	os_unfair_lock_unlock(&f->lock);
	if (winNo <= 0) return;

	// Find window by number safely (avoids stale pointer issues)
	NSWindow *selfWin = winsnap_find_window_by_number(f->selfWindowNumber);
	if (!selfWin || ![selfWin isVisible]) return;
	// Order just above the target window (same "normal" level), without activating.
	[selfWin orderWindow:NSWindowAbove relativeTo:winNo];
}

static void winsnap_register_activation_observer(WinsnapFollower *f) {
	if (!f) return;
	if (f->activationObserver) return;
	NSNotificationCenter *nc = [[NSWorkspace sharedWorkspace] notificationCenter];
	id observer = [nc addObserverForName:NSWorkspaceDidActivateApplicationNotification
	                              object:nil
	                               queue:[NSOperationQueue mainQueue]
	                          usingBlock:^(NSNotification *note) {
		(void)note;
		if (!f || f->stopping) return;
		// When the target app becomes frontmost again, re-assert ordering above its window.
		winsnap_order_above_target(f);
	}];
	f->activationObserver = (__bridge_retained void *)observer;
}

static void winsnap_unregister_activation_observer(WinsnapFollower *f) {
	if (!f || !f->activationObserver) return;
	NSNotificationCenter *nc = [[NSWorkspace sharedWorkspace] notificationCenter];
	id observer = (__bridge_transfer id)f->activationObserver;
	[nc removeObserver:observer];
	f->activationObserver = NULL;
}

static void winsnap_sync_to_target(WinsnapFollower *f) {
	if (!f) return;
	if (f->stopping) return;
	if (f->selfWindowNumber <= 0) return;
	if (!f->appElem) return;

	AXUIElementRef win = f->observedWindow;
	if (!win) {
		win = winsnap_copy_best_window_ex(f->appElem, f->preferLandscape);
		if (win) {
			f->observedWindow = win;
		}
	}
	if (!win) return;

	CGRect target = CGRectZero;
	if (!winsnap_get_ax_frame(win, &target)) return;
	int winNo = winsnap_get_ax_window_number(win, f->pid);

	// Convert AX coordinates to Cocoa coordinates in the callback thread (reduces main-thread workload).
	// AX: origin at top-left of primary screen, Y grows downward.
	// Cocoa: origin at bottom-left of primary screen, Y grows upward.
	CGFloat targetCocoaX = f->axOriginX + target.origin.x;
	CGFloat targetCocoaY = f->axOriginY - target.origin.y - target.size.height;

	// Attach our window to the right side, aligned at top.
	// Use target window's height as our window's height.
	CGFloat cocoaX = targetCocoaX + target.size.width + (CGFloat)f->gap;
	CGFloat cocoaY = targetCocoaY;
	CGFloat newHeight = target.size.height;

	// Threshold filter: only update if moved more than 2 pixels (reduces jitter from micro-movements).
	os_unfair_lock_lock(&f->lock);
	CGFloat dx = fabs(cocoaX - f->lastAppliedOrigin.x);
	CGFloat dy = fabs(cocoaY - f->lastAppliedOrigin.y);
	f->latestTargetWindowNumber = winNo;
	if (dx < 2.0 && dy < 2.0 && fabs(newHeight - f->selfHeight) < 2.0 && f->frameGen > 0) {
		// Movement too small, skip update.
		os_unfair_lock_unlock(&f->lock);
		return;
	}

	f->latestCocoaOrigin = CGPointMake(cocoaX, cocoaY);
	f->selfHeight = newHeight; // Update cached height
	f->frameGen++;
	if (f->applyScheduled) {
		os_unfair_lock_unlock(&f->lock);
		return;
	}
	f->applyScheduled = true;
	int selfWinNo = f->selfWindowNumber; // Copy for use in block
	os_unfair_lock_unlock(&f->lock);

	CFRunLoopRef mainRL = CFRunLoopGetMain();
	CFRetain(mainRL);
	CFRunLoopPerformBlock(mainRL, kCFRunLoopCommonModes, ^{
		WinsnapFollower *ff = f;
		if (!ff || ff->stopping) {
			if (ff) {
				os_unfair_lock_lock(&ff->lock);
				ff->applyScheduled = false;
				os_unfair_lock_unlock(&ff->lock);
			}
			CFRelease(mainRL);
			return;
		}

		// Find window by number safely (avoids stale pointer issues)
		NSWindow *selfWin = winsnap_find_window_by_number(selfWinNo);
		if (!selfWin || ![selfWin isVisible]) {
			os_unfair_lock_lock(&ff->lock);
			ff->applyScheduled = false;
			os_unfair_lock_unlock(&ff->lock);
			CFRelease(mainRL);
			return;
		}

		// Apply the pre-computed Cocoa coordinates (all heavy computation done in AX callback thread).
		os_unfair_lock_lock(&ff->lock);
		CGPoint newOrigin = ff->latestCocoaOrigin;
		CGFloat height = ff->selfHeight;
		CGFloat width = ff->selfWidth;
		int targetWinNo = ff->latestTargetWindowNumber;
		uint64_t gen = ff->frameGen;
		ff->appliedGen = gen;
		ff->lastAppliedOrigin = newOrigin;
		os_unfair_lock_unlock(&ff->lock);

		// Set both position and size to match target window height
		NSRect newFrame = NSMakeRect(newOrigin.x, newOrigin.y, width, height);
		[selfWin setFrame:newFrame display:YES animate:NO];

		// Z-order: keep the winsnap window just above the target window,
		// but only when the target application is frontmost (so we don't cover other apps).
		if (targetWinNo > 0 && winsnap_is_frontmost_pid(ff->pid)) {
			[selfWin orderWindow:NSWindowAbove relativeTo:targetWinNo];
		}

		// If more updates arrived while applying, schedule another run.
		bool needResched = false;
		os_unfair_lock_lock(&ff->lock);
		ff->applyScheduled = false;
		needResched = (ff->frameGen != ff->appliedGen);
		os_unfair_lock_unlock(&ff->lock);
		if (needResched) winsnap_sync_to_target(ff);
		CFRelease(mainRL);
	});
	CFRunLoopWakeUp(mainRL);
}

static void winsnap_update_observed_window(WinsnapFollower *f) {
	if (!f || !f->observer || !f->appElem) return;

	AXUIElementRef newWin = winsnap_copy_best_window_ex(f->appElem, f->preferLandscape);
	if (!newWin) return;

	if (f->observedWindow && CFEqual(f->observedWindow, newWin)) {
		CFRelease(newWin);
		return;
	}

	// Remove notifications from old window.
	if (f->observedWindow) {
		AXObserverRemoveNotification(f->observer, f->observedWindow, kAXMovedNotification);
		AXObserverRemoveNotification(f->observer, f->observedWindow, kAXResizedNotification);
		CFRelease(f->observedWindow);
		f->observedWindow = NULL;
	}

	f->observedWindow = newWin; // retained by copy
	AXObserverAddNotification(f->observer, f->observedWindow, kAXMovedNotification, f);
	AXObserverAddNotification(f->observer, f->observedWindow, kAXResizedNotification, f);
}

static void winsnap_ax_callback(AXObserverRef observer, AXUIElementRef element, CFStringRef notification, void *refcon) {
	(void)observer;
	(void)element;
	WinsnapFollower *f = (WinsnapFollower *)refcon;
	if (!f || f->stopping) return;

	if (CFStringCompare(notification, kAXFocusedWindowChangedNotification, 0) == kCFCompareEqualTo) {
		winsnap_update_observed_window(f);
		winsnap_sync_to_target(f);
		return;
	}
	if (CFStringCompare(notification, kAXMainWindowChangedNotification, 0) == kCFCompareEqualTo) {
		winsnap_update_observed_window(f);
		winsnap_sync_to_target(f);
		return;
	}

	if (CFStringCompare(notification, kAXMovedNotification, 0) == kCFCompareEqualTo ||
		CFStringCompare(notification, kAXResizedNotification, 0) == kCFCompareEqualTo) {
		winsnap_sync_to_target(f);
		return;
	}
}

static WinsnapFollower* winsnap_follower_create(void *selfWindow, pid_t pid, int gap, ScreenInfo *screenInfo, bool preferLandscape, char **errOut) {
	if (!selfWindow) {
		winsnap_set_err(errOut, @"winsnap: self window is null");
		return NULL;
	}
	if (pid <= 0) {
		winsnap_set_err(errOut, @"winsnap: invalid pid");
		return NULL;
	}
	if (!winsnap_ax_is_trusted()) {
		winsnap_set_err(errOut, @"winsnap: accessibility permission required");
		return NULL;
	}

	WinsnapFollower *f = (WinsnapFollower *)calloc(1, sizeof(WinsnapFollower));
	f->pid = pid;
	f->gap = gap;
	f->selfWindow = selfWindow;
	f->preferLandscape = preferLandscape;
	f->runLoop = NULL;
	f->stopping = false;
	f->lock = OS_UNFAIR_LOCK_INIT;
	f->latestTargetWindowNumber = 0;
	f->activationObserver = NULL;

	// Cache coordinate conversion constants and self window size.
	// Also get window number for safe lookups later.
	NSWindow *selfWin = (__bridge NSWindow *)selfWindow;
	NSRect selfFrame = [selfWin frame];
	f->selfWidth = selfFrame.size.width;
	f->selfHeight = selfFrame.size.height;
	f->selfWindowNumber = (int)[selfWin windowNumber];

	// 优先使用 Wails Screen API 提供的屏幕信息
	if (screenInfo != NULL && screenInfo->width > 0 && screenInfo->height > 0) {
		f->axOriginX = (CGFloat)screenInfo->x;
		f->axOriginY = (CGFloat)screenInfo->y + (CGFloat)screenInfo->height;
	} else {
		// Fallback: 使用 NSScreen API
		NSScreen *primaryScreen = [[NSScreen screens] firstObject];
		if (!primaryScreen) primaryScreen = [NSScreen mainScreen];
		NSRect primaryFrame = primaryScreen ? [primaryScreen frame] : NSMakeRect(0, 0, 1920, 1080);
		f->axOriginX = primaryFrame.origin.x;
		f->axOriginY = primaryFrame.origin.y + primaryFrame.size.height;
	}

	f->lastAppliedOrigin = CGPointMake(-10000, -10000); // Invalid initial value

	f->appElem = AXUIElementCreateApplication(pid);
	if (!f->appElem) {
		winsnap_set_err(errOut, @"winsnap: failed to create AX application element");
		free(f);
		return NULL;
	}

	AXObserverRef obs = NULL;
	AXError aerr = AXObserverCreate(pid, winsnap_ax_callback, &obs);
	if (aerr != kAXErrorSuccess || !obs) {
		winsnap_set_err(errOut, [NSString stringWithFormat:@"winsnap: AXObserverCreate failed (%d)", (int)aerr]);
		CFRelease(f->appElem);
		free(f);
		return NULL;
	}
	f->observer = obs;

	// Observe focused window changes so we can rebind to the main/focused window.
	AXObserverAddNotification(f->observer, f->appElem, kAXFocusedWindowChangedNotification, f);
	AXObserverAddNotification(f->observer, f->appElem, kAXMainWindowChangedNotification, f);

	// Bind to current window and observe its move/resize.
	winsnap_update_observed_window(f);

	// Initial sync.
	winsnap_sync_to_target(f);

	// Register activation observer on the main thread (for z-order re-assertion when target app is activated).
	CFRunLoopRef mainRL = CFRunLoopGetMain();
	CFRetain(mainRL);
	CFRunLoopPerformBlock(mainRL, kCFRunLoopCommonModes, ^{
		winsnap_register_activation_observer(f);
		// If the target is already frontmost, order above immediately.
		winsnap_order_above_target(f);
		CFRelease(mainRL);
	});
	CFRunLoopWakeUp(mainRL);

	return f;
}

static void winsnap_follower_run(WinsnapFollower *f) {
	if (!f || !f->observer) return;
	f->runLoop = CFRunLoopGetCurrent();

	CFRunLoopSourceRef src = AXObserverGetRunLoopSource(f->observer);
	if (src) {
		CFRunLoopAddSource(f->runLoop, src, kCFRunLoopDefaultMode);
	}
	CFRunLoopRun();

	// Cleanup after runloop exits.
	f->stopping = true;

	// Remove activation observer on the main thread before freeing.
	CFRunLoopRef mainRL = CFRunLoopGetMain();
	CFRetain(mainRL);
	CFRunLoopPerformBlock(mainRL, kCFRunLoopCommonModes, ^{
		winsnap_unregister_activation_observer(f);
		CFRelease(mainRL);
	});
	CFRunLoopWakeUp(mainRL);

	if (f->observer) {
		if (f->appElem) {
			AXObserverRemoveNotification(f->observer, f->appElem, kAXFocusedWindowChangedNotification);
			AXObserverRemoveNotification(f->observer, f->appElem, kAXMainWindowChangedNotification);
		}
		if (f->observedWindow) {
			AXObserverRemoveNotification(f->observer, f->observedWindow, kAXMovedNotification);
			AXObserverRemoveNotification(f->observer, f->observedWindow, kAXResizedNotification);
		}
		CFRelease(f->observer);
		f->observer = NULL;
	}
	if (f->observedWindow) {
		CFRelease(f->observedWindow);
		f->observedWindow = NULL;
	}
	if (f->appElem) {
		CFRelease(f->appElem);
		f->appElem = NULL;
	}
	free(f);
}

static void winsnap_follower_request_stop(WinsnapFollower *f) {
	if (!f) return;
	f->stopping = true;
	if (f->runLoop) {
		CFRunLoopStop(f->runLoop);
		CFRunLoopWakeUp(f->runLoop);
	}
}

*/
import "C"

import (
	"errors"
	"runtime"
	"strings"
	"sync"
	"time"
	"unsafe"
)

type darwinFollower struct {
	mu     sync.Mutex
	f      *C.WinsnapFollower
	closed bool
	done   chan struct{}
	ready  chan struct{}
	err    error
}

func attachRightOfProcess(opts AttachOptions) (Controller, error) {
	if opts.Window == nil {
		return nil, ErrWinsnapWindowInvalid
	}

	selfHWND := uintptr(opts.Window.NativeWindow())
	if selfHWND == 0 {
		return nil, ErrWinsnapWindowInvalid
	}

	targetName := normalizeMacTargetName(opts.TargetProcessName)
	if targetName == "" {
		return nil, errors.New("winsnap: TargetProcessName is empty")
	}

	findTimeout := opts.FindTimeout
	if findTimeout <= 0 {
		findTimeout = 20 * time.Second
	}

	deadline := time.Now().Add(findTimeout)
	var pid C.pid_t
	for {
		cname := C.CString(targetName)
		pid = C.winsnap_find_pid_by_name(cname)
		C.free(unsafe.Pointer(cname))

		if pid > 0 {
			break
		}
		if time.Now().After(deadline) {
			return nil, errors.New(ErrTargetWindowNotFound.Error() + ": " + targetName)
		}
		time.Sleep(250 * time.Millisecond)
	}

	// 获取屏幕信息用于坐标转换
	var primaryScreen *C.ScreenInfo
	if opts.App != nil && opts.App.Screen != nil {
		screen := opts.App.Screen.GetPrimary()
		if screen != nil {
			primaryScreen = &C.ScreenInfo{
				x:      C.int(screen.X),
				y:      C.int(screen.Y),
				width:  C.int(screen.Size.Width),
				height: C.int(screen.Size.Height),
			}
		}
	}

	// Apps like Douyin have both chat (landscape) and video (portrait) windows.
	// Prefer landscape windows so we attach to the chat window.
	preferLandscape := isDouyinTarget(targetName)

	df := &darwinFollower{done: make(chan struct{})}
	df.ready = make(chan struct{})

	// Run AX observer on a dedicated OS thread with a CFRunLoop.
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		defer close(df.done)

		var cErr *C.char
		f := C.winsnap_follower_create(unsafe.Pointer(selfHWND), pid, C.int(opts.Gap), primaryScreen, C.bool(preferLandscape), &cErr)
		if cErr != nil {
			msg := C.GoString(cErr)
			C.free(unsafe.Pointer(cErr))
			df.mu.Lock()
			df.err = errors.New(msg)
			df.mu.Unlock()
			close(df.ready)
			return
		}

		df.mu.Lock()
		df.f = f
		close(df.ready)
		closed := df.closed
		df.mu.Unlock()
		if closed {
			C.winsnap_follower_request_stop(f)
			return
		}

		C.winsnap_follower_run(f)
	}()

	<-df.ready
	df.mu.Lock()
	defer df.mu.Unlock()
	if df.err != nil {
		return nil, df.err
	}
	if df.f == nil {
		return nil, errors.New("winsnap: failed to start")
	}
	return df, nil
}

func (d *darwinFollower) Stop() error {
	d.mu.Lock()
	if d.closed {
		d.mu.Unlock()
		return nil
	}
	d.closed = true
	f := d.f
	d.mu.Unlock()

	if f != nil {
		C.winsnap_follower_request_stop(f)
	}
	<-d.done
	return nil
}

func normalizeMacTargetName(name string) string {
	n := strings.TrimSpace(name)
	if n == "" {
		return ""
	}
	// Drop path components if user passes a full path.
	n = n[strings.LastIndex(n, "/")+1:]
	// Drop common .app suffix to match localizedName.
	ln := strings.ToLower(n)
	if strings.HasSuffix(ln, ".app") {
		n = strings.TrimSpace(n[:len(n)-4])
		ln = strings.ToLower(n)
	}
	// Drop Windows-style .exe suffix.
	if strings.HasSuffix(ln, ".exe") {
		n = strings.TrimSpace(n[:len(n)-4])
	}
	return n
}

// isDouyinTarget checks if the target process name belongs to Douyin.
// Douyin has multiple windows (chat + video); we need landscape preference
// to pick the chat window.
func isDouyinTarget(name string) bool {
	ln := strings.ToLower(name)
	return ln == "douyin" || ln == "douyin.exe" || ln == "抖音"
}
