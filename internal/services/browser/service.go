package browser

import (
	"net/url"
	"strings"
	"willchat/internal/errs"

	"github.com/wailsapp/wails/v3/pkg/application"
)

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

// urlpkgParse is a small wrapper for testability and to keep imports clean.
func urlpkgParse(raw string) (*url.URL, error) {
	return url.Parse(raw)
}
