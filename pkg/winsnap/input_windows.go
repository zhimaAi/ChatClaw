//go:build windows

package winsnap

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Reusable callback for getProcessWindowsBoundsForClick.
var (
	gpwbClickCBOnce  sync.Once
	gpwbClickCB      uintptr
	gpwbClickMu      sync.Mutex
	gpwbClickPID     uint32
	gpwbClickBounds  rectInput
	gpwbClickFound   bool
)

func gpwbClickEnumProc(hwnd uintptr, _ uintptr) uintptr {
	h := windows.HWND(hwnd)
	if !isWindowVisible(h) || isWindowIconic(h) {
		return 1
	}
	winPID, err := getWindowProcessID(h)
	if err != nil || winPID != gpwbClickPID {
		return 1
	}
	ex := getExStyle(h)
	if ex&wsExToolWindow != 0 {
		return 1
	}
	var r rect
	if err := getWindowRect(h, &r); err != nil {
		return 1
	}
	if r.Right <= r.Left || r.Bottom <= r.Top {
		return 1
	}
	if r.Left < gpwbClickBounds.left {
		gpwbClickBounds.left = r.Left
	}
	if r.Top < gpwbClickBounds.top {
		gpwbClickBounds.top = r.Top
	}
	if r.Right > gpwbClickBounds.right {
		gpwbClickBounds.right = r.Right
	}
	if r.Bottom > gpwbClickBounds.bottom {
		gpwbClickBounds.bottom = r.Bottom
	}
	gpwbClickFound = true
	return 1
}

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
	procGetClassNameW    = modUser32.NewProc("GetClassNameW")
	procGetWindowTextWIn = modUser32.NewProc("GetWindowTextW")
	procEnumChildWindows = modUser32.NewProc("EnumChildWindows")
	procSetFocusIn        = modUser32.NewProc("SetFocus")
	procSendMessageW      = modUser32.NewProc("SendMessageW")
	procPostMessageW      = modUser32.NewProc("PostMessageW")
	procGetWindowRectIn   = modUser32.NewProc("GetWindowRect")
	procSetCursorPos      = modUser32.NewProc("SetCursorPos")
	procMouse_event       = modUser32.NewProc("mouse_event")
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
// noClick: if true, skip mouse click (for apps that auto-keep focus on input)
// clickOffsetX: pixels from left edge for input focus (0 = center)
// clickOffsetY: pixels from bottom for input focus (0 = use default based on app)
func SendTextToTarget(targetProcess string, text string, triggerSend bool, sendKeyStrategy string, noClick bool, clickOffsetX, clickOffsetY int) error {
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

	// Use the proven wake method to activate target - same as WakeAttachedWindow
	activateHwndInput(targetHWND)
	time.Sleep(250 * time.Millisecond)

	// Try to find editable child window
	editHwnd := findEditableChild(uintptr(targetHWND))

	// Get window class to detect DingTalk or other Qt apps
	className := getWindowClassName(uintptr(targetHWND))
	isQtApp := strings.Contains(className, "Qt")

	// Detect Chromium/CEF-based apps (e.g., Douyin, modern WeChat).
	// In these apps the actual chat input is rendered inside the web content,
	// NOT as a native Edit control. Even if findEditableChild returns a native
	// Edit handle (e.g., a search bar or title-bar input), it is NOT the chat
	// input box. We must click to focus the web-rendered input area.
	chatChildHwnd := findChatChildWindow(uintptr(targetHWND))
	isChromiumApp := chatChildHwnd != 0

	// Skip click if noClick mode is enabled (for apps that auto-keep focus on input)
	if noClick {
		// Just a small delay after activation
		time.Sleep(50 * time.Millisecond)
	} else if editHwnd == 0 || isQtApp || isChromiumApp {
		// Click on input area when:
		// - No standard edit control found, OR
		// - Qt app (native edit controls are unreliable after deactivation), OR
		// - Chromium/CEF app (native edit controls are NOT the actual chat input)
		clickInputAreaOfWindow(uintptr(targetHWND), clickOffsetX, clickOffsetY, targetProcess)
		// Wait for click to register and input to get focus
		time.Sleep(200 * time.Millisecond)
	} else {
		// Try to set focus to the edit control
		setWindowFocus(editHwnd)
		time.Sleep(100 * time.Millisecond)
	}

	// Strategy: Use keybd_event Ctrl+V which works at system level
	sendCtrlV()
	time.Sleep(150 * time.Millisecond)

	// Optionally trigger send
	if triggerSend {
		time.Sleep(200 * time.Millisecond)
		if sendKeyStrategy == "ctrl_enter" {
			sendCtrlEnter()
		} else {
			sendEnter()
		}
	}

	return nil
}

