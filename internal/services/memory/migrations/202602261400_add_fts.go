package migrations

import (
	"context"

	"github.com/uptrace/bun"
)

func init() {
	Migrations.MustRegister(
		func(ctx context.Context, db *bun.DB) error {
			sql := `
CREATE VIRTUAL TABLE IF NOT EXISTS thematic_facts_fts USING fts5(
    tokens,
    agent_id,
    content='',
    tokenize='unicode61'
);

CREATE VIRTUAL TABLE IF NOT EXISTS event_streams_fts USING fts5(
    tokens,
    agent_id,
    content='',
    tokenize='unicode61'
);
`
			if _, err := db.ExecContext(ctx, sql); err != nil {
				return err
			}
			return nil
		},
		func(ctx context.Context, db *bun.DB) error {
			sql := `
DROP TABLE IF EXISTS thematic_facts_fts;
DROP TABLE IF EXISTS event_streams_fts;
`
			if _, err := db.ExecContext(ctx, sql); err != nil {
				return err
			}
			return nil
		},
	)
}
