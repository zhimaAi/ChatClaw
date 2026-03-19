package channels

import (
	"context"
	"strings"

	"chatclaw/internal/sqlite"

	"github.com/uptrace/bun"
)

func UpdateChannelLastSenderID(ctx context.Context, db *bun.DB, channelID int64, senderID string) error {
	senderID = strings.TrimSpace(senderID)
	if channelID <= 0 || senderID == "" {
		return nil
	}

	_, err := db.NewUpdate().
		Model((*channelModel)(nil)).
		Set("last_sender_id = ?", senderID).
		Set("updated_at = ?", sqlite.NowUTC()).
		Where("id = ?", channelID).
		Exec(ctx)
	return err
}
