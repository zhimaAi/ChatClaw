package toolchain

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"chatclaw/internal/define"
	"chatclaw/internal/errs"

	"github.com/wailsapp/wails/v3/pkg/application"
)

const (
	ghProxyPrefix = "https://gh-proxy.org/"

	// 获取 OSS 下载链接的 API
	ossAPIURL = "https://api.chatclaw.com/tool-download"

	googleProbeURL     = "https://www.google.com"
	googleProbeTimeout = 3 * time.Second

	downloadTimeout = 5 * time.Minute
)

// 国内可用的 GitHub 代理列表（按优先级排序，会依次尝试直到成功）
var chinaMirrors = []string{
	"https://ghproxy.com/",
	"https://pd.zwc.workers.dev/",
	"https://gh.api.99988866.xyz/",
	"https://gh-proxy.org/",
}

// ToolStatus represents the current state of a managed tool (returned to frontend).
type ToolStatus struct {
	Name             string `json:"name"`
	Installed        bool   `json:"installed"`
	InstalledVersion string `json:"installed_version"`
	LatestVersion    string `json:"latest_version,omitempty"`
	HasUpdate        bool   `json:"has_update"`
	Installing       bool   `json:"installing"`
	BinPath          string `json:"bin_path"`
}

// toolSpec defines how to resolve download URLs and extract a tool binary.
type toolSpec struct {
	name             string
	latestReleaseAPI string
	archiveFormat    func(goos string) string
	// binaryPathInArchive returns the filename to match inside the archive.
	binaryPathInArchive func(goos, goarch string) string
	binaryName          func(goos string) string
	versionArgs         []string
	downloadURL         func(version, goos, goarch string) string
	// aliases returns extra names that should be symlinked (Unix) or copied
	// (Windows) to the main binary after installation. For example, bun
	// needs a "bunx" alias so that "bunx <pkg>" works the same as "bun x <pkg>".
	aliases func(goos string) []string
}

var registry = map[string]toolSpec{
	"uv":    uvSpec(),
	"bun":   bunSpec(),
	"codex": codexSpec(),
}

// ToolchainService manages external tool binaries (uv, bun) within the app data dir.
// It is registered as a Wails service so the frontend can query status and trigger installs.
type ToolchainService struct {
	app    *application.App
	binDir string

	initOnce sync.Once

	mu          sync.Mutex
	installing  map[string]bool // tracks which tools are currently being installed
	proxyProbed bool
	needProxy   bool
}

// NewToolchainService creates a new ToolchainService.
func NewToolchainService(app *application.App) *ToolchainService {
	return &ToolchainService{
		app:        app,
		installing: make(map[string]bool),
	}
}

// BinDir returns the path to the bin directory where tools are installed.
func (s *ToolchainService) BinDir() string {
	s.initOnce.Do(func() {
		cfgDir, err := os.UserConfigDir()
		if err != nil {
			if s.app != nil {
				s.app.Logger.Error("toolchain: failed to get config dir", "error", err)
			}
			return
		}
		s.binDir = filepath.Join(cfgDir, define.AppID, "bin")
	})
	return s.binDir
}

// ---- Frontend-facing API ----

// GetAllToolStatus returns the status of every managed tool.
func (s *ToolchainService) GetAllToolStatus() []ToolStatus {
	result := make([]ToolStatus, 0, len(registry))
	for name := range registry {
		result = append(result, s.getStatus(name))
	}
	return result
}

// GetToolStatus returns the status of a single tool by name.
func (s *ToolchainService) GetToolStatus(name string) (*ToolStatus, error) {
	if _, ok := registry[name]; !ok {
		return nil, errs.Newf("error.toolchain_unknown_tool", map[string]any{"Name": name})
	}
	st := s.getStatus(name)
	return &st, nil
}

