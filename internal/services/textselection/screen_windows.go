//go:build windows

package textselection

import (
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	procMonitorFromPoint = modUser32.NewProc("MonitorFromPoint")
	procGetMonitorInfoW  = modUser32.NewProc("GetMonitorInfoW")
	procGetDpiForMonitor = modShcore.NewProc("GetDpiForMonitor")
)

const (
	monitorDefaultToNearest = 2 // MONITOR_DEFAULTTONEAREST
	mdtEffectiveDPI         = 0 // MDT_EFFECTIVE_DPI
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

// monitorFromPointPacked calls MonitorFromPoint with correctly packed POINT parameter.
// On x64 Windows, MonitorFromPoint(POINT pt, DWORD dwFlags) expects the POINT struct
// (8 bytes) to be packed into a single 64-bit register: low 32 bits = x, high 32 bits = y.
// Passing x and y as separate parameters is WRONG on x64 — it causes y to be read as 0
// and dwFlags to be misinterpreted as the y value.
func monitorFromPointPacked(x, y int32, flags uintptr) uintptr {
	// Pack POINT: low 32 bits = x, high 32 bits = y
	packed := uintptr(uint32(x)) | (uintptr(uint32(y)) << 32)
	hMonitor, _, _ := procMonitorFromPoint.Call(packed, flags)
	return hMonitor
}

// getDPIScaleForPoint returns the DPI scale factor for the monitor containing the specified point.
// This correctly handles multi-monitor setups where each monitor may have a different DPI.
// Falls back to the cached system DPI if per-monitor API is unavailable (pre-Windows 8.1).
func getDPIScaleForPoint(x, y int32) float64 {
	hMonitor := monitorFromPointPacked(x, y, monitorDefaultToNearest)

	if hMonitor == 0 {
		fmt.Printf("[MONITOR-DEBUG] getDPIScaleForPoint: MonitorFromPoint returned NULL for (%d,%d), fallback to system DPI\n", x, y)
		return getDPIScale() // fallback to system DPI
	}

	// Try GetDpiForMonitor (Windows 8.1+)
	if procGetDpiForMonitor.Find() == nil {
		var dpiX, dpiY uint32
		ret, _, _ := procGetDpiForMonitor.Call(
			hMonitor,
			mdtEffectiveDPI,
			uintptr(unsafe.Pointer(&dpiX)),
			uintptr(unsafe.Pointer(&dpiY)),
		)
		if ret == 0 && dpiX > 0 { // S_OK = 0
			scale := float64(dpiX) / 96.0
			fmt.Printf("[MONITOR-DEBUG] getDPIScaleForPoint(%d,%d) → dpi=%d scale=%.2f\n", x, y, dpiX, scale)
			return scale
		}
	}

	// Fallback to system DPI
	fmt.Printf("[MONITOR-DEBUG] getDPIScaleForPoint: GetDpiForMonitor failed for (%d,%d), fallback to system DPI\n", x, y)
	return getDPIScale()
}

// getWorkAreaAtPoint returns the work area of the monitor containing the specified point.
// This handles multi-monitor setups correctly by finding the monitor at the given coordinates.
func getWorkAreaAtPoint(x, y int) WorkArea {
	// Use correctly packed MonitorFromPoint call
	hMonitor := monitorFromPointPacked(int32(x), int32(y), monitorDefaultToNearest)

	if hMonitor == 0 {
		fmt.Printf("[MONITOR-DEBUG] getWorkAreaAtPoint: MonitorFromPoint returned NULL for (%d,%d), using fallback\n", x, y)
		// Fallback to default values
		return WorkArea{X: 0, Y: 0, Width: 1920, Height: 1080}
	}

	// Get monitor info
	var mi monitorInfo
	mi.CbSize = uint32(unsafe.Sizeof(mi))
	ret, _, _ := procGetMonitorInfoW.Call(hMonitor, uintptr(unsafe.Pointer(&mi)))
	if ret == 0 {
		fmt.Printf("[MONITOR-DEBUG] getWorkAreaAtPoint: GetMonitorInfoW failed for (%d,%d), using fallback\n", x, y)
		// Fallback to default values
		return WorkArea{X: 0, Y: 0, Width: 1920, Height: 1080}
	}

	wa := WorkArea{
		X:      int(mi.RcWork.Left),
		Y:      int(mi.RcWork.Top),
		Width:  int(mi.RcWork.Right - mi.RcWork.Left),
		Height: int(mi.RcWork.Bottom - mi.RcWork.Top),
	}
	fmt.Printf("[MONITOR-DEBUG] getWorkAreaAtPoint(%d,%d) → workArea=(%d,%d,%d,%d)\n", x, y, wa.X, wa.Y, wa.Width, wa.Height)
	return wa
}

// clampToWorkArea ensures the popup position (in physical pixels) is within the
// work area bounds. All coordinates and dimensions are in physical (virtual-screen)
// pixels. MonitorFromPoint receives physical coords and returns physical work area,
// so the clamp is performed entirely in a single coordinate system.
//
// Parameters:
//   - popX, popY: proposed popup position in physical pixels
//   - popWidth, popHeight: popup dimensions in physical pixels
//   - mouseX, mouseY: mouse position in physical pixels (for MonitorFromPoint)
//
// Returns adjusted x, y in physical pixels.
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
		popY = mouseY + 20
	}
	if popY+popHeight > wa.Y+wa.Height {
		popY = wa.Y + wa.Height - popHeight
	}

	return popX, popY
}
