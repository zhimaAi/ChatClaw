package webviewpanel

import (
	"sync"
)

// PanelManager manages WebviewPanels for a window.
// It provides methods to create, retrieve, and manage panels.
type PanelManager struct {
	parentHwnd uintptr // Parent window handle
	panels     map[uint]*WebviewPanel
	panelsLock sync.RWMutex
	debugMode  bool

	// dispatchSync, if set, runs the given function on the UI thread that owns the parent window.
	// This is critical on Windows because WebView2/COM objects are thread-affine.
	dispatchSync func(func())
}

// NewPanelManager creates a new panel manager for the specified parent window.
// parentHwnd is the native window handle (HWND on Windows, NSWindow* on macOS).
// debugMode enables developer tools and additional logging.
func NewPanelManager(parentHwnd uintptr, debugMode bool) *PanelManager {
	return &PanelManager{
		parentHwnd: parentHwnd,
		panels:     make(map[uint]*WebviewPanel),
		debugMode:  debugMode,
	}
}

// SetDispatchSync configures a synchronous dispatcher used to run panel operations on the correct UI thread.
// In a Wails app on Windows, you typically pass application.InvokeSync.
func (m *PanelManager) SetDispatchSync(dispatch func(func())) {
	m.dispatchSync = dispatch
}

func (m *PanelManager) dispatch(fn func()) {
	if m.dispatchSync != nil {
		m.dispatchSync(fn)
		return
	}
	fn()
}

// NewPanel creates a new WebviewPanel with the given options and adds it to this manager.
// The panel is a secondary webview that can be positioned anywhere within the window.
//
// Example:
//
//	panel := manager.NewPanel(webviewpanel.WebviewPanelOptions{
//		X:      0,
//		Y:      0,
//		Width:  300,
//		Height: 400,
//		URL:    "https://example.com",
//	})
func (m *PanelManager) NewPanel(options WebviewPanelOptions) *WebviewPanel {
	panel := newPanel(options, m)

	m.panelsLock.Lock()
	m.panels[panel.id] = panel
	m.panelsLock.Unlock()

	// Start the panel
	m.dispatch(func() {
		panel.run(m.parentHwnd)
	})

	return panel
}

// GetPanel returns a panel by its name, or nil if not found.
func (m *PanelManager) GetPanel(name string) *WebviewPanel {
	m.panelsLock.RLock()
	defer m.panelsLock.RUnlock()

	for _, panel := range m.panels {
		if panel.name == name {
			return panel
		}
	}
	return nil
}

// GetPanelByID returns a panel by its ID, or nil if not found.
func (m *PanelManager) GetPanelByID(id uint) *WebviewPanel {
	m.panelsLock.RLock()
	defer m.panelsLock.RUnlock()
	return m.panels[id]
}

// GetPanels returns all panels managed by this manager.
func (m *PanelManager) GetPanels() []*WebviewPanel {
	m.panelsLock.RLock()
	defer m.panelsLock.RUnlock()

	panels := make([]*WebviewPanel, 0, len(m.panels))
	for _, panel := range m.panels {
		panels = append(panels, panel)
	}
	return panels
}

// RemovePanel removes a panel by its name.
// Returns true if the panel was found and removed.
func (m *PanelManager) RemovePanel(name string) bool {
	panel := m.GetPanel(name)
	if panel == nil {
		return false
	}
	panel.Destroy()
	return true
}

// RemovePanelByID removes a panel by its ID.
// Returns true if the panel was found and removed.
func (m *PanelManager) RemovePanelByID(id uint) bool {
	panel := m.GetPanelByID(id)
	if panel == nil {
		return false
	}
	panel.Destroy()
	return true
}

// removePanel is called by WebviewPanel.Destroy() to remove itself from the manager
func (m *PanelManager) removePanel(id uint) {
	m.panelsLock.Lock()
	defer m.panelsLock.Unlock()
	delete(m.panels, id)
}

// DestroyAll destroys all panels managed by this manager.
func (m *PanelManager) DestroyAll() {
	m.panelsLock.Lock()
	panels := make([]*WebviewPanel, 0, len(m.panels))
	for _, panel := range m.panels {
		panels = append(panels, panel)
	}
	m.panelsLock.Unlock()

	for _, panel := range panels {
		panel.Destroy()
	}
}

// IsDebugMode returns whether debug mode is enabled.
func (m *PanelManager) IsDebugMode() bool {
	return m.debugMode
}

// ParentHwnd returns the parent window handle.
func (m *PanelManager) ParentHwnd() uintptr {
	return m.parentHwnd
}
