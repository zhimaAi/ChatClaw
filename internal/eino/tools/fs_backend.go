// Package tools provides filesystem tools for the agent's ReAct loop.
//
// The Backend type implements filesystem.Backend + filesystem.Shell, providing
// a unified interface for file operations and command execution that works
// in both native and sandbox (codex-cli) modes.
package tools

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/cloudwego/eino/adk/filesystem"
)

// BackendConfig configures the filesystem backend.
type BackendConfig struct {
	HomeDir    string
	WorkDir    string
	CodexBin   string // empty means no sandbox
	NetworkEnabled bool
	ToolchainBinDir string
}

// Backend implements filesystem.Backend and filesystem.Shell.
// In sandbox mode (CodexBin != ""), write and execute operations
// are routed through codex-cli for OS-level isolation.
type Backend struct {
	homeDir         string
	workDir         string
	codexBin        string
	networkEnabled  bool
	toolchainBinDir string
}

// Compile-time interface checks.
var (
	_ filesystem.Backend = (*Backend)(nil)
	_ filesystem.Shell   = (*Backend)(nil)
)

// NewBackend creates a filesystem backend.
func NewBackend(cfg *BackendConfig) *Backend {
	return &Backend{
		homeDir:         cfg.HomeDir,
		workDir:         cfg.WorkDir,
		codexBin:        cfg.CodexBin,
		networkEnabled:  cfg.NetworkEnabled,
		toolchainBinDir: cfg.ToolchainBinDir,
	}
}

func (b *Backend) HomeDir() string { return b.homeDir }
func (b *Backend) WorkDir() string { return b.workDir }
func (b *Backend) SandboxEnabled() bool { return b.codexBin != "" }
func (b *Backend) ToolchainBinDir() string { return b.toolchainBinDir }

// ---------------------------------------------------------------------------
// filesystem.Backend — File operations
// ---------------------------------------------------------------------------

func (b *Backend) LsInfo(_ context.Context, req *filesystem.LsInfoRequest) ([]filesystem.FileInfo, error) {
	path := req.Path
	if path == "" {
		path = b.homeDir
	}
	path = filepath.Clean(path)

	if b.IsSensitivePath(path) {
		return nil, fmt.Errorf("access denied: path contains sensitive credentials")
	}

	entries, err := os.ReadDir(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name() < entries[j].Name()
	})

	result := make([]filesystem.FileInfo, 0, len(entries))
	for _, e := range entries {
		fullPath := filepath.Join(path, e.Name())
		if b.IsSensitivePath(fullPath) {
			continue
		}
		fi := filesystem.FileInfo{
			Path:  fullPath,
			IsDir: e.IsDir(),
		}
		if info, infoErr := e.Info(); infoErr == nil {
			fi.Size = info.Size()
			fi.ModifiedAt = info.ModTime().Format("2006-01-02T15:04:05Z07:00")
		}
		result = append(result, fi)
	}
	return result, nil
}

