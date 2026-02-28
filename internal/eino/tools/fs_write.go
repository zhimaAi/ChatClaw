package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
)

type writeFileInput struct {
	FilePath string `json:"file_path" jsonschema:"description=Absolute path of the file to write."`
	Content  string `json:"content" jsonschema:"description=The content to write to the file."`
}

// NewWriteFileTool creates a write_file tool that writes to the real filesystem.
// In sandbox mode, paths are restricted to the working directory.
func NewWriteFileTool(cfg *FsToolsConfig) (tool.BaseTool, error) {
	return utils.InferTool(ToolIDWriteFile,
		"Create or overwrite a file with the given content. Use absolute paths. Prefer this over shell echo for creating files.",
		func(ctx context.Context, input *writeFileInput) (string, error) {
			filePath, err := cfg.ResolveWritePath(input.FilePath)
			if err != nil {
				return "", err
			}
			if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
				return "", fmt.Errorf("failed to create directory: %w", err)
			}
			if err := os.WriteFile(filePath, []byte(input.Content), 0o644); err != nil {
				return "", fmt.Errorf("failed to write file: %w", err)
			}
			return fmt.Sprintf("Updated file %s", input.FilePath), nil
		})
}

type editFileInput struct {
	FilePath   string `json:"file_path" jsonschema:"description=Absolute path of the file to edit."`
	OldString  string `json:"old_string" jsonschema:"description=The exact string to find and replace."`
	NewString  string `json:"new_string" jsonschema:"description=The replacement string."`
	ReplaceAll bool   `json:"replace_all,omitempty" jsonschema:"description=If true replace all occurrences. Default false (single occurrence only)."`
}

// NewEditFileTool creates an edit_file tool that performs string replacement on disk.
func NewEditFileTool(cfg *FsToolsConfig) (tool.BaseTool, error) {
	return utils.InferTool(ToolIDEditFile,
		"Edit a file by replacing exact string matches. If replace_all is false and old_string appears more than once, the call fails (set replace_all=true to replace all).",
		func(ctx context.Context, input *editFileInput) (string, error) {
			if input.OldString == "" {
				return "", fmt.Errorf("old_string cannot be empty")
			}
			if input.OldString == input.NewString {
				return "", fmt.Errorf("old_string and new_string are identical")
			}

			filePath, err := cfg.ResolveWritePath(input.FilePath)
			if err != nil {
				return "", err
			}

			data, err := os.ReadFile(filePath)
			if err != nil {
				return "", fmt.Errorf("failed to read file: %w", err)
			}

			content := string(data)
			count := strings.Count(content, input.OldString)
			if count == 0 {
				return "", fmt.Errorf("old_string not found in file")
			}
			if !input.ReplaceAll && count > 1 {
				return "", fmt.Errorf("old_string found %d times, use replace_all=true to replace all occurrences", count)
			}

			var newContent string
			if input.ReplaceAll {
				newContent = strings.ReplaceAll(content, input.OldString, input.NewString)
			} else {
				newContent = strings.Replace(content, input.OldString, input.NewString, 1)
			}
			if err := os.WriteFile(filePath, []byte(newContent), 0o644); err != nil {
				return "", fmt.Errorf("failed to write file: %w", err)
			}
			return fmt.Sprintf("Successfully replaced the string in '%s'", input.FilePath), nil
		})
}

type patchFileInput struct {
	FilePath   string         `json:"file_path" jsonschema:"description=The absolute path of the file to patch."`
	Operations []patchOpInput `json:"operations" jsonschema:"description=List of patch operations to apply."`
}

type patchOpInput struct {
	Action    string   `json:"action" jsonschema:"description=The operation type: insert or delete or replace.,enum=insert,enum=delete,enum=replace"`
	StartLine int      `json:"start_line" jsonschema:"description=1-based starting line number. For insert this is the line BEFORE which new content is inserted."`
	EndLine   int      `json:"end_line,omitempty" jsonschema:"description=1-based ending line number (inclusive). Required for delete and replace. Ignored for insert."`
	Content   []string `json:"content,omitempty" jsonschema:"description=Lines of new content (without trailing newlines). Required for insert and replace. Ignored for delete."`
}

