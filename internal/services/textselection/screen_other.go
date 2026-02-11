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

// clampToWorkArea is a no-op on non-Windows platforms.
// macOS uses Cocoa's unified coordinate system which handles multi-monitor natively.
func clampToWorkArea(popX, popY, popWidth, popHeight, mouseX, mouseY int) (int, int) {
	return popX, popY
}

// getDPIScaleForPoint is only meaningful on Windows; returns 1.0 on other platforms.
func getDPIScaleForPoint(_, _ int32) float64 {
	return 1.0
}
