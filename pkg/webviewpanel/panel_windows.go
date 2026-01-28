//go:build windows

package webviewpanel

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"

	"github.com/wailsapp/go-webview2/pkg/edge"
	"golang.org/x/sys/windows"
)

var (
	user32   = windows.NewLazySystemDLL("user32.dll")
	kernel32 = windows.NewLazySystemDLL("kernel32.dll")
	ole32    = windows.NewLazySystemDLL("ole32.dll")
	gdi32    = windows.NewLazySystemDLL("gdi32.dll")

	procCreateWindowExW  = user32.NewProc("CreateWindowExW")
	procDestroyWindow    = user32.NewProc("DestroyWindow")
	procShowWindow       = user32.NewProc("ShowWindow")
	procSetWindowPos     = user32.NewProc("SetWindowPos")
	procGetWindowRect    = user32.NewProc("GetWindowRect")
	procGetClientRect    = user32.NewProc("GetClientRect")
	procClientToScreen   = user32.NewProc("ClientToScreen")
	procSetFocus         = user32.NewProc("SetFocus")
	procGetFocus         = user32.NewProc("GetFocus")
	procGetWindowLongW   = user32.NewProc("GetWindowLongW")
	procGetDpiForWindow  = user32.NewProc("GetDpiForWindow")
	procRegisterClassExW = user32.NewProc("RegisterClassExW")
	procDefWindowProcW   = user32.NewProc("DefWindowProcW")

	procGetModuleHandleW = kernel32.NewProc("GetModuleHandleW")

	procCoInitializeEx = ole32.NewProc("CoInitializeEx")

	procCreateSolidBrush = gdi32.NewProc("CreateSolidBrush")
)

const (
	COINIT_APARTMENTTHREADED = 0x2
)

const (
	WS_CHILD        = 0x40000000
	WS_VISIBLE      = 0x10000000
	WS_CLIPSIBLINGS = 0x04000000
	WS_CLIPCHILDREN = 0x02000000

	SW_SHOW = 5
	SW_HIDE = 0

	SWP_NOZORDER   = 0x0004
	SWP_NOACTIVATE = 0x0010
	SWP_NOMOVE     = 0x0002
	SWP_NOSIZE     = 0x0001

	HWND_TOP    = 0
	HWND_BOTTOM = 1
)

// GWL_STYLE needs special handling for 64-bit
var gwlStyle = ^uintptr(15) // equivalent to -16 as uintptr

type RECT struct {
	Left, Top, Right, Bottom int32
}

type POINT struct {
	X, Y int32
}

type WNDCLASSEX struct {
	Size       uint32
	Style      uint32
	WndProc    uintptr
	ClsExtra   int32
	WndExtra   int32
	Instance   syscall.Handle
	Icon       syscall.Handle
	Cursor     syscall.Handle
	Background syscall.Handle
	MenuName   *uint16
	ClassName  *uint16
	IconSm     syscall.Handle
}

var (
	panelClassRegistered bool
	panelClassName       = syscall.StringToUTF16Ptr("WailsPanelClass")
	panelBgBrush         syscall.Handle // dark background brush to avoid white flash
)

type windowsPanelImpl struct {
	panel      *WebviewPanel
	parentHwnd uintptr
	hwnd       uintptr
	chromium   *edge.Chromium
	debugMode  bool

	// Track navigation state
	navigationCompleted bool

	// Prevent initial "blank chrome frame" flash:
	// keep WebView2 controller hidden until first NavigationCompleted.
	controllerShown bool
}

func newPanelImpl(panel *WebviewPanel, parentHwnd uintptr) webviewPanelImpl {
	debugMode := false
	if panel.manager != nil {
		debugMode = panel.manager.IsDebugMode()
	}

	return &windowsPanelImpl{
		panel:      panel,
		parentHwnd: parentHwnd,
		debugMode:  debugMode,
	}
}

