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
	// Role of this file per OpenClaw spec: "agents" | "soul" | "identity" | "user" |
	// "tools" | "heartbeat" | "bootstrap" | "memory" | "daily" | "unknown"
	Role string `json:"role"`
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
			Path:    rel,
			Name:    info.Name(),
			Role:    fileRole(rel),
			ModTime: info.ModTime(),
			Size:    info.Size(),
		})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk workspace: %w", err)
	}

	sort.Slice(files, func(i, j int) bool {
		oi, oj := roleOrder(files[i].Role), roleOrder(files[j].Role)
		if oi != oj {
			return oi < oj
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
	root, err := define.OpenClawDataRootDir()
	if err != nil {
		return "", fmt.Errorf("resolve openclaw data dir: %w", err)
	}
	return filepath.Join(root, "workspace-"+openclawAgentID), nil
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

// fileRole maps a workspace file to its OpenClaw role.
func fileRole(relPath string) string {
	upper := strings.ToUpper(filepath.Base(relPath))
	switch upper {
	case "AGENTS.MD":
		return "agents"
	case "SOUL.MD":
		return "soul"
	case "IDENTITY.MD":
		return "identity"
	case "USER.MD":
		return "user"
	case "TOOLS.MD":
		return "tools"
	case "HEARTBEAT.MD":
		return "heartbeat"
	case "BOOTSTRAP.MD":
		return "bootstrap"
	case "MEMORY.MD":
		return "memory"
	}
	if strings.HasPrefix(relPath, "memory/") || strings.HasPrefix(relPath, "memory\\") {
		return "daily"
	}
	return "unknown"
}

// roleOrder defines display order following the OpenClaw session-startup sequence.
func roleOrder(role string) int {
	switch role {
	case "agents":
		return 0
	case "soul":
		return 1
	case "identity":
		return 2
	case "user":
		return 3
	case "memory":
		return 4
	case "daily":
		return 5
	case "tools":
		return 6
	case "heartbeat":
		return 7
	case "bootstrap":
		return 8
	default:
		return 9
	}
}
