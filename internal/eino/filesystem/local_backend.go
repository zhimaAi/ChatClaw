// Package filesystem provides a local filesystem backend for the eino ADK filesystem middleware.
package filesystem

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/cloudwego/eino/adk/filesystem"
)

// ShellPolicy defines security constraints for shell command execution.
type ShellPolicy struct {
	TrustedDirs     []string      // Allowed working directories. Empty = no restriction.
	BlockedCommands []string      // Rejected command patterns (substring match).
	DefaultTimeout  time.Duration // Max execution time per command. 0 = 120s default.
}

// LocalBackend implements filesystem.Backend and filesystem.ShellBackend
// using the real local filesystem, rooted at a base directory (typically user's home).
type LocalBackend struct {
	baseDir string
	policy  *ShellPolicy
}

// LocalBackendConfig configures the LocalBackend.
type LocalBackendConfig struct {
	BaseDir     string       // Root for all filesystem ops. Empty = user home directory.
	ShellPolicy *ShellPolicy // Security constraints. Nil = no restrictions.
}

// NewLocalBackend creates a new LocalBackend with the given configuration.
func NewLocalBackend(config *LocalBackendConfig) (*LocalBackend, error) {
	baseDir := config.BaseDir
	if baseDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("failed to get user home directory: %w", err)
		}
		baseDir = home
	}

	info, err := os.Stat(baseDir)
	if err != nil {
		return nil, fmt.Errorf("failed to stat base directory: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("base path is not a directory: %s", baseDir)
	}

	return &LocalBackend{baseDir: baseDir, policy: config.ShellPolicy}, nil
}

// BaseDir returns the root directory for all filesystem operations.
func (b *LocalBackend) BaseDir() string {
	return b.baseDir
}

// resolvePath converts a path to an absolute filesystem path.
// Absolute OS paths (e.g. "C:\foo", "/home/user/foo") are used directly.
// Relative paths and "/" are resolved against baseDir.
// Returns an error if the result escapes baseDir.
func (b *LocalBackend) resolvePath(p string) (string, error) {
	if p == "" || p == "/" {
		return b.baseDir, nil
	}

	var resolved string
	if filepath.IsAbs(p) {
		resolved = p
	} else {
		cleanPath := strings.TrimPrefix(p, "/")
		cleanPath = strings.TrimPrefix(cleanPath, "\\")
		resolved = filepath.Join(b.baseDir, cleanPath)
	}

	absResolved, err := filepath.Abs(resolved)
	if err != nil {
		return "", fmt.Errorf("failed to resolve path: %w", err)
	}
	absBase, err := filepath.Abs(b.baseDir)
	if err != nil {
		return "", fmt.Errorf("failed to resolve base path: %w", err)
	}

	rel, err := filepath.Rel(filepath.Clean(absBase), filepath.Clean(absResolved))
	if err != nil || strings.HasPrefix(rel, "..") {
		return "", fmt.Errorf("path escapes base directory: %s", p)
	}

	return absResolved, nil
}

// toAPIPath returns the display path shown to the LLM.
// Uses the real OS absolute path so all tools see consistent paths.
func (b *LocalBackend) toAPIPath(fsPath string) string {
	return fsPath
}

// LsInfo lists file/directory information at the given path.
func (b *LocalBackend) LsInfo(ctx context.Context, req *filesystem.LsInfoRequest) ([]filesystem.FileInfo, error) {
	targetPath, err := b.resolvePath(req.Path)
	if err != nil {
		return nil, err
	}

	info, err := os.Stat(targetPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("path does not exist: %s", req.Path)
		}
		return nil, fmt.Errorf("failed to stat path: %w", err)
	}

	if !info.IsDir() {
		return []filesystem.FileInfo{{Path: b.formatFileEntry(targetPath, info)}}, nil
	}

	entries, err := os.ReadDir(targetPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read directory: %w", err)
	}

	var result []filesystem.FileInfo
	for _, entry := range entries {
		fullPath := filepath.Join(targetPath, entry.Name())
		entryInfo, err := entry.Info()
		if err != nil {
			result = append(result, filesystem.FileInfo{Path: b.toAPIPath(fullPath)})
			continue
		}
		result = append(result, filesystem.FileInfo{Path: b.formatFileEntry(fullPath, entryInfo)})
	}

	if len(result) == 0 {
		return []filesystem.FileInfo{{Path: "(empty directory)"}}, nil
	}
	return result, nil
}

// formatFileEntry formats: "[type] path  size  modified_time"
func (b *LocalBackend) formatFileEntry(fsPath string, info os.FileInfo) string {
	apiPath := b.toAPIPath(fsPath)

	typeIndicator := "[file]"
	if info.IsDir() {
		typeIndicator = "[dir] "
	} else if info.Mode()&os.ModeSymlink != 0 {
		typeIndicator = "[link]"
	}

	modTime := info.ModTime().Format("2006-01-02 15:04")

	if !info.IsDir() {
		return fmt.Sprintf("%s %s  %s  %s", typeIndicator, apiPath, formatSize(info.Size()), modTime)
	}
	return fmt.Sprintf("%s %s  %s", typeIndicator, apiPath, modTime)
}

