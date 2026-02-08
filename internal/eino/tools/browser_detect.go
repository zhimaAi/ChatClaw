package tools

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// detectBrowserPath returns the path to the first available Chromium-based browser.
// Detection order: Chrome -> Edge -> Brave.
// Returns an empty string when no browser is found (chromedp will fall back to its
// own detection logic in that case).
func detectBrowserPath() string {
	candidates := browserCandidates()
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	// Try $PATH lookup as last resort
	for _, name := range []string{"google-chrome", "google-chrome-stable", "chromium-browser", "chromium", "microsoft-edge"} {
		if p, err := exec.LookPath(name); err == nil {
			return p
		}
	}
	return ""
}

// browserCandidates returns a list of well-known browser executable paths for the
// current OS, ordered by preference (Chrome > Edge > Brave).
func browserCandidates() []string {
	switch runtime.GOOS {
	case "darwin":
		return []string{
			"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
			"/Applications/Microsoft Edge.app/Contents/MacOS/Microsoft Edge",
			"/Applications/Brave Browser.app/Contents/MacOS/Brave Browser",
		}
	case "windows":
		// Expand common env-based paths
		programFiles := os.Getenv("ProgramFiles")
		if programFiles == "" {
			programFiles = `C:\Program Files`
		}
		programFilesX86 := os.Getenv("ProgramFiles(x86)")
		if programFilesX86 == "" {
			programFilesX86 = `C:\Program Files (x86)`
		}
		localAppData := os.Getenv("LOCALAPPDATA")

		paths := []string{
			filepath.Join(programFiles, `Google\Chrome\Application\chrome.exe`),
			filepath.Join(programFilesX86, `Google\Chrome\Application\chrome.exe`),
		}
		if localAppData != "" {
			paths = append(paths, filepath.Join(localAppData, `Google\Chrome\Application\chrome.exe`))
		}
		paths = append(paths,
			filepath.Join(programFiles, `Microsoft\Edge\Application\msedge.exe`),
			filepath.Join(programFilesX86, `Microsoft\Edge\Application\msedge.exe`),
			filepath.Join(programFiles, `BraveSoftware\Brave-Browser\Application\brave.exe`),
			filepath.Join(programFilesX86, `BraveSoftware\Brave-Browser\Application\brave.exe`),
		)
		return paths
	default: // linux / freebsd
		return []string{
			"/usr/bin/google-chrome",
			"/usr/bin/google-chrome-stable",
			"/usr/bin/chromium-browser",
			"/usr/bin/chromium",
			"/usr/bin/microsoft-edge",
			"/usr/bin/brave-browser",
			"/snap/bin/chromium",
		}
	}
}
