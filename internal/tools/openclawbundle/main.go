package main

import (
	"archive/tar"
	"archive/zip"
	"bufio"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const nodeDistBaseURL = "https://nodejs.org/dist"

type runtimeConfig struct {
	OpenClawVersion string
	NodeVersion     string
}

type runtimeManifest struct {
	OpenClawVersion string `json:"openclawVersion"`
	NodeVersion     string `json:"nodeVersion"`
	Platform        string `json:"platform"`
	Arch            string `json:"arch"`
	Target          string `json:"target"`
	BuiltAt         string `json:"builtAt"`
}

func main() {
	configPath := flag.String("config", "build/runtime.yml", "path to runtime config")
	targetOS := flag.String("os", runtime.GOOS, "target OS")
	targetArch := flag.String("arch", runtime.GOARCH, "target arch")
	outputDir := flag.String("out", "", "output directory")
	flag.Parse()

	cfg, err := loadRuntimeConfig(*configPath)
	if err != nil {
		fail(err)
	}
	if cfg.OpenClawVersion == "" {
		fail(fmt.Errorf("openclawVersion is required in %s", *configPath))
	}
	if cfg.NodeVersion == "" {
		fail(fmt.Errorf("nodeVersion is required in %s", *configPath))
	}

	target, err := bundleTarget(*targetOS, *targetArch)
	if err != nil {
		fail(err)
	}

	out := *outputDir
	if out == "" {
		out = filepath.Join("build", "openclaw-runtime", target.dirName())
	}

	if err := bundleRuntime(cfg, target, out); err != nil {
		fail(err)
	}
}

type targetSpec struct {
	goos        string
	goarch      string
	npmPlatform string
	npmArch     string
	nodeOS      string
	nodeArch    string
}

func (t targetSpec) dirName() string {
	return t.goos + "-" + t.goarch
}

func bundleTarget(goos, goarch string) (targetSpec, error) {
	switch goos {
	case "windows":
		switch goarch {
		case "amd64":
			return targetSpec{goos: goos, goarch: goarch, npmPlatform: "win32", npmArch: "x64", nodeOS: "win", nodeArch: "x64"}, nil
		case "arm64":
			return targetSpec{goos: goos, goarch: goarch, npmPlatform: "win32", npmArch: "arm64", nodeOS: "win", nodeArch: "arm64"}, nil
		}
	case "darwin":
		switch goarch {
		case "amd64":
			return targetSpec{goos: goos, goarch: goarch, npmPlatform: "darwin", npmArch: "x64", nodeOS: "darwin", nodeArch: "x64"}, nil
		case "arm64":
			return targetSpec{goos: goos, goarch: goarch, npmPlatform: "darwin", npmArch: "arm64", nodeOS: "darwin", nodeArch: "arm64"}, nil
		}
	case "linux":
		switch goarch {
		case "amd64":
			return targetSpec{goos: goos, goarch: goarch, npmPlatform: "linux", npmArch: "x64", nodeOS: "linux", nodeArch: "x64"}, nil
		case "arm64":
			return targetSpec{goos: goos, goarch: goarch, npmPlatform: "linux", npmArch: "arm64", nodeOS: "linux", nodeArch: "arm64"}, nil
		}
	}
	return targetSpec{}, fmt.Errorf("unsupported bundle target %s/%s", goos, goarch)
}

func isBundleFresh(cfg runtimeConfig, target targetSpec, outputDir string) bool {
	data, err := os.ReadFile(filepath.Join(outputDir, "manifest.json"))
	if err != nil {
		return false
	}
	var m runtimeManifest
	if err := json.Unmarshal(data, &m); err != nil {
		return false
	}
	if m.OpenClawVersion != cfg.OpenClawVersion || m.NodeVersion != cfg.NodeVersion ||
		m.Platform != target.goos || m.Arch != target.goarch {
		return false
	}
	return verifyRuntimeAssets(outputDir) == nil
}

// copyNpmGlobalLib copies npm global install output into the bundle layout (outputDir/lib/node_modules/...).
// Unix npm uses prefix/lib/node_modules; Windows npm may use prefix/node_modules instead.
func copyNpmGlobalLib(installPrefix, outputDir string) error {
	libRoot := filepath.Join(installPrefix, "lib")
	libNodeModules := filepath.Join(libRoot, "node_modules")
	if _, err := os.Stat(libNodeModules); err == nil {
		return copyNpmTreeWithProgress(libRoot, filepath.Join(outputDir, "lib"))
	}
	flatNodeModules := filepath.Join(installPrefix, "node_modules")
	if _, err := os.Stat(flatNodeModules); err == nil {
		dst := filepath.Join(outputDir, "lib", "node_modules")
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		return copyNpmTreeWithProgress(flatNodeModules, dst)
	}
	return fmt.Errorf("npm global install produced neither %s nor %s", libNodeModules, flatNodeModules)
}

func copyNpmTreeWithProgress(srcRoot, dstRoot string) error {
	fmt.Printf("Scanning npm tree to count files (one-time pass)...\n")
	total, err := countTreeFiles(srcRoot)
	if err != nil {
		return fmt.Errorf("count files under %s: %w", srcRoot, err)
	}
	prog := &copyProgress{total: total, label: "npm payload"}
	fmt.Printf("Copying npm install into bundle (%d files). Do not interrupt; progress updates on the line below.\n", total)
	if err := copyDirImpl(srcRoot, dstRoot, prog); err != nil {
		return err
	}
	prog.finishLine()
	return nil
}

// countTreeFiles counts regular files and symlinks (same units as copyDirImpl progress).
func countTreeFiles(root string) (int64, error) {
	var n int64
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		mode := info.Mode()
		if mode.IsRegular() || mode&os.ModeSymlink != 0 {
			n++
		}
		return nil
	})
	return n, err
}

