package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/adk/filesystem"
	"github.com/cloudwego/eino/adk/middlewares/plantask"
)

// diskBackend implements both reduction.Backend (Write) and plantask.Backend
// (LsInfo, Read, Write, Delete) using the local filesystem. Offloaded content
// can be read back via the standard read_file tool.
type diskBackend struct{}

func (d *diskBackend) LsInfo(_ context.Context, req *filesystem.LsInfoRequest) ([]filesystem.FileInfo, error) {
	entries, err := os.ReadDir(req.Path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	result := make([]filesystem.FileInfo, 0, len(entries))
	for _, e := range entries {
		info, infoErr := e.Info()
		if infoErr != nil {
			continue
		}
		result = append(result, filesystem.FileInfo{
			Path:       e.Name(),
			IsDir:      e.IsDir(),
			Size:       info.Size(),
			ModifiedAt: info.ModTime().Format("2006-01-02T15:04:05Z07:00"),
		})
	}
	return result, nil
}

func (d *diskBackend) Read(_ context.Context, req *filesystem.ReadRequest) (string, error) {
	data, err := os.ReadFile(req.FilePath)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func (d *diskBackend) Write(_ context.Context, req *filesystem.WriteRequest) error {
	dir := filepath.Dir(req.FilePath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", dir, err)
	}
	return os.WriteFile(req.FilePath, []byte(req.Content), 0o644)
}

func (d *diskBackend) Delete(_ context.Context, req *plantask.DeleteRequest) error {
	err := os.Remove(req.FilePath)
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

// writeTranscript serializes the full conversation history to a JSONL file.
// Each message is written as a single JSON line for easy streaming reads.
func writeTranscript(path string, messages []adk.Message) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", dir, err)
	}

	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	enc := json.NewEncoder(f)
	enc.SetEscapeHTML(false)
	for _, msg := range messages {
		if err := enc.Encode(msg); err != nil {
			return err
		}
	}
	return nil
}
