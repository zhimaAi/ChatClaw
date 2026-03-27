package channels

import (
	"fmt"
	"strconv"
	"strings"
)

const (
	channelConversationExternalIDPrefix = "ch"
	ChannelConversationScopeDM          = "dm"
	ChannelConversationScopeGroup       = "group"
)

type ChannelConversationSource struct {
	ChannelID int64
	Scope     string
	TargetID  string
}

// NormalizeChannelConversationTargetID lowercases trimmed target ids for stable keys and display fallbacks.
func NormalizeChannelConversationTargetID(targetID string) string {
	// WeCom / channel APIs may return the same chat or user id with inconsistent casing;
	// normalize so lookups and session sync always hit one conversation row.
	return strings.ToLower(strings.TrimSpace(targetID))
}

func normalizeChannelConversationTargetID(targetID string) string {
	return NormalizeChannelConversationTargetID(targetID)
}

func BuildChannelConversationExternalID(channelID int64, scope, targetID string) string {
	targetID = normalizeChannelConversationTargetID(targetID)
	scope = normalizeChannelConversationScope(scope)
	if channelID <= 0 || targetID == "" {
		return ""
	}
	if scope == "" {
		return fmt.Sprintf("%s:%d:%s", channelConversationExternalIDPrefix, channelID, targetID)
	}
	return fmt.Sprintf("%s:%d:%s:%s", channelConversationExternalIDPrefix, channelID, scope, targetID)
}

// CanonicalChannelConversationExternalID returns a stable external_id string: same logical
// channel chat always maps to one value even if legacy rows used mixed-case target segments.
func CanonicalChannelConversationExternalID(externalID string) string {
	externalID = strings.TrimSpace(externalID)
	if externalID == "" {
		return ""
	}
	src, ok := ParseChannelConversationExternalID(externalID)
	if !ok {
		return externalID
	}
	return BuildChannelConversationExternalID(src.ChannelID, src.Scope, src.TargetID)
}

func ParseChannelConversationExternalID(externalID string) (ChannelConversationSource, bool) {
	externalID = strings.TrimSpace(externalID)
	if !strings.HasPrefix(externalID, channelConversationExternalIDPrefix+":") {
		return ChannelConversationSource{}, false
	}

	parts := strings.SplitN(externalID, ":", 4)
	switch len(parts) {
	case 3:
		channelID, err := strconv.ParseInt(strings.TrimSpace(parts[1]), 10, 64)
		if err != nil || channelID <= 0 {
			return ChannelConversationSource{}, false
		}
		targetID := normalizeChannelConversationTargetID(parts[2])
		if targetID == "" {
			return ChannelConversationSource{}, false
		}
		return ChannelConversationSource{
			ChannelID: channelID,
			TargetID:  targetID,
		}, true
	case 4:
		channelID, err := strconv.ParseInt(strings.TrimSpace(parts[1]), 10, 64)
		if err != nil || channelID <= 0 {
			return ChannelConversationSource{}, false
		}
		scope := normalizeChannelConversationScope(parts[2])
		targetID := normalizeChannelConversationTargetID(parts[3])
		if targetID == "" {
			return ChannelConversationSource{}, false
		}
		return ChannelConversationSource{
			ChannelID: channelID,
			Scope:     scope,
			TargetID:  targetID,
		}, true
	default:
		return ChannelConversationSource{}, false
	}
}

func normalizeChannelConversationScope(scope string) string {
	switch strings.ToLower(strings.TrimSpace(scope)) {
	case ChannelConversationScopeDM:
		return ChannelConversationScopeDM
	case ChannelConversationScopeGroup:
		return ChannelConversationScopeGroup
	default:
		return ""
	}
}

// ConversationTitleFromFirstMessage derives a session title from the first inbound message
// when the platform does not provide a chat display name (e.g. WeCom group ChatName empty).
func ConversationTitleFromFirstMessage(content string) string {
	content = strings.TrimSpace(content)
	if content == "" {
		return ""
	}
	fields := strings.Fields(content)
	content = strings.Join(fields, " ")
	rs := []rune(content)
	if len(rs) > 100 {
		return string(rs[:100])
	}
	return content
}

// IsWeComGroupPlaceholderConversationName matches titles that should be replaced once we
// have first-message text (legacy "group:…", synced "group:…", or 「企微群」… fallback).
func IsWeComGroupPlaceholderConversationName(name string) bool {
	n := strings.TrimSpace(name)
	if n == "" {
		return true
	}
	if strings.HasPrefix(n, "group:") {
		return true
	}
	if strings.HasPrefix(n, "「企微群」") {
		return true
	}
	return false
}
