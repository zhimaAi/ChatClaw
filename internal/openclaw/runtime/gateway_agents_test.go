package openclawruntime

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEnsureLongTermMemoryFileCreatesMissingFile(t *testing.T) {
	t.Parallel()

	workspaceDir := filepath.Join(t.TempDir(), "workspace-main")
	if err := ensureLongTermMemoryFile(workspaceDir); err != nil {
		t.Fatalf("ensureLongTermMemoryFile() error = %v", err)
	}

	info, err := os.Stat(filepath.Join(workspaceDir, longTermMemoryFileName))
	if err != nil {
		t.Fatalf("stat MEMORY.md: %v", err)
	}
	if info.IsDir() {
		t.Fatalf("expected MEMORY.md to be a file")
	}
	if info.Size() != 0 {
		t.Fatalf("expected empty MEMORY.md, got size %d", info.Size())
	}
}

func TestEnsureLongTermMemoryFileKeepsExistingContent(t *testing.T) {
	t.Parallel()

	workspaceDir := t.TempDir()
	path := filepath.Join(workspaceDir, longTermMemoryFileName)
	const want = "remember this\n"
	if err := os.WriteFile(path, []byte(want), 0o644); err != nil {
		t.Fatalf("seed MEMORY.md: %v", err)
	}

	if err := ensureLongTermMemoryFile(workspaceDir); err != nil {
		t.Fatalf("ensureLongTermMemoryFile() error = %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read MEMORY.md: %v", err)
	}
	if string(got) != want {
		t.Fatalf("unexpected MEMORY.md content: got %q want %q", string(got), want)
	}
}
