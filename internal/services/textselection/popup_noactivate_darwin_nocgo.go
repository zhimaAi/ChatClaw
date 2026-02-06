//go:build darwin && !cgo

package textselection

import "github.com/wailsapp/wails/v3/pkg/application"

// tryConfigurePopupNoActivate is a no-op without CGO on macOS.
func tryConfigurePopupNoActivate(_ *application.WebviewWindow) {}

// removePopupSubclass is a no-op on macOS.
func removePopupSubclass(_ uintptr) {}
