package filesystem

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/cloudwego/eino/adk/filesystem"
)

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
