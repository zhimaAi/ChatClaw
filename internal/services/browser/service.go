package browser

import (
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"chatclaw/internal/errs"
	"chatclaw/internal/sysinfo"

	"github.com/wailsapp/wails/v3/pkg/application"
)


// LoginParams holds query parameters for the ChatClaw login page (e.g. /chatclaw/login?os_type=...).
type LoginParams struct {
	OsType    string `json:"os_type"`    // e.g. "Windows", "macOS", "Linux"
	OsVersion string `json:"os_version"` // e.g. "11", "14.2.1", best-effort
}

// BrowserService 浏览器服务（暴露给前端调用）
type BrowserService struct {
	app *application.App
}

func NewBrowserService(app *application.App) *BrowserService {
	return &BrowserService{app: app}
}

// OpenURL 在系统默认浏览器中打开 URL
func (s *BrowserService) OpenURL(url string) error {
	u := strings.TrimSpace(url)
	if u == "" {
		return errs.New("error.browser_url_required")
	}

	parsed, err := urlpkgParse(u)
	if err != nil || parsed == nil {
		return errs.New("error.browser_invalid_url")
	}
	scheme := strings.ToLower(parsed.Scheme)
	if scheme != "http" && scheme != "https" {
		return errs.New("error.browser_unsupported_url_scheme")
	}

	// On Windows, use custom impl to avoid flashing cmd window; Wails app.Browser.OpenURL spawns cmd.
	if runtime.GOOS == "windows" {
		if err := openURLWindows(u); err != nil {
			return errs.Wrap("error.browser_open_failed", err)
		}
		return nil
	}
	if err := s.app.Browser.OpenURL(u); err != nil {
		return errs.Wrap("error.browser_open_failed", err)
	}
	return nil
}

// OpenDirectory opens a directory in the system file manager.
// It creates the directory if it doesn't exist.
func (s *BrowserService) OpenDirectory(dir string) error {
	dir = strings.TrimSpace(dir)
	if dir == "" {
		return errs.New("error.browser_url_required")
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return errs.Wrap("error.browser_open_failed", err)
	}

	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", dir)
	case "windows":
		cmd = exec.Command("explorer", dir)
		setCmdHideWindow(cmd)
	default:
		cmd = exec.Command("xdg-open", dir)
	}

	if err := cmd.Start(); err != nil {
		return errs.Wrap("error.browser_open_failed", err)
	}
	return nil
}

// OpenFile opens a file with the system's default application.
func (s *BrowserService) OpenFile(filePath string) error {
	filePath = strings.TrimSpace(filePath)
	if filePath == "" {
		return errs.New("error.browser_url_required")
	}

	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return errs.New("error.file_not_found")
	}

	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", filePath)
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", "", filePath)
		setCmdHideWindow(cmd)
	default:
		cmd = exec.Command("xdg-open", filePath)
	}

	if err := cmd.Start(); err != nil {
		return errs.Wrap("error.browser_open_failed", err)
	}
	return nil
}

// GetLoginParams returns os_type and os_version for appending to the login URL.
// Used when redirecting to /chatclaw/login from account management.
func (s *BrowserService) GetLoginParams() LoginParams {
	return LoginParams{
		OsType:    sysinfo.OSType(),
		OsVersion: sysinfo.OSVersion(),
	}
}

// urlpkgParse is a small wrapper for testability and to keep imports clean.
func urlpkgParse(raw string) (*url.URL, error) {
	return url.Parse(raw)
}
