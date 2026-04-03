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

	"github.com/Masterminds/semver/v3"
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

// openclawVersion is the bundled OpenClaw runtime version (matches build/runtime.yml).
// Used for OSS downloads when no bundled runtime is available. Must match backend tool-download registry format.
const openclawVersion = "2026.3.24"

// openclawSpec is a placeholder entry for the openclaw runtime in the registry.
// The actual download URL is resolved via the API at runtime (see InstallOpenClawRuntime),
// so binaryPathInArchive is intentionally omitted — this spec is not used for
// downloadWithSingleURL. Instead InstallOpenClawRuntime handles the full
// download → extract → atomic activation flow independently.
var openclawSpec = toolSpec{
	name: "openclaw",
	archiveFormat: func(goos string) string {
		if goos == "windows" {
			return "zip"
		}
		return "tar.gz"
	},
}

// InstallOpenClawRuntime downloads the OpenClaw runtime package from OSS and installs it
// to ~/.chatclaw/openclaw/runtime/<target>/current using an atomic staging+backup+activate
// pattern. It uses a fixed version number and fetches the download URL from the backend API.
//
// This is called as a fallback when reconcileLocked cannot find any bundled runtime.
// The installed layout matches the output of internal/tools/openclawbundle:
//
//	tools/node/, lib/node_modules/openclaw/, bin/openclaw(.cmd), manifest.json
func (s *ToolchainService) InstallOpenClawRuntime() error {
	target := runtime.GOOS + "-" + runtime.GOARCH
	version := openclawVersion

	s.app.Logger.Info("toolchain: installing openclaw runtime from OSS", "target", target, "version", version)

	emitProgress := func(progress int, message string) {
		if s.upgradeProgressCb != nil {
			s.upgradeProgressCb(progress, message)
		}
	}

	// Fetch download URL from API using fixed version
	emitProgress(5, "Fetching download URL...")
	ossURL, err := s.fetchOSSDownloadURL("openclaw", version, runtime.GOOS, runtime.GOARCH)
	if err != nil {
		s.app.Logger.Error("toolchain: failed to fetch openclaw download URL from API", "error", err)
		return fmt.Errorf("fetch openclaw download URL: %w", err)
	}
	s.app.Logger.Info("toolchain: openclaw runtime", "version", version, "url", ossURL)

	// Prepare target dirs
	userTargetDir, err := openclawRuntimeTargetDir(target)
	if err != nil {
		return fmt.Errorf("resolve runtime target dir: %w", err)
	}
	if err := os.MkdirAll(userTargetDir, 0o700); err != nil {
		return fmt.Errorf("create runtime target dir: %w", err)
	}

	stagingDir := filepath.Join(userTargetDir, fmt.Sprintf(".staging-oss-%d", time.Now().UnixNano()))
	backupDir := filepath.Join(userTargetDir, ".backup")
	cleanup := func() { _ = os.RemoveAll(stagingDir) }

	if err := os.MkdirAll(stagingDir, 0o755); err != nil {
		return fmt.Errorf("create staging dir: %w", err)
	}

	// Download from OSS with progress reporting
	archiveFormat := openclawSpec.archiveFormat(runtime.GOOS)
	archivePath := filepath.Join(stagingDir, "archive."+archiveFormat)
	emitProgress(10, "Downloading OpenClaw runtime...")
	if err := s.downloadOpenClawArchive(context.Background(), ossURL, archivePath, version); err != nil {
		cleanup()
		return fmt.Errorf("download openclaw from OSS: %w", err)
	}
	emitProgress(60, "Download complete, extracting archive...")

	// Extract: OSS zip is flat (bin/, lib/, tools/, manifest.json). Do not use stripArchiveRoot.
	// Windows: prefer bundled tar -xf (same as NSIS) for large trees; fall back to Go flat unzip.
	if err := extractOpenClawRuntimeArchive(archivePath, stagingDir, archiveFormat); err != nil {
		if s.app != nil {
			s.app.Logger.Error("toolchain: extract openclaw archive failed", "error", err, "archive", archivePath)
		}
		cleanup()
		return fmt.Errorf("extract openclaw archive: %w", err)
	}
	_ = os.Remove(archivePath)
	emitProgress(70, "Archive extracted, verifying installation...")

	// Verify extracted layout
	if err := verifyOpenClawLibLayout(stagingDir); err != nil {
		cleanup()
		return fmt.Errorf("verify openclaw layout: %w", err)
	}

	// Write CLI wrappers if missing
	if err := writeOpenClawCLIWrappers(stagingDir, runtime.GOOS); err != nil {
		cleanup()
		return fmt.Errorf("write CLI wrappers: %w", err)
	}

	// Verify the CLI runs
	if err := verifyOpenClawCLI(filepath.Join(stagingDir, "bin", openClawCLIName())); err != nil {
		cleanup()
		return fmt.Errorf("openclaw CLI smoke check: %w", err)
	}
	emitProgress(75, "Installation verified, activating...")

	// Atomic activation: backup current, rename staging to current
	currentDir, err := openclawRuntimeCurrentDir(target)
	if err != nil {
		cleanup()
		return fmt.Errorf("resolve current dir: %w", err)
	}
	hadCurrent := false
	if _, err := os.Stat(currentDir); err == nil {
		hadCurrent = true
		emitProgress(80, "Backing up current runtime...")
		if err := os.Rename(currentDir, backupDir); err != nil {
			cleanup()
			return fmt.Errorf("backup current runtime: %w", err)
		}
	}

	emitProgress(85, "Activating new runtime...")
	if err := os.Rename(stagingDir, currentDir); err != nil {
		if hadCurrent {
			_ = os.Rename(backupDir, currentDir)
		}
		cleanup()
		return fmt.Errorf("activate openclaw runtime: %w", err)
	}
	emitProgress(95, "Cleaning up...")

	cleanup = func() {
		_ = os.RemoveAll(stagingDir)
		_ = os.RemoveAll(backupDir)
	}
	cleanup()
	emitProgress(100, "Installation complete")

	s.app.Logger.Info("toolchain: openclaw runtime installed", "version", version, "path", currentDir)
	return nil
}

