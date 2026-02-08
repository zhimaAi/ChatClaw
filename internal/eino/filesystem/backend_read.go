package filesystem

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/cloudwego/eino/adk/filesystem"
)

// Read reads file content with line-based offset and limit.
func (b *LocalBackend) Read(ctx context.Context, req *filesystem.ReadRequest) (string, error) {
	filePath, err := b.resolvePath(req.FilePath)
	if err != nil {
		return "", err
	}

	file, err := os.Open(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	offset := req.Offset
	if offset < 0 {
		offset = 0
	}
	limit := req.Limit
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
