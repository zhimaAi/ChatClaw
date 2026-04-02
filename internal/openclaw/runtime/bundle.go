package openclawruntime

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"chatclaw/internal/define"
	openclaw "chatclaw/internal/openclaw"
)

type bundledRuntimeManifest struct {
	OpenClawVersion string `json:"openclawVersion"`
	NodeVersion     string `json:"nodeVersion"`
	Platform        string `json:"platform"`
	Arch            string `json:"arch"`
}

type bundledRuntime struct {
	Root       string
	CLIPath    string
	StateDir   string
	ConfigPath string
	LogsDir    string
	Source     string
	Manifest   bundledRuntimeManifest
}

type runtimeCandidate struct {
	Root   string
	Source string
}

const (
	runtimeSourceUser        = "user"
	runtimeSourceEmbedded    = "embedded"
	runtimeSourceDevelopment = "development"
)

func resolveBundledRuntime() (*bundledRuntime, error) {
	target := runtime.GOOS + "-" + runtime.GOARCH
	stateDir, err := define.OpenClawDataRootDir()
	if err != nil {
		return nil, fmt.Errorf("resolve openclaw data directory: %w", err)
	}

	candidates, err := bundledRuntimeCandidates(target)
	if err != nil {
		return nil, err
	}

	return resolveBundledRuntimeFromCandidates(stateDir, runtime.GOOS, runtime.GOARCH, target, candidates)
}

func resolveBundledRuntimeFromCandidates(
	stateDir string,
	expectedOS string,
	expectedArch string,
	target string,
	candidates []runtimeCandidate,
) (*bundledRuntime, error) {
	checked := make([]string, 0, len(candidates))
	issues := make([]string, 0)
	for _, candidate := range candidates {
		root := filepath.Clean(strings.TrimSpace(candidate.Root))
		if root == "" || root == "." {
			continue
		}
		checked = append(checked, root)
		bundle, err := loadBundledRuntimeCandidate(stateDir, expectedOS, expectedArch, candidate)
		if err == nil {
			return bundle, nil
		}
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		issues = append(issues, fmt.Sprintf("%s (%s): %v", root, candidate.Source, err))
	}

	if len(checked) == 0 {
		return nil, fmt.Errorf("OpenClaw runtime lookup has no candidates for %s", target)
	}

	msg := fmt.Sprintf("OpenClaw runtime not found for %s; checked %s", target, strings.Join(checked, ", "))
	if len(issues) > 0 {
		msg += "; invalid candidates: " + strings.Join(issues, "; ")
	}
	return nil, errors.New(msg)
}

func loadBundledRuntimeCandidate(
	stateDir string,
	expectedOS string,
	expectedArch string,
	candidate runtimeCandidate,
) (*bundledRuntime, error) {
	root := filepath.Clean(strings.TrimSpace(candidate.Root))
	manifestPath := filepath.Join(root, "manifest.json")
	raw, err := os.ReadFile(manifestPath)
	if err != nil {
		return nil, err
	}

	var manifest bundledRuntimeManifest
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return nil, fmt.Errorf("parse runtime manifest: %w", err)
	}
	if manifest.Platform != expectedOS || manifest.Arch != expectedArch {
		return nil, fmt.Errorf("target mismatch: got %s/%s want %s/%s",
			manifest.Platform, manifest.Arch, expectedOS, expectedArch)
	}

	cliPath := filepath.Join(root, "bin", cliName())
	if _, err := os.Stat(cliPath); err != nil {
		return nil, fmt.Errorf("runtime CLI missing at %s: %w", cliPath, err)
	}

	return &bundledRuntime{
		Root:       root,
		CLIPath:    cliPath,
		StateDir:   stateDir,
		ConfigPath: filepath.Join(stateDir, "openclaw.json"),
		LogsDir:    filepath.Join(stateDir, "logs"),
		Source:     candidate.Source,
		Manifest:   manifest,
	}, nil
}

func bundledRuntimeCandidates(target string) ([]runtimeCandidate, error) {
	execPath, _ := os.Executable()
	if execPath == "" {
		execPath = "."
	}
	execDir := filepath.Dir(execPath)
	cwd, _ := os.Getwd()

	var candidates []runtimeCandidate

	// --- Embedded (bundled with installer / NSIS) — fallback priority ---
	// Used when no user override exists. Bundled runtime is the authoritative
	// version for production when the user has not initiated an upgrade.
	if runtime.GOOS == "darwin" {
		candidates = append(candidates, runtimeCandidate{
			Root:   filepath.Clean(filepath.Join(execDir, "..", "Resources", "rt", target)),
			Source: runtimeSourceEmbedded,
		})
	}
	candidates = append(candidates, runtimeCandidate{
		Root:   filepath.Join(execDir, "rt", target),
		Source: runtimeSourceEmbedded,
	})

	// --- Development paths — only in dev builds ---
	if cwd != "" {
		candidates = append(candidates, runtimeCandidate{
			Root:   filepath.Join(cwd, "build", "openclaw-runtime", target),
			Source: runtimeSourceDevelopment,
		})
	}
	for dir := filepath.Clean(execDir); ; {
		candidates = append(candidates, runtimeCandidate{
			Root:   filepath.Join(dir, "build", "openclaw-runtime", target),
			Source: runtimeSourceDevelopment,
		})
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	// --- User overrides (OSS install / UI upgrade) — highest priority ---
	// These take precedence over bundled and development paths so that a
	// user-initiated upgrade always wins over stale development or bundled runtimes.
	if current, err := openclaw.UserRuntimeCurrentDir(target); err == nil {
		candidates = append(candidates, runtimeCandidate{Root: current, Source: runtimeSourceUser})
	}
	if userTarget, err := openclaw.UserRuntimeTargetDir(target); err == nil {
		candidates = append(candidates, runtimeCandidate{Root: userTarget, Source: runtimeSourceUser})
	}

	// Deduplicate
	seen := map[string]struct{}{}
	out := make([]runtimeCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		p := filepath.Clean(strings.TrimSpace(candidate.Root))
		if p == "" {
			continue
		}
		if _, ok := seen[p]; ok {
			continue
		}
		seen[p] = struct{}{}
		candidate.Root = p
		out = append(out, candidate)
	}
	return out, nil
}

func cliName() string {
	if runtime.GOOS == "windows" {
		return "openclaw.cmd"
	}
	return "openclaw"
}

// BundledSkillsDir returns the skills directory shipped inside the bundled OpenClaw npm package
// (node_modules/openclaw/skills), if present. Matches OpenClaw docs: bundled install skills.
func BundledSkillsDir() (string, error) {
	b, err := resolveBundledRuntime()
	if err != nil {
		return "", err
	}
	p := filepath.Join(b.Root, "node_modules", "openclaw", "skills")
	st, err := os.Stat(p)
	if err != nil || !st.IsDir() {
		return "", fmt.Errorf("bundled openclaw skills directory not found")
	}
	return p, nil
}
