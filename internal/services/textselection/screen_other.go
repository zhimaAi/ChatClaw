//go:build !windows

package textselection

// WorkArea represents a screen's work area (excluding taskbar)
type WorkArea struct {
	X      int
	Y      int
	Width  int
	Height int
}

// getWorkAreaAtPoint returns the work area of the monitor containing the specified point.
// On non-Windows platforms, this returns a default work area.
// macOS handles multi-monitor differently through its coordinate system.
func getWorkAreaAtPoint(x, y int) WorkArea {
	// On macOS, the coordinate system is unified across all displays
	// and Wails handles positioning correctly.
	// Return a large default area that won't cause clamping issues.
	return WorkArea{X: -10000, Y: -10000, Width: 30000, Height: 30000}
}

// clampToWorkArea ensures the popup position is within the work area bounds.
// On non-Windows platforms, this is a no-op as macOS handles coordinates differently.
func clampToWorkArea(popX, popY, popWidth, popHeight, mouseX, mouseY int) (int, int) {
	// macOS uses a unified coordinate system, no clamping needed
	return popX, popY
}