func registerPanelClass() {
	if panelClassRegistered {
		return
	}

	hInstance, _, _ := procGetModuleHandleW.Call(0)

	// Create a dark background brush (RGB: 30, 30, 30) to avoid white flash
	// when the child window is visible but WebView2 hasn't rendered yet.
	// COLORREF format: 0x00BBGGRR
	darkColor := uintptr(0x001E1E1E) // RGB(30, 30, 30)
	brush, _, _ := procCreateSolidBrush.Call(darkColor)
	panelBgBrush = syscall.Handle(brush)

	wc := WNDCLASSEX{
		Size:       uint32(unsafe.Sizeof(WNDCLASSEX{})),
		WndProc:    syscall.NewCallback(defWindowProc),
		Instance:   syscall.Handle(hInstance),
		ClassName:  panelClassName,
		Background: panelBgBrush,
	}

	procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))
	panelClassRegistered = true
}

func defWindowProc(hwnd uintptr, msg uint32, wParam, lParam uintptr) uintptr {
	ret, _, _ := procDefWindowProcW.Call(hwnd, uintptr(msg), wParam, lParam)
	return ret
}

func (p *windowsPanelImpl) create() {
	registerPanelClass()

	options := p.panel.options

	// Create a child window to host the WebView2
	// 保持窗口可见（有稳定的 client rect），但让 WebView2 controller 自己先隐藏，
	// 等首个 NavigationCompleted 后再显示，避免“浏览器边框/白块”闪动。
	// Use WS_VISIBLE to ensure WebView2 gets correct client rect dimensions.
	// The dark background brush (set in registerPanelClass) prevents white flash.
	// WebView2 controller is hidden initially and shown after NavigationCompleted.
	style := uintptr(WS_CHILD | WS_CLIPSIBLINGS | WS_CLIPCHILDREN | WS_VISIBLE)

	// Convert DIP coordinates to physical pixels
	bounds := p.dipToPhysical(Rect{
		X:      options.X,
		Y:      options.Y,
		Width:  options.Width,
		Height: options.Height,
	})

	hInstance, _, _ := procGetModuleHandleW.Call(0)

	// Create the child window
	hwnd, _, _ := procCreateWindowExW.Call(
		0,
		uintptr(unsafe.Pointer(panelClassName)),
		0,
		style,
		uintptr(bounds.X),
		uintptr(bounds.Y),
		uintptr(bounds.Width),
		uintptr(bounds.Height),
		p.parentHwnd,
		0,
		hInstance,
		0,
	)

	if hwnd == 0 {
		fmt.Println("failed to create panel child window")
		return
	}

	p.hwnd = hwnd

	// Ensure the panel window is above siblings (Wails main WebView host is also a child window)
	procSetWindowPos.Call(
		p.hwnd,
		HWND_TOP,
		0, 0, 0, 0,
		SWP_NOMOVE|SWP_NOSIZE|SWP_NOACTIVATE,
	)

	// Setup WebView2 (Chromium)
	p.setupChromium()
}

