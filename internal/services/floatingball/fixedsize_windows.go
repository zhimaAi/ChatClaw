//go:build windows

package floatingball

// WindowMask is disabled on Windows, so resizing is safe.
func isWindowsFixedSize() bool { return false }

