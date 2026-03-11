package migrations

import (
	"context"

	"github.com/uptrace/bun"
)

func init() {
	Migrations.MustRegister(
		func(ctx context.Context, db *bun.DB) error {
			sql := `
-- team_type: person | team, default person
ALTER TABLE conversations ADD COLUMN team_type VARCHAR(20) NOT NULL DEFAULT 'person';
-- dialogue_id: team mode only, links to team dialogue
ALTER TABLE conversations ADD COLUMN dialogue_id INTEGER NOT NULL DEFAULT 0;
`
			if _, err := db.ExecContext(ctx, sql); err != nil {
				return err
			}
			return nil
		},
		func(ctx context.Context, db *bun.DB) error {
			// SQLite does not support dropping columns directly; keep the columns on rollback.
			return nil
		},
	)
}
