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
func (s *UpdaterService) ServiceStartup(ctx context.Context, options application.ServiceOptions) error {
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

// cleanupOldBinary removes the .<exe>.old backup left by go-selfupdate.
//
// On Windows the old binary is hidden and its dot-prefixed name cannot be
// accessed via absolute paths (Win32 treats "\.X" as a relative reference).
// We work around this by running a bat script that cd's into the directory
// and uses the relative filename.
func (s *UpdaterService) cleanupOldBinary() {
	exe, err := os.Executable()
	if err != nil {
		return
	}

	dir := filepath.Dir(exe)
	oldName := "." + filepath.Base(exe) + ".old"

	if runtime.GOOS == "windows" {
		// Only run the cleanup bat if an .old file likely exists; this avoids
		// spawning a cmd.exe process (which may flash a console window) on
		// every normal startup.
		// We cannot stat the dot-prefixed file directly (Win32 path bug), so
		// we check via a quick "dir /A /B" in the exe directory.
		probe := exec.Command("cmd", "/C", fmt.Sprintf(`cd /D "%s" & dir /A /B "%s"`, dir, oldName))
		setDetachedProcess(probe)
		if probe.Run() != nil {
			return // file not found — nothing to clean
		}

		script := fmt.Sprintf(
			"@echo off\r\ncd /D \"%s\"\r\nattrib -H \"%s\" >nul 2>&1\r\ndel /F \"%s\" >nul 2>&1\r\n",
			dir, oldName, oldName,
		)
		batPath := filepath.Join(os.TempDir(), "willclaw_cleanup.bat")
		if err := os.WriteFile(batPath, []byte(script), 0o644); err != nil {
			return
		}
		cmd := exec.Command("cmd", "/C", batPath)
		setDetachedProcess(cmd)
		_ = cmd.Run()
		_ = os.Remove(batPath)
	} else {
		oldPath := filepath.Join(dir, oldName)
		_ = os.Remove(oldPath)
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

// GetPendingUpdate returns and clears update info persisted by DownloadAndApply.
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

	// Windows needs special handling: SingleInstance rejects a second process,
	// and the .old binary cleanup requires cd + relative path. Use a bat script
	// that waits for this process to exit, cleans up, then launches the new exe.
	if runtime.GOOS == "windows" {
		exeDir := filepath.Dir(exe)
		exeName := filepath.Base(exe)
		oldName := "." + exeName + ".old"
		batPath := filepath.Join(os.TempDir(), "willclaw_restart.bat")
		batContent := fmt.Sprintf(
			"@echo off\r\n"+
				"ping localhost -n 3 >nul\r\n"+
				"cd /D \"%s\"\r\n"+
				"attrib -H \"%s\" >nul 2>&1\r\n"+
				"del /F \"%s\" >nul 2>&1\r\n"+
				"start \"\" \"%s\"\r\n"+
				"del \"%%~f0\"\r\n",
			exeDir, oldName, oldName, exe,
		)
		if err := os.WriteFile(batPath, []byte(batContent), 0o644); err != nil {
			return errs.Wrap("error.update_restart_failed", err)
		}

		cmd := exec.Command("cmd", "/C", batPath)
		setDetachedProcess(cmd)

		if err := cmd.Start(); err != nil {
			return errs.Wrap("error.update_restart_failed", err)
		}

		go func() {
			time.Sleep(200 * time.Millisecond)
			s.app.Quit()
		}()

		return nil
	}

	// macOS / Linux: keep the original logic exactly as-is.
	exe, err = filepath.EvalSymlinks(exe)
	if err != nil {
		return errs.Wrap("error.update_restart_failed", err)
	}

	var cmd *exec.Cmd

	switch runtime.GOOS {
	case "darwin":
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
