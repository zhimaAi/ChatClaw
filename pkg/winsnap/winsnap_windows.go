//go:build windows

package winsnap

import (
	"errors"
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"github.com/wailsapp/wails/v3/pkg/application"
	"golang.org/x/sys/windows"
)

var (
)

func attachRightOfProcess(opts AttachOptions) (Controller, error) {
	if opts.Window == nil {
		return nil, errors.New("winsnap: Window is nil")
	}
	
	selfHWND := uintptr(opts.Window.NativeWindow())
	if selfHWND == 0 {
		return nil, errors.New("winsnap: native window handle is 0")
	}
	
	targetName := normalizeProcessName(opts.TargetProcessName)
	if targetName == "" {
		return nil, errors.New("winsnap: TargetProcessName is empty")
	}
	findTimeout := opts.FindTimeout
	if findTimeout <= 0 {
		findTimeout = 20 * time.Second
	}

	deadline := time.Now().Add(findTimeout)
	var target windows.HWND
	for {
		h, err := findMainWindowByProcessName(targetName)
		if err == nil && h != 0 {
			target = h
			break
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("%w: %s", ErrTargetWindowNotFound, targetName)
		}
		time.Sleep(250 * time.Millisecond)
	}

	f := &follower{
		self:   windows.HWND(selfHWND),
		target: target,
		gap:    opts.Gap,
		ready:  make(chan struct{}),
		app:    opts.App,
	}
	if err := f.start(); err != nil {
		_ = f.Stop()
		return nil, err
	}
	return f, nil
}

type follower struct {
	self   windows.HWND
	target windows.HWND
	gap    int
	app    *application.App

	mu         sync.Mutex
	hook       uintptr
	tid        uint32
	ready      chan struct{}
	closed     bool
	selfWidth  int32 // 缓存自己的宽度
}

func (f *follower) Stop() error {
	f.mu.Lock()
	if f.closed {
		f.mu.Unlock()
		return nil
	}
	f.closed = true
	hook := f.hook
	tid := f.tid
	f.hook = 0
	f.mu.Unlock()

	if hook != 0 {
		_, _, _ = procUnhookWinEvent.Call(hook)
	}
	if tid != 0 {
		_, _, _ = procPostThreadMessageW.Call(uintptr(tid), uintptr(wmQuit), 0, 0)
	}
	return nil
}

func (f *follower) start() error {
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()

		f.mu.Lock()
		f.tid = getCurrentThreadId()
		f.mu.Unlock()

		// 缓存自己的初始宽度
		var selfWin rect
		if err := getWindowRect(f.self, &selfWin); err == nil {
			f.selfWidth = selfWin.Right - selfWin.Left
		}

		// Initial sync.
		_ = f.syncToTarget()

		pid, err := getWindowProcessID(f.target)
		if err != nil {
			close(f.ready)
			return
		}

		cb := syscall.NewCallback(f.winEventProc)
		hook, _, errNo := procSetWinEventHook.Call(
			uintptr(eventObjectLocationChange),
			uintptr(eventObjectLocationChange),
			0,
			cb,
			uintptr(pid),
			0,
			uintptr(wineventOutOfContext|wineventSkipOwnProcess),
		)
		f.mu.Lock()
		f.hook = hook
		f.mu.Unlock()
		close(f.ready)

		if hook == 0 {
			_ = errNo
			return
		}

		var msg msg
		for {
			ret, _, _ := procGetMessageW.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
			switch int32(ret) {
			case -1:
				return
			case 0:
				return
			default:
				_, _, _ = procTranslateMessage.Call(uintptr(unsafe.Pointer(&msg)))
				_, _, _ = procDispatchMessageW.Call(uintptr(unsafe.Pointer(&msg)))
			}
		}
	}()

	<-f.ready

	f.mu.Lock()
	defer f.mu.Unlock()
	if f.hook == 0 {
		return errors.New("winsnap: failed to install WinEvent hook")
	}
	return nil
}

func (f *follower) winEventProc(_ uintptr, event uint32, hwnd windows.HWND, idObject, idChild int32, _ uint32, _ uint32) uintptr {
	if event != eventObjectLocationChange {
		return 0
	}
	if hwnd != f.target {
		return 0
	}
	// We only care about the window object itself.
	if idObject != objidWindow || idChild != 0 {
		return 0
	}
	_ = f.syncToTarget()
	return 0
}

