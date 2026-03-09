package tools

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/cloudwego/eino/adk/filesystem"
)

func newSandboxBackend(t *testing.T) (*Backend, string) {
	t.Helper()
	home := t.TempDir()

	workDir := filepath.Join(home, "workspace")
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		t.Fatal(err)
	}

	b := NewBackend(&BackendConfig{
		HomeDir:  home,
		WorkDir:  workDir,
		CodexBin: "/usr/local/bin/fake-codex",
	})
	return b, home
}

func newNativeBackend(t *testing.T) (*Backend, string) {
	t.Helper()
	home := t.TempDir()

	workDir := filepath.Join(home, "workspace")
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		t.Fatal(err)
	}

	b := NewBackend(&BackendConfig{
		HomeDir: home,
		WorkDir: workDir,
	})
	return b, home
}

// --- Sandbox mode tests: sensitive paths should be blocked ---

func TestIsSensitivePath_Sandbox(t *testing.T) {
	b, home := newSandboxBackend(t)

	sensitive := []string{
		filepath.Join(home, ".ssh"),
		filepath.Join(home, ".ssh", "id_rsa"),
		filepath.Join(home, ".ssh", "config"),
		filepath.Join(home, ".aws"),
		filepath.Join(home, ".aws", "credentials"),
		filepath.Join(home, ".gnupg", "pubring.kbx"),
		filepath.Join(home, ".kube", "config"),
		filepath.Join(home, ".docker", "config.json"),
		filepath.Join(home, ".npmrc"),
		filepath.Join(home, ".netrc"),
		filepath.Join(home, ".git-credentials"),
		filepath.Join(home, ".config", "gcloud", "credentials.db"),
		filepath.Join(home, ".config", "gh", "hosts.yml"),
		filepath.Join(home, "project", ".env"),
		filepath.Join(home, "project", ".env.local"),
		filepath.Join(home, "project", ".env.production"),
		filepath.Join(home, "project", "credentials.json"),
		filepath.Join(home, "project", "secrets.yaml"),
	}

	for _, p := range sensitive {
		if !b.IsSensitivePath(p) {
			t.Errorf("sandbox: expected %q to be sensitive, but was not", p)
		}
	}

	safe := []string{
		filepath.Join(home, "workspace", "main.go"),
		filepath.Join(home, "Documents", "readme.md"),
		filepath.Join(home, ".config", "some-app", "config.json"),
		filepath.Join(home, ".bashrc"),
		filepath.Join(home, "code", "project", "app.py"),
	}

	for _, p := range safe {
		if b.IsSensitivePath(p) {
			t.Errorf("sandbox: expected %q to be safe, but was marked sensitive", p)
		}
	}
}

// --- Native mode tests: everything should be accessible ---

func TestIsSensitivePath_NativeAllowsAll(t *testing.T) {
	b, home := newNativeBackend(t)

	paths := []string{
		filepath.Join(home, ".ssh", "id_rsa"),
		filepath.Join(home, ".aws", "credentials"),
		filepath.Join(home, ".gnupg", "pubring.kbx"),
		filepath.Join(home, ".kube", "config"),
		filepath.Join(home, "project", ".env"),
		filepath.Join(home, "project", "credentials.json"),
	}

	for _, p := range paths {
		if b.IsSensitivePath(p) {
			t.Errorf("native: %q should NOT be marked sensitive in native mode", p)
		}
	}
}

func TestResolvePath_Sandbox_BlocksSensitive(t *testing.T) {
	b, _ := newSandboxBackend(t)

	cases := []string{
		".ssh/id_rsa",
		".aws/credentials",
		".gnupg/pubring.kbx",
		".kube/config",
	}
	for _, c := range cases {
		_, err := b.ResolvePath(c)
		if err == nil {
			t.Errorf("sandbox ResolvePath(%q): expected error, got nil", c)
		}
	}

	_, err := b.ResolvePath("workspace/main.go")
	if err != nil {
		t.Errorf("sandbox ResolvePath(workspace/main.go): unexpected error: %v", err)
	}
}

