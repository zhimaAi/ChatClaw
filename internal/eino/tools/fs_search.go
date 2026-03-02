package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
)

type globInput struct {
	Pattern string `json:"pattern" jsonschema:"description=Glob pattern to match (e.g. '*.go' or '**/*.ts'). Supports ** for recursive matching."`
	Path    string `json:"path,omitempty" jsonschema:"description=Directory to search in. Defaults to home directory."`
}

// NewGlobTool creates a glob tool that finds files matching a pattern on disk.
func NewGlobTool(cfg *FsToolsConfig) (tool.BaseTool, error) {
	return utils.InferTool(ToolIDGlob,
		"Find files matching a glob pattern. Returns absolute file paths, one per line.",
		func(ctx context.Context, input *globInput) (string, error) {
			basePath, err := cfg.ResolvePath(input.Path)
			if err != nil {
				return "", err
			}

			pattern := input.Pattern
			if pattern == "" {
				pattern = "*"
			}

			var matches []string
			if strings.Contains(pattern, "**") {
				matches, err = globRecursive(basePath, pattern)
			} else {
				matches, err = filepath.Glob(filepath.Join(basePath, pattern))
			}
			if err != nil {
				return "", fmt.Errorf("glob failed: %w", err)
			}

			if len(matches) == 0 {
				return "(no matches found)", nil
			}
			return strings.Join(matches, "\n"), nil
		})
}

func globRecursive(basePath, pattern string) ([]string, error) {
	var matches []string
	const maxResults = 500

	regexPattern := regexp.QuoteMeta(pattern)
	regexPattern = strings.ReplaceAll(regexPattern, `\*\*`, `.*`)
	regexPattern = strings.ReplaceAll(regexPattern, `\*`, `[^/]*`)
	regexPattern = strings.ReplaceAll(regexPattern, `\?`, `.`)
	re, err := regexp.Compile("^" + regexPattern + "$")
	if err != nil {
		return nil, fmt.Errorf("invalid glob pattern: %w", err)
	}

	err = filepath.Walk(basePath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		relPath, _ := filepath.Rel(basePath, path)
		if re.MatchString(filepath.ToSlash(relPath)) {
			matches = append(matches, path)
		}
		if len(matches) >= maxResults {
			return filepath.SkipAll
		}
		return nil
	})
	return matches, err
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

// NewGrepTool creates an enhanced grep tool that searches files on disk.
func NewGrepTool(cfg *FsToolsConfig) (tool.BaseTool, error) {
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
			return grepEnhanced(cfg, input)
		})
}

func grepEnhanced(cfg *FsToolsConfig, input *grepInput) (string, error) {
	basePath, err := cfg.ResolvePath(input.Path)
	if err != nil {
		return "", err
	}

	maxMatches := input.MaxMatches
	if maxMatches <= 0 {
		maxMatches = 100
	}
	contextBefore := input.ContextBefore
	if contextBefore < 0 {
		contextBefore = 0
	}
	contextAfter := input.ContextAfter
	if contextAfter < 0 {
		contextAfter = 0
	}

	patternStr := input.Pattern
	if input.IgnoreCase {
		patternStr = "(?i)" + patternStr
	}
	re, regexErr := regexp.Compile(patternStr)
	useLiteral := regexErr != nil
	literalPattern := input.Pattern
	if useLiteral && input.IgnoreCase {
		literalPattern = strings.ToLower(input.Pattern)
	}

	type matchEntry struct {
		path    string
		line    int
		content string
		before  []string
		after   []string
	}

	var allMatches []matchEntry
	seenFiles := map[string]int{}

	info, err := os.Stat(basePath)
	if err != nil {
		return "", fmt.Errorf("path not found: %w", err)
	}

	var walkFunc func(path string, info os.FileInfo, walkErr error) error
	walkFunc = func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil || info.IsDir() {
			return nil
		}
		if input.Glob != "" {
			matched, matchErr := filepath.Match(input.Glob, filepath.Base(path))
			if matchErr != nil || !matched {
				return nil
			}
		}
		if IsBinaryFile(path) {
			return nil
		}

		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil
		}
		lines := strings.Split(string(data), "\n")

		for i, line := range lines {
			matched := false
			if useLiteral {
				if input.IgnoreCase {
					matched = strings.Contains(strings.ToLower(line), literalPattern)
				} else {
					matched = strings.Contains(line, input.Pattern)
				}
			} else {
				matched = re.MatchString(line)
			}
			if !matched {
				continue
			}

			seenFiles[path]++

			if input.OutputMode == "files_with_matches" || input.OutputMode == "count" {
				continue
			}

			entry := matchEntry{path: path, line: i + 1, content: line}
			if contextBefore > 0 {
				start := i - contextBefore
				if start < 0 {
					start = 0
				}
				entry.before = lines[start:i]
			}
			if contextAfter > 0 {
				end := i + 1 + contextAfter
				if end > len(lines) {
					end = len(lines)
				}
				entry.after = lines[i+1 : end]
			}

			allMatches = append(allMatches, entry)
			if len(allMatches) >= maxMatches {
				return filepath.SkipAll
			}
		}
		return nil
	}

	if !info.IsDir() {
		if err := walkFunc(basePath, info, nil); err != nil {
			return "", err
		}
	} else {
		if err := filepath.Walk(basePath, walkFunc); err != nil {
			return "", fmt.Errorf("grep failed: %w", err)
		}
	}

	outputMode := input.OutputMode
	if outputMode == "" {
		outputMode = "content"
	}

	switch outputMode {
	case "count":
		total := 0
		for _, c := range seenFiles {
			total += c
		}
		return fmt.Sprintf("%d matches in %d files", total, len(seenFiles)), nil

	case "files_with_matches":
		if len(seenFiles) == 0 {
			return "(no matches found)", nil
		}
		var files []string
		for f := range seenFiles {
			files = append(files, f)
		}
		return strings.Join(files, "\n"), nil

	default:
		if len(allMatches) == 0 {
			return "(no matches found)", nil
		}
		var sb strings.Builder
		lastFile := ""
		for idx, m := range allMatches {
			if m.path != lastFile {
				if lastFile != "" {
					sb.WriteString("\n")
				}
				sb.WriteString(m.path)
				sb.WriteString("\n")
				lastFile = m.path
			} else if idx > 0 && (contextBefore > 0 || contextAfter > 0) {
				sb.WriteString("--\n")
			}
			for j, cl := range m.before {
				lineNum := m.line - len(m.before) + j
				if input.IncludeLineNumbers {
					sb.WriteString(fmt.Sprintf("%d-", lineNum))
				}
				sb.WriteString(cl)
				sb.WriteString("\n")
			}
			if input.IncludeLineNumbers {
				sb.WriteString(fmt.Sprintf("%d:", m.line))
			}
			sb.WriteString(m.content)
			sb.WriteString("\n")
			for j, cl := range m.after {
				lineNum := m.line + 1 + j
				if input.IncludeLineNumbers {
					sb.WriteString(fmt.Sprintf("%d-", lineNum))
				}
				sb.WriteString(cl)
				sb.WriteString("\n")
			}
		}
		return sb.String(), nil
	}
}

