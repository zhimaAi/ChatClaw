package toolchain

import "fmt"

// bunSpec defines the download and extraction rules for the bun runtime.
//
// Archive layout (zip):
//
//	bun-{target}/
//	  bun        (or bun.exe on Windows)
func bunSpec() toolSpec {
	return toolSpec{
		name:             "bun",
		latestReleaseAPI: "https://github.com/oven-sh/bun/releases/latest",
		versionArgs:      []string{"--version"},
		archiveFormat: func(_ string) string {
			return "zip"
		},
		binaryPathInArchive: func(goos, _ string) string {
			if goos == "windows" {
				return "bun.exe"
			}
			return "bun"
		},
		binaryName: func(goos string) string {
			if goos == "windows" {
				return "bun.exe"
			}
			return "bun"
		},
		downloadURL: func(version, goos, goarch string) string {
			target := bunTarget(goos, goarch)
			return fmt.Sprintf(
				"https://github.com/oven-sh/bun/releases/download/bun-v%s/bun-%s.zip",
				version, target,
			)
		},
	}
}

// bunTarget maps Go GOOS/GOARCH to the bun release target name.
func bunTarget(goos, goarch string) string {
	switch goos {
	case "darwin":
		switch goarch {
		case "arm64":
			return "darwin-aarch64"
		default:
			return "darwin-x64"
		}
	case "windows":
		return "windows-x64"
	default: // linux
		switch goarch {
		case "arm64":
			return "linux-aarch64"
		default:
			return "linux-x64"
		}
	}
}
