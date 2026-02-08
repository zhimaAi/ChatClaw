// Package filesystem provides a local filesystem backend for the eino ADK filesystem middleware.
//
// The backend is split across multiple files by functionality:
//   - local_backend.go  — core types, constructor, path resolution, helpers
//   - backend_ls.go     — LsInfo
//   - backend_read.go   — Read
//   - backend_write.go  — Write, Edit
//   - backend_grep.go   — GrepRaw, GrepEnhanced
//   - backend_glob.go   — GlobInfo
//   - backend_execute.go— Execute, ShellPolicy
//   - backend_patch.go  — Patch, PatchOperation
//   - grep_tool.go      — grep tool definition (for agent registration)
//   - patch_tool.go     — patch_file tool definition (for agent registration)
package filesystem

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/cloudwego/eino/adk/filesystem"
)

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

// formatSize returns a human-readable file size string.
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

// isBinaryFile returns true if the file extension indicates a binary format.
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

// Interface compliance assertions.
var _ filesystem.Backend = (*LocalBackend)(nil)
var _ filesystem.ShellBackend = (*LocalBackend)(nil)
