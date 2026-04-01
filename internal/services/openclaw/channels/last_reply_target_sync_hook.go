package openclawchannels

import (
	"encoding/json"
	"strings"

	"chatclaw/internal/services/channels"
)

const openClawLastReplyTargetSyncListenerKey = "openclaw-last-reply-target-sync"

// OnGatewayReadyOpenClawLastReplyTargetSync 注册统一的网关监听器。
// OnGatewayReadyOpenClawLastReplyTargetSync registers one gateway listener that updates
// the channel last reply target for plugin-managed platforms after OpenClaw emits sessionKey.
func (s *OpenClawChannelService) OnGatewayReadyOpenClawLastReplyTargetSync() {
	if s == nil || s.openclawManager == nil {
		return
	}
	m := s.openclawManager
	m.RemoveEventListener(openClawLastReplyTargetSyncListenerKey)
	m.AddEventListener(openClawLastReplyTargetSyncListenerKey, func(event string, payload json.RawMessage) {
		s.handleGatewayEventForOpenClawLastReplyTargetSync(event, payload)
	})
}

func (s *OpenClawChannelService) handleGatewayEventForOpenClawLastReplyTargetSync(event string, payload json.RawMessage) {
	sessionKey := s.extractSessionKeyFromGatewayEvent(event, payload)
	if sessionKey == "" {
		return
	}

	openclawAgentStr, platform, ok := parseOpenClawPluginSessionKeyPrefix(sessionKey)
	if !ok || !isOpenClawLastReplyTargetSyncPlatform(platform) {
		return
	}

	// 标准化平台名称，避免日志和后续分支同时处理别名。
	// Normalize platform aliases so logs and branching use the same canonical name.
	platform = canonicalOpenClawLastReplyTargetPlatform(platform)

	localID, err := s.agentsSvc.ResolveLocalIDByOpenClawAgentID(openclawAgentStr)
	if err != nil || localID <= 0 {
		return
	}

	go func(agentID int64, normalizedPlatform string, sk string) {
		if err := s.updateChannelLastReplyTargetBySessionKey(agentID, sk); err != nil {
			if s.app != nil {
				s.app.Logger.Warn("openclaw: immediate last reply target update failed",
					"platform", normalizedPlatform, "agentId", agentID, "sessionKey", sk, "error", err)
			}
			return
		}
		if s.app != nil {
			s.app.Logger.Info("openclaw: immediate last reply target updated",
				"platform", normalizedPlatform, "agentId", agentID, "sessionKey", sk)
		}
	}(localID, platform, sessionKey)
}

// extractSessionKeyFromGatewayEvent 从 OpenClaw 网关事件里提取 sessionKey。
// extractSessionKeyFromGatewayEvent extracts sessionKey from OpenClaw agent/chat events.
func (s *OpenClawChannelService) extractSessionKeyFromGatewayEvent(event string, payload json.RawMessage) string {
	switch strings.TrimSpace(event) {
	case "agent":
		var frame struct {
			SessionKey string `json:"sessionKey"`
		}
		if json.Unmarshal(payload, &frame) != nil || strings.TrimSpace(frame.SessionKey) == "" {
			return ""
		}
		return strings.TrimSpace(frame.SessionKey)
	case "chat":
		var frame struct {
			SessionKey string `json:"sessionKey"`
		}
		if json.Unmarshal(payload, &frame) != nil || strings.TrimSpace(frame.SessionKey) == "" {
			return ""
		}
		return strings.TrimSpace(frame.SessionKey)
	default:
		return ""
	}
}

// isOpenClawLastReplyTargetSyncPlatform 判断该平台是否需要即时刷新 last_sender_id/chat_id。
// isOpenClawLastReplyTargetSyncPlatform decides whether a platform should refresh
// last_sender_id/chat_id immediately from sessionKey.
func isOpenClawLastReplyTargetSyncPlatform(platform string) bool {
	switch canonicalOpenClawLastReplyTargetPlatform(platform) {
	case channels.PlatformDingTalk, channels.PlatformFeishu, channels.PlatformWeCom, channels.PlatformQQ:
		return true
	default:
		return false
	}
}

// canonicalOpenClawLastReplyTargetPlatform 将 OpenClaw sessionKey 里的平台别名归一化。
// canonicalOpenClawLastReplyTargetPlatform normalizes platform aliases found in sessionKey.
func canonicalOpenClawLastReplyTargetPlatform(platform string) string {
	switch strings.ToLower(strings.TrimSpace(platform)) {
	case strings.ToLower(channels.PlatformDingTalk), "dingtalk-connector":
		return channels.PlatformDingTalk
	case strings.ToLower(channels.PlatformFeishu):
		return channels.PlatformFeishu
	case strings.ToLower(channels.PlatformWeCom):
		return channels.PlatformWeCom
	case strings.ToLower(channels.PlatformQQ), openClawQQChannelID:
		return channels.PlatformQQ
	default:
		return strings.TrimSpace(platform)
	}
}
