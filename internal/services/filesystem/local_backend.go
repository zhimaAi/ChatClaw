// Package filesystem provides a local filesystem backend for the eino ADK filesystem middleware.
package filesystem

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/cloudwego/eino/adk/filesystem"
)

// LocalBackend implements filesystem.Backend using the real local filesystem.
// For security, all paths are resolved relative to a base directory (typically user's home).
type LocalBackend struct {
	// baseDir is the root directory that all operations are relative to.
	// Paths starting with "/" are treated as relative to this directory.
	baseDir string
}

// LocalBackendConfig configures the LocalBackend.
type LocalBackendConfig struct {
	// BaseDir is the root directory for all filesystem operations.
	// All paths will be resolved relative to this directory.
	// If empty, defaults to the user's home directory.
	BaseDir string
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

	return &LocalBackend{baseDir: baseDir}, nil
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
func grepFile(filePath, pattern string, b *LocalBackend) ([]filesystem.GrepMatch, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var matches []filesystem.GrepMatch
	scanner := bufio.NewScanner(file)
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		line := scanner.Text()
		if strings.Contains(line, pattern) {
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

// Ensure LocalBackend implements filesystem.Backend interface
var _ filesystem.Backend = (*LocalBackend)(nil)
