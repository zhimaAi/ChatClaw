package tools

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

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
