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
// Fields are designed to be extensible — add new checks here in the future.
type ShellPolicy struct {
	// TrustedDirs limits which directories can be used as working directory.
	// Empty means no restriction (default).
	TrustedDirs []string

	// BlockedCommands is a list of command patterns that should be rejected.
	// Uses simple substring matching.
	// Example: ["rm -rf /", "mkfs", "dd if=", ":(){:|:&};:"]
	BlockedCommands []string

	// DefaultTimeout is the max execution time for a single command.
	// 0 means use the hardcoded default (120s).
	DefaultTimeout time.Duration
}

// LocalBackend implements filesystem.Backend and filesystem.ShellBackend
// using the real local filesystem.
// For security, all paths are resolved relative to a base directory (typically user's home).
type LocalBackend struct {
	// baseDir is the root directory that all operations are relative to.
	// Paths starting with "/" are treated as relative to this directory.
	baseDir string

	// policy holds the shell execution security policy. Nil means no restrictions.
	policy *ShellPolicy
}

// LocalBackendConfig configures the LocalBackend.
type LocalBackendConfig struct {
	// BaseDir is the root directory for all filesystem operations.
	// All paths will be resolved relative to this directory.
	// If empty, defaults to the user's home directory.
	BaseDir string

	// ShellPolicy defines security constraints for shell command execution.
	// Nil means no restrictions (default).
	ShellPolicy *ShellPolicy
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

	// Ensure baseDir exists and is a directory
	info, err := os.Stat(baseDir)
	if err != nil {
		return nil, fmt.Errorf("failed to stat base directory: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("base path is not a directory: %s", baseDir)
	}

	return &LocalBackend{baseDir: baseDir, policy: config.ShellPolicy}, nil
}

// resolvePath converts an API path to an absolute filesystem path.
// Paths starting with "/" are treated as relative to baseDir.
// Returns an error if the resolved path escapes the base directory.
func (b *LocalBackend) resolvePath(p string) (string, error) {
	// Treat paths as relative to baseDir
	if p == "" || p == "/" {
		return b.baseDir, nil
	}

	// Remove leading slash if present (handles both Unix "/" and Windows "\")
	cleanPath := strings.TrimPrefix(p, "/")
	cleanPath = strings.TrimPrefix(cleanPath, "\\")
	resolved := filepath.Join(b.baseDir, cleanPath)

	// Security check: ensure the resolved path is within baseDir
	absResolved, err := filepath.Abs(resolved)
	if err != nil {
		return "", fmt.Errorf("failed to resolve path: %w", err)
	}
	absBase, err := filepath.Abs(b.baseDir)
	if err != nil {
		return "", fmt.Errorf("failed to resolve base path: %w", err)
	}

	// Normalize paths for comparison (handles Windows case-insensitivity and path separators)
	// Use filepath.Clean to normalize separators, then compare
	normalizedResolved := filepath.Clean(absResolved)
	normalizedBase := filepath.Clean(absBase)

	// Use filepath.Rel to check if resolved path is under baseDir
	// If Rel returns a path starting with "..", it means resolved is outside baseDir
	rel, err := filepath.Rel(normalizedBase, normalizedResolved)
	if err != nil {
		return "", fmt.Errorf("path escapes base directory: %s", p)
	}
	if strings.HasPrefix(rel, "..") {
		return "", fmt.Errorf("path escapes base directory: %s", p)
	}

	return absResolved, nil
}

// toAPIPath converts a filesystem path back to an API path (relative to baseDir).
func (b *LocalBackend) toAPIPath(fsPath string) string {
	rel, err := filepath.Rel(b.baseDir, fsPath)
	if err != nil {
		return fsPath
	}
	// Convert to forward slashes and add leading slash
	return "/" + filepath.ToSlash(rel)
}

// LsInfo lists file information under the given path.
// If the path is a file, returns the file's info.
// If the path is a directory, returns the list of entries.
// The output includes detailed information: type, size, modification time.
func (b *LocalBackend) LsInfo(ctx context.Context, req *filesystem.LsInfoRequest) ([]filesystem.FileInfo, error) {
	targetPath, err := b.resolvePath(req.Path)
	if err != nil {
		return nil, err
	}

	// Check if path exists and determine its type
	info, err := os.Stat(targetPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("path does not exist: %s", req.Path)
		}
		return nil, fmt.Errorf("failed to stat path: %w", err)
	}

	// If it's a file, return its info directly
	if !info.IsDir() {
		return []filesystem.FileInfo{{
			Path: b.formatFileEntry(targetPath, info),
		}}, nil
	}

	// It's a directory, read its entries
	entries, err := os.ReadDir(targetPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read directory: %w", err)
	}

	var result []filesystem.FileInfo
	for _, entry := range entries {
		fullPath := filepath.Join(targetPath, entry.Name())
		entryInfo, err := entry.Info()
		if err != nil {
			// If we can't get info, just show the path
			result = append(result, filesystem.FileInfo{
				Path: b.toAPIPath(fullPath),
			})
			continue
		}
		result = append(result, filesystem.FileInfo{
			Path: b.formatFileEntry(fullPath, entryInfo),
		})
	}

	// Return a placeholder if directory is empty to avoid empty tool result issues
	if len(result) == 0 {
		return []filesystem.FileInfo{{Path: "(empty directory)"}}, nil
	}

	return result, nil
}