type copyProgress struct {
	total   int64
	done    atomic.Int64
	label   string
	mu      sync.Mutex
	lastLog time.Time
}

func (p *copyProgress) bump() {
	if p == nil {
		return
	}
	n := p.done.Add(1)
	now := time.Now()
	p.mu.Lock()
	defer p.mu.Unlock()
	// Throttle: every 400 files, on last file, or every ~1.5s
	if p.total > 0 {
		pct := 100.0 * float64(n) / float64(p.total)
		if n%400 != 0 && n != p.total && now.Sub(p.lastLog) < 1500*time.Millisecond {
			return
		}
		p.lastLog = now
		fmt.Fprintf(os.Stdout, "\r  %s: %d / %d files (%.1f%%)    ", p.label, n, p.total, pct)
		return
	}
	if n%600 != 0 && now.Sub(p.lastLog) < 2*time.Second {
		return
	}
	p.lastLog = now
	fmt.Fprintf(os.Stdout, "\r  %s: copied %d files...    ", p.label, n)
}

func (p *copyProgress) finishLine() {
	if p == nil {
		return
	}
	n := p.done.Load()
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.total > 0 {
		fmt.Fprintf(os.Stdout, "\r  %s: %d / %d files (100.0%%) — copy finished\n", p.label, n, p.total)
		return
	}
	fmt.Fprintf(os.Stdout, "\r  %s: copied %d files — copy finished\n", p.label, n)
}

