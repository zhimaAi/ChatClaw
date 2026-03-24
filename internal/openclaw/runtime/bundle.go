package openclawruntime

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"chatclaw/internal/define"
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
	Manifest   bundledRuntimeManifest
}

func resolveBundledRuntime() (*bundledRuntime, error) {
	target := runtime.GOOS + "-" + runtime.GOARCH
	stateDir, err := define.OpenClawDataRootDir()
	if err != nil {
		return nil, fmt.Errorf("resolve openclaw data directory: %w", err)
	}

	candidates := bundledRuntimeCandidates(target)
	for _, candidate := range candidates {
		raw, err := os.ReadFile(filepath.Join(candidate, "manifest.json"))
		if err != nil {
			continue
		}
		var manifest bundledRuntimeManifest
		if err := json.Unmarshal(raw, &manifest); err != nil {
			return nil, fmt.Errorf("parse bundled runtime manifest at %s: %w", candidate, err)
		}
		if manifest.Platform != runtime.GOOS || manifest.Arch != runtime.GOARCH {
			return nil, fmt.Errorf("bundled runtime target mismatch at %s: got %s/%s want %s/%s",
				candidate, manifest.Platform, manifest.Arch, runtime.GOOS, runtime.GOARCH)
		}

		cliPath := filepath.Join(candidate, "bin", cliName())
		if _, err := os.Stat(cliPath); err != nil {
			return nil, fmt.Errorf("bundled runtime CLI is missing at %s", cliPath)
		}

		return &bundledRuntime{
			Root:       candidate,
			CLIPath:    cliPath,
			StateDir:   stateDir,
			ConfigPath: filepath.Join(stateDir, "openclaw.json"),
			LogsDir:    filepath.Join(stateDir, "logs"),
			Manifest:   manifest,
		}, nil
	}

	return nil, fmt.Errorf("bundled OpenClaw runtime not found for %s; checked %s",
		target, strings.Join(candidates, ", "))
}

func bundledRuntimeCandidates(target string) []string {
	execPath, _ := os.Executable()
	if execPath == "" {
		execPath = "."
	}
	execDir := filepath.Dir(execPath)
	cwd, _ := os.Getwd()

	var candidates []string
	if runtime.GOOS == "darwin" {
		candidates = append(candidates, filepath.Clean(filepath.Join(execDir, "..", "Resources", "rt", target)))
	}
	candidates = append(candidates, filepath.Join(execDir, "rt", target))
	if cwd != "" {
		candidates = append(candidates, filepath.Join(cwd, "build", "openclaw-runtime", target))
	}

	// Walk up from exec dir looking for build/openclaw-runtime/<target>
	for dir := filepath.Clean(execDir); ; {
		candidates = append(candidates, filepath.Join(dir, "build", "openclaw-runtime", target))
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	// Deduplicate
	seen := map[string]struct{}{}
	out := make([]string, 0, len(candidates))
	for _, p := range candidates {
		if p == "" {
			continue
		}
		if _, ok := seen[p]; ok {
			continue
		}
		seen[p] = struct{}{}
		out = append(out, p)
	}
	return out
}

func cliName() string {
	if runtime.GOOS == "windows" {
		return "openclaw.cmd"
	}
	return "openclaw"
}