// InstallTool installs or updates a single tool by name.
// It runs synchronously — the frontend should call this from an async context.
// Progress events are emitted via "toolchain:status" during the process.
func (s *ToolchainService) InstallTool(name string) (*ToolStatus, error) {
	spec, ok := registry[name]
	if !ok {
		return nil, errs.Newf("error.toolchain_unknown_tool", map[string]any{"Name": name})
	}

	binDir := s.BinDir()
	if binDir == "" {
		return nil, errs.New("error.toolchain_no_bin_dir")
	}
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		return nil, errs.Wrap("error.toolchain_create_dir_failed", err)
	}

	if !s.markInstalling(name) {
		return nil, errs.Newf("error.toolchain_already_installing", map[string]any{"Name": name})
	}
	defer s.clearInstalling(name)
	s.emitStatus(name)

	needProxy := s.resolveProxy()

	latestVersion, err := s.fetchLatestVersion(spec)
	if err != nil {
		return nil, errs.Wrap("error.toolchain_fetch_version_failed", err)
	}

	if err := s.downloadAndInstall(spec, latestVersion, binDir, needProxy); err != nil {
		return nil, errs.Wrap("error.toolchain_install_failed", err)
	}

	s.app.Logger.Info("toolchain: installed successfully",
		"tool", name, "version", latestVersion)

	MarkInstalled(name)
	st := s.getStatus(name)
	s.emitStatus(name)
	return &st, nil
}

// EnsureAll checks and installs/updates all managed tools.
// Can be called from bootstrap (background goroutine) or from the frontend.
func (s *ToolchainService) EnsureAll() {
	binDir := s.BinDir()
	if binDir == "" {
		return
	}
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		s.app.Logger.Error("toolchain: failed to create bin dir", "error", err, "dir", binDir)
		return
	}

	needProxy := s.resolveProxy()

	var wg sync.WaitGroup
	for _, spec := range registry {
		wg.Add(1)
		go func(spec toolSpec) {
			defer wg.Done()
			s.ensureTool(spec, binDir, needProxy)
		}(spec)
	}
	wg.Wait()
	s.app.Logger.Info("toolchain: all tools checked")
	s.syncState()
}

// syncState refreshes the package-level state snapshot from actual binary checks.
func (s *ToolchainService) syncState() {
	binDir := s.BinDir()
	installed := make(map[string]bool, len(registry))
	for name, spec := range registry {
		binPath := filepath.Join(binDir, spec.binaryName(runtime.GOOS))
		if s.getInstalledVersion(binPath, spec.versionArgs) != "" {
			installed[name] = true
		}
	}
	SetState(binDir, installed)
}

// ---- Internal helpers ----

func (s *ToolchainService) getStatus(name string) ToolStatus {
	spec, ok := registry[name]
	if !ok {
		return ToolStatus{Name: name}
	}

	binDir := s.BinDir()
	binName := spec.binaryName(runtime.GOOS)
	binPath := filepath.Join(binDir, binName)
	installed := s.getInstalledVersion(binPath, spec.versionArgs)

	s.mu.Lock()
	isInstalling := s.installing[name]
	s.mu.Unlock()

	return ToolStatus{
		Name:             name,
		Installed:        installed != "",
		InstalledVersion: installed,
		HasUpdate:        false, // populated lazily — checking remote on every call is too slow
		Installing:       isInstalling,
		BinPath:          binPath,
	}
}

func (s *ToolchainService) emitStatus(name string) {
	if s.app != nil {
		s.app.Event.Emit("toolchain:status", s.getStatus(name))
	}
}

func (s *ToolchainService) markInstalling(name string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.installing[name] {
		return false
	}
	s.installing[name] = true
	return true
}

func (s *ToolchainService) clearInstalling(name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.installing, name)
}

// resolveProxy probes connectivity to determine if we need a proxy.
// It checks both direct GitHub access and falls back to proxy if needed.
func (s *ToolchainService) resolveProxy() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.proxyProbed {
		s.needProxy = !isGitHubReachable()
		s.proxyProbed = true
		if s.needProxy {
			s.app.Logger.Info("toolchain: GitHub unreachable, will use mirrors for downloads")
		}
	}
	return s.needProxy
}

