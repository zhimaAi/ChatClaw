//go:build !windows

package winsnap

import "github.com/wailsapp/wails/v3/pkg/application"

// IsWindowMinimized is not supported on non-Windows platforms.
func IsWindowMinimized(_ *application.WebviewWindow) (bool, error) {
	return false, ErrNotSupported
}
