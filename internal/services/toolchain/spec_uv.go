package toolchain

import "fmt"

// uvSpec defines the download and extraction rules for the uv tool.
//
// Archive layout (tar.gz):
//
//	uv-{target}/
//	  uv
//	  uvx
//
// Archive layout (zip, Windows):
//
//	uv-{target}/
//	  uv.exe
//	  uvx.exe
func uvSpec() toolSpec {
	return toolSpec{
		name:             "uv",
		latestReleaseAPI: "https://github.com/astral-sh/uv/releases/latest",
		versionArgs:      []string{"--version"},
		archiveFormat: func(goos string) string {
			if goos == "windows" {
				return "zip"
			}
			return "tar.gz"
		},
		binaryPathInArchive: func(goos, goarch string) string {
			if goos == "windows" {
				return "uv.exe"
			}
			return "uv"
		},
		binaryName: func(goos string) string {
			if goos == "windows" {
				return "uv.exe"
			}
			return "uv"
		},
		downloadURL: func(version, goos, goarch string) string {
			target := uvTarget(goos, goarch)
			ext := "tar.gz"
			if goos == "windows" {
				ext = "zip"
			}
			return fmt.Sprintf(
				"https://github.com/astral-sh/uv/releases/download/%s/uv-%s.%s",
				version, target, ext,
			)
		},
		aliases: func(goos string) []string {
			if goos == "windows" {
				return []string{"uvx.exe"}
			}
			return []string{"uvx"}
		},
	}
}

// uvTarget maps Go GOOS/GOARCH to the uv release target triple.
func uvTarget(goos, goarch string) string {
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