// NewPatchFileTool creates a patch_file tool for line-based file patching.
func NewPatchFileTool(cfg *FsToolsConfig) (tool.BaseTool, error) {
	return utils.InferTool(ToolIDPatchFile,
		`Apply line-based patch operations to a file. Each operation specifies an action (insert, delete, or replace) and a line range.
This is more efficient than edit_file when you need to modify specific line ranges, especially for multi-line insertions, deletions, or replacements.

Line numbers are 1-based. Multiple operations can be applied in a single call — they are automatically applied from bottom to top so line numbers stay stable.

Actions:
- "insert": Insert new lines BEFORE start_line. Use start_line = total_lines + 1 to append at end. end_line is ignored.
- "delete": Delete lines from start_line to end_line (inclusive).
- "replace": Replace lines from start_line to end_line (inclusive) with the provided content lines.

Tips:
- Always read_file first to know current line numbers before patching.
- Combine multiple operations in one call for atomic multi-site edits.`,
		func(ctx context.Context, input *patchFileInput) (string, error) {
			if len(input.Operations) == 0 {
				return "", fmt.Errorf("operations list must not be empty")
			}

			absPath, err := cfg.ResolveWritePath(input.FilePath)
			if err != nil {
				return "", err
			}

			data, err := os.ReadFile(absPath)
			if err != nil {
				return "", fmt.Errorf("failed to read file: %w", err)
			}

			lines := strings.Split(string(data), "\n")
			totalLines := len(lines)

			for i, op := range input.Operations {
				switch op.Action {
				case "insert":
					if op.StartLine < 1 || op.StartLine > totalLines+1 {
						return "", fmt.Errorf("operation %d: start_line %d out of range [1, %d]", i, op.StartLine, totalLines+1)
					}
				case "delete", "replace":
					if op.StartLine < 1 || op.StartLine > totalLines {
						return "", fmt.Errorf("operation %d: start_line %d out of range [1, %d]", i, op.StartLine, totalLines)
					}
					if op.EndLine < op.StartLine || op.EndLine > totalLines {
						return "", fmt.Errorf("operation %d: end_line %d out of range [%d, %d]", i, op.EndLine, op.StartLine, totalLines)
					}
					if op.Action == "replace" && len(op.Content) == 0 {
						return "", fmt.Errorf("operation %d: replace requires non-empty content", i)
					}
				default:
					return "", fmt.Errorf("operation %d: unknown action %q (must be insert, delete, or replace)", i, op.Action)
				}
			}

			sorted := make([]patchOpInput, len(input.Operations))
			copy(sorted, input.Operations)
			for i := 1; i < len(sorted); i++ {
				key := sorted[i]
				j := i - 1
				for j >= 0 && sorted[j].StartLine < key.StartLine {
					sorted[j+1] = sorted[j]
					j--
				}
				sorted[j+1] = key
			}

			for _, op := range sorted {
				switch op.Action {
				case "insert":
					idx := op.StartLine - 1
					newLines := make([]string, 0, len(lines)+len(op.Content))
					newLines = append(newLines, lines[:idx]...)
					newLines = append(newLines, op.Content...)
					newLines = append(newLines, lines[idx:]...)
					lines = newLines
				case "delete":
					startIdx := op.StartLine - 1
					endIdx := op.EndLine
					newLines := make([]string, 0, len(lines)-(endIdx-startIdx))
					newLines = append(newLines, lines[:startIdx]...)
					newLines = append(newLines, lines[endIdx:]...)
					lines = newLines
				case "replace":
					startIdx := op.StartLine - 1
					endIdx := op.EndLine
					newLines := make([]string, 0, len(lines)-(endIdx-startIdx)+len(op.Content))
					newLines = append(newLines, lines[:startIdx]...)
					newLines = append(newLines, op.Content...)
					newLines = append(newLines, lines[endIdx:]...)
					lines = newLines
				}
			}

			result := strings.Join(lines, "\n")
			if err := os.WriteFile(absPath, []byte(result), 0o644); err != nil {
				return "", fmt.Errorf("failed to write file: %w", err)
			}

			return fmt.Sprintf("Successfully patched %s (%d operations applied)", input.FilePath, len(input.Operations)), nil
		})
}