func (f *follower) syncToTarget() error {
	if !isWindow(f.target) {
		return errors.New("winsnap: target window is not valid")
	}

	var targetWin, targetFrame rect
	if err := getWindowRect(f.target, &targetWin); err != nil {
		return err
	}
	if err := getExtendedFrameBounds(f.target, &targetFrame); err != nil {
		// Fallback when DWM call fails.
		targetFrame = targetWin
	}

	var selfWin, selfFrame rect
	if err := getWindowRect(f.self, &selfWin); err != nil {
		return err
	}
	if err := getExtendedFrameBounds(f.self, &selfFrame); err != nil {
		selfFrame = selfWin
	}

	// Windows 10/11 often have an invisible resize border or extended frame.
	// Align *visible* frame edges, not the raw window rect edges, so "Gap=0"
	// looks truly adjacent.
	selfOffsetX := selfFrame.Left - selfWin.Left
	selfOffsetY := selfFrame.Top - selfWin.Top

	x := targetFrame.Right + int32(f.gap) - selfOffsetX
	y := targetFrame.Top - selfOffsetY
	
	// 使用目标窗口的高度作为自己的高度，保持固定宽度
	targetHeight := targetFrame.Bottom - targetFrame.Top
	width := f.selfWidth
	if width <= 0 {
		width = selfWin.Right - selfWin.Left
	}
	
	return setWindowPosWithSize(f.self, x, y, width, targetHeight)
}

func normalizeProcessName(name string) string {
	n := strings.TrimSpace(name)
	if n == "" {
		return ""
	}
	n = filepath.Base(n)
	ln := strings.ToLower(n)
	if !strings.HasSuffix(ln, ".exe") {
		n += ".exe"
	}
	return n
}

func findMainWindowByProcessName(processName string) (windows.HWND, error) {
	targetLower := strings.ToLower(processName)
	var found windows.HWND
	cb := syscall.NewCallback(func(hwnd uintptr, _ uintptr) uintptr {
		h := windows.HWND(hwnd)
		if !isTopLevelCandidate(h) {
			return 1
		}
		pid, err := getWindowProcessID(h)
		if err != nil || pid == 0 {
			return 1
		}
		exe, err := getProcessImageBaseName(pid)
		if err != nil {
			return 1
		}
		if strings.ToLower(exe) != targetLower {
			return 1
		}
		found = h
		return 0
	})
	_, _, _ = procEnumWindows.Call(cb, 0)
	if found == 0 {
		return 0, ErrTargetWindowNotFound
	}
	return found, nil
}

func isTopLevelCandidate(hwnd windows.HWND) bool {
	if hwnd == 0 {
		return false
	}
	if !isWindowVisible(hwnd) {
		return false
	}
	// Exclude owned windows (tooltips, dialogs, etc.)
	if getWindowOwner(hwnd) != 0 {
		return false
	}
	// Prefer windows with titles (main windows usually have a title).
	if getWindowTextLength(hwnd) == 0 {
		return false
	}
	return true
}

// --- Win32 bindings ---

const (
	eventObjectLocationChange = 0x800B
	objidWindow               = 0

	wineventOutOfContext   = 0x0000
	wineventSkipOwnProcess = 0x0002

	gwOwner = 4

	swpNoSize     = 0x0001
	swpNoZOrder   = 0x0004
	swpNoActivate = 0x0010

	wmQuit = 0x0012
)

type rect struct {
	Left   int32
	Top    int32
	Right  int32
	Bottom int32
}

type point struct {
	X int32
	Y int32
}

type msg struct {
	HWnd    windows.HWND
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Pt      point
}

var (
	modUser32   = windows.NewLazySystemDLL("user32.dll")
	modKernel32 = windows.NewLazySystemDLL("kernel32.dll")
	modDwmapi   = windows.NewLazySystemDLL("dwmapi.dll")

	procEnumWindows          = modUser32.NewProc("EnumWindows")
	procGetWindowThreadPID   = modUser32.NewProc("GetWindowThreadProcessId")
	procIsWindowVisible      = modUser32.NewProc("IsWindowVisible")
	procIsWindow             = modUser32.NewProc("IsWindow")
	procGetWindow            = modUser32.NewProc("GetWindow")
	procGetWindowTextLengthW = modUser32.NewProc("GetWindowTextLengthW")
	procGetWindowRect        = modUser32.NewProc("GetWindowRect")
	procSetWindowPos         = modUser32.NewProc("SetWindowPos")
	procSetWinEventHook      = modUser32.NewProc("SetWinEventHook")
	procUnhookWinEvent       = modUser32.NewProc("UnhookWinEvent")
	procGetMessageW          = modUser32.NewProc("GetMessageW")
	procTranslateMessage     = modUser32.NewProc("TranslateMessage")
	procDispatchMessageW     = modUser32.NewProc("DispatchMessageW")
	procPostThreadMessageW   = modUser32.NewProc("PostThreadMessageW")

	procGetCurrentThreadId = modKernel32.NewProc("GetCurrentThreadId")

	procDwmGetWindowAttribute = modDwmapi.NewProc("DwmGetWindowAttribute")
)

