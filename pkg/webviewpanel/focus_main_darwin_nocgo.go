//go:build darwin && !ios && !cgo

package webviewpanel

// FocusMainWebview is a no-op when cgo is disabled.
func FocusMainWebview(_ uintptr) {}

