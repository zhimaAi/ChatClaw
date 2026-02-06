//go:build darwin && !cgo

package textselection

import "github.com/wailsapp/wails/v3/pkg/application"

// forcePopupTopMostNoActivate is a no-op without CGO on macOS.
func forcePopupTopMostNoActivate(_ *application.WebviewWindow) {}
