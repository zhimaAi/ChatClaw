package filesystem

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/cloudwego/eino/adk/filesystem"
)

// GrepRaw searches for a pattern in files under the given path.
// Pattern is treated as regex; falls back to literal match on invalid regex.
// This method implements the filesystem.Backend interface and is used by the
// built-in eino grep tool.
func (b *LocalBackend) GrepRaw(ctx context.Context, req *filesystem.GrepRequest) ([]filesystem.GrepMatch, error) {
	basePath, err := b.resolvePath(req.Path)
	if err != nil {
		return nil, err
	}

	var matches []filesystem.GrepMatch
	const maxMatches = 100

	err = filepath.Walk(basePath, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		if req.Glob != "" {
			matched, err := filepath.Match(req.Glob, filepath.Base(path))
			if err != nil || !matched {
				return nil
			}
		}
		if isBinaryFile(path) {
			return nil
		}
		fileMatches, err := grepFile(path, req.Pattern, b)
		if err != nil {
			return nil
		}
		matches = append(matches, fileMatches...)
		if len(matches) >= maxMatches {
			return filepath.SkipAll
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("grep failed: %w", err)
	}

	if len(matches) == 0 {
		return []filesystem.GrepMatch{{Path: "(no matches found)", Line: 0, Content: ""}}, nil
	}
	return matches, nil
}

func grepFile(filePath, pattern string, b *LocalBackend) ([]filesystem.GrepMatch, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	re, regexErr := regexp.Compile(pattern)
	useLiteral := regexErr != nil

	var matches []filesystem.GrepMatch
	scanner := bufio.NewScanner(file)
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := scanner.Text()
		matched := false
		if useLiteral {
			matched = strings.Contains(line, pattern)
		} else {
			matched = re.MatchString(line)
		}
		if matched {
			matches = append(matches, filesystem.GrepMatch{
				Path:    b.toAPIPath(filePath),
				Line:    lineNum,
				Content: line,
			})
		}
	}
	return matches, scanner.Err()
}

// GrepOptions configures the enhanced grep operation.
type GrepOptions struct {
	Pattern        string // Search pattern (regex; falls back to literal on invalid regex).
	Path           string // Directory or file path to search in.
	Glob           string // Optional filename glob filter (e.g. "*.go").
	IgnoreCase     bool   // Case-insensitive matching.
	ContextBefore  int    // Number of context lines before each match.
	ContextAfter   int    // Number of context lines after each match.
	MaxMatches     int    // Max matches to return (0 = default 100).
	IncludeLineNum bool   // Whether to prefix each line with its line number.
	OutputMode     string // "content", "files_with_matches", or "count".
}

// GrepEnhanced performs a grep search with context lines, case-insensitive
// matching, and output mode control. It returns a formatted string result.
func (b *LocalBackend) GrepEnhanced(ctx context.Context, opts *GrepOptions) (string, error) {
	basePath, err := b.resolvePath(opts.Path)
	if err != nil {
		return "", err
	}

	maxMatches := opts.MaxMatches
	if maxMatches <= 0 {
		maxMatches = 100
	}
	contextBefore := opts.ContextBefore
	if contextBefore < 0 {
		contextBefore = 0
	}
	contextAfter := opts.ContextAfter
	if contextAfter < 0 {
		contextAfter = 0
	}

	// Compile pattern.
	patternStr := opts.Pattern
	if opts.IgnoreCase {
		patternStr = "(?i)" + patternStr
	}
	re, regexErr := regexp.Compile(patternStr)
	useLiteral := regexErr != nil
	literalPattern := opts.Pattern
	if useLiteral && opts.IgnoreCase {
		literalPattern = strings.ToLower(opts.Pattern)
	}

	type matchEntry struct {
		path    string
		line    int
		content string
		before  []string // context lines before
		after   []string // context lines after
	}

	var allMatches []matchEntry
	seenFiles := map[string]int{} // file -> match count

	err = filepath.Walk(basePath, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil || info.IsDir() {
			return nil
		}
		if opts.Glob != "" {
			matched, matchErr := filepath.Match(opts.Glob, filepath.Base(path))
			if matchErr != nil || !matched {
				return nil
			}
		}
		if isBinaryFile(path) {
			return nil
		}

		// Read the entire file into lines for context support.
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil
		}
		lines := strings.Split(string(data), "\n")
		apiPath := b.toAPIPath(path)

		for i, line := range lines {
			matched := false
			if useLiteral {
				if opts.IgnoreCase {
					matched = strings.Contains(strings.ToLower(line), literalPattern)
				} else {
					matched = strings.Contains(line, opts.Pattern)
				}
			} else {
				matched = re.MatchString(line)
			}
			if !matched {
				continue
			}

			seenFiles[apiPath]++

			// For files_with_matches / count mode, we don't need context.
			if opts.OutputMode == "files_with_matches" || opts.OutputMode == "count" {
				continue
			}

			entry := matchEntry{
				path:    apiPath,
				line:    i + 1,
				content: line,
			}

			// Collect before-context.
			if contextBefore > 0 {
				start := i - contextBefore
				if start < 0 {
					start = 0
				}
				entry.before = lines[start:i]
			}

			// Collect after-context.
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
	})
	if err != nil {
		return "", fmt.Errorf("grep failed: %w", err)
	}

	// Format output based on mode.
	switch opts.OutputMode {
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

	default: // "content"
		if len(allMatches) == 0 {
			return "(no matches found)", nil
		}
		var sb strings.Builder
		lastFile := ""
		for idx, m := range allMatches {
			// File header.
			if m.path != lastFile {
				if lastFile != "" {
					sb.WriteString("\n")
				}
				sb.WriteString(m.path)
				sb.WriteString("\n")
				lastFile = m.path
			} else if idx > 0 && (contextBefore > 0 || contextAfter > 0) {
				sb.WriteString("--\n") // separator between match groups
			}

			// Before-context lines.
			for j, cl := range m.before {
				lineNum := m.line - len(m.before) + j
				if opts.IncludeLineNum {
					sb.WriteString(fmt.Sprintf("%d-", lineNum))
				}
				sb.WriteString(cl)
				sb.WriteString("\n")
			}

			// Match line.
			if opts.IncludeLineNum {
				sb.WriteString(fmt.Sprintf("%d:", m.line))
			}
			sb.WriteString(m.content)
			sb.WriteString("\n")

			// After-context lines.
			for j, cl := range m.after {
				lineNum := m.line + 1 + j
				if opts.IncludeLineNum {
					sb.WriteString(fmt.Sprintf("%d-", lineNum))
				}
				sb.WriteString(cl)
				sb.WriteString("\n")
			}
		}
		return sb.String(), nil
	}
}
