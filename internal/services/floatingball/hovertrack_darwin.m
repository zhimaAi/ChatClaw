#import <Cocoa/Cocoa.h>
#import <WebKit/WebKit.h>
#import <objc/runtime.h>

static const void* kFloatingBallTrackingAreaKey = &kFloatingBallTrackingAreaKey;
static const void* kFloatingBallHoverTrackerKey = &kFloatingBallHoverTrackerKey;

// Minimal interface for Wails' WebviewWindow (NSWindow subclass).
@interface WebviewWindow : NSWindow
@property (nonatomic, retain) WKWebView* webView;
@end

@interface FloatingBallHoverTracker : NSObject
// NOTE: This project builds Objective-C with manual reference counting (MRC),
// so we can't use 'weak'. Use assign (non-retaining) instead.
@property (nonatomic, assign) WebviewWindow* window;
@end

@implementation FloatingBallHoverTracker
- (void)sendHover:(BOOL)entered {
  WebviewWindow* window = self.window;
  if (window == nil) {
    return;
  }
  WKWebView* webView = window.webView;
  if (webView == nil) {
    return;
  }
  NSString* js = [NSString stringWithFormat:@"window.__floatingballNativeHover && window.__floatingballNativeHover(%@);",
                  entered ? @"true" : @"false"];
  [webView evaluateJavaScript:js completionHandler:nil];
}

- (void)mouseEntered:(NSEvent*)event {
  (void)event;
  [self sendHover:YES];
}

- (void)mouseExited:(NSEvent*)event {
  (void)event;
  [self sendHover:NO];
}
@end

// Enable mouse moved events for a non-activating window so the webview can receive hover/pointer events
// without requiring the window to become key/active.
void floatingballEnableHoverTracking(void* nsWindowPtr) {
  if (nsWindowPtr == NULL) {
    return;
  }
  WebviewWindow* window = (__bridge WebviewWindow*)nsWindowPtr;
  [window setAcceptsMouseMovedEvents:YES];
  [window setIgnoresMouseEvents:NO];

  NSView* contentView = [window contentView];
  if (contentView == nil) {
    return;
  }

  FloatingBallHoverTracker* tracker = (FloatingBallHoverTracker*)objc_getAssociatedObject(contentView, kFloatingBallHoverTrackerKey);
  if (tracker == nil) {
    tracker = [[FloatingBallHoverTracker alloc] init];
    tracker.window = window;
    objc_setAssociatedObject(contentView, kFloatingBallHoverTrackerKey, tracker, OBJC_ASSOCIATION_RETAIN_NONATOMIC);
  } else {
    tracker.window = window;
  }

  NSTrackingArea* oldArea = (NSTrackingArea*)objc_getAssociatedObject(contentView, kFloatingBallTrackingAreaKey);
  if (oldArea != nil) {
    @try {
      [contentView removeTrackingArea:oldArea];
    } @catch (NSException* e) {
      // ignore
    }
  }

  NSTrackingAreaOptions opts = NSTrackingMouseEnteredAndExited |
                              NSTrackingActiveAlways |
                              NSTrackingInVisibleRect;
  NSTrackingArea* area = [[NSTrackingArea alloc] initWithRect:NSZeroRect
                                                     options:opts
                                                       owner:tracker
                                                    userInfo:nil];
  [contentView addTrackingArea:area];
  objc_setAssociatedObject(contentView, kFloatingBallTrackingAreaKey, area, OBJC_ASSOCIATION_RETAIN_NONATOMIC);
}

