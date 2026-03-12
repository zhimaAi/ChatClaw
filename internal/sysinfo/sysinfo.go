// Package sysinfo provides best-effort OS type and version detection used
// across services (e.g. browser login URL params, token refresh requests).
package sysinfo

import (
	"os"
	"os/exec"
	"runtime"
	"strings"
)

// OSType returns a display name for the current OS: "Windows", "macOS", "Linux", or runtime.GOOS.
func OSType() string {
	switch runtime.GOOS {
	case "windows":
		return "Windows"
	case "darwin":
		return "macOS"
	case "linux":
		return "Linux"
	default:
		return runtime.GOOS
	}
}

// OSVersion returns a best-effort version string for the current OS
// (e.g. "14.2.1" on macOS, "11" on Windows 11, "22.04" on Ubuntu).
// Returns "" on failure.
func OSVersion() string {
	switch runtime.GOOS {
	case "windows":
		return windowsVersion()
	case "darwin":
		return darwinVersion()
	case "linux":
		return linuxVersion()
	}
	return ""
}

func windowsVersion() string {
	// CurrentBuild >= 22000 → Windows 11; else Windows 10
	cmd := exec.Command("powershell", "-NoProfile", "-Command",
		"$b = (Get-ItemProperty 'HKLM:\\SOFTWARE\\Microsoft\\Windows NT\\CurrentVersion').CurrentBuild; if ([int]$b -ge 22000) { '11' } else { '10' }")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func darwinVersion() string {
	cmd := exec.Command("sw_vers", "-productVersion")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func linuxVersion() string {
	data, err := os.ReadFile("/etc/os-release")
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "VERSION_ID=") {
			v := strings.TrimPrefix(line, "VERSION_ID=")
			v = strings.Trim(v, "\"'\n\r\t ")
			return v
		}
	}
	return ""
}