// OpenClawRuntimeStatus mirrors the ToolStatus shape for the openclaw runtime entry in the UI.
type OpenClawRuntimeStatus struct {
	Name             string `json:"name"`
	Installed        bool   `json:"installed"`
	InstalledVersion string `json:"installed_version"`
	LatestVersion    string `json:"latest_version,omitempty"`
	HasUpdate        bool   `json:"has_update"`
	Installing       bool   `json:"installing"`
	RuntimePath      string `json:"runtime_path"`
	GatewayStatus    string `json:"gateway_status"` // "idle" | "running" | "error"
}

// GetOpenClawRuntimeStatus returns the installation status of the OpenClaw runtime
// by reading manifest.json from (in order):
//   1) ~/.chatclaw/openclaw/runtime/<target>/current (OSS download install)
//   2) <exeDir>/rt/<target> (NSIS / installer bundle next to ChatClaw.exe)
//   3) on macOS: <exeDir>/../Resources/rt/<target> (app bundle)
// We do not spawn openclaw --version here: on Windows that flashes a console window, and CLI
// output is verbose. Upgrade availability is handled by the OpenClaw runtime service (manager UI).
func (s *ToolchainService) GetOpenClawRuntimeStatus() (*OpenClawRuntimeStatus, error) {
	target := runtime.GOOS + "-" + runtime.GOARCH

	status := &OpenClawRuntimeStatus{Name: "openclaw", Installing: false}

	root, err := resolveOpenClawRuntimeRootForStatus(target)
	if err != nil {
		return nil, err
	}
	if root == "" {
		return status, nil
	}

	manifestPath := filepath.Join(root, "manifest.json")
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return nil, fmt.Errorf("read manifest: %w", err)
	}

	var manifest struct {
		OpenClawVersion string `json:"openclawVersion"`
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		return nil, fmt.Errorf("parse manifest: %w", err)
	}

	status.InstalledVersion = strings.TrimSpace(manifest.OpenClawVersion)
	status.Installed = true
	status.RuntimePath = root

	// Extension settings only show install/path/version. OpenClaw upgrade checks belong to
	// the OpenClaw manager (runtime service), not the toolchain extension list.
	status.LatestVersion = ""
	status.HasUpdate = false

	return status, nil
}