// ensureTool installs or updates a single tool.
func (s *ToolchainService) ensureTool(spec toolSpec, binDir string, needProxy bool) {
	binName := spec.binaryName(runtime.GOOS)
	binPath := filepath.Join(binDir, binName)

	installedVersion := s.getInstalledVersion(binPath, spec.versionArgs)
	latestVersion, err := s.fetchLatestVersion(spec)
	if err != nil {
		s.app.Logger.Warn("toolchain: failed to fetch latest version",
			"tool", spec.name, "error", err)
		if installedVersion != "" {
			s.app.Logger.Info("toolchain: keeping existing version",
				"tool", spec.name, "version", installedVersion)
		}
		return
	}

	if installedVersion != "" && installedVersion == latestVersion {
		s.app.Logger.Info("toolchain: already up to date",
			"tool", spec.name, "version", installedVersion)
		return
	}

	if !s.markInstalling(spec.name) {
		s.app.Logger.Info("toolchain: already being installed by another caller",
			"tool", spec.name)
		return
	}
	defer s.clearInstalling(spec.name)
	s.emitStatus(spec.name)

	if installedVersion != "" {
		s.app.Logger.Info("toolchain: updating",
			"tool", spec.name, "from", installedVersion, "to", latestVersion)
	} else {
		s.app.Logger.Info("toolchain: installing",
			"tool", spec.name, "version", latestVersion)
	}

	if err := s.downloadAndInstall(spec, latestVersion, binDir, needProxy); err != nil {
		s.app.Logger.Error("toolchain: install failed",
			"tool", spec.name, "version", latestVersion, "error", err)
		s.emitStatus(spec.name)
		return
	}

	s.app.Logger.Info("toolchain: installed successfully",
		"tool", spec.name, "version", latestVersion, "path", binPath)
	s.emitStatus(spec.name)
}

// getInstalledVersion runs the binary with version args and parses the output.
func (s *ToolchainService) getInstalledVersion(binPath string, versionArgs []string) string {
	if _, err := os.Stat(binPath); os.IsNotExist(err) {
		return ""
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, binPath, versionArgs...)
	hideWindow(cmd)
	out, err := cmd.Output()
	if err != nil {
		return ""
	}

	return extractVersion(strings.TrimSpace(string(out)))
}

// fetchLatestVersion queries the GitHub releases/latest redirect to get the tag.
func (s *ToolchainService) fetchLatestVersion(spec toolSpec) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodHead, spec.latestReleaseAPI, nil)
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusFound && resp.StatusCode != http.StatusMovedPermanently {
		return "", fmt.Errorf("unexpected status %d", resp.StatusCode)
	}

	location := resp.Header.Get("Location")
	if location == "" {
		return "", fmt.Errorf("no Location header in redirect")
	}

	// Location: https://github.com/.../releases/tag/0.10.6 or .../tag/bun-v1.3.9
	parts := strings.Split(location, "/")
	if len(parts) == 0 {
		return "", fmt.Errorf("cannot parse version from Location: %s", location)
	}

	tag := parts[len(parts)-1]
	return extractVersion(tag), nil
}

// downloadAndInstall downloads the archive and extracts the binary.
// It tries multiple mirrors in China if the primary proxy fails,
// and falls back to OSS backup if all mirrors fail.
func (s *ToolchainService) downloadAndInstall(spec toolSpec, version, binDir string, needProxy bool) error {
	goos := runtime.GOOS
	goarch := runtime.GOARCH

	rawURL := spec.downloadURL(version, goos, goarch)

	// 如果需要代理，尝试多个镜像
	if needProxy {
		err := s.tryDownloadWithMirrors(spec, rawURL, binDir)
		if err == nil {
			return nil
		}
		// 所有镜像都失败，尝试直连
		s.app.Logger.Warn("toolchain: all mirrors failed, trying direct connection", "tool", spec.name)
	}

	// 直连尝试
	err := s.downloadWithSingleURL(spec, rawURL, binDir)
	if err == nil {
		return nil
	}

	// 直连也失败，最后尝试 OSS 备份
	s.app.Logger.Warn("toolchain: direct connection failed, trying OSS backup", "tool", spec.name, "error", err)
	return s.downloadFromOSS(spec, version, binDir)
}