func bundleRuntime(cfg runtimeConfig, target targetSpec, outputDir string) error {
	if isBundleFresh(cfg, target, outputDir) {
		fmt.Printf("OpenClaw runtime %s (Node %s) already up-to-date at %s, skipping bundle\n",
			cfg.OpenClawVersion, cfg.NodeVersion, outputDir)
		return nil
	}

	tmpDir, err := os.MkdirTemp("", "chatclaw-openclawbundle-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)

	installPrefix := filepath.Join(tmpDir, "prefix")
	fmt.Printf("Installing openclaw@%s via npm (global, temp prefix)...\n", cfg.OpenClawVersion)
	if err := installOpenClaw(cfg, target, installPrefix); err != nil {
		return err
	}

	fmt.Printf("Downloading Node.js %s for %s...\n", cfg.NodeVersion, target.dirName())
	nodeArchive, err := downloadNodeArchive(cfg.NodeVersion, target, tmpDir)
	if err != nil {
		return err
	}

	if err := os.RemoveAll(outputDir); err != nil {
		return fmt.Errorf("remove old bundle: %w", err)
	}
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return fmt.Errorf("create output dir: %w", err)
	}

	fmt.Printf("Extracting Node.js into bundle...\n")
	if err := extractNodeArchive(nodeArchive, filepath.Join(outputDir, "tools", "node")); err != nil {
		return err
	}
	if err := copyNpmGlobalLib(installPrefix, outputDir); err != nil {
		return err
	}
	fmt.Printf("Writing CLI wrappers and manifest...\n")
	if err := os.MkdirAll(filepath.Join(outputDir, "bin"), 0o755); err != nil {
		return err
	}
	if err := writeCLIWrappers(outputDir, target.goos); err != nil {
		return err
	}
	if err := pruneBundle(outputDir); err != nil {
		return err
	}

	manifest := runtimeManifest{
		OpenClawVersion: cfg.OpenClawVersion,
		NodeVersion:     cfg.NodeVersion,
		Platform:        target.goos,
		Arch:            target.goarch,
		Target:          target.dirName(),
		BuiltAt:         time.Now().UTC().Format(time.RFC3339),
	}
	if err := writeJSON(filepath.Join(outputDir, "manifest.json"), manifest); err != nil {
		return err
	}

	fmt.Printf("Running smoke check (openclaw --version)...\n")
	if err := smokeCheck(outputDir, target, manifest.OpenClawVersion); err != nil {
		return err
	}

	fmt.Printf("Bundled OpenClaw runtime %s with Node %s at %s\n", cfg.OpenClawVersion, cfg.NodeVersion, outputDir)
	return nil
}

func installOpenClaw(cfg runtimeConfig, target targetSpec, installPrefix string) error {
	if _, err := exec.LookPath("npm"); err != nil {
		return fmt.Errorf("npm is required to bundle OpenClaw runtime: %w", err)
	}

	args := []string{
		"install", "-g",
		"--prefix", installPrefix,
		"--loglevel", "error",
		"--no-fund", "--no-audit",
		"openclaw@" + cfg.OpenClawVersion,
	}
	cmd := exec.Command("npm", args...)
	cmd.Env = append(os.Environ(),
		"SHARP_IGNORE_GLOBAL_LIBVIPS=1",
		"NODE_LLAMA_CPP_SKIP_DOWNLOAD=1",
		"NPM_CONFIG_LOGLEVEL=error",
		"NPM_CONFIG_FUND=false",
		"NPM_CONFIG_AUDIT=false",
		"npm_config_platform="+target.npmPlatform,
		"npm_config_arch="+target.npmArch,
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("install openclaw runtime: %w", err)
	}
	return nil
}

func downloadNodeArchive(nodeVersion string, target targetSpec, tmpDir string) (string, error) {
	archiveName := nodeArchiveName(nodeVersion, target)
	baseURL := fmt.Sprintf("%s/v%s", nodeDistBaseURL, nodeVersion)
	shaURL := baseURL + "/SHASUMS256.txt"
	archiveURL := baseURL + "/" + archiveName

	shaPath := filepath.Join(tmpDir, "SHASUMS256.txt")
	if err := downloadFile(shaURL, shaPath); err != nil {
		return "", err
	}
	expectedSHA, err := lookupSHA256(shaPath, archiveName)
	if err != nil {
		return "", err
	}

	archivePath := filepath.Join(tmpDir, archiveName)
	if err := downloadFile(archiveURL, archivePath); err != nil {
		return "", err
	}

	actualSHA, err := sha256File(archivePath)
	if err != nil {
		return "", err
	}
	if !strings.EqualFold(expectedSHA, actualSHA) {
		return "", fmt.Errorf("node archive checksum mismatch for %s", archiveName)
	}
	return archivePath, nil
}

