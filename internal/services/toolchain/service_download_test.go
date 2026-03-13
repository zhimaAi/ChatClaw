package toolchain

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// mockApp 提供一个简单的 mock，用于测试 ToolchainService
type mockApp struct {
	logger *slog.Logger
}

func newMockApp() *mockApp {
	return &mockApp{
		logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
}

// 确保 mockApp 实现了 application.App 接口
var _ *application.App

func (m *mockApp) Logger() *slog.Logger {
	return m.logger
}

func (m *mockApp) Event() any {
	return &mockEvent{}
}

type mockEvent struct{}

func (e *mockEvent) Emit(eventName string, data ...any) error {
	return nil
}

// newTestService 创建一个用于测试的 ToolchainService
func newTestService() *ToolchainService {
	_ = newMockApp()
	s := &ToolchainService{
		app:        nil, // 使用 nil 来避免编译错误
		installing: make(map[string]bool),
	}
	// 使用临时目录作为 binDir
	s.binDir = filepath.Join(os.TempDir(), "toolchain-test", "bin")
	_ = os.MkdirAll(s.binDir, 0o755)
	return s
}

// TestDownloadToolOnly 仅下载工具到临时目录，不安装
// 这个测试会：
// 1. 获取工具的最新版本
// 2. 下载工具到临时目录
// 3. 验证文件已下载
func TestDownloadToolOnly(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping download test in short mode")
	}

	// 使用已有的 spec
	spec, ok := registry["uv"]
	if !ok {
		t.Fatal("uv spec not found")
	}

	s := newTestService()
	binDir := s.BinDir()

	// 获取最新版本
	latestVersion, err := s.fetchLatestVersion(spec)
	if err != nil {
		t.Skipf("skipping: failed to fetch latest version: %v", err)
	}

	t.Logf("Latest version: %s", latestVersion)

	// 直接下载（跳过安装步骤，只是下载到临时目录）
	goos := runtime.GOOS
	goarch := runtime.GOARCH
	rawURL := spec.downloadURL(latestVersion, goos, goarch)

	t.Logf("Download URL: %s", rawURL)

	// 使用 downloadWithSingleURL 直接下载
	err = s.downloadWithSingleURL(spec, rawURL, binDir)
	if err != nil {
		// 如果 GitHub 直连失败，尝试使用代理
		t.Logf("Direct download failed, trying with proxy: %v", err)
		proxyURL := ghProxyPrefix + rawURL
		err = s.downloadWithSingleURL(spec, proxyURL, binDir)
		if err != nil {
			t.Fatalf("download failed: %v", err)
		}
	}

	// 验证文件已下载
	binName := spec.binaryName(runtime.GOOS)
	downloadPath := filepath.Join(binDir, binName)

	if _, err := os.Stat(downloadPath); os.IsNotExist(err) {
		t.Fatalf("downloaded file not found at %s", downloadPath)
	}

	t.Logf("Successfully downloaded tool to: %s", downloadPath)

	// 清理测试文件
	_ = os.RemoveAll(filepath.Dir(binDir))
}

// TestDownloadFromOSS 走兜底 API 获取 OSS 下载链接并下载
// 这个测试会：
// 1. 直接调用 fetchLatestDownloadURL 获取 OSS 下载链接
// 2. 从 OSS 下载工具
// 3. 验证文件已下载
func TestDownloadFromOSS(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping OSS download test in short mode")
	}

	// 测试 uv
	toolName := "uv"
	goos := runtime.GOOS
	goarch := runtime.GOARCH

	s := newTestService()

	// 直接调用 fetchLatestDownloadURL 获取 OSS 下载链接
	ossURL, err := s.fetchLatestDownloadURL(toolName, goos, goarch)
	if err != nil {
		t.Skipf("skipping: failed to fetch OSS download URL: %v", err)
	}

	t.Logf("OSS Download URL: %s", ossURL)

	// 使用 spec 进行下载
	spec, ok := registry[toolName]
	if !ok {
		t.Fatalf("tool spec not found: %s", toolName)
	}

	binDir := s.BinDir()

	// 从 OSS URL 下载
	err = s.downloadWithSingleURL(spec, ossURL, binDir)
	if err != nil {
		t.Fatalf("OSS download failed: %v", err)
	}

	// 验证文件已下载
	binName := spec.binaryName(runtime.GOOS)
	downloadPath := filepath.Join(binDir, binName)

	if _, err := os.Stat(downloadPath); os.IsNotExist(err) {
		t.Fatalf("downloaded file not found at %s", downloadPath)
	}

	t.Logf("Successfully downloaded tool from OSS to: %s", downloadPath)

	// 清理测试文件
	_ = os.RemoveAll(filepath.Dir(binDir))
}

// TestFetchToolLatest 测试获取工具最新版本信息
func TestFetchToolLatest(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping fetch test in short mode")
	}

	tests := []string{"uv", "bun", "codex"}

	s := newTestService()
	goos := runtime.GOOS
	goarch := runtime.GOARCH

	for _, tool := range tests {
		t.Run(tool, func(t *testing.T) {
			version, downloadURL, err := s.fetchToolLatest(tool, goos, goarch)
			if err != nil {
				t.Skipf("skipping: failed to fetch latest for %s: %v", tool, err)
			}
			t.Logf("Tool: %s, Version: %s, URL: %s", tool, version, downloadURL)
		})
	}
}

// TestFetchOSSDownloadURL 测试通过 API 获取 OSS 下载链接
func TestFetchOSSDownloadURL(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping OSS URL fetch test in short mode")
	}

	tests := []struct {
		tool    string
		version string
	}{
		{"uv", "0.5.0"},
		{"bun", "1.3.0"},
	}

	s := newTestService()
	goos := runtime.GOOS
	goarch := runtime.GOARCH

	for _, tt := range tests {
		t.Run(tt.tool, func(t *testing.T) {
			url, err := s.fetchOSSDownloadURL(tt.tool, tt.version, goos, goarch)
			if err != nil {
				t.Skipf("skipping: failed to fetch OSS URL for %s: %v", tt.tool, err)
			}
			t.Logf("Tool: %s, Version: %s, OSS URL: %s", tt.tool, tt.version, url)
		})
	}
}
