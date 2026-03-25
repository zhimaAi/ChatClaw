package channels

import (
	"context"
	"database/sql"
	"testing"

	_ "github.com/mattn/go-sqlite3"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/sqlitedialect"
)

func TestUpdateChannelLastReplyTargetPrefersChatID(t *testing.T) {
	db := newChannelsLastSenderTestDB(t)
	insertChannelLastSenderTestRow(t, db, 88, "")

	if err := UpdateChannelLastReplyTarget(context.Background(), db, 88, "cid_conversation_1", "ou_sender_1"); err != nil {
		t.Fatalf("UpdateChannelLastReplyTarget returned error: %v", err)
	}

	if got := readChannelLastSenderID(t, db, 88); got != "cid_conversation_1" {
		t.Fatalf("expected last_sender_id cid_conversation_1, got %q", got)
	}
}

func TestUpdateChannelLastReplyTargetFallsBackToSenderID(t *testing.T) {
	db := newChannelsLastSenderTestDB(t)
	insertChannelLastSenderTestRow(t, db, 89, "")

	if err := UpdateChannelLastReplyTarget(context.Background(), db, 89, "   ", "existing_sender"); err != nil {
		t.Fatalf("UpdateChannelLastReplyTarget returned error: %v", err)
	}

	if got := readChannelLastSenderID(t, db, 89); got != "existing_sender" {
		t.Fatalf("expected last_sender_id to remain existing_sender, got %q", got)
	}
}

func TestUpdateChannelLastReplyTargetSkipsEmptyReplyTarget(t *testing.T) {
	db := newChannelsLastSenderTestDB(t)
	insertChannelLastSenderTestRow(t, db, 90, "existing_target")

	if err := UpdateChannelLastReplyTarget(context.Background(), db, 90, "   ", "   "); err != nil {
		t.Fatalf("UpdateChannelLastReplyTarget returned error: %v", err)
	}

	if got := readChannelLastSenderID(t, db, 90); got != "existing_target" {
		t.Fatalf("expected last_sender_id to remain existing_target, got %q", got)
	}
}

func newChannelsLastSenderTestDB(t *testing.T) *bun.DB {
	t.Helper()

	sqlDB, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() {
		_ = sqlDB.Close()
	})

	db := bun.NewDB(sqlDB, sqlitedialect.New())
	t.Cleanup(func() {
		_ = db.Close()
	})

	if _, err := db.Exec(`
		create table channels (
			id integer primary key,
			last_sender_id text not null default '',
			updated_at datetime not null default current_timestamp
		);
	`); err != nil {
		t.Fatalf("create channels table failed: %v", err)
	}

	return db
}

func insertChannelLastSenderTestRow(t *testing.T, db *bun.DB, channelID int64, lastSenderID string) {
	t.Helper()
	if _, err := db.Exec(`insert into channels(id, last_sender_id) values (?, ?)`, channelID, lastSenderID); err != nil {
		t.Fatalf("insert channel failed: %v", err)
	}
}

func readChannelLastSenderID(t *testing.T, db *bun.DB, channelID int64) string {
	t.Helper()

	var lastSenderID string
	if err := db.NewSelect().
		Table("channels").
		Column("last_sender_id").
		Where("id = ?", channelID).
		Scan(context.Background(), &lastSenderID); err != nil {
		t.Fatalf("read last_sender_id failed: %v", err)
	}
	return lastSenderID
}
