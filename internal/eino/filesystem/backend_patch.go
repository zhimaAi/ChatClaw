package filesystem

import (
	"context"
	"fmt"
	"os"
	"strings"
)

// PatchOperation represents a single line-based operation in a patch.
type PatchOperation struct {
	// Action is the type of operation: "insert", "delete", or "replace".
	Action string
	// StartLine is the 1-based starting line number.
	// For "insert", the new content is inserted BEFORE this line.
	// Use a value one past the last line to append at the end.
	StartLine int
	// EndLine is the 1-based ending line number (inclusive). Only used for
	// "delete" and "replace". Ignored for "insert".
	EndLine int
	// Content is the new text. Used by "insert" and "replace". Each element
	// in the slice is one line (without trailing newline). Ignored for "delete".
	Content []string
}

// Patch applies a series of line-based operations (insert, delete, replace)
// to a file. Operations are applied in reverse line order so that earlier
// operations don't shift the line numbers of later ones.
func (b *LocalBackend) Patch(ctx context.Context, filePath string, ops []PatchOperation) (string, error) {
	absPath, err := b.resolvePath(filePath)
	if err != nil {
		return "", err
	}

	data, err := os.ReadFile(absPath)
	if err != nil {
		return "", fmt.Errorf("failed to read file: %w", err)
	}

	lines := strings.Split(string(data), "\n")
	totalLines := len(lines)

	// Validate all operations before applying any changes.
	for i, op := range ops {
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

	// Sort operations by start_line descending so we apply from bottom to top.
	// This preserves line numbers for earlier operations.
	sorted := make([]PatchOperation, len(ops))
	copy(sorted, ops)
	sortPatchOps(sorted)

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
			endIdx := op.EndLine // endIdx is exclusive in slice terms
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

	return fmt.Sprintf("Successfully patched %s (%d operations applied)", filePath, len(ops)), nil
}

// sortPatchOps sorts patch operations by StartLine descending (stable).
func sortPatchOps(ops []PatchOperation) {
	// Simple insertion sort — typically very few operations.
	for i := 1; i < len(ops); i++ {
		key := ops[i]
		j := i - 1
		for j >= 0 && ops[j].StartLine < key.StartLine {
			ops[j+1] = ops[j]
			j--
		}
		ops[j+1] = key
	}
}