func nodeArchiveName(nodeVersion string, target targetSpec) string {
	if target.goos == "windows" {
		return fmt.Sprintf("node-v%s-%s-%s.zip", nodeVersion, target.nodeOS, target.nodeArch)
	}
	return fmt.Sprintf("node-v%s-%s-%s.tar.gz", nodeVersion, target.nodeOS, target.nodeArch)
}

func writeCLIWrappers(outputDir, goos string) error {
	if goos == "windows" {
		content := strings.Join([]string{
			"@echo off",
			"setlocal",
			`set "SCRIPT_DIR=%~dp0"`,
			"set OPENCLAW_EMBEDDED_IN=ChatClaw",
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

func smokeCheck(outputDir string, target targetSpec, expectedVersion string) error {
	if err := verifyRuntimeAssets(outputDir); err != nil {
		return err
	}

	if target.goos != runtime.GOOS || target.goarch != runtime.GOARCH {
		return nil
	}

	commandPath := filepath.Join(outputDir, "bin", "openclaw")
	if target.goos == "windows" {
		commandPath += ".cmd"
	}

	versionOutput, err := exec.Command(commandPath, "--version").CombinedOutput()
	if err != nil {
		return fmt.Errorf("verify bundled openclaw version: %w: %s", err, strings.TrimSpace(string(versionOutput)))
	}
	version, err := parseOpenClawVersionOutput(string(versionOutput))
	if err != nil {
		return err
	}
	if version != expectedVersion {
		return fmt.Errorf("bundled openclaw version mismatch: got %s want %s", version, expectedVersion)
	}

	helpOutput, err := exec.Command(commandPath, "gateway", "run", "--help").CombinedOutput()
	if err != nil {
		return fmt.Errorf("verify bundled gateway CLI: %w: %s", err, strings.TrimSpace(string(helpOutput)))
	}
	return nil
}

func verifyRuntimeAssets(outputDir string) error {
	requiredPaths := []string{
		filepath.Join(outputDir, "lib", "node_modules", "openclaw", "docs", "reference", "templates", "AGENTS.md"),
	}
	for _, path := range requiredPaths {
		info, err := os.Stat(path)
		if err != nil {
			return fmt.Errorf("required bundled runtime asset is missing: %s", path)
		}
		if info.IsDir() {
			return fmt.Errorf("required bundled runtime asset is a directory: %s", path)
		}
	}
	return nil
}

func pruneBundle(outputDir string) error {
	for _, path := range []string{
		filepath.Join(outputDir, "lib", "node_modules", "openclaw", "README.md"),
		filepath.Join(outputDir, "lib", "node_modules", "openclaw", "CHANGELOG.md"),
		filepath.Join(outputDir, "lib", "node_modules", "openclaw", "test"),
		filepath.Join(outputDir, "lib", "node_modules", "openclaw", "tests"),
	} {
		_ = os.RemoveAll(path)
	}
	return nil
}

// --- Archive extraction ---

func extractNodeArchive(archivePath, destination string) error {
	if err := os.RemoveAll(destination); err != nil {
		return err
	}
	if err := os.MkdirAll(destination, 0o755); err != nil {
		return err
	}

	switch {
	case strings.HasSuffix(archivePath, ".tar.gz"):
		return extractTarGz(archivePath, destination)
	case strings.HasSuffix(archivePath, ".zip"):
		return extractZip(archivePath, destination)
	default:
		return fmt.Errorf("unsupported archive format %s", archivePath)
	}
}

func extractTarGz(archivePath, destination string) error {
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
		header, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}

		relativePath, ok := stripArchiveRoot(header.Name)
		if !ok {
			continue
		}
		targetPath := filepath.Join(destination, relativePath)

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(targetPath, 0o755); err != nil {
				return err
			}
		case tar.TypeReg, tar.TypeRegA:
			if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
				return err
			}
			fileMode := os.FileMode(header.Mode)
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
			if err := os.Symlink(header.Linkname, targetPath); err != nil && !os.IsExist(err) {
				return err
			}
		}
	}
}

