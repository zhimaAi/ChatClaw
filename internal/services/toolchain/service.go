package toolchain

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
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

	googleProbeURL     = "https://github.com"
	googleProbeTimeout = 3 * time.Second

	downloadTimeout = 50 * time.Minute
)

// 国内可用的 GitHub 下载代理列表（按优先级排序）
// 这些代理用于下载文件，代理方式是：在原始下载链接前加代理域名
var downloadProxies = []string{
	"https://gh-proxy.org/",         // gh-proxy.org (主推荐)
	"https://hk.gh-proxy.org/",      // 香港线路
	"https://cdn.gh-proxy.org/",     // CDN 加速
	"https://edgeone.gh-proxy.org/", // Edgeone 加速
}

// 国内可用的 GitHub API 镜像列表（用于版本检测）
// 这些镜像用于访问 GitHub 页面，代理方式是将 github.com 域名替换为镜像域名
var apiMirrors = []string{
	"https://bgithub.xyz", // bgithub.xyz (主推荐，不带末尾斜杠)
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

	// testInstall 用于测试安装的上下文（支持取消）
	testInstallCtx    map[string]context.CancelFunc // tool name -> cancel function
	testInstallCalled map[string]bool               // tool name -> 是否已调用过（用于触发进度事件）
}

// NewToolchainService creates a new ToolchainService.
func NewToolchainService(app *application.App) *ToolchainService {
	// 注意：installing map 初始为空，启动时会自动清除任何残留状态
	// 因为这是内存中的状态，程序重启后本来就是空的
	return &ToolchainService{
		app:               app,
		installing:        make(map[string]bool),
		testInstallCtx:    make(map[string]context.CancelFunc),
		testInstallCalled: make(map[string]bool),
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

	// 优先尝试从 GitHub 获取最新版本
	latestVersion, err := s.fetchLatestVersion(spec)
	if err != nil {
		return nil, errs.Wrap("error.toolchain_fetch_version_failed", err)
	}

	// 优先尝试从 GitHub 直连/代理下载，失败后尝试 API/OSS 兜底
	if err := s.downloadAndInstall(spec, latestVersion, binDir, needProxy); err != nil {
		// GitHub 下载失败，尝试 API/OSS 兜底
		s.app.Logger.Warn("toolchain: GitHub download failed, trying API/OSS", "tool", spec.name, "error", err)
		ossURL, ossErr := s.fetchLatestDownloadURL(spec.name, runtime.GOOS, runtime.GOARCH)
		if ossErr != nil {
			return nil, errs.Wrap("error.toolchain_install_failed", fmt.Errorf("GitHub failed: %w, API failed: %w", err, ossErr))
		}
		if err := s.downloadFromOSSURL(spec, ossURL); err != nil {
			return nil, errs.Wrap("error.toolchain_install_failed", err)
		}
		s.app.Logger.Info("toolchain: installed successfully via OSS fallback", "tool", name)
	} else {
		s.app.Logger.Info("toolchain: installed successfully", "tool", name, "version", latestVersion)
	}

	MarkInstalled(name)
	st := s.getStatus(name)
	s.emitStatus(name)
	return &st, nil
}

// DownloadMethod 下载方式
type DownloadMethod string

const (
	DownloadMethodDirect DownloadMethod = "direct" // 直连
	DownloadMethodProxy  DownloadMethod = "proxy"  // 代理
	DownloadMethodOSS    DownloadMethod = "oss"    // OSS 兜底
)

// TestInstallConfig 测试安装配置（从前端传入）
type TestInstallConfig struct {
	Tool           string         `json:"tool"`           // 工具名称
	DownloadMethod DownloadMethod `json:"downloadMethod"` // 下载方式
	ProxyURL       string         `json:"proxyURL"`       // 代理服务器地址（当 downloadMethod 为 proxy 时）
}

// TestInstallResult 测试安装结果
type TestInstallResult struct {
	Success     bool   `json:"success"`
	Message     string `json:"message"`
	Tool        string `json:"tool"`
	Version     string `json:"version"`
	DownloadURL string `json:"downloadURL"`
	TotalSize   int64  `json:"totalSize"`
	Downloaded  int64  `json:"downloaded"`
	FinalURL    string `json:"finalURL"`
	MethodUsed  string `json:"methodUsed"` // 最终使用的下载方式
}

// TestInstall 执行测试安装流程（模拟安装但不实际写入文件）
// 支持取消操作
func (s *ToolchainService) TestInstall(config TestInstallConfig) (*TestInstallResult, error) {
	spec, ok := registry[config.Tool]
	if !ok {
		return nil, errs.Newf("error.toolchain_unknown_tool", map[string]any{"Name": config.Tool})
	}

	// 创建可取消的上下文
	ctx, cancel := context.WithCancel(context.Background())
	s.mu.Lock()
	s.testInstallCtx[config.Tool] = cancel
	s.testInstallCalled[config.Tool] = true
	s.mu.Unlock()

	// 确保清理
	defer func() {
		s.mu.Lock()
		delete(s.testInstallCtx, config.Tool)
		s.mu.Unlock()
	}()

	// 发送初始状态
	s.emitTestInstallStatus(config.Tool, "fetching_version", "正在获取版本信息...")

	// 获取版本信息（使用指定的方式或自动检测）
	var latestVersion string
	var err error

	switch config.DownloadMethod {
	case DownloadMethodDirect:
		// 直连模式
		s.emitTestInstallStatus(config.Tool, "fetching_version", "正在直连获取版本...")
		latestVersion, err = s.fetchLatestVersion(spec)
		if err != nil {
			return &TestInstallResult{
				Success:     false,
				Message:     "直连获取版本失败: " + err.Error(),
				Tool:        config.Tool,
				DownloadURL: spec.latestReleaseAPI,
			}, nil
		}
	case DownloadMethodProxy:
		// 代理模式
		s.emitTestInstallStatus(config.Tool, "fetching_version", "正在通过代理获取版本...")
		latestVersion, err = s.fetchLatestVersion(spec)
		if err != nil {
			// 代理也失败，尝试直连
			s.emitTestInstallStatus(config.Tool, "fetching_version", "代理获取版本失败，尝试直连...")
			latestVersion, err = s.fetchLatestVersion(spec)
			if err != nil {
				return &TestInstallResult{
					Success:     false,
					Message:     "获取版本失败: " + err.Error(),
					Tool:        config.Tool,
					DownloadURL: spec.latestReleaseAPI,
				}, nil
			}
		}
	case DownloadMethodOSS:
		// OSS 兜底模式
		s.emitTestInstallStatus(config.Tool, "fetching_version", "正在通过 API/OSS 获取版本...")
		version, ossURL, apiErr := s.fetchToolLatest(spec.name, runtime.GOOS, runtime.GOARCH)
		if apiErr != nil {
			return &TestInstallResult{
				Success:     false,
				Message:     "API/OSS 获取版本失败: " + apiErr.Error(),
				Tool:        config.Tool,
				DownloadURL: define.ServerURL + "/tool-latest",
			}, nil
		}
		latestVersion = version
		s.emitTestInstallStatus(config.Tool, "version_fetched", fmt.Sprintf("获取到版本: %s", latestVersion))

		// 发送模拟的下载进度
		s.simulateDownloadProgress(ctx, config.Tool, spec, ossURL)

		return &TestInstallResult{
			Success:     true,
			Message:     "测试安装完成（OSS 模式）",
			Tool:        config.Tool,
			Version:     latestVersion,
			DownloadURL: ossURL,
			TotalSize:   0,
			Downloaded:  0,
			FinalURL:    ossURL,
			MethodUsed:  string(DownloadMethodOSS),
		}, nil
	default:
		// 默认自动检测
		needProxy := s.resolveProxy()
		if needProxy {
			s.emitTestInstallStatus(config.Tool, "fetching_version", "检测到需要代理，正在获取版本...")
		} else {
			s.emitTestInstallStatus(config.Tool, "fetching_version", "直连获取版本...")
		}
		latestVersion, err = s.fetchLatestVersion(spec)
		if err != nil {
			// 尝试 API 兜底
			version, ossURL, apiErr := s.fetchToolLatest(spec.name, runtime.GOOS, runtime.GOARCH)
			if apiErr != nil {
				return &TestInstallResult{
					Success:     false,
					Message:     "获取版本失败: " + err.Error(),
					Tool:        config.Tool,
					DownloadURL: spec.latestReleaseAPI,
				}, nil
			}
			latestVersion = version
			s.emitTestInstallStatus(config.Tool, "version_fetched", fmt.Sprintf("API 获取到版本: %s", latestVersion))

			// 模拟 OSS 下载进度
			s.simulateDownloadProgress(ctx, config.Tool, spec, ossURL)

			return &TestInstallResult{
				Success:     true,
				Message:     "测试安装完成（API/OSS 模式）",
				Tool:        config.Tool,
				Version:     latestVersion,
				DownloadURL: ossURL,
				TotalSize:   0,
				Downloaded:  0,
				FinalURL:    ossURL,
				MethodUsed:  string(DownloadMethodOSS),
			}, nil
		}
	}

	s.emitTestInstallStatus(config.Tool, "version_fetched", fmt.Sprintf("获取到版本: %s", latestVersion))

	// 构建下载 URL
	goos := runtime.GOOS
	goarch := runtime.GOARCH
	rawURL := spec.downloadURL(latestVersion, goos, goarch)

	var finalURL string
	var methodUsed string

	// 根据下载方式进行下载
	switch config.DownloadMethod {
	case DownloadMethodDirect:
		finalURL = rawURL
		methodUsed = string(DownloadMethodDirect)
	case DownloadMethodProxy:
		if config.ProxyURL != "" {
			finalURL = config.ProxyURL + rawURL
		} else {
			finalURL = ghProxyPrefix + rawURL
		}
		methodUsed = string(DownloadMethodProxy)
	default:
		// 获取 OSS URL
		ossURL, ossErr := s.fetchLatestDownloadURL(spec.name, goos, goarch)
		if ossErr == nil {
			finalURL = ossURL
			methodUsed = string(DownloadMethodOSS)
		} else {
			finalURL = rawURL
			methodUsed = string(DownloadMethodDirect)
		}
	}

	// 模拟下载进度（真正的下载会发送进度事件，这里我们模拟一些进度）
	s.simulateDownloadProgress(ctx, config.Tool, spec, finalURL)

	return &TestInstallResult{
		Success:     true,
		Message:     "测试安装完成",
		Tool:        config.Tool,
		Version:     latestVersion,
		DownloadURL: rawURL,
		TotalSize:   0,
		Downloaded:  0,
		FinalURL:    finalURL,
		MethodUsed:  methodUsed,
	}, nil
}

// simulateDownloadProgress 模拟下载进度（发送一些模拟的进度事件）
func (s *ToolchainService) simulateDownloadProgress(ctx context.Context, toolName string, spec toolSpec, url string) {
	// 模拟几个进度阶段
	stages := []struct {
		percent    float64
		downloaded int64
		totalSize  int64
		message    string
	}{
		{10, 5 * 1024 * 1024, 50 * 1024 * 1024, "正在连接..."},
		{30, 15 * 1024 * 1024, 50 * 1024 * 1024, "正在下载..."},
		{60, 30 * 1024 * 1024, 50 * 1024 * 1024, "下载中..."},
		{80, 40 * 1024 * 1024, 50 * 1024 * 1024, "即将完成..."},
		{100, 50 * 1024 * 1024, 50 * 1024 * 1024, "下载完成"},
	}

	for i, stage := range stages {
		select {
		case <-ctx.Done():
			s.emitTestInstallStatus(toolName, "cancelled", "已取消下载")
			return
		default:
		}

		s.emitProgress(DownloadProgress{
			Tool:        toolName,
			URL:         url,
			TotalSize:   stage.totalSize,
			Downloaded:  stage.downloaded,
			Percent:     stage.percent,
			Speed:       1024 * 1024 * 2, // 2 MB/s
			ElapsedTime: int64(i * 1000),
			Remaining:   int64((5 - i) * 500),
		})

		s.emitTestInstallStatus(toolName, "downloading", stage.message)

		// 等待一小段时间模拟下载
		time.Sleep(200 * time.Millisecond)
	}
}

// emitTestInstallStatus 发送测试安装状态到前端
func (s *ToolchainService) emitTestInstallStatus(toolName, status, message string) {
	if s.app != nil {
		s.app.Event.Emit("toolchain:test-install-status", map[string]string{
			"tool":    toolName,
			"status":  status,
			"message": message,
		})
	}
}

// IsDevMode returns whether the application is running in development mode.
func (s *ToolchainService) IsDevMode() bool {
	return define.IsDev()
}

// AbortDownload 终止当前下载（仅对 TestInstall 有效）
func (s *ToolchainService) AbortDownload(toolName string) {
	s.mu.Lock()
	cancel, ok := s.testInstallCtx[toolName]
	s.mu.Unlock()

	if ok && cancel != nil {
		cancel()
		s.emitTestInstallStatus(toolName, "aborted", "已终止下载")
	}
}

// ClearInstallingState 清除工具的安装状态（用于恢复卡住的安装状态）
func (s *ToolchainService) ClearInstallingState(toolName string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.installing[toolName]; ok {
		delete(s.installing, toolName)
		s.app.Logger.Info("toolchain: cleared installing state", "tool", toolName)
	}
}

// GetInstallingState 获取当前正在安装的工具列表
func (s *ToolchainService) GetInstallingState() []string {
	s.mu.Lock()
	defer s.mu.Unlock()

	result := make([]string, 0, len(s.installing))
	for name := range s.installing {
		result = append(result, name)
	}
	return result
}

// EnsureAll checks and installs/updates all managed tools.
// Can be called from bootstrap (background goroutine) or from the frontend.
func (s *ToolchainService) EnsureAll() {
	// 启动时清除所有残留的安装状态
	s.mu.Lock()
	for name := range s.installing {
		s.app.Logger.Info("toolchain: clearing stale installing state on startup", "tool", name)
	}
	s.installing = make(map[string]bool)
	s.mu.Unlock()

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

// emitProgress 发送下载进度到前端
func (s *ToolchainService) emitProgress(progress DownloadProgress) {
	if s.app != nil {
		s.app.Event.Emit("toolchain:download-progress", progress)
	}
}

// progressReader 带进度跟踪的 Reader
type progressReader struct {
	reader     io.Reader
	callback   func(downloaded int64, percent float64)
	downloaded int64
	totalSize  int64
}

func (p *progressReader) Read(buf []byte) (int, error) {
	n, err := p.reader.Read(buf)
	if n > 0 {
		p.downloaded += int64(n)
		var percent float64
		if p.totalSize > 0 {
			percent = float64(p.downloaded) / float64(p.totalSize) * 100
		}
		p.callback(p.downloaded, percent)
	}
	return n, err
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

	// 优先尝试从 GitHub 获取最新版本
	latestVersion, err := s.fetchLatestVersion(spec)
	if err != nil {
		s.app.Logger.Warn("toolchain: failed to fetch latest version from GitHub",
			"tool", spec.name, "error", err)
		// 尝试 API 兜底获取版本
		_, ossURL, apiErr := s.fetchToolLatest(spec.name, runtime.GOOS, runtime.GOARCH)
		if apiErr != nil {
			if installedVersion != "" {
				s.app.Logger.Info("toolchain: keeping existing version",
					"tool", spec.name, "version", installedVersion)
			}
			return
		}
		// API 获取到版本，继续使用 OSS 下载
		latestVersion = ""
		if installedVersion == "" {
			s.app.Logger.Info("toolchain: no version info, will use OSS",
				"tool", spec.name)
		}
		if !s.markInstalling(spec.name) {
			s.app.Logger.Info("toolchain: already being installed by another caller",
				"tool", spec.name)
			return
		}
		defer func() {
			s.clearInstalling(spec.name)
			s.emitStatus(spec.name)
		}()
		s.emitStatus(spec.name)

		if installedVersion != "" {
			s.app.Logger.Info("toolchain: updating via OSS",
				"tool", spec.name, "from", installedVersion)
		} else {
			s.app.Logger.Info("toolchain: installing via OSS",
				"tool", spec.name)
		}

		s.app.Logger.Info("toolchain: downloading from OSS", "tool", spec.name, "url", ossURL)
		if err := s.downloadFromOSSURL(spec, ossURL); err != nil {
			s.app.Logger.Error("toolchain: install failed",
				"tool", spec.name, "error", err)
			return
		}
		s.app.Logger.Info("toolchain: installed successfully via OSS",
			"tool", spec.name, "path", binPath)
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
	// defer 必须在所有 return 之前，确保 installing 状态被清除并通知前端
	defer func() {
		s.clearInstalling(spec.name)
		s.emitStatus(spec.name)
	}()
	s.emitStatus(spec.name)

	if installedVersion != "" {
		s.app.Logger.Info("toolchain: updating",
			"tool", spec.name, "from", installedVersion, "to", latestVersion)
	} else {
		s.app.Logger.Info("toolchain: installing",
			"tool", spec.name, "version", latestVersion)
	}

	// 优先尝试从 GitHub 直连/代理下载，失败后尝试 API/OSS 兜底
	if err := s.downloadAndInstall(spec, latestVersion, binDir, needProxy); err != nil {
		// GitHub 下载失败，尝试 API/OSS 兜底
		s.app.Logger.Warn("toolchain: GitHub download failed, trying API/OSS", "tool", spec.name, "error", err)
		ossURL, ossErr := s.fetchLatestDownloadURL(spec.name, runtime.GOOS, runtime.GOARCH)
		if ossErr != nil {
			s.app.Logger.Error("toolchain: install failed (GitHub and API both failed)",
				"tool", spec.name, "version", latestVersion, "error", err)
			// defer 会处理状态更新
			return
		}
		s.app.Logger.Info("toolchain: downloading from OSS", "tool", spec.name, "url", ossURL)
		if err := s.downloadFromOSSURL(spec, ossURL); err != nil {
			s.app.Logger.Error("toolchain: install failed",
				"tool", spec.name, "version", latestVersion, "error", err)
			// defer 会处理状态更新
			return
		}
	}

	s.app.Logger.Info("toolchain: installed successfully",
		"tool", spec.name, "version", latestVersion, "path", binPath)
	// defer 会处理状态更新
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
// It tries direct connection first, then falls back to API mirrors.
func (s *ToolchainService) fetchLatestVersion(spec toolSpec) (string, error) {
	// 先尝试直连
	version, err := s.fetchLatestVersionDirect(spec.latestReleaseAPI)
	if err == nil {
		return version, nil
	}

	// 直连失败，尝试 API 镜像（域名替换方式）
	s.app.Logger.Warn("toolchain: direct connection failed, trying API mirrors", "tool", spec.name, "error", err)

	for _, mirror := range apiMirrors {
		if mirror == "" {
			continue
		}

		// 替换 github.com 为镜像域名
		mirroredURL := strings.Replace(spec.latestReleaseAPI, "https://github.com", mirror, 1)
		s.app.Logger.Info("toolchain: trying API mirror", "tool", spec.name, "url", mirroredURL)

		version, err := s.fetchLatestVersionDirect(mirroredURL)
		if err == nil {
			s.app.Logger.Info("toolchain: fetched version via mirror", "tool", spec.name, "mirror", mirror, "version", version)
			return version, nil
		}
		s.app.Logger.Warn("toolchain: API mirror failed", "tool", spec.name, "mirror", mirror, "error", err)
	}

	return "", fmt.Errorf("failed to fetch latest version: %w", err)
}

// fetchLatestVersionDirect 直接请求获取版本（不经过代理）
func (s *ToolchainService) fetchLatestVersionDirect(url string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodHead, url, nil)
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

	// API 请求体（与 OpenAPI 格式一致）
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

	// 使用 ServerURL + /toolDownload（符合 /openapi 前缀约定）
	apiURL := define.ServerURL + "/toolDownload"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, bytes.NewReader(bodyBytes))
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

// ToolLatestItem 单个工具的最新版本信息（与服务端对应）
type ToolLatestItem struct {
	Tool    string `json:"tool"`
	Version string `json:"version"`
	OS      string `json:"os"`
	Arch    string `json:"arch"`
	URL     string `json:"url"`
}

// fetchToolLatest 从 API 获取工具最新版本信息
func (s *ToolchainService) fetchToolLatest(tool, goos, goarch string) (version string, downloadURL string, err error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	apiURL := define.ServerURL + "/tool-latest"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return "", "", err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("API returned status %d", resp.StatusCode)
	}

	type Response struct {
		Code    int              `json:"code"`
		Data    []ToolLatestItem `json:"data"`
		Message string           `json:"message"`
	}

	var result Response
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", "", err
	}

	// 查找匹配的 tool + os + arch
	for _, item := range result.Data {
		if item.Tool == tool && item.OS == goos && item.Arch == goarch {
			return item.Version, item.URL, nil
		}
	}

	return "", "", fmt.Errorf("no latest version found for %s-%s-%s", tool, goos, goarch)
}

// fetchLatestDownloadURL 从 API 获取最新下载链接（国内使用 OSS）
func (s *ToolchainService) fetchLatestDownloadURL(tool, goos, goarch string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	apiURL := define.ServerURL + "/tool-latest"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return "", err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("API returned status %d", resp.StatusCode)
	}

	type Response struct {
		Code    int              `json:"code"`
		Data    []ToolLatestItem `json:"data"`
		Message string           `json:"message"`
	}

	var result Response
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}

	// 查找匹配的 tool + os + arch
	for _, item := range result.Data {
		if item.Tool == tool && item.OS == goos && item.Arch == goarch {
			return item.URL, nil
		}
	}

	return "", fmt.Errorf("no download URL found for %s-%s-%s", tool, goos, goarch)
}

