package channels

import (
	"context"
	"strings"

	"chatclaw/internal/sqlite"

	"github.com/uptrace/bun"
)

// ResolveChannelReplyTarget prefers chat-scoped targets so follow-up notifications
// can reach the same conversation instead of a per-user sender identifier.
func ResolveChannelReplyTarget(chatID, senderID string) string {
	chatID = strings.TrimSpace(chatID)
	if chatID != "" {
		return chatID
	}
	return strings.TrimSpace(senderID)
}

// UpdateChannelLastReplyTarget stores the latest reply target in the legacy
// last_sender_id column for compatibility with existing notification logic.
func UpdateChannelLastReplyTarget(ctx context.Context, db *bun.DB, channelID int64, chatID, senderID string) error {
	replyTarget := ResolveChannelReplyTarget(chatID, senderID)
	if channelID <= 0 || replyTarget == "" {
		return nil
	}

	_, err := db.NewUpdate().
		Model((*channelModel)(nil)).
		Set("last_sender_id = ?", replyTarget).
		Set("updated_at = ?", sqlite.NowUTC()).
		Where("id = ?", channelID).
		Exec(ctx)
	return err
}