// resolveOpenClawRuntimeRootForStatus returns the runtime root directory whose manifest to read
// for UI status. Order matches internal/openclaw/runtime bundledRuntimeCandidates.
func resolveOpenClawRuntimeRootForStatus(target string) (string, error) {
	currentDir, err := openclawRuntimeCurrentDir(target)
	if err != nil {
		return "", err
	}
	if ok, err := isValidOpenClawRuntimeRoot(currentDir); err != nil {
		return "", err
	} else if ok {
		return currentDir, nil
	}

	execPath, err := os.Executable()
	if err != nil || strings.TrimSpace(execPath) == "" {
		return "", nil
	}
	execDir := filepath.Dir(execPath)

	var embedded []string
	if runtime.GOOS == "darwin" {
		embedded = append(embedded, filepath.Clean(filepath.Join(execDir, "..", "Resources", "rt", target)))
	}
	embedded = append(embedded, filepath.Join(execDir, "rt", target))

	for _, root := range embedded {
		if ok, err := isValidOpenClawRuntimeRoot(root); err != nil {
			return "", err
		} else if ok {
			return root, nil
		}
	}

	return "", nil
}

func isValidOpenClawRuntimeRoot(root string) (bool, error) {
	manifestPath := filepath.Join(root, "manifest.json")
	if _, err := os.Stat(manifestPath); err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	cli := filepath.Join(root, "bin", openClawCLIName())
	if _, err := os.Stat(cli); err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// openclawRuntimeTargetDir returns ~/.chatclaw/openclaw/runtime/<target>
func openclawRuntimeTargetDir(target string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".chatclaw", "openclaw", "runtime", target), nil
}

// openclawRuntimeCurrentDir returns ~/.chatclaw/openclaw/runtime/<target>/current
func openclawRuntimeCurrentDir(target string) (string, error) {
	dir, err := openclawRuntimeTargetDir(target)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "current"), nil
}

func openClawCLIName() string {
	if runtime.GOOS == "windows" {
		return "openclaw.cmd"
	}
	return "openclaw"
}

func verifyOpenClawLibLayout(dir string) error {
	pkg := filepath.Join(dir, "lib", "node_modules", "openclaw", "package.json")
	if _, err := os.Stat(pkg); err != nil {
		return fmt.Errorf("openclaw package missing at %s", pkg)
	}
	return nil
}