func extractZip(archivePath, destination string) error {
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
		targetPath := filepath.Join(destination, relativePath)
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

func stripArchiveRoot(name string) (string, bool) {
	name = filepath.ToSlash(name)
	name = strings.TrimPrefix(name, "./")
	parts := strings.SplitN(name, "/", 2)
	if len(parts) < 2 || parts[1] == "" {
		return "", false
	}
	return filepath.FromSlash(parts[1]), true
}

// --- Filesystem helpers ---

func copyDir(src, dst string) error {
	return copyDirImpl(src, dst, nil)
}

func copyDirImpl(src, dst string, prog *copyProgress) error {
	entries, err := os.ReadDir(src)
	if err != nil {
		return fmt.Errorf("read dir %s: %w", src, err)
	}
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return err
	}
	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		dstPath := filepath.Join(dst, entry.Name())

		info, err := entry.Info()
		if err != nil {
			return err
		}
		switch mode := info.Mode(); {
		case mode.IsDir():
			if err := copyDirImpl(srcPath, dstPath, prog); err != nil {
				return err
			}
		case mode&os.ModeSymlink != 0:
			linkTarget, err := os.Readlink(srcPath)
			if err != nil {
				return err
			}
			if err := os.Symlink(linkTarget, dstPath); err != nil && !os.IsExist(err) {
				return err
			}
			prog.bump()
		case mode.IsRegular():
			if err := copyFile(srcPath, dstPath, info.Mode()); err != nil {
				return err
			}
			prog.bump()
		}
	}
	return nil
}

func copyFile(src, dst string, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// --- Config / download helpers ---

func loadRuntimeConfig(path string) (runtimeConfig, error) {
	f, err := os.Open(path)
	if err != nil {
		return runtimeConfig{}, err
	}
	defer f.Close()

	var cfg runtimeConfig
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = parseYAMLScalar(value)
		switch key {
		case "openclawVersion":
			cfg.OpenClawVersion = value
		case "nodeVersion":
			cfg.NodeVersion = value
		}
	}
	return cfg, scanner.Err()
}

func parseYAMLScalar(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	if strings.HasPrefix(s, "\"") && strings.HasSuffix(s, "\"") && len(s) >= 2 {
		return strings.TrimSpace(s[1 : len(s)-1])
	}
	if strings.HasPrefix(s, "'") && strings.HasSuffix(s, "'") && len(s) >= 2 {
		return strings.TrimSpace(s[1 : len(s)-1])
	}
	if i := strings.IndexByte(s, '#'); i >= 0 {
		s = s[:i]
	}
	return strings.TrimSpace(s)
}

func downloadFile(url, destination string) error {
	resp, err := http.Get(url) //nolint:gosec
	if err != nil {
		return fmt.Errorf("download %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download %s: unexpected status %s", url, resp.Status)
	}
	file, err := os.Create(destination)
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = io.Copy(file, resp.Body)
	return err
}

func lookupSHA256(path, archiveName string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) != 2 {
			continue
		}
		if strings.TrimPrefix(fields[1], "*") == archiveName {
			return fields[0], nil
		}
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}
	return "", fmt.Errorf("checksum not found for %s", archiveName)
}

func sha256File(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	sum := sha256.New()
	if _, err := io.Copy(sum, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(sum.Sum(nil)), nil
}

func writeJSON(path string, value any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

func parseOpenClawVersionOutput(output string) (string, error) {
	for _, field := range strings.Fields(strings.TrimSpace(output)) {
		candidate := strings.Trim(field, "(),")
		candidate = strings.TrimPrefix(candidate, "v")
		if looksLikeVersionToken(candidate) {
			return candidate, nil
		}
	}
	version := strings.TrimSpace(output)
	if version == "" {
		return "", fmt.Errorf("openclaw version output was empty")
	}
	return "", fmt.Errorf("could not parse openclaw version from %q", version)
}

func looksLikeVersionToken(value string) bool {
	dotCount := 0
	for _, r := range value {
		switch {
		case r >= '0' && r <= '9':
		case r == '.':
			dotCount++
		default:
			return false
		}
	}
	return dotCount >= 2
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