// formatFileEntry formats a file entry with detailed information.
// Format: "[type] path  size  modified_time"
func (b *LocalBackend) formatFileEntry(fsPath string, info os.FileInfo) string {
	apiPath := b.toAPIPath(fsPath)
	
	// Determine type indicator
	typeIndicator := "[file]"
	if info.IsDir() {
		typeIndicator = "[dir] "
	} else if info.Mode()&os.ModeSymlink != 0 {
		typeIndicator = "[link]"
	}

	// Format size (only for files)
	sizeStr := ""
	if !info.IsDir() {
		sizeStr = formatSize(info.Size())
	}

	// Format modification time
	modTime := info.ModTime().Format("2006-01-02 15:04")

	// Build formatted string
	if sizeStr != "" {
		return fmt.Sprintf("%s %s  %s  %s", typeIndicator, apiPath, sizeStr, modTime)
	}
	return fmt.Sprintf("%s %s  %s", typeIndicator, apiPath, modTime)
}

// formatSize formats file size in human-readable format.
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

// Read reads file content with support for line-based offset and limit.
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
	// Increase buffer size for long lines
	const maxCapacity = 1024 * 1024 // 1MB
	buf := make([]byte, maxCapacity)
	scanner.Buffer(buf, maxCapacity)

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

// GrepRaw searches for content matching the specified pattern in files.
func (b *LocalBackend) GrepRaw(ctx context.Context, req *filesystem.GrepRequest) ([]filesystem.GrepMatch, error) {
	basePath, err := b.resolvePath(req.Path)
	if err != nil {
		return nil, err
	}

	var matches []filesystem.GrepMatch
	const maxMatches = 100

	err = filepath.Walk(basePath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip files we can't access
		}
		if info.IsDir() {
			return nil
		}

		// Apply glob filter if specified
		if req.Glob != "" {
			matched, err := filepath.Match(req.Glob, filepath.Base(path))
			if err != nil || !matched {
				return nil
			}
		}

		// Skip binary files (simple heuristic)
		if isBinaryFile(path) {
			return nil
		}

		fileMatches, err := grepFile(path, req.Pattern, b)
		if err != nil {
			return nil // Skip files we can't read
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

// grepFile searches for pattern in a single file.
// Pattern is treated as a regular expression; falls back to literal match on invalid regex.
func grepFile(filePath, pattern string, b *LocalBackend) ([]filesystem.GrepMatch, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	// Try to compile pattern as regex; fall back to literal match if invalid
	re, regexErr := regexp.Compile(pattern)
	useLiteral := regexErr != nil

	var matches []filesystem.GrepMatch
	scanner := bufio.NewScanner(file)
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		line := scanner.Text()

		var matched bool
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

// isBinaryFile checks if a file is likely binary based on extension.
func isBinaryFile(path string) bool {
	binaryExts := map[string]bool{
		".exe": true, ".dll": true, ".so": true, ".dylib": true,
		".zip": true, ".tar": true, ".gz": true, ".bz2": true, ".xz": true,
		".png": true, ".jpg": true, ".jpeg": true, ".gif": true, ".webp": true,
		".mp3": true, ".mp4": true, ".avi": true, ".mov": true,
		".pdf": true, ".doc": true, ".docx": true, ".xls": true, ".xlsx": true,
		".bin": true, ".dat": true, ".db": true, ".sqlite": true,
	}
	ext := strings.ToLower(filepath.Ext(path))
	return binaryExts[ext]
}

// GlobInfo returns file information matching the glob pattern.
func (b *LocalBackend) GlobInfo(ctx context.Context, req *filesystem.GlobInfoRequest) ([]filesystem.FileInfo, error) {
	basePath, err := b.resolvePath(req.Path)
	if err != nil {
		return nil, err
	}

	pattern := req.Pattern
	if pattern == "" {
		pattern = "*"
	}

	// Handle ** patterns for recursive matching
	var matches []string
	if strings.Contains(pattern, "**") {
		matches, err = globRecursive(basePath, pattern)
	} else {
		fullPattern := filepath.Join(basePath, pattern)
		matches, err = filepath.Glob(fullPattern)
	}

	if err != nil {
		return nil, fmt.Errorf("glob failed: %w", err)
	}

	var result []filesystem.FileInfo
	for _, m := range matches {
		result = append(result, filesystem.FileInfo{
			Path: b.toAPIPath(m),
		})
	}

	if len(result) == 0 {
		return []filesystem.FileInfo{{Path: "(no matches found)"}}, nil
	}

	return result, nil
}

// globRecursive handles ** patterns for recursive glob matching.
func globRecursive(basePath, pattern string) ([]string, error) {
	var matches []string
	const maxResults = 500

	// Convert ** to regex-compatible pattern
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
		relPath = filepath.ToSlash(relPath)
		if re.MatchString(relPath) {
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

	// Ensure parent directory exists
	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	if err := os.WriteFile(filePath, []byte(req.Content), 0o644); err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	return nil
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

	if err := os.WriteFile(filePath, []byte(newContent), 0o644); err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	return nil
}

// Execute runs a shell command via a subprocess and returns its output.
// Uses the platform's native default shell: cmd.exe on Windows, zsh on macOS, bash on Linux.
// The command runs in baseDir as the working directory.
func (b *LocalBackend) Execute(ctx context.Context, req *filesystem.ExecuteRequest) (*filesystem.ExecuteResponse, error) {
	// Validate command against security policy
	if err := b.validateCommand(req.Command); err != nil {
		exitCode := -1
		return &filesystem.ExecuteResponse{
			Output:   "Command blocked: " + err.Error(),
			ExitCode: &exitCode,
		}, nil
	}

	// Set timeout from policy or use default (120s)
	timeout := 120 * time.Second
	if b.policy != nil && b.policy.DefaultTimeout > 0 {
		timeout = b.policy.DefaultTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Build command using the platform's native default shell
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.CommandContext(ctx, "cmd.exe", "/C", req.Command)
	case "darwin":
		cmd = exec.CommandContext(ctx, "/bin/zsh", "-c", req.Command)
	default: // linux and others
		cmd = exec.CommandContext(ctx, "/bin/bash", "-c", req.Command)
	}
	cmd.Dir = b.baseDir

	// Capture combined stdout+stderr output
	output, err := cmd.CombinedOutput()

	exitCode := 0
	if cmd.ProcessState != nil {
		exitCode = cmd.ProcessState.ExitCode()
	}

	// Truncate output if too large to avoid token explosion
	const maxOutput = 128 * 1024 // 128KB
	outputStr := string(output)
	truncated := false
	if len(outputStr) > maxOutput {
		outputStr = outputStr[:maxOutput]
		truncated = true
	}

	// Handle timeout case
	if ctx.Err() == context.DeadlineExceeded {
		outputStr += "\n[Command timed out]"
	}

	// If there was an exec-level error (e.g. command not found) but we still have output, include it
	if err != nil && len(outputStr) == 0 {
		outputStr = err.Error()
	}

	return &filesystem.ExecuteResponse{
		Output:    outputStr,
		ExitCode:  &exitCode,
		Truncated: truncated,
	}, nil
}

// validateCommand checks the command against the shell security policy.
// Returns nil if allowed, error with reason if blocked.
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

// Ensure LocalBackend implements both filesystem.Backend and filesystem.ShellBackend interfaces
var _ filesystem.Backend = (*LocalBackend)(nil)
var _ filesystem.ShellBackend = (*LocalBackend)(nil)
