#import <Cocoa/Cocoa.h>
#import <WebKit/WebKit.h>

#include "panel_darwin.h"

@interface WVPPanel : NSObject
@property (nonatomic, weak) NSWindow *window;
@property (nonatomic, strong) NSView *container;
@property (nonatomic, strong) WKWebView *webview;
@property (nonatomic, assign) double zoom;
@end

@implementation WVPPanel
@end

static NSString * const _WVPPanelContainerIdentifier = @"wvp_panel_container";

static NSRect _wvpanel_frame_from_top_left(NSWindow *window, int x, int y, int w, int h) {
    NSView *content = [window contentView];
    if (content == nil) {
        return NSMakeRect(x, y, w, h);
    }
    CGFloat H = content.bounds.size.height;
    // Convert top-left origin (CSS-like) to Cocoa bottom-left origin
    CGFloat oy = H - (CGFloat)y - (CGFloat)h;
    return NSMakeRect((CGFloat)x, oy, (CGFloat)w, (CGFloat)h);
}

wvpanel_handle wvpanel_create(void* nsWindow, int x, int y, int w, int h) {
    @autoreleasepool {
        NSWindow *window = (__bridge NSWindow *)nsWindow;
        if (window == nil) return NULL;

        WVPPanel *p = [WVPPanel new];
        p.window = window;
        p.zoom = 1.0;

        NSView *content = [window contentView];
        if (content == nil) return NULL;

        NSRect frame = _wvpanel_frame_from_top_left(window, x, y, w, h);

        NSView *container = [[NSView alloc] initWithFrame:frame];
        container.wantsLayer = YES;
        container.layer.backgroundColor = [[NSColor clearColor] CGColor];
        container.identifier = _WVPPanelContainerIdentifier;

        WKWebViewConfiguration *cfg = [WKWebViewConfiguration new];
        WKWebView *wv = [[WKWebView alloc] initWithFrame:container.bounds configuration:cfg];
        wv.autoresizingMask = NSViewWidthSizable | NSViewHeightSizable;

        [container addSubview:wv];

        // Place above existing subviews (Wails host webview is also in contentView)
        [content addSubview:container positioned:NSWindowAbove relativeTo:nil];

        p.container = container;
        p.webview = wv;

        return (__bridge_retained void*)p;
    }
}

void wvpanel_destroy(wvpanel_handle panel) {
    @autoreleasepool {
        if (panel == NULL) return;
        WVPPanel *p = (__bridge_transfer WVPPanel*)panel;
        if (p.container) {
            [p.container removeFromSuperview];
        }
        p.webview = nil;
        p.container = nil;
    }
}

void wvpanel_set_bounds(wvpanel_handle panel, int x, int y, int w, int h) {
    @autoreleasepool {
        if (panel == NULL) return;
        WVPPanel *p = (__bridge WVPPanel*)panel;
        if (p.window == nil || p.container == nil) return;
        NSRect frame = _wvpanel_frame_from_top_left(p.window, x, y, w, h);
        [p.container setFrame:frame];
    }
}

void wvpanel_set_zindex(wvpanel_handle panel, int z) {
    @autoreleasepool {
        if (panel == NULL) return;
        WVPPanel *p = (__bridge WVPPanel*)panel;
        if (p.container == nil) return;
        NSView *super = [p.container superview];
        if (super == nil) return;
        // Re-add to adjust z-order: z>0 => top, else => bottom
        [p.container removeFromSuperview];
        if (z > 0) {
            [super addSubview:p.container positioned:NSWindowAbove relativeTo:nil];
        } else {
            [super addSubview:p.container positioned:NSWindowBelow relativeTo:nil];
        }
    }
}

void wvpanel_set_url(wvpanel_handle panel, const char* url) {
    @autoreleasepool {
        if (panel == NULL || url == NULL) return;
        WVPPanel *p = (__bridge WVPPanel*)panel;
        if (p.webview == nil) return;
        NSString *s = [NSString stringWithUTF8String:url];
        if (s == nil) return;
        NSURL *u = [NSURL URLWithString:s];
        if (u == nil) return;
        NSURLRequest *req = [NSURLRequest requestWithURL:u];
        [p.webview loadRequest:req];
    }
}

void wvpanel_set_html(wvpanel_handle panel, const char* html) {
    @autoreleasepool {
        if (panel == NULL || html == NULL) return;
        WVPPanel *p = (__bridge WVPPanel*)panel;
        if (p.webview == nil) return;
        NSString *s = [NSString stringWithUTF8String:html];
        if (s == nil) return;
        [p.webview loadHTMLString:s baseURL:nil];
    }
}

