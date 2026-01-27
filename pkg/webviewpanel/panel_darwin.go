//go:build darwin && !ios && cgo

package webviewpanel

/*
#cgo darwin CFLAGS: -fobjc-arc
#cgo darwin LDFLAGS: -framework Cocoa -framework WebKit

#include <stdlib.h>
#include "panel_darwin.h"
*/
import "C"

import (
	"unsafe"
)

type darwinPanelImpl struct {
	panel      *WebviewPanel
	parentHwnd uintptr // NSWindow*
	handle     C.wvpanel_handle
}

func newPanelImpl(panel *WebviewPanel, parentHwnd uintptr) webviewPanelImpl {
	return &darwinPanelImpl{panel: panel, parentHwnd: parentHwnd}
}

func (p *darwinPanelImpl) create() {
	opts := p.panel.options
	p.handle = C.wvpanel_create(unsafe.Pointer(p.parentHwnd), C.int(opts.X), C.int(opts.Y), C.int(opts.Width), C.int(opts.Height))
	if p.handle == nil {
		return
	}

	// Initial content
	if opts.HTML != "" {
		ch := C.CString(opts.HTML)
		defer C.free(unsafe.Pointer(ch))
		C.wvpanel_set_html(p.handle, ch)
	} else if opts.URL != "" {
		cu := C.CString(opts.URL)
		defer C.free(unsafe.Pointer(cu))
		C.wvpanel_set_url(p.handle, cu)
	}

	// Mark ready for ExecJS queue flushing
	p.panel.markRuntimeLoaded()

	if opts.Visible != nil && !*opts.Visible {
		C.wvpanel_hide(p.handle)
	}
}

func (p *darwinPanelImpl) destroy() {
	if p.handle != nil {
		C.wvpanel_destroy(p.handle)
		p.handle = nil
	}
}

func (p *darwinPanelImpl) setBounds(r Rect) {
	if p.handle == nil {
		return
	}
	C.wvpanel_set_bounds(p.handle, C.int(r.X), C.int(r.Y), C.int(r.Width), C.int(r.Height))
}

func (p *darwinPanelImpl) bounds() Rect {
	return Rect{
		X:      p.panel.options.X,
		Y:      p.panel.options.Y,
		Width:  p.panel.options.Width,
		Height: p.panel.options.Height,
	}
}

func (p *darwinPanelImpl) setZIndex(z int) {
	if p.handle == nil {
		return
	}
	C.wvpanel_set_zindex(p.handle, C.int(z))
}

func (p *darwinPanelImpl) setURL(url string) {
	if p.handle == nil {
		return
	}
	cu := C.CString(url)
	defer C.free(unsafe.Pointer(cu))
	C.wvpanel_set_url(p.handle, cu)
}

func (p *darwinPanelImpl) setHTML(html string) {
	if p.handle == nil {
		return
	}
	ch := C.CString(html)
	defer C.free(unsafe.Pointer(ch))
	C.wvpanel_set_html(p.handle, ch)
}

func (p *darwinPanelImpl) execJS(js string) {
	if p.handle == nil {
		return
	}
	cj := C.CString(js)
	defer C.free(unsafe.Pointer(cj))
	C.wvpanel_eval_js(p.handle, cj)
}

func (p *darwinPanelImpl) reload() {
	if p.handle == nil {
		return
	}
	C.wvpanel_reload(p.handle)
}

func (p *darwinPanelImpl) forceReload() {
	p.reload()
}

func (p *darwinPanelImpl) show() {
	if p.handle == nil {
		return
	}
	C.wvpanel_show(p.handle)
}

func (p *darwinPanelImpl) hide() {
	if p.handle == nil {
		return
	}
	C.wvpanel_hide(p.handle)
}

func (p *darwinPanelImpl) isVisible() bool {
	if p.handle == nil {
		return p.panel.options.Visible != nil && *p.panel.options.Visible
	}
	return bool(C.wvpanel_is_visible(p.handle))
}

func (p *darwinPanelImpl) setZoom(z float64) {
	if p.handle == nil {
		return
	}
	C.wvpanel_set_zoom(p.handle, C.double(z))
}

func (p *darwinPanelImpl) getZoom() float64 {
	if p.handle == nil {
		return 1.0
	}
	return float64(C.wvpanel_get_zoom(p.handle))
}

func (p *darwinPanelImpl) openDevTools() {
	// WKWebView doesn't have an embedded devtools window like WebView2.
}

func (p *darwinPanelImpl) focus() {
	if p.handle == nil {
		return
	}
	C.wvpanel_focus(p.handle)
}

func (p *darwinPanelImpl) isFocused() bool {
	return false
}