// PasteTextToTarget sends text to the target application's edit box without triggering send.
// noClick: if true, skip mouse click (for apps that auto-keep focus on input)
// clickOffsetX: pixels from left edge for input focus (0 = center)
// clickOffsetY: pixels from bottom for input focus (0 = use default based on app)
func PasteTextToTarget(targetProcess string, text string, noClick bool, clickOffsetX, clickOffsetY int) error {
	return SendTextToTarget(targetProcess, text, false, "", noClick, clickOffsetX, clickOffsetY)
}

// rect structure for Windows RECT
type rectInput struct {
	left   int32
	top    int32
	right  int32
	bottom int32
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

// WM_PASTE constant
const WM_PASTE = 0x0302

// childWindowInfo stores info about enumerated child windows
type childWindowInfo struct {
	hwnd      uintptr
	className string
	title     string
}

var (
	enumChildResults  []childWindowInfo
	enumChildCBOnce   sync.Once
	enumChildCB       uintptr
)

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
	enumChildCBOnce.Do(func() {
		enumChildCB = syscall.NewCallback(enumChildCallback)
	})
	enumChildResults = nil
	procEnumChildWindows.Call(parentHwnd, enumChildCB, 0)
	return enumChildResults
}

// sendWMPaste sends WM_PASTE message to a window
func sendWMPaste(hwnd uintptr) {
	procSendMessageW.Call(hwnd, WM_PASTE, 0, 0)
}

// findEditableChild tries to find an editable child window (input box)
// Returns the hwnd of the best candidate, or 0 if not found
func findEditableChild(parentHwnd uintptr) uintptr {
	children := enumerateChildWindows(parentHwnd)

	// Priority order for finding input boxes:
	// 1. Windows with "Edit" in class name
	// 2. Windows with "RichEdit" in class name
	// 3. Qt widgets that might be edit controls

	for _, child := range children {
		className := child.className
		// Check for standard Edit controls
		if className == "Edit" || className == "RichEdit" || className == "RichEdit20W" || className == "RichEdit20A" {
			return child.hwnd
		}
	}

	// For Qt applications like DingTalk, look for specific Qt widget classes
	// Qt edit controls often have class names like "Qt5151..." or contain "QWidget"
	for _, child := range children {
		className := child.className
		// Qt applications may have various widget types
		// We need to look deeper - Qt embeds its edit controls
		if strings.Contains(className, "Qt") {
			// Check children of Qt widgets
			grandChildren := enumerateChildWindows(child.hwnd)
			for _, gc := range grandChildren {
				if gc.className == "Edit" || strings.Contains(gc.className, "Edit") {
					return gc.hwnd
				}
			}
		}
	}

	return 0
}

// setWindowFocus sets keyboard focus to a window
// This requires the calling thread to be attached to the target thread's input queue
func setWindowFocus(hwnd uintptr) bool {
	ret, _, _ := procSetFocusIn.Call(hwnd)
	return ret != 0
}

// mouse_event flags
const (
	MOUSEEVENTF_LEFTDOWN = 0x0002
	MOUSEEVENTF_LEFTUP   = 0x0004
)

// POINT structure for GetCursorPos
type pointInput struct {
	x int32
	y int32
}

var procGetCursorPos = modUser32.NewProc("GetCursorPos")

// clickAtPosition simulates a mouse click at the given screen coordinates
// and restores the cursor to its original position afterward
func clickAtPosition(x, y int32) {
	// Save original cursor position
	var origPos pointInput
	procGetCursorPos.Call(uintptr(unsafe.Pointer(&origPos)))
	
	// Move cursor to target position
	procSetCursorPos.Call(uintptr(x), uintptr(y))
	time.Sleep(50 * time.Millisecond)
	
	// Mouse down and up
	procMouse_event.Call(MOUSEEVENTF_LEFTDOWN, 0, 0, 0, 0)
	time.Sleep(20 * time.Millisecond)
	procMouse_event.Call(MOUSEEVENTF_LEFTUP, 0, 0, 0, 0)
	
	// Wait a bit then restore cursor position
	time.Sleep(50 * time.Millisecond)
	procSetCursorPos.Call(uintptr(origPos.x), uintptr(origPos.y))
}

