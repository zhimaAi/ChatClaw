//go:build windows

package winsnap

import (
	"errors"
	"fmt"
	"time"

	"golang.org/x/sys/windows"
)

// SyncRightOfProcessNow force-syncs the given window to the right side of the target process window.
//
// This is a best-effort "re-adhesion" used when the winsnap window is restored (e.g. after Win+D)
// but no target move/foreground event is fired yet, which could otherwise leave winsnap far away.
//
// It does NOT activate or steal focus.
func SyncRightOfProcessNow(opts AttachOptions) error {
	if opts.Window == nil {
		return errors.New("winsnap: Window is nil")
	}

	selfHWND := uintptr(opts.Window.NativeWindow())
	if selfHWND == 0 {
		return errors.New("winsnap: native window handle is 0")
	}

	targetNames := expandWindowsTargetNames(opts.TargetProcessName)
	if len(targetNames) == 0 {
		return errors.New("winsnap: TargetProcessName is empty")
	}

	// Keep a short retry window because restore/show events may race with the target HWND enumeration.
	deadline := time.Now().Add(800 * time.Millisecond)
	var target windows.HWND
	for {
		for _, name := range targetNames {
			h, err := findMainWindowByProcessName(name)
			if err == nil && h != 0 {
				target = h
				break
			}
		}
		if target != 0 {
			break
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("%w: %s", ErrTargetWindowNotFound, opts.TargetProcessName)
		}
		time.Sleep(60 * time.Millisecond)
	}

	gap := opts.Gap
	var targetWin, targetFrame rect
	if err := getWindowRect(target, &targetWin); err != nil {
		return err
	}
	if err := getExtendedFrameBounds(target, &targetFrame); err != nil {
		// Fallback when DWM call fails.
		targetFrame = targetWin
	}

	self := windows.HWND(selfHWND)
	var selfWin, selfFrame rect
	if err := getWindowRect(self, &selfWin); err != nil {
		return err
	}
	if err := getExtendedFrameBounds(self, &selfFrame); err != nil {
		selfFrame = selfWin
	}

	// Align *visible* frame edges, not the raw window rect edges.
	selfOffsetX := selfFrame.Left - selfWin.Left
	selfOffsetY := selfFrame.Top - selfWin.Top

	x := targetFrame.Right + int32(gap) - selfOffsetX
	y := targetFrame.Top - selfOffsetY

	targetHeight := targetFrame.Bottom - targetFrame.Top
	width := selfWin.Right - selfWin.Left
	if width <= 0 {
		// If width is invalid (rare on restore), just keep current size by not resizing.
		return setWindowPosNoSizeNoZ(self, x, y)
	}

	// Keep same z-order group as target without activation.
	if isTopMost(target) {
		_ = setWindowTopMostNoActivate(self)
	} else {
		_ = setWindowNoTopMostNoActivate(self)
	}
	return setWindowPosWithSizeAfter(self, target, x, y, width, targetHeight)
}
