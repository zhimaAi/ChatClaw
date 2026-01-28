//go:build linux && !android && !cgo

package webviewpanel

// Linux stub implementation for WebviewPanel
// This is a placeholder until proper Linux implementation is available.
// For full functionality, wait for wails PR #4880 to be merged.

type linuxPanelImpl struct {
	panel *WebviewPanel
}

func newPanelImpl(panel *WebviewPanel, parentHwnd uintptr) webviewPanelImpl {
	return &linuxPanelImpl{panel: panel}
}

func (p *linuxPanelImpl) create() {
	// Not implemented on Linux yet
	// TODO: Implement using WebKitWebView similar to PR #4880
}

func (p *linuxPanelImpl) destroy() {
	// Not implemented on Linux yet
}

func (p *linuxPanelImpl) setBounds(_ Rect) {
	// Not implemented on Linux yet
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
	// Not implemented on Linux yet
}

func (p *linuxPanelImpl) setURL(_ string) {
	// Not implemented on Linux yet
}

func (p *linuxPanelImpl) setHTML(_ string) {
	// Not implemented on Linux yet
}

func (p *linuxPanelImpl) execJS(_ string) {
	// Not implemented on Linux yet
}

func (p *linuxPanelImpl) reload() {
	// Not implemented on Linux yet
}

func (p *linuxPanelImpl) forceReload() {
	// Not implemented on Linux yet
}

func (p *linuxPanelImpl) show() {
	// Not implemented on Linux yet
}

func (p *linuxPanelImpl) hide() {
	// Not implemented on Linux yet
}

func (p *linuxPanelImpl) isVisible() bool {
	return p.panel.options.Visible != nil && *p.panel.options.Visible
}

func (p *linuxPanelImpl) setZoom(_ float64) {
	// Not implemented on Linux yet
}

func (p *linuxPanelImpl) getZoom() float64 {
	return 1.0
}

func (p *linuxPanelImpl) openDevTools() {
	// Not implemented on Linux yet
}

func (p *linuxPanelImpl) focus() {
	// Not implemented on Linux yet
}

func (p *linuxPanelImpl) isFocused() bool {
	return false
}