// findChatChildWindow finds the chat content child window (like DingChatWnd or CefBrowserWindow).
// For Chromium/Electron apps, EnumChildWindows may return many Chrome_WidgetWin_1 descendants
// (render widgets, popups, overlays). We pick the LARGEST one by area, which is typically the
// main content area, not a tiny internal widget.
func findChatChildWindow(parentHwnd uintptr) uintptr {
	children := enumerateChildWindows(parentHwnd)

	// Priority 1: DingChatWnd (app-specific, unique)
	for _, child := range children {
		if child.className == "DingChatWnd" {
			return child.hwnd
		}
	}
	// Priority 2: CefBrowserWindow (app-specific, usually unique)
	for _, child := range children {
		if child.className == "CefBrowserWindow" {
			return child.hwnd
		}
	}
	// Priority 3: Chrome_WidgetWin_1 — pick the one with the largest area.
	// Chromium apps often have many Chrome_WidgetWin_1 descendants at different levels;
	// the first one enumerated may be a tiny internal widget whose rect is too small
	// to calculate a meaningful click offset from.
	var bestHwnd uintptr
	var bestArea int64
	for _, child := range children {
		if child.className != "Chrome_WidgetWin_1" {
			continue
		}
		var r rectInput
		ret, _, _ := procGetWindowRectIn.Call(child.hwnd, uintptr(unsafe.Pointer(&r)))
		if ret == 0 {
			continue
		}
		w := int64(r.right - r.left)
		h := int64(r.bottom - r.top)
		area := w * h
		if area > bestArea {
			bestArea = area
			bestHwnd = child.hwnd
		}
	}
	return bestHwnd
}

// Default click offset from bottom (pixels) - varies by app
const defaultClickOffsetY = 120

// getDefaultClickOffsetY returns the default Y offset for a target process
// Different apps have different input box positions
func getDefaultClickOffsetY(targetProcess string) int {
	switch targetProcess {
	case "Feishu.exe", "Lark.exe":
		return 50
	default:
		return defaultClickOffsetY
	}
}

// getProcessWindowsBoundsForClick calculates the combined bounds of all visible windows
// belonging to the target process. Used for calculating click position relative to
// the entire application (e.g., WeCom with sidebar + main window).
func getProcessWindowsBoundsForClick(targetProcess string) (bounds rectInput, found bool) {
	targetNames := expandWindowsTargetNames(targetProcess)
	if len(targetNames) == 0 {
		return bounds, false
	}

	// Find target PID first
	var targetPID uint32
	for _, name := range targetNames {
		h, err := findMainWindowByProcessName(name)
		if err == nil && h != 0 {
			pid, err := getWindowProcessID(h)
			if err == nil && pid != 0 {
				targetPID = pid
				break
			}
		}
	}
	if targetPID == 0 {
		return bounds, false
	}

	gpwbClickMu.Lock()
	defer gpwbClickMu.Unlock()

	gpwbClickPID = targetPID
	gpwbClickBounds = rectInput{
		left:   0x7FFFFFFF,
		top:    0x7FFFFFFF,
		right:  -0x7FFFFFFF,
		bottom: -0x7FFFFFFF,
	}
	gpwbClickFound = false

	gpwbClickCBOnce.Do(func() {
		gpwbClickCB = syscall.NewCallback(gpwbClickEnumProc)
	})
	_, _, _ = procEnumWindows.Call(gpwbClickCB, 0)

	return gpwbClickBounds, gpwbClickFound
}