// downloadFromOSS downloads from Ali Cloud OSS as final fallback.
// It first fetches the download URL from the API, then downloads.
func (s *ToolchainService) downloadFromOSS(spec toolSpec, version, binDir string) error {
	goos := runtime.GOOS
	goarch := runtime.GOARCH

	// 通过 API 获取 OSS 下载链接
	dlURL, err := s.fetchOSSDownloadURL(spec.name, version, goos, goarch)
	if err != nil {
		s.app.Logger.Error("toolchain: failed to fetch OSS URL", "tool", spec.name, "error", err)
		return fmt.Errorf("fetch OSS URL failed: %w", err)
	}

	s.app.Logger.Info("toolchain: downloading from OSS", "tool", spec.name, "url", dlURL)
	return s.downloadWithSingleURL(spec, dlURL, binDir)
}

// fetchOSSDownloadURL calls the API to get the OSS download URL for a tool.
func (s *ToolchainService) fetchOSSDownloadURL(tool, version, goos, goarch string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// API 请求体
	type Request struct {
		Tool    string `json:"tool"`
		Version string `json:"version"`
		OS      string `json:"os"`
		Arch    string `json:"arch"`
	}

	type Response struct {
		URL string `json:"url"`
	}

	reqBody := Request{
		Tool:    tool,
		Version: version,
		OS:      goos,
		Arch:    goarch,
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, ossAPIURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("API returned status %d", resp.StatusCode)
	}

	var result Response
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}

	if result.URL == "" {
		return "", fmt.Errorf("API returned empty URL")
	}

	return result.URL, nil
}

// tryDownloadWithMirrors tries to download using multiple China mirrors.
func (s *ToolchainService) tryDownloadWithMirrors(spec toolSpec, rawURL, binDir string) error {
	var lastErr error

	// 首先尝试 gh-proxy.org（当前默认）
	dlURL := ghProxyPrefix + rawURL
	if err := s.downloadWithSingleURL(spec, dlURL, binDir); err != nil {
		lastErr = err
		s.app.Logger.Warn("toolchain: primary mirror failed, trying alternatives",
			"tool", spec.name, "mirror", ghProxyPrefix, "error", err)
	} else {
		return nil
	}

	// 尝试其他镜像
	for _, mirror := range chinaMirrors {
		dlURL := mirror + rawURL
		s.app.Logger.Info("toolchain: trying mirror", "tool", spec.name, "mirror", mirror)
		if err := s.downloadWithSingleURL(spec, dlURL, binDir); err != nil {
			lastErr = err
			s.app.Logger.Warn("toolchain: mirror failed",
				"tool", spec.name, "mirror", mirror, "error", err)
			continue
		}
		s.app.Logger.Info("toolchain: mirror worked", "tool", spec.name, "mirror", mirror)
		return nil
	}

	return fmt.Errorf("all mirrors failed: %w", lastErr)
}

