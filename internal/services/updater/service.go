package updater

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	selfupdate "github.com/creativeprojects/go-selfupdate"
	"github.com/wailsapp/wails/v3/pkg/application"

	"willclaw/internal/define"
	"willclaw/internal/errs"
	"willclaw/internal/services/settings"
)

const (
	// GitHub repository slug for WillClaw
	repoOwner = "zhimaAi"
	repoName  = "WillClaw"

	// Google reachability probe: if Google is accessible, the network is
	// not behind the GFW, so we can safely use GitHub for downloads.
	googleProbeURL     = "https://www.google.com"
	googleProbeTimeout = 3 * time.Second
)

// UpdateInfo is the result returned to the frontend.
type UpdateInfo struct {
	HasUpdate      bool   `json:"has_update"`
	CurrentVersion string `json:"current_version"`
	LatestVersion  string `json:"latest_version"`
	ReleaseNotes   string `json:"release_notes"`
	AssetSize      int    `json:"asset_size"`
}

// UpdaterService provides auto-update functionality exposed to the frontend.
type UpdaterService struct {
	app *application.App

	mu     sync.Mutex
	latest *selfupdate.Release // cached latest release after CheckForUpdate
	source selfupdate.Source   // cached source selected during CheckForUpdate
}

// NewUpdaterService creates a new updater service.
func NewUpdaterService(app *application.App) *UpdaterService {
	return &UpdaterService{app: app}
}

// ServiceStartup is called by Wails after the application starts.
// It always schedules a background update check so the frontend can show
// a badge on the "Check for Update" button. The auto_update setting only
// controls whether the update dialog is shown automatically.
func (s *UpdaterService) ServiceStartup(ctx context.Context, options application.ServiceOptions) error {
	// Clean up leftover .old binary from a previous update (Windows leaves it behind
	// because the old process was still running when the update was applied).
	s.cleanupOldBinary()

	go func() {
		time.Sleep(3 * time.Second)

		info, err := s.CheckForUpdate()
		if err != nil {
			s.app.Logger.Warn("auto update check failed", "error", err)
			return
		}
		if info != nil && info.HasUpdate {
			s.app.Logger.Info("new version available", "version", info.LatestVersion)
			// Always emit the event so the frontend can display an update badge.
			// The frontend decides whether to auto-show the dialog based on
			// the auto_update setting.
			s.app.Event.Emit("update:available", *info)
		}
	}()
	return nil
}

// cleanupOldBinary removes .old files left behind by a previous update.
// On Windows, the running process cannot delete itself, so the library renames
// the old binary to .<name>.old and hides it. We clean it up on next launch
// when the file is no longer locked.
//
// IMPORTANT: We use selfupdate.ExecutablePath() (not os.Executable()) because
// go-selfupdate resolves the exe path via GetFinalPathNameByHandle on Windows,
// which may return a different casing/path than os.Executable(). The .old file
// lives next to the resolved path, so we must use the same resolution.
func (s *UpdaterService) cleanupOldBinary() {
	logToFile := newFileLogger()

	// Use the same path resolution as go-selfupdate so we look in the right directory.
	exe, err := selfupdate.ExecutablePath()
	if err != nil {
		logToFile("[cleanup] selfupdate.ExecutablePath failed: %v, falling back to os.Executable", err)
		exe, err = os.Executable()
		if err != nil {
			logToFile("[cleanup] os.Executable also failed: %v", err)
			return
		}
	}

	dir := filepath.Dir(exe)
	name := filepath.Base(exe)

	// go-selfupdate uses the pattern ".<name>.old" for the backup file
	oldPath := filepath.Join(dir, "."+name+".old")
	logToFile("[cleanup] looking for old binary at: %s", oldPath)

	if err := os.Remove(oldPath); err != nil {
		if os.IsNotExist(err) {
			logToFile("[cleanup] no old binary found (OK)")
		} else {
			logToFile("[cleanup] remove failed: %v", err)
		}
	} else {
		logToFile("[cleanup] removed old binary: %s", oldPath)
	}
}

// newFileLogger returns a function that appends log lines to the willclaw.log
// file. This bypasses slog (which does not write to disk in production).
func newFileLogger() func(format string, args ...any) {
	cfgDir, err := os.UserConfigDir()
	if err != nil {
		return func(string, ...any) {}
	}
	logPath := filepath.Join(cfgDir, define.AppID, "willclaw.log")
	return func(format string, args ...any) {
		f, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
		if err != nil {
			return
		}
		defer f.Close()
		fmt.Fprintf(f, format+"\n", args...)
	}
}

