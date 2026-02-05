//go:build windows

package winsnap

import (
	"errors"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	procOpenClipboard                = modUser32.NewProc("OpenClipboard")
	procCloseClipboard               = modUser32.NewProc("CloseClipboard")
	procEmptyClipboard               = modUser32.NewProc("EmptyClipboard")
	procSetClipboardData             = modUser32.NewProc("SetClipboardData")
	procGlobalAlloc                  = modKernel32.NewProc("GlobalAlloc")
	procGlobalLock                   = modKernel32.NewProc("GlobalLock")
	procGlobalUnlock                 = modKernel32.NewProc("GlobalUnlock")
	procKeybd_event                  = modUser32.NewProc("keybd_event")
	procSetForegroundWindowInput     = modUser32.NewProc("SetForegroundWindow")
	procGetWindowThreadProcIdInput   = modUser32.NewProc("GetWindowThreadProcessId")
	procAttachThreadInputInput       = modUser32.NewProc("AttachThreadInput")
	procGetCurrentThreadIdInput      = modKernel32.NewProc("GetCurrentThreadId")
	procShowWindowInput              = modUser32.NewProc("ShowWindow")
	procBringWindowToTopInput        = modUser32.NewProc("BringWindowToTop")
	procGetForegroundWindowInput     = modUser32.NewProc("GetForegroundWindow")
)

const (
	CF_UNICODETEXT        = 13
	GMEM_MOVEABLE         = 0x0002
	KEYEVENTF_KEYUP_INPUT = 0x0002
	VK_CONTROL_INPUT      = 0x11
	VK_RETURN_INPUT       = 0x0D
	VK_V_INPUT            = 0x56
	SW_RESTORE_INPUT      = 9
)

// SendTextToTarget sends text to the target application by:
// 1. Copying text to clipboard
// 2. Activating target window (using same method as wake)
// 3. Simulating Ctrl+V to paste
// 4. Optionally simulating Enter or Ctrl+Enter to send
func SendTextToTarget(targetProcess string, text string, triggerSend bool, sendKeyStrategy string) error {
	if targetProcess == "" {
		return errors.New("winsnap: target process is empty")
	}
	if text == "" {
		return errors.New("winsnap: text is empty")
	}

	// Find target window
	targetNames := expandWindowsTargetNames(targetProcess)
	if len(targetNames) == 0 {
		return errors.New("winsnap: invalid target process name")
	}

	var targetHWND windows.HWND
	for _, name := range targetNames {
		h, err := findMainWindowByProcessName(name)
		if err == nil && h != 0 {
			targetHWND = h
			break
		}
	}
	if targetHWND == 0 {
		return ErrTargetWindowNotFound
	}

	// Copy text to clipboard first
	if err := setClipboardText(text); err != nil {
		return err
	}

	// Activate target window - use the same proven method as activateHwnd in wake_windows.go
	activateTargetWindow(targetHWND)
	time.Sleep(150 * time.Millisecond)

	// Simulate Ctrl+V to paste
	simulateCtrlV()
	time.Sleep(80 * time.Millisecond)

	// Optionally trigger send
	if triggerSend {
		time.Sleep(100 * time.Millisecond)
		if sendKeyStrategy == "ctrl_enter" {
			simulateCtrlEnter()
		} else {
			simulateEnter()
		}
	}

	return nil
}

// PasteTextToTarget sends text to the target application's edit box without triggering send.
func PasteTextToTarget(targetProcess string, text string) error {
	return SendTextToTarget(targetProcess, text, false, "")
}

