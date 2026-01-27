package webviewpanel

import (
	"fmt"
	"sync"
	"sync/atomic"
)

// webviewPanelImpl is the platform-specific interface for WebviewPanel
type webviewPanelImpl interface {
	// Lifecycle
	create()
	destroy()

	// Position and size
	setBounds(bounds Rect)
	bounds() Rect
	setZIndex(zIndex int)

	// Content
	setURL(url string)
	setHTML(html string)
	execJS(js string)
	reload()
	forceReload()

	// Visibility
	show()
	hide()
	isVisible() bool

	// Zoom
	setZoom(zoom float64)
	getZoom() float64

	// DevTools
	openDevTools()

	// Focus
	focus()
	isFocused() bool
}

var panelID uint32

func getNextPanelID() uint {
	return uint(atomic.AddUint32(&panelID, 1))
}

// WebviewPanel represents an embedded webview within a window.
// Unlike WebviewWindow, a WebviewPanel is a child view that exists within
// a parent window and can be positioned anywhere within that window.
// This is similar to Electron's BrowserView or the deprecated webview tag.
type WebviewPanel struct {
	id      uint
	name    string
	options WebviewPanelOptions
	impl    webviewPanelImpl
	manager *PanelManager

	// Track if the panel has been destroyed
	destroyed     bool
	destroyedLock sync.RWMutex

	// Track if runtime has been loaded (protected by runtimeLock)
	runtimeLoaded bool
	pendingJS     []string
	runtimeLock   sync.Mutex
}

func (p *WebviewPanel) dispatch(fn func()) {
	if p.manager != nil {
		p.manager.dispatch(fn)
		return
	}
	fn()
}

// newPanel creates a new WebviewPanel with the given options.
func newPanel(options WebviewPanelOptions, manager *PanelManager) *WebviewPanel {
	id := getNextPanelID()

	// Apply defaults
	if options.Width == 0 {
		options.Width = 400
	}
	if options.Height == 0 {
		options.Height = 300
	}
	if options.ZIndex == 0 {
		options.ZIndex = 1
	}
	if options.Zoom == 0 {
		options.Zoom = 1.0
	}
	if options.Name == "" {
		options.Name = fmt.Sprintf("panel-%d", id)
	}
	// Default to visible
	if options.Visible == nil {
		visible := true
		options.Visible = &visible
	}

	return &WebviewPanel{
		id:      id,
		name:    options.Name,
		options: options,
		manager: manager,
	}
}

// ID returns the unique identifier for this panel
func (p *WebviewPanel) ID() uint {
	return p.id
}

// Name returns the name of this panel
func (p *WebviewPanel) Name() string {
	return p.name
}

// SetBounds sets the position and size of the panel within its parent window
func (p *WebviewPanel) SetBounds(bounds Rect) *WebviewPanel {
	p.options.X = bounds.X
	p.options.Y = bounds.Y
	p.options.Width = bounds.Width
	p.options.Height = bounds.Height

	p.dispatch(func() {
		if p.impl != nil && !p.isDestroyed() {
			p.impl.setBounds(bounds)
		}
	})
	return p
}

// Bounds returns the current bounds of the panel
func (p *WebviewPanel) Bounds() Rect {
	if p.impl != nil && !p.isDestroyed() {
		var res Rect
		p.dispatch(func() {
			if p.impl != nil && !p.isDestroyed() {
				res = p.impl.bounds()
			}
		})
		return res
	}
	return Rect{
		X:      p.options.X,
		Y:      p.options.Y,
		Width:  p.options.Width,
		Height: p.options.Height,
	}
}

// SetPosition sets the position of the panel within its parent window
func (p *WebviewPanel) SetPosition(x, y int) *WebviewPanel {
	bounds := p.Bounds()
	bounds.X = x
	bounds.Y = y
	return p.SetBounds(bounds)
}

// Position returns the current position of the panel
func (p *WebviewPanel) Position() (int, int) {
	bounds := p.Bounds()
	return bounds.X, bounds.Y
}

// SetSize sets the size of the panel
func (p *WebviewPanel) SetSize(width, height int) *WebviewPanel {
	bounds := p.Bounds()
	bounds.Width = width
	bounds.Height = height
	return p.SetBounds(bounds)
}

// Size returns the current size of the panel
func (p *WebviewPanel) Size() (int, int) {
	bounds := p.Bounds()
	return bounds.Width, bounds.Height
}

// SetZIndex sets the stacking order of the panel
func (p *WebviewPanel) SetZIndex(zIndex int) *WebviewPanel {
	p.options.ZIndex = zIndex
	p.dispatch(func() {
		if p.impl != nil && !p.isDestroyed() {
			p.impl.setZIndex(zIndex)
		}
	})
	return p
}

// ZIndex returns the current z-index of the panel
func (p *WebviewPanel) ZIndex() int {
	return p.options.ZIndex
}

// SetURL navigates the panel to the specified URL
func (p *WebviewPanel) SetURL(url string) *WebviewPanel {
	p.options.URL = url
	p.dispatch(func() {
		if p.impl != nil && !p.isDestroyed() {
			p.impl.setURL(url)
		}
	})
	return p
}

// URL returns the current URL of the panel
func (p *WebviewPanel) URL() string {
	return p.options.URL
}

