package openclawruntime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"chatclaw/internal/openclaw"

	"github.com/Masterminds/semver/v3"
)

var openClawRegistryURLs = []string{
	"https://registry.npmjs.org",
	"https://registry.npmmirror.com",
}

func (m *Manager) upgradeRuntimeLocked() (*RuntimeUpgradeResult, error) {
	if m.isShuttingDown() {
		return nil, fmt.Errorf("runtime is shutting down")
	}

	cfg := m.store.Get()
	activeBundle, err := resolveBundledRuntime()
	if err != nil {
		return nil, err
	}

	currentVersion, err := verifyInstalled(activeBundle)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	latestVersion, registryURL, err := fetchLatestOpenClawVersion(ctx)
	if err != nil {
		return nil, err
	}

	result := &RuntimeUpgradeResult{
		PreviousVersion: currentVersion,
		CurrentVersion:  currentVersion,
		LatestVersion:   latestVersion,
		RuntimeSource:   activeBundle.Source,
		RuntimePath:     activeBundle.Root,
	}
	if !isRuntimeUpgradeAvailable(currentVersion, latestVersion) {
		return result, nil
	}

	m.broadcastStatus(RuntimeStatus{
		Phase:            PhaseUpgrading,
		Message:          fmt.Sprintf("Upgrading OpenClaw to %s", latestVersion),
		InstalledVersion: currentVersion,
		RuntimeSource:    activeBundle.Source,
		RuntimePath:      activeBundle.Root,
		GatewayURL:       gatewayURL(cfg.GatewayPort),
	})

	m.closeClient()
	m.stopProcess()

	stagedBundle, restore, cleanup, err := installUserRuntimeOverride(activeBundle, latestVersion, registryURL)
	if err != nil {
		_ = m.reconcileLocked(false)
		return nil, err
	}
	defer cleanup()

	if err := m.reconcileLocked(false); err != nil {
		if restoreErr := restore(); restoreErr != nil {
			m.app.Logger.Error("openclaw: restore previous runtime failed", "error", restoreErr)
		}
		_ = m.reconcileLocked(false)
		return nil, err
	}

	status := m.GetStatus()
	result.Upgraded = true
	result.CurrentVersion = latestVersion
	result.RuntimeSource = status.RuntimeSource
	result.RuntimePath = status.RuntimePath
	if status.RuntimePath == "" {
		result.RuntimePath = stagedBundle.Root
	}
	if status.RuntimeSource == "" {
		result.RuntimeSource = stagedBundle.Source
	}
	return result, nil
}

func isRuntimeUpgradeAvailable(currentVersion, latestVersion string) bool {
	current, err := semver.NewVersion(strings.TrimSpace(currentVersion))
	if err != nil {
		return strings.TrimSpace(currentVersion) != strings.TrimSpace(latestVersion) &&
			strings.TrimSpace(latestVersion) != ""
	}
	latest, err := semver.NewVersion(strings.TrimSpace(latestVersion))
	if err != nil {
		return strings.TrimSpace(currentVersion) != strings.TrimSpace(latestVersion) &&
			strings.TrimSpace(latestVersion) != ""
	}
	return latest.GreaterThan(current)
}

func fetchLatestOpenClawVersion(ctx context.Context) (string, string, error) {
	var issues []string
	for _, registryURL := range openClawRegistryURLs {
		version, err := fetchLatestOpenClawVersionFromRegistry(ctx, registryURL)
		if err != nil {
			issues = append(issues, fmt.Sprintf("%s: %v", registryURL, err))
			continue
		}
		return version, strings.TrimRight(registryURL, "/"), nil
	}

	if len(issues) == 0 {
		return "", "", errors.New("fetch OpenClaw version failed")
	}
	return "", "", fmt.Errorf("fetch latest OpenClaw version failed: %s", strings.Join(issues, "; "))
}

func fetchLatestOpenClawVersionFromRegistry(ctx context.Context, registryURL string) (string, error) {
	type latestPayload struct {
		Version string `json:"version"`
	}

	client := &http.Client{Timeout: 20 * time.Second}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(registryURL, "/")+"/openclaw/latest", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("unexpected status %s", resp.Status)
	}

	var payload latestPayload
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return "", fmt.Errorf("decode latest payload: %w", err)
	}
	payload.Version = strings.TrimSpace(payload.Version)
	if payload.Version == "" {
		return "", errors.New("missing version")
	}
	return payload.Version, nil
}

