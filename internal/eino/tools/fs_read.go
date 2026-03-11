package tools

import (
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

// NewReadFileTool creates a read_file tool backed by Backend.
func NewReadFileTool(b *Backend) (tool.BaseTool, error) {
	return utils.InferTool(ToolIDReadFile,
		selectDesc(
			"Read file content with optional line offset and limit. Use absolute paths. Prefer this over shell cat/head/tail for reading files.",
			"读取文件内容，支持行偏移和行数限制。使用绝对路径。读取文件时优先使用此工具而非 shell 的 cat/head/tail。",
		),
		func(ctx context.Context, input *readFileInput) (string, error) {
			filePath, err := b.ResolvePath(input.FilePath)
			if err != nil {
				return "", err
			}

			limit := input.Limit
			if limit <= 0 {
				limit = 200
			}

			fc, err := b.Read(ctx, &filesystem.ReadRequest{
				FilePath: filePath,
				Offset:   input.Offset + 1, // tool uses 0-based, backend uses 1-based
				Limit:    limit,
			})
			if err != nil {
				return "", err
			}
			return fc.Content, nil
		})
}

type lsInput struct {
	Path string `json:"path" jsonschema:"description=Absolute path of the directory to list."`
}

// NewLsTool creates an ls tool with rich output (type, size, time).
func NewLsTool(b *Backend) (tool.BaseTool, error) {
	return utils.InferTool(ToolIDLs,
		selectDesc(
			"List files and directories at the given path. Returns type, path, size, and modification time. Use absolute paths (e.g. working directory when user mentions it).",
			"列出指定路径下的文件和目录。返回类型、路径、大小和修改时间。使用绝对路径（如用户提到工作目录时）。",
		),
		func(ctx context.Context, input *lsInput) (string, error) {
			targetPath, err := b.ResolvePath(input.Path)
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
