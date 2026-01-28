//go:build !windows && !darwin && !linux

package webviewpanel

// FindWindowByTitle finds a window by its title.
// This is a stub for non-Windows platforms.
func FindWindowByTitle(title string) uintptr {
	return 0
}

// FindWindowByTitleContains finds a window whose title contains the given substring.
// This is a stub for non-Windows platforms.
func FindWindowByTitleContains(titleSubstring string) uintptr {
	return 0
}

// FindChildWindowByClassContains is a stub for non-Windows platforms.
func FindChildWindowByClassContains(parentHwnd uintptr, classSubstring string) uintptr {
	return 0
}

// GetClientSizeDIP is a stub for non-Windows platforms.
func GetClientSizeDIP(hwnd uintptr) (int, int) {
	return 0, 0
}

// ResizeWailsWebviewChrome is a stub for non-Windows platforms.
func ResizeWailsWebviewChrome(parentHwnd uintptr, chromeHeightDIP int) {
	// no-op
}