// clickInputAreaOfWindow clicks near the bottom of the window where the input box
// usually is. Coordinates are calculated relative to the passed-in main window
// (hwnd) or its chat child — never by enumerating all process windows — so that
// popup / preview windows cannot distort the click position.
//
// offsetX: pixels from the left edge of the target window (0 = center)
// offsetY: pixels from bottom (0 = use default based on target process)
// targetProcess: used only to determine default offsetY per-app
func clickInputAreaOfWindow(hwnd uintptr, offsetX, offsetY int, targetProcess string) bool {
	// Get the main window rect first (always needed as fallback and for X calculation).
	var mainRect rectInput
	if ret, _, _ := procGetWindowRectIn.Call(hwnd, uintptr(unsafe.Pointer(&mainRect))); ret == 0 {
		return false
	}
	mainHeight := mainRect.bottom - mainRect.top

	fmt.Printf("[SNAP_DEBUG] clickInputAreaOfWindow: mainRect={L:%d T:%d R:%d B:%d} h=%d, offsetX=%d offsetY=%d target=%s\n",
		mainRect.left, mainRect.top, mainRect.right, mainRect.bottom, mainHeight, offsetX, offsetY, targetProcess)

	// Try to find the chat child window for more precise Y targeting.
	chatChild := findChatChildWindow(hwnd)
	windowRect := mainRect // default: use main window rect for Y calculation
	if chatChild != 0 {
		var childRect rectInput
		if ret, _, _ := procGetWindowRectIn.Call(chatChild, uintptr(unsafe.Pointer(&childRect))); ret != 0 {
			childHeight := childRect.bottom - childRect.top
			fmt.Printf("[SNAP_DEBUG]   chatChild found: rect={L:%d T:%d R:%d B:%d} h=%d\n",
				childRect.left, childRect.top, childRect.right, childRect.bottom, childHeight)
			// Only use the chat child rect if it is large enough (at least half the main
			// window height). Small Chrome_WidgetWin_1 children are internal Chromium
			// widgets whose rect would cause the Y-offset clamp to always trigger.
			if childHeight >= mainHeight/2 && childHeight >= 300 {
				windowRect = childRect
				fmt.Printf("[SNAP_DEBUG]   using chatChild rect for Y calculation\n")
			} else {
				fmt.Printf("[SNAP_DEBUG]   chatChild too small (h=%d < mainH/2=%d or <300), using mainRect\n", childHeight, mainHeight/2)
			}
		}
	} else {
		fmt.Printf("[SNAP_DEBUG]   no chatChild found, using mainRect\n")
	}

	// Use provided offsetY or default based on app
	clickOffsetY := offsetY
	if clickOffsetY <= 0 {
		clickOffsetY = getDefaultClickOffsetY(targetProcess)
		fmt.Printf("[SNAP_DEBUG]   offsetY<=0, using default: %d\n", clickOffsetY)
	}

	// Calculate X position: relative to the main window, not combined process bounds
	var x int32
	if offsetX > 0 {
		x = mainRect.left + int32(offsetX)
		// Clamp within the main window
		if x > mainRect.right-10 {
			x = (mainRect.left + mainRect.right) / 2
		}
	} else {
		// Center horizontally within the main window
		x = (mainRect.left + mainRect.right) / 2
	}

	// Calculate Y position: from bottom of the target (child or main) window
	y := windowRect.bottom - int32(clickOffsetY)

	// Make sure we're within the window
	clamped := false
	if y < windowRect.top+100 {
		y = (windowRect.top + windowRect.bottom) / 2
		clamped = true
	}

	fmt.Printf("[SNAP_DEBUG]   FINAL click: x=%d y=%d (clamped=%v, windowRect={L:%d T:%d R:%d B:%d}, clickOffsetY=%d)\n",
		x, y, clamped, windowRect.left, windowRect.top, windowRect.right, windowRect.bottom, clickOffsetY)

	clickAtPosition(x, y)
	return true
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
	ret, _, _ := procSendInput.Call(
		uintptr(len(inputs)),
		uintptr(unsafe.Pointer(&inputs[0])),
		inputSize,
	)
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
	// Use keybd_event API - more compatible with some applications like DingTalk
	keybdEventKeyDown(VK_CONTROL)
	time.Sleep(30 * time.Millisecond)

	keybdEventKeyDown(VK_V)
	time.Sleep(30 * time.Millisecond)

	keybdEventKeyUp(VK_V)
	time.Sleep(30 * time.Millisecond)

	keybdEventKeyUp(VK_CONTROL)

	return 4
}

func sendEnter() uintptr {
	keybdEventKeyDown(VK_RETURN)
	time.Sleep(30 * time.Millisecond)

	keybdEventKeyUp(VK_RETURN)

	return 2
}

func sendCtrlEnter() uintptr {
	keybdEventKeyDown(VK_CONTROL)
	time.Sleep(30 * time.Millisecond)

	keybdEventKeyDown(VK_RETURN)
	time.Sleep(30 * time.Millisecond)

	keybdEventKeyUp(VK_RETURN)
	time.Sleep(30 * time.Millisecond)

	keybdEventKeyUp(VK_CONTROL)

	return 4
}
