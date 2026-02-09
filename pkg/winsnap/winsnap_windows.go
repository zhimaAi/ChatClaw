//go:build windows

package winsnap

import (
	"errors"
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"

	"github.com/wailsapp/wails/v3/pkg/application"
	"golang.org/x/sys/windows"
)

// ---------- Reusable EnumWindows callbacks ----------
// Go on Windows has a hard limit (~2000) for syscall.NewCallback slots.
// We create each callback once and communicate per-call parameters via
// package-level variables guarded by a mutex. EnumWindows invokes the
// callback synchronously, so the mutex is held for the entire enumeration.

var (
	// getProcessWindowsBounds callback
	gpwbCBOnce      sync.Once
	gpwbCB          uintptr
	gpwbMu          sync.Mutex
	gpwbPID         uint32
	gpwbSelfPID     uint32
	gpwbBounds      rect
	gpwbFound       bool
)

func gpwbEnumProc(hwnd uintptr, _ uintptr) uintptr {
	h := windows.HWND(hwnd)
	if !isWindowVisible(h) || isWindowIconic(h) {
		return 1
	}
	winPID, err := getWindowProcessID(h)
	if err != nil || winPID != gpwbPID {
		return 1
	}
	if gpwbSelfPID != 0 && winPID == gpwbSelfPID {
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
	if r.Left < gpwbBounds.Left {
		gpwbBounds.Left = r.Left
	}
	if r.Top < gpwbBounds.Top {
		gpwbBounds.Top = r.Top
	}
	if r.Right > gpwbBounds.Right {
		gpwbBounds.Right = r.Right
	}
	if r.Bottom > gpwbBounds.Bottom {
		gpwbBounds.Bottom = r.Bottom
	}
	gpwbFound = true
	return 1
}

var (
	// findMainWindowByProcessNameEx callback
	fmwCBOnce         sync.Once
	fmwCB             uintptr
	fmwMu             sync.Mutex
	fmwTargetLower    string
	fmwIncludeMin     bool
	fmwBestHwnd       windows.HWND
	fmwBestScore      int
	fmwBestArea       int64
	fmwBestMinimized  bool
)

func fmwEnumProc(hwnd uintptr, _ uintptr) uintptr {
	h := windows.HWND(hwnd)
	isMin := isWindowIconic(h)
	if !isWindowVisible(h) {
		return 1
	}
	if isMin && !fmwIncludeMin {
		return 1
	}
	pid, err := getWindowProcessID(h)
	if err != nil || pid == 0 {
		return 1
	}
	exe, err := getProcessImageBaseName(pid)
	if err != nil || strings.ToLower(exe) != fmwTargetLower {
		return 1
	}
	ex := getExStyle(h)
	owner := getWindowOwner(h)
	titleLen := getWindowTextLength(h)
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
	if isMin {
		score -= 10
	}
	var r rect
	if err := getWindowRect(h, &r); err == nil {
		w := int64(r.Right - r.Left)
		hh := int64(r.Bottom - r.Top)
		if w > 0 && hh > 0 {
			area := w * hh
			if fmwBestHwnd == 0 || score > fmwBestScore || (score == fmwBestScore && area > fmwBestArea) {
				fmwBestHwnd = h
				fmwBestScore = score
				fmwBestArea = area
				fmwBestMinimized = isMin
			}
		}
	} else {
		if fmwBestHwnd == 0 || score > fmwBestScore {
			fmwBestHwnd = h
			fmwBestScore = score
			fmwBestArea = 0
			fmwBestMinimized = isMin
		}
	}
	return 1
}

// follower WinEventHook callback: created once, dispatches via thread-ID map.
// SetWinEventHook with WINEVENT_OUTOFCONTEXT invokes the callback on the
// same thread's message pump, so we map threadID → *follower.
var (
	followerCBOnce sync.Once
	followerCB     uintptr
	followerMap    sync.Map // uint32 (thread ID) -> *follower
)

func followerWinEventShim(hHook uintptr, event uint32, hwnd windows.HWND, idObject, idChild int32, eventThread, eventTime uint32) uintptr {
	tid := getCurrentThreadId()
	if v, ok := followerMap.Load(tid); ok {
		return v.(*follower).winEventProc(hHook, event, hwnd, idObject, idChild, eventThread, eventTime)
	}
	return 0
}

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
	selfWidth int32 // cached initial width of our window

	// Throttling for syncToTarget to avoid overwhelming system calls
	syncing int32 // atomic flag: 1 = currently syncing, 0 = idle
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

		// Register this follower for WinEventHook dispatch; create callback once.
		followerMap.Store(f.tid, f)
		defer followerMap.Delete(f.tid)
		followerCBOnce.Do(func() {
			followerCB = syscall.NewCallback(followerWinEventShim)
		})

		// 1) 监听目标窗口位置变化（移动/缩放）
		hookLoc, _, errNo := procSetWinEventHook.Call(
			uintptr(eventObjectLocationChange),
			uintptr(eventObjectLocationChange),
			0,
			followerCB,
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
			followerCB,
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
	// Note: SetWinEventHook is already configured with pid filter, so we don't need
	// to call getWindowProcessID here. All events received are from the target process.
	switch event {
	case eventObjectLocationChange:
		// We only care about the window object itself (not child objects like scrollbars).
		if idObject != objidWindow || idChild != 0 {
			return 0
		}
		// Only track the anchored main target window — ignore popup/preview windows
		// from the same process. This matches the demo project's proven approach.
		if hwnd != f.target {
			return 0
		}
		_ = f.syncToTarget()
	case eventSystemForeground:
		// Skip if we are the one becoming foreground
		if hwnd == f.self {
			return 0
		}
		// When target becomes foreground, we need to ensure our window is also
		// brought to the top, placed right after the target in z-order.
		_ = f.syncToTargetWithZOrderFix()
	default:
		return 0
	}
	return 0
}

// getProcessWindowsBounds calculates the combined bounds of all visible windows
// belonging to the target process. This handles apps like WeCom that have multiple
// windows (sidebar + main chat window).
//
// Parameters:
//   - pid: target process ID
//   - selfHWND: our own window handle (to exclude from bounds calculation)
//
// Returns the combined bounds (left, top, right, bottom) of all visible windows.
// The winsnap window (selfHWND) is excluded from the bounds calculation.
func getProcessWindowsBounds(pid uint32, selfHWND windows.HWND) (bounds rect, found bool) {
	var selfPID uint32
	if selfHWND != 0 {
		selfPID, _ = getWindowProcessID(selfHWND)
	}

	gpwbMu.Lock()
	defer gpwbMu.Unlock()

	gpwbPID = pid
	gpwbSelfPID = selfPID
	gpwbBounds = rect{
		Left:   0x7FFFFFFF,
		Top:    0x7FFFFFFF,
		Right:  -0x7FFFFFFF,
		Bottom: -0x7FFFFFFF,
	}
	gpwbFound = false

	gpwbCBOnce.Do(func() {
		gpwbCB = syscall.NewCallback(gpwbEnumProc)
	})
	_, _, _ = procEnumWindows.Call(gpwbCB, 0)

	return gpwbBounds, gpwbFound
}

// getTargetBounds returns the bounds of the single anchored target window.
// Always uses f.target only — never enumerates other process windows — so that
// popup / preview windows from the same process cannot distort the position.
func (f *follower) getTargetBounds() (targetBounds rect, err error) {
	if err = getExtendedFrameBounds(f.target, &targetBounds); err != nil {
		var targetWin rect
		if err = getWindowRect(f.target, &targetWin); err != nil {
			return
		}
		targetBounds = targetWin
	}
	return
}

// calcSnapGeometry computes the x, y, width, height for positioning the winsnap
// window to the right of the target window.
func (f *follower) calcSnapGeometry(targetBounds rect) (x, y, width, height int32, err error) {
	var selfWin, selfFrame rect
	if err = getWindowRect(f.self, &selfWin); err != nil {
		return
	}
	if err = getExtendedFrameBounds(f.self, &selfFrame); err != nil {
		selfFrame = selfWin
		err = nil // non-fatal
	}

	// Windows 10/11 often have an invisible resize border or extended frame.
	// Align *visible* frame edges, not the raw window rect edges, so "Gap=0"
	// looks truly adjacent.
	selfOffsetX := selfFrame.Left - selfWin.Left
	selfOffsetY := selfFrame.Top - selfWin.Top

	x = targetBounds.Right + int32(f.gap) - selfOffsetX
	y = targetBounds.Top - selfOffsetY
	height = targetBounds.Bottom - targetBounds.Top
	width = f.selfWidth
	if width <= 0 {
		width = selfWin.Right - selfWin.Left
	}
	return
}

func (f *follower) syncToTarget() error {
	// Throttle: skip if already syncing to avoid overwhelming system
	if !atomic.CompareAndSwapInt32(&f.syncing, 0, 1) {
		return nil
	}
	defer atomic.StoreInt32(&f.syncing, 0)

	if !isWindow(f.target) {
		return errors.New("winsnap: target window is not valid")
	}

	targetBounds, err := f.getTargetBounds()
	if err != nil {
		return err
	}

	x, y, width, height, err := f.calcSnapGeometry(targetBounds)
	if err != nil {
		return err
	}

	// Match z-order layer with target window:
	// - If target is top-most, promote self into top-most group and place after target.
	// - Otherwise, ensure self is not top-most and place after target.
	if isTopMost(f.target) {
		_ = setWindowTopMostNoActivate(f.self)
		return setWindowPosWithSizeAfter(f.self, f.target, x, y, width, height)
	}
	_ = setWindowNoTopMostNoActivate(f.self)
	return setWindowPosWithSizeAfter(f.self, f.target, x, y, width, height)
}

// syncToTargetWithZOrderFix is called when the target window becomes foreground.
// It ensures our winsnap window is brought to the top along with the target.
// This is needed because SetWindowPos with insertAfter=target may not bring our
// window above other windows that were previously above us.
//
// Uses the same single-window bounds as syncToTarget (anchored to f.target only).
func (f *follower) syncToTargetWithZOrderFix() error {
	if !isWindow(f.target) {
		return errors.New("winsnap: target window is not valid")
	}

	targetBounds, err := f.getTargetBounds()
	if err != nil {
		return err
	}

	x, y, width, height, err := f.calcSnapGeometry(targetBounds)
	if err != nil {
		return err
	}

	// When target becomes foreground, we need to ensure winsnap is also visible.
	// First, bring our window to HWND_TOP (above all non-topmost windows),
	// then position it properly after the target.
	if isTopMost(f.target) {
		_ = setWindowTopMostNoActivate(f.self)
	} else {
		_ = setWindowNoTopMostNoActivate(f.self)
		// Explicitly bring to top first before positioning after target
		_ = bringWindowToTopNoActivate(f.self)
	}
	return setWindowPosWithSizeAfter(f.self, f.target, x, y, width, height)
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
	return findMainWindowByProcessNameEx(processName, false)
}

// findMainWindowByProcessNameIncludingMinimized finds the main window even if it's minimized.
// This is used for wake operations where we need to restore minimized windows.
func findMainWindowByProcessNameIncludingMinimized(processName string) (windows.HWND, error) {
	return findMainWindowByProcessNameEx(processName, true)
}

// findMainWindowByProcessNameEx finds a representative main window by process name.
// This is used for both event listening and position anchoring — the follower
// only tracks this single window (no multi-window combined bounds).
// If includeMinimized is true, it will also consider minimized windows.
func findMainWindowByProcessNameEx(processName string, includeMinimized bool) (windows.HWND, error) {
	fmwMu.Lock()
	defer fmwMu.Unlock()

	fmwTargetLower = strings.ToLower(processName)
	fmwIncludeMin = includeMinimized
	fmwBestHwnd = 0
	fmwBestScore = 0
	fmwBestArea = 0
	fmwBestMinimized = false

	fmwCBOnce.Do(func() {
		fmwCB = syscall.NewCallback(fmwEnumProc)
	})
	_, _, _ = procEnumWindows.Call(fmwCB, 0)

	if fmwBestHwnd == 0 {
		return 0, ErrTargetWindowNotFound
	}
	return fmwBestHwnd, nil
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
	// NOTE: Do NOT filter on WS_POPUP here. Many modern apps (WeChat, DingTalk)
	// use WS_POPUP for their main window because they implement custom title bars.
	// The real protection against popup/preview interference comes from:
	// - winEventProc only tracking f.target (ignores other process windows)
	// - syncToTarget using single-window bounds (no combined bounds)
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
	hwndTop       = uintptr(0)  // (HWND)0 - Places the window at the top of the Z order
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

// bringWindowToTopNoActivate brings the window to the top of the z-order
// without activating it. This is useful when the target window becomes foreground
// and we need to ensure our window is also visible above other windows.
func bringWindowToTopNoActivate(hwnd windows.HWND) error {
	flags := uintptr(swpNoMove | swpNoSize | swpNoActivate)
	r1, _, errNo := procSetWindowPos.Call(uintptr(hwnd), hwndTop, 0, 0, 0, 0, flags)
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
