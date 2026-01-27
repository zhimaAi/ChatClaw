//go:build darwin && !ios && !cgo

package webviewpanel

// Darwin stub implementation for WebviewPanel when cgo is disabled.

type darwinPanelImpl struct {
	panel *WebviewPanel
}

func newPanelImpl(panel *WebviewPanel, parentHwnd uintptr) webviewPanelImpl {
	return &darwinPanelImpl{panel: panel}
}

func (p *darwinPanelImpl) create()                        {}
func (p *darwinPanelImpl) destroy()                       {}
func (p *darwinPanelImpl) setBounds(_ Rect)               {}
func (p *darwinPanelImpl) bounds() Rect                   { return Rect{X: p.panel.options.X, Y: p.panel.options.Y, Width: p.panel.options.Width, Height: p.panel.options.Height} }
func (p *darwinPanelImpl) setZIndex(_ int)                {}
func (p *darwinPanelImpl) setURL(_ string)                {}
func (p *darwinPanelImpl) setHTML(_ string)               {}
func (p *darwinPanelImpl) execJS(_ string)                {}
func (p *darwinPanelImpl) reload()                        {}
func (p *darwinPanelImpl) forceReload()                   {}
func (p *darwinPanelImpl) show()                          {}
func (p *darwinPanelImpl) hide()                          {}
func (p *darwinPanelImpl) isVisible() bool                { return p.panel.options.Visible != nil && *p.panel.options.Visible }
func (p *darwinPanelImpl) setZoom(_ float64)              {}
func (p *darwinPanelImpl) getZoom() float64               { return 1.0 }
func (p *darwinPanelImpl) openDevTools()                  {}
func (p *darwinPanelImpl) focus()                         {}
func (p *darwinPanelImpl) isFocused() bool                { return false }

