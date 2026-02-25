//go:build windows

package windows

import (
	"encoding/csv"
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
		`Get-Process | `+
			`Where-Object { $_.MainWindowHandle -ne 0 -and $_.MainWindowTitle -ne "" } | `+
			`Select-Object -Unique @{Name="ProcessName";Expression={$_.ProcessName}} | `+
			`ConvertTo-Csv -NoTypeInformation`)
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	reader := csv.NewReader(strings.NewReader(string(output)))
	seen := make(map[string]struct{})
	apps := make([]SnapAppCandidate, 0, 128)

	for {
		record, readErr := reader.Read()
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			continue
		}
		if len(record) == 0 {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(record[0]), "ProcessName") {
			continue
		}

		rawProcessName := strings.TrimSpace(record[0])
		rawProcessName = strings.Trim(rawProcessName, "\"")
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
