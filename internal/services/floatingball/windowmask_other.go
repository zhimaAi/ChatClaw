//go:build !windows

package floatingball

// Non-windows: no window mask needed.
func floatingBallWindowMask() []byte { return nil }