func TestResolvePath_Native_AllowsSensitive(t *testing.T) {
	b, home := newNativeBackend(t)

	sshDir := filepath.Join(home, ".ssh")
	if err := os.MkdirAll(sshDir, 0o755); err != nil {
		t.Fatal(err)
	}

	result, err := b.ResolvePath(".ssh/id_rsa")
	if err != nil {
		t.Errorf("native ResolvePath(.ssh/id_rsa): unexpected error: %v", err)
	}
	expected := filepath.Join(home, ".ssh", "id_rsa")
	if result != expected {
		t.Errorf("native ResolvePath: got %q, want %q", result, expected)
	}
}

func TestLsInfo_Sandbox_FiltersSensitive(t *testing.T) {
	b, home := newSandboxBackend(t)

	dirs := []string{".ssh", ".aws", "safe-dir"}
	for _, d := range dirs {
		if err := os.MkdirAll(filepath.Join(home, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	results, err := b.LsInfo(context.Background(), &filesystem.LsInfoRequest{Path: home})
	if err != nil {
		t.Fatal(err)
	}

	for _, fi := range results {
		base := filepath.Base(fi.Path)
		if base == ".ssh" || base == ".aws" {
			t.Errorf("sandbox LsInfo should have filtered %q", fi.Path)
		}
	}

	foundSafe := false
	for _, fi := range results {
		if filepath.Base(fi.Path) == "safe-dir" {
			foundSafe = true
		}
	}
	if !foundSafe {
		t.Error("sandbox LsInfo should still list non-sensitive directories")
	}
}

func TestLsInfo_Native_ShowsAll(t *testing.T) {
	b, home := newNativeBackend(t)

	dirs := []string{".ssh", ".aws", "safe-dir"}
	for _, d := range dirs {
		if err := os.MkdirAll(filepath.Join(home, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	results, err := b.LsInfo(context.Background(), &filesystem.LsInfoRequest{Path: home})
	if err != nil {
		t.Fatal(err)
	}

	names := map[string]bool{}
	for _, fi := range results {
		names[filepath.Base(fi.Path)] = true
	}

	for _, expected := range []string{".ssh", ".aws", "safe-dir"} {
		if !names[expected] {
			t.Errorf("native LsInfo should list %q", expected)
		}
	}
}

func TestRead_Sandbox_BlocksSensitive(t *testing.T) {
	b, home := newSandboxBackend(t)

	sshDir := filepath.Join(home, ".ssh")
	if err := os.MkdirAll(sshDir, 0o755); err != nil {
		t.Fatal(err)
	}
	keyFile := filepath.Join(sshDir, "id_rsa")
	if err := os.WriteFile(keyFile, []byte("secret-key"), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := b.Read(context.Background(), &filesystem.ReadRequest{
		FilePath: keyFile,
		Offset:   1,
		Limit:    100,
	})
	if err == nil {
		t.Error("sandbox Read on .ssh/id_rsa should return error")
	}
}

func TestRead_Native_AllowsSensitive(t *testing.T) {
	b, home := newNativeBackend(t)

	sshDir := filepath.Join(home, ".ssh")
	if err := os.MkdirAll(sshDir, 0o755); err != nil {
		t.Fatal(err)
	}
	keyFile := filepath.Join(sshDir, "id_rsa")
	if err := os.WriteFile(keyFile, []byte("secret-key"), 0o600); err != nil {
		t.Fatal(err)
	}

	content, err := b.Read(context.Background(), &filesystem.ReadRequest{
		FilePath: keyFile,
		Offset:   1,
		Limit:    100,
	})
	if err != nil {
		t.Errorf("native Read on .ssh/id_rsa: unexpected error: %v", err)
	}
	if content != "secret-key" {
		t.Errorf("native Read: got %q, want %q", content, "secret-key")
	}
}

func TestGrepRaw_Sandbox_SkipsSensitive(t *testing.T) {
	b, home := newSandboxBackend(t)

	sshDir := filepath.Join(home, ".ssh")
	if err := os.MkdirAll(sshDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sshDir, "config"), []byte("Host secret-server"), 0o600); err != nil {
		t.Fatal(err)
	}

	safeDir := filepath.Join(home, "project")
	if err := os.MkdirAll(safeDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(safeDir, "main.go"), []byte("Host safe-server"), 0o644); err != nil {
		t.Fatal(err)
	}

	matches, err := b.GrepRaw(context.Background(), &filesystem.GrepRequest{
		Pattern: "Host",
		Path:    home,
	})
	if err != nil {
		t.Fatal(err)
	}

	for _, m := range matches {
		if filepath.Base(filepath.Dir(m.Path)) == ".ssh" {
			t.Errorf("sandbox GrepRaw should skip .ssh, but matched %q", m.Path)
		}
	}

	if len(matches) == 0 {
		t.Error("sandbox GrepRaw should still find matches in safe directories")
	}
}

func TestGrepRaw_Native_IncludesAll(t *testing.T) {
	b, home := newNativeBackend(t)

	sshDir := filepath.Join(home, ".ssh")
	if err := os.MkdirAll(sshDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sshDir, "config"), []byte("Host secret-server"), 0o600); err != nil {
		t.Fatal(err)
	}

	matches, err := b.GrepRaw(context.Background(), &filesystem.GrepRequest{
		Pattern: "Host",
		Path:    home,
	})
	if err != nil {
		t.Fatal(err)
	}

	foundSSH := false
	for _, m := range matches {
		if filepath.Base(filepath.Dir(m.Path)) == ".ssh" {
			foundSSH = true
		}
	}
	if !foundSSH {
		t.Error("native GrepRaw should include .ssh results")
	}
}

func TestValidateSensitivePaths(t *testing.T) {
	homeDir := "/Users/testuser"

	blocked := []string{
		"cat ~/.ssh/id_rsa",
		"cat /Users/testuser/.ssh/id_rsa",
		"head ~/.aws/credentials",
		"tail -f ~/.gnupg/pubring.kbx",
		"cp ~/.kube/config /tmp/",
		"base64 ~/.ssh/id_ed25519",
	}
	for _, cmd := range blocked {
		if err := validateSensitivePaths(cmd, homeDir); err == nil {
			t.Errorf("validateSensitivePaths(%q): expected error, got nil", cmd)
		}
	}

	allowed := []string{
		"ls -la",
		"echo hello",
		"cat workspace/main.go",
		"npm install",
		"go build ./...",
		"python script.py",
	}
	for _, cmd := range allowed {
		if err := validateSensitivePaths(cmd, homeDir); err != nil {
			t.Errorf("validateSensitivePaths(%q): unexpected error: %v", cmd, err)
		}
	}
}

func TestSensitiveFileNames_Sandbox_Blocked(t *testing.T) {
	b, home := newSandboxBackend(t)

	projectDir := filepath.Join(home, "project")
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatal(err)
	}

	envFile := filepath.Join(projectDir, ".env")
	if err := os.WriteFile(envFile, []byte("API_KEY=secret"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := b.ResolvePath(filepath.Join("project", ".env"))
	if err == nil {
		t.Error("sandbox ResolvePath should block .env files")
	}
}

func TestSensitiveFileNames_Native_Allowed(t *testing.T) {
	b, home := newNativeBackend(t)

	projectDir := filepath.Join(home, "project")
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatal(err)
	}

	envFile := filepath.Join(projectDir, ".env")
	if err := os.WriteFile(envFile, []byte("API_KEY=secret"), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := b.ResolvePath(filepath.Join("project", ".env"))
	if err != nil {
		t.Errorf("native ResolvePath should allow .env files, got error: %v", err)
	}
	if result != envFile {
		t.Errorf("native ResolvePath: got %q, want %q", result, envFile)
	}
}
