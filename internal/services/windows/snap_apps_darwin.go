//go:build darwin

package windows

import (
	"os/exec"
	"sort"
	"strings"
)

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
        try
          set bundleID to bundle identifier of p
        end try
        set outText to outText & appName & "|" & bundleID & linefeed
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
	seen := make(map[string]struct{}, len(lines))
	apps := make([]SnapAppCandidate, 0, len(lines))

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "|", 2)
		name := strings.TrimSpace(parts[0])
		if name == "" {
			continue
		}

		processName := name
		if len(parts) > 1 {
			bundleID := strings.TrimSpace(parts[1])
			if bundleID != "" {
				processName = bundleID
			}
		}

		lower := strings.ToLower(processName)
		if _, exists := seen[lower]; exists {
			continue
		}
		seen[lower] = struct{}{}

		apps = append(apps, SnapAppCandidate{
			Name:        name,
			ProcessName: processName,
			Icon:        inferSnapIcon(name, processName),
		})
	}

	sort.Slice(apps, func(i, j int) bool {
		return strings.ToLower(apps[i].Name) < strings.ToLower(apps[j].Name)
	})
	return apps, nil
}
