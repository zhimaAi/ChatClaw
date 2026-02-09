#import <Cocoa/Cocoa.h>
#import <stdbool.h>

// Returns the primary screen's visible work area in a Quartz-like coordinate system:
// - Origin at top-left of the primary screen
// - Y grows downward
// Values are in logical points (DIP).
bool floatingballPrimaryWorkArea(int *outX, int *outY, int *outW, int *outH, float *outScale) {
  @autoreleasepool {
    // Determine the primary screen reliably:
    // In Cocoa global coordinates, the primary screen's frame contains the origin (0,0).
    NSScreen *primary = nil;
    NSArray<NSScreen *> *screens = [NSScreen screens];
    NSPoint origin = NSMakePoint(0, 0);
    for (NSScreen *sc in screens) {
      if (sc == nil) continue;
      if (NSPointInRect(origin, [sc frame])) {
        primary = sc;
        break;
      }
    }
    if (primary == nil) {
      // Fallback: mainScreen is usually correct but may be the screen containing the key window.
      primary = [NSScreen mainScreen];
    }
    if (primary == nil) {
      return false;
    }

    NSRect frame = [primary frame];
    NSRect visible = [primary visibleFrame];
    if (frame.size.width <= 0 || frame.size.height <= 0 || visible.size.width <= 0 || visible.size.height <= 0) {
      return false;
    }

    // Convert from Cocoa (origin bottom-left, Y up) to Quartz-like (origin top-left of primary, Y down).
    CGFloat top = frame.origin.y + frame.size.height;
    CGFloat qx = visible.origin.x - frame.origin.x;
    CGFloat qy = top - (visible.origin.y + visible.size.height);

    if (outX) *outX = (int)llround(qx);
    if (outY) *outY = (int)llround(qy);
    if (outW) *outW = (int)llround(visible.size.width);
    if (outH) *outH = (int)llround(visible.size.height);
    if (outScale) *outScale = (float)[primary backingScaleFactor];
    return true;
  }
}

