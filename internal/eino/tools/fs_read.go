package tools

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/cloudwego/eino/adk/filesystem"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
)

type readFileInput struct {
	FilePath string `json:"file_path" jsonschema:"description=Absolute path of the file to read."`
	Offset   int    `json:"offset,omitempty" jsonschema:"description=0-based line offset to start reading from. Default 0."`
	Limit    int    `json:"limit,omitempty" jsonschema:"description=Maximum number of lines to read. Default 200."`
}

// NewReadFileTool creates a read_file tool that dispatches between the
// in-memory virtual filesystem (for reduction offloaded results) and the
// real local filesystem.
func NewReadFileTool(cfg *FsToolsConfig) (tool.BaseTool, error) {
	return utils.InferTool(ToolIDReadFile,
		"Read file content with optional line offset and limit. Use absolute paths.",
		func(ctx context.Context, input *readFileInput) (string, error) {
			if cfg.IsVirtualPath(input.FilePath) {
				return cfg.MemBackend.Read(ctx, &filesystem.ReadRequest{
					FilePath: input.FilePath,
					Offset:   input.Offset,
					Limit:    input.Limit,
				})
			}
			return readLocalFile(cfg, input)
		})
}

func readLocalFile(cfg *FsToolsConfig, input *readFileInput) (string, error) {
	filePath, err := cfg.ResolvePath(input.FilePath)
	if err != nil {
		return "", err
	}

	file, err := os.Open(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	offset := input.Offset
	if offset < 0 {
		offset = 0
	}
	limit := input.Limit
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

type lsInput struct {
	Path string `json:"path" jsonschema:"description=Absolute path of the directory to list."`
}

// NewLsTool creates an ls tool that lists files in a directory.
func NewLsTool(cfg *FsToolsConfig) (tool.BaseTool, error) {
	return utils.InferTool(ToolIDLs,
		"List files and directories at the given path. Returns type, path, size, and modification time.",
		func(ctx context.Context, input *lsInput) (string, error) {
			targetPath, err := cfg.ResolvePath(input.Path)
			if err != nil {
				return "", err
			}

			info, err := os.Stat(targetPath)
			if err != nil {
				if os.IsNotExist(err) {
					return "", fmt.Errorf("path does not exist: %s", input.Path)
				}
				return "", fmt.Errorf("failed to stat path: %w", err)
			}

			if !info.IsDir() {
				return FormatFileEntry(targetPath, info), nil
			}

			entries, err := os.ReadDir(targetPath)
			if err != nil {
				return "", fmt.Errorf("failed to read directory: %w", err)
			}

			if len(entries) == 0 {
				return "(empty directory)", nil
			}

			var sb strings.Builder
			for i, entry := range entries {
				fullPath := filepath.Join(targetPath, entry.Name())
				entryInfo, infoErr := entry.Info()
				if infoErr != nil {
					sb.WriteString(fullPath)
				} else {
					sb.WriteString(FormatFileEntry(fullPath, entryInfo))
				}
				if i < len(entries)-1 {
					sb.WriteString("\n")
				}
			}
			return sb.String(), nil
		})
}
