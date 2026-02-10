package settings

import (
	"context"
	"database/sql"
	"time"

	"willclaw/internal/sqlite"

	"github.com/uptrace/bun"
)

type settingModel struct {
	bun.BaseModel `bun:"table:settings,alias:s"`

	Key         string         `bun:"key,pk" json:"key"`
	Value       sql.NullString `bun:"value,nullzero" json:"-"`
	Type        string         `bun:"type" json:"type"`
	Category    string         `bun:"category" json:"category"`
	Description sql.NullString `bun:"description,nullzero" json:"-"`
	CreatedAt   time.Time      `bun:"created_at" json:"created_at"`
	UpdatedAt   time.Time      `bun:"updated_at" json:"updated_at"`
}

// BeforeInsert 在 INSERT 时自动设置 created_at 和 updated_at（字符串格式）
var _ bun.BeforeInsertHook = (*settingModel)(nil)

func (*settingModel) BeforeInsert(ctx context.Context, query *bun.InsertQuery) error {
	now := sqlite.NowUTC()
	query.Value("created_at", "?", now)
	query.Value("updated_at", "?", now)
	return nil
}

// BeforeUpdate 在 UPDATE 时自动设置 updated_at（字符串格式）
var _ bun.BeforeUpdateHook = (*settingModel)(nil)

func (*settingModel) BeforeUpdate(ctx context.Context, query *bun.UpdateQuery) error {
	query.Set("updated_at = ?", sqlite.NowUTC())
	return nil
}

func (m *settingModel) toDTO() Setting {
	out := Setting{
		Key:       m.Key,
		Type:      m.Type,
		Category:  Category(m.Category),
		CreatedAt: m.CreatedAt,
		UpdatedAt: m.UpdatedAt,
	}
	if m.Value.Valid {
		out.Value = m.Value.String
	}
	if m.Description.Valid {
		out.Description = m.Description.String
	}
	return out
}
