package webviewpanel

import (
	"sync"
)

// TabInfo represents a browser tab's information
type TabInfo struct {
	ID     uint   `json:"id"`
	URL    string `json:"url"`
	Title  string `json:"title"`
	Active bool   `json:"active"`
}

// TabBrowserConfig contains configuration for TabBrowser
type TabBrowserConfig struct {
	// DebugMode enables developer tools
	DebugMode bool

	// DefaultURL is the URL to load when creating a new tab without specifying a URL
	DefaultURL string

	// InitialTabs are URLs to open when the browser is first activated
	InitialTabs []string

	// OnTabsChanged is called whenever the tab list changes
	// The callback receives the current list of tabs and the active tab index
	OnTabsChanged func(tabs []TabInfo, activeIdx int)
}

// TabBrowser provides a high-level API for managing a multi-tab browser experience.
// It wraps PanelManager and provides tab-specific operations like switching,
// creating, and closing tabs with proper visibility management.
//
// Example usage:
//
//	browser := webviewpanel.NewTabBrowser(hwnd, webviewpanel.TabBrowserConfig{
//		DebugMode:   true,
//		DefaultURL:  "https://www.bing.com",
//		InitialTabs: []string{"https://www.baidu.com", "https://www.google.com"},
//		OnTabsChanged: func(tabs []TabInfo, activeIdx int) {
//			// Update UI
//		},
//	})
//
//	browser.SetDispatchSync(application.InvokeSync) // Required on Windows
//	browser.Activate(layout)
type TabBrowser struct {
	manager      *PanelManager
	config       TabBrowserConfig
	tabs         []*tabEntry
	activeTabIdx int
	layout       Rect
	haveLayout   bool
	active       bool
	initialized  bool

	mu sync.Mutex
}

type tabEntry struct {
	panel *WebviewPanel
	url   string
	title string
}

// NewTabBrowser creates a new TabBrowser instance.
// parentHwnd is the native window handle where panels will be embedded.
func NewTabBrowser(parentHwnd uintptr, config TabBrowserConfig) *TabBrowser {
	if config.DefaultURL == "" {
		config.DefaultURL = "https://www.bing.com"
	}

	return &TabBrowser{
		manager:      NewPanelManager(parentHwnd, config.DebugMode),
		config:       config,
		tabs:         make([]*tabEntry, 0),
		activeTabIdx: -1,
	}
}

// SetDispatchSync configures the UI thread dispatcher.
// This is required on Windows due to WebView2/COM thread affinity requirements.
// In a Wails app, pass application.InvokeSync.
func (b *TabBrowser) SetDispatchSync(dispatch func(func())) {
	b.manager.SetDispatchSync(dispatch)
}

// SetLayout updates the layout rectangle where tabs will be displayed.
// This should be called when the container's size or position changes.
func (b *TabBrowser) SetLayout(layout Rect) {
	b.mu.Lock()
	b.layout = layout
	b.haveLayout = true
	active := b.active
	b.mu.Unlock()

	if !active {
		return
	}

	// Apply layout to all existing panels
	b.applyLayout()
}

// Activate activates the tab browser and shows the current active tab.
// If no tabs exist and InitialTabs is configured, those tabs will be created.
func (b *TabBrowser) Activate(layout Rect) {
	b.mu.Lock()
	b.active = true
	b.layout = layout
	b.haveLayout = layout.Width > 0 && layout.Height > 0
	b.mu.Unlock()

	// Create initial tabs if needed
	if !b.initialized && b.haveLayout {
		b.initialized = true
		if len(b.config.InitialTabs) > 0 {
			for _, url := range b.config.InitialTabs {
				b.createTabInternal(url)
			}
		}
	}

	b.applyLayout()
	b.showActiveTab()
	b.notifyTabsChanged()
}

// Deactivate hides all tabs (but doesn't destroy them).
func (b *TabBrowser) Deactivate() {
	b.mu.Lock()
	b.active = false
	b.mu.Unlock()

	for _, tab := range b.tabs {
		tab.panel.Hide()
	}
}

// NewTab creates a new tab with the given URL.
// If url is empty, the DefaultURL from config is used.
// Returns the index of the new tab.
func (b *TabBrowser) NewTab(url string) int {
	if url == "" {
		url = b.config.DefaultURL
	}

	b.mu.Lock()
	if !b.haveLayout {
		b.mu.Unlock()
		return -1
	}
	b.mu.Unlock()

	b.createTabInternal(url)
	b.notifyTabsChanged()

	return len(b.tabs) - 1
}

// SwitchTab switches to the tab at the given index.
func (b *TabBrowser) SwitchTab(idx int) bool {
	b.mu.Lock()
	if idx < 0 || idx >= len(b.tabs) {
		b.mu.Unlock()
		return false
	}
	b.mu.Unlock()

	// Hide current tab
	b.mu.Lock()
	if b.activeTabIdx >= 0 && b.activeTabIdx < len(b.tabs) && b.activeTabIdx != idx {
		b.tabs[b.activeTabIdx].panel.Hide()
	}
	b.activeTabIdx = idx
	b.mu.Unlock()

	// Show new tab
	b.tabs[idx].panel.Show()
	b.tabs[idx].panel.Focus()
	b.notifyTabsChanged()

	return true
}

