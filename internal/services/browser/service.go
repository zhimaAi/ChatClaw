package browser

import (
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"chatclaw/internal/errs"

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
	default:
		cmd = exec.Command("xdg-open", dir)
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
		OsType:    getOSType(),
		OsVersion: getOSVersion(),
	}
}

func getOSType() string {
	switch runtime.GOOS {
	case "windows":
		return "Windows"
	case "darwin":
		return "macOS"
	case "linux":
		return "Linux"
	default:
		return runtime.GOOS
	}
}

func getOSVersion() string {
	switch runtime.GOOS {
	case "windows":
		return getWindowsVersion()
	case "darwin":
		return getDarwinVersion()
	case "linux":
		return getLinuxVersion()
	}
	return ""
}

func getWindowsVersion() string {
	// CurrentBuild >= 22000 is Windows 11; else Windows 10
	cmd := exec.Command("powershell", "-NoProfile", "-Command",
		"$b = (Get-ItemProperty 'HKLM:\\SOFTWARE\\Microsoft\\Windows NT\\CurrentVersion').CurrentBuild; if ([int]$b -ge 22000) { '11' } else { '10' }")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func getDarwinVersion() string {
	cmd := exec.Command("sw_vers", "-productVersion")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func getLinuxVersion() string {
	// Prefer /etc/os-release VERSION_ID
	data, err := os.ReadFile("/etc/os-release")
	if err != nil {
		return ""
	}
	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "VERSION_ID=") {
			v := strings.TrimPrefix(line, "VERSION_ID=")
			v = strings.Trim(v, "\"'\n\r\t ")
			return v
		}
	}
	return ""
}

// urlpkgParse is a small wrapper for testability and to keep imports clean.
func urlpkgParse(raw string) (*url.URL, error) {
	return url.Parse(raw)
}
