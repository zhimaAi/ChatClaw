//go:build darwin && !ios && !cgo

package webviewpanel

// Stub (cgo disabled)
func FindWindowByTitle(title string) uintptr { return 0 }
func FindWindowByTitleContains(titleSubstring string) uintptr { return 0 }

