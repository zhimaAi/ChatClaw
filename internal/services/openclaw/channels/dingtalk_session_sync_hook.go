package openclawchannels

import (
	"encoding/json"
	"strings"
	"time"

	"chatclaw/internal/services/channels"
)

const openClawPluginSessionSyncListenerKey = "openclaw-plugin-session-sync"

// OnGatewayReadyOpenClawPluginSessionSync registers a gateway listener so that when an
// OpenClaw plugin-managed run finishes (DingTalk, WeChat/weixin, etc.), we mirror
// sessions.json into local conversations.
func (s *OpenClawChannelService) OnGatewayReadyOpenClawPluginSessionSync() {
	if s == nil || s.openclawManager == nil {
		return
	}
	m := s.openclawManager
	m.RemoveEventListener(openClawPluginSessionSyncListenerKey)
	m.AddEventListener(openClawPluginSessionSyncListenerKey, func(event string, payload json.RawMessage) {
		s.handleGatewayEventForOpenClawPluginSessionSync(event, payload)
	})
}

func (s *OpenClawChannelService) handleGatewayEventForOpenClawPluginSessionSync(event string, payload json.RawMessage) {
	sessionKey := ""
	switch strings.TrimSpace(event) {
	case "agent":
		var frame struct {
			Stream     string          `json:"stream"`
			SessionKey string          `json:"sessionKey"`
			Data       json.RawMessage `json:"data"`
		}
		if json.Unmarshal(payload, &frame) != nil || strings.TrimSpace(frame.SessionKey) == "" {
			return
		}
		if strings.TrimSpace(frame.Stream) != "lifecycle" {
			return
		}
		var data struct {
			Phase string `json:"phase"`
		}
		if json.Unmarshal(frame.Data, &data) != nil {
			return
		}
		switch strings.TrimSpace(strings.ToLower(data.Phase)) {
		case "end", "error":
			sessionKey = frame.SessionKey
		default:
			return
		}
	case "chat":
		var frame struct {
			SessionKey string `json:"sessionKey"`
			State      string `json:"state"`
		}
		if json.Unmarshal(payload, &frame) != nil || strings.TrimSpace(frame.SessionKey) == "" {
			return
		}
		if strings.TrimSpace(strings.ToLower(frame.State)) != "final" {
			return
		}
		sessionKey = frame.SessionKey
	default:
		return
	}

	openclawAgentStr, platform, ok := parseOpenClawPluginSessionKeyPrefix(sessionKey)
	if !ok || !isOpenClawPluginSessionSyncPlatform(platform) {
		return
	}

	localID, err := s.agentsSvc.ResolveLocalIDByOpenClawAgentID(openclawAgentStr)
	if err != nil || localID <= 0 {
		return
	}

	go func(agentID int64) {
		// Allow OpenClaw to flush sessions.json after lifecycle end.
		time.Sleep(400 * time.Millisecond)
		if err := s.SyncAgentConversations(agentID); err != nil {
			if s.app != nil {
				s.app.Logger.Warn("openclaw: plugin session sync after gateway completion failed",
					"agentId", agentID, "error", err)
			}
			return
		}
		if s.app != nil {
			s.app.Event.Emit("conversations:changed", map[string]any{
				"agent_id": agentID,
			})
		}
	}(localID)
}

func parseOpenClawPluginSessionKeyPrefix(sessionKey string) (openclawAgentID, platform string, ok bool) {
	sessionKey = strings.TrimSpace(sessionKey)
	prefix := "agent:"
	if !strings.HasPrefix(sessionKey, prefix) {
		return "", "", false
	}
	parts := strings.Split(sessionKey, ":")
	if len(parts) < 5 {
		return "", "", false
	}
	return strings.TrimSpace(parts[1]), strings.TrimSpace(parts[2]), true
}

func isOpenClawPluginSessionSyncPlatform(platform string) bool {
	switch strings.ToLower(strings.TrimSpace(platform)) {
	case strings.ToLower(channels.PlatformDingTalk), "dingtalk-connector":
		return true
	case strings.ToLower(channels.PlatformWechat), "openclaw-weixin", "weixin":
		return true
	default:
		return false
	}
}
