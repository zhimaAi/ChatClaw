//go:build !windows && !darwin

package textselection

import "github.com/wailsapp/wails/v3/pkg/application"

func tryConfigurePopupNoActivate(_ *application.WebviewWindow) {}

func removePopupSubclass(_ uintptr) {}