// CloseTab closes the tab at the given index.
func (b *TabBrowser) CloseTab(idx int) bool {
	b.mu.Lock()
	if idx < 0 || idx >= len(b.tabs) {
		b.mu.Unlock()
		return false
	}

	// Destroy panel
	tab := b.tabs[idx]
	tab.panel.Destroy()

	// Remove from list
	b.tabs = append(b.tabs[:idx], b.tabs[idx+1:]...)

	// Adjust active index
	wasActive := b.activeTabIdx == idx
	if b.activeTabIdx > idx {
		b.activeTabIdx--
	} else if wasActive {
		if len(b.tabs) > 0 {
			newIdx := idx
			if newIdx >= len(b.tabs) {
				newIdx = len(b.tabs) - 1
			}
			b.activeTabIdx = newIdx
		} else {
			b.activeTabIdx = -1
		}
	}
	b.mu.Unlock()

	// Show new active tab
	if wasActive && len(b.tabs) > 0 {
		b.tabs[b.activeTabIdx].panel.Show()
		b.tabs[b.activeTabIdx].panel.Focus()
	}

	b.notifyTabsChanged()
	return true
}

// Navigate navigates the current active tab to the given URL.
func (b *TabBrowser) Navigate(url string) bool {
	b.mu.Lock()
	if b.activeTabIdx < 0 || b.activeTabIdx >= len(b.tabs) {
		b.mu.Unlock()
		return false
	}
	tab := b.tabs[b.activeTabIdx]
	tab.url = url
	b.mu.Unlock()

	tab.panel.SetURL(url)
	b.notifyTabsChanged()
	return true
}

// Refresh reloads the current active tab.
func (b *TabBrowser) Refresh() bool {
	b.mu.Lock()
	if b.activeTabIdx < 0 || b.activeTabIdx >= len(b.tabs) {
		b.mu.Unlock()
		return false
	}
	tab := b.tabs[b.activeTabIdx]
	b.mu.Unlock()

	tab.panel.Reload()
	return true
}

// GetTabs returns the current list of tabs.
func (b *TabBrowser) GetTabs() []TabInfo {
	b.mu.Lock()
	defer b.mu.Unlock()

	tabs := make([]TabInfo, len(b.tabs))
	for i, tab := range b.tabs {
		tabs[i] = TabInfo{
			ID:     tab.panel.ID(),
			URL:    tab.url,
			Title:  tab.title,
			Active: i == b.activeTabIdx,
		}
	}
	return tabs
}

// GetActiveTabIndex returns the index of the currently active tab.
func (b *TabBrowser) GetActiveTabIndex() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.activeTabIdx
}

// TabCount returns the number of tabs.
func (b *TabBrowser) TabCount() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.tabs)
}

// Destroy closes all tabs and cleans up resources.
func (b *TabBrowser) Destroy() {
	b.mu.Lock()
	b.active = false
	b.mu.Unlock()

	b.manager.DestroyAll()

	b.mu.Lock()
	b.tabs = make([]*tabEntry, 0)
	b.activeTabIdx = -1
	b.initialized = false
	b.mu.Unlock()
}

// --- Internal methods ---

func (b *TabBrowser) createTabInternal(url string) {
	b.mu.Lock()
	layout := b.layout
	// Hide current active tab
	if b.activeTabIdx >= 0 && b.activeTabIdx < len(b.tabs) {
		b.tabs[b.activeTabIdx].panel.Hide()
	}
	b.mu.Unlock()

	// Create new panel
	visible := true
	panel := b.manager.NewPanel(WebviewPanelOptions{
		URL:             url,
		X:               layout.X,
		Y:               layout.Y,
		Width:           layout.Width,
		Height:          layout.Height,
		Visible:         &visible,
		DevToolsEnabled: boolPtr(b.config.DebugMode),
		ZIndex:          1,
	})

	if panel == nil {
		return
	}

	tab := &tabEntry{
		panel: panel,
		url:   url,
		title: url,
	}

	b.mu.Lock()
	b.tabs = append(b.tabs, tab)
	b.activeTabIdx = len(b.tabs) - 1
	b.mu.Unlock()
}

func (b *TabBrowser) applyLayout() {
	b.mu.Lock()
	if !b.haveLayout || b.layout.Width < 1 || b.layout.Height < 1 {
		b.mu.Unlock()
		return
	}
	layout := b.layout
	tabs := b.tabs
	b.mu.Unlock()

	for _, tab := range tabs {
		tab.panel.SetBounds(layout)
		tab.panel.SetZIndex(1)
	}
}

func (b *TabBrowser) showActiveTab() {
	b.mu.Lock()
	activeIdx := b.activeTabIdx
	tabs := b.tabs
	b.mu.Unlock()

	for i, tab := range tabs {
		if i == activeIdx {
			tab.panel.Show()
			tab.panel.Focus()
		} else {
			tab.panel.Hide()
		}
	}
}

func (b *TabBrowser) notifyTabsChanged() {
	if b.config.OnTabsChanged == nil {
		return
	}

	tabs := b.GetTabs()
	b.mu.Lock()
	activeIdx := b.activeTabIdx
	b.mu.Unlock()

	b.config.OnTabsChanged(tabs, activeIdx)
}

func boolPtr(v bool) *bool {
	return &v
}
