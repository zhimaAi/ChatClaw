package windows

import (
	"context"
	"runtime"
	"sort"
	"sync"
	"time"

	"willchat/internal/errs"
	"willchat/internal/services/settings"
	"willchat/pkg/winsnap"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

type SnapState string

const (
	SnapStateStopped  SnapState = "stopped"
	SnapStateAttached SnapState = "attached"
	SnapStateHidden   SnapState = "hidden"
)

type SnapStatus struct {
	State           SnapState `json:"state"`
	EnabledKeys     []string  `json:"enabledKeys"`
	EnabledTargets  []string  `json:"enabledTargets"`
	TargetProcess   string    `json:"targetProcess"`
	LastError       string    `json:"lastError,omitempty"`
	UpdatedAtUnixMs int64     `json:"updatedAtUnixMs"`
}

// SnapService manages the single "winsnap" window and dynamically attaches it to
// the top-most window among all enabled target applications.
//
// Rules:
// - Each toggle is independent (not mutually exclusive).
// - Any enabled toggle keeps winsnap window running.
// - Attach to the highest z-order visible window among enabled apps.
// - If no enabled app is visible, move winsnap window off-screen and mark state hidden.
type SnapService struct {
	app    *application.App
	winSvc *WindowService

	mu sync.Mutex

	enabledKeys    []string
	enabledTargets []string

	win       *application.WebviewWindow
	readyOnce sync.Once
	readyCh   chan struct{}

	ctrl          winsnap.Controller
	currentTarget string

	loopCancel context.CancelFunc

	status SnapStatus
}

func NewSnapService(app *application.App, winSvc *WindowService) (*SnapService, error) {
	if app == nil {
		return nil, errs.New("error.app_required")
	}
	if winSvc == nil {
		return nil, errs.New("error.window_service_required")
	}
	s := &SnapService{
		app:    app,
		winSvc: winSvc,
		readyCh: func() chan struct{} {
			ch := make(chan struct{})
			return ch
		}(),
		status: SnapStatus{
			State: SnapStateStopped,
		},
	}
	return s, nil
}

func (s *SnapService) GetStatus() SnapStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.status
}

// WakeAttached brings the attached target window and the winsnap window to the front.
// This is useful when winsnap is attached but both windows are behind other apps.
func (s *SnapService) WakeAttached() error {
	s.mu.Lock()
	w := s.win
	target := s.currentTarget
	s.mu.Unlock()

	if w == nil || target == "" {
		return nil
	}
	return winsnap.WakeAttachedWindow(w, target)
}

// SyncFromSettings reads snap toggles from settings cache, then starts/stops
// snapping loop accordingly.
func (s *SnapService) SyncFromSettings() (SnapStatus, error) {
	enabledKeys, enabledTargets := readSnapTargetsFromSettings()

	s.mu.Lock()
	s.enabledKeys = enabledKeys
	s.enabledTargets = enabledTargets
	s.status.EnabledKeys = append([]string(nil), enabledKeys...)
	s.status.EnabledTargets = append([]string(nil), enabledTargets...)
	s.status.LastError = ""
	s.touchLocked("")
	shouldRun := len(enabledTargets) > 0
	s.mu.Unlock()

	if !shouldRun {
		_ = s.stop()
		return s.GetStatus(), nil
	}

	if err := s.ensureRunning(); err != nil {
		return s.GetStatus(), err
	}
	return s.GetStatus(), nil
}

