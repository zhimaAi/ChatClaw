package tools

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// FsToolsConfig holds shared configuration for all filesystem tools.
type FsToolsConfig struct {
	// HomeDir is the user's home directory (read-only root for path resolution).
	HomeDir string
	// WorkDir is the primary working directory for write operations and execute cwd.
	WorkDir string
	// SandboxEnabled restricts write operations to WorkDir when true.
	SandboxEnabled bool
	// SandboxNetworkEnabled allows network access inside the codex sandbox.
	SandboxNetworkEnabled bool

	// CodexBin is the path to the codex binary (empty if not available).
	CodexBin string

	// ToolchainBinDir is the directory containing managed tool binaries (uv, bun, etc.).
	// When non-empty, it is prepended to PATH for executed commands.
	ToolchainBinDir string
}

// ResolvePath converts a path to an absolute filesystem path.
// Relative paths are resolved against HomeDir.
func (c *FsToolsConfig) ResolvePath(p string) (string, error) {
	if p == "" || p == "/" {
		return c.HomeDir, nil
	}

	var resolved string
	if filepath.IsAbs(p) {
		resolved = p
	} else {
		resolved = filepath.Join(c.HomeDir, strings.TrimPrefix(strings.TrimPrefix(p, "/"), "\\"))
	}

	absResolved, err := filepath.Abs(resolved)
	if err != nil {
		return "", fmt.Errorf("failed to resolve path: %w", err)
	}
	absBase, err := filepath.Abs(c.HomeDir)
	if err != nil {
		return "", fmt.Errorf("failed to resolve home dir: %w", err)
	}

	rel, err := filepath.Rel(filepath.Clean(absBase), filepath.Clean(absResolved))
	if err != nil || strings.HasPrefix(rel, "..") {
		return "", fmt.Errorf("path escapes home directory: %s", p)
	}

	return absResolved, nil
}

// ResolveWritePath resolves a path for write operations.
// When SandboxEnabled is true, the path must be within WorkDir.
func (c *FsToolsConfig) ResolveWritePath(p string) (string, error) {
	absPath, err := c.ResolvePath(p)
	if err != nil {
		return "", err
	}

	if !c.SandboxEnabled {
		return absPath, nil
	}

	absSandbox, err := filepath.Abs(c.WorkDir)
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

// IsBinaryFile returns true if the file extension indicates a binary format.
func IsBinaryFile(path string) bool {
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

// FormatSize returns a human-readable file size string.
func FormatSize(bytes int64) string {
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

// FormatFileEntry formats: "[type] path  size  modified_time"
func FormatFileEntry(path string, info os.FileInfo) string {
	typeIndicator := "[file]"
	if info.IsDir() {
		typeIndicator = "[dir] "
	} else if info.Mode()&os.ModeSymlink != 0 {
		typeIndicator = "[link]"
	}

	modTime := info.ModTime().Format("2006-01-02 15:04")

	if !info.IsDir() {
		return fmt.Sprintf("%s %s  %s  %s", typeIndicator, path, FormatSize(info.Size()), modTime)
	}
	return fmt.Sprintf("%s %s  %s", typeIndicator, path, modTime)
}
