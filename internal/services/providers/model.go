package providers

import (
	"context"
	"time"

	"github.com/uptrace/bun"
)

// Provider 供应商 DTO（暴露给前端）
type Provider struct {
	ID          int64     `json:"id"`
	ProviderID  string    `json:"provider_id"`
	Name        string    `json:"name"`
	Type        string    `json:"type"`
	Icon        string    `json:"icon"`
	IsBuiltin   bool      `json:"is_builtin"`
	Enabled     bool      `json:"enabled"`
	SortOrder   int       `json:"sort_order"`
	APIEndpoint string    `json:"api_endpoint"`
	APIKey      string    `json:"api_key"`
	ExtraConfig string    `json:"extra_config"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// Model 模型 DTO（暴露给前端）
type Model struct {
	ID         int64     `json:"id"`
	ProviderID string    `json:"provider_id"`
	ModelID    string    `json:"model_id"`
	Name       string    `json:"name"`
	Type       string    `json:"type"` // llm, embedding
	IsBuiltin  bool      `json:"is_builtin"`
	Enabled    bool      `json:"enabled"`
	SortOrder  int       `json:"sort_order"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// ModelGroup 模型分组（按类型分组）
type ModelGroup struct {
	Type   string  `json:"type"`
	Models []Model `json:"models"`
}

// ProviderWithModels 供应商及其模型
type ProviderWithModels struct {
	Provider    Provider     `json:"provider"`
	ModelGroups []ModelGroup `json:"model_groups"`
}

// UpdateProviderInput 更新供应商的输入参数
type UpdateProviderInput struct {
	Enabled     *bool   `json:"enabled"`
	APIKey      *string `json:"api_key"`
	APIEndpoint *string `json:"api_endpoint"`
	ExtraConfig *string `json:"extra_config"`
}

// providerModel 数据库模型
type providerModel struct {
	bun.BaseModel `bun:"table:providers,alias:p"`

	ID          int64     `bun:"id,pk,autoincrement"`
	ProviderID  string    `bun:"provider_id,notnull"`
	Name        string    `bun:"name,notnull"`
	Type        string    `bun:"type,notnull"`
	Icon        string    `bun:"icon,notnull"`
	IsBuiltin   bool      `bun:"is_builtin,notnull"`
	Enabled     bool      `bun:"enabled,notnull"`
	SortOrder   int       `bun:"sort_order,notnull"`
	APIEndpoint string    `bun:"api_endpoint,notnull"`
	APIKey      string    `bun:"api_key,notnull"`
	ExtraConfig string    `bun:"extra_config,notnull"`
	CreatedAt   time.Time `bun:"created_at,notnull"`
	UpdatedAt   time.Time `bun:"updated_at,notnull"`
}

func (m *providerModel) BeforeAppendModel(ctx context.Context, query bun.Query) error {
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

func (m *providerModel) toDTO() Provider {
	return Provider{
		ID:          m.ID,
		ProviderID:  m.ProviderID,
		Name:        m.Name,
		Type:        m.Type,
		Icon:        m.Icon,
		IsBuiltin:   m.IsBuiltin,
		Enabled:     m.Enabled,
		SortOrder:   m.SortOrder,
		APIEndpoint: m.APIEndpoint,
		APIKey:      m.APIKey,
		ExtraConfig: m.ExtraConfig,
		CreatedAt:   m.CreatedAt,
		UpdatedAt:   m.UpdatedAt,
	}
}

// modelModel 数据库模型
type modelModel struct {
	bun.BaseModel `bun:"table:models,alias:m"`

	ID         int64     `bun:"id,pk,autoincrement"`
	ProviderID string    `bun:"provider_id,notnull"`
	ModelID    string    `bun:"model_id,notnull"`
	Name       string    `bun:"name,notnull"`
	Type       string    `bun:"type,notnull"`
	IsBuiltin  bool      `bun:"is_builtin,notnull"`
	Enabled    bool      `bun:"enabled,notnull"`
	SortOrder  int       `bun:"sort_order,notnull"`
	CreatedAt  time.Time `bun:"created_at,notnull"`
	UpdatedAt  time.Time `bun:"updated_at,notnull"`
}

func (m *modelModel) BeforeAppendModel(ctx context.Context, query bun.Query) error {
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

func (m *modelModel) toDTO() Model {
	return Model{
		ID:         m.ID,
		ProviderID: m.ProviderID,
		ModelID:    m.ModelID,
		Name:       m.Name,
		Type:       m.Type,
		IsBuiltin:  m.IsBuiltin,
		Enabled:    m.Enabled,
		SortOrder:  m.SortOrder,
		CreatedAt:  m.CreatedAt,
		UpdatedAt:  m.UpdatedAt,
	}
}