func (b *Backend) Read(_ context.Context, req *filesystem.ReadRequest) (*filesystem.FileContent, error) {
	path := filepath.Clean(req.FilePath)

	if b.IsSensitivePath(path) {
		return nil, fmt.Errorf("access denied: path contains sensitive credentials")
	}

	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	// filesystem.ReadRequest.Offset is 1-based; convert to 0-based internally.
	offset := req.Offset - 1
	if offset < 0 {
		offset = 0
	}
	limit := req.Limit
	if limit <= 0 {
		limit = 2000
	}

	scanner := bufio.NewScanner(file)
	const maxCapacity = 1024 * 1024
	scanner.Buffer(make([]byte, maxCapacity), maxCapacity)

	var lines []string
	lineNum := 0
	for scanner.Scan() {
		if lineNum >= offset && len(lines) < limit {
			lines = append(lines, scanner.Text())
		}
		lineNum++
		if len(lines) >= limit {
			break
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	content := strings.Join(lines, "\n")
	if content == "" {
		return &filesystem.FileContent{Content: "(empty file)"}, nil
	}
	return &filesystem.FileContent{Content: content}, nil
}

func (b *Backend) Write(_ context.Context, req *filesystem.WriteRequest) error {
	path := filepath.Clean(req.FilePath)
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", dir, err)
	}
	return os.WriteFile(path, []byte(req.Content), 0o644)
}

func (b *Backend) Edit(_ context.Context, req *filesystem.EditRequest) error {
	path := filepath.Clean(req.FilePath)

	if req.OldString == "" {
		return fmt.Errorf("old_string is required")
	}
	if req.OldString == req.NewString {
		return fmt.Errorf("new_string must be different from old_string")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("failed to read file: %w", err)
	}

	content := string(data)
	count := strings.Count(content, req.OldString)
	if count == 0 {
		return fmt.Errorf("old_string not found in file")
	}
	if count > 1 && !req.ReplaceAll {
		return fmt.Errorf("old_string found %d times, use replace_all=true to replace all occurrences", count)
	}

	var newContent string
	if req.ReplaceAll {
		newContent = strings.ReplaceAll(content, req.OldString, req.NewString)
	} else {
		newContent = strings.Replace(content, req.OldString, req.NewString, 1)
	}
	return os.WriteFile(path, []byte(newContent), 0o644)
}

func (b *Backend) GrepRaw(ctx context.Context, req *filesystem.GrepRequest) ([]filesystem.GrepMatch, error) {
	basePath := req.Path
	if basePath == "" {
		basePath = b.homeDir
	}
	basePath = filepath.Clean(basePath)

	if b.IsSensitivePath(basePath) {
		return nil, fmt.Errorf("access denied: path contains sensitive credentials")
	}

	patternStr := req.Pattern
	if req.CaseInsensitive {
		patternStr = "(?i)" + patternStr
	}
	re, regexErr := regexp.Compile(patternStr)
	useLiteral := regexErr != nil
	literalPattern := req.Pattern
	if useLiteral && req.CaseInsensitive {
		literalPattern = strings.ToLower(req.Pattern)
	}

	const maxMatches = 500
	var matches []filesystem.GrepMatch

	err := filepath.WalkDir(basePath, func(p string, d os.DirEntry, err error) error {
		if ctx.Err() != nil {
			return filepath.SkipAll
		}
		if err != nil {
			if os.IsPermission(err) {
				return filepath.SkipDir
			}
			return nil
		}
		if b.IsSensitivePath(p) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			return nil
		}
		if req.Glob != "" {
			matched, matchErr := filepath.Match(req.Glob, d.Name())
			if matchErr != nil || !matched {
				return nil
			}
		}
		if IsBinaryFile(p) {
			return nil
		}

		data, readErr := os.ReadFile(p)
		if readErr != nil {
			return nil
		}
		lines := strings.Split(string(data), "\n")

		for i, line := range lines {
			matched := false
			if useLiteral {
				if req.CaseInsensitive {
					matched = strings.Contains(strings.ToLower(line), literalPattern)
				} else {
					matched = strings.Contains(line, req.Pattern)
				}
			} else {
				matched = re.MatchString(line)
			}
			if !matched {
				continue
			}
			matches = append(matches, filesystem.GrepMatch{
				Path:    p,
				Line:    i + 1,
				Content: line,
			})
			if len(matches) >= maxMatches {
				return filepath.SkipAll
			}
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("grep failed: %w", err)
	}
	return matches, nil
}

func (b *Backend) GlobInfo(ctx context.Context, req *filesystem.GlobInfoRequest) ([]filesystem.FileInfo, error) {
	basePath := req.Path
	if basePath == "" {
		basePath = b.homeDir
	}
	basePath = filepath.Clean(basePath)

	if b.IsSensitivePath(basePath) {
		return nil, fmt.Errorf("access denied: path contains sensitive credentials")
	}

	pattern := req.Pattern
	if pattern == "" {
		pattern = "*"
	}

	const maxResults = 500
	var results []filesystem.FileInfo

	if strings.Contains(pattern, "**") {
		regexPattern := regexp.QuoteMeta(pattern)
		regexPattern = strings.ReplaceAll(regexPattern, `\*\*`, `.*`)
		regexPattern = strings.ReplaceAll(regexPattern, `\*`, `[^/]*`)
		regexPattern = strings.ReplaceAll(regexPattern, `\?`, `.`)
		re, err := regexp.Compile("^" + regexPattern + "$")
		if err != nil {
			return nil, fmt.Errorf("invalid glob pattern: %w", err)
		}

		err = filepath.Walk(basePath, func(p string, info os.FileInfo, walkErr error) error {
			if ctx.Err() != nil {
				return filepath.SkipAll
			}
			if walkErr != nil {
				return nil
			}
			if b.IsSensitivePath(p) {
				if info.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
			relPath, _ := filepath.Rel(basePath, p)
			if re.MatchString(filepath.ToSlash(relPath)) {
				results = append(results, filesystem.FileInfo{Path: p, IsDir: info.IsDir()})
			}
			if len(results) >= maxResults {
				return filepath.SkipAll
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	} else {
		globMatches, err := filepath.Glob(filepath.Join(basePath, pattern))
		if err != nil {
			return nil, err
		}
		for _, m := range globMatches {
			if b.IsSensitivePath(m) {
				continue
			}
			info, statErr := os.Stat(m)
			isDir := false
			if statErr == nil {
				isDir = info.IsDir()
			}
			results = append(results, filesystem.FileInfo{Path: m, IsDir: isDir})
			if len(results) >= maxResults {
				break
			}
		}
	}

	return results, nil
}

// ---------------------------------------------------------------------------
// filesystem.Shell — Command execution
// ---------------------------------------------------------------------------

func (b *Backend) Execute(parentCtx context.Context, input *filesystem.ExecuteRequest) (*filesystem.ExecuteResponse, error) {
	if input.Command == "" {
		return nil, fmt.Errorf("command is required")
	}

	timeout := 60 * time.Second
	ctx, cancel := context.WithTimeout(parentCtx, timeout)
	defer cancel()

	var cmd *exec.Cmd
	if b.codexBin != "" {
		cmd = b.buildCodexCommand(input.Command)
	} else {
		cmd = buildNativeShellCommand(input.Command)
		cmd.Dir = b.workDir
	}
	b.applyToolchainEnv(cmd)
	setProcGroup(cmd)

	pr, pw, err := os.Pipe()
	if err != nil {
		return nil, err
	}
	cmd.Stdout = pw
	cmd.Stderr = pw

	if err := cmd.Start(); err != nil {
		pw.Close()
		pr.Close()
		return nil, fmt.Errorf("failed to start command: %w", err)
	}
	pw.Close()

	var buf bytes.Buffer
	readDone := make(chan struct{})
	go func() {
		_, _ = io.Copy(&buf, pr)
		close(readDone)
	}()

	waitDone := make(chan error, 1)
	go func() { waitDone <- cmd.Wait() }()

	var cmdErr error
	timedOut := false
	select {
	case cmdErr = <-waitDone:
	case <-ctx.Done():
		timedOut = true
		killProcessGroup(cmd)
		pr.Close()
		select {
		case cmdErr = <-waitDone:
		case <-time.After(5 * time.Second):
		}
	}

	pr.Close()
	select {
	case <-readDone:
	case <-time.After(2 * time.Second):
	}

	exitCode := 0
	if cmd.ProcessState != nil {
		exitCode = cmd.ProcessState.ExitCode()
	}

	const maxOutput = 128 * 1024
	output := buf.String()
	if len(output) > maxOutput {
		output = output[:maxOutput]
	}

	truncated := false
	if timedOut {
		output += fmt.Sprintf("\n[Command timed out after %s]", timeout)
		truncated = true
	}
	if cmdErr != nil && output == "" {
		output = cmdErr.Error()
	}

	return &filesystem.ExecuteResponse{
		Output:    output,
		ExitCode:  &exitCode,
		Truncated: truncated,
	}, nil
}

// ---------------------------------------------------------------------------
// Sandbox — codex-cli wrapping
// ---------------------------------------------------------------------------

// writableRoots lists home-relative directories that common package managers
// need write access to.
var writableRoots = []string{
	".npm", ".bun", ".cache", ".local", ".yarn", ".openclaw",
}

func (b *Backend) buildCodexCommand(command string) *exec.Cmd {
	platform := "macos"
	switch runtime.GOOS {
	case "linux":
		platform = "linux"
	case "windows":
		platform = "windows"
	}

	args := []string{"sandbox", platform, "--full-auto"}

	if b.networkEnabled {
		args = append(args, "-c", "sandbox_workspace_write.network_access=true")
	}

	roots := make([]string, 0, len(writableRoots))
	for _, rel := range writableRoots {
		roots = append(roots, fmt.Sprintf("%q", filepath.Join(b.homeDir, rel)))
	}
	args = append(args, "-c",
		fmt.Sprintf("sandbox_workspace_write.writable_roots=[%s]", strings.Join(roots, ",")))

	if runtime.GOOS == "windows" {
		wrappedCmd := "[Console]::OutputEncoding = [System.Text.Encoding]::UTF8; " +
			"$OutputEncoding = [System.Text.Encoding]::UTF8; " +
			command
		args = append(args, "--", "powershell.exe",
			"-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass",
			"-Command", wrappedCmd)
	} else {
		args = append(args, "--", "sh", "-c", command)
	}

	cmd := exec.Command(b.codexBin, args...)
	cmd.Dir = b.workDir
	return cmd
}

func buildNativeShellCommand(command string) *exec.Cmd {
	switch runtime.GOOS {
	case "windows":
		wrappedCmd := "[Console]::OutputEncoding = [System.Text.Encoding]::UTF8; " +
			"$OutputEncoding = [System.Text.Encoding]::UTF8; " +
			command
		return exec.Command("powershell.exe",
			"-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass",
			"-Command", wrappedCmd)
	case "darwin":
		return exec.Command("/bin/zsh", "-l", "-c", command)
	default:
		return exec.Command("/bin/bash", "-l", "-c", command)
	}
}

func (b *Backend) applyToolchainEnv(cmd *exec.Cmd) {
	if b.toolchainBinDir == "" {
		return
	}
	env := os.Environ()
	pathKey := "PATH"
	found := false
	for i, e := range env {
		if strings.HasPrefix(e, pathKey+"=") {
			env[i] = pathKey + "=" + b.toolchainBinDir + string(filepath.ListSeparator) + e[len(pathKey)+1:]
			found = true
			break
		}
	}
	if !found {
		env = append(env, pathKey+"="+b.toolchainBinDir)
	}
	cmd.Env = env
}

// ---------------------------------------------------------------------------
// Sensitive path protection
// ---------------------------------------------------------------------------

// sensitiveHomeDirs are home-relative directories that contain credentials,
// private keys, or other secrets. Access to these paths is blocked for both
// read and write operations performed by the sandbox agent.
var sensitiveHomeDirs = []string{
	".ssh",
	".gnupg",
	".gpg",
	".aws",
	".azure",
	".config/gcloud",
	".kube",
	".docker",
	".npmrc",
	".pypirc",
	".gem/credentials",
	".netrc",
	".git-credentials",
	".config/gh",
	".config/hub",
	".local/share/keyrings",
	".password-store",
	".vault-token",
	".config/op",
	".1password",
}

// sensitiveFileNames are specific file names (matched anywhere) that should
// be blocked regardless of directory.
var sensitiveFileNames = []string{
	".env",
	".env.local",
	".env.production",
	".env.staging",
	"credentials.json",
	"service-account.json",
	"secrets.yaml",
	"secrets.yml",
}

// IsSensitivePath reports whether the given absolute path falls inside a
// sensitive directory or matches a sensitive file name relative to homeDir.
// This check only applies in sandbox mode; native mode has full access.
func (b *Backend) IsSensitivePath(absPath string) bool {
	if !b.SandboxEnabled() {
		return false
	}

	cleaned := filepath.Clean(absPath)
	homeClean := filepath.Clean(b.homeDir)

	rel, err := filepath.Rel(homeClean, cleaned)
	if err != nil {
		return false
	}
	relSlash := filepath.ToSlash(rel)

	for _, dir := range sensitiveHomeDirs {
		dirSlash := filepath.ToSlash(dir)
		if relSlash == dirSlash || strings.HasPrefix(relSlash, dirSlash+"/") {
			return true
		}
	}

	baseName := filepath.Base(cleaned)
	for _, name := range sensitiveFileNames {
		if strings.EqualFold(baseName, name) {
			return true
		}
	}

	return false
}

// ---------------------------------------------------------------------------
// Path resolution helpers (used by tool wrappers)
// ---------------------------------------------------------------------------

// ResolvePath converts a path to an absolute filesystem path.
// It rejects paths that escape homeDir or target sensitive locations.
func (b *Backend) ResolvePath(p string) (string, error) {
	if p == "" || p == "/" {
		return b.homeDir, nil
	}

	var resolved string
	if filepath.IsAbs(p) {
		resolved = p
	} else {
		resolved = filepath.Join(b.homeDir, strings.TrimPrefix(strings.TrimPrefix(p, "/"), "\\"))
	}

	absResolved, err := filepath.Abs(resolved)
	if err != nil {
		return "", fmt.Errorf("failed to resolve path: %w", err)
	}
	absBase, err := filepath.Abs(b.homeDir)
	if err != nil {
		return "", fmt.Errorf("failed to resolve home dir: %w", err)
	}

	rel, err := filepath.Rel(filepath.Clean(absBase), filepath.Clean(absResolved))
	if err != nil || strings.HasPrefix(rel, "..") {
		return "", fmt.Errorf("path escapes home directory: %s", p)
	}

	if b.IsSensitivePath(absResolved) {
		return "", fmt.Errorf("access denied: %q is a sensitive path containing credentials or secrets", p)
	}

	return absResolved, nil
}

// ResolveWritePath resolves a path for write operations.
// When sandbox is enabled, the path must be within WorkDir.
func (b *Backend) ResolveWritePath(p string) (string, error) {
	absPath, err := b.ResolvePath(p)
	if err != nil {
		return "", err
	}

	if b.codexBin == "" {
		return absPath, nil
	}

	absSandbox, err := filepath.Abs(b.workDir)
	if err != nil {
		return "", fmt.Errorf("failed to resolve work dir: %w", err)
	}

	rel, err := filepath.Rel(filepath.Clean(absSandbox), filepath.Clean(absPath))
	if err != nil || strings.HasPrefix(rel, "..") {
		return "", fmt.Errorf(
			"write blocked: path %q is outside the working directory %q — all write operations must target paths within the working directory",
			p, absSandbox,
		)
	}
	return absPath, nil
}
