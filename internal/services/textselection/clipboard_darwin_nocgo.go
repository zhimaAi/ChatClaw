//go:build darwin && !cgo

package textselection

// getDPIScale fallback for darwin when CGO is disabled.
func getDPIScale() float64 { return 1.0 }