func (s *SnapService) ensureRunning() error {
	// Ensure window exists & shown (do not focus).
	if err := s.winSvc.Show(WindowWinsnap); err != nil {
		return err
	}

	s.mu.Lock()
	// We can safely call ensure here because we're in the same package; do NOT expose
	// any *application.WebviewWindow types to the frontend bindings.
	w, err := s.winSvc.ensure(WindowWinsnap)
	if err != nil {
		s.mu.Unlock()
		return err
	}
	s.installWindowHooksLocked(w)

	alreadyLooping := s.loopCancel != nil
	if alreadyLooping {
		s.mu.Unlock()
		return nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	s.loopCancel = cancel
	s.mu.Unlock()

	go s.loop(ctx)
	return nil
}

func (s *SnapService) stop() error {
	s.mu.Lock()
	cancel := s.loopCancel
	s.loopCancel = nil
	s.mu.Unlock()
	if cancel != nil {
		cancel()
	}

	s.mu.Lock()
	if s.ctrl != nil {
		_ = s.ctrl.Stop()
		s.ctrl = nil
	}
	s.currentTarget = ""
	s.status.State = SnapStateStopped
	s.status.TargetProcess = ""
	s.status.LastError = ""
	s.touchLocked("")
	s.mu.Unlock()

	// Close winsnap window when fully stopped (no toggles enabled).
	_ = s.winSvc.Close(WindowWinsnap)
	return nil
}

func (s *SnapService) loop(ctx context.Context) {
	// Wait until the winsnap window is ready (or at least shown) to ensure a valid native handle.
	s.mu.Lock()
	readyCh := s.readyCh
	s.mu.Unlock()
	select {
	case <-readyCh:
	case <-ctx.Done():
		return
	}

	// Run immediately then on a ticker.
	s.step()
	ticker := time.NewTicker(400 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			s.step()
		case <-ctx.Done():
			return
		}
	}
}

func (s *SnapService) step() {
	s.mu.Lock()
	enabledTargets := append([]string(nil), s.enabledTargets...)
	w := s.win
	s.mu.Unlock()

	if len(enabledTargets) == 0 {
		// If toggles became empty while loop is still running, stop.
		_ = s.stop()
		return
	}
	if w == nil {
		// Window got closed; re-create.
		if err := s.ensureRunning(); err != nil {
			s.mu.Lock()
			s.status.LastError = err.Error()
			s.touchLocked(err.Error())
			s.mu.Unlock()
		}
		return
	}

	target, found, err := winsnap.TopMostVisibleProcessName(enabledTargets)
	if err != nil {
		// Non-windows: snapping is not supported.
		s.mu.Lock()
		s.status.LastError = err.Error()
		s.touchLocked(err.Error())
		s.mu.Unlock()
		return
	}

	if !found || target == "" {
		s.hideOffscreen(w)
		return
	}
	s.attachTo(w, target)
}

func (s *SnapService) hideOffscreen(w *application.WebviewWindow) {
	s.mu.Lock()
	// Stop any existing controller first.
	if s.ctrl != nil {
		_ = s.ctrl.Stop()
		s.ctrl = nil
	}
	s.currentTarget = ""
	s.status.State = SnapStateHidden
	s.status.TargetProcess = ""
	s.status.LastError = ""
	s.touchLocked("")
	s.mu.Unlock()

	_ = winsnap.MoveOffscreen(w)
}

func (s *SnapService) attachTo(w *application.WebviewWindow, targetProcess string) {
	s.mu.Lock()
	if s.currentTarget == targetProcess && s.ctrl != nil {
		s.mu.Unlock()
		return
	}
	// Stop current controller (if any) before switching target.
	if s.ctrl != nil {
		_ = s.ctrl.Stop()
		s.ctrl = nil
	}
	s.mu.Unlock()

	c, err := winsnap.AttachRightOfProcess(winsnap.AttachOptions{
		TargetProcessName: targetProcess,
		Gap:               0,
		FindTimeout:       800 * time.Millisecond,
		App:               s.app,
		Window:            w,
	})
	if err != nil {
		// Treat as temporarily hidden; next tick will retry.
		s.mu.Lock()
		s.status.LastError = err.Error()
		s.touchLocked(err.Error())
		s.mu.Unlock()
		return
	}

	s.mu.Lock()
	s.ctrl = c
	s.currentTarget = targetProcess
	s.status.State = SnapStateAttached
	s.status.TargetProcess = targetProcess
	s.status.LastError = ""
	s.touchLocked("")
	s.mu.Unlock()
}

