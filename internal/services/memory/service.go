package memory

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"chatclaw/internal/define"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// MemoryService provides read/write access to OpenClaw agent workspace
// memory files (Markdown). Exposed to frontend via Wails bindings.
type MemoryService struct {
	app *application.App
}

func NewMemoryService(app *application.App) *MemoryService {
	return &MemoryService{app: app}
}

// MemoryFile represents a single memory file in the workspace.
type MemoryFile struct {
	// Path relative to workspace root (e.g. "MEMORY.md", "memory/2026-03-20.md")
	Path string `json:"path"`
	// Display name
	Name string `json:"name"`
	// Category: "core" | "daily" | "persona"
	Category string `json:"category"`
	// File modification time
	ModTime time.Time `json:"mod_time"`
	// File size in bytes
	Size int64 `json:"size"`
}

// ListMemoryFiles lists all .md files in the agent's OpenClaw workspace.
// Returns empty list (not error) if workspace doesn't exist yet.
func (s *MemoryService) ListMemoryFiles(openclawAgentID string) ([]MemoryFile, error) {
	if openclawAgentID == "" {
		return []MemoryFile{}, nil
	}

	wsDir, err := s.workspacePath(openclawAgentID)
	if err != nil {
		return nil, err
	}

	if _, err := os.Stat(wsDir); os.IsNotExist(err) {
		return []MemoryFile{}, nil
	}

	var files []MemoryFile

	err = filepath.Walk(wsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // skip unreadable entries
		}
		if info.IsDir() {
			if strings.HasPrefix(info.Name(), ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(strings.ToLower(info.Name()), ".md") {
			return nil
		}

		rel, _ := filepath.Rel(wsDir, path)
		rel = filepath.ToSlash(rel)

		files = append(files, MemoryFile{
			Path:     rel,
			Name:     info.Name(),
			Category: categorize(rel),
			ModTime:  info.ModTime(),
			Size:     info.Size(),
		})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk workspace: %w", err)
	}

	sort.Slice(files, func(i, j int) bool {
		ci, cj := categoryOrder(files[i].Category), categoryOrder(files[j].Category)
		if ci != cj {
			return ci < cj
		}
		return files[i].ModTime.After(files[j].ModTime)
	})

	return files, nil
}

// ReadMemoryFile reads the content of a memory file.
func (s *MemoryService) ReadMemoryFile(openclawAgentID, filePath string) (string, error) {
	if openclawAgentID == "" {
		return "", fmt.Errorf("workspace not configured")
	}

	absPath, err := s.resolveFilePath(openclawAgentID, filePath)
	if err != nil {
		return "", err
	}

	data, err := os.ReadFile(absPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", fmt.Errorf("read file: %w", err)
	}
	return string(data), nil
}

// WriteMemoryFile writes content to a memory file. Creates parent dirs if needed.
func (s *MemoryService) WriteMemoryFile(openclawAgentID, filePath, content string) error {
	if openclawAgentID == "" {
		return fmt.Errorf("workspace not configured")
	}

	absPath, err := s.resolveFilePath(openclawAgentID, filePath)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(absPath), 0755); err != nil {
		return fmt.Errorf("create directory: %w", err)
	}

	return os.WriteFile(absPath, []byte(content), 0644)
}

// DeleteMemoryFile deletes a memory file.
func (s *MemoryService) DeleteMemoryFile(openclawAgentID, filePath string) error {
	if openclawAgentID == "" {
		return fmt.Errorf("workspace not configured")
	}

	absPath, err := s.resolveFilePath(openclawAgentID, filePath)
	if err != nil {
		return err
	}

	if err := os.Remove(absPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("delete file: %w", err)
	}
	return nil
}

// workspacePath returns the absolute path to an agent's OpenClaw workspace.
func (s *MemoryService) workspacePath(openclawAgentID string) (string, error) {
	appDir, err := define.AppDataDir()
	if err != nil {
		return "", fmt.Errorf("resolve app data dir: %w", err)
	}
	return filepath.Join(appDir, "workspaces", openclawAgentID), nil
}

// resolveFilePath validates and resolves a relative file path within the workspace.
// Prevents path traversal attacks.
func (s *MemoryService) resolveFilePath(openclawAgentID, filePath string) (string, error) {
	wsDir, err := s.workspacePath(openclawAgentID)
	if err != nil {
		return "", err
	}

	absPath := filepath.Join(wsDir, filepath.FromSlash(filePath))

	absPath, err = filepath.Abs(absPath)
	if err != nil {
		return "", fmt.Errorf("resolve path: %w", err)
	}
	wsAbs, _ := filepath.Abs(wsDir)
	if !strings.HasPrefix(absPath, wsAbs+string(filepath.Separator)) && absPath != wsAbs {
		return "", fmt.Errorf("path traversal not allowed")
	}

	return absPath, nil
}

func categorize(relPath string) string {
	upper := strings.ToUpper(filepath.Base(relPath))
	if upper == "MEMORY.MD" {
		return "core"
	}
	if strings.HasPrefix(relPath, "memory/") || strings.HasPrefix(relPath, "memory\\") {
		return "daily"
	}
	if upper == "SOUL.MD" || upper == "USER.MD" || upper == "IDENTITY.MD" {
		return "persona"
	}
	return "other"
}

func categoryOrder(cat string) int {
	switch cat {
	case "core":
		return 0
	case "persona":
		return 1
	case "daily":
		return 2
	default:
		return 3
	}
}
