//go:build linux && cgo && !android

package webviewpanel

/*
#cgo linux pkg-config: gtk+-3.0 webkit2gtk-4.1 gdk-3.0

#include <gtk/gtk.h>
#include <gdk/gdk.h>
#include <webkit2/webkit2.h>
#include <stdlib.h>
#include <string.h>

// Data keys on GtkWindow
static const char *WV_OVERLAY_KEY = "chatwiki_wvpanel_overlay";
static const char *WV_FIXED_KEY   = "chatwiki_wvpanel_fixed";

typedef struct wvpanel {
  GtkWindow *window;
  GtkWidget *overlay;   // GtkOverlay
  GtkWidget *fixed;     // GtkFixed overlay layer
  GtkWidget *webview;   // WebKitWebView (GtkWidget)
  int x;
  int y;
  int w;
  int h;
} wvpanel;

static GtkWidget* _wvpanel_get_or_create_fixed(GtkWindow *window) {
  if (!GTK_IS_WINDOW(window)) return NULL;

  GtkWidget *fixed = (GtkWidget*)g_object_get_data(G_OBJECT(window), WV_FIXED_KEY);
  if (fixed && GTK_IS_WIDGET(fixed)) {
    return fixed;
  }

  // Get current child of the window (Wails uses a vbox)
  GtkWidget *child = gtk_bin_get_child(GTK_BIN(window));
  if (child == NULL) {
    return NULL;
  }

  // Wrap existing child into an overlay
  g_object_ref(child);
  gtk_container_remove(GTK_CONTAINER(window), child);

  GtkWidget *overlay = gtk_overlay_new();
  gtk_container_add(GTK_CONTAINER(overlay), child); // main content
  g_object_unref(child);

  gtk_container_add(GTK_CONTAINER(window), overlay);

  // Create overlay fixed layer
  fixed = gtk_fixed_new();
  gtk_overlay_add_overlay(GTK_OVERLAY(overlay), fixed);

  gtk_widget_show_all(overlay);

  g_object_set_data(G_OBJECT(window), WV_OVERLAY_KEY, overlay);
  g_object_set_data(G_OBJECT(window), WV_FIXED_KEY, fixed);
  return fixed;
}

static wvpanel* wvpanel_create(void *windowPtr, int x, int y, int w, int h) {
  GtkWindow *window = (GtkWindow*)windowPtr;
  GtkWidget *fixed = _wvpanel_get_or_create_fixed(window);
  if (fixed == NULL) return NULL;

  wvpanel *p = (wvpanel*)calloc(1, sizeof(wvpanel));
  p->window = window;
  p->fixed = fixed;
  p->overlay = (GtkWidget*)g_object_get_data(G_OBJECT(window), WV_OVERLAY_KEY);
  p->x = x; p->y = y; p->w = w; p->h = h;

  GtkWidget *wv = webkit_web_view_new();
  p->webview = wv;

  gtk_widget_set_size_request(wv, w, h);
  gtk_fixed_put(GTK_FIXED(fixed), wv, x, y);
  gtk_widget_show(wv);
  return p;
}

static void wvpanel_destroy(wvpanel *p) {
  if (p == NULL) return;
  if (p->webview) {
    gtk_widget_destroy(p->webview);
    p->webview = NULL;
  }
  free(p);
}

static void wvpanel_set_bounds(wvpanel *p, int x, int y, int w, int h) {
  if (p == NULL || p->webview == NULL || p->fixed == NULL) return;
  p->x = x; p->y = y; p->w = w; p->h = h;
  gtk_widget_set_size_request(p->webview, w, h);
  gtk_fixed_move(GTK_FIXED(p->fixed), p->webview, x, y);
  gtk_widget_show(p->webview);
}

static void wvpanel_set_url(wvpanel *p, const char *url) {
  if (p == NULL || p->webview == NULL || url == NULL) return;
  webkit_web_view_load_uri(WEBKIT_WEB_VIEW(p->webview), url);
}

static void wvpanel_set_html(wvpanel *p, const char *html) {
  if (p == NULL || p->webview == NULL || html == NULL) return;
  webkit_web_view_load_html(WEBKIT_WEB_VIEW(p->webview), html, NULL);
}

static void wvpanel_eval_js(wvpanel *p, const char *js) {
  if (p == NULL || p->webview == NULL || js == NULL) return;
  webkit_web_view_evaluate_javascript(
    WEBKIT_WEB_VIEW(p->webview),
    js,
    (gssize)strlen(js),
    NULL,
    "",
    NULL,
    NULL,
    NULL
  );
}

static void wvpanel_reload(wvpanel *p) {
  if (p == NULL || p->webview == NULL) return;
  webkit_web_view_reload(WEBKIT_WEB_VIEW(p->webview));
}

static void wvpanel_show(wvpanel *p) {
  if (p == NULL || p->webview == NULL) return;
  gtk_widget_show(p->webview);
}

static void wvpanel_hide(wvpanel *p) {
  if (p == NULL || p->webview == NULL) return;
  gtk_widget_hide(p->webview);
}

static gboolean wvpanel_is_visible(wvpanel *p) {
  if (p == NULL || p->webview == NULL) return FALSE;
  return gtk_widget_get_visible(p->webview);
}

static void wvpanel_set_zoom(wvpanel *p, double zoom) {
  if (p == NULL || p->webview == NULL) return;
  webkit_web_view_set_zoom_level(WEBKIT_WEB_VIEW(p->webview), zoom);
}

static double wvpanel_get_zoom(wvpanel *p) {
  if (p == NULL || p->webview == NULL) return 1.0;
  return webkit_web_view_get_zoom_level(WEBKIT_WEB_VIEW(p->webview));
}

static void wvpanel_focus(wvpanel *p) {
  if (p == NULL || p->webview == NULL) return;
  gtk_widget_grab_focus(p->webview);
}
*/
import "C"

