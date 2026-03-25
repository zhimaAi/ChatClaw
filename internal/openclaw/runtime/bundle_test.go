package openclawruntime

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestResolveBundledRuntimeFromCandidatesPrefersUserRuntime(t *testing.T) {
	t.Parallel()

	stateDir := t.TempDir()
	userRuntime := filepath.Join(t.TempDir(), "user")
	embeddedRuntime := filepath.Join(t.TempDir(), "embedded")

	writeRuntimeFixture(t, userRuntime, true)
	writeRuntimeFixture(t, embeddedRuntime, true)

	bundle, err := resolveBundledRuntimeFromCandidates(
		stateDir,
		runtime.GOOS,
		runtime.GOARCH,
		runtime.GOOS+"-"+runtime.GOARCH,
		[]runtimeCandidate{
			{Root: userRuntime, Source: runtimeSourceUser},
			{Root: embeddedRuntime, Source: runtimeSourceEmbedded},
		},
	)
	if err != nil {
		t.Fatalf("resolve runtime: %v", err)
	}
	if bundle.Root != userRuntime {
		t.Fatalf("expected user runtime, got %s", bundle.Root)
	}
	if bundle.Source != runtimeSourceUser {
		t.Fatalf("expected source %q, got %q", runtimeSourceUser, bundle.Source)
	}
}

func TestResolveBundledRuntimeFromCandidatesFallsBackFromBrokenUserRuntime(t *testing.T) {
	t.Parallel()

	stateDir := t.TempDir()
	userRuntime := filepath.Join(t.TempDir(), "user")
	embeddedRuntime := filepath.Join(t.TempDir(), "embedded")

	if err := os.MkdirAll(userRuntime, 0o755); err != nil {
		t.Fatalf("create user runtime dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(userRuntime, "manifest.json"), []byte("{bad json"), 0o644); err != nil {
		t.Fatalf("write broken user manifest: %v", err)
	}
	writeRuntimeFixture(t, embeddedRuntime, true)

	bundle, err := resolveBundledRuntimeFromCandidates(
		stateDir,
		runtime.GOOS,
		runtime.GOARCH,
		runtime.GOOS+"-"+runtime.GOARCH,
		[]runtimeCandidate{
			{Root: userRuntime, Source: runtimeSourceUser},
			{Root: embeddedRuntime, Source: runtimeSourceEmbedded},
		},
	)
	if err != nil {
		t.Fatalf("resolve runtime with fallback: %v", err)
	}
	if bundle.Root != embeddedRuntime {
		t.Fatalf("expected embedded fallback, got %s", bundle.Root)
	}
	if bundle.Source != runtimeSourceEmbedded {
		t.Fatalf("expected source %q, got %q", runtimeSourceEmbedded, bundle.Source)
	}
}

func writeRuntimeFixture(t *testing.T, root string, withCLI bool) {
	t.Helper()

	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatalf("create runtime root: %v", err)
	}
	manifest := []byte(
		"{\n" +
			`  "openclawVersion": "2026.3.23",` + "\n" +
			`  "nodeVersion": "24.14.0",` + "\n" +
			`  "platform": "` + runtime.GOOS + `",` + "\n" +
			`  "arch": "` + runtime.GOARCH + `"` + "\n" +
			"}\n",
	)
	if err := os.WriteFile(filepath.Join(root, "manifest.json"), manifest, 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	if !withCLI {
		return
	}

	cliPath := filepath.Join(root, "bin", cliName())
	if err := os.MkdirAll(filepath.Dir(cliPath), 0o755); err != nil {
		t.Fatalf("create cli dir: %v", err)
	}
	mode := os.FileMode(0o644)
	if runtime.GOOS != "windows" {
		mode = 0o755
	}
	if err := os.WriteFile(cliPath, []byte("placeholder"), mode); err != nil {
		t.Fatalf("write cli: %v", err)
	}
}
