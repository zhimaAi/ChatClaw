// Package filesystem provides a local filesystem backend for the eino ADK filesystem middleware.
package filesystem

import (
	"context"
	"fmt"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
)

// GrepToolID is the tool name registered with the agent.
// This replaces the built-in eino "grep" tool with an enhanced version.
const GrepToolID = "grep"

// GrepToolDesc is the default description shown to the LLM.
const GrepToolDesc = `Search for a pattern in files.

Usage:
- The pattern parameter is the text to search for. Treated as regex; falls back to literal match on invalid regex.
- The path parameter filters which directory or file to search in (defaults to home directory).
- The glob parameter accepts a glob pattern to filter which files to search (e.g. "*.go", "*.ts").
- The ignore_case parameter enables case-insensitive matching.
- The context_before / context_after parameters show surrounding lines for each match (like grep -B / -A).
- The include_line_numbers parameter prefixes each output line with its line number.
- The output_mode parameter controls the output format:
  - "content": Show matching lines with file path, line numbers and optional context (default)
  - "files_with_matches": List only file paths containing matches
  - "count": Show total match count summary

Examples:
- Search all files: grep(pattern="TODO")
- Search Go files only: grep(pattern="func", glob="*.go")
- Show context: grep(pattern="error", context_before=3, context_after=3, include_line_numbers=true)
- Case-insensitive: grep(pattern="readme", ignore_case=true, output_mode="files_with_matches")`

// grepFileInput is the JSON schema for the grep_file tool arguments.
type grepFileInput struct {
	Pattern          string `json:"pattern" jsonschema:"description=The search pattern. Treated as regex; falls back to literal match on invalid regex."`
	Path             string `json:"path,omitempty" jsonschema:"description=Directory or file path to search in. Defaults to home directory if empty."`
	Glob             string `json:"glob,omitempty" jsonschema:"description=Optional filename glob filter (e.g. '*.go' or '*.ts'). Only files matching this pattern are searched."`
	IgnoreCase       bool   `json:"ignore_case,omitempty" jsonschema:"description=If true performs case-insensitive matching."`
	ContextBefore    int    `json:"context_before,omitempty" jsonschema:"description=Number of lines to show before each match (like grep -B). Default 0."`
	ContextAfter     int    `json:"context_after,omitempty" jsonschema:"description=Number of lines to show after each match (like grep -A). Default 0."`
	MaxMatches       int    `json:"max_matches,omitempty" jsonschema:"description=Maximum number of matches to return. Default 100."`
	IncludeLineNumbers bool `json:"include_line_numbers,omitempty" jsonschema:"description=If true prefix each output line with its line number. Default false."`
	OutputMode       string `json:"output_mode,omitempty" jsonschema:"description=Output mode: content (default shows matching lines with context) or files_with_matches (file paths only) or count (match count summary).,enum=content,enum=files_with_matches,enum=count"`
}

// NewGrepTool creates the grep_file tool backed by the given LocalBackend.
func NewGrepTool(backend *LocalBackend) (tool.BaseTool, error) {
	return utils.InferTool(GrepToolID, GrepToolDesc, func(ctx context.Context, input *grepFileInput) (string, error) {
		if input.Pattern == "" {
			return "", fmt.Errorf("pattern must not be empty")
		}

		outputMode := input.OutputMode
		if outputMode == "" {
			outputMode = "content"
		}

		return backend.GrepEnhanced(ctx, &GrepOptions{
			Pattern:        input.Pattern,
			Path:           input.Path,
			Glob:           input.Glob,
			IgnoreCase:     input.IgnoreCase,
			ContextBefore:  input.ContextBefore,
			ContextAfter:   input.ContextAfter,
			MaxMatches:     input.MaxMatches,
			IncludeLineNum: input.IncludeLineNumbers,
			OutputMode:     outputMode,
		})
	})
}