func installUserRuntimeOverride(activeBundle *bundledRuntime, version, registryURL string) (*bundledRuntime, func() error, func(), error) {
	target := runtime.GOOS + "-" + runtime.GOARCH
	userTargetDir, err := openclaw.UserRuntimeTargetDir(target)
	if err != nil {
		return nil, nil, nil, err
	}
	currentDir, err := openclaw.UserRuntimeCurrentDir(target)
	if err != nil {
		return nil, nil, nil, err
	}
	if err := os.MkdirAll(userTargetDir, 0o700); err != nil {
		return nil, nil, nil, fmt.Errorf("create user runtime dir: %w", err)
	}

	stagingDir := filepath.Join(userTargetDir, fmt.Sprintf(".staging-%s-%d", sanitizeRuntimeVersion(version), time.Now().UnixNano()))
	backupDir := filepath.Join(userTargetDir, ".backup")
	restore := func() error { return nil }
	cleanup := func() {
		_ = os.RemoveAll(stagingDir)
	}

	if err := os.MkdirAll(stagingDir, 0o755); err != nil {
		return nil, nil, cleanup, fmt.Errorf("create staging runtime dir: %w", err)
	}
	if err := copyDirRecursive(filepath.Join(activeBundle.Root, "tools", "node"), filepath.Join(stagingDir, "tools", "node")); err != nil {
		return nil, nil, cleanup, fmt.Errorf("copy bundled node: %w", err)
	}

	npmPrefix := npmGlobalInstallPrefix(stagingDir, runtime.GOOS)
	if runtime.GOOS == "windows" {
		if err := os.MkdirAll(npmPrefix, 0o755); err != nil {
			return nil, nil, cleanup, fmt.Errorf("create npm prefix dir: %w", err)
		}
	}
	if err := installOpenClawPackage(activeBundle.Root, version, registryURL, npmPrefix); err != nil {
		return nil, nil, cleanup, err
	}
	if err := verifyOpenClawLibLayout(stagingDir); err != nil {
		return nil, nil, cleanup, err
	}
	if err := os.MkdirAll(filepath.Join(stagingDir, "bin"), 0o755); err != nil {
		return nil, nil, cleanup, fmt.Errorf("create runtime bin dir: %w", err)
	}
	if err := writeCLIWrappers(stagingDir, runtime.GOOS); err != nil {
		return nil, nil, cleanup, fmt.Errorf("write runtime CLI wrappers: %w", err)
	}
	if err := writeRuntimeManifest(filepath.Join(stagingDir, "manifest.json"), bundledRuntimeManifest{
		OpenClawVersion: version,
		NodeVersion:     activeBundle.Manifest.NodeVersion,
		Platform:        runtime.GOOS,
		Arch:            runtime.GOARCH,
	}); err != nil {
		return nil, nil, cleanup, fmt.Errorf("write runtime manifest: %w", err)
	}

	stagedBundle, err := loadBundledRuntimeCandidate(
		activeBundle.StateDir,
		runtime.GOOS,
		runtime.GOARCH,
		runtimeCandidate{Root: stagingDir, Source: runtimeSourceUser},
	)
	if err != nil {
		return nil, nil, cleanup, err
	}
	if _, err := verifyInstalled(stagedBundle); err != nil {
		return nil, nil, cleanup, fmt.Errorf("verify staged runtime: %w", err)
	}

	_ = os.RemoveAll(backupDir)
	hadCurrent := false
	if _, err := os.Stat(currentDir); err == nil {
		hadCurrent = true
		if err := os.Rename(currentDir, backupDir); err != nil {
			return nil, nil, cleanup, fmt.Errorf("backup current runtime: %w", err)
		}
	}

	if err := os.Rename(stagingDir, currentDir); err != nil {
		if hadCurrent {
			_ = os.Rename(backupDir, currentDir)
		}
		return nil, nil, cleanup, fmt.Errorf("activate upgraded runtime: %w", err)
	}

	restore = func() error {
		_ = os.RemoveAll(currentDir)
		if !hadCurrent {
			return nil
		}
		return os.Rename(backupDir, currentDir)
	}
	cleanup = func() {
		_ = os.RemoveAll(stagingDir)
		_ = os.RemoveAll(backupDir)
	}

	stagedBundle.Root = currentDir
	stagedBundle.CLIPath = filepath.Join(currentDir, "bin", cliName())
	stagedBundle.Source = runtimeSourceUser
	return stagedBundle, restore, cleanup, nil
}

func installOpenClawPackage(bundleRoot, version, registryURL, npmPrefix string) error {
	npmPath := bundledNpmPath(bundleRoot)
	if _, err := os.Stat(npmPath); err != nil {
		return fmt.Errorf("bundled npm is missing at %s: %w", npmPath, err)
	}

	args := []string{
		"install", "-g",
		"--prefix", npmPrefix,
		"--loglevel", "error",
		"--no-fund", "--no-audit",
		"--registry", registryURL,
		"openclaw@" + version,
	}
	cmd := exec.Command(npmPath, args...)
	cmd.Env = buildBundledNodeEnv(bundleRoot)
	cmd.Stdout = io.Discard
	var stderr strings.Builder
	cmd.Stderr = &stderr
	setCmdHideWindow(cmd)
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg != "" {
			return fmt.Errorf("install openclaw runtime: %w: %s", err, msg)
		}
		return fmt.Errorf("install openclaw runtime: %w", err)
	}
	return nil
}