func formatSize(bytes int64) string {
	const (
		KB = 1024
		MB = KB * 1024
		GB = MB * 1024
	)
	switch {
	case bytes >= GB:
		return fmt.Sprintf("%.1fG", float64(bytes)/GB)
	case bytes >= MB:
		return fmt.Sprintf("%.1fM", float64(bytes)/MB)
	case bytes >= KB:
		return fmt.Sprintf("%.1fK", float64(bytes)/KB)
	default:
		return fmt.Sprintf("%dB", bytes)
	}
}

// Read reads file content with line-based offset and limit.
func (b *LocalBackend) Read(ctx context.Context, req *filesystem.ReadRequest) (string, error) {
	filePath, err := b.resolvePath(req.FilePath)
	if err != nil {
		return "", err
	}

	file, err := os.Open(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	offset := req.Offset
	if offset < 0 {
		offset = 0
	}
	limit := req.Limit
	if limit <= 0 {
		limit = 200
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
		return "", fmt.Errorf("failed to read file: %w", err)
	}

	content := strings.Join(lines, "\n")
	if content == "" {
		return "(empty file)", nil
	}
	return content, nil
}

// GrepRaw searches for a pattern in files under the given path.
// Pattern is treated as regex; falls back to literal match on invalid regex.
func (b *LocalBackend) GrepRaw(ctx context.Context, req *filesystem.GrepRequest) ([]filesystem.GrepMatch, error) {
	basePath, err := b.resolvePath(req.Path)
	if err != nil {
		return nil, err
	}

	var matches []filesystem.GrepMatch
	const maxMatches = 100

	err = filepath.Walk(basePath, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		if req.Glob != "" {
			matched, err := filepath.Match(req.Glob, filepath.Base(path))
			if err != nil || !matched {
				return nil
			}
		}
		if isBinaryFile(path) {
			return nil
		}
		fileMatches, err := grepFile(path, req.Pattern, b)
		if err != nil {
			return nil
		}
		matches = append(matches, fileMatches...)
		if len(matches) >= maxMatches {
			return filepath.SkipAll
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("grep failed: %w", err)
	}

	if len(matches) == 0 {
		return []filesystem.GrepMatch{{Path: "(no matches found)", Line: 0, Content: ""}}, nil
	}
	return matches, nil
}

func grepFile(filePath, pattern string, b *LocalBackend) ([]filesystem.GrepMatch, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	re, regexErr := regexp.Compile(pattern)
	useLiteral := regexErr != nil

	var matches []filesystem.GrepMatch
	scanner := bufio.NewScanner(file)
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := scanner.Text()
		matched := false
		if useLiteral {
			matched = strings.Contains(line, pattern)
		} else {
			matched = re.MatchString(line)
		}
		if matched {
			matches = append(matches, filesystem.GrepMatch{
				Path:    b.toAPIPath(filePath),
				Line:    lineNum,
				Content: line,
			})
		}
	}
	return matches, scanner.Err()
}

func isBinaryFile(path string) bool {
	binaryExts := map[string]bool{
		".exe": true, ".dll": true, ".so": true, ".dylib": true,
		".zip": true, ".tar": true, ".gz": true, ".bz2": true, ".xz": true,
		".png": true, ".jpg": true, ".jpeg": true, ".gif": true, ".webp": true,
		".mp3": true, ".mp4": true, ".avi": true, ".mov": true,
		".pdf": true, ".doc": true, ".docx": true, ".xls": true, ".xlsx": true,
		".bin": true, ".dat": true, ".db": true, ".sqlite": true,
	}
	return binaryExts[strings.ToLower(filepath.Ext(path))]
}

// GlobInfo returns file paths matching the glob pattern.
func (b *LocalBackend) GlobInfo(ctx context.Context, req *filesystem.GlobInfoRequest) ([]filesystem.FileInfo, error) {
	basePath, err := b.resolvePath(req.Path)
	if err != nil {
		return nil, err
	}

	pattern := req.Pattern
	if pattern == "" {
		pattern = "*"
	}

	var matches []string
	if strings.Contains(pattern, "**") {
		matches, err = globRecursive(basePath, pattern)
	} else {
		matches, err = filepath.Glob(filepath.Join(basePath, pattern))
	}
	if err != nil {
		return nil, fmt.Errorf("glob failed: %w", err)
	}

	var result []filesystem.FileInfo
	for _, m := range matches {
		result = append(result, filesystem.FileInfo{Path: b.toAPIPath(m)})
	}

	if len(result) == 0 {
		return []filesystem.FileInfo{{Path: "(no matches found)"}}, nil
	}
	return result, nil
}

func globRecursive(basePath, pattern string) ([]string, error) {
	var matches []string
	const maxResults = 500

	regexPattern := regexp.QuoteMeta(pattern)
	regexPattern = strings.ReplaceAll(regexPattern, `\*\*`, `.*`)
	regexPattern = strings.ReplaceAll(regexPattern, `\*`, `[^/]*`)
	regexPattern = strings.ReplaceAll(regexPattern, `\?`, `.`)
	re, err := regexp.Compile("^" + regexPattern + "$")
	if err != nil {
		return nil, fmt.Errorf("invalid glob pattern: %w", err)
	}

	err = filepath.Walk(basePath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		relPath, _ := filepath.Rel(basePath, path)
		if re.MatchString(filepath.ToSlash(relPath)) {
			matches = append(matches, path)
		}
		if len(matches) >= maxResults {
			return filepath.SkipAll
		}
		return nil
	})
	return matches, err
}