// downloadFromOSSURL 从 OSS URL 直接下载并安装
func (s *ToolchainService) downloadFromOSSURL(spec toolSpec, ossURL string) error {
	binDir := s.BinDir()
	if binDir == "" {
		return fmt.Errorf("no bin dir")
	}

	s.app.Logger.Info("toolchain: downloading from OSS", "tool", spec.name, "url", ossURL)
	return s.downloadWithSingleURL(spec, ossURL, binDir)
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
	for _, mirror := range downloadProxies {
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

// DownloadProgress 下载进度信息（用于事件推送）
type DownloadProgress struct {
	Tool        string  `json:"tool"`        // 工具名称
	URL         string  `json:"url"`         // 正在下载的 URL
	TotalSize   int64   `json:"totalSize"`   // 总大小（字节）
	Downloaded  int64   `json:"downloaded"`  // 已下载（字节）
	Percent     float64 `json:"percent"`     // 百分比 (0-100)
	Speed       float64 `json:"speed"`       // 下载速度 (KB/s)
	ElapsedTime int64   `json:"elapsedTime"` // 已用时间 (毫秒)
	Remaining   int64   `json:"remaining"`   // 预计剩余时间 (毫秒)
}

// downloadWithSingleURL downloads from a single URL and extracts the binary.
// 支持进度回调和事件推送
func (s *ToolchainService) downloadWithSingleURL(spec toolSpec, dlURL, binDir string) error {
	s.app.Logger.Info("toolchain: downloading", "tool", spec.name, "url", dlURL)

	// 连接超时和读取超时配置
	connectTimeout := 10 * time.Second
	readTimeout := 50 * time.Minute

	// 创建带超时的 HTTP 客户端
	transport := &http.Transport{
		DialContext: (&net.Dialer{
			Timeout: connectTimeout,
		}).DialContext,
		TLSHandshakeTimeout:   connectTimeout,
		ResponseHeaderTimeout: readTimeout,
		DisableCompression:    true,
	}

	client := &http.Client{
		Transport: transport,
		Timeout:   connectTimeout + readTimeout,
	}

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, dlURL, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Accept-Encoding", "identity")

	resp, err := client.Do(req)
	if err != nil {
		// 区分错误类型
		var netErr net.Error
		if errors.As(err, &netErr) {
			if netErr.Timeout() {
				s.app.Logger.Error("toolchain: download timeout", "tool", spec.name, "url", dlURL, "error", err)
				return fmt.Errorf("connection/download timeout: %w", err)
			}
			s.app.Logger.Error("toolchain: connection failed", "tool", spec.name, "url", dlURL, "error", err)
			return fmt.Errorf("connection failed: %w", err)
		}
		return fmt.Errorf("download failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download returned status %d", resp.StatusCode)
	}

	// 获取总大小
	totalSize := resp.ContentLength
	startTime := time.Now()

	// 使用带进度跟踪的 Reader
	reader := &progressReader{
		reader: resp.Body,
		callback: func(downloaded int64, percent float64) {
			elapsed := time.Since(startTime)
			var speed float64
			if elapsed > 0 {
				speed = float64(downloaded) / elapsed.Seconds() / 1024 // KB/s
			}

			var remaining int64
			if speed > 0 && totalSize > 0 {
				remaining = int64(float64(totalSize-downloaded) / speed * 1000) // 毫秒
			}

			// 通过 Wails 事件推送到前端
			progress := DownloadProgress{
				Tool:        spec.name,
				URL:         dlURL,
				TotalSize:   totalSize,
				Downloaded:  downloaded,
				Percent:     percent,
				Speed:       speed,
				ElapsedTime: elapsed.Milliseconds(),
				Remaining:   remaining,
			}
			s.emitProgress(progress)
		},
		totalSize: totalSize,
	}

	data, err := io.ReadAll(reader)
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
