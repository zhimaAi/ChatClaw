package windows

import "strings"

func inferSnapIcon(name, processName string) string {
	lower := strings.ToLower(strings.TrimSpace(name) + " " + strings.TrimSpace(processName))
	switch {
	case strings.Contains(lower, "weixin"), strings.Contains(lower, "wechat"), strings.Contains(lower, "xinwechat"):
		return "wechat"
	case strings.Contains(lower, "wecom"), strings.Contains(lower, "wework"), strings.Contains(lower, "wxwork"), strings.Contains(lower, "qiyeweixin"):
		return "wecom"
	case strings.Contains(lower, "qq"):
		return "qq"
	case strings.Contains(lower, "dingtalk"):
		return "dingtalk"
	case strings.Contains(lower, "feishu"), strings.Contains(lower, "lark"):
		return "feishu"
	case strings.Contains(lower, "douyin"):
		return "douyin"
	default:
		return "app"
	}
}