func verifyOpenClawCLI(cliPath string) error {
	if runtime.GOOS == "windows" {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, cliPath, "--version")
	hideWindow(cmd)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("openclaw CLI check: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func writeOpenClawCLIWrappers(outputDir, goos string) error {
	binDir := filepath.Join(outputDir, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		return err
	}

	if goos == "windows" {
		cliPath := filepath.Join(binDir, "openclaw.cmd")
		if _, err := os.Stat(cliPath); err == nil {
			return nil // already exists
		}
		content := strings.Join([]string{
			"@echo off",
			"setlocal",
			`set "SCRIPT_DIR=%~dp0"`,
			"set OPENCLAW_EMBEDDED_IN=ChatClaw",
			`"%SCRIPT_DIR%..\tools\node\node.exe" "%SCRIPT_DIR%..\lib\node_modules\openclaw\dist\entry.js" %*`,
			"",
		}, "\r\n")
		return os.WriteFile(cliPath, []byte(content), 0o644)
	}

	cliPath := filepath.Join(binDir, "openclaw")
	if _, err := os.Stat(cliPath); err == nil {
		return nil
	}
	content := strings.Join([]string{
		"#!/bin/sh",
		"set -eu",
		`SCRIPT_DIR="$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)"`,
		`export OPENCLAW_EMBEDDED_IN="ChatClaw"`,
		`exec "$SCRIPT_DIR/../tools/node/bin/node" "$SCRIPT_DIR/../lib/node_modules/openclaw/dist/entry.js" "$@"`,
		"",
	}, "\n")
	if err := os.WriteFile(cliPath, []byte(content), 0o755); err != nil {
		return err
	}
	return os.Chmod(cliPath, 0o755)
}

// downloadOpenClawArchive downloads the OSS archive to disk with progress reporting.
// It uses streaming so the file never fully resides in memory.
func (s *ToolchainService) downloadOpenClawArchive(ctx context.Context, dlURL, destPath, version string) error {
	s.app.Logger.Info("toolchain: downloading openclaw runtime from OSS", "url", dlURL)

	connectTimeout := 10 * time.Second
	readTimeout := 50 * time.Minute
	transport := &http.Transport{
		DialContext:           (&net.Dialer{Timeout: connectTimeout}).DialContext,
		TLSHandshakeTimeout:   connectTimeout,
		ResponseHeaderTimeout: readTimeout,
		DisableCompression:    true,
	}
	client := &http.Client{Transport: transport, Timeout: connectTimeout + readTimeout}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, dlURL, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Accept-Encoding", "identity")

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("download failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("OSS returned HTTP %d", resp.StatusCode)
	}

	totalSize := resp.ContentLength
	startTime := time.Now()

	// Stream to disk with progress
	f, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("create dest file: %w", err)
	}

	var downloaded int64
	for {
		select {
		case <-ctx.Done():
			f.Close()
			_ = os.Remove(destPath)
			return ctx.Err()
		default:
		}

		const bufSize = 32 * 1024
		buf := make([]byte, bufSize)
		n, readErr := resp.Body.Read(buf)
		if n > 0 {
			if _, writeErr := f.Write(buf[:n]); writeErr != nil {
				f.Close()
				_ = os.Remove(destPath)
				return fmt.Errorf("write: %w", writeErr)
			}
			downloaded += int64(n)
		}
		if readErr != nil {
			if readErr == io.EOF {
				break
			}
			f.Close()
			_ = os.Remove(destPath)
			return fmt.Errorf("read: %w", readErr)
		}

		// Emit progress every ~1 MB or on last chunk
		if downloaded%1024*1024 < bufSize || readErr == io.EOF {
			var speed float64
			elapsed := time.Since(startTime)
			if elapsed > 0 {
				speed = float64(downloaded) / elapsed.Seconds() / 1024
			}
			var remaining int64
			if speed > 0 && totalSize > 0 {
				remaining = int64(float64(totalSize-downloaded)/speed) * 1000
			}
			percent := 0.0
			if totalSize > 0 {
				percent = float64(downloaded) * 100 / float64(totalSize)
			}
			s.emitProgress(DownloadProgress{
				Tool:        "openclaw",
				URL:         dlURL,
				TotalSize:   totalSize,
				Downloaded:  downloaded,
				Percent:     percent,
				Speed:       speed,
				ElapsedTime: elapsed.Milliseconds(),
				Remaining:   remaining,
			})
		}
	}

	if err := f.Close(); err != nil {
		_ = os.Remove(destPath)
		return fmt.Errorf("close file: %w", err)
	}

	s.app.Logger.Info("toolchain: openclaw runtime downloaded", "bytes", downloaded, "elapsed", time.Since(startTime))
	return nil
}

// extractArchiveToDir extracts a zip or tar.gz archive to destDir, stripping the top-level
// directory entry (consistent with how openclawbundle builds the archive).
func extractArchiveToDir(archivePath, destDir, format string) error {
	switch format {
	case "zip":
		return extractZipToDir(archivePath, destDir)
	case "tar.gz":
		return extractTarGzToDir(archivePath, destDir)
	default:
		return fmt.Errorf("unsupported archive format: %s", format)
	}
}

