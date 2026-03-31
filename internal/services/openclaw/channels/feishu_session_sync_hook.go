package openclawchannels

import (
	"encoding/json"
	"strings"
	"time"

	"chatclaw/internal/services/channels"
)

const feishuOpenClawSessionSyncListenerKey = "openclaw-feishu-session-sync"

// OnGatewayReadyFeishuSessionSync registers a gateway listener so that when a Feishu
// plugin-managed OpenClaw run finishes, we mirror sessions.json into local conversations.
func (s *OpenClawChannelService) OnGatewayReadyFeishuSessionSync() {
	if s == nil || s.openclawManager == nil {
		return
	}
	m := s.openclawManager
	m.RemoveEventListener(feishuOpenClawSessionSyncListenerKey)
	m.AddEventListener(feishuOpenClawSessionSyncListenerKey, func(event string, payload json.RawMessage) {
		s.handleGatewayEventForFeishuSessionSync(event, payload)
	})
}

func (s *OpenClawChannelService) handleGatewayEventForFeishuSessionSync(event string, payload json.RawMessage) {
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
	if !ok || !isFeishuSessionPlatform(platform) {
		return
	}

	localID, err := s.agentsSvc.ResolveLocalIDByOpenClawAgentID(openclawAgentStr)
	if err != nil || localID <= 0 {
		return
	}

	go func(agentID int64) {
		time.Sleep(400 * time.Millisecond)
		if err := s.SyncAgentConversations(agentID); err != nil {
			if s.app != nil {
				s.app.Logger.Warn("openclaw: feishu session sync after gateway completion failed",
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

func isFeishuSessionPlatform(platform string) bool {
	return strings.EqualFold(strings.TrimSpace(platform), channels.PlatformFeishu)
}