// CheckForUpdate checks if a newer version is available.
// It tries GitHub first, then falls back to Gitee if GitHub is unreachable.
func (s *UpdaterService) CheckForUpdate() (*UpdateInfo, error) {
	currentVersion := define.Version
	if currentVersion == "" || currentVersion == "dev" {
		return &UpdateInfo{
			HasUpdate:      false,
			CurrentVersion: currentVersion,
			LatestVersion:  currentVersion,
		}, nil
	}

	source, err := s.selectSource()
	if err != nil {
		return nil, errs.Wrap("error.update_source_failed", err)
	}

	updater, err := selfupdate.NewUpdater(selfupdate.Config{
		Source: source,
	})
	if err != nil {
		return nil, errs.Wrap("error.update_init_failed", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	latest, found, err := updater.DetectLatest(ctx, selfupdate.NewRepositorySlug(repoOwner, repoName))
	if err != nil {
		return nil, errs.Wrap("error.update_check_failed", err)
	}

	if !found || latest.LessOrEqual(currentVersion) {
		return &UpdateInfo{
			HasUpdate:      false,
			CurrentVersion: currentVersion,
			LatestVersion:  currentVersion,
		}, nil
	}

	// Cache the latest release and source for DownloadAndApply,
	// so that the same source is used for both check and download.
	s.mu.Lock()
	s.latest = latest
	s.source = source
	s.mu.Unlock()

	return &UpdateInfo{
		HasUpdate:      true,
		CurrentVersion: currentVersion,
		LatestVersion:  latest.Version(),
		ReleaseNotes:   latest.ReleaseNotes,
		AssetSize:      latest.AssetByteSize,
	}, nil
}

// DownloadAndApply downloads the latest release and replaces the current binary.
// Must call CheckForUpdate first to detect the latest release.
func (s *UpdaterService) DownloadAndApply() error {
	s.mu.Lock()
	latest := s.latest
	source := s.source
	s.mu.Unlock()

	if latest == nil {
		return errs.New("error.update_no_release")
	}
	if source == nil {
		return errs.New("error.update_no_source")
	}

	updater, err := selfupdate.NewUpdater(selfupdate.Config{
		Source: source,
	})
	if err != nil {
		return errs.Wrap("error.update_init_failed", err)
	}

	exe, err := selfupdate.ExecutablePath()
	if err != nil {
		return errs.Wrap("error.update_exe_path_failed", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	if err := updater.UpdateTo(ctx, latest, exe); err != nil {
		return errs.Wrap("error.update_apply_failed", err)
	}

	// Persist version + release notes so the next launch can show "just updated" dialog
	settingsSvc := settings.NewSettingsService(s.app)
	if _, err := settingsSvc.SetValue("pending_update_version", latest.Version()); err != nil {
		s.app.Logger.Warn("failed to persist pending_update_version", "error", err)
	}
	if _, err := settingsSvc.SetValue("pending_update_notes", latest.ReleaseNotes); err != nil {
		s.app.Logger.Warn("failed to persist pending_update_notes", "error", err)
	}

	s.app.Logger.Info("update applied successfully", "version", latest.Version())
	return nil
}

// GetPendingUpdate returns update info persisted by the previous DownloadAndApply call,
// then clears the persisted data. Returns nil if no pending update exists.
// This is called by the frontend on startup to show the "just updated" dialog.
func (s *UpdaterService) GetPendingUpdate() *UpdateInfo {
	version, _ := settings.GetValue("pending_update_version")
	notes, _ := settings.GetValue("pending_update_notes")

	if version == "" {
		return nil
	}

	// Clear the persisted data so it only shows once
	settingsSvc := settings.NewSettingsService(s.app)
	if _, err := settingsSvc.SetValue("pending_update_version", ""); err != nil {
		s.app.Logger.Warn("failed to clear pending_update_version", "error", err)
	}
	if _, err := settingsSvc.SetValue("pending_update_notes", ""); err != nil {
		s.app.Logger.Warn("failed to clear pending_update_notes", "error", err)
	}

	return &UpdateInfo{
		HasUpdate:      false,
		CurrentVersion: define.Version,
		LatestVersion:  version,
		ReleaseNotes:   notes,
	}
}

// RestartApp launches a new instance of the application and exits the current one.
//
// On Windows, the application uses SingleInstance mode which prevents a second
// process from running while the first is still alive. Therefore we must quit
// the current process FIRST and use a shell-delayed launch (ping localhost to
// wait ~2s) so the new process starts only after this one has fully exited.
func (s *UpdaterService) RestartApp() error {
	exe, err := os.Executable()
	if err != nil {
		return errs.Wrap("error.update_restart_failed", err)
	}

	switch runtime.GOOS {
	case "darwin":
		exe, err = filepath.EvalSymlinks(exe)
		if err != nil {
			return errs.Wrap("error.update_restart_failed", err)
		}
		// On macOS, use "open" to launch the .app bundle.
		// exe is like /Applications/WillClaw.app/Contents/MacOS/WillClaw
		appPath := exe
		for i := 0; i < 3; i++ {
			appPath = filepath.Dir(appPath)
		}

		var cmd *exec.Cmd
		if filepath.Ext(appPath) == ".app" {
			cmd = exec.Command("open", "-n", appPath)
		} else {
			cmd = exec.Command(exe)
		}

		s.app.Logger.Info("restarting application", "exe", exe, "cmd", cmd.Args)
		if err := cmd.Start(); err != nil {
			return errs.Wrap("error.update_restart_failed", err)
		}

		go func() {
			time.Sleep(1 * time.Second)
			s.app.Quit()
		}()

	case "windows":
		// On Windows with SingleInstance, we cannot start the new process while
		// the current one is still running — it would be rejected as a second
		// instance. Write a temporary .bat script that waits ~2s (via ping)
		// then launches the new exe. Using a .bat file avoids all Go/cmd.exe
		// argument quoting issues.
		batPath := filepath.Join(os.TempDir(), "willclaw_restart.bat")
		batContent := fmt.Sprintf("@echo off\r\nping localhost -n 3 >nul\r\nstart \"\" \"%s\"\r\ndel \"%%~f0\"\r\n", exe)
		if err := os.WriteFile(batPath, []byte(batContent), 0o644); err != nil {
			return errs.Wrap("error.update_restart_failed", err)
		}

		s.app.Logger.Info("restarting application (windows delayed launch)", "exe", exe, "bat", batPath)

		cmd := exec.Command("cmd", "/C", batPath)
		setDetachedProcess(cmd)

		if err := cmd.Start(); err != nil {
			return errs.Wrap("error.update_restart_failed", err)
		}

		// Quit immediately so the SingleInstance lock is released before
		// the delayed bat script fires.
		go func() {
			time.Sleep(200 * time.Millisecond)
			s.app.Quit()
		}()

	default:
		exe, err = filepath.EvalSymlinks(exe)
		if err != nil {
			return errs.Wrap("error.update_restart_failed", err)
		}
		cmd := exec.Command(exe)

		s.app.Logger.Info("restarting application", "exe", exe, "cmd", cmd.Args)
		if err := cmd.Start(); err != nil {
			return errs.Wrap("error.update_restart_failed", err)
		}

		go func() {
			time.Sleep(1 * time.Second)
			s.app.Quit()
		}()
	}

	return nil
}

// selectSource probes Google to decide whether the network can reach
// international sites; if yes it uses GitHub, otherwise falls back to Gitee.
func (s *UpdaterService) selectSource() (selfupdate.Source, error) {
	if isGoogleReachable() {
		s.app.Logger.Debug("Google reachable, using GitHub as update source")
		return selfupdate.NewGitHubSource(selfupdate.GitHubConfig{})
	}

	s.app.Logger.Info("Google unreachable, using Gitee as update source")
	return NewGiteeSource(), nil
}

// isGoogleReachable checks if Google is accessible within the probe timeout.
// When Google is reachable the user is on an unrestricted network, so GitHub
// downloads will also work. This is a stricter check than probing GitHub API
// directly, because GitHub API may respond while release asset downloads are
// still throttled or blocked.
func isGoogleReachable() bool {
	ctx, cancel := context.WithTimeout(context.Background(), googleProbeTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodHead, googleProbeURL, nil)
	if err != nil {
		return false
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode < 500
}

// GetRepoSlug returns the repository slug string (for display purposes).
func GetRepoSlug() string {
	return fmt.Sprintf("%s/%s", repoOwner, repoName)
}