// Write creates or updates file content.
func (b *LocalBackend) Write(ctx context.Context, req *filesystem.WriteRequest) error {
	filePath, err := b.resolvePath(req.FilePath)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}
	return os.WriteFile(filePath, []byte(req.Content), 0o644)
}

// Edit replaces string occurrences in a file.
func (b *LocalBackend) Edit(ctx context.Context, req *filesystem.EditRequest) error {
	if req.OldString == "" {
		return fmt.Errorf("old_string cannot be empty")
	}
	if req.OldString == req.NewString {
		return fmt.Errorf("old_string and new_string are identical")
	}

	filePath, err := b.resolvePath(req.FilePath)
	if err != nil {
		return err
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("failed to read file: %w", err)
	}

	content := string(data)
	count := strings.Count(content, req.OldString)
	if count == 0 {
		return fmt.Errorf("old_string not found in file")
	}
	if !req.ReplaceAll && count > 1 {
		return fmt.Errorf("old_string found %d times, use replace_all=true to replace all occurrences", count)
	}

	var newContent string
	if req.ReplaceAll {
		newContent = strings.ReplaceAll(content, req.OldString, req.NewString)
	} else {
		newContent = strings.Replace(content, req.OldString, req.NewString, 1)
	}
	return os.WriteFile(filePath, []byte(newContent), 0o644)
}

// Execute runs a shell command and returns its output.
// Shell: powershell on Windows, zsh on macOS, bash on Linux.
// Working directory: baseDir.
func (b *LocalBackend) Execute(ctx context.Context, req *filesystem.ExecuteRequest) (*filesystem.ExecuteResponse, error) {
	if err := b.validateCommand(req.Command); err != nil {
		exitCode := -1
		return &filesystem.ExecuteResponse{
			Output:   "Command blocked: " + err.Error(),
			ExitCode: &exitCode,
		}, nil
	}

	timeout := 120 * time.Second
	if b.policy != nil && b.policy.DefaultTimeout > 0 {
		timeout = b.policy.DefaultTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		// Prepend UTF-8 encoding directives so Chinese and other non-ASCII
		// characters in command output are not garbled.  Both the .NET
		// Console encoding and PowerShell's $OutputEncoding must be set
		// because they govern different output paths.
		wrappedCmd := "[Console]::OutputEncoding = [System.Text.Encoding]::UTF8; " +
			"$OutputEncoding = [System.Text.Encoding]::UTF8; " +
			req.Command
		cmd = exec.CommandContext(ctx, "powershell.exe",
			"-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass",
			"-Command", wrappedCmd,
		)
	case "darwin":
		cmd = exec.CommandContext(ctx, "/bin/zsh", "-c", req.Command)
	default:
		cmd = exec.CommandContext(ctx, "/bin/bash", "-c", req.Command)
	}
	cmd.Dir = b.baseDir

	output, err := cmd.CombinedOutput()

	exitCode := 0
	if cmd.ProcessState != nil {
		exitCode = cmd.ProcessState.ExitCode()
	}

	const maxOutput = 128 * 1024
	outputStr := string(output)
	truncated := false
	if len(outputStr) > maxOutput {
		outputStr = outputStr[:maxOutput]
		truncated = true
	}

	if ctx.Err() == context.DeadlineExceeded {
		outputStr += "\n[Command timed out]"
	}

	if err != nil && len(outputStr) == 0 {
		outputStr = err.Error()
	}

	// Always include exit code — some LLM APIs reject empty tool result content.
	if outputStr == "" {
		outputStr = fmt.Sprintf("[exit code: %d]", exitCode)
	} else {
		outputStr = fmt.Sprintf("%s\n[exit code: %d]", outputStr, exitCode)
	}

	return &filesystem.ExecuteResponse{
		Output:    outputStr,
		ExitCode:  &exitCode,
		Truncated: truncated,
	}, nil
}

func (b *LocalBackend) validateCommand(command string) error {
	if b.policy == nil {
		return nil
	}
	for _, blocked := range b.policy.BlockedCommands {
		if strings.Contains(command, blocked) {
			return fmt.Errorf("command contains blocked pattern: %q", blocked)
		}
	}
	return nil
}

var _ filesystem.Backend = (*LocalBackend)(nil)
var _ filesystem.ShellBackend = (*LocalBackend)(nil)
