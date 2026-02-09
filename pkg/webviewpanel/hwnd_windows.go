//go:build windows

package webviewpanel

import (
	"fmt"
	"sync"
	"syscall"
	"unsafe"
)

var (
	procFindWindowW   = user32.NewProc("FindWindowW")
	procEnumWindows   = user32.NewProc("EnumWindows")
	procGetWindowText = user32.NewProc("GetWindowTextW")

	procEnumChildWindows = user32.NewProc("EnumChildWindows")
	procGetClassNameW    = user32.NewProc("GetClassNameW")
)

// FindWindowByTitle finds a window by its title.
// Returns the window handle (HWND) or 0 if not found.
func FindWindowByTitle(title string) uintptr {
	titlePtr, _ := syscall.UTF16PtrFromString(title)
	hwnd, _, _ := procFindWindowW.Call(0, uintptr(unsafe.Pointer(titlePtr)))
	return hwnd
}

// ---------- Reusable EnumWindows / EnumChildWindows callbacks ----------
// Created once to avoid exhausting Go's fixed callback-slot table on Windows.

var (
	fwbtCBOnce       sync.Once
	fwbtCB           uintptr
	fwbtMu           sync.Mutex
	fwbtSubstring    string
	fwbtResult       uintptr
)

func fwbtEnumProc(hwnd, lParam uintptr) uintptr {
	title := make([]uint16, 256)
	procGetWindowText.Call(hwnd, uintptr(unsafe.Pointer(&title[0])), 256)
	windowTitle := syscall.UTF16ToString(title)
	if windowTitle != "" && contains(windowTitle, fwbtSubstring) {
		fwbtResult = hwnd
		return 0
	}
	return 1
}

var (
	fcwbcCBOnce      sync.Once
	fcwbcCB          uintptr
	fcwbcMu          sync.Mutex
	fcwbcSubstring   string
	fcwbcBestHwnd    uintptr
	fcwbcBestArea    int64
)

func fcwbcEnumProc(hwnd, lParam uintptr) uintptr {
	class := make([]uint16, 256)
	procGetClassNameW.Call(hwnd, uintptr(unsafe.Pointer(&class[0])), 256)
	className := syscall.UTF16ToString(class)
	if className == "" || !contains(className, fcwbcSubstring) {
		return 1
	}
	var rect RECT
	procGetWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&rect)))
	w := int64(rect.Right - rect.Left)
	h := int64(rect.Bottom - rect.Top)
	area := w * h
	if area > fcwbcBestArea {
		fcwbcBestHwnd = hwnd
		fcwbcBestArea = area
	}
	return 1
}

// FindWindowByTitleContains finds a window whose title contains the given substring.
// Returns the window handle (HWND) or 0 if not found.
func FindWindowByTitleContains(titleSubstring string) uintptr {
	fwbtMu.Lock()
	defer fwbtMu.Unlock()

	fwbtSubstring = titleSubstring
	fwbtResult = 0

	fwbtCBOnce.Do(func() {
		fwbtCB = syscall.NewCallback(fwbtEnumProc)
	})
	procEnumWindows.Call(fwbtCB, 0)
	return fwbtResult
}

// FindChildWindowByClassContains finds a child window whose class name contains the given substring.
// It returns the "largest" matching child (by window rect area) because the WebView host often has
// multiple Chrome_WidgetWin_* children.
func FindChildWindowByClassContains(parentHwnd uintptr, classSubstring string) uintptr {
	if parentHwnd == 0 {
		return 0
	}

	fcwbcMu.Lock()
	defer fcwbcMu.Unlock()

	fcwbcSubstring = classSubstring
	fcwbcBestHwnd = 0
	fcwbcBestArea = 0

	fcwbcCBOnce.Do(func() {
		fcwbcCB = syscall.NewCallback(fcwbcEnumProc)
	})
	procEnumChildWindows.Call(parentHwnd, fcwbcCB, 0)
	return fcwbcBestHwnd
}

// GetClientSizeDIP returns the client width/height in DIP (96 DPI base).
func GetClientSizeDIP(hwnd uintptr) (int, int) {
	if hwnd == 0 {
		return 0, 0
	}
	var rect RECT
	procGetClientRect.Call(hwnd, uintptr(unsafe.Pointer(&rect)))
	w := int(rect.Right - rect.Left)
	h := int(rect.Bottom - rect.Top)

	dpi, _, _ := procGetDpiForWindow.Call(hwnd)
	if dpi == 0 {
		return w, h
	}
	scale := float64(dpi) / 96.0
	return int(float64(w) / scale), int(float64(h) / scale)
}

// ResizeWailsWebviewChrome tries to resize the window's main WebView host to only occupy the top chrome area.
// This is needed so additional WebView2 panels created as child windows can be visible in the remaining area.
func ResizeWailsWebviewChrome(parentHwnd uintptr, chromeHeightDIP int) {
	if parentHwnd == 0 || chromeHeightDIP <= 0 {
		return
	}

	// Find the WebView host child. In WebView2 it is typically Chrome_WidgetWin_*.
	host := FindChildWindowByClassContains(parentHwnd, "Chrome_WidgetWin")
	if host == 0 {
		// Not fatal; just log to help debugging.
		fmt.Printf("[webviewpanel] could not find webview host child under hwnd=%d\n", parentHwnd)
		return
	}

	// Get parent client size in physical pixels
	var rect RECT
	procGetClientRect.Call(parentHwnd, uintptr(unsafe.Pointer(&rect)))
	clientW := int(rect.Right - rect.Left)

	// Convert chrome height DIP -> physical pixels
	dpi, _, _ := procGetDpiForWindow.Call(parentHwnd)
	if dpi == 0 {
		dpi = 96
	}
	scale := float64(dpi) / 96.0
	chromeH := int(float64(chromeHeightDIP) * scale)

	// Resize/move host to top area
	procSetWindowPos.Call(
		host,
		0,
		0,
		0,
		uintptr(clientW),
		uintptr(chromeH),
		SWP_NOZORDER|SWP_NOACTIVATE,
	)
}

func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