func extractZipToDir(archivePath, destDir string) error {
	reader, err := zip.OpenReader(archivePath)
	if err != nil {
		return err
	}
	defer reader.Close()

	for _, file := range reader.File {
		relativePath, ok := stripArchiveRoot(file.Name)
		if !ok {
			continue
		}
		targetPath := filepath.Join(destDir, relativePath)
		if file.FileInfo().IsDir() {
			if err := os.MkdirAll(targetPath, 0o755); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
			return err
		}
		src, err := file.Open()
		if err != nil {
			return err
		}
		dst, err := os.OpenFile(targetPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, file.Mode())
		if err != nil {
			src.Close()
			return err
		}
		if _, err := io.Copy(dst, src); err != nil {
			src.Close()
			dst.Close()
			return err
		}
		src.Close()
		if err := dst.Close(); err != nil {
			return err
		}
	}
	return nil
}

func extractTarGzToDir(archivePath, destDir string) error {
	file, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer file.Close()

	gzr, err := gzip.NewReader(file)
	if err != nil {
		return err
	}
	defer gzr.Close()

	tr := tar.NewReader(gzr)
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		relativePath, ok := stripArchiveRoot(hdr.Name)
		if !ok {
			continue
		}
		targetPath := filepath.Join(destDir, relativePath)

		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(targetPath, 0o755); err != nil {
				return err
			}
		case tar.TypeReg, tar.TypeRegA:
			if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
				return err
			}
			fileMode := os.FileMode(hdr.Mode)
			if fileMode == 0 {
				fileMode = 0o644
			}
			dst, err := os.OpenFile(targetPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, fileMode)
			if err != nil {
				return err
			}
			if _, err := io.Copy(dst, tr); err != nil {
				dst.Close()
				return err
			}
			if err := dst.Close(); err != nil {
				return err
			}
		case tar.TypeSymlink:
			if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
				return err
			}
			if err := os.Symlink(hdr.Linkname, targetPath); err != nil && !os.IsExist(err) {
				return err
			}
		}
	}
}

// stripArchiveRoot strips the top-level directory component from an archive entry name.
func stripArchiveRoot(name string) (string, bool) {
	name = filepath.ToSlash(name)
	name = strings.TrimPrefix(name, "./")
	parts := strings.SplitN(name, "/", 2)
	if len(parts) < 2 || parts[1] == "" {
		return "", false
	}
	return filepath.FromSlash(parts[1]), true
}

// zipSanitizedRelativePath returns a safe relative path for a zip entry (flat layout, no zip-slip).
func zipSanitizedRelativePath(name string) (string, bool) {
	name = filepath.ToSlash(strings.TrimSpace(name))
	if name == "" {
		return "", false
	}
	name = strings.TrimPrefix(name, "./")
	if strings.HasPrefix(name, "/") {
		return "", false
	}
	if len(name) >= 2 && name[1] == ':' { // Windows "C:/..."
		return "", false
	}
	for _, seg := range strings.Split(name, "/") {
		if seg == ".." {
			return "", false
		}
	}
	name = strings.TrimSuffix(name, "/")
	if name == "" {
		return "", false
	}
	return filepath.FromSlash(name), true
}

func pathWithinDir(baseDir, targetPath string) bool {
	baseAbs, err := filepath.Abs(baseDir)
	if err != nil {
		return false
	}
	targetAbs, err := filepath.Abs(targetPath)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(baseAbs, targetAbs)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator))
}

// extractOpenClawRuntimeArchive unpacks the OpenClaw OSS bundle (flat root: bin, lib, tools, manifest.json).
func extractOpenClawRuntimeArchive(archivePath, destDir, format string) error {
	switch format {
	case "zip":
		if runtime.GOOS == "windows" {
			if err := extractZipViaWindowsTar(archivePath, destDir); err == nil {
				return nil
			}
		}
		return extractZipToDirFlat(archivePath, destDir)
	case "tar.gz":
		return extractTarGzToDirFlat(archivePath, destDir)
	default:
		return fmt.Errorf("unsupported archive format: %s", format)
	}
}

