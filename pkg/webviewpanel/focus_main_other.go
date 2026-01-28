//go:build !darwin || ios

package webviewpanel

// FocusMainWebview is a no-op on non-macOS platforms (or iOS).
func FocusMainWebview(_ uintptr) {}