func buildBundledNodeEnv(bundleRoot string) []string {
	envMap := map[string]string{}
	for _, entry := range os.Environ() {
		if k, v, ok := strings.Cut(entry, "="); ok {
			envMap[k] = v
		}
	}
	envMap["SHARP_IGNORE_GLOBAL_LIBVIPS"] = "1"
	envMap["NODE_LLAMA_CPP_SKIP_DOWNLOAD"] = "1"
	envMap["NPM_CONFIG_LOGLEVEL"] = "error"
	envMap["NPM_CONFIG_FUND"] = "false"
	envMap["NPM_CONFIG_AUDIT"] = "false"
	envMap["OPENCLAW_EMBEDDED_IN"] = "ChatClaw"

	var pathKey, nodeBin string
	if runtime.GOOS == "windows" {
		pathKey, nodeBin = "Path", filepath.Join(bundleRoot, "tools", "node")
	} else {
		pathKey, nodeBin = "PATH", filepath.Join(bundleRoot, "tools", "node", "bin")
	}
	if cur := envMap[pathKey]; cur != "" {
		envMap[pathKey] = nodeBin + string(os.PathListSeparator) + cur
	} else {
		envMap[pathKey] = nodeBin
	}

	env := make([]string, 0, len(envMap))
	for k, v := range envMap {
		env = append(env, k+"="+v)
	}
	return env
}

func bundledNpmPath(bundleRoot string) string {
	if runtime.GOOS == "windows" {
		return filepath.Join(bundleRoot, "tools", "node", "npm.cmd")
	}
	return filepath.Join(bundleRoot, "tools", "node", "bin", "npm")
}

func writeRuntimeManifest(path string, manifest bundledRuntimeManifest) error {
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

func sanitizeRuntimeVersion(version string) string {
	var b strings.Builder
	for _, r := range strings.TrimSpace(version) {
		switch {
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '.':
			b.WriteRune(r)
		default:
			b.WriteRune('-')
		}
	}
	out := strings.Trim(b.String(), "-.")
	if out == "" {
		return "unknown"
	}
	return out
}

// npmGlobalInstallPrefix matches the bundle builder layout.
// Unix npm uses prefix/lib/node_modules, while Windows uses prefix/node_modules.
func npmGlobalInstallPrefix(outputDir string, goos string) string {
	if goos == "windows" {
		return filepath.Join(outputDir, "lib")
	}
	return outputDir
}

func verifyOpenClawLibLayout(outputDir string) error {
	pkg := filepath.Join(outputDir, "lib", "node_modules", "openclaw", "package.json")
	if _, err := os.Stat(pkg); err != nil {
		return fmt.Errorf("openclaw package missing at %s after npm install", pkg)
	}
	return nil
}

func writeCLIWrappers(outputDir, goos string) error {
	if goos == "windows" {
		content := strings.Join([]string{
			"@echo off",
			"setlocal",
			`set "SCRIPT_DIR=%~dp0"`,
			`set "OPENCLAW_EMBEDDED_IN=ChatClaw"`,
			`"%SCRIPT_DIR%..\tools\node\node.exe" "%SCRIPT_DIR%..\lib\node_modules\openclaw\dist\entry.js" %*`,
			"",
		}, "\r\n")
		return os.WriteFile(filepath.Join(outputDir, "bin", "openclaw.cmd"), []byte(content), 0o644)
	}

	content := strings.Join([]string{
		"#!/bin/sh",
		"set -eu",
		`SCRIPT_DIR="$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)"`,
		`export OPENCLAW_EMBEDDED_IN="ChatClaw"`,
		`exec "$SCRIPT_DIR/../tools/node/bin/node" "$SCRIPT_DIR/../lib/node_modules/openclaw/dist/entry.js" "$@"`,
		"",
	}, "\n")
	path := filepath.Join(outputDir, "bin", "openclaw")
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		return err
	}
	return os.Chmod(path, 0o755)
}

func copyDirRecursive(src, dst string) error {
	info, err := os.Lstat(src)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("%s is not a directory", src)
	}
	if err := os.MkdirAll(dst, info.Mode().Perm()); err != nil {
		return err
	}

	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		dstPath := filepath.Join(dst, entry.Name())
		info, err := os.Lstat(srcPath)
		if err != nil {
			return err
		}
		switch mode := info.Mode(); {
		case mode&os.ModeSymlink != 0:
			target, err := os.Readlink(srcPath)
			if err != nil {
				return err
			}
			if err := os.Symlink(target, dstPath); err != nil {
				return err
			}
		case info.IsDir():
			if err := copyDirRecursive(srcPath, dstPath); err != nil {
				return err
			}
		default:
			if err := copyFile(srcPath, dstPath, info.Mode().Perm()); err != nil {
				return err
			}
		}
	}
	return nil
}

func copyFile(src, dst string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Close()
}
