//go:build darwin && !ios && cgo

package webviewpanel

/*
#cgo darwin CFLAGS: -x objective-c -fobjc-arc
#cgo darwin LDFLAGS: -framework Cocoa

#import <Cocoa/Cocoa.h>
#include <stdlib.h>
#include <string.h>

static bool _wvpanel_contains(const char *s, const char *substr) {
    if (s == NULL || substr == NULL) return false;
    return strstr(s, substr) != NULL;
}

static void* _wvpanel_find_window(const char *title, bool contains) {
    if (title == NULL) return NULL;
    NSString *t = [NSString stringWithUTF8String:title];
    for (NSWindow *w in [NSApp windows]) {
        NSString *wt = [w title];
        if (wt == nil) continue;
        if (!contains) {
            if ([wt isEqualToString:t]) {
                return (__bridge void*)w;
            }
        } else {
            const char *wtc = [wt UTF8String];
            if (_wvpanel_contains(wtc, title)) {
                return (__bridge void*)w;
            }
        }
    }
    return NULL;
}
*/
import "C"

import "unsafe"

// FindWindowByTitle finds an NSWindow by its title and returns it as uintptr.
func FindWindowByTitle(title string) uintptr {
	ct := C.CString(title)
	defer C.free(unsafe.Pointer(ct))
	return uintptr(C._wvpanel_find_window(ct, false))
}

// FindWindowByTitleContains finds an NSWindow whose title contains the given substring.
func FindWindowByTitleContains(titleSubstring string) uintptr {
	ct := C.CString(titleSubstring)
	defer C.free(unsafe.Pointer(ct))
	return uintptr(C._wvpanel_find_window(ct, true))
}

