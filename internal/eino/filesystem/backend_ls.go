package filesystem

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/cloudwego/eino/adk/filesystem"
)

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