func (s *SnapService) installWindowHooksLocked(w *application.WebviewWindow) {
	if s.win == w && s.readyCh != nil {
		// Already installed for this window instance.
		return
	}

	s.win = w
	s.readyOnce = sync.Once{}
	s.readyCh = make(chan struct{})

	w.OnWindowEvent(events.Common.WindowRuntimeReady, func(_ *application.WindowEvent) {
		s.mu.Lock()
		defer s.mu.Unlock()
		if s.win != w {
			return
		}
		s.readyOnce.Do(func() { close(s.readyCh) })
	})
	w.OnWindowEvent(events.Common.WindowShow, func(_ *application.WindowEvent) {
		s.mu.Lock()
		defer s.mu.Unlock()
		if s.win != w {
			return
		}
		s.readyOnce.Do(func() { close(s.readyCh) })
	})
	w.OnWindowEvent(events.Common.WindowClosing, func(_ *application.WindowEvent) {
		s.mu.Lock()
		if s.ctrl != nil {
			_ = s.ctrl.Stop()
			s.ctrl = nil
		}
		if s.win == w {
			s.win = nil
		}
		s.readyOnce.Do(func() { close(s.readyCh) })
		s.mu.Unlock()
	})
}

func (s *SnapService) touchLocked(lastErr string) {
	s.status.UpdatedAtUnixMs = time.Now().UnixMilli()
	if lastErr != "" {
		s.status.LastError = lastErr
	}
}

func readSnapTargetsFromSettings() (enabledKeys []string, enabledTargets []string) {
	type item struct {
		key     string
		targets []string
	}

	// Multi-target per key to support different executables across versions.
	items := []item{
		{key: "snap_wechat", targets: snapTargetsForKey("snap_wechat")},
		{key: "snap_wecom", targets: snapTargetsForKey("snap_wecom")},
		{key: "snap_qq", targets: snapTargetsForKey("snap_qq")},
		{key: "snap_dingtalk", targets: snapTargetsForKey("snap_dingtalk")},
		{key: "snap_feishu", targets: snapTargetsForKey("snap_feishu")},
		{key: "snap_douyin", targets: snapTargetsForKey("snap_douyin")},
	}

	for _, it := range items {
		if !settings.GetBool(it.key, false) {
			continue
		}
		enabledKeys = append(enabledKeys, it.key)
		enabledTargets = append(enabledTargets, it.targets...)
	}

	// Stable ordering for status/debugging.
	sort.Strings(enabledKeys)
	enabledTargets = uniqueStrings(enabledTargets)
	sort.Strings(enabledTargets)
	return enabledKeys, enabledTargets
}

func uniqueStrings(in []string) []string {
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		if s == "" {
			continue
		}
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}

func snapTargetsForKey(key string) []string {
	// Windows focus for now; macOS names are best-effort.
	switch runtime.GOOS {
	case "windows":
		switch key {
		case "snap_wechat":
			// WeChat process name differs across versions/channels.
			// NOTE: Windows matching is based on process image name (exe). "Weixin.exe" is used by some channels.
			return []string{"Weixin.exe", "WeChat.exe", "WeChatApp.exe", "WeChatAppEx.exe"}
		case "snap_wecom":
			return []string{"WXWork.exe"}
		case "snap_qq":
			return []string{"QQ.exe", "QQNT.exe"}
		case "snap_dingtalk":
			return []string{"DingTalk.exe"}
		case "snap_feishu":
			return []string{"Feishu.exe", "Lark.exe"}
		case "snap_douyin":
			return []string{"Douyin.exe"}
		default:
			return nil
		}
	default:
		// macOS / others: use app localized names where possible.
		switch key {
		case "snap_wechat":
			// macOS: localized name / executable / common aliases / bundle id.
			return []string{"微信", "Weixin", "weixin", "WeChat", "wechat", "com.tencent.xinWeChat"}
		case "snap_wecom":
			return []string{"企业微信", "WeCom", "wecom", "WeWork", "wework", "WXWork", "wxwork", "qiyeweixin", "com.tencent.WeWorkMac"}
		case "snap_qq":
			return []string{"QQ", "qq", "com.tencent.qq"}
		case "snap_dingtalk":
			return []string{"钉钉", "DingTalk", "dingtalk", "com.alibaba.DingTalkMac"}
		case "snap_feishu":
			return []string{"飞书", "Feishu", "feishu", "Lark", "lark", "com.bytedance.feishu", "com.bytedance.Lark"}
		case "snap_douyin":
			return []string{"抖音", "Douyin", "douyin"}
		default:
			return nil
		}
	}
}
