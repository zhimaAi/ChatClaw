//go:build windows

package winsnap

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// #region agent log
const debugLogPath = `c:\work\GoPro\willchat-client\.cursor\debug.log`

func debugLog(hypothesisId, location, message string, data map[string]interface{}) {
	entry := map[string]interface{}{
		"timestamp":    time.Now().UnixMilli(),
		"sessionId":    "debug-session",
		"hypothesisId": hypothesisId,
		"location":     location,
		"message":      message,
		"data":         data,
	}
	jsonBytes, _ := json.Marshal(entry)
	f, err := os.OpenFile(debugLogPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err == nil {
		f.WriteString(string(jsonBytes) + "\n")
		f.Close()
	}
}

// #endregion

var (
	procOpenClipboard     = modUser32.NewProc("OpenClipboard")
	procCloseClipboard    = modUser32.NewProc("CloseClipboard")
	procEmptyClipboard    = modUser32.NewProc("EmptyClipboard")
	procSetClipboardData  = modUser32.NewProc("SetClipboardData")
	procGlobalAlloc       = modKernel32.NewProc("GlobalAlloc")
	procGlobalLock        = modKernel32.NewProc("GlobalLock")
	procGlobalUnlock      = modKernel32.NewProc("GlobalUnlock")
	procSendInput         = modUser32.NewProc("SendInput")
	procMapVirtualKeyW    = modUser32.NewProc("MapVirtualKeyW")
	procGetFocusIn        = modUser32.NewProc("GetFocus")
	procKeybdEvent        = modUser32.NewProc("keybd_event")
	procGetClassNameW     = modUser32.NewProc("GetClassNameW")
	procGetWindowTextWIn  = modUser32.NewProc("GetWindowTextW")
	procGetGUIThreadInfo  = modUser32.NewProc("GetGUIThreadInfo")
	procEnumChildWindows  = modUser32.NewProc("EnumChildWindows")
	procSetFocusIn        = modUser32.NewProc("SetFocus")
	procSendMessageW      = modUser32.NewProc("SendMessageW")
	procPostMessageW      = modUser32.NewProc("PostMessageW")
)

const (
	CF_UNICODETEXT = 13
	GMEM_MOVEABLE  = 0x0002

	// SendInput constants
	INPUT_KEYBOARD        = 1
	KEYEVENTF_KEYUP       = 0x0002
	KEYEVENTF_SCANCODE    = 0x0008
	KEYEVENTF_EXTENDEDKEY = 0x0001
	VK_CONTROL            = 0x11
	VK_RETURN             = 0x0D
	VK_V                  = 0x56

	// MapVirtualKey mapping type
	MAPVK_VK_TO_VSC = 0
)

// KEYBDINPUT structure for SendInput - matches Windows KEYBDINPUT exactly
type keyboardInput struct {
	wVk         uint16
	wScan       uint16
	dwFlags     uint32
	time        uint32
	dwExtraInfo uintptr
}

// INPUT structure for SendInput - matches Windows INPUT exactly on 64-bit
// Uses the same memory layout as the Windows API
type inputUnion struct {
	inputType uint32
	padding   uint32 // Align to 8-byte boundary
	ki        keyboardInput
	// Padding to make total size 40 bytes (sizeof(INPUT) on 64-bit Windows)
	// keyboardInput is 20 bytes, we need 12 more bytes of padding
	_pad [12]byte
}

// SendTextToTarget sends text to the target application by:
// 1. Copying text to clipboard
// 2. Bringing target window to front (without stealing focus from Wails)
// 3. Simulating Ctrl+V to paste using SendInput
// 4. Optionally simulating Enter or Ctrl+Enter to send
func SendTextToTarget(targetProcess string, text string, triggerSend bool, sendKeyStrategy string) error {
	// #region agent log
	debugLog("A", "input_windows.go:SendTextToTarget:entry", "Function called", map[string]interface{}{
		"targetProcess":   targetProcess,
		"textLen":         len(text),
		"triggerSend":     triggerSend,
		"sendKeyStrategy": sendKeyStrategy,
	})
	// #endregion

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

	// #region agent log
	debugLog("A", "input_windows.go:SendTextToTarget:findWindow", "Searching for window", map[string]interface{}{
		"targetNames": targetNames,
	})
	// #endregion

	var targetHWND windows.HWND
	for _, name := range targetNames {
		h, err := findMainWindowByProcessName(name)
		if err == nil && h != 0 {
			targetHWND = h
			// #region agent log
			debugLog("A", "input_windows.go:SendTextToTarget:foundWindow", "Window found", map[string]interface{}{
				"processName": name,
				"hwnd":        fmt.Sprintf("0x%X", h),
			})
			// #endregion
			break
		}
	}
	if targetHWND == 0 {
		// #region agent log
		debugLog("A", "input_windows.go:SendTextToTarget:error", "Target window NOT found", map[string]interface{}{})
		// #endregion
		return ErrTargetWindowNotFound
	}

	// Copy text to clipboard first
	if err := setClipboardText(text); err != nil {
		// #region agent log
		debugLog("A", "input_windows.go:SendTextToTarget:clipboardError", "Clipboard error", map[string]interface{}{
			"error": err.Error(),
		})
		// #endregion
		return err
	}

	// Get window info before activation
	targetClass := getWindowClassName(uintptr(targetHWND))
	targetTitle := getWindowTitle(uintptr(targetHWND))

	// #region agent log
	debugLog("B", "input_windows.go:SendTextToTarget:beforeActivate", "About to activate window", map[string]interface{}{
		"targetHWND":  fmt.Sprintf("0x%X", targetHWND),
		"targetClass": targetClass,
		"targetTitle": targetTitle,
	})
	// #endregion

	// Use the proven wake method to activate target - same as WakeAttachedWindow
	activateHwndInput(targetHWND)
	time.Sleep(250 * time.Millisecond)

	// Check which window has focus after activation
	foregroundAfter, _, _ := procGetForegroundWindowWake.Call()

	// Get the thread ID of the target window for focus info
	var targetPid uint32
	targetTid, _, _ := procGetWindowThreadProcIdWake.Call(uintptr(targetHWND), uintptr(unsafe.Pointer(&targetPid)))

	// Get detailed focus info
	focusInfo := getFocusedWindowInfo(uint32(targetTid))

	// #region agent log
	debugLog("J", "input_windows.go:SendTextToTarget:afterActivate", "Activation and focus result", map[string]interface{}{
		"foregroundHWND": fmt.Sprintf("0x%X", foregroundAfter),
		"targetHWND":     fmt.Sprintf("0x%X", targetHWND),
		"match":          foregroundAfter == uintptr(targetHWND),
		"targetTid":      targetTid,
		"focusInfo":      focusInfo,
	})
	// #endregion

	// Enumerate child windows to understand the window structure
	childWindows := enumerateChildWindows(uintptr(targetHWND))

	// #region agent log
	childInfos := make([]map[string]interface{}, 0)
	for i, cw := range childWindows {
		if i < 20 { // Limit to first 20 children
			childInfos = append(childInfos, map[string]interface{}{
				"hwnd":      fmt.Sprintf("0x%X", cw.hwnd),
				"className": cw.className,
				"title":     cw.title,
			})
		}
	}
	debugLog("J", "input_windows.go:SendTextToTarget:childWindows", "Child windows enumerated", map[string]interface{}{
		"totalCount": len(childWindows),
		"children":   childInfos,
	})
	// #endregion

	// Try sending WM_PASTE directly to the main window first
	// #region agent log
	debugLog("K", "input_windows.go:SendTextToTarget:tryWMPaste", "Trying WM_PASTE to main window", map[string]interface{}{
		"targetHWND": fmt.Sprintf("0x%X", targetHWND),
	})
	// #endregion

	sendWMPaste(uintptr(targetHWND))
	time.Sleep(200 * time.Millisecond)

	// Check if paste worked by NOT sending any keyboard events for now
	// This is to isolate the issue - if DingTalk still hides, it's not the keyboard events

	// #region agent log
	debugLog("K", "input_windows.go:SendTextToTarget:afterWMPaste", "WM_PASTE sent, skipping keyboard events for diagnosis", map[string]interface{}{
		"triggerSend":     triggerSend,
		"skippingKeyboard": true,
	})
	// #endregion

	// TEMPORARILY DISABLED: keyboard events to diagnose if they cause the crash
	// if triggerSend {
	//     time.Sleep(200 * time.Millisecond)
	//     if sendKeyStrategy == "ctrl_enter" {
	//         sendCtrlEnter()
	//     } else {
	//         sendEnter()
	//     }
	// }

	// #region agent log
	debugLog("C", "input_windows.go:SendTextToTarget:completed", "Function completed (keyboard disabled for diagnosis)", map[string]interface{}{})
	// #endregion

	return nil
}

// PasteTextToTarget sends text to the target application's edit box without triggering send.
func PasteTextToTarget(targetProcess string, text string) error {
	return SendTextToTarget(targetProcess, text, false, "")
}

// rect structure for Windows RECT
type rectInput struct {
	left   int32
	top    int32
	right  int32
	bottom int32
}

// GUITHREADINFO structure for GetGUIThreadInfo
type guiThreadInfo struct {
	cbSize        uint32
	flags         uint32
	hwndActive    uintptr
	hwndFocus     uintptr
	hwndCapture   uintptr
	hwndMenuOwner uintptr
	hwndMoveSize  uintptr
	hwndCaret     uintptr
	rcCaret       rectInput
}

// getWindowClassName returns the class name of a window
func getWindowClassName(hwnd uintptr) string {
	buf := make([]uint16, 256)
	procGetClassNameW.Call(hwnd, uintptr(unsafe.Pointer(&buf[0])), 256)
	return syscall.UTF16ToString(buf)
}

// getWindowTitle returns the title of a window
func getWindowTitle(hwnd uintptr) string {
	buf := make([]uint16, 256)
	procGetWindowTextWIn.Call(hwnd, uintptr(unsafe.Pointer(&buf[0])), 256)
	return syscall.UTF16ToString(buf)
}

// getFocusedWindowInfo gets info about which window has keyboard focus
func getFocusedWindowInfo(threadId uint32) map[string]interface{} {
	info := guiThreadInfo{cbSize: uint32(unsafe.Sizeof(guiThreadInfo{}))}
	ret, _, _ := procGetGUIThreadInfo.Call(uintptr(threadId), uintptr(unsafe.Pointer(&info)))

	result := map[string]interface{}{
		"success":     ret != 0,
		"hwndActive":  fmt.Sprintf("0x%X", info.hwndActive),
		"hwndFocus":   fmt.Sprintf("0x%X", info.hwndFocus),
		"hwndCapture": fmt.Sprintf("0x%X", info.hwndCapture),
	}

	if info.hwndFocus != 0 {
		result["focusClass"] = getWindowClassName(info.hwndFocus)
		result["focusTitle"] = getWindowTitle(info.hwndFocus)
	}

	return result
}

// WM_PASTE constant
const WM_PASTE = 0x0302

// childWindowInfo stores info about enumerated child windows
type childWindowInfo struct {
	hwnd      uintptr
	className string
	title     string
}

var enumChildResults []childWindowInfo

// enumChildCallback is called for each child window
func enumChildCallback(hwnd uintptr, lParam uintptr) uintptr {
	className := getWindowClassName(hwnd)
	title := getWindowTitle(hwnd)
	enumChildResults = append(enumChildResults, childWindowInfo{
		hwnd:      hwnd,
		className: className,
		title:     title,
	})
	return 1 // Continue enumeration
}

// enumerateChildWindows returns all child windows of a parent
func enumerateChildWindows(parentHwnd uintptr) []childWindowInfo {
	enumChildResults = nil
	procEnumChildWindows.Call(parentHwnd, syscall.NewCallback(enumChildCallback), 0)
	return enumChildResults
}

// sendWMPaste sends WM_PASTE message to a window
func sendWMPaste(hwnd uintptr) {
	procSendMessageW.Call(hwnd, WM_PASTE, 0, 0)
}

// activateHwndInput activates the target window using the same proven method from wake_windows.go
// This is a copy of activateHwnd to avoid import cycles
func activateHwndInput(hwnd windows.HWND) {
	if hwnd == 0 {
		return
	}

	foregroundHwnd, _, _ := procGetForegroundWindowWake.Call()
	var attached bool
	var foregroundTid, currentTid uintptr

	if foregroundHwnd != 0 {
		var foregroundPid uint32
		foregroundTid, _, _ = procGetWindowThreadProcIdWake.Call(
			foregroundHwnd,
			uintptr(unsafe.Pointer(&foregroundPid)),
		)
		currentTid, _, _ = procGetCurrentThreadIdWake.Call()
		if foregroundTid != currentTid {
			ret, _, _ := procAttachThreadInputWake.Call(currentTid, foregroundTid, 1)
			attached = ret != 0
		}
	}

	procShowWindowWake.Call(uintptr(hwnd), swRestoreWake)
	procSetForegroundWindowWake.Call(uintptr(hwnd))
	procBringWindowToTopWake.Call(uintptr(hwnd))

	if attached {
		procAttachThreadInputWake.Call(currentTid, foregroundTid, 0)
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

// getScanCode returns the scan code for a virtual key code
func getScanCode(vk uint16) uint16 {
	scan, _, _ := procMapVirtualKeyW.Call(uintptr(vk), MAPVK_VK_TO_VSC)
	return uint16(scan)
}

// sendKeyboardInput sends keyboard input using SendInput API
// Returns the number of events successfully sent
func sendKeyboardInput(inputs []inputUnion) uintptr {
	if len(inputs) == 0 {
		return 0
	}
	inputSize := unsafe.Sizeof(inputs[0])

	// #region agent log
	debugLog("H", "input_windows.go:sendKeyboardInput", "SendInput params", map[string]interface{}{
		"inputCount":     len(inputs),
		"inputSize":      inputSize,
		"expectedSize":   40, // sizeof(INPUT) on 64-bit Windows should be 40 bytes
		"firstInputType": inputs[0].inputType,
		"firstVK":        inputs[0].ki.wVk,
		"firstScan":      inputs[0].ki.wScan,
		"firstFlags":     inputs[0].ki.dwFlags,
	})
	// #endregion
	ret, _, lastErr := procSendInput.Call(
		uintptr(len(inputs)),
		uintptr(unsafe.Pointer(&inputs[0])),
		inputSize,
	)
	// #region agent log
	debugLog("H", "input_windows.go:sendKeyboardInput:result", "SendInput returned", map[string]interface{}{
		"ret":     ret,
		"lastErr": fmt.Sprintf("%v", lastErr),
	})
	// #endregion
	return ret
}

// makeKeyDown creates a key down input with both virtual key and scan code
func makeKeyDown(vk uint16) inputUnion {
	scan := getScanCode(vk)
	return inputUnion{
		inputType: INPUT_KEYBOARD,
		ki: keyboardInput{
			wVk:     vk,
			wScan:   scan,
			dwFlags: 0,
		},
	}
}

// makeKeyUp creates a key up input with both virtual key and scan code
func makeKeyUp(vk uint16) inputUnion {
	scan := getScanCode(vk)
	return inputUnion{
		inputType: INPUT_KEYBOARD,
		ki: keyboardInput{
			wVk:     vk,
			wScan:   scan,
			dwFlags: KEYEVENTF_KEYUP,
		},
	}
}

// keybd_event constants
const (
	KEYEVENTF_KEYUP_KBD = 0x0002
)

// keybdEventKeyDown sends a key down event using keybd_event API
func keybdEventKeyDown(vk uint16) {
	scan := getScanCode(vk)
	procKeybdEvent.Call(uintptr(vk), uintptr(scan), 0, 0)
}

// keybdEventKeyUp sends a key up event using keybd_event API
func keybdEventKeyUp(vk uint16) {
	scan := getScanCode(vk)
	procKeybdEvent.Call(uintptr(vk), uintptr(scan), KEYEVENTF_KEYUP_KBD, 0)
}

func sendCtrlV() uintptr {
	// #region agent log
	debugLog("I", "input_windows.go:sendCtrlV", "Using keybd_event API", map[string]interface{}{})
	// #endregion

	// Use keybd_event API - more compatible with some applications like DingTalk
	keybdEventKeyDown(VK_CONTROL)
	time.Sleep(30 * time.Millisecond)

	keybdEventKeyDown(VK_V)
	time.Sleep(30 * time.Millisecond)

	keybdEventKeyUp(VK_V)
	time.Sleep(30 * time.Millisecond)

	keybdEventKeyUp(VK_CONTROL)

	// #region agent log
	debugLog("I", "input_windows.go:sendCtrlV:done", "keybd_event completed", map[string]interface{}{})
	// #endregion

	return 4
}

func sendEnter() uintptr {
	// #region agent log
	debugLog("I", "input_windows.go:sendEnter", "Using keybd_event API", map[string]interface{}{})
	// #endregion

	keybdEventKeyDown(VK_RETURN)
	time.Sleep(30 * time.Millisecond)

	keybdEventKeyUp(VK_RETURN)

	return 2
}

func sendCtrlEnter() uintptr {
	// #region agent log
	debugLog("I", "input_windows.go:sendCtrlEnter", "Using keybd_event API", map[string]interface{}{})
	// #endregion

	keybdEventKeyDown(VK_CONTROL)
	time.Sleep(30 * time.Millisecond)

	keybdEventKeyDown(VK_RETURN)
	time.Sleep(30 * time.Millisecond)

	keybdEventKeyUp(VK_RETURN)
	time.Sleep(30 * time.Millisecond)

	keybdEventKeyUp(VK_CONTROL)

	return 4
}
