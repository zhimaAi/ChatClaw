//go:build windows

package textselection

import (
	"sync"
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

	procGetSysColor = modUser32.NewProc("GetSysColor")
)

const (
	srccopy = 0x00CC0020
	biRGB   = 0

	// COLOR_HIGHLIGHT: system color index for text/item selection background.
	colorHighlightID = 13
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

// ---------- helpers ----------

func absInt(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// colorNear returns true if two RGB colors are within the given per-channel tolerance.
func colorNear(r1, g1, b1, r2, g2, b2 byte, tolerance int) bool {
	return absInt(int(r1)-int(r2)) <= tolerance &&
		absInt(int(g1)-int(g2)) <= tolerance &&
		absInt(int(b1)-int(b2)) <= tolerance
}

// ---------- System highlight color ----------

var (
	sysHighlightR    byte
	sysHighlightG    byte
	sysHighlightB    byte
	sysHighlightOnce sync.Once
)

// getSystemHighlightRGB returns the system text selection highlight color.
func getSystemHighlightRGB() (r, g, b byte) {
	sysHighlightOnce.Do(func() {
		ret, _, _ := procGetSysColor.Call(uintptr(colorHighlightID))
		// COLORREF layout: 0x00BBGGRR
		sysHighlightR = byte(ret & 0xFF)
		sysHighlightG = byte((ret >> 8) & 0xFF)
		sysHighlightB = byte((ret >> 16) & 0xFF)
	})
	return sysHighlightR, sysHighlightG, sysHighlightB
}

// ---------- Screen capture ----------

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

	screenDC, _, _ := procGetDC.Call(0)
	if screenDC == 0 {
		return nil
	}
	defer procReleaseDC.Call(0, screenDC)

	memDC, _, _ := procCreateCompatibleDC.Call(screenDC)
	if memDC == 0 {
		return nil
	}
	defer procDeleteDC.Call(memDC)

	bitmap, _, _ := procCreateCompatibleBitmap.Call(screenDC, uintptr(width), uintptr(height))
	if bitmap == 0 {
		return nil
	}
	defer procDeleteObject.Call(bitmap)

	procSelectObject.Call(memDC, bitmap)

	ret, _, _ := procBitBlt.Call(
		memDC, 0, 0, uintptr(width), uintptr(height),
		screenDC, uintptr(x), uintptr(y),
		srccopy,
	)
	if ret == 0 {
		return nil
	}

	bi := bitmapInfo{
		BmiHeader: bitmapInfoHeader{
			BiSize:        uint32(unsafe.Sizeof(bitmapInfoHeader{})),
			BiWidth:       width,
			BiHeight:      -height, // top-down
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
		0,
	)
	if ret == 0 {
		return nil
	}

	return pixels
}

// ---------- Detection method 1: System highlight color ----------

// hasSelectionHighlight checks whether the "after" screenshot shows NEW pixels
// matching the system text selection highlight color that were NOT present in "before".
// Catches standard Windows apps that use the system highlight color.
func hasSelectionHighlight(before, after []byte, minRatio float64) bool {
	if len(before) == 0 || len(after) == 0 || len(before) != len(after) {
		return false
	}

	hR, hG, hB := getSystemHighlightRGB()
	totalPixels := len(before) / 4
	newHighlightCount := 0
	const tol = 60

	for i := 0; i < len(before)-3; i += 4 {
		aR, aG, aB := after[i+2], after[i+1], after[i]
		bR, bG, bB := before[i+2], before[i+1], before[i]

		if colorNear(aR, aG, aB, hR, hG, hB, tol) && !colorNear(bR, bG, bB, hR, hG, hB, tol) {
			newHighlightCount++
		}
	}

	return float64(newHighlightCount)/float64(totalPixels) >= minRatio
}

// ---------- Detection method 2: Uniform chromatic change ----------

// hasUniformChromaticChange detects text selection by checking whether the
// changed pixels cluster around a single CHROMATIC (non-gray) color.
//
// Algorithm:
//  1. Find all pixels that changed significantly between before/after.
//  2. Build a color histogram of the "after" values of changed pixels
//     (quantized to 6 levels per channel = 216 buckets).
//  3. Find the most popular bucket (dominant color).
//  4. Reject if the dominant color is achromatic (gray/white/black),
//     since window movement, screenshot overlays, and scrolling produce
//     gray-ish changes, NOT colored highlight bands.
//  5. Accept if the dominant bucket holds ≥ 35% of all changed pixels
//     (indicates a uniform color, like a text selection highlight).
//
// This catches custom highlight colors (DingTalk light-blue, VS Code dark-blue, etc.)
// that do not match the system GetSysColor(COLOR_HIGHLIGHT).
func hasUniformChromaticChange(before, after []byte, minChangedRatio float64) bool {
	if len(before) == 0 || len(after) == 0 || len(before) != len(after) {
		return false
	}

	totalPixels := len(before) / 4
	const pixelDiff = 25 // per-channel diff to count pixel as "changed"

	// Quantize to 6 levels per channel → 216 buckets.
	// Each bucket spans ~42 values per channel, providing implicit ±21 tolerance.
	const levels = 6
	const totalBuckets = levels * levels * levels

	var histogram [totalBuckets]int
	// Track RGB sums per bucket to compute the dominant color later.
	type rgbSum struct{ r, g, b int64 }
	var bucketSums [totalBuckets]rgbSum
	changedCount := 0

	for i := 0; i < len(before)-3; i += 4 {
		db := absInt(int(before[i]) - int(after[i]))
		dg := absInt(int(before[i+1]) - int(after[i+1]))
		dr := absInt(int(before[i+2]) - int(after[i+2]))

		if db > pixelDiff || dg > pixelDiff || dr > pixelDiff {
			changedCount++
			r, g, b := after[i+2], after[i+1], after[i] // BGRA → R, G, B
			ri := int(r) * levels / 256
			gi := int(g) * levels / 256
			bi := int(b) * levels / 256
			// Clamp to valid range (255 * 6 / 256 = 5, but be safe)
			if ri >= levels {
				ri = levels - 1
			}
			if gi >= levels {
				gi = levels - 1
			}
			if bi >= levels {
				bi = levels - 1
			}
			bucket := ri*levels*levels + gi*levels + bi
			histogram[bucket]++
			bucketSums[bucket].r += int64(r)
			bucketSums[bucket].g += int64(g)
			bucketSums[bucket].b += int64(b)
		}
	}

	// Gate 1: need a minimum percentage of pixels to have changed.
	if float64(changedCount)/float64(totalPixels) < minChangedRatio {
		return false
	}

	// Find the most popular bucket.
	maxBucket := 0
	maxCount := 0
	for i, c := range histogram {
		if c > maxCount {
			maxCount = c
			maxBucket = i
		}
	}
	if maxCount == 0 {
		return false
	}

	// Gate 2: dominant color must be CHROMATIC (has hue / saturation).
	// Gray, white, and black are achromatic → max(R,G,B) - min(R,G,B) is small.
	// Text selection highlights always have color (blue, teal, purple, etc.).
	// Window movement exposes gray toolbars / white backgrounds / dark overlays → rejected.
	n := int64(maxCount)
	avgR := byte(bucketSums[maxBucket].r / n)
	avgG := byte(bucketSums[maxBucket].g / n)
	avgB := byte(bucketSums[maxBucket].b / n)

	maxCh := avgR
	if avgG > maxCh {
		maxCh = avgG
	}
	if avgB > maxCh {
		maxCh = avgB
	}
	minCh := avgR
	if avgG < minCh {
		minCh = avgG
	}
	if avgB < minCh {
		minCh = avgB
	}
	if int(maxCh)-int(minCh) < 20 {
		// Dominant color is gray / near-gray → not a selection highlight.
		return false
	}

	// Gate 3: dominant bucket must represent a significant share of changed pixels.
	// Text selection: highlight background pixels cluster in one bucket → 40–70%.
	// Window movement: varied colors scatter across many buckets → < 25%.
	modeRatio := float64(maxCount) / float64(changedCount)
	return modeRatio >= 0.35
}
