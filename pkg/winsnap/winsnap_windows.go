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

var ()

func attachRightOfProcess(opts AttachOptions) (Controller, error) {
	if opts.Window == nil {
		return nil, ErrWinsnapWindowInvalid
	}

	selfHWND := uintptr(opts.Window.NativeWindow())
	if selfHWND == 0 {
		return nil, ErrWinsnapWindowInvalid
	}

	targetNames := expandWindowsTargetNames(opts.TargetProcessName)
	if len(targetNames) == 0 {
		return nil, errors.New("winsnap: TargetProcessName is empty")
	}
	findTimeout := opts.FindTimeout
	if findTimeout <= 0 {
		findTimeout = 20 * time.Second
	}

	deadline := time.Now().Add(findTimeout)
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
			return nil, fmt.Errorf("%w: %s", ErrTargetWindowNotFound, strings.Join(targetNames, ","))
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

	mu        sync.Mutex
	hookLoc   uintptr
	hookFG    uintptr
	tid       uint32
	ready     chan struct{}
	closed    bool
	selfWidth int32 // 缓存自己的宽度
}

func (f *follower) Stop() error {
	f.mu.Lock()
	if f.closed {
		f.mu.Unlock()
		return nil
	}
	f.closed = true
	hookLoc := f.hookLoc
	hookFG := f.hookFG
	tid := f.tid
	f.hookLoc = 0
	f.hookFG = 0
	f.mu.Unlock()

	if hookLoc != 0 {
		_, _, _ = procUnhookWinEvent.Call(hookLoc)
	}
	if hookFG != 0 {
		_, _, _ = procUnhookWinEvent.Call(hookFG)
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
		// 1) 监听目标窗口位置变化（移动/缩放）
		hookLoc, _, errNo := procSetWinEventHook.Call(
			uintptr(eventObjectLocationChange),
			uintptr(eventObjectLocationChange),
			0,
			cb,
			uintptr(pid),
			0,
			uintptr(wineventOutOfContext|wineventSkipOwnProcess),
		)

		// 2) 监听目标进程前台窗口变化（激活/置前），用于同步 z-order，
		// 让吸附框始终处于“与目标窗口同层级（紧贴其上方）”。
		hookFG, _, _ := procSetWinEventHook.Call(
			uintptr(eventSystemForeground),
			uintptr(eventSystemForeground),
			0,
			cb,
			uintptr(pid),
			0,
			uintptr(wineventOutOfContext|wineventSkipOwnProcess),
		)

		f.mu.Lock()
		f.hookLoc = hookLoc
		f.hookFG = hookFG
		f.mu.Unlock()
		close(f.ready)

		if hookLoc == 0 {
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
	if f.hookLoc == 0 {
		return errors.New("winsnap: failed to install WinEvent hook")
	}
	return nil
}

func (f *follower) winEventProc(_ uintptr, event uint32, hwnd windows.HWND, idObject, idChild int32, _ uint32, _ uint32) uintptr {
	switch event {
	case eventObjectLocationChange:
		if hwnd != f.target {
			return 0
		}
		// We only care about the window object itself.
		if idObject != objidWindow || idChild != 0 {
			return 0
		}
	case eventSystemForeground:
		// Foreground event doesn't use object/child the same way; just react
		// when the foreground window is the target window.
		if hwnd != f.target {
			return 0
		}
	default:
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

	// 与目标窗口同层级：
	// - 若目标是 top-most，则吸附框也进入 top-most 组，并紧贴在目标窗口之上
	// - 若目标不是 top-most，则确保吸附框不在 top-most 组，并紧贴在目标窗口之上
	if isTopMost(f.target) {
		// Ensure self is top-most first (no move/size), then place after target.
		_ = setWindowTopMostNoActivate(f.self)
		return setWindowPosWithSizeAfter(f.self, f.target, x, y, width, targetHeight)
	}
	_ = setWindowNoTopMostNoActivate(f.self)
	return setWindowPosWithSizeAfter(f.self, f.target, x, y, width, targetHeight)
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

func expandWindowsTargetNames(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	// Normalize input for alias matching.
	key := strings.ToLower(strings.TrimSpace(filepath.Base(raw)))
	key = strings.TrimSuffix(key, ".exe")
	key = strings.TrimSuffix(key, ".app")

	// Common aliases (Chinese / pinyin / branding).
	switch key {
	case "微信", "weixin", "wechat":
		// Different channels/versions use different exe names.
		return []string{"Weixin.exe", "WeChat.exe", "WeChatApp.exe", "WeChatAppEx.exe"}
	case "企业微信", "wecom", "wework", "wxwork", "qiyeweixin":
		return []string{"WXWork.exe"}
	case "qq":
		return []string{"QQ.exe", "QQNT.exe"}
	case "钉钉", "dingtalk":
		return []string{"DingTalk.exe"}
	case "飞书", "feishu", "lark":
		return []string{"Feishu.exe", "Lark.exe"}
	case "抖音", "douyin":
		return []string{"Douyin.exe"}
	default:
		return []string{normalizeProcessName(raw)}
	}
}

func findMainWindowByProcessName(processName string) (windows.HWND, error) {
	targetLower := strings.ToLower(processName)

	type cand struct {
		hwnd  windows.HWND
		score int
		area  int64
	}
	var best cand

	cb := syscall.NewCallback(func(hwnd uintptr, _ uintptr) uintptr {
		h := windows.HWND(hwnd)
		// Keep this check permissive; some apps (e.g. WeChat) may have empty titles or owned main windows.
		if !isWindowVisible(h) || isWindowIconic(h) {
			return 1
		}

		pid, err := getWindowProcessID(h)
		if err != nil || pid == 0 {
			return 1
		}
		exe, err := getProcessImageBaseName(pid)
		if err != nil || strings.ToLower(exe) != targetLower {
			return 1
		}

		ex := getExStyle(h)
		owner := getWindowOwner(h)
		titleLen := getWindowTextLength(h)

		// Compute a score that prefers "main app window".
		score := 0
		if ex&wsExToolWindow != 0 {
			score -= 200
		}
		if ex&wsExNoActivate != 0 {
			score -= 50
		}
		if ex&wsExAppWindow != 0 {
			score += 30
		}
		if owner == 0 {
			score += 100
		} else {
			score -= 20
		}
		if titleLen > 0 {
			score += 20
		}

		var r rect
		if err := getWindowRect(h, &r); err == nil {
			w := int64(r.Right - r.Left)
			hh := int64(r.Bottom - r.Top)
			if w > 0 && hh > 0 {
				area := w * hh
				// Prefer higher score; tie-break by larger area.
				if best.hwnd == 0 || score > best.score || (score == best.score && area > best.area) {
					best = cand{hwnd: h, score: score, area: area}
				}
			}
		} else {
			// No rect: still allow score-only selection.
			if best.hwnd == 0 || score > best.score {
				best = cand{hwnd: h, score: score, area: 0}
			}
		}
		return 1 // continue enumeration
	})
	_, _, _ = procEnumWindows.Call(cb, 0)

	if best.hwnd == 0 {
		return 0, ErrTargetWindowNotFound
	}
	return best.hwnd, nil
}

func isTopLevelCandidate(hwnd windows.HWND) bool {
	if hwnd == 0 {
		return false
	}
	if !isWindowVisible(hwnd) {
		return false
	}
	// Treat minimized windows as "not displayed".
	if isWindowIconic(hwnd) {
		return false
	}
	ex := getExStyle(hwnd)
	// Exclude tool windows (tooltips, floating tool palettes, etc.)
	if ex&wsExToolWindow != 0 {
		return false
	}
	// Exclude owned windows unless they explicitly opt-in as app windows.
	if getWindowOwner(hwnd) != 0 && ex&wsExAppWindow == 0 {
		return false
	}
	return true
}

// --- Win32 bindings ---

const (
	eventSystemForeground     = 0x0003
	eventObjectLocationChange = 0x800B
	objidWindow               = 0

	wineventOutOfContext   = 0x0000
	wineventSkipOwnProcess = 0x0002

	gwOwner = 4

	// Special HWND values for SetWindowPos
	// https://learn.microsoft.com/windows/win32/api/winuser/nf-winuser-setwindowpos
	hwndTopMost   = ^uintptr(0) // (HWND)-1
	hwndNoTopMost = ^uintptr(1) // (HWND)-2

	swpNoSize     = 0x0001
	swpNoMove     = 0x0002
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
	procIsIconic             = modUser32.NewProc("IsIconic")
	procGetWindow            = modUser32.NewProc("GetWindow")
	procGetWindowTextLengthW = modUser32.NewProc("GetWindowTextLengthW")
	procGetWindowRect        = modUser32.NewProc("GetWindowRect")
	procSetWindowPos         = modUser32.NewProc("SetWindowPos")
	procGetWindowLongPtrW    = modUser32.NewProc("GetWindowLongPtrW")
	procSetWinEventHook      = modUser32.NewProc("SetWinEventHook")
	procUnhookWinEvent       = modUser32.NewProc("UnhookWinEvent")
	procGetMessageW          = modUser32.NewProc("GetMessageW")
	procTranslateMessage     = modUser32.NewProc("TranslateMessage")
	procDispatchMessageW     = modUser32.NewProc("DispatchMessageW")
	procPostThreadMessageW   = modUser32.NewProc("PostThreadMessageW")

	procGetCurrentThreadId = modKernel32.NewProc("GetCurrentThreadId")

	procDwmGetWindowAttribute = modDwmapi.NewProc("DwmGetWindowAttribute")
)

// GWL_EXSTYLE = -20 (needs special handling for 64-bit)
var gwlExStyle = ^uintptr(19) // equivalent to -20 as uintptr

const wsExTopMost = 0x00000008

const (
	wsExToolWindow = 0x00000080
	wsExAppWindow  = 0x00040000
	wsExNoActivate = 0x08000000
)

func getExStyle(hwnd windows.HWND) uintptr {
	style, _, _ := procGetWindowLongPtrW.Call(uintptr(hwnd), gwlExStyle)
	return style
}

func isTopMost(hwnd windows.HWND) bool {
	return getExStyle(hwnd)&wsExTopMost != 0
}

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

func isWindowIconic(hwnd windows.HWND) bool {
	r1, _, _ := procIsIconic.Call(uintptr(hwnd))
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

func setWindowPosWithSizeAfter(hwnd windows.HWND, insertAfter windows.HWND, x, y, width, height int32) error {
	flags := uintptr(swpNoActivate)
	r1, _, errNo := procSetWindowPos.Call(
		uintptr(hwnd),
		uintptr(insertAfter),
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

func setWindowTopMostNoActivate(hwnd windows.HWND) error {
	flags := uintptr(swpNoMove | swpNoSize | swpNoActivate)
	r1, _, errNo := procSetWindowPos.Call(uintptr(hwnd), hwndTopMost, 0, 0, 0, 0, flags)
	if r1 == 0 {
		if errNo != nil && errNo != syscall.Errno(0) {
			return errNo
		}
		return syscall.EINVAL
	}
	return nil
}

func setWindowNoTopMostNoActivate(hwnd windows.HWND) error {
	flags := uintptr(swpNoMove | swpNoSize | swpNoActivate)
	r1, _, errNo := procSetWindowPos.Call(uintptr(hwnd), hwndNoTopMost, 0, 0, 0, 0, flags)
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
