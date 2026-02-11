//go:build windows

package textselection

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	modGdi32 = windows.NewLazySystemDLL("gdi32.dll")

	procCreateCompatibleDC     = modGdi32.NewProc("CreateCompatibleDC")
	procCreateCompatibleBitmap = modGdi32.NewProc("CreateCompatibleBitmap")
	procSelectObject           = modGdi32.NewProc("SelectObject")
	procDeleteDC               = modGdi32.NewProc("DeleteDC")
	procDeleteObject           = modGdi32.NewProc("DeleteObject")
	procBitBlt                 = modGdi32.NewProc("BitBlt")
	procGetDIBits              = modGdi32.NewProc("GetDIBits")
)

const (
	srccopy = 0x00CC0020
	biRGB   = 0
)

type bitmapInfoHeader struct {
	BiSize          uint32
	BiWidth         int32
	BiHeight        int32
	BiPlanes        uint16
	BiBitCount      uint16
	BiCompression   uint32
	BiSizeImage     uint32
	BiXPelsPerMeter int32
	BiYPelsPerMeter int32
	BiClrUsed       uint32
	BiClrImportant  uint32
}

type bitmapInfo struct {
	BmiHeader bitmapInfoHeader
	BmiColors [1]uint32
}

// captureScreenPixels captures a rectangular region of the screen as raw BGRA pixel data.
// x, y are physical screen coordinates (top-left of the capture region).
// Returns nil on failure. The returned slice has length width*height*4 (BGRA).
//
// NOTE: BitBlt from the screen DC does NOT include the mouse cursor,
// so cursor movement between captures does not cause false pixel differences.
func captureScreenPixels(x, y, width, height int32) []byte {
	if width <= 0 || height <= 0 {
		return nil
	}

	// Get screen device context
	screenDC, _, _ := procGetDC.Call(0)
	if screenDC == 0 {
		return nil
	}
	defer procReleaseDC.Call(0, screenDC)

	// Create memory DC compatible with screen
	memDC, _, _ := procCreateCompatibleDC.Call(screenDC)
	if memDC == 0 {
		return nil
	}
	defer procDeleteDC.Call(memDC)

	// Create bitmap compatible with screen
	bitmap, _, _ := procCreateCompatibleBitmap.Call(screenDC, uintptr(width), uintptr(height))
	if bitmap == 0 {
		return nil
	}
	defer procDeleteObject.Call(bitmap)

	// Select bitmap into memory DC
	procSelectObject.Call(memDC, bitmap)

	// Copy screen pixels to memory DC
	ret, _, _ := procBitBlt.Call(
		memDC, 0, 0, uintptr(width), uintptr(height),
		screenDC, uintptr(x), uintptr(y),
		srccopy,
	)
	if ret == 0 {
		return nil
	}

	// Read pixel data from bitmap
	bi := bitmapInfo{
		BmiHeader: bitmapInfoHeader{
			BiSize:        uint32(unsafe.Sizeof(bitmapInfoHeader{})),
			BiWidth:       width,
			BiHeight:      -height, // negative = top-down DIB
			BiPlanes:      1,
			BiBitCount:    32,
			BiCompression: biRGB,
		},
	}

	pixels := make([]byte, int(width)*int(height)*4)
	ret, _, _ = procGetDIBits.Call(
		memDC, bitmap, 0, uintptr(height),
		uintptr(unsafe.Pointer(&pixels[0])),
		uintptr(unsafe.Pointer(&bi)),
		0, // DIB_RGB_COLORS
	)
	if ret == 0 {
		return nil
	}

	return pixels
}

// hasSignificantPixelChange compares two BGRA pixel buffers and returns true
// if the fraction of visually changed pixels exceeds the given threshold.
//
// threshold is a ratio in [0, 1], e.g. 0.05 means 5% of pixels must differ.
// A per-channel difference > colorDiff counts a pixel as "changed".
//
// This is used to detect text selection highlights: when text is selected,
// the OS draws a colored highlight band that changes many pixels at once.
// Dragging on non-text areas (desktop, images) causes no pixel change.
func hasSignificantPixelChange(before, after []byte, threshold float64) bool {
	if len(before) == 0 || len(after) == 0 || len(before) != len(after) {
		return false
	}

	totalPixels := len(before) / 4
	changedPixels := 0
	const colorDiff = 25 // minimum per-channel difference to count as changed

	for i := 0; i < len(before)-3; i += 4 {
		// BGRA layout: [B, G, R, A]
		db := int(before[i]) - int(after[i])
		dg := int(before[i+1]) - int(after[i+1])
		dr := int(before[i+2]) - int(after[i+2])

		if db < 0 {
			db = -db
		}
		if dg < 0 {
			dg = -dg
		}
		if dr < 0 {
			dr = -dr
		}

		if db > colorDiff || dg > colorDiff || dr > colorDiff {
			changedPixels++
		}
	}

	ratio := float64(changedPixels) / float64(totalPixels)
	return ratio >= threshold
}
