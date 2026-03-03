package tools

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/cloudwego/eino/adk/filesystem"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
)

type globInput struct {
	Pattern string `json:"pattern" jsonschema:"description=Glob pattern to match (e.g. '*.go' or '**/*.ts'). Supports ** for recursive matching."`
	Path    string `json:"path,omitempty" jsonschema:"description=Directory to search in. Defaults to home directory."`
}

// NewGlobTool creates a glob tool backed by Backend.
func NewGlobTool(b *Backend) (tool.BaseTool, error) {
	return utils.InferTool(ToolIDGlob,
		"Find files matching a glob pattern. Returns absolute file paths, one per line.",
		func(ctx context.Context, input *globInput) (string, error) {
			basePath, err := b.ResolvePath(input.Path)
			if err != nil {
				return "", err
			}

			results, err := b.GlobInfo(ctx, &filesystem.GlobInfoRequest{
				Pattern: input.Pattern,
				Path:    basePath,
			})
			if err != nil {
				return "", fmt.Errorf("glob failed: %w", err)
			}
			if len(results) == 0 {
				return "(no matches found)", nil
			}

			paths := make([]string, len(results))
			for i, r := range results {
				paths[i] = r.Path
			}
			return strings.Join(paths, "\n"), nil
		})
}

type grepInput struct {
	Pattern            string `json:"pattern" jsonschema:"description=The search pattern. Treated as regex; falls back to literal match on invalid regex."`
	Path               string `json:"path,omitempty" jsonschema:"description=Directory or file path to search in. Defaults to home directory if empty."`
	Glob               string `json:"glob,omitempty" jsonschema:"description=Optional filename glob filter (e.g. '*.go' or '*.ts'). Only files matching this pattern are searched."`
	IgnoreCase         bool   `json:"ignore_case,omitempty" jsonschema:"description=If true performs case-insensitive matching."`
	ContextBefore      int    `json:"context_before,omitempty" jsonschema:"description=Number of lines to show before each match (like grep -B). Default 0."`
	ContextAfter       int    `json:"context_after,omitempty" jsonschema:"description=Number of lines to show after each match (like grep -A). Default 0."`
	MaxMatches         int    `json:"max_matches,omitempty" jsonschema:"description=Maximum number of matches to return. Default 100."`
	IncludeLineNumbers bool   `json:"include_line_numbers,omitempty" jsonschema:"description=If true prefix each output line with its line number. Default false."`
	OutputMode         string `json:"output_mode,omitempty" jsonschema:"description=Output mode: content (default shows matching lines with context) or files_with_matches (file paths only) or count (match count summary).,enum=content,enum=files_with_matches,enum=count"`
}

// NewGrepTool creates a grep tool backed by Backend.
// This uses Backend.GrepRaw for the core matching, then applies context lines,
// output formatting, and pagination locally for richer output than the base interface.
func NewGrepTool(b *Backend) (tool.BaseTool, error) {
	return utils.InferTool(ToolIDGrep,
		`Search for a pattern in files.

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
- Case-insensitive: grep(pattern="readme", ignore_case=true, output_mode="files_with_matches")`,
		func(ctx context.Context, input *grepInput) (string, error) {
			if input.Pattern == "" {
				return "", fmt.Errorf("pattern must not be empty")
			}
			return grepEnhanced(b, ctx, input)
		})
}

func grepEnhanced(b *Backend, ctx context.Context, input *grepInput) (string, error) {
	basePath, err := b.ResolvePath(input.Path)
	if err != nil {
		return "", err
	}

	matches, err := b.GrepRaw(ctx, &filesystem.GrepRequest{
		Pattern:         input.Pattern,
		Path:            basePath,
		Glob:            input.Glob,
		CaseInsensitive: input.IgnoreCase,
	})
	if err != nil {
		return "", err
	}

	outputMode := input.OutputMode
	if outputMode == "" {
		outputMode = "content"
	}

	switch outputMode {
	case "count":
		fileCount := map[string]int{}
		for _, m := range matches {
			fileCount[m.Path]++
		}
		return fmt.Sprintf("%d matches in %d files", len(matches), len(fileCount)), nil

	case "files_with_matches":
		if len(matches) == 0 {
			return "(no matches found)", nil
		}
		seen := map[string]bool{}
		var files []string
		for _, m := range matches {
			if !seen[m.Path] {
				seen[m.Path] = true
				files = append(files, m.Path)
			}
		}
		return strings.Join(files, "\n"), nil

	default:
		if len(matches) == 0 {
			return "(no matches found)", nil
		}

		maxMatches := input.MaxMatches
		if maxMatches <= 0 {
			maxMatches = 100
		}
		if len(matches) > maxMatches {
			matches = matches[:maxMatches]
		}

		contextBefore := input.ContextBefore
		if contextBefore < 0 {
			contextBefore = 0
		}
		contextAfter := input.ContextAfter
		if contextAfter < 0 {
			contextAfter = 0
		}

		var sb strings.Builder
		lastFile := ""
		for idx, m := range matches {
			if m.Path != lastFile {
				if lastFile != "" {
					sb.WriteString("\n")
				}
				sb.WriteString(m.Path)
				sb.WriteString("\n")
				lastFile = m.Path
			} else if idx > 0 && (contextBefore > 0 || contextAfter > 0) {
				sb.WriteString("--\n")
			}

			if contextBefore > 0 || contextAfter > 0 {
				contextLines := readContextLines(m.Path, m.Line, contextBefore, contextAfter)
				for _, cl := range contextLines.before {
					if input.IncludeLineNumbers {
						sb.WriteString(fmt.Sprintf("%d-", cl.num))
					}
					sb.WriteString(cl.text)
					sb.WriteString("\n")
				}
				if input.IncludeLineNumbers {
					sb.WriteString(fmt.Sprintf("%d:", m.Line))
				}
				sb.WriteString(m.Content)
				sb.WriteString("\n")
				for _, cl := range contextLines.after {
					if input.IncludeLineNumbers {
						sb.WriteString(fmt.Sprintf("%d-", cl.num))
					}
					sb.WriteString(cl.text)
					sb.WriteString("\n")
				}
			} else {
				if input.IncludeLineNumbers {
					sb.WriteString(fmt.Sprintf("%d:", m.Line))
				}
				sb.WriteString(m.Content)
				sb.WriteString("\n")
			}
		}
		return sb.String(), nil
	}
}

type contextLine struct {
	num  int
	text string
}

type contextResult struct {
	before []contextLine
	after  []contextLine
}

func readContextLines(path string, matchLine, before, after int) contextResult {
	var result contextResult

	data, err := readFileLines(path)
	if err != nil || len(data) == 0 {
		return result
	}

	lineIdx := matchLine - 1
	if before > 0 {
		start := lineIdx - before
		if start < 0 {
			start = 0
		}
		for i := start; i < lineIdx; i++ {
			result.before = append(result.before, contextLine{num: i + 1, text: data[i]})
		}
	}
	if after > 0 {
		end := lineIdx + 1 + after
		if end > len(data) {
			end = len(data)
		}
		for i := lineIdx + 1; i < end; i++ {
			result.after = append(result.after, contextLine{num: i + 1, text: data[i]})
		}
	}
	return result
}

func readFileLines(path string) ([]string, error) {
	data, err := readFileBytes(path)
	if err != nil {
		return nil, err
	}
	return strings.Split(string(data), "\n"), nil
}

func readFileBytes(path string) ([]byte, error) {
	return readFileBytesOS(path)
}

func readFileBytesOS(path string) ([]byte, error) {
	return os.ReadFile(path)
}