// activateTargetWindow brings the target window to front and gives it focus
// This uses the same approach as activateHwnd in wake_windows.go which is proven to work
func activateTargetWindow(hwnd windows.HWND) {
	if hwnd == 0 {
		return
	}

	foregroundHwnd, _, _ := procGetForegroundWindowInput.Call()
	var attached bool
	var foregroundTid, currentTid uintptr

	if foregroundHwnd != 0 {
		var foregroundPid uint32
		foregroundTid, _, _ = procGetWindowThreadProcIdInput.Call(
			foregroundHwnd,
			uintptr(unsafe.Pointer(&foregroundPid)),
		)
		currentTid, _, _ = procGetCurrentThreadIdInput.Call()
		if foregroundTid != currentTid {
			ret, _, _ := procAttachThreadInputInput.Call(currentTid, foregroundTid, 1)
			attached = ret != 0
		}
	}

	// ShowWindow with SW_RESTORE to restore if minimized
	procShowWindowInput.Call(uintptr(hwnd), SW_RESTORE_INPUT)
	// SetForegroundWindow to bring to front
	procSetForegroundWindowInput.Call(uintptr(hwnd))
	// BringWindowToTop for good measure
	procBringWindowToTopInput.Call(uintptr(hwnd))

	if attached {
		procAttachThreadInputInput.Call(currentTid, foregroundTid, 0)
	}
}

func setClipboardText(text string) error {
	// Convert to UTF-16
	utf16, err := syscall.UTF16FromString(text)
	if err != nil {
		return err
	}

	// Open clipboard with retry
	var ret uintptr
	for i := 0; i < 10; i++ {
		ret, _, _ = procOpenClipboard.Call(0)
		if ret != 0 {
			break
		}
		time.Sleep(30 * time.Millisecond)
	}
	if ret == 0 {
		return errors.New("winsnap: failed to open clipboard")
	}
	defer procCloseClipboard.Call()

	// Empty clipboard
	procEmptyClipboard.Call()

	// Allocate global memory
	size := len(utf16) * 2
	hMem, _, _ := procGlobalAlloc.Call(GMEM_MOVEABLE, uintptr(size))
	if hMem == 0 {
		return errors.New("winsnap: failed to allocate memory")
	}

	// Lock and copy data
	ptr, _, _ := procGlobalLock.Call(hMem)
	if ptr == 0 {
		return errors.New("winsnap: failed to lock memory")
	}

	// Copy UTF-16 data
	dst := unsafe.Slice((*uint16)(unsafe.Pointer(ptr)), len(utf16))
	copy(dst, utf16)

	procGlobalUnlock.Call(hMem)

	// Set clipboard data
	ret, _, _ = procSetClipboardData.Call(CF_UNICODETEXT, hMem)
	if ret == 0 {
		return errors.New("winsnap: failed to set clipboard data")
	}

	return nil
}

func simulateCtrlV() {
	// Press Ctrl
	procKeybd_event.Call(VK_CONTROL_INPUT, 0, 0, 0)
	time.Sleep(30 * time.Millisecond)

	// Press V
	procKeybd_event.Call(VK_V_INPUT, 0, 0, 0)
	time.Sleep(30 * time.Millisecond)

	// Release V
	procKeybd_event.Call(VK_V_INPUT, 0, KEYEVENTF_KEYUP_INPUT, 0)
	time.Sleep(30 * time.Millisecond)

	// Release Ctrl
	procKeybd_event.Call(VK_CONTROL_INPUT, 0, KEYEVENTF_KEYUP_INPUT, 0)
}

func simulateEnter() {
	// Press Enter
	procKeybd_event.Call(VK_RETURN_INPUT, 0, 0, 0)
	time.Sleep(30 * time.Millisecond)

	// Release Enter
	procKeybd_event.Call(VK_RETURN_INPUT, 0, KEYEVENTF_KEYUP_INPUT, 0)
}

func simulateCtrlEnter() {
	// Press Ctrl
	procKeybd_event.Call(VK_CONTROL_INPUT, 0, 0, 0)
	time.Sleep(30 * time.Millisecond)

	// Press Enter
	procKeybd_event.Call(VK_RETURN_INPUT, 0, 0, 0)
	time.Sleep(30 * time.Millisecond)

	// Release Enter
	procKeybd_event.Call(VK_RETURN_INPUT, 0, KEYEVENTF_KEYUP_INPUT, 0)
	time.Sleep(30 * time.Millisecond)

	// Release Ctrl
	procKeybd_event.Call(VK_CONTROL_INPUT, 0, KEYEVENTF_KEYUP_INPUT, 0)
}
