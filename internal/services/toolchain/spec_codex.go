package toolchain

import "fmt"

// codexSpec defines the download and extraction rules for the OpenAI Codex CLI.
//
// Archive layout (tar.gz, macOS/Linux):
//
//	codex-aarch64-apple-darwin        (single file, name = "codex-" + target triple)
//
// Archive layout (exe, Windows):
//
//	codex-x86_64-pc-windows-msvc.exe  (standalone binary)
func codexSpec() toolSpec {
	return toolSpec{
		name:             "codex",
		latestReleaseAPI: "https://github.com/openai/codex/releases/latest",
		versionArgs:      []string{"--version"},
		archiveFormat: func(goos string) string {
			if goos == "windows" {
				return "exe"
			}
			return "tar.gz"
		},
		binaryPathInArchive: func(goos, goarch string) string {
			target := codexTarget(goos, goarch)
			if goos == "windows" {
				return fmt.Sprintf("codex-%s.exe", target)
			}
			return fmt.Sprintf("codex-%s", target)
		},
		binaryName: func(goos string) string {
			if goos == "windows" {
				return "codex.exe"
			}
			return "codex"
		},
		downloadURL: func(version, goos, goarch string) string {
			target := codexTarget(goos, goarch)
			if goos == "windows" {
				return fmt.Sprintf(
					"https://github.com/openai/codex/releases/download/rust-v%s/codex-%s.exe",
					version, target,
				)
			}
			return fmt.Sprintf(
				"https://github.com/openai/codex/releases/download/rust-v%s/codex-%s.tar.gz",
				version, target,
			)
		},
	}
}

// codexTarget maps Go GOOS/GOARCH to the codex release target triple.
func codexTarget(goos, goarch string) string {
	switch goos {
	case "darwin":
		switch goarch {
		case "arm64":
			return "aarch64-apple-darwin"
		default:
			return "x86_64-apple-darwin"
		}
	case "windows":
		switch goarch {
		case "arm64":
			return "aarch64-pc-windows-msvc"
		default:
			return "x86_64-pc-windows-msvc"
		}
	default: // linux
		switch goarch {
		case "arm64":
			return "aarch64-unknown-linux-gnu"
		default:
			return "x86_64-unknown-linux-gnu"
		}
	}
}