import (
	"fmt"
	"unsafe"
)

type linuxPanelImpl struct {
	panel      *WebviewPanel
	parentHwnd uintptr // GtkWindow*

	handle *C.wvpanel
}

func newPanelImpl(panel *WebviewPanel, parentHwnd uintptr) webviewPanelImpl {
	return &linuxPanelImpl{panel: panel, parentHwnd: parentHwnd}
}

func (p *linuxPanelImpl) create() {
	opts := p.panel.options
	h := C.wvpanel_create(unsafe.Pointer(p.parentHwnd), C.int(opts.X), C.int(opts.Y), C.int(opts.Width), C.int(opts.Height))
	if h == nil {
		fmt.Printf("[webviewpanel] linux: failed to create panel\n")
		return
	}
	p.handle = h

	// Navigate initial content
	if opts.HTML != "" {
		html := C.CString(opts.HTML)
		defer C.free(unsafe.Pointer(html))
		C.wvpanel_set_html(p.handle, html)
	} else if opts.URL != "" {
		url := C.CString(opts.URL)
		defer C.free(unsafe.Pointer(url))
		C.wvpanel_set_url(p.handle, url)
	}

	// Mark ready for ExecJS queue flushing
	p.panel.markRuntimeLoaded()

	// Apply visibility
	if opts.Visible != nil && !*opts.Visible {
		C.wvpanel_hide(p.handle)
	}
}

func (p *linuxPanelImpl) destroy() {
	if p.handle != nil {
		C.wvpanel_destroy(p.handle)
		p.handle = nil
	}
}

func (p *linuxPanelImpl) setBounds(bounds Rect) {
	if p.handle == nil {
		return
	}
	C.wvpanel_set_bounds(p.handle, C.int(bounds.X), C.int(bounds.Y), C.int(bounds.Width), C.int(bounds.Height))
}

func (p *linuxPanelImpl) bounds() Rect {
	return Rect{
		X:      p.panel.options.X,
		Y:      p.panel.options.Y,
		Width:  p.panel.options.Width,
		Height: p.panel.options.Height,
	}
}

func (p *linuxPanelImpl) setZIndex(_ int) {
	// GTK overlay stacking is not strictly defined here; we bring to front on Show().
}

func (p *linuxPanelImpl) setURL(url string) {
	if p.handle == nil {
		return
	}
	cu := C.CString(url)
	defer C.free(unsafe.Pointer(cu))
	C.wvpanel_set_url(p.handle, cu)
}

func (p *linuxPanelImpl) setHTML(html string) {
	if p.handle == nil {
		return
	}
	ch := C.CString(html)
	defer C.free(unsafe.Pointer(ch))
	C.wvpanel_set_html(p.handle, ch)
}

func (p *linuxPanelImpl) execJS(js string) {
	if p.handle == nil {
		return
	}
	cj := C.CString(js)
	defer C.free(unsafe.Pointer(cj))
	C.wvpanel_eval_js(p.handle, cj)
}

func (p *linuxPanelImpl) reload() {
	if p.handle == nil {
		return
	}
	C.wvpanel_reload(p.handle)
}

func (p *linuxPanelImpl) forceReload() { p.reload() }

func (p *linuxPanelImpl) show() {
	if p.handle == nil {
		return
	}
	C.wvpanel_show(p.handle)
}

func (p *linuxPanelImpl) hide() {
	if p.handle == nil {
		return
	}
	C.wvpanel_hide(p.handle)
}

func (p *linuxPanelImpl) isVisible() bool {
	if p.handle == nil {
		return p.panel.options.Visible != nil && *p.panel.options.Visible
	}
	return C.wvpanel_is_visible(p.handle) != 0
}

func (p *linuxPanelImpl) setZoom(zoom float64) {
	if p.handle == nil {
		return
	}
	C.wvpanel_set_zoom(p.handle, C.double(zoom))
}

func (p *linuxPanelImpl) getZoom() float64 {
	if p.handle == nil {
		return 1.0
	}
	return float64(C.wvpanel_get_zoom(p.handle))
}

func (p *linuxPanelImpl) openDevTools() {
	// WebKitGTK inspector can be enabled via settings; not wired yet.
}

func (p *linuxPanelImpl) focus() {
	if p.handle == nil {
		return
	}
	C.wvpanel_focus(p.handle)
}

func (p *linuxPanelImpl) isFocused() bool { return false }

