#pragma once

#include <stdbool.h>

// Opaque panel handle
typedef void* wvpanel_handle;

// Create a WKWebView panel in the given NSWindow (top-left coordinates in points).
wvpanel_handle wvpanel_create(void* nsWindow, int x, int y, int w, int h);
void wvpanel_destroy(wvpanel_handle panel);

void wvpanel_set_bounds(wvpanel_handle panel, int x, int y, int w, int h);
void wvpanel_set_zindex(wvpanel_handle panel, int z);

void wvpanel_set_url(wvpanel_handle panel, const char* url);
void wvpanel_set_html(wvpanel_handle panel, const char* html);
void wvpanel_eval_js(wvpanel_handle panel, const char* js);

void wvpanel_reload(wvpanel_handle panel);
void wvpanel_show(wvpanel_handle panel);
void wvpanel_hide(wvpanel_handle panel);
bool wvpanel_is_visible(wvpanel_handle panel);

void wvpanel_set_zoom(wvpanel_handle panel, double zoom);
double wvpanel_get_zoom(wvpanel_handle panel);
void wvpanel_focus(wvpanel_handle panel);

// Focus the main (host) WKWebView in the given NSWindow.
// This is used as a workaround for focus issues when switching between
// embedded WKWebViews (panels) and the host webview.
void wvpanel_focus_main_webview(void* nsWindow);

