//go:build darwin

package windows

import (
	"encoding/base64"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

func currentDarwinProcessNamesForFilter() map[string]struct{} {
	names := make(map[string]struct{})

	add := func(name string) {
		normalized := strings.ToLower(strings.TrimSpace(name))
		if normalized == "" {
			return
		}
		names[normalized] = struct{}{}
		names[strings.TrimSuffix(normalized, filepath.Ext(normalized))] = struct{}{}
	}

	if exePath, err := os.Executable(); err == nil {
		add(filepath.Base(exePath))
	}
	if len(os.Args) > 0 {
		add(filepath.Base(os.Args[0]))
	}

	return names
}

func listRunningApps() ([]SnapAppCandidate, error) {
	script := `
tell application "System Events"
  set outText to ""
  set appList to application processes whose background only is false
  repeat with p in appList
    try
      set appName to name of p
      set isVisible to visible of p
      if isVisible is true then
        set bundleID to ""
        set appPath to ""
        try
          set bundleID to bundle identifier of p
        end try
        try
          set appPath to POSIX path of (file of p as alias)
        end try
        set outText to outText & appName & "|" & bundleID & "|" & appPath & linefeed
      end if
    end try
  end repeat
  return outText
end tell
`
	cmd := exec.Command("osascript", "-e", script)
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	raw := strings.TrimSpace(string(output))
	if raw == "" {
		return []SnapAppCandidate{}, nil
	}

	lines := strings.Split(raw, "\n")
	selfProcessNames := currentDarwinProcessNamesForFilter()
	seen := make(map[string]struct{}, len(lines))
	iconCache := make(map[string]string, len(lines))
	apps := make([]SnapAppCandidate, 0, len(lines))

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "|", 3)
		name := strings.TrimSpace(parts[0])
		if name == "" {
			continue
		}

		processName := name
		appPath := ""
		if len(parts) > 1 {
			bundleID := strings.TrimSpace(parts[1])
			if bundleID != "" {
				processName = bundleID
			}
		}
		if len(parts) > 2 {
			appPath = strings.TrimSpace(parts[2])
		}

		lower := strings.ToLower(processName)
		nameLower := strings.ToLower(name)
		if _, isSelfProcess := selfProcessNames[lower]; isSelfProcess {
			continue
		}
		if _, isSelfName := selfProcessNames[nameLower]; isSelfName {
			continue
		}
		if _, exists := seen[lower]; exists {
			continue
		}
		seen[lower] = struct{}{}

		icon := inferSnapIcon(name, processName)
		if icon == "app" {
			if cached, ok := iconCache[appPath]; ok {
				if cached != "" {
					icon = cached
				}
			} else {
				dataURL := buildDarwinAppIconDataURL(appPath)
				iconCache[appPath] = dataURL
				if dataURL != "" {
					icon = dataURL
				}
			}
		}

		apps = append(apps, SnapAppCandidate{
			Name:        name,
			ProcessName: processName,
			Icon:        icon,
		})
	}

	sort.Slice(apps, func(i, j int) bool {
		return strings.ToLower(apps[i].Name) < strings.ToLower(apps[j].Name)
	})
	return apps, nil
}

func buildDarwinAppIconDataURL(appPath string) string {
	iconPath := resolveDarwinAppIconPath(appPath)
	if iconPath == "" {
		return ""
	}

	tmpFile, err := os.CreateTemp("", "snap-app-icon-*.png")
	if err != nil {
		return ""
	}
	tmpPath := tmpFile.Name()
	_ = tmpFile.Close()
	defer os.Remove(tmpPath)

	if err := exec.Command("sips", "-s", "format", "png", iconPath, "--out", tmpPath).Run(); err != nil {
		return ""
	}
	content, err := os.ReadFile(tmpPath)
	if err != nil || len(content) == 0 {
		return ""
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(content)
}

func resolveDarwinAppIconPath(appPath string) string {
	trimmed := strings.TrimSpace(appPath)
	if trimmed == "" {
		return ""
	}
	resourcesDir := filepath.Join(trimmed, "Contents", "Resources")
	infoPlist := filepath.Join(trimmed, "Contents", "Info.plist")
	if _, err := os.Stat(resourcesDir); err != nil {
		return ""
	}

	plistQueries := []string{
		"Print :CFBundleIconFile",
		"Print :CFBundleIcons:CFBundlePrimaryIcon:CFBundleIconFiles:0",
	}
	for _, query := range plistQueries {
		out, err := exec.Command("/usr/libexec/PlistBuddy", "-c", query, infoPlist).Output()
		if err != nil {
			continue
		}
		iconName := strings.TrimSpace(string(out))
		if iconName == "" {
			continue
		}
		candidate := iconName
		if filepath.Ext(candidate) == "" {
			candidate += ".icns"
		}
		iconPath := filepath.Join(resourcesDir, candidate)
		if _, err := os.Stat(iconPath); err == nil {
			return iconPath
		}
	}

	candidates, err := filepath.Glob(filepath.Join(resourcesDir, "*.icns"))
	if err != nil || len(candidates) == 0 {
		return ""
	}
	for _, candidate := range candidates {
		if strings.Contains(strings.ToLower(filepath.Base(candidate)), "appicon") {
			return candidate
		}
	}
	return candidates[0]
}
