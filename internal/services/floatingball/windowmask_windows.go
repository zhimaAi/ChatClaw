//go:build windows

package floatingball

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"math"
	"sync"
)

var (
	maskOnce sync.Once
	maskPNG  []byte
)

// floatingBallWindowMask returns a 64x64 circular alpha mask PNG.
// This makes the window appear as a rounded "ball" on Windows.
func floatingBallWindowMask() []byte {
	maskOnce.Do(func() {
		const w, h = ballSize, ballSize
		img := image.NewRGBA(image.Rect(0, 0, w, h))
		cx, cy := float64(w-1)/2, float64(h-1)/2
		r := float64(minInt(w, h)) / 2
		r2 := r * r

		white := color.RGBA{R: 255, G: 255, B: 255, A: 255}
		transparent := color.RGBA{R: 0, G: 0, B: 0, A: 0}

		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				dx := float64(x) - cx
				dy := float64(y) - cy
				if dx*dx+dy*dy <= r2 {
					img.SetRGBA(x, y, white)
				} else {
					img.SetRGBA(x, y, transparent)
				}
			}
		}

		var buf bytes.Buffer
		_ = png.Encode(&buf, img)
		maskPNG = buf.Bytes()
	})
	return maskPNG
}

func minInt(a, b int) int {
	return int(math.Min(float64(a), float64(b)))
}