func (p *windowsPanelImpl) setupChromium() {
	// Initialize COM for this thread (required for WebView2)
	// Use COINIT_APARTMENTTHREADED for STA (single-threaded apartment)
	hr, _, _ := procCoInitializeEx.Call(0, COINIT_APARTMENTTHREADED)
	// S_OK = 0, S_FALSE = 1 (already initialized), both are acceptable
	if hr != 0 && hr != 1 {
		fmt.Printf("CoInitializeEx failed with hr=0x%x\n", hr)
	}
	
	p.chromium = edge.NewChromium()
	// Use a dedicated user data folder per panel to avoid environment conflicts
	if p.panel != nil {
		base := os.Getenv("AppData")
		if base == "" {
			base = "."
		}
		panelID := fmt.Sprintf("panel-%d", p.panel.id)
		p.chromium.DataPath = filepath.Join(base, "chatwiki-panels", panelID)
	}
	// Prevent os.Exit on WebView2 errors; log instead
	p.chromium.SetErrorCallback(func(err error) {
		fmt.Printf("[WebView2] panel error: %v\n", err)
	})

	// Embed the WebView2 into our child window
	p.chromium.Embed(p.hwnd)
	// 注意：如果此时子 HWND 还不可见，GetClientRect 可能返回 0，导致 WebView2 实际尺寸为 0。
	// 我们会在真正 show 后再补一次 Resize()，这里先做一次“尽力而为”。
	p.chromium.Resize()

	// Set up callbacks
	p.chromium.MessageCallback = p.processMessage
	p.chromium.NavigationCompletedCallback = p.navigationCompletedCallback

	// Configure settings
	settings, err := p.chromium.GetSettings()
	if err != nil {
		fmt.Printf("failed to get chromium settings: %v\n", err)
		return
	}

	// Disable context menus unless in debug mode or explicitly enabled
	devToolsEnabled := p.debugMode
	if p.panel.options.DevToolsEnabled != nil {
		devToolsEnabled = *p.panel.options.DevToolsEnabled
	}
	_ = settings.PutAreDefaultContextMenusEnabled(devToolsEnabled)
	_ = settings.PutAreDevToolsEnabled(devToolsEnabled)

	// Set zoom if specified
	if p.panel.options.Zoom > 0 && p.panel.options.Zoom != 1.0 {
		p.chromium.PutZoomFactor(p.panel.options.Zoom)
	}

	// Set background colour to match the dark window background brush (RGB 30,30,30)
	// This prevents white flash before content loads.
	if p.panel.options.Transparent {
		p.chromium.SetBackgroundColour(0, 0, 0, 0)
	} else {
		bg := p.panel.options.BackgroundColour
		// If no background colour specified (all zeros), use dark grey to match HWND background
		if bg.Red == 0 && bg.Green == 0 && bg.Blue == 0 && bg.Alpha == 0 {
			p.chromium.SetBackgroundColour(30, 30, 30, 255)
		} else {
			p.chromium.SetBackgroundColour(bg.Red, bg.Green, bg.Blue, bg.Alpha)
		}
	}

	// 关键：先隐藏 controller，等首帧就绪再显示（减少“空白边框/白块”闪动）
	p.controllerShown = false
	_ = p.chromium.Hide()

	// Navigate to initial content
	if p.panel.options.HTML != "" {
		p.loadHTMLWithScripts()
	} else if p.panel.options.URL != "" {
		p.chromium.Navigate(p.panel.options.URL)
	}

	// Open inspector if requested
	if p.debugMode && p.panel.options.OpenInspectorOnStartup {
		p.chromium.OpenDevToolsWindow()
	}

	// 注意：显示时机延后到 navigationCompletedCallback（首帧就绪）
	if p.panel.options.Visible != nil && !*p.panel.options.Visible {
		p.hide()
	}
}

