//go:build windows

package textselection

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	procMonitorFromPoint = modUser32.NewProc("MonitorFromPoint")
	procGetMonitorInfoW  = modUser32.NewProc("GetMonitorInfoW")
)

const (
	monitorDefaultToNearest = 2 // MONITOR_DEFAULTTONEAREST
)

// POINT structure for Windows API
type pointStruct struct {
	X int32
	Y int32
}

// MONITORINFO structure for Windows API
type monitorInfo struct {
	CbSize    uint32
	RcMonitor windows.Rect
	RcWork    windows.Rect
	DwFlags   uint32
}

// WorkArea represents a screen's work area (excluding taskbar)
type WorkArea struct {
	X      int
	Y      int
	Width  int
	Height int
}

// getWorkAreaAtPoint returns the work area of the monitor containing the specified point.
// This handles multi-monitor setups correctly by finding the monitor at the given coordinates.
func getWorkAreaAtPoint(x, y int) WorkArea {
	pt := pointStruct{X: int32(x), Y: int32(y)}

	// Get the monitor that contains the point
	hMonitor, _, _ := procMonitorFromPoint.Call(
		uintptr(pt.X),
		uintptr(pt.Y),
		monitorDefaultToNearest,
	)

	if hMonitor == 0 {
		// Fallback to default values
		return WorkArea{X: 0, Y: 0, Width: 1920, Height: 1080}
	}

	// Get monitor info
	var mi monitorInfo
	mi.CbSize = uint32(unsafe.Sizeof(mi))
	ret, _, _ := procGetMonitorInfoW.Call(hMonitor, uintptr(unsafe.Pointer(&mi)))
	if ret == 0 {
		// Fallback to default values
		return WorkArea{X: 0, Y: 0, Width: 1920, Height: 1080}
	}

	return WorkArea{
		X:      int(mi.RcWork.Left),
		Y:      int(mi.RcWork.Top),
		Width:  int(mi.RcWork.Right - mi.RcWork.Left),
		Height: int(mi.RcWork.Bottom - mi.RcWork.Top),
	}
}

// clampToWorkArea ensures the popup position is within the work area bounds.
// Parameters:
//   - popX, popY: proposed popup position
//   - popWidth, popHeight: popup dimensions
//   - mouseX, mouseY: mouse position (used to determine which monitor)
//
// Returns adjusted x, y coordinates.
func clampToWorkArea(popX, popY, popWidth, popHeight, mouseX, mouseY int) (int, int) {
	wa := getWorkAreaAtPoint(mouseX, mouseY)

	// Clamp X
	if popX < wa.X {
		popX = wa.X
	}
	if popX+popWidth > wa.X+wa.Width {
		popX = wa.X + wa.Width - popWidth
	}

	// Clamp Y - if above screen top, show below mouse instead
	if popY < wa.Y {
		// Show below mouse with some offset
		popY = mouseY + 20
	}
	if popY+popHeight > wa.Y+wa.Height {
		popY = wa.Y + wa.Height - popHeight
	}

	return popX, popY
}
