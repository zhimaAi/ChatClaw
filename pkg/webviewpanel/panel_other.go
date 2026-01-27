//go:build !windows && !darwin && !linux

package webviewpanel

// Stub implementation for unsupported platforms

type otherPanelImpl struct {
	panel *WebviewPanel
}

func newPanelImpl(panel *WebviewPanel, parentHwnd uintptr) webviewPanelImpl {
	return &otherPanelImpl{panel: panel}
}

func (p *otherPanelImpl) create() {
	// Not implemented
}

func (p *otherPanelImpl) destroy() {
	// Not implemented
}

func (p *otherPanelImpl) setBounds(_ Rect) {
	// Not implemented
}

func (p *otherPanelImpl) bounds() Rect {
	return Rect{
		X:      p.panel.options.X,
		Y:      p.panel.options.Y,
		Width:  p.panel.options.Width,
		Height: p.panel.options.Height,
	}
}

func (p *otherPanelImpl) setZIndex(_ int) {
	// Not implemented
}

func (p *otherPanelImpl) setURL(_ string) {
	// Not implemented
}

func (p *otherPanelImpl) setHTML(_ string) {
	// Not implemented
}

func (p *otherPanelImpl) execJS(_ string) {
	// Not implemented
}

func (p *otherPanelImpl) reload() {
	// Not implemented
}

func (p *otherPanelImpl) forceReload() {
	// Not implemented
}

func (p *otherPanelImpl) show() {
	// Not implemented
}

func (p *otherPanelImpl) hide() {
	// Not implemented
}

func (p *otherPanelImpl) isVisible() bool {
	return p.panel.options.Visible != nil && *p.panel.options.Visible
}

func (p *otherPanelImpl) setZoom(_ float64) {
	// Not implemented
}

func (p *otherPanelImpl) getZoom() float64 {
	return 1.0
}

func (p *otherPanelImpl) openDevTools() {
	// Not implemented
}

func (p *otherPanelImpl) focus() {
	// Not implemented
}

func (p *otherPanelImpl) isFocused() bool {
	return false
}