func (p *windowsPanelImpl) loadHTMLWithScripts() {
	var script string
	if p.panel.options.JS != "" {
		script = p.panel.options.JS
	}
	if p.panel.options.CSS != "" {
		// Escape CSS for safe injection into JavaScript string
		escapedCSS := strings.ReplaceAll(p.panel.options.CSS, `\`, `\\`)
		escapedCSS = strings.ReplaceAll(escapedCSS, `"`, `\"`)
		escapedCSS = strings.ReplaceAll(escapedCSS, "\n", `\n`)
		escapedCSS = strings.ReplaceAll(escapedCSS, "\r", `\r`)
		script += fmt.Sprintf(
			"; addEventListener(\"DOMContentLoaded\", (event) => { document.head.appendChild(document.createElement('style')).innerHTML=\"%s\"; });",
			escapedCSS,
		)
	}
	if script != "" {
		p.chromium.Init(script)
	}
	p.chromium.NavigateToString(p.panel.options.HTML)
}

func (p *windowsPanelImpl) processMessage(message string, _ *edge.ICoreWebView2, _ *edge.ICoreWebView2WebMessageReceivedEventArgs) {
	// For now, just log panel messages
	fmt.Printf("Panel message received: panel=%s, message=%s\n", p.panel.name, message)
}

func (p *windowsPanelImpl) navigationCompletedCallback(_ *edge.ICoreWebView2, _ *edge.ICoreWebView2NavigationCompletedEventArgs) {
	p.navigationCompleted = true

	// First paint is ready: show controller now to avoid initial flash.
	if !p.controllerShown {
		p.controllerShown = true
		shouldShow := p.panel.options.Visible == nil || *p.panel.options.Visible
		if shouldShow {
			p.show()
		}
	}

	// Execute any pending JS
	if p.panel.options.JS != "" && p.panel.options.HTML == "" {
		p.execJS(p.panel.options.JS)
	}
	if p.panel.options.CSS != "" && p.panel.options.HTML == "" {
		// Escape CSS for safe injection into JavaScript string
		escapedCSS := strings.ReplaceAll(p.panel.options.CSS, `\`, `\\`)
		escapedCSS = strings.ReplaceAll(escapedCSS, `'`, `\'`)
		escapedCSS = strings.ReplaceAll(escapedCSS, "\n", `\n`)
		escapedCSS = strings.ReplaceAll(escapedCSS, "\r", `\r`)
		js := fmt.Sprintf(
			"(function() { var style = document.createElement('style'); style.appendChild(document.createTextNode('%s')); document.head.appendChild(style); })();",
			escapedCSS,
		)
		p.execJS(js)
	}

	// Mark runtime as loaded
	p.panel.markRuntimeLoaded()
}

func (p *windowsPanelImpl) destroy() {
	if p.chromium != nil {
		p.chromium.ShuttingDown()
	}
	if p.hwnd != 0 {
		procDestroyWindow.Call(p.hwnd)
		p.hwnd = 0
	}
	p.chromium = nil
}

func (p *windowsPanelImpl) setBounds(bounds Rect) {
	if p.hwnd == 0 {
		return
	}

	// Convert DIP to physical pixels
	physicalBounds := p.dipToPhysical(bounds)

	// Move and resize the child window
	procSetWindowPos.Call(
		p.hwnd,
		HWND_TOP,
		uintptr(physicalBounds.X),
		uintptr(physicalBounds.Y),
		uintptr(physicalBounds.Width),
		uintptr(physicalBounds.Height),
		SWP_NOACTIVATE,
	)

	// Resize the WebView2 to fill the child window
	if p.chromium != nil {
		p.chromium.Resize()
	}
}

func (p *windowsPanelImpl) bounds() Rect {
	if p.hwnd == 0 {
		return Rect{}
	}

	var rect RECT
	procGetWindowRect.Call(p.hwnd, uintptr(unsafe.Pointer(&rect)))

	// Get parent window position to calculate relative position
	var parentRect RECT
	procGetWindowRect.Call(p.parentHwnd, uintptr(unsafe.Pointer(&parentRect)))

	// Calculate position relative to parent's client area
	var clientPoint POINT
	procClientToScreen.Call(p.parentHwnd, uintptr(unsafe.Pointer(&clientPoint)))

	physicalBounds := Rect{
		X:      int(rect.Left) - int(clientPoint.X),
		Y:      int(rect.Top) - int(clientPoint.Y),
		Width:  int(rect.Right - rect.Left),
		Height: int(rect.Bottom - rect.Top),
	}

	return p.physicalToDip(physicalBounds)
}

func (p *windowsPanelImpl) setZIndex(zIndex int) {
	if p.hwnd == 0 {
		return
	}

	// Use SetWindowPos to change z-order
	var insertAfter uintptr
	if zIndex > 0 {
		insertAfter = HWND_TOP
	} else {
		insertAfter = HWND_BOTTOM
	}

	procSetWindowPos.Call(
		p.hwnd,
		insertAfter,
		0, 0, 0, 0,
		SWP_NOMOVE|SWP_NOSIZE|SWP_NOACTIVATE,
	)
}

func (p *windowsPanelImpl) setURL(url string) {
	if p.chromium == nil {
		return
	}
	p.navigationCompleted = false
	p.chromium.Navigate(url)
}

func (p *windowsPanelImpl) setHTML(html string) {
	if p.chromium == nil {
		return
	}
	p.chromium.NavigateToString(html)
}

func (p *windowsPanelImpl) execJS(js string) {
	if p.chromium == nil {
		return
	}
	p.chromium.Eval(js)
}

func (p *windowsPanelImpl) reload() {
	p.execJS("window.location.reload();")
}

func (p *windowsPanelImpl) forceReload() {
	// WebView2 doesn't have a cache-bypass reload, so just reload normally
	p.reload()
}

func (p *windowsPanelImpl) show() {
	if p.hwnd == 0 {
		return
	}
	// Keep it above siblings
	procSetWindowPos.Call(
		p.hwnd,
		HWND_TOP,
		0, 0, 0, 0,
		SWP_NOMOVE|SWP_NOSIZE|SWP_NOACTIVATE,
	)
	procShowWindow.Call(p.hwnd, SW_SHOW)
	if p.chromium != nil {
		// WebView2 controller visibility is separate from HWND visibility.
		// If the HWND was created hidden, the controller may start as invisible.
		_ = p.chromium.Show()
		p.chromium.Resize()
	}
}

func (p *windowsPanelImpl) hide() {
	if p.hwnd == 0 {
		return
	}
	if p.chromium != nil {
		_ = p.chromium.Hide()
	}
	procShowWindow.Call(p.hwnd, SW_HIDE)
}

func (p *windowsPanelImpl) isVisible() bool {
	if p.hwnd == 0 {
		return false
	}
	style, _, _ := procGetWindowLongW.Call(p.hwnd, gwlStyle)
	return style&WS_VISIBLE != 0
}

func (p *windowsPanelImpl) setZoom(zoom float64) {
	if p.chromium == nil {
		return
	}
	p.chromium.PutZoomFactor(zoom)
}

func (p *windowsPanelImpl) getZoom() float64 {
	if p.chromium == nil {
		return 1.0
	}
	controller := p.chromium.GetController()
	if controller == nil {
		return 1.0
	}
	factor, err := controller.GetZoomFactor()
	if err != nil {
		return 1.0
	}
	return factor
}

func (p *windowsPanelImpl) openDevTools() {
	if p.chromium == nil {
		return
	}
	p.chromium.OpenDevToolsWindow()
}

func (p *windowsPanelImpl) focus() {
	if p.hwnd == 0 {
		return
	}
	procSetFocus.Call(p.hwnd)
	if p.chromium != nil {
		p.chromium.Focus()
	}
}

func (p *windowsPanelImpl) isFocused() bool {
	if p.hwnd == 0 {
		return false
	}
	focusedHwnd, _, _ := procGetFocus.Call()
	return focusedHwnd == p.hwnd
}

// DPI scaling helpers
func (p *windowsPanelImpl) getDPI() float64 {
	if p.parentHwnd == 0 {
		return 96.0
	}
	dpi, _, _ := procGetDpiForWindow.Call(p.parentHwnd)
	if dpi == 0 {
		return 96.0
	}
	return float64(dpi)
}

func (p *windowsPanelImpl) dipToPhysical(rect Rect) Rect {
	scale := p.getDPI() / 96.0
	return Rect{
		X:      int(float64(rect.X) * scale),
		Y:      int(float64(rect.Y) * scale),
		Width:  int(float64(rect.Width) * scale),
		Height: int(float64(rect.Height) * scale),
	}
}

func (p *windowsPanelImpl) physicalToDip(rect Rect) Rect {
	scale := p.getDPI() / 96.0
	return Rect{
		X:      int(float64(rect.X) / scale),
		Y:      int(float64(rect.Y) / scale),
		Width:  int(float64(rect.Width) / scale),
		Height: int(float64(rect.Height) / scale),
	}
}