// extractZipViaWindowsTar uses the system tar.exe (Windows 10+) to expand zip, matching NSIS installer behavior.
func extractZipViaWindowsTar(archivePath, destDir string) error {
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return err
	}
	cmd := exec.Command("tar", "-xf", archivePath, "-C", destDir)
	hideWindow(cmd)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("tar -xf: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// extractZipToDirFlat extracts zip entries preserving paths from archive root (no top-level strip).
func extractZipToDirFlat(archivePath, destDir string) error {
	destAbs, err := filepath.Abs(destDir)
	if err != nil {
		return err
	}
	reader, err := zip.OpenReader(archivePath)
	if err != nil {
		return err
	}
	defer reader.Close()

	for _, file := range reader.File {
		rel, ok := zipSanitizedRelativePath(file.Name)
		if !ok {
			continue
		}
		targetPath := filepath.Join(destAbs, rel)
		if !pathWithinDir(destAbs, targetPath) {
			continue
		}
		if file.FileInfo().IsDir() {
			if err := os.MkdirAll(targetPath, 0o755); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
			return err
		}
		src, err := file.Open()
		if err != nil {
			return err
		}
		dst, err := os.OpenFile(targetPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, file.Mode())
		if err != nil {
			src.Close()
			return err
		}
		_, copyErr := io.Copy(dst, src)
		_ = src.Close()
		closeErr := dst.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeErr != nil {
			return closeErr
		}
	}
	return nil
}

// extractTarGzToDirFlat extracts tar.gz preserving paths from archive root (no top-level strip).
func extractTarGzToDirFlat(archivePath, destDir string) error {
	destAbs, err := filepath.Abs(destDir)
	if err != nil {
		return err
	}
	f, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer f.Close()

	gzr, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gzr.Close()

	tr := tar.NewReader(gzr)
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		rel, ok := zipSanitizedRelativePath(hdr.Name) // same rules as zip entries
		if !ok {
			continue
		}
		targetPath := filepath.Join(destAbs, rel)
		if !pathWithinDir(destAbs, targetPath) {
			continue
		}

		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(targetPath, 0o755); err != nil {
				return err
			}
		case tar.TypeReg, tar.TypeRegA:
			if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
				return err
			}
			fileMode := os.FileMode(hdr.Mode)
			if fileMode == 0 {
				fileMode = 0o644
			}
			dst, err := os.OpenFile(targetPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, fileMode)
			if err != nil {
				return err
			}
			if _, err := io.Copy(dst, tr); err != nil {
				dst.Close()
				return err
			}
			if err := dst.Close(); err != nil {
				return err
			}
		case tar.TypeSymlink:
			if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
				return err
			}
			if err := os.Symlink(hdr.Linkname, targetPath); err != nil && !os.IsExist(err) {
				return err
			}
		}
	}
}

