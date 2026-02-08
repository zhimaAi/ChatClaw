package filesystem

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/cloudwego/eino/adk/filesystem"
)

// GlobInfo returns file paths matching the glob pattern.
func (b *LocalBackend) GlobInfo(ctx context.Context, req *filesystem.GlobInfoRequest) ([]filesystem.FileInfo, error) {
	basePath, err := b.resolvePath(req.Path)
	if err != nil {
		return nil, err
	}

	pattern := req.Pattern
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
		return nil, fmt.Errorf("glob failed: %w", err)
	}

	var result []filesystem.FileInfo
	for _, m := range matches {
		result = append(result, filesystem.FileInfo{Path: b.toAPIPath(m)})
	}

	if len(result) == 0 {
		return []filesystem.FileInfo{{Path: "(no matches found)"}}, nil
	}
	return result, nil
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
