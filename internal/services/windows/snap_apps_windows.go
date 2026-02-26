//go:build windows

package windows

import (
	"encoding/base64"
	"encoding/csv"
	"encoding/json"
	"io"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

var windowsProcessDisplayNameMap = map[string]string{
	"weixin.exe":      "微信",
	"wechat.exe":      "微信",
	"wechatapp.exe":   "微信",
	"wechatappex.exe": "微信",
	"wxwork.exe":      "企业微信",
	"qq.exe":          "QQ",
	"qqnt.exe":        "QQ",
	"dingtalk.exe":    "钉钉",
	"feishu.exe":      "飞书",
	"lark.exe":        "飞书",
	"douyin.exe":      "抖音",
}

var windowsExcludedProcessPrefixes = []string{
	"applicationframehost",
	"shellexperiencehost",
	"searchhost",
	"startmenuexperiencehost",
	"textinputhost",
	"windowsinternal.",
}

type windowsAppItem struct {
	ProcessNameB64 string `json:"ProcessNameB64"`
	WindowTitleB64 string `json:"WindowTitleB64"`
}

func decodeWindowsB64(value string) string {
	if strings.TrimSpace(value) == "" {
		return ""
	}
	raw, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(raw))
}

func parseWindowsAppItems(output []byte) []windowsAppItem {
	var items []windowsAppItem
	if err := json.Unmarshal(output, &items); err == nil {
		return items
	}
	var single windowsAppItem
	if err := json.Unmarshal(output, &single); err == nil && single.ProcessNameB64 != "" {
		return []windowsAppItem{single}
	}
	// Fallback to CSV for safety if JSON parse fails unexpectedly.
	reader := csv.NewReader(strings.NewReader(string(output)))
	for {
		record, readErr := reader.Read()
		if readErr == io.EOF {
			break
		}
		if readErr != nil || len(record) == 0 {
			continue
		}
		processName := strings.Trim(strings.TrimSpace(record[0]), "\"")
		if strings.EqualFold(processName, "ProcessName") || processName == "" {
			continue
		}
		items = append(items, windowsAppItem{
			ProcessNameB64: base64.StdEncoding.EncodeToString([]byte(processName)),
		})
	}
	return items
}

func resolveWindowsDisplayName(processName string) string {
	normalized := strings.ToLower(strings.TrimSpace(processName))
	if displayName, ok := windowsProcessDisplayNameMap[normalized]; ok && strings.TrimSpace(displayName) != "" {
		return displayName
	}
	trimmed := strings.TrimSuffix(processName, filepath.Ext(processName))
	if strings.TrimSpace(trimmed) != "" {
		return trimmed
	}
	return processName
}

func listRunningApps() ([]SnapAppCandidate, error) {
	// Keep only processes that own a main window, which matches the
	// "Apps" section in Windows Task Manager and excludes background services.
	cmd := exec.Command("powershell", "-NoProfile", "-Command",
		`$apps = Get-Process | `+
			`Where-Object { $_.MainWindowHandle -ne 0 -and $_.MainWindowTitle -ne "" } | `+
			`Select-Object -Unique ProcessName,MainWindowTitle | `+
			`ForEach-Object { [PSCustomObject]@{ `+
			`ProcessNameB64 = [Convert]::ToBase64String([Text.Encoding]::UTF8.GetBytes($_.ProcessName)); `+
			`WindowTitleB64 = [Convert]::ToBase64String([Text.Encoding]::UTF8.GetBytes($_.MainWindowTitle)) `+
			`} }; `+
			`$apps | ConvertTo-Json -Compress`)
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	items := parseWindowsAppItems(output)
	seen := make(map[string]struct{})
	apps := make([]SnapAppCandidate, 0, 128)

	for _, item := range items {
		rawProcessName := decodeWindowsB64(item.ProcessNameB64)
		if rawProcessName == "" {
			continue
		}

		processName := rawProcessName
		if filepath.Ext(processName) == "" {
			processName += ".exe"
		}
		if processName == "" {
			continue
		}
		lowerProcess := strings.ToLower(processName)
		if lowerProcess == "system idle process" ||
			lowerProcess == "system" ||
			lowerProcess == "registry" ||
			lowerProcess == "memory compression" {
			continue
		}
		shouldSkip := false
		for _, prefix := range windowsExcludedProcessPrefixes {
			if strings.HasPrefix(lowerProcess, prefix) {
				shouldSkip = true
				break
			}
		}
		if shouldSkip {
			continue
		}
		if _, exists := seen[lowerProcess]; exists {
			continue
		}
		seen[lowerProcess] = struct{}{}

		displayName := resolveWindowsDisplayName(processName)

		apps = append(apps, SnapAppCandidate{
			Name:        displayName,
			ProcessName: processName,
			Icon:        inferSnapIcon(displayName, processName),
		})
	}

	sort.Slice(apps, func(i, j int) bool {
		if strings.EqualFold(apps[i].Name, apps[j].Name) {
			return strings.ToLower(apps[i].ProcessName) < strings.ToLower(apps[j].ProcessName)
		}
		return strings.ToLower(apps[i].Name) < strings.ToLower(apps[j].Name)
	})
	return apps, nil
}