func getCurrentThreadId() uint32 {
	r1, _, _ := procGetCurrentThreadId.Call()
	return uint32(r1)
}

func isWindowVisible(hwnd windows.HWND) bool {
	r1, _, _ := procIsWindowVisible.Call(uintptr(hwnd))
	return r1 != 0
}

func isWindow(hwnd windows.HWND) bool {
	r1, _, _ := procIsWindow.Call(uintptr(hwnd))
	return r1 != 0
}

func getWindowOwner(hwnd windows.HWND) windows.HWND {
	r1, _, _ := procGetWindow.Call(uintptr(hwnd), uintptr(gwOwner))
	return windows.HWND(r1)
}

func getWindowTextLength(hwnd windows.HWND) int {
	r1, _, _ := procGetWindowTextLengthW.Call(uintptr(hwnd))
	return int(r1)
}

func getWindowRect(hwnd windows.HWND, out *rect) error {
	r1, _, errNo := procGetWindowRect.Call(uintptr(hwnd), uintptr(unsafe.Pointer(out)))
	if r1 == 0 {
		if errNo != nil && errNo != syscall.Errno(0) {
			return errNo
		}
		return syscall.EINVAL
	}
	return nil
}

const dwmwaExtendedFrameBounds = 9

func getExtendedFrameBounds(hwnd windows.HWND, out *rect) error {
	// HRESULT DwmGetWindowAttribute(HWND, DWORD, PVOID, DWORD)
	r1, _, _ := procDwmGetWindowAttribute.Call(
		uintptr(hwnd),
		uintptr(dwmwaExtendedFrameBounds),
		uintptr(unsafe.Pointer(out)),
		uintptr(unsafe.Sizeof(*out)),
	)
	if int32(r1) != 0 {
		return fmt.Errorf("dwm get window attribute failed: 0x%X", uint32(r1))
	}
	return nil
}

func setWindowPosNoSizeNoZ(hwnd windows.HWND, x, y int32) error {
	flags := uintptr(swpNoSize | swpNoZOrder | swpNoActivate)
	r1, _, errNo := procSetWindowPos.Call(
		uintptr(hwnd),
		0,
		uintptr(x),
		uintptr(y),
		0,
		0,
		flags,
	)
	if r1 == 0 {
		if errNo != nil && errNo != syscall.Errno(0) {
			return errNo
		}
		return syscall.EINVAL
	}
	return nil
}

func setWindowPosWithSize(hwnd windows.HWND, x, y, width, height int32) error {
	flags := uintptr(swpNoZOrder | swpNoActivate)
	r1, _, errNo := procSetWindowPos.Call(
		uintptr(hwnd),
		0,
		uintptr(x),
		uintptr(y),
		uintptr(width),
		uintptr(height),
		flags,
	)
	if r1 == 0 {
		if errNo != nil && errNo != syscall.Errno(0) {
			return errNo
		}
		return syscall.EINVAL
	}
	return nil
}

func getWindowProcessID(hwnd windows.HWND) (uint32, error) {
	var pid uint32
	_, _, errNo := procGetWindowThreadPID.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&pid)))
	if pid == 0 {
		if errNo != nil && errNo != syscall.Errno(0) {
			return 0, errNo
		}
		return 0, syscall.EINVAL
	}
	return pid, nil
}

func getProcessImageBaseName(pid uint32) (string, error) {
	// PROCESS_QUERY_LIMITED_INFORMATION = 0x1000
	h, err := windows.OpenProcess(0x1000, false, pid)
	if err != nil {
		return "", err
	}
	defer windows.CloseHandle(h)

	// QueryFullProcessImageNameW
	const max = 32767
	buf := make([]uint16, max)
	size := uint32(len(buf))
	if err := windows.QueryFullProcessImageName(h, 0, &buf[0], &size); err != nil {
		return "", err
	}
	full := windows.UTF16ToString(buf[:size])
	return filepath.Base(full), nil
}