// SetHTML sets the HTML content of the panel
func (p *WebviewPanel) SetHTML(html string) *WebviewPanel {
	p.options.HTML = html
	p.dispatch(func() {
		if p.impl != nil && !p.isDestroyed() {
			p.impl.setHTML(html)
		}
	})
	return p
}

// ExecJS executes JavaScript in the panel's context
func (p *WebviewPanel) ExecJS(js string) {
	if p.impl == nil || p.isDestroyed() {
		return
	}

	p.runtimeLock.Lock()
	if p.runtimeLoaded {
		p.runtimeLock.Unlock()
		p.dispatch(func() {
			if p.impl != nil && !p.isDestroyed() {
				p.impl.execJS(js)
			}
		})
	} else {
		p.pendingJS = append(p.pendingJS, js)
		p.runtimeLock.Unlock()
	}
}

// Reload reloads the current page
func (p *WebviewPanel) Reload() {
	p.dispatch(func() {
		if p.impl != nil && !p.isDestroyed() {
			p.impl.reload()
		}
	})
}

// ForceReload reloads the current page, bypassing the cache
func (p *WebviewPanel) ForceReload() {
	p.dispatch(func() {
		if p.impl != nil && !p.isDestroyed() {
			p.impl.forceReload()
		}
	})
}

// Show makes the panel visible
func (p *WebviewPanel) Show() *WebviewPanel {
	visible := true
	p.options.Visible = &visible
	p.dispatch(func() {
		if p.impl != nil && !p.isDestroyed() {
			p.impl.show()
		}
	})
	return p
}

// Hide hides the panel
func (p *WebviewPanel) Hide() *WebviewPanel {
	visible := false
	p.options.Visible = &visible
	p.dispatch(func() {
		if p.impl != nil && !p.isDestroyed() {
			p.impl.hide()
		}
	})
	return p
}

// IsVisible returns whether the panel is currently visible
func (p *WebviewPanel) IsVisible() bool {
	if p.impl != nil && !p.isDestroyed() {
		var res bool
		p.dispatch(func() {
			if p.impl != nil && !p.isDestroyed() {
				res = p.impl.isVisible()
			}
		})
		return res
	}
	return p.options.Visible != nil && *p.options.Visible
}

// SetZoom sets the zoom level of the panel
func (p *WebviewPanel) SetZoom(zoom float64) *WebviewPanel {
	p.options.Zoom = zoom
	p.dispatch(func() {
		if p.impl != nil && !p.isDestroyed() {
			p.impl.setZoom(zoom)
		}
	})
	return p
}

// GetZoom returns the current zoom level of the panel
func (p *WebviewPanel) GetZoom() float64 {
	if p.impl != nil && !p.isDestroyed() {
		var res float64
		p.dispatch(func() {
			if p.impl != nil && !p.isDestroyed() {
				res = p.impl.getZoom()
			}
		})
		return res
	}
	return p.options.Zoom
}

// OpenDevTools opens the developer tools for this panel
func (p *WebviewPanel) OpenDevTools() {
	p.dispatch(func() {
		if p.impl != nil && !p.isDestroyed() {
			p.impl.openDevTools()
		}
	})
}

// Focus gives focus to this panel
func (p *WebviewPanel) Focus() {
	p.dispatch(func() {
		if p.impl != nil && !p.isDestroyed() {
			p.impl.focus()
		}
	})
}

// IsFocused returns whether this panel currently has focus
func (p *WebviewPanel) IsFocused() bool {
	if p.impl != nil && !p.isDestroyed() {
		var res bool
		p.dispatch(func() {
			if p.impl != nil && !p.isDestroyed() {
				res = p.impl.isFocused()
			}
		})
		return res
	}
	return false
}

// Destroy removes the panel from its parent window and releases resources
func (p *WebviewPanel) Destroy() {
	if p.isDestroyed() {
		return
	}

	p.destroyedLock.Lock()
	p.destroyed = true
	p.destroyedLock.Unlock()

	p.dispatch(func() {
		if p.impl != nil {
			p.impl.destroy()
		}
	})

	// Remove from manager
	if p.manager != nil {
		p.manager.removePanel(p.id)
	}
}

// isDestroyed returns whether the panel has been destroyed
func (p *WebviewPanel) isDestroyed() bool {
	p.destroyedLock.RLock()
	defer p.destroyedLock.RUnlock()
	return p.destroyed
}

// run initializes the platform-specific implementation
func (p *WebviewPanel) run(parentHwnd uintptr) {
	p.destroyedLock.Lock()
	if p.impl != nil || p.destroyed {
		p.destroyedLock.Unlock()
		return
	}
	p.impl = newPanelImpl(p, parentHwnd)
	p.destroyedLock.Unlock()

	p.dispatch(func() {
		if p.impl != nil {
			p.impl.create()
		}
	})
}

// markRuntimeLoaded is called when the runtime JavaScript has been loaded
func (p *WebviewPanel) markRuntimeLoaded() {
	p.runtimeLock.Lock()
	p.runtimeLoaded = true
	pendingJS := p.pendingJS
	p.pendingJS = nil
	p.runtimeLock.Unlock()

	// Execute any pending JavaScript outside the lock
	for _, js := range pendingJS {
		jsCopy := js
		p.dispatch(func() {
			if p.impl != nil && !p.isDestroyed() {
				p.impl.execJS(jsCopy)
			}
		})
	}
}