// ToolchainService manages external tool binaries (uv, bun) within the app data dir.
// It is registered as a Wails service so the frontend can query status and trigger installs.
type ToolchainService struct {
	app    *application.App
	binDir string

	initOnce sync.Once

	mu          sync.Mutex
	installing  map[string]bool // tracks which tools are currently being installed
	// pendingToolLatest records a newer remote version discovered during EnsureAll;
	// we do not auto-download when the tool is already installed — user must use Install from settings.
	pendingToolLatest map[string]string
	proxyProbed       bool
	needProxy         bool

	// testInstall 用于测试安装的上下文（支持取消）
	testInstallCtx    map[string]context.CancelFunc // tool name -> cancel function
	testInstallCalled map[string]bool               // tool name -> 是否已调用过（用于触发进度事件）

	upgradeProgressCb func(progress int, message string)
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

// SetUpgradeProgressCallback sets a callback to be invoked during OpenClaw runtime
// upgrade/install progress updates. The callback receives progress (0-100) and a message.
func (s *ToolchainService) SetUpgradeProgressCallback(cb func(progress int, message string)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.upgradeProgressCb = cb
}

// BinDir returns the path to the bin directory where tools are installed.
func (s *ToolchainService) BinDir() string {
	s.initOnce.Do(func() {
		dir, err := define.AppDataDir()
		if err != nil {
			if s.app != nil {
				s.app.Logger.Error("toolchain: failed to get app data dir", "error", err)
			}
			return
		}
		s.binDir = filepath.Join(dir, "bin")
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

	s.clearPendingToolUpdate(name)
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

func (s *ToolchainService) setPendingToolUpdate(name, latest string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.pendingToolLatest == nil {
		s.pendingToolLatest = make(map[string]string)
	}
	s.pendingToolLatest[name] = latest
}

func (s *ToolchainService) clearPendingToolUpdate(name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.pendingToolLatest != nil {
		delete(s.pendingToolLatest, name)
	}
}

func (s *ToolchainService) pendingToolLatestVersion(name string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.pendingToolLatest == nil {
		return ""
	}
	return s.pendingToolLatest[name]
}

// isManagedToolUpgradeAvailable mirrors OpenClaw runtime semver rules for uv/bun/codex tags.
func isManagedToolUpgradeAvailable(currentVersion, latestVersion string) bool {
	current, err1 := semver.NewVersion(strings.TrimSpace(extractVersion(currentVersion)))
	latest, err2 := semver.NewVersion(strings.TrimSpace(extractVersion(latestVersion)))
	if err1 != nil || err2 != nil {
		return strings.TrimSpace(extractVersion(currentVersion)) != strings.TrimSpace(extractVersion(latestVersion)) &&
			strings.TrimSpace(extractVersion(latestVersion)) != ""
	}
	return latest.GreaterThan(current)
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
	s.emitUpdatesAvailableIfAny()
}

// emitUpdatesAvailableIfAny notifies the UI when optional updates were deferred (no auto-install).
func (s *ToolchainService) emitUpdatesAvailableIfAny() {
	if s.app == nil {
		return
	}
	s.mu.Lock()
	payload := make(map[string]string, len(s.pendingToolLatest)+1)
	for k, v := range s.pendingToolLatest {
		payload[k] = v
	}
	s.mu.Unlock()

	if len(payload) == 0 {
		return
	}
	s.app.Event.Emit("toolchain:updates-available", payload)
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

	st := ToolStatus{
		Name:             name,
		Installed:        installed != "",
		InstalledVersion: installed,
		HasUpdate:        false,
		Installing:       isInstalling,
		BinPath:          binPath,
	}
	if pending := s.pendingToolLatestVersion(name); pending != "" && installed != "" &&
		isManagedToolUpgradeAvailable(installed, pending) {
		st.LatestVersion = pending
		st.HasUpdate = true
	}
	return st
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

// ensureTool installs a missing tool on startup. If the tool is already installed but a newer
// version exists, we only record a pending update and emit status — the user must tap Update in settings.
func (s *ToolchainService) ensureTool(spec toolSpec, binDir string, needProxy bool) {
	binName := spec.binaryName(runtime.GOOS)
	binPath := filepath.Join(binDir, binName)

	installedVersion := s.getInstalledVersion(binPath, spec.versionArgs)

	latestVersion, err := s.fetchLatestVersion(spec)
	if err != nil {
		s.app.Logger.Warn("toolchain: failed to fetch latest version from GitHub",
			"tool", spec.name, "error", err)
		apiVersionRaw, ossURL, apiErr := s.fetchToolLatest(spec.name, runtime.GOOS, runtime.GOARCH)
		if apiErr != nil {
			if installedVersion != "" {
				s.app.Logger.Info("toolchain: keeping existing version",
					"tool", spec.name, "version", installedVersion)
			}
			return
		}
		apiVer := extractVersion(strings.TrimSpace(apiVersionRaw))
		if apiVer == "" {
			apiVer = strings.TrimSpace(apiVersionRaw)
		}

		if installedVersion != "" {
			if !isManagedToolUpgradeAvailable(installedVersion, apiVer) {
				s.clearPendingToolUpdate(spec.name)
				s.emitStatus(spec.name)
				s.app.Logger.Info("toolchain: already up to date (API)",
					"tool", spec.name, "version", installedVersion)
				return
			}
			s.setPendingToolUpdate(spec.name, apiVer)
			s.emitStatus(spec.name)
			s.app.Logger.Info("toolchain: update available (API), deferred until user installs from settings",
				"tool", spec.name, "installed", installedVersion, "latest", apiVer)
			return
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

		s.app.Logger.Info("toolchain: installing via OSS", "tool", spec.name)
		s.app.Logger.Info("toolchain: downloading from OSS", "tool", spec.name, "url", ossURL)
		if err := s.downloadFromOSSURL(spec, ossURL); err != nil {
			s.app.Logger.Error("toolchain: install failed",
				"tool", spec.name, "error", err)
			return
		}
		s.clearPendingToolUpdate(spec.name)
		s.app.Logger.Info("toolchain: installed successfully via OSS",
			"tool", spec.name, "path", binPath)
		return
	}

	if installedVersion != "" {
		if !isManagedToolUpgradeAvailable(installedVersion, latestVersion) {
			s.clearPendingToolUpdate(spec.name)
			s.emitStatus(spec.name)
			s.app.Logger.Info("toolchain: already up to date",
				"tool", spec.name, "version", installedVersion)
			return
		}
		s.setPendingToolUpdate(spec.name, latestVersion)
		s.emitStatus(spec.name)
		s.app.Logger.Info("toolchain: update available, deferred until user installs from settings",
			"tool", spec.name, "installed", installedVersion, "latest", latestVersion)
		return
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

	s.app.Logger.Info("toolchain: installing",
		"tool", spec.name, "version", latestVersion)

	if err := s.downloadAndInstall(spec, latestVersion, binDir, needProxy); err != nil {
		s.app.Logger.Warn("toolchain: GitHub download failed, trying API/OSS", "tool", spec.name, "error", err)
		ossURL, ossErr := s.fetchLatestDownloadURL(spec.name, runtime.GOOS, runtime.GOARCH)
		if ossErr != nil {
			s.app.Logger.Error("toolchain: install failed (GitHub and API both failed)",
				"tool", spec.name, "version", latestVersion, "error", err)
			return
		}
		s.app.Logger.Info("toolchain: downloading from OSS", "tool", spec.name, "url", ossURL)
		if err := s.downloadFromOSSURL(spec, ossURL); err != nil {
			s.app.Logger.Error("toolchain: install failed",
				"tool", spec.name, "version", latestVersion, "error", err)
			return
		}
	}

	s.clearPendingToolUpdate(spec.name)
	s.app.Logger.Info("toolchain: installed successfully",
		"tool", spec.name, "version", latestVersion, "path", binPath)
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

	// Response matches openapi: { "code": 0, "data": { "url": "..." }, "message": "ok" }
	type toolDownloadResponse struct {
		Code int `json:"code"`
		Data struct {
			URL string `json:"url"`
		} `json:"data"`
		Message string `json:"message"`
		// Legacy: some deployments may return url at root
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

	// 使用 ServerURL + /tool-download（符合 /openapi 前缀约定）
	apiURL := define.ServerURL + "/tool-download"
	if s.app != nil {
		s.app.Logger.Info("toolchain: tool-download request", "url", apiURL, "body", string(bodyBytes))
	}

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

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	if resp.StatusCode != http.StatusOK {
		if s.app != nil {
			s.app.Logger.Error("toolchain: tool-download HTTP error", "status", resp.StatusCode, "body", string(body))
		}
		return "", fmt.Errorf("API returned status %d", resp.StatusCode)
	}

	var result toolDownloadResponse
	if err := json.Unmarshal(body, &result); err != nil {
		if s.app != nil {
			s.app.Logger.Error("toolchain: tool-download parse error", "error", err, "body", string(body))
		}
		return "", fmt.Errorf("decode tool-download response: %w", err)
	}

	dlURL := strings.TrimSpace(result.Data.URL)
	if dlURL == "" {
		dlURL = strings.TrimSpace(result.URL)
	}
	if dlURL == "" {
		if s.app != nil {
			s.app.Logger.Error("toolchain: tool-download empty url", "body", string(body))
		}
		return "", fmt.Errorf("API returned empty URL")
	}

	if s.app != nil {
		s.app.Logger.Info("toolchain: tool-download ok", "resolved_url_len", len(dlURL))
	}
	return dlURL, nil
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
