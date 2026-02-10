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
// It schedules a background update check if auto_update is enabled.
func (s *UpdaterService) ServiceStartup(ctx context.Context, options application.ServiceOptions) error {
	go func() {
		time.Sleep(3 * time.Second)

		if !settings.GetBool("auto_update", true) {
			return
		}
		info, err := s.CheckForUpdate()
		if err != nil {
			s.app.Logger.Warn("auto update check failed", "error", err)
			return
		}
		if info != nil && info.HasUpdate {
			s.app.Logger.Info("new version available", "version", info.LatestVersion)
			s.app.Event.Emit("update:available", *info)
		}
	}()
	return nil
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
func (s *UpdaterService) RestartApp() error {
	exe, err := os.Executable()
	if err != nil {
		return errs.Wrap("error.update_restart_failed", err)
	}
	exe, err = filepath.EvalSymlinks(exe)
	if err != nil {
		return errs.Wrap("error.update_restart_failed", err)
	}

	var cmd *exec.Cmd

	switch runtime.GOOS {
	case "darwin":
		// On macOS, use "open" to launch the .app bundle
		// exe is like /Applications/WillClaw.app/Contents/MacOS/WillClaw
		appPath := exe
		for i := 0; i < 3; i++ {
			appPath = filepath.Dir(appPath)
		}
		if filepath.Ext(appPath) == ".app" {
			cmd = exec.Command("open", "-n", appPath)
		} else {
			cmd = exec.Command(exe)
		}
	default:
		cmd = exec.Command(exe)
	}

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return errs.Wrap("error.update_restart_failed", err)
	}

	// Give the new process a moment to start, then exit
	go func() {
		time.Sleep(500 * time.Millisecond)
		os.Exit(0)
	}()

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
