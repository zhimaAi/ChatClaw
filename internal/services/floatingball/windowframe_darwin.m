#import <Cocoa/Cocoa.h>
#import <stdbool.h>
#import <dispatch/dispatch.h>

static NSScreen* floatingball_primary_screen(void) {
  NSArray<NSScreen *> *screens = [NSScreen screens];
  NSPoint origin = NSMakePoint(0, 0);
  for (NSScreen *sc in screens) {
    if (sc == nil) continue;
    if (NSPointInRect(origin, [sc frame])) {
      return sc;
    }
  }
  return [NSScreen mainScreen];
}

// Convert from Quartz-like (origin top-left of primary, y down) into Cocoa global coords (origin bottom-left, y up).
static NSPoint floatingball_quartz_to_cocoa(int qx, int qy, int w, int h) {
  NSScreen *primary = floatingball_primary_screen();
  NSRect pf = primary ? [primary frame] : NSMakeRect(0, 0, 1920, 1080);
  CGFloat top = pf.origin.y + pf.size.height;
  CGFloat cx = pf.origin.x + (CGFloat)qx;
  CGFloat cy = top - (CGFloat)qy - (CGFloat)h;
  return NSMakePoint(cx, cy);
}

// Convert from Cocoa global coords into Quartz-like coords.
static void floatingball_cocoa_to_quartz(NSRect cocoaFrame, int *outQx, int *outQy) {
  NSScreen *primary = floatingball_primary_screen();
  NSRect pf = primary ? [primary frame] : NSMakeRect(0, 0, 1920, 1080);
  CGFloat top = pf.origin.y + pf.size.height;
  CGFloat qx = cocoaFrame.origin.x - pf.origin.x;
  CGFloat qy = top - (cocoaFrame.origin.y + cocoaFrame.size.height);
  if (outQx) *outQx = (int)llround(qx);
  if (outQy) *outQy = (int)llround(qy);
}

bool floatingballGetWindowQuartzFrame(void *nsWindowPtr, int *outX, int *outY, int *outW, int *outH) {
  if (nsWindowPtr == NULL) return false;
  if (![NSThread isMainThread]) {
    __block bool ok = false;
    dispatch_sync(dispatch_get_main_queue(), ^{
      ok = floatingballGetWindowQuartzFrame(nsWindowPtr, outX, outY, outW, outH);
    });
    return ok;
  }
  @autoreleasepool {
    NSWindow *win = (__bridge NSWindow *)nsWindowPtr;
    if (win == nil) return false;
    NSRect fr = [win frame];
    if (fr.size.width <= 0 || fr.size.height <= 0) return false;
    int qx = 0, qy = 0;
    floatingball_cocoa_to_quartz(fr, &qx, &qy);
    if (outX) *outX = qx;
    if (outY) *outY = qy;
    if (outW) *outW = (int)llround(fr.size.width);
    if (outH) *outH = (int)llround(fr.size.height);
    return true;
  }
}

bool floatingballSetWindowQuartzFrame(void *nsWindowPtr, int qx, int qy, int w, int h) {
  if (nsWindowPtr == NULL) return false;
  if (![NSThread isMainThread]) {
    __block bool ok = false;
    dispatch_sync(dispatch_get_main_queue(), ^{
      ok = floatingballSetWindowQuartzFrame(nsWindowPtr, qx, qy, w, h);
    });
    return ok;
  }
  @autoreleasepool {
    NSWindow *win = (__bridge NSWindow *)nsWindowPtr;
    if (win == nil) return false;
    if (w <= 0 || h <= 0) return false;
    NSPoint origin = floatingball_quartz_to_cocoa(qx, qy, w, h);
    NSRect fr = NSMakeRect(origin.x, origin.y, (CGFloat)w, (CGFloat)h);
    [win setFrame:fr display:YES animate:NO];
    return true;
  }
}

