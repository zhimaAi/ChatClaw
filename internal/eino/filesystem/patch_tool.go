// Package filesystem provides a local filesystem backend for the eino ADK filesystem middleware.
package filesystem

import (
	"context"
	"fmt"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
)

// PatchToolID is the tool name registered with the agent.
const PatchToolID = "patch_file"

// PatchToolDesc is the default description shown to the LLM.
const PatchToolDesc = `Apply line-based patch operations to a file. Each operation specifies an action (insert, delete, or replace) and a line range.
This is more efficient than edit_file when you need to modify specific line ranges, especially for multi-line insertions, deletions, or replacements.

Line numbers are 1-based. Multiple operations can be applied in a single call — they are automatically applied from bottom to top so line numbers stay stable.

Actions:
- "insert": Insert new lines BEFORE start_line. Use start_line = total_lines + 1 to append at end. end_line is ignored.
- "delete": Delete lines from start_line to end_line (inclusive).
- "replace": Replace lines from start_line to end_line (inclusive) with the provided content lines.

Tips:
- Always read_file first to know current line numbers before patching.
- Combine multiple operations in one call for atomic multi-site edits.`

// patchFileInput is the JSON schema for the patch_file tool arguments.
type patchFileInput struct {
	FilePath   string          `json:"file_path" jsonschema:"description=The absolute path of the file to patch."`
	Operations []patchOpInput  `json:"operations" jsonschema:"description=List of patch operations to apply."`
}

// patchOpInput is a single operation within a patch.
type patchOpInput struct {
	Action    string   `json:"action" jsonschema:"description=The operation type: insert or delete or replace.,enum=insert,enum=delete,enum=replace"`
	StartLine int      `json:"start_line" jsonschema:"description=1-based starting line number. For insert this is the line BEFORE which new content is inserted."`
	EndLine   int      `json:"end_line,omitempty" jsonschema:"description=1-based ending line number (inclusive). Required for delete and replace. Ignored for insert."`
	Content   []string `json:"content,omitempty" jsonschema:"description=Lines of new content (without trailing newlines). Required for insert and replace. Ignored for delete."`
}

// NewPatchTool creates the patch_file tool backed by the given LocalBackend.
func NewPatchTool(backend *LocalBackend) (tool.BaseTool, error) {
	return utils.InferTool(PatchToolID, PatchToolDesc, func(ctx context.Context, input *patchFileInput) (string, error) {
		if len(input.Operations) == 0 {
			return "", fmt.Errorf("operations list must not be empty")
		}

		ops := make([]PatchOperation, len(input.Operations))
		for i, op := range input.Operations {
			ops[i] = PatchOperation{
				Action:    op.Action,
				StartLine: op.StartLine,
				EndLine:   op.EndLine,
				Content:   op.Content,
			}
		}

		return backend.Patch(ctx, input.FilePath, ops)
	})
}