void wvpanel_eval_js(wvpanel_handle panel, const char* js) {
    @autoreleasepool {
        if (panel == NULL || js == NULL) return;
        WVPPanel *p = (__bridge WVPPanel*)panel;
        if (p.webview == nil) return;
        NSString *s = [NSString stringWithUTF8String:js];
        if (s == nil) return;
        [p.webview evaluateJavaScript:s completionHandler:nil];
    }
}

void wvpanel_reload(wvpanel_handle panel) {
    @autoreleasepool {
        if (panel == NULL) return;
        WVPPanel *p = (__bridge WVPPanel*)panel;
        if (p.webview == nil) return;
        [p.webview reload];
    }
}

void wvpanel_show(wvpanel_handle panel) {
    @autoreleasepool {
        if (panel == NULL) return;
        WVPPanel *p = (__bridge WVPPanel*)panel;
        if (p.container == nil) return;
        [p.container setHidden:NO];
        // Keep above siblings
        wvpanel_set_zindex(panel, 1);
    }
}

void wvpanel_hide(wvpanel_handle panel) {
    @autoreleasepool {
        if (panel == NULL) return;
        WVPPanel *p = (__bridge WVPPanel*)panel;
        if (p.container == nil) return;
        [p.container setHidden:YES];
    }
}

bool wvpanel_is_visible(wvpanel_handle panel) {
    if (panel == NULL) return false;
    WVPPanel *p = (__bridge WVPPanel*)panel;
    if (p.container == nil) return false;
    return !p.container.isHidden;
}

void wvpanel_set_zoom(wvpanel_handle panel, double zoom) {
    @autoreleasepool {
        if (panel == NULL) return;
        WVPPanel *p = (__bridge WVPPanel*)panel;
        if (p.webview == nil) return;
        if (zoom <= 0) zoom = 1.0;
        p.zoom = zoom;
        if (@available(macOS 11.0, *)) {
            p.webview.pageZoom = zoom;
        } else {
            // Fallback: CSS zoom
            NSString *js = [NSString stringWithFormat:@"document.body.style.zoom = %f;", zoom];
            [p.webview evaluateJavaScript:js completionHandler:nil];
        }
    }
}

double wvpanel_get_zoom(wvpanel_handle panel) {
    if (panel == NULL) return 1.0;
    WVPPanel *p = (__bridge WVPPanel*)panel;
    if (p.webview == nil) return p.zoom;
    if (@available(macOS 11.0, *)) {
        return p.webview.pageZoom;
    }
    return p.zoom;
}

void wvpanel_focus(wvpanel_handle panel) {
    @autoreleasepool {
        if (panel == NULL) return;
        WVPPanel *p = (__bridge WVPPanel*)panel;
        if (p.webview == nil || p.window == nil) return;
        [p.window makeFirstResponder:p.webview];
    }
}

static void _wvpanel_find_largest_wkwebview(NSView *root,
                                           NSView *content,
                                           bool insidePanelContainer,
                                           bool skipPanelWebviews,
                                           WKWebView **best,
                                           CGFloat *bestArea) {
    if (root == nil) return;

    bool nowInsidePanel = insidePanelContainer;
    if (root.identifier != nil && [root.identifier isEqualToString:_WVPPanelContainerIdentifier]) {
        nowInsidePanel = true;
    }

    if ([root isKindOfClass:[WKWebView class]]) {
        if (!(skipPanelWebviews && nowInsidePanel)) {
            WKWebView *wv = (WKWebView *)root;
            NSRect r = [wv convertRect:wv.bounds toView:content];
            CGFloat area = r.size.width * r.size.height;
            if (area > *bestArea) {
                *bestArea = area;
                *best = wv;
            }
        }
    }

    for (NSView *v in root.subviews) {
        _wvpanel_find_largest_wkwebview(v, content, nowInsidePanel, skipPanelWebviews, best, bestArea);
    }
}

void wvpanel_focus_main_webview(void* nsWindow) {
    @autoreleasepool {
        NSWindow *window = (__bridge NSWindow *)nsWindow;
        if (window == nil) return;
        NSView *content = [window contentView];
        if (content == nil) return;

        // Prefer the host (main) WKWebView: skip WKWebViews that live inside our panel containers.
        WKWebView *best = nil;
        CGFloat bestArea = 0;
        _wvpanel_find_largest_wkwebview(content, content, false, true, &best, &bestArea);
        // Fallback: if we didn't find any, pick the largest WKWebView anyway.
        if (best == nil) {
            bestArea = 0;
            _wvpanel_find_largest_wkwebview(content, content, false, false, &best, &bestArea);
        }
        if (best != nil) {
            [window makeFirstResponder:best];
        }
    }
}