// downloadWithSingleURL downloads from a single URL and extracts the binary.
func (s *ToolchainService) downloadWithSingleURL(spec toolSpec, dlURL, binDir string) error {
	s.app.Logger.Info("toolchain: downloading", "tool", spec.name, "url", dlURL)

	ctx, cancel := context.WithTimeout(context.Background(), downloadTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, dlURL, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("download failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download returned status %d", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read body: %w", err)
	}

	format := spec.archiveFormat(runtime.GOOS)
	binName := spec.binaryName(runtime.GOOS)
	destPath := filepath.Join(binDir, binName)

	tmpPath := destPath + ".tmp"
	defer os.Remove(tmpPath)

	switch format {
	case "zip":
		binaryInArchive := spec.binaryPathInArchive(runtime.GOOS, runtime.GOARCH)
		if err := extractFromZip(data, binaryInArchive, tmpPath); err != nil {
			return fmt.Errorf("extract zip: %w", err)
		}
	case "tar.gz":
		binaryInArchive := spec.binaryPathInArchive(runtime.GOOS, runtime.GOARCH)
		if err := extractFromTarGz(data, binaryInArchive, tmpPath); err != nil {
			return fmt.Errorf("extract tar.gz: %w", err)
		}
	case "exe":
		if err := os.WriteFile(tmpPath, data, 0o644); err != nil {
			return fmt.Errorf("write exe: %w", err)
		}
	default:
		return fmt.Errorf("unsupported format: %s", format)
	}

	if err := os.Chmod(tmpPath, 0o755); err != nil {
		return fmt.Errorf("chmod: %w", err)
	}

	if err := os.Rename(tmpPath, destPath); err != nil {
		return fmt.Errorf("rename: %w", err)
	}

	if spec.aliases != nil {
		for _, alias := range spec.aliases(runtime.GOOS) {
			aliasPath := filepath.Join(binDir, alias)
			_ = os.Remove(aliasPath)
			if runtime.GOOS == "windows" {
				copyFile(destPath, aliasPath)
			} else {
				os.Symlink(binName, aliasPath)
			}
		}
	}

	return nil
}

// ---- Archive extraction ----

func extractFromZip(data []byte, targetPath, destPath string) error {
	r, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return err
	}

	for _, f := range r.File {
		if f.Name == targetPath || strings.HasSuffix(f.Name, "/"+targetPath) ||
			filepath.Base(f.Name) == filepath.Base(targetPath) {
			rc, err := f.Open()
			if err != nil {
				return err
			}
			defer rc.Close()

			out, err := os.Create(destPath)
			if err != nil {
				return err
			}
			defer out.Close()

			if _, err := io.Copy(out, rc); err != nil {
				return err
			}
			return nil
		}
	}

	return fmt.Errorf("file %q not found in archive", targetPath)
}

func extractFromTarGz(data []byte, targetPath, destPath string) error {
	gz, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return err
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		if hdr.Name == targetPath || strings.HasSuffix(hdr.Name, "/"+targetPath) ||
			filepath.Base(hdr.Name) == filepath.Base(targetPath) {
			out, err := os.Create(destPath)
			if err != nil {
				return err
			}
			defer out.Close()

			if _, err := io.Copy(out, tr); err != nil {
				return err
			}
			return nil
		}
	}

	return fmt.Errorf("file %q not found in archive", targetPath)
}

// ---- Utilities ----

// copyFile copies src to dst (used on Windows to create binary aliases).
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return os.Chmod(dst, 0o755)
}

// extractVersion normalises a tag/version string: strips leading "v", "bun-v", etc.
func extractVersion(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "bun-v")
	s = strings.TrimPrefix(s, "bun-")
	s = strings.TrimPrefix(s, "uv ")
	s = strings.TrimPrefix(s, "rust-v")
	s = strings.TrimPrefix(s, "codex-cli/")
	s = strings.TrimPrefix(s, "codex-cli ")
	s = strings.TrimPrefix(s, "codex/")
	s = strings.TrimPrefix(s, "v")
	if idx := strings.IndexAny(s, " \t("); idx > 0 {
		s = s[:idx]
	}
	return s
}

func isGitHubReachable() bool {
	ctx, cancel := context.WithTimeout(context.Background(), googleProbeTimeout)
	defer cancel()

	// 直接探测 GitHub API 或 releases 页面
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, "https://github.com", nil)
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
