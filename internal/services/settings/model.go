package settings

import (
	"context"
	"database/sql"
	"time"

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

// BeforeAppendModel 让 Bun 自动维护 created_at / updated_at
func (m *settingModel) BeforeAppendModel(ctx context.Context, query bun.Query) error {
	_ = ctx
	now := time.Now().UTC()

	switch query.(type) {
	case *bun.InsertQuery:
		if m.CreatedAt.IsZero() {
			m.CreatedAt = now
		}
		m.UpdatedAt = now
	case *bun.UpdateQuery:
		m.UpdatedAt = now
	}
	return nil
}

func (m *settingModel) toDTO() Setting {
	out := Setting{
		Key:       m.Key,
		Type:      m.Type,
		Category:  m.Category,
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
